package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// mail 是原生的临时邮箱功能：内置免费渠道、收件箱记录与本地 /mail-api。
// 记录含渠道凭据，文件权限 0600，不进版本库。

// mailChannel 是本层对渠道引擎的唯一依赖，测试可用假渠道替换。
type mailChannel interface {
	Meta() mailProviderMeta
	AttachDomains() []string
	Create(domain, username string) (mailInboxData, error)
	Messages(in mailInboxData) ([]mailMessage, error)
	Message(in mailInboxData, id string) (mailMessage, error)
	Close(in mailInboxData) error
	PollInterval() time.Duration
}

type mailInbox struct {
	ID        string            `json:"id"`
	Provider  string            `json:"provider"`
	Address   string            `json:"address"`
	AuthData  map[string]string `json:"auth_data"`
	APIBase   string            `json:"api_base,omitempty"`
	Service   string            `json:"target_service,omitempty"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	ClosedAt  *time.Time        `json:"closed_at,omitempty"`
}

type mailInboxView struct {
	ID            string     `json:"id"`
	Provider      string     `json:"provider"`
	Address       string     `json:"address"`
	TargetService string     `json:"target_service,omitempty"`
	Status        string     `json:"status"`
	State         string     `json:"state"` // active / expired / closed，供分类列出
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"` // 自定义邮箱不过期，省略
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
}

type mailFile struct {
	SchemaVersion int         `json:"schema_version"`
	Inboxes       []mailInbox `json:"inboxes"`
}

const mailKeepClosed = 24 * time.Hour
const mailMaxRecords = 200

// mailStore 是 data/mail.json 的内存副本。读取失败时保留错误并拒绝写入，
// 避免把损坏文件静默覆盖成空文件。
type mailStore struct {
	mu    sync.Mutex
	path  string
	items []mailInbox
	err   error
}

func newMailStore(path string) *mailStore {
	s := &mailStore{path: path}
	b, e := os.ReadFile(path)
	switch {
	case os.IsNotExist(e):
	case e != nil:
		s.err = errors.New("邮箱记录文件无法读取，请检查 " + path)
	default:
		var f mailFile
		if e = strictJSON(b, &f); e != nil || f.SchemaVersion != 1 {
			s.err = errors.New("邮箱记录文件损坏，请修复或移走 " + path + " 后重启")
		} else {
			s.items = f.Inboxes
		}
	}
	return s
}

// save 假定调用方已持锁。
func (s *mailStore) save() error {
	if s.err != nil {
		return s.err
	}
	s.prune()
	b, e := json.MarshalIndent(mailFile{SchemaVersion: 1, Inboxes: s.items}, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(b, '\n'))
}

func (s *mailStore) prune() {
	now := time.Now()
	kept := s.items[:0]
	for _, it := range s.items {
		if it.Status == "closed" && it.ClosedAt != nil && now.Sub(*it.ClosedAt) > mailKeepClosed {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) > mailMaxRecords {
		kept = kept[len(kept)-mailMaxRecords:]
	}
	s.items = kept
}

func (s *mailStore) get(id string) (mailInbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.ID == id {
			return it, true
		}
	}
	return mailInbox{}, false
}

func (s *mailStore) add(it mailInbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, it)
	if e := s.save(); e != nil {
		s.items = s.items[:len(s.items)-1]
		return e
	}
	return nil
}

func (s *mailStore) close(id string) (mailInbox, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		it := s.items[i]
		if it.Status == "closed" {
			return it, false, nil
		}
		now := time.Now().UTC()
		s.items[i].Status, s.items[i].ClosedAt = "closed", &now
		// 关闭后不再轮询，授权码没有保留理由。
		s.items[i].AuthData = nil
		if e := s.save(); e != nil {
			s.items[i].Status, s.items[i].ClosedAt, s.items[i].AuthData = it.Status, it.ClosedAt, it.AuthData
			return it, false, e
		}
		it.Status, it.ClosedAt = "closed", &now
		return it, true, nil
	}
	return mailInbox{}, false, nil
}

// mailState 是「有效 / 已过期 / 已关闭」的唯一判定，列表与管理端显示共用。
func mailState(it mailInbox, now time.Time) string {
	if it.Status == "closed" {
		return "closed"
	}
	if !it.ExpiresAt.IsZero() && !now.Before(it.ExpiresAt) {
		return "expired"
	}
	return "active"
}

func (s *mailStore) list(status string, limit int) ([]mailInboxView, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []mailInboxView
	for i := len(s.items) - 1; i >= 0; i-- {
		it := s.items[i]
		state := mailState(it, now)
		if status != "" && status != "all" && status != state {
			continue
		}
		var expires *time.Time
		if !it.ExpiresAt.IsZero() {
			e := it.ExpiresAt
			expires = &e
		}
		out = append(out, mailInboxView{it.ID, it.Provider, it.Address, it.Service, it.Status, state, it.CreatedAt, expires, it.ClosedAt})
	}
	total := len(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total
}

// activeByAddress 供「直接接入」复用同一地址，避免同一信箱被重复登记和轮询。
func (s *mailStore) activeByAddress(address string) (mailInbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := len(s.items) - 1; i >= 0; i-- {
		it := s.items[i]
		if it.Status == "active" && mailState(it, now) == "active" && strings.EqualFold(it.Address, address) {
			return it, true
		}
	}
	return mailInbox{}, false
}

type mailPoll struct {
	mu           sync.Mutex
	at           time.Time
	msgs         []mailMessage
	verifiedID   string
	verification map[string]any
}

// mailCustomChannel 是按地址接入的自定义邮箱（IMAP），需要一次性校验凭据。
type mailCustomChannel interface {
	mailChannel
	Connect(address, password, host string) (mailInboxData, error)
}

// mailService 串起渠道、记录和轮询节流。轮询节流避免 /messages 与 /code
// 同时调用时打爆渠道的每分钟配额。
type mailService struct {
	store     *mailStore
	channels  []mailChannel     // 内置渠道：参与创建与自动调度
	imap      mailCustomChannel // 自定义邮箱：只能按地址 + 授权码接入，不参与调度
	mu        sync.Mutex
	polls     map[string]*mailPoll
	cooldowns map[string]time.Time
}

func newMailService(dir string) *mailService {
	s := &mailService{store: newMailStore(filepath.Join(dir, "mail.json")), polls: map[string]*mailPoll{}, imap: newMailImapChannel()}
	for _, p := range mailProviders {
		s.channels = append(s.channels, p)
	}
	return s
}

func (s *mailService) channel(name string) mailChannel {
	if s.imap != nil && name == s.imap.Meta().Name {
		return s.imap
	}
	for _, c := range s.channels {
		if c.Meta().Name == name {
			return c
		}
	}
	return nil
}

// attachDomains 汇总所有「地址即信箱」渠道的域名，供页面提示可用地址。
func (s *mailService) attachDomains() []string {
	out := []string{}
	seen := map[string]bool{}
	for _, c := range s.channels {
		for _, domain := range c.AttachDomains() {
			domain = strings.ToLower(domain)
			if !seen[domain] {
				seen[domain] = true
				out = append(out, domain)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (s *mailService) inboxData(it mailInbox) mailInboxData {
	return mailInboxData{Provider: it.Provider, Address: it.Address, AuthData: it.AuthData, APIBase: it.APIBase, ExpiresAt: it.ExpiresAt}
}

// messages 在渠道轮询间隔内复用上一次结果。
func (s *mailService) messages(it mailInbox, c mailChannel) ([]mailMessage, error) {
	s.mu.Lock()
	last := s.polls[it.ID]
	if last == nil {
		// shortcut: 缓存只保留最近 200 个邮箱，超过单机规模时改为有界 LRU。
		if len(s.polls) >= mailMaxRecords {
			for id, cached := range s.polls {
				if cached.mu.TryLock() {
					delete(s.polls, id)
					cached.mu.Unlock()
					break
				}
			}
		}
		last = &mailPoll{}
		s.polls[it.ID] = last
	}
	s.mu.Unlock()
	last.mu.Lock()
	defer last.mu.Unlock()
	if time.Since(last.at) < c.PollInterval() {
		return append([]mailMessage(nil), last.msgs...), nil
	}
	msgs, err := c.Messages(s.inboxData(it))
	if err != nil {
		return nil, err
	}
	last.at, last.msgs = time.Now(), msgs
	return append([]mailMessage(nil), msgs...), nil
}

func (s *mailService) forget(id string) {
	s.mu.Lock()
	delete(s.polls, id)
	s.mu.Unlock()
}

func (s *mailService) cooling(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.cooldowns[name])
}

var mailIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var mailMessagePattern = regexp.MustCompile(`^[A-Za-z0-9_@.=-]{1,256}$`)

// mailRouteMethod 只放行下面这张表，未列出的路径一律 404，避免变成通用代理。
func mailRouteMethod(path string) string {
	switch path {
	case "/status", "/providers", "/inboxes", "/dispatch/preview":
		return http.MethodGet
	case "/inbox":
		return http.MethodPost
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "inbox" || !mailIDPattern.MatchString(parts[1]) {
		return ""
	}
	switch {
	case len(parts) == 2:
		return http.MethodDelete
	case len(parts) == 3 && (parts[2] == "messages" || parts[2] == "code"):
		return http.MethodGet
	case len(parts) == 4 && parts[2] == "messages" && parts[3] != "." && parts[3] != ".." && mailMessagePattern.MatchString(parts[3]):
		return http.MethodGet
	}
	return ""
}

func mailJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (a *App) mailAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int, message string) { mailJSON(w, status, map[string]string{"error": message}) }
	path := strings.TrimPrefix(r.URL.Path, "/mail-api")
	method := mailRouteMethod(path)
	if method == "" {
		fail(404, "邮箱接口不存在")
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		fail(405, "请求方法不允许")
		return
	}
	// 页面首次加载还没有 CSRF 令牌，/status 在校验之前返回它；其余接口必须带令牌。
	if path == "/status" {
		domains := []string{}
		if a.mail != nil {
			domains = a.mail.attachDomains()
		}
		mailJSON(w, 200, map[string]any{"csrf": a.token, "attachDomains": domains})
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(a.token)) != 1 || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) || (method != http.MethodGet && r.Header.Get("Origin") != origin) {
		fail(403, "邮箱请求校验失败，请刷新页面")
		return
	}
	if a.mail == nil {
		fail(503, "邮箱功能未初始化")
		return
	}
	if path != "/providers" && path != "/dispatch/preview" {
		if e := a.mail.store.err; e != nil {
			fail(503, e.Error())
			return
		}
	}
	switch {
	case path == "/providers":
		a.mail.providers(w)
	case path == "/inboxes":
		a.mail.inboxes(w, r)
	case path == "/dispatch/preview":
		a.mail.preview(w, r)
	case path == "/inbox":
		a.mail.create(w, r)
	default:
		a.mail.inbox(w, r, path)
	}
}

func (s *mailService) providers(w http.ResponseWriter) {
	type view struct {
		mailProviderMeta
		Enabled bool `json:"enabled"`
	}
	out := make([]view, 0, len(s.channels)+1)
	for _, c := range s.channels {
		out = append(out, view{c.Meta(), true})
	}
	if s.imap != nil {
		// 自定义邮箱只能在「已有邮箱地址」里接入，不出现在创建下拉（enabled=false）。
		out = append(out, view{s.imap.Meta(), false})
	}
	mailJSON(w, 200, map[string]any{"providers": out})
}

func (s *mailService) preview(w http.ResponseWriter, r *http.Request) {
	type candidate struct {
		Provider    string `json:"provider"`
		DisplayName string `json:"displayName"`
		Eligible    bool   `json:"eligible"`
		Reason      string `json:"reason"`
	}
	service := strings.TrimSpace(r.URL.Query().Get("for"))
	out := make([]candidate, 0, len(s.channels))
	for _, c := range s.channels {
		meta := c.Meta()
		cand := candidate{Provider: meta.Name, DisplayName: meta.DisplayName, Eligible: meta.Features.PollInbox}
		if !cand.Eligible {
			cand.Reason = "不支持轮询收件"
		} else if s.cooling(meta.Name) {
			cand.Eligible, cand.Reason = false, "最近创建失败，自动调度冷却 60 秒；可指定渠道重试"
		}
		out = append(out, cand)
	}
	mailJSON(w, 200, map[string]any{"service": service, "candidates": out})
}

func (s *mailService) inboxes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, pageSize := 1, 50
	fmt.Sscanf(query.Get("page"), "%d", &page)
	fmt.Sscanf(query.Get("pageSize"), "%d", &pageSize)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	items, total := s.store.list(query.Get("status"), pageSize*page)
	if len(items) > pageSize*(page-1) {
		items = items[pageSize*(page-1):]
	} else {
		items = nil
	}
	mailJSON(w, 200, map[string]any{"inboxes": items, "page": page, "pageSize": pageSize, "total": total})
}

type mailCreateRequest struct {
	For      string `json:"for"`
	Provider string `json:"provider"`
	Domain   string `json:"domain"`
	Address  string `json:"address"`
	Password string `json:"password"` // 自定义域名邮箱的 IMAP 应用专用密码
	ImapHost string `json:"imapHost"` // 留空则按域名自动识别
}

// mailInputError 是可以直接展示给用户的输入错误（与内部故障区分 400/500）。
type mailInputError string

func (e mailInputError) Error() string { return string(e) }

func mailBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if e != nil {
		mailJSON(w, 413, map[string]string{"error": "邮箱请求过大或读取失败"})
		return false
	}
	trimmed := strings.TrimSpace(string(body))
	if !json.Valid(body) || !strings.HasPrefix(trimmed, "{") {
		mailJSON(w, 400, map[string]string{"error": "邮箱请求须为 JSON 对象"})
		return false
	}
	if e = json.Unmarshal(body, dst); e != nil {
		mailJSON(w, 400, map[string]string{"error": "邮箱请求格式无效"})
		return false
	}
	return true
}

func (s *mailService) create(w http.ResponseWriter, r *http.Request) {
	var req mailCreateRequest
	if !mailBody(w, r, &req) {
		return
	}
	req.For = strings.TrimSpace(req.For)
	if req.For == "" || len(req.For) > 253 || strings.ContainsAny(req.For, " \t\r\n") {
		mailJSON(w, 400, map[string]string{"error": "请填写目标服务域名"})
		return
	}
	if address := strings.TrimSpace(req.Address); address != "" {
		s.attach(w, address, req.For, req.Password, strings.TrimSpace(req.ImapHost))
		return
	}
	candidates := s.channels
	if req.Provider != "" {
		c := s.channel(req.Provider)
		if c == nil {
			mailJSON(w, 400, map[string]string{"error": "未知渠道"})
			return
		}
		candidates = []mailChannel{c}
	}
	var failures []string
	for _, c := range candidates {
		auto := req.Provider == "" && req.Domain == ""
		if auto && s.cooling(c.Meta().Name) {
			failures = append(failures, c.Meta().DisplayName+"：暂时冷却，可指定渠道重试")
			continue
		}
		data, err := c.Create(req.Domain, "")
		if err != nil {
			if auto {
				s.mu.Lock()
				if s.cooldowns == nil {
					s.cooldowns = map[string]time.Time{}
				}
				s.cooldowns[c.Meta().Name] = time.Now().Add(time.Minute)
				s.mu.Unlock()
			}
			failures = append(failures, c.Meta().DisplayName+"："+err.Error())
			continue
		}
		s.mu.Lock()
		delete(s.cooldowns, c.Meta().Name)
		s.mu.Unlock()
		if data.ExpiresAt.IsZero() {
			data.ExpiresAt = time.Now().Add(24 * time.Hour)
		}
		rec := mailInbox{
			ID:        newMailID(),
			Provider:  c.Meta().Name,
			Address:   data.Address,
			AuthData:  data.AuthData,
			APIBase:   data.APIBase,
			Service:   req.For,
			Status:    "active",
			CreatedAt: time.Now().UTC(),
			ExpiresAt: data.ExpiresAt.UTC(),
		}
		if err = s.store.add(rec); err != nil {
			c.Close(data)
			mailJSON(w, 500, map[string]string{"error": "邮箱记录保存失败，请检查 data/mail.json"})
			return
		}
		log.Printf("邮箱创建成功：%s（%s）", rec.Address, rec.Provider)
		mailJSON(w, 201, map[string]any{"id": rec.ID, "address": rec.Address, "provider": rec.Provider, "expiresAt": rec.ExpiresAt, "features": c.Meta().Features})
		return
	}
	if len(failures) == 0 {
		mailJSON(w, 503, map[string]string{"error": "没有可用渠道"})
		return
	}
	mailJSON(w, 503, map[string]string{"error": "所有渠道均失败：" + strings.Join(failures, "；")})
}

// attach 接入用户已有的邮箱：「地址即信箱」的域名免密码，其余域名走 IMAP 授权码。
func (s *mailService) attach(w http.ResponseWriter, address, service, password, imapHost string) {
	rec, err := s.attachInbox(address, service, password, imapHost)
	if err != nil {
		var input mailInputError
		if errors.As(err, &input) {
			mailJSON(w, 400, map[string]string{"error": input.Error()})
		} else {
			mailJSON(w, 500, map[string]string{"error": "邮箱记录保存失败，请检查 data/mail.json"})
		}
		return
	}
	log.Printf("邮箱接入成功：%s（%s）", rec.Address, rec.Provider)
	features := mailFeatures{}
	if c := s.channel(rec.Provider); c != nil {
		features = c.Meta().Features
	}
	resp := map[string]any{"id": rec.ID, "address": rec.Address, "provider": rec.Provider, "features": features}
	if !rec.ExpiresAt.IsZero() {
		resp["expiresAt"] = rec.ExpiresAt
	}
	mailJSON(w, 201, resp)
}

var mailAddressLocalRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

func mailSplitAddress(address string) (local, domain string, err error) {
	at := strings.LastIndexByte(address, '@')
	if at <= 0 || at == len(address)-1 || len(address) > 254 {
		return "", "", mailInputError("请填写完整邮箱地址，例如 name@maildrop.cc 或 me@gmail.com")
	}
	local = address[:at]
	domain = strings.ToLower(strings.TrimSuffix(address[at+1:], "."))
	if !mailAddressLocalRE.MatchString(local) || !mailDomainRE.MatchString(domain) {
		return "", "", mailInputError("邮箱地址格式无效")
	}
	return local, domain, nil
}

// stripMailPlusTag 去掉 + 子地址：IMAP 只能登录基础账号，基础收件箱本来就收子地址的邮件。
func stripMailPlusTag(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return address
	}
	if plus := strings.IndexByte(address[:at], '+'); plus > 0 {
		return address[:plus] + address[at:]
	}
	return address
}

func (s *mailService) attachChannel(domain string) (mailChannel, bool) {
	for _, c := range s.channels {
		for _, d := range c.AttachDomains() {
			if strings.EqualFold(d, domain) {
				return c, true
			}
		}
	}
	return nil, false
}

func (s *mailService) attachInbox(address, service, password, imapHost string) (mailInbox, error) {
	local, domain, err := mailSplitAddress(address)
	if err != nil {
		return mailInbox{}, err
	}
	c, attach := s.attachChannel(domain)
	// 规范化成真正读的那个信箱，重复接入才认得出是同一个。
	target := local + "@" + domain
	if attach {
		target = strings.ToLower(local) + "@" + domain
	} else {
		target = stripMailPlusTag(target)
	}
	if existing, ok := s.store.activeByAddress(target); ok {
		return existing, nil
	}
	rec := mailInbox{
		ID:        newMailID(),
		Address:   target,
		Service:   service,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
	}
	if attach {
		data, err := c.Create(domain, strings.ToLower(local))
		if err != nil {
			return mailInbox{}, mailInputError(c.Meta().DisplayName + "：" + err.Error())
		}
		rec.Provider, rec.AuthData, rec.APIBase = c.Meta().Name, data.AuthData, data.APIBase
		rec.ExpiresAt = time.Now().Add(24 * time.Hour).UTC()
	} else {
		if password == "" {
			message := "该域名需要 IMAP 授权码：请填写邮箱的应用专用密码（Gmail / QQ / 163 等），IMAP 服务器留空则自动识别"
			if domains := s.attachDomains(); len(domains) > 0 {
				message += "。无需密码可直接接入的域名：" + strings.Join(domains, "、")
			}
			return mailInbox{}, mailInputError(message)
		}
		if s.imap == nil {
			return mailInbox{}, errors.New("自定义邮箱渠道不可用")
		}
		data, err := s.imap.Connect(target, password, imapHost)
		if err != nil {
			return mailInbox{}, mailInputError(err.Error())
		}
		rec.Provider, rec.AuthData, rec.APIBase = s.imap.Meta().Name, data.AuthData, data.APIBase
		// 自己的邮箱不会到期：不设 ExpiresAt，列表里始终显示「有效」。
	}
	if err = s.store.add(rec); err != nil {
		return mailInbox{}, err
	}
	return rec, nil
}

func (s *mailService) inbox(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	rec, ok := s.store.get(parts[1])
	if !ok {
		mailJSON(w, 404, map[string]string{"error": "收件箱不存在"})
		return
	}
	if len(parts) == 2 {
		s.close(w, rec)
		return
	}
	if rec.Status == "closed" {
		if parts[2] == "messages" {
			mailJSON(w, 200, map[string]any{"status": "closed"})
			return
		}
		mailJSON(w, 409, map[string]string{"error": "收件箱已关闭"})
		return
	}
	if mailState(rec, time.Now()) == "expired" {
		mailJSON(w, 410, map[string]string{"error": "收件箱已过期，请创建新邮箱"})
		return
	}
	c := s.channel(rec.Provider)
	if c == nil {
		mailJSON(w, 503, map[string]string{"error": "渠道已不可用，请关闭后重建邮箱"})
		return
	}
	switch {
	case parts[2] == "messages" && len(parts) == 3:
		msgs, err := s.messages(rec, c)
		if err != nil {
			mailJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		sort.SliceStable(msgs, func(i, j int) bool { return mailReceived(msgs[i]) > mailReceived(msgs[j]) })
		data := map[string]any{"messages": msgs, "status": rec.Status, "address": rec.Address, "provider": rec.Provider, "pollAfter": c.PollInterval().Seconds()}
		if r.URL.Query().Get("includeCode") == "true" {
			data["verification"] = s.verification(rec, c, msgs)
		}
		mailJSON(w, 200, data)
	case parts[2] == "messages":
		detail, err := c.Message(s.inboxData(rec), parts[3])
		if err != nil {
			mailJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		if detail.Text == "" && detail.HTML != "" {
			detail.Text = mailStripHTML(detail.HTML)
		}
		mailJSON(w, 200, detail)
	default:
		s.code(w, rec, c)
	}
}

func (s *mailService) code(w http.ResponseWriter, rec mailInbox, c mailChannel) {
	msgs, err := s.messages(rec, c)
	if err != nil {
		mailJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	sort.SliceStable(msgs, func(i, j int) bool { return mailReceived(msgs[i]) > mailReceived(msgs[j]) })
	mailJSON(w, 200, s.verification(rec, c, msgs))
}

func (s *mailService) verification(rec mailInbox, c mailChannel, msgs []mailMessage) map[string]any {
	empty := map[string]any{"codes": []mailCode{}, "email": nil, "messageId": nil, "receivedAt": nil}
	if len(msgs) == 0 {
		return empty
	}
	latest := msgs[0]
	s.mu.Lock()
	cached := s.polls[rec.ID]
	s.mu.Unlock()
	if cached != nil {
		cached.mu.Lock()
		defer cached.mu.Unlock()
		if cached.verifiedID == latest.ID && cached.verification != nil {
			return cached.verification
		}
	}
	detail, err := c.Message(s.inboxData(rec), latest.ID)
	if err != nil {
		detail = latest
		empty["error"] = "验证码正文读取失败，可打开邮件重试"
	}
	if detail.ID == "" {
		detail.ID = latest.ID
	}
	codes := extractMailCodes(detail.Subject, detail.Text, detail.HTML)
	if len(codes) == 0 && detail.Excerpt != "" {
		codes = extractMailCodes(detail.Subject, detail.Excerpt, "")
	}
	if codes == nil {
		codes = []mailCode{}
	}
	empty["codes"], empty["email"] = codes, map[string]string{"from": detail.From, "subject": detail.Subject}
	empty["messageId"], empty["receivedAt"] = detail.ID, detail.ReceivedAt
	if cached != nil && err == nil {
		cached.verifiedID, cached.verification = latest.ID, empty
	}
	return empty
}

func mailReceived(m mailMessage) int64 {
	t, e := time.Parse(time.RFC3339, m.ReceivedAt)
	if e != nil {
		return 0
	}
	return t.UnixNano()
}

func (s *mailService) close(w http.ResponseWriter, rec mailInbox) {
	it, changed, err := s.store.close(rec.ID)
	if err != nil {
		mailJSON(w, 500, map[string]string{"error": "邮箱记录保存失败，请检查 data/mail.json"})
		return
	}
	s.forget(rec.ID)
	if changed {
		if c := s.channel(it.Provider); c != nil {
			if e := c.Close(s.inboxData(it)); e != nil {
				// 上游清理失败不影响本地关闭；记录里已标记 closed，不再轮询。
				log.Printf("邮箱上游清理失败：%s（%s）", it.Address, it.Provider)
			}
		}
	}
	mailJSON(w, 200, map[string]any{"ok": true})
}

func newMailID() string {
	var b [8]byte
	if _, e := rand.Read(b[:]); e != nil {
		return fmt.Sprintf("m%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
