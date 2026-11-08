package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var web embed.FS

// Default listen address; tests and default runs rely on it.
const host = "127.0.0.1:8787"
const origin = "http://" + host

type App struct {
	mail                         *mailService
	store                        *Store
	catalog, builder, out, token string
	templates                    *template.Template
	inviteKey                    string
	addr                         string // Actual listen address; Host/Origin checks use it.
	building                     sync.Mutex // Held for the full navigation task.
}
type ViewRow struct {
	Provider            Provider
	Current, Input      string
	Configured, Editing bool
}
type View struct {
	Rows                            []ViewRow
	Token, Revision, Message, Error string
}

func (a *App) page(w http.ResponseWriter, status int, message, problem, id, input string) {
	a.store.mu.Lock()
	r, rev, e := a.store.load()
	a.store.mu.Unlock()
	if e != nil {
		http.Error(w, "配置读取失败，请保留原文件并修复后重试", 500)
		return
	}
	v := View{Token: a.token, Revision: rev, Message: message, Error: problem}
	for _, p := range a.store.providers {
		raw := r.Invitations[p.ID]
		current := p.URL
		if raw != "" {
			current = raw
		}
		editInput := raw
		if id == p.ID {
			editInput = input
		}
		v.Rows = append(v.Rows, ViewRow{p, current, editInput, raw != "", id == p.ID})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if e = a.templates.ExecuteTemplate(w, "index.gohtml", v); e != nil {
		log.Print("管理员页面渲染失败")
	}
}
func (a *App) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != a.addr {
		http.Error(w, "不允许此 Host", 403)
		return
	}
	private := len(a.inviteKey)==64 && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Invite-Admin")), []byte(a.inviteKey)) == 1
	if !private {
		switch r.URL.Path {
		case "/":
			if r.Method != "GET" && r.Method != "HEAD" {
				http.Error(w, "方法不允许", 405)
				return
			}
			http.Redirect(w, r, "/preview/", http.StatusSeeOther)
			return
		case "/save", "/restore", "/build", "/backup", "/styles.css", "/admin.js":
			http.NotFound(w, r)
			return
		}
	}
	switch r.URL.Path {
	case "/":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "只允许 GET", 405)
			return
		}
		message := ""
		switch r.URL.Query().Get("notice") {
		case "saved":
			message = "链接配置已保存，尚未更新导航；请点击“更新本地导航”。"
		case "built":
			message = "本地导航已更新，可打开预览核对购买入口。"
		}
		a.page(w, 200, message, "", "", "")
	case "/styles.css", "/admin.js":
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "方法不允许", 405)
			return
		}
		b, e := web.ReadFile("web" + r.URL.Path)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		if r.Method == "GET" {
			w.Write(b)
		}
	case "/backup":
		if r.Method != "GET" {
			http.Error(w, "只允许 GET", 405)
			return
		}
		a.store.mu.Lock()
		data, _, e := a.store.load()
		a.store.mu.Unlock()
		if e != nil {
			http.Error(w, "配置读取失败", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="referrals.backup.json"`)
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(data)
	case "/save", "/restore", "/build":
		a.mutate(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/mail-api/") {
			a.mailAPI(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/preview/") {
			a.preview(w, r)
			return
		}
		http.NotFound(w, r)
	}
}
func (a *App) mutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "修改只接受 POST", 405)
		return
	}
	if r.Header.Get("Origin") != "http://"+a.addr {
		http.Error(w, "请求来源不允许", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if e := r.ParseForm(); e != nil {
		http.Error(w, "表单过大或格式错误", 413)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(a.token)) != 1 {
		http.Error(w, "表单校验失败，请刷新页面", 403)
		return
	}
	for k, values := range r.PostForm {
		if len(values) != 1 || !(k == "csrf" || k == "revision" || k == "id" || k == "url") {
			http.Error(w, "不支持的表单字段", 400)
			return
		}
	}
	if r.URL.Path == "/build" {
		if !a.building.TryLock() {
			a.page(w, 409, "", "已有导航更新任务，请稍后刷新", "", "")
			return
		}
		defer a.building.Unlock()
		e := a.build(r.Context())
		if e != nil {
			var exit *exec.ExitError
			if errors.As(e, &exit) {
				log.Printf("导航生成任务失败，退出码 %d", exit.ExitCode())
			} else {
				log.Print("导航生成任务失败，无法启动或读取配置")
			}
			a.page(w, 500, "", "链接配置仍已保存；导航生成失败，仍显示上一版。请确认生成器已构建：go build -trimpath -o bin/sitegen.exe ./cmd/sitegen", "", "")
		} else {
			log.Print("导航生成任务成功")
			http.Redirect(w, r, "/?notice=built", http.StatusSeeOther)
		}
		return
	}
	id, raw := r.PostForm.Get("id"), r.PostForm.Get("url")
	e := a.store.update(id, raw, r.PostForm.Get("revision"), r.URL.Path == "/restore")
	if e != nil {
		status := 400
		problem := "保存失败，请检查输入；原配置未被覆盖"
		if errors.Is(e, errConflict) {
			status = 409
			problem = e.Error()
		} else if p, ok := a.store.provider(id); ok && r.URL.Path == "/save" {
			if ve := validateURL(raw, p.Hosts); ve != nil {
				problem = ve.Error()
			}
		}
		a.page(w, status, "", problem, id, raw)
		return
	}
	http.Redirect(w, r, "/?notice=saved", http.StatusSeeOther)
}
func (a *App) build(parent context.Context) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if _, _, e := a.store.load(); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, a.builder, "-catalog", a.catalog, "-referrals", a.store.path, "-out", a.out).Run()
}
func (a *App) preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "预览只读", 405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/preview/")
	if name == "" {
		name = "index.html"
	}
	if name != "index.html" && name != "styles.css" && name != "app.js" {
		http.NotFound(w, r)
		return
	}
	root, e := os.OpenRoot(a.out)
	if e != nil {
		http.Error(w, "导航尚未生成，请更新本地导航", 404)
		return
	}
	defer root.Close()
	info, e := root.Lstat(name)
	if e != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, e := root.Open(name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), f)
}
func newApp(catalog, referrals, builder, out string) (*App, error) {
	providers, e := loadProviders(catalog)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(referrals), 0700); e != nil {
		return nil, e
	}
	if _, e = os.Stat(referrals); os.IsNotExist(e) {
		f, e := os.OpenFile(referrals, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, we := f.WriteString("{\"schema_version\":1,\"invitations\":{}}\n")
		ce := f.Close()
		if we != nil || ce != nil {
			return nil, errors.Join(we, ce)
		}
	} else if e != nil {
		return nil, e
	}
	s := &Store{path: referrals, providers: providers}
	if _, _, e = s.load(); e != nil {
		return nil, e
	}
	t, e := template.ParseFS(web, "web/index.gohtml")
	if e != nil {
		return nil, e
	}
	var token [32]byte
	if _, e = rand.Read(token[:]); e != nil {
		return nil, e
	}
	var inviteKey [32]byte
	if _,e=rand.Read(inviteKey[:]);e!=nil{return nil,e}
	return &App{store: s, catalog: catalog, builder: builder, out: out, token: hex.EncodeToString(token[:]), inviteKey:hex.EncodeToString(inviteKey[:]), templates: t, addr: host, mail: newMailService(filepath.Dir(referrals))}, nil
}
func main() {
	catalog := flag.String("catalog", "", "目录 JSON（必填）")
	referrals := flag.String("referrals", "", "私有邀请文件（必填）")
	builder := flag.String("builder", "", "sitegen 可执行文件（必填）")
	out := flag.String("site-out", "", "静态产物目录（必填）")
	addr := flag.String("addr", host, "监听地址，例如 127.0.0.1:8787")
	flag.Parse()
	for _, p := range []*string{catalog, referrals, builder, out} {
		if *p == "" {
			flag.Usage()
			os.Exit(2)
		}
		abs, e := filepath.Abs(*p)
		if e != nil {
			log.Fatal("路径无效")
		}
		*p = abs
	}
	app, e := newApp(*catalog, *referrals, *builder, *out)
	if e != nil {
		log.Fatal("启动失败：请检查目录与邀请配置，原文件不会自动重置")
	}
	server := &http.Server{Addr: *addr, Handler: http.HandlerFunc(app.handler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	listener, e := net.Listen("tcp", *addr)
	if e != nil {
		log.Fatal("启动失败：端口已被占用或不可用")
	}
	if e = writeAtomic(filepath.Join(filepath.Dir(*referrals), ".invite-admin.key"), []byte(app.inviteKey)); e != nil {
		listener.Close()
		log.Fatal("无法初始化私有管理通道")
	}
	// Record the actual address so the separate invite tool can reach the private channel.
	if e = writeAtomic(filepath.Join(filepath.Dir(*referrals), ".invite-admin.port"), []byte(*addr)); e != nil {
		listener.Close()
		log.Fatal("无法记录管理端口")
	}
	app.addr = *addr
	fmt.Println("identity：http://" + *addr + "/preview/")
	log.Fatal(server.Serve(listener))
}
