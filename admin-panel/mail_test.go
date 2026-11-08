package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeChannel struct {
	name      string
	interval  time.Duration
	fail      bool
	created   int
	attempts  int
	polls     int
	closed    int
	domain    string
	attach    []string
	expiresAt time.Time
	msgs      []mailMessage
	detail    mailMessage
	details   int
	detailErr bool
}

func (f *fakeChannel) Meta() mailProviderMeta {
	return mailProviderMeta{Name: f.name, DisplayName: "假渠道 " + f.name, Tier: "free", Retention: "1h", Features: mailFeatures{PollInbox: true}}
}
func (f *fakeChannel) PollInterval() time.Duration { return f.interval }
func (f *fakeChannel) AttachDomains() []string     { return f.attach }
func (f *fakeChannel) Create(domain, username string) (mailInboxData, error) {
	f.attempts++
	if f.fail {
		return mailInboxData{}, fmt.Errorf("%s 上游拒绝创建", f.name)
	}
	f.created++
	f.domain = domain
	address := fmt.Sprintf("box%d@fake.test", f.created)
	if domain != "" && username != "" {
		address = username + "@" + domain
	}
	expires := f.expiresAt
	if expires.IsZero() {
		expires = time.Now().Add(time.Hour)
	}
	return mailInboxData{Provider: f.name, Address: address, AuthData: map[string]string{"token": "private-token"}, ExpiresAt: expires}, nil
}
func (f *fakeChannel) Messages(mailInboxData) ([]mailMessage, error) { f.polls++; return f.msgs, nil }
func (f *fakeChannel) Message(mailInboxData, string) (mailMessage, error) {
	f.details++
	if f.detailErr {
		return mailMessage{}, fmt.Errorf("detail unavailable")
	}
	return f.detail, nil
}
func (f *fakeChannel) Close(mailInboxData) error { f.closed++; return nil }

func newTestMailService(dir string) *mailService {
	return &mailService{store: newMailStore(filepath.Join(dir, "mail.json")), polls: map[string]*mailPoll{}}
}

// fakeImap 模拟自定义邮箱（IMAP）渠道：Connect 只校验是否拿到授权码。
type fakeImap struct {
	*fakeChannel
	connected int
	loginAs   string
}

func (f *fakeImap) Connect(address, password, host string) (mailInboxData, error) {
	if f.fail {
		return mailInboxData{}, fmt.Errorf("自定义邮箱登录失败，请检查地址与应用专用密码")
	}
	f.connected++
	f.loginAs = address
	return mailInboxData{Provider: "imap", Address: address, AuthData: map[string]string{"host": "imap.gmail.com", "port": "993", "username": address, "password": password}}, nil
}

func newTestApp(t *testing.T) (*App, *fakeChannel, *fakeChannel, string) {
	t.Helper()
	dir := t.TempDir()
	first := &fakeChannel{name: "mailtm", interval: time.Second}
	second := &fakeChannel{name: "mailgw", interval: time.Second}
	a := &App{token: "csrf-test", addr: host, mail: newTestMailService(dir)}
	a.mail.channels = []mailChannel{first, second}
	return a, first, second, dir
}

func mailCall(a *App, method, path, body, requestOrigin, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, origin+"/mail-api"+path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("Origin", requestOrigin)
	r.Header.Set("X-CSRF-Token", token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handler(w, r)
	return w
}

func mailDecode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatalf("响应不是 JSON 对象：%s", w.Body.String())
	}
	return m
}

func TestMailLifecycle(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.msgs = []mailMessage{{ID: "m1", From: "no-reply@example.com", Subject: "验证码", Excerpt: "482910", ReceivedAt: "2026-11-08T00:00:00Z"}}
	first.detail = mailMessage{ID: "m1", From: "no-reply@example.com", Subject: "验证码: 482910", Text: "Your verification code: 482910", ReceivedAt: "2026-11-08T00:00:00Z"}

	if w := mailCall(a, "GET", "/status", "", origin, a.token); w.Code != 200 || !strings.Contains(w.Body.String(), "csrf-test") {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	if w := mailCall(a, "GET", "/providers", "", origin, a.token); w.Code != 200 || !strings.Contains(w.Body.String(), `"mailtm"`) || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("providers: %d %s", w.Code, w.Body.String())
	}
	if w := mailCall(a, "GET", "/dispatch/preview?for=example.com", "", origin, a.token); w.Code != 200 || !strings.Contains(w.Body.String(), `"eligible":true`) {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}

	w := mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := mailDecode(t, w)
	id, _ := created["id"].(string)
	if id == "" || created["address"] != "box1@fake.test" || created["provider"] != "mailtm" {
		t.Fatalf("create response: %v", created)
	}
	if created["expiresAt"] == nil {
		t.Fatal("缺少 expiresAt")
	}

	w = mailCall(a, "GET", "/inboxes?status=active&pageSize=100", "", origin, a.token)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	list := mailDecode(t, w)
	inboxes := list["inboxes"].([]any)
	if len(inboxes) != 1 || list["total"].(float64) != 1 {
		t.Fatalf("list response: %v", list)
	}
	row := inboxes[0].(map[string]any)
	if row["id"] != id || row["target_service"] != "example.com" || row["status"] != "active" || row["state"] != "active" {
		t.Fatalf("list row: %v", row)
	}

	w = mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"active"`) || !strings.Contains(w.Body.String(), `"m1"`) {
		t.Fatalf("messages: %d %s", w.Code, w.Body.String())
	}
	w = mailCall(a, "GET", "/inbox/"+id+"/messages/m1", "", origin, a.token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "482910") {
		t.Fatalf("message detail: %d %s", w.Code, w.Body.String())
	}

	w = mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token)
	if w.Code != 200 {
		t.Fatalf("code: %d %s", w.Code, w.Body.String())
	}
	codeData := mailDecode(t, w)
	codes, _ := codeData["codes"].([]any)
	if len(codes) == 0 || codes[0].(map[string]any)["value"] != "482910" {
		t.Fatalf("code response: %v", codeData)
	}

	if w = mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if first.closed != 1 {
		t.Fatal("关闭未释放上游资源")
	}
	if w = mailCall(a, "GET", "/inboxes?status=active", "", origin, a.token); strings.Contains(w.Body.String(), id) {
		t.Fatal("已关闭收件箱仍在有效列表")
	}
	if w = mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token); !strings.Contains(w.Body.String(), `"status":"closed"`) {
		t.Fatalf("关闭状态未上报：%d %s", w.Code, w.Body.String())
	}
	if w = mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token); w.Code != 409 {
		t.Fatalf("已关闭收件箱仍可取码：%d", w.Code)
	}
}

func TestMailCreateFallsBackAndFails(t *testing.T) {
	a, first, second, _ := newTestApp(t)
	first.fail = true
	w := mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"provider":"mailgw"`) {
		t.Fatalf("自动调度未落到下一渠道：%d %s", w.Code, w.Body.String())
	}
	if second.created != 1 {
		t.Fatal("第二渠道未被创建")
	}

	second.fail = true
	w = mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "假渠道 mailtm") || !strings.Contains(w.Body.String(), "假渠道 mailgw") {
		t.Fatalf("全部失败未如实报告：%d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-token") {
		t.Fatal("错误信息泄露渠道凭据")
	}

	if w = mailCall(a, "POST", "/inbox", `{"for":"example.com","provider":"nope"}`, origin, a.token); w.Code != 400 {
		t.Fatalf("未知渠道：%d %s", w.Code, w.Body.String())
	}
	if w = mailCall(a, "POST", "/inbox", `{"for":"  "}`, origin, a.token); w.Code != 400 {
		t.Fatalf("空目标服务：%d", w.Code)
	}
}

func TestMailPollThrottle(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.interval = time.Hour
	first.msgs = []mailMessage{{ID: "m1", Subject: "code 123456", ReceivedAt: "2026-11-08T00:00:00Z"}}
	first.detail = mailMessage{ID: "m1", Subject: "code 123456", ReceivedAt: "2026-11-08T00:00:00Z"}

	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token)
	mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token)
	if first.polls != 1 {
		t.Fatalf("轮询节流失效：上游被调用 %d 次", first.polls)
	}
}

func TestMailPersistenceAndCorruption(t *testing.T) {
	a, _, _, dir := newTestApp(t)
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)

	reloaded := newTestMailService(dir)
	if _, ok := reloaded.store.get(id); !ok {
		t.Fatal("重启后收件箱记录丢失")
	}
	b, e := os.ReadFile(filepath.Join(dir, "mail.json"))
	if e != nil || !strings.Contains(string(b), `"auth_data"`) || !strings.Contains(string(b), `"schema_version": 1`) {
		t.Fatalf("记录文件内容异常：%s", b)
	}
	if strings.Contains(string(b), `"private-token"`) == false {
		t.Fatal("渠道凭据未落盘，重启后无法再取件")
	}

	brokenDir := t.TempDir()
	broken := filepath.Join(brokenDir, "mail.json")
	if e = os.WriteFile(broken, []byte("{not json"), 0600); e != nil {
		t.Fatal(e)
	}
	bad := &App{token: "csrf-test", addr: host, mail: newTestMailService(brokenDir)}
	if w := mailCall(bad, "GET", "/inboxes", "", origin, bad.token); w.Code != 503 {
		t.Fatalf("损坏文件未报错：%d %s", w.Code, w.Body.String())
	}
	if w := mailCall(bad, "POST", "/inbox", `{"for":"example.com"}`, origin, bad.token); w.Code != 503 {
		t.Fatalf("损坏文件下仍尝试创建：%d", w.Code)
	}
	if after, e := os.ReadFile(broken); e != nil || string(after) != "{not json" {
		t.Fatal("损坏文件被覆盖")
	}
}

func TestMailSecurityBoundaries(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	for _, attack := range []struct {
		method, path, body, requestOrigin, token string
		status                                   int
	}{
		{"GET", "/providers", "", origin, "wrong", 403},
		{"POST", "/inbox", `{"for":"example.com"}`, "https://evil.test", a.token, 403},
		{"DELETE", "/inbox/abc", "", "", a.token, 403},
		{"GET", "/keys", "", origin, a.token, 404},
		{"GET", "/inbox/abc/keys", "", origin, a.token, 404},
		{"GET", "/inbox/abc/messages/..", "", origin, a.token, 404},
		{"GET", "/inbox/../messages", "", origin, a.token, 404},
		{"POST", "/providers", `{}`, origin, a.token, 405},
		{"POST", "/inbox", `{"for":"` + strings.Repeat("x", 17000) + `"}`, origin, a.token, 413},
		{"POST", "/inbox", `[]`, origin, a.token, 400},
		{"POST", "/inbox", `not json`, origin, a.token, 400},
		{"GET", "/inbox/missing/messages", "", origin, a.token, 404},
	} {
		if w := mailCall(a, attack.method, attack.path, attack.body, attack.requestOrigin, attack.token); w.Code != attack.status {
			t.Fatalf("%s %s: got %d want %d (%s)", attack.method, attack.path, w.Code, attack.status, w.Body.String())
		}
	}
	if first.created != 0 || first.polls != 0 {
		t.Fatal("被拦截的请求到达了渠道")
	}
}

func TestMailRouteTable(t *testing.T) {
	for path, want := range map[string]string{
		"/status": http.MethodGet, "/providers": http.MethodGet, "/inboxes": http.MethodGet, "/dispatch/preview": http.MethodGet,
		"/inbox": http.MethodPost, "/inbox/abc": http.MethodDelete, "/inbox/abc/messages": http.MethodGet,
		"/inbox/abc/code": http.MethodGet, "/inbox/abc/messages/m1": http.MethodGet,
		"/inbox/abc/messages/a@b.c": http.MethodGet,
		"/inbox/abc/report":         "", "/inbox/abc/messages/m1/x": "", "/inboxes/x": "", "/providers/x": "", "/": "", "/inbox/abc/": "",
	} {
		if got := mailRouteMethod(path); got != want {
			t.Fatalf("%s: got %q want %q", path, got, want)
		}
	}
}

func TestMailAttachExistingAddress(t *testing.T) {
	a, _, second, _ := newTestApp(t)
	second.attach = []string{"Maildrop.CC"}
	second.msgs = []mailMessage{{ID: "m1", Subject: "code 123456", ReceivedAt: "2026-11-08T00:00:00Z"}}

	w := mailCall(a, "GET", "/status", "", origin, a.token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"attachDomains":["maildrop.cc"]`) {
		t.Fatalf("status 未公布可接入域名：%s", w.Body.String())
	}

	w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"MyBox@maildrop.cc"}`, origin, a.token)
	if w.Code != 201 {
		t.Fatalf("接入失败：%d %s", w.Code, w.Body.String())
	}
	created := mailDecode(t, w)
	id, _ := created["id"].(string)
	if created["address"] != "mybox@maildrop.cc" || created["provider"] != "mailgw" {
		t.Fatalf("接入结果：%v", created)
	}
	if second.domain != "maildrop.cc" {
		t.Fatalf("渠道域名未规范化：%q", second.domain)
	}

	// 同一地址重复接入应复用记录，不重复轮询
	w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"mybox@maildrop.cc"}`, origin, a.token)
	if w.Code != 201 || mailDecode(t, w)["id"] != id || second.created != 1 {
		t.Fatalf("重复接入未复用：%d %s created=%d", w.Code, w.Body.String(), second.created)
	}

	if w = mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token); w.Code != 200 || !strings.Contains(w.Body.String(), `"m1"`) {
		t.Fatalf("接入后无法收件：%d %s", w.Code, w.Body.String())
	}

	w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"a@example.org"}`, origin, a.token)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "maildrop.cc") {
		t.Fatalf("未知域名未拒绝或未给出可用列表：%d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{"nope", "@maildrop.cc", "a b@maildrop.cc", "a@-bad.com"} {
		if w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"`+bad+`"}`, origin, a.token); w.Code != 400 {
			t.Fatalf("非法地址 %q 未拒绝：%d %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestMailAttachCustomDomain(t *testing.T) {
	a, _, second, _ := newTestApp(t)
	second.attach = []string{"maildrop.cc"}
	imap := &fakeImap{fakeChannel: &fakeChannel{name: "imap", interval: 10 * time.Second}}
	a.mail.imap = imap
	imap.msgs = []mailMessage{{ID: "42", Subject: "验证码 123456", ReceivedAt: "2026-11-08T00:00:00Z"}}

	// 没有授权码时明确要求填写，并说明哪些域名可以免密码
	w := mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"me@gmail.com"}`, origin, a.token)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "IMAP 授权码") || !strings.Contains(w.Body.String(), "maildrop.cc") {
		t.Fatalf("缺少授权码未拒绝：%d %s", w.Code, w.Body.String())
	}

	w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"Me@gmail.com","password":"app-pass","imapHost":""}`, origin, a.token)
	if w.Code != 201 {
		t.Fatalf("自定义域名接入失败：%d %s", w.Code, w.Body.String())
	}
	created := mailDecode(t, w)
	id, _ := created["id"].(string)
	if created["provider"] != "imap" || created["address"] != "Me@gmail.com" || created["expiresAt"] != nil {
		t.Fatalf("接入结果：%v", created)
	}
	if imap.loginAs != "Me@gmail.com" {
		t.Fatalf("登录账号被改写：%q", imap.loginAs)
	}
	if strings.Contains(w.Body.String(), "app-pass") {
		t.Fatal("响应泄露授权码")
	}

	// + 子地址登录基础账号，记录也按基础地址存
	w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"you+shop@gmail.com","password":"app-pass"}`, origin, a.token)
	if w.Code != 201 || mailDecode(t, w)["address"] != "you@gmail.com" {
		t.Fatalf("+ 子地址未归到基础账号：%d %s", w.Code, w.Body.String())
	}

	// 同一地址重复接入复用
	if w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"me@gmail.com","password":"app-pass"}`, origin, a.token); mailDecode(t, w)["id"] != id {
		t.Fatal("重复接入未复用记录")
	}
	if imap.connected != 2 {
		t.Fatalf("重复接入重复登录：%d", imap.connected)
	}

	// 登录失败按 400 报中文错误，且不暴露服务器原文
	imap.fail = true
	if w = mailCall(a, "POST", "/inbox", `{"for":"example.com","address":"bad@gmail.com","password":"wrong"}`, origin, a.token); w.Code != 400 || !strings.Contains(w.Body.String(), "登录失败") {
		t.Fatalf("登录失败未按 400 处理：%d %s", w.Code, w.Body.String())
	}
	imap.fail = false

	// 已接入的自定义邮箱走 imap 渠道取件，且不出现在创建下拉里
	if w = mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token); w.Code != 200 || !strings.Contains(w.Body.String(), `"42"`) {
		t.Fatalf("自定义邮箱取件失败：%d %s", w.Code, w.Body.String())
	}
	listed := false
	for _, entry := range mailDecode(t, mailCall(a, "GET", "/providers", "", origin, a.token))["providers"].([]any) {
		provider := entry.(map[string]any)
		if provider["name"] != "imap" {
			continue
		}
		listed = true
		if provider["enabled"] != false {
			t.Fatalf("imap 渠道不应出现在创建下拉：%v", provider)
		}
	}
	if !listed {
		t.Fatal("providers 未列出 imap")
	}
	if w = mailCall(a, "GET", "/dispatch/preview?for=example.com", "", origin, a.token); strings.Contains(w.Body.String(), `"provider":"imap"`) {
		t.Fatal("自定义邮箱进入了自动调度候选")
	}

	if w = mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 {
		t.Fatalf("关闭自定义邮箱失败：%d", w.Code)
	}
	if imap.closed != 1 {
		t.Fatal("关闭未调用渠道清理")
	}
}

func TestMailListStates(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.expiresAt = time.Now().Add(-time.Hour)
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)

	state := func(query string) string {
		w := mailCall(a, "GET", "/inboxes"+query, "", origin, a.token)
		if w.Code != 200 {
			t.Fatalf("inboxes%s: %d", query, w.Code)
		}
		list := mailDecode(t, w)
		rows, _ := list["inboxes"].([]any)
		for _, row := range rows {
			if row.(map[string]any)["id"] == id {
				return row.(map[string]any)["state"].(string)
			}
		}
		return ""
	}
	if got := state("?pageSize=100"); got != "expired" {
		t.Fatalf("过期状态：%q", got)
	}
	if got := state("?status=active"); got != "" {
		t.Fatal("过期收件箱出现在有效筛选里")
	}
	mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token)
	if got := state("?pageSize=100"); got != "closed" {
		t.Fatalf("关闭状态：%q", got)
	}
}

func TestMailViewNeverLeaksAuthData(t *testing.T) {
	a, _, _, _ := newTestApp(t)
	w := mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token)
	if strings.Contains(w.Body.String(), "private-token") {
		t.Fatal("创建响应泄露凭据")
	}
	if w = mailCall(a, "GET", "/inboxes", "", origin, a.token); strings.Contains(w.Body.String(), "private-token") || strings.Contains(w.Body.String(), "auth_data") {
		t.Fatal("列表泄露凭据")
	}
}

func TestMailCombinedPollingAndDetailCache(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.msgs = []mailMessage{
		{ID: "old", Subject: "older", ReceivedAt: "2026-11-08T00:00:00Z"},
		{ID: "new", Subject: "验证码", ReceivedAt: "2026-11-08T00:01:00Z"},
	}
	first.detail = mailMessage{ID: "new", Subject: "验证码 482910"}
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	path := "/inbox/" + id + "/messages?includeCode=true"
	w := mailCall(a, "GET", path, "", origin, a.token)
	data := mailDecode(t, w)
	if w.Code != 200 || data["verification"].(map[string]any)["messageId"] != "new" || data["pollAfter"] != float64(1) {
		t.Fatalf("combined response: %d %s", w.Code, w.Body.String())
	}
	if data["messages"].([]any)[0].(map[string]any)["id"] != "new" || first.msgs[0].ID != "old" {
		t.Fatal("列表应排序，不能修改渠道缓存")
	}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() { defer group.Done(); mailCall(a, "GET", path, "", origin, a.token) }()
	}
	group.Wait()
	mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token)
	if first.polls != 1 || first.details != 1 {
		t.Fatalf("重复取件: lists=%d details=%d", first.polls, first.details)
	}
}

func TestMailConcurrentFirstPoll(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() { defer group.Done(); mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token) }()
	}
	group.Wait()
	if first.polls != 1 {
		t.Fatalf("首次并发请求取件 %d 次", first.polls)
	}
}

func TestMailExpiredInboxDoesNotPoll(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.expiresAt = time.Now().Add(-time.Minute)
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	for _, suffix := range []string{"/messages", "/messages?includeCode=true", "/messages/m1", "/code"} {
		w := mailCall(a, "GET", "/inbox/"+id+suffix, "", origin, a.token)
		if w.Code != 410 {
			t.Fatalf("%s: %d %s", suffix, w.Code, w.Body.String())
		}
	}
	if first.polls != 0 || first.details != 0 {
		t.Fatal("过期后仍然请求上游")
	}
	if w := mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 || first.closed != 1 {
		t.Fatal("过期后仍应能关闭并释放资源")
	}
}

func TestMailVerificationDetailFailureRetries(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.msgs = []mailMessage{{ID: "m1", Excerpt: "Your code: 482910"}}
	first.detailErr = true
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	path := "/inbox/" + id + "/messages?includeCode=true"
	data := mailDecode(t, mailCall(a, "GET", path, "", origin, a.token))["verification"].(map[string]any)
	if data["error"] == nil || data["codes"].([]any)[0].(map[string]any)["value"] != "482910" {
		t.Fatalf("应保留摘要提取结果并提示正文失败: %v", data)
	}
	first.detailErr = false
	first.detail = mailMessage{ID: "m1", Subject: "OTP 735911"}
	data = mailDecode(t, mailCall(a, "GET", path, "", origin, a.token))["verification"].(map[string]any)
	if data["error"] != nil || data["codes"].([]any)[0].(map[string]any)["value"] != "735911" || first.details != 2 {
		t.Fatalf("正文失败不能缓存成无验证码: %v", data)
	}
}

func TestMailAutomaticDispatchCoolsFailedProviders(t *testing.T) {
	a, first, second, _ := newTestApp(t)
	first.fail = true
	for range 2 {
		if w := mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token); w.Code != 201 {
			t.Fatalf("自动切换失败: %d %s", w.Code, w.Body.String())
		}
	}
	if first.attempts != 1 || second.attempts != 2 {
		t.Fatal("下一次自动调度应跳过刚失败的渠道")
	}
	preview := mailDecode(t, mailCall(a, "GET", "/dispatch/preview", "", origin, a.token))["candidates"].([]any)
	if preview[0].(map[string]any)["eligible"] != false || !strings.Contains(preview[0].(map[string]any)["reason"].(string), "冷却") {
		t.Fatalf("预览未体现冷却: %v", preview)
	}
	first.fail = false
	w := mailCall(a, "POST", "/inbox", `{"for":"example.com","provider":"mailtm"}`, origin, a.token)
	if w.Code != 201 || first.attempts != 2 || a.mail.cooling("mailtm") {
		t.Fatal("显式选择应允许立即重试，成功后解除冷却")
	}
	a.mail.cooldowns["mailtm"] = time.Now().Add(-time.Second)
	if a.mail.cooling("mailtm") {
		t.Fatal("冷却到期后应自动恢复")
	}
}

func TestMailHTMLOnlyMessageReturnsSafeText(t *testing.T) {
	a, first, _, _ := newTestApp(t)
	first.detail = mailMessage{ID: "m1", HTML: `<script>alert(1)</script><style>p{color:red}</style><p>Your code: &#52;&#56;&#50;&#57;&#49;&#48;</p><img src="https://example.com/tracker">`}
	id := mailDecode(t, mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token))["id"].(string)
	w := mailCall(a, "GET", "/inbox/"+id+"/messages/m1", "", origin, a.token)
	if text := mailDecode(t, w)["text"]; w.Code != 200 || text != "Your code: 482910" {
		t.Fatalf("HTML 正文应在服务端转成纯文本: %v", text)
	}
}
