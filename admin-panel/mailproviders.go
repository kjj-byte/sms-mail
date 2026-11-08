package main

// 邮箱渠道引擎：声明式配置 + 通用 REST/GraphQL 调用，语义对齐 mail-hub 的 template-provider。

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mailMaxResponse = 2 << 20
	// 域名列表变动慢，成功结果缓存 5 分钟；失败只缓存 1 分钟，免得一个挂掉的上游拖慢每次自动调度。
	mailDomainCacheTTL  = 5 * time.Minute
	mailDomainFailTTL   = time.Minute
	mailMessageCacheMax = 200
)

var mailUpstreamClient = &http.Client{
	Timeout: 15 * time.Second,
	// 跟随后源站的 3xx 会把带凭据的请求发往任意主机，一律不跟随。
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type mailFeatures struct {
	CustomUsername bool `json:"customUsername"`
	PollInbox      bool `json:"pollInbox"`
	Attachments    bool `json:"attachments"`
}

type mailProviderMeta struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"displayName"`
	Tier        string       `json:"tier"`
	Retention   string       `json:"retention"`
	Features    mailFeatures `json:"features"`
}

type mailInboxData struct {
	Provider  string            `json:"provider"`
	Address   string            `json:"address"`
	AuthData  map[string]string `json:"authData"`
	APIBase   string            `json:"apiBase,omitempty"`
	ExpiresAt time.Time         `json:"expiresAt,omitempty"`
}

type mailMessage struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	Subject    string `json:"subject"`
	Excerpt    string `json:"excerpt"`
	Text       string `json:"text,omitempty"`
	HTML       string `json:"html,omitempty"`
	ReceivedAt string `json:"receivedAt,omitempty"` // RFC3339 UTC，未知为空
}

type mailRequest struct {
	Path      string
	Method    string
	Body      map[string]any
	BodyType  string // ""/json | form
	AuthFrom  string // "provider" | "inbox"
	AuthField string
}

type mailMapping struct {
	ID         string
	From       string
	Subject    string
	Excerpt    string
	ReceivedAt string
	Text       string
	HTML       string
}

type mailDomainsConfig struct {
	Mode         string // "static" | "endpoint" | "from_create"
	List         []string
	Request      mailRequest
	ResultPath   string
	DomainField  string
	FilterField  string
	FilterEquals any
}

type mailCreateConfig struct {
	Skip        bool
	Request     mailRequest
	AddressPath string
	AuthPaths   map[string]string
	ExpiresIn   int // 秒
}

type mailPostCreateConfig struct {
	Request   mailRequest
	AuthPaths map[string]string
}

type mailMessagesConfig struct {
	Request         mailRequest
	ResultPath      string
	TimestampFormat string
	Mapping         mailMapping
}

type mailMessageDetailConfig struct {
	FromList        bool
	Request         mailRequest
	TimestampFormat string
	Mapping         mailMapping
}

type mailProviderConfig struct {
	Name          string
	DisplayName   string
	Tier          string
	Retention     string
	PollPerMinute int
	Features      mailFeatures
	APIBase       string
	ExtraHeaders  map[string]string

	Domains       mailDomainsConfig
	Create        mailCreateConfig
	PostCreate    *mailPostCreateConfig
	Messages      mailMessagesConfig
	MessageDetail mailMessageDetailConfig
	Delete        *mailRequest
}

type mailProvider struct {
	cfg      mailProviderConfig
	mu       sync.Mutex
	domains  []string
	domainAt time.Time
	domainOK bool
	items    map[string][]any
}

func newMailProvider(cfg mailProviderConfig) *mailProvider { return &mailProvider{cfg: cfg} }

func (p *mailProvider) Meta() mailProviderMeta {
	return mailProviderMeta{
		Name:        p.cfg.Name,
		DisplayName: p.cfg.DisplayName,
		Tier:        p.cfg.Tier,
		Retention:   p.cfg.Retention,
		Features:    p.cfg.Features,
	}
}

func (p *mailProvider) PollInterval() time.Duration {
	if p.cfg.PollPerMinute <= 0 {
		return time.Minute
	}
	return time.Minute / time.Duration(p.cfg.PollPerMinute)
}

// AttachDomains 返回「地址即信箱」的静态域名：这些渠道不注册也能读任意本域地址，
// 因此用户输入自己的地址即可直接接入，无需调用创建接口。
func (p *mailProvider) AttachDomains() []string {
	if !p.cfg.Create.Skip || p.cfg.Domains.Mode != "static" {
		return nil
	}
	return slices.Clone(p.cfg.Domains.List)
}

func (p *mailProvider) Domains() []string {
	cfg := p.cfg.Domains
	switch cfg.Mode {
	case "static":
		return slices.Clone(cfg.List)
	case "from_create":
		if domains, ok, cached := p.cachedDomains(); ok && cached {
			return domains
		}
		in, err := p.Create("", "")
		if err != nil {
			return nil
		}
		at := strings.LastIndexByte(in.Address, '@')
		if at < 0 || at == len(in.Address)-1 {
			return nil
		}
		domains := []string{in.Address[at+1:]}
		p.storeDomains(domains, true)
		return slices.Clone(domains)
	}
	if domains, ok, cached := p.cachedDomains(); cached {
		if !ok {
			return nil
		}
		return domains
	}
	domains, valid := p.fetchDomains()
	p.storeDomains(domains, valid)
	if !valid {
		return nil
	}
	return slices.Clone(domains)
}

// cachedDomains 的第三个返回值区分「没缓存」和「缓存了一个失败」：失败也必须命中缓存，否则上游挂了会每次重试。
func (p *mailProvider) cachedDomains() ([]string, bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Now().After(p.domainAt) {
		return nil, false, false
	}
	return slices.Clone(p.domains), p.domainOK, true
}

func (p *mailProvider) storeDomains(domains []string, ok bool) {
	ttl := mailDomainFailTTL
	if ok {
		ttl = mailDomainCacheTTL
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.domains = domains
	p.domainOK = ok
	p.domainAt = time.Now().Add(ttl)
}

func (p *mailProvider) fetchDomains() ([]string, bool) {
	cfg := p.cfg.Domains
	req := cfg.Request
	if req.Path == "" {
		req.Path = "/domains"
	}
	raw, err := p.call(nil, req, nil)
	if err != nil {
		return nil, false
	}
	data, err := mailDecodeJSON(raw)
	if err != nil {
		return nil, false
	}
	items, ok := mailResolvePath(data, mailRootPath(cfg.ResultPath)).([]any)
	if !ok {
		return nil, false
	}
	domains := make([]string, 0, len(items))
	for _, item := range items {
		if cfg.FilterField != "" && mailScalar(mailResolvePath(item, cfg.FilterField)) != mailScalar(cfg.FilterEquals) {
			continue
		}
		if cfg.DomainField != "" {
			if domain := mailString(mailResolvePath(item, cfg.DomainField)); domain != "" {
				domains = append(domains, domain)
			}
		} else if domain, ok := item.(string); ok && domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains, true
}

var (
	mailUsernameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	mailDomainRE   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
)

func (p *mailProvider) Create(domain, username string) (mailInboxData, error) {
	cfg := p.cfg
	// 用户名和域名会插进上游 URL 路径与查询串；只放行 URL 安全的字符，避免客户端借它改写上游请求。
	if username != "" && !mailUsernameRE.MatchString(username) {
		return mailInboxData{}, errors.New("用户名只能包含字母、数字、点、下划线和短横线")
	}
	if domain != "" && !mailDomainRE.MatchString(domain) {
		return mailInboxData{}, errors.New("域名格式无效")
	}
	if domain == "" && cfg.Domains.Mode != "from_create" {
		domains := p.Domains()
		if len(domains) == 0 {
			return mailInboxData{}, fmt.Errorf("%s: 没有可用域名", cfg.Name)
		}
		domain = domains[mailRandomIndex(len(domains))]
	}
	if username == "" {
		username = mailRandomString(10)
	}
	vars := map[string]string{
		"username": username,
		"domain":   domain,
		"password": mailRandomPassword(),
	}
	if domain != "" {
		vars["address"] = username + "@" + domain
	} else {
		vars["address"] = ""
	}

	var data any
	if !cfg.Create.Skip {
		req := cfg.Create.Request
		if req.Body == nil && mailMethodAllowsBody(req.Method) {
			req.Body = map[string]any{"address": "{{address}}", "password": "{{password}}"}
		}
		raw, err := p.call(nil, req, vars)
		if err != nil {
			return mailInboxData{}, err
		}
		if data, err = mailDecodeJSON(raw); err != nil {
			return mailInboxData{}, fmt.Errorf("%s: 上游返回格式无效", cfg.Name)
		}
	}

	address := ""
	if !cfg.Create.Skip {
		address = mailString(mailResolvePath(data, cfg.Create.AddressPath))
	}
	if address == "" {
		address = vars["address"]
	}
	authData := make(map[string]string, len(vars)+len(cfg.Create.AuthPaths))
	for key, value := range vars {
		authData[key] = value
	}
	for key, path := range cfg.Create.AuthPaths {
		if value := mailResolvePath(data, path); value != nil {
			authData[key] = mailString(value)
		}
	}

	if pc := cfg.PostCreate; pc != nil {
		pcVars := make(map[string]string, len(authData)+1)
		for key, value := range authData {
			pcVars[key] = value
		}
		pcVars["address"] = address
		raw, err := p.call(nil, pc.Request, pcVars)
		if err != nil {
			// 上游对换 token 的非 2xx 静默忽略；只有连不上才让创建失败。
			var status mailStatusError
			if !errors.As(err, &status) {
				return mailInboxData{}, err
			}
		} else if data, err := mailDecodeJSON(raw); err == nil {
			for key, path := range pc.AuthPaths {
				if value := mailResolvePath(data, path); value != nil {
					authData[key] = mailString(value)
				}
			}
		}
	}

	in := mailInboxData{Provider: cfg.Name, Address: address, AuthData: authData, APIBase: cfg.APIBase}
	if cfg.Create.ExpiresIn > 0 {
		in.ExpiresAt = time.Now().Add(time.Duration(cfg.Create.ExpiresIn) * time.Second).UTC()
	}
	return in, nil
}

func (p *mailProvider) Messages(in mailInboxData) ([]mailMessage, error) {
	cfg := p.cfg.Messages
	raw, err := p.call(&in, cfg.Request, mailVars(in))
	if err != nil {
		return nil, err
	}
	data, err := mailDecodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: 上游返回格式无效", p.cfg.Name)
	}
	items, ok := mailResolvePath(data, mailRootPath(cfg.ResultPath)).([]any)
	if !ok {
		return nil, nil
	}
	p.rememberItems(in.Address, items)
	messages := make([]mailMessage, 0, len(items))
	for _, item := range items {
		messages = append(messages, mailMessage{
			ID:         mailString(mailResolvePath(item, cfg.Mapping.ID)),
			From:       mailString(mailResolvePath(item, cfg.Mapping.From)),
			Subject:    mailString(mailResolvePath(item, cfg.Mapping.Subject)),
			Excerpt:    mailClip(mailString(mailResolvePath(item, cfg.Mapping.Excerpt)), 200),
			ReceivedAt: mailTime(mailResolvePath(item, cfg.Mapping.ReceivedAt), cfg.TimestampFormat),
		})
	}
	return messages, nil
}

func (p *mailProvider) Message(in mailInboxData, id string) (mailMessage, error) {
	cfg := p.cfg.MessageDetail
	if cfg.FromList {
		items := p.recentItems(in.Address)
		if items == nil {
			if _, err := p.Messages(in); err != nil {
				return mailMessage{}, err
			}
			items = p.recentItems(in.Address)
		}
		list := p.cfg.Messages
		for _, item := range items {
			if mailString(mailResolvePath(item, list.Mapping.ID)) != id {
				continue
			}
			return mailMessage{
				ID:         id,
				From:       mailString(mailResolvePath(item, list.Mapping.From)),
				Subject:    mailString(mailResolvePath(item, list.Mapping.Subject)),
				Excerpt:    mailClip(mailString(mailResolvePath(item, list.Mapping.Excerpt)), 200),
				Text:       mailString(mailResolvePath(item, list.Mapping.Text)),
				HTML:       mailString(mailResolvePath(item, list.Mapping.HTML)),
				ReceivedAt: mailTime(mailResolvePath(item, list.Mapping.ReceivedAt), list.TimestampFormat),
			}, nil
		}
		return mailMessage{}, fmt.Errorf("%s: 未找到邮件", p.cfg.Name)
	}

	vars := mailVars(in)
	vars["messageId"] = id
	raw, err := p.call(&in, cfg.Request, vars)
	if err != nil {
		return mailMessage{}, err
	}
	data, err := mailDecodeJSON(raw)
	if err != nil {
		return mailMessage{}, fmt.Errorf("%s: 上游返回格式无效", p.cfg.Name)
	}
	text := mailString(mailResolvePath(data, cfg.Mapping.Text))
	excerpt := cfg.Mapping.Excerpt
	if excerpt == "" {
		excerpt = cfg.Mapping.Text
	}
	messageID := mailString(mailResolvePath(data, cfg.Mapping.ID))
	if messageID == "" {
		messageID = id
	}
	return mailMessage{
		ID:         messageID,
		From:       mailString(mailResolvePath(data, cfg.Mapping.From)),
		Subject:    mailString(mailResolvePath(data, cfg.Mapping.Subject)),
		Excerpt:    mailClip(mailString(mailResolvePath(data, excerpt)), 200),
		Text:       text,
		HTML:       mailString(mailResolvePath(data, cfg.Mapping.HTML)),
		ReceivedAt: mailTime(mailResolvePath(data, cfg.Mapping.ReceivedAt), cfg.TimestampFormat),
	}, nil
}

func (p *mailProvider) Close(in mailInboxData) error {
	if p.cfg.Delete == nil {
		return nil
	}
	_, err := p.call(&in, *p.cfg.Delete, mailVars(in))
	var status mailStatusError
	// 上游删除接口的状态码不可控（可能已过期返回 404），本地关闭不应被它阻塞。
	if errors.As(err, &status) {
		return nil
	}
	return err
}

func (p *mailProvider) rememberItems(address string, items []any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.items == nil {
		p.items = make(map[string][]any)
	}
	// 上限保护：长期运行不能靠每封邮件无限攒内存，淘汰谁不影响正确性。
	if len(p.items) >= mailMessageCacheMax {
		for key := range p.items {
			delete(p.items, key)
			break
		}
	}
	p.items[address] = items
}

func (p *mailProvider) recentItems(address string) []any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.items[address]
}

func (p *mailProvider) call(in *mailInboxData, req mailRequest, vars map[string]string) ([]byte, error) {
	base := p.cfg.APIBase
	if in != nil && in.APIBase != "" {
		base = in.APIBase
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	contentType := "application/json"
	var body io.Reader
	if req.Body != nil {
		serialized, serializedType := mailSerializeBody(mailInterpolateBody(req.Body, vars), req.BodyType)
		body = strings.NewReader(serialized)
		contentType = serializedType
	}
	httpReq, err := http.NewRequest(method, strings.TrimRight(base, "/")+mailInterpolate(req.Path, vars), body)
	if err != nil {
		return nil, fmt.Errorf("%s: 请求地址无效", p.cfg.Name)
	}
	httpReq.Header.Set("Content-Type", contentType)
	for key, value := range p.cfg.ExtraHeaders {
		httpReq.Header.Set(key, value)
	}
	if req.AuthFrom == "inbox" {
		field := req.AuthField
		if field == "" {
			field = "token"
		}
		token := ""
		if in != nil {
			token = in.AuthData[field]
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := mailUpstreamClient.Do(httpReq)
	if err != nil {
		// 底层错误里可能带完整 URL（含 token 查询参数），只报可读结论。
		return nil, fmt.Errorf("%s: 无法连接上游服务，请稍后重试", p.cfg.Name)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, mailMaxResponse))
		return nil, mailStatusError{provider: p.cfg.Name, status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, mailMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("%s: 读取上游响应失败", p.cfg.Name)
	}
	if len(data) > mailMaxResponse {
		return nil, fmt.Errorf("%s: 上游响应过大", p.cfg.Name)
	}
	return data, nil
}

// mailStatusError 只描述状态码：上游响应体可能包含凭据，绝不能进错误文案。
type mailStatusError struct {
	provider string
	status   int
}

func (e mailStatusError) Error() string {
	return fmt.Sprintf("%s: 上游返回 HTTP %d", e.provider, e.status)
}

func mailVars(in mailInboxData) map[string]string {
	vars := make(map[string]string, len(in.AuthData)+1)
	for key, value := range in.AuthData {
		vars[key] = value
	}
	vars["address"] = in.Address
	return vars
}

func mailRootPath(path string) string {
	if path == "" {
		return "$root"
	}
	return path
}

func mailMethodAllowsBody(method string) bool {
	return method != "" && method != "GET" && method != "HEAD"
}

var mailTemplateRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

func mailInterpolate(template string, vars map[string]string) string {
	if !strings.Contains(template, "{{") {
		return template
	}
	return mailTemplateRE.ReplaceAllStringFunc(template, func(match string) string {
		return vars[mailTemplateRE.FindStringSubmatch(match)[1]]
	})
}

func mailInterpolateBody(body map[string]any, vars map[string]string) map[string]any {
	out := make(map[string]any, len(body))
	for key, value := range body {
		switch typed := value.(type) {
		case string:
			out[key] = mailInterpolate(typed, vars)
		case map[string]any:
			out[key] = mailInterpolateBody(typed, vars)
		default:
			out[key] = value
		}
	}
	return out
}

func mailSerializeBody(body map[string]any, bodyType string) (string, string) {
	if bodyType == "form" {
		form := url.Values{}
		for key, value := range body {
			if value == nil {
				continue
			}
			if text, ok := value.(string); ok {
				form.Set(key, text)
			} else if raw, err := json.Marshal(value); err == nil {
				form.Set(key, string(raw))
			}
		}
		return form.Encode(), "application/x-www-form-urlencoded"
	}
	raw, _ := json.Marshal(body)
	return string(raw), "application/json"
}

func mailDecodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func mailResolvePath(root any, path string) any {
	if path == "$root" {
		return root
	}
	if path == "" {
		return nil
	}
	current := root
	for _, key := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}
	return current
}

func mailString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	}
	return ""
}

func mailScalar(value any) string {
	switch value.(type) {
	case string, json.Number, bool, float64, nil:
		return mailString(value)
	}
	return fmt.Sprint(value)
}

func mailInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return n, true
		}
		if f, err := typed.Float64(); err == nil {
			return int64(f), true
		}
	case float64:
		return int64(typed), true
	case string:
		if n, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func mailTime(value any, format string) string {
	// 上游用 0 表示「没有时间」（例如 Guerrilla Mail 的欢迎邮件 mail_timestamp），
	// 不能当 1970 年展示。
	switch format {
	case "unix_seconds":
		if n, ok := mailInt(value); ok && n > 0 {
			return time.Unix(n, 0).UTC().Format(time.RFC3339)
		}
		return ""
	case "unix_milliseconds":
		if n, ok := mailInt(value); ok && n > 0 {
			return time.UnixMilli(n).UTC().Format(time.RFC3339)
		}
		return ""
	}
	text := mailString(value)
	if text == "" {
		return ""
	}
	// 只接受带时区的绝对时间；无时区的上游时间无法确定时刻，保持未知。
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func mailClip(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}

func mailRandomString(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, length)
	rand.Read(buf)
	for index, byteValue := range buf {
		buf[index] = alphabet[int(byteValue)%len(alphabet)]
	}
	return string(buf)
}

func mailRandomPassword() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func mailRandomIndex(n int) int {
	if n <= 1 {
		return 0
	}
	buf := make([]byte, 1)
	rand.Read(buf)
	return int(buf[0]) % n
}

func mailProviderByName(name string) *mailProvider {
	for _, provider := range mailProviders {
		if provider.cfg.Name == name {
			return provider
		}
	}
	return nil
}

var mailProviders = []*mailProvider{
	newMailProvider(mailTemplatedConfig("mailtm", "Mail.tm", "https://api.mail.tm", "~days", 60)),
	newMailProvider(mailTemplatedConfig("mailgw", "Mail.gw", "https://api.mail.gw", "8 days", 50)),
	newMailProvider(mailProviderConfig{
		Name:          "tempmail-lol",
		DisplayName:   "TempMail.lol",
		Tier:          "free",
		Retention:     "1h",
		PollPerMinute: 10,
		Features:      mailFeatures{CustomUsername: true, PollInbox: true},
		APIBase:       "https://api.tempmail.lol/v2",
		Domains:       mailDomainsConfig{Mode: "from_create"},
		Create: mailCreateConfig{
			Request: mailRequest{
				Path: "/inbox/create", Method: "POST",
				Body: map[string]any{"community": true, "prefix": "{{username}}", "domain": "{{domain}}"},
			},
			AddressPath: "address",
			AuthPaths:   map[string]string{"token": "token"},
			ExpiresIn:   3600,
		},
		Messages: mailMessagesConfig{
			Request:         mailRequest{Path: "/inbox?token={{token}}", Method: "GET", AuthFrom: "inbox", AuthField: "token"},
			ResultPath:      "emails",
			TimestampFormat: "unix_milliseconds",
			Mapping:         mailMapping{ID: "date", From: "from", Subject: "subject", Excerpt: "body", ReceivedAt: "date", Text: "body", HTML: "html"},
		},
		MessageDetail: mailMessageDetailConfig{
			FromList:        true,
			TimestampFormat: "unix_milliseconds",
			Mapping:         mailMapping{ID: "date", From: "from", Subject: "subject", Text: "body", HTML: "html", ReceivedAt: "date"},
		},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "tempmail-ing",
		DisplayName:   "TempMail.ing",
		Tier:          "free",
		Retention:     "10min",
		PollPerMinute: 10,
		Features:      mailFeatures{PollInbox: true},
		APIBase:       "https://api.tempmail.ing",
		Domains:       mailDomainsConfig{Mode: "from_create"},
		Create: mailCreateConfig{
			Request:     mailRequest{Path: "/api/generate", Method: "POST", Body: map[string]any{"duration": 10}},
			AddressPath: "email.address",
			AuthPaths:   map[string]string{"address": "email.address"},
			ExpiresIn:   600,
		},
		Messages: mailMessagesConfig{
			Request:    mailRequest{Path: "/api/emails/{{address}}", Method: "GET", AuthFrom: "inbox", AuthField: "address"},
			ResultPath: "emails",
			Mapping:    mailMapping{ID: "id", From: "from_address", Subject: "subject", Excerpt: "text", ReceivedAt: "received_at", Text: "text", HTML: "content"},
		},
		MessageDetail: mailMessageDetailConfig{
			FromList: true,
			Mapping:  mailMapping{ID: "id", From: "from_address", Subject: "subject", Text: "text", HTML: "content", ReceivedAt: "received_at"},
		},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "guerrillamail",
		DisplayName:   "Guerrilla Mail",
		Tier:          "free",
		Retention:     "1h",
		PollPerMinute: 60,
		Features:      mailFeatures{PollInbox: true},
		APIBase:       "https://www.guerrillamail.com/ajax.php",
		Domains:       mailDomainsConfig{Mode: "static", List: []string{"guerrillamailblock.com", "sharklasers.com"}},
		Create: mailCreateConfig{
			Request:     mailRequest{Path: "?f=get_email_address", Method: "GET", AuthFrom: "provider"},
			AddressPath: "email_addr",
			AuthPaths:   map[string]string{"sid": "sid_token"},
			ExpiresIn:   3600,
		},
		Messages: mailMessagesConfig{
			Request:         mailRequest{Path: "?f=check_email&sid_token={{sid}}&seq=0", Method: "GET", AuthFrom: "provider"},
			ResultPath:      "list",
			TimestampFormat: "unix_seconds",
			Mapping:         mailMapping{ID: "mail_id", From: "mail_from", Subject: "mail_subject", Excerpt: "mail_excerpt", ReceivedAt: "mail_timestamp"},
		},
		MessageDetail: mailMessageDetailConfig{
			Request:         mailRequest{Path: "?f=fetch_email&sid_token={{sid}}&email_id={{messageId}}", Method: "GET", AuthFrom: "provider"},
			TimestampFormat: "unix_seconds",
			Mapping:         mailMapping{ID: "mail_id", From: "mail_from", Subject: "mail_subject", Text: "mail_text", HTML: "mail_body", ReceivedAt: "mail_timestamp"},
		},
		Delete: &mailRequest{Path: "?f=forget_me&sid_token={{sid}}&email_addr={{address}}", Method: "GET", AuthFrom: "provider"},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "maildrop",
		DisplayName:   "Maildrop.cc",
		Tier:          "free",
		Retention:     "~24h (shared inbox, publicly readable)",
		PollPerMinute: 30,
		Features:      mailFeatures{CustomUsername: true, PollInbox: true},
		APIBase:       "https://api.maildrop.cc/graphql",
		Domains:       mailDomainsConfig{Mode: "static", List: []string{"maildrop.cc"}},
		Create:        mailCreateConfig{Skip: true},
		Messages: mailMessagesConfig{
			Request: mailRequest{
				Path: "", Method: "POST", AuthFrom: "provider",
				Body: map[string]any{"query": `query { inbox(mailbox:"{{username}}") { id headerfrom subject date } }`},
			},
			ResultPath: "data.inbox",
			Mapping:    mailMapping{ID: "id", From: "headerfrom", Subject: "subject", Excerpt: "subject", ReceivedAt: "date"},
		},
		MessageDetail: mailMessageDetailConfig{
			Request: mailRequest{
				Path: "", Method: "POST", AuthFrom: "provider",
				Body: map[string]any{"query": `query { message(mailbox:"{{username}}", id:"{{messageId}}") { id headerfrom subject date data html } }`},
			},
			Mapping: mailMapping{ID: "data.message.id", From: "data.message.headerfrom", Subject: "data.message.subject", Text: "data.message.data", HTML: "data.message.html", ReceivedAt: "data.message.date"},
		},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "tempmail-io",
		DisplayName:   "Temp-Mail.io",
		Tier:          "free",
		Retention:     "~24h",
		PollPerMinute: 30,
		Features:      mailFeatures{PollInbox: true},
		APIBase:       "https://api.internal.temp-mail.io",
		Domains: mailDomainsConfig{
			Mode:         "endpoint",
			Request:      mailRequest{Path: "/api/v3/domains", Method: "GET", AuthFrom: "provider"},
			ResultPath:   "domains",
			DomainField:  "name",
			FilterField:  "type",
			FilterEquals: "public",
		},
		Create: mailCreateConfig{
			Request:     mailRequest{Path: "/api/v3/email/new", Method: "POST", Body: map[string]any{"min_name_length": 10, "max_name_length": 10}},
			AddressPath: "email",
			AuthPaths:   map[string]string{"token": "token"},
		},
		Messages: mailMessagesConfig{
			Request: mailRequest{Path: "/api/v3/email/{{address}}/messages", Method: "GET", AuthFrom: "provider"},
			Mapping: mailMapping{ID: "id", From: "from", Subject: "subject", Excerpt: "body_text", ReceivedAt: "created_at", Text: "body_text", HTML: "body_html"},
		},
		MessageDetail: mailMessageDetailConfig{
			FromList: true,
			Mapping:  mailMapping{ID: "id", From: "from", Subject: "subject", Text: "body_text", HTML: "body_html", ReceivedAt: "created_at"},
		},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "catchmail",
		DisplayName:   "Catchmail.io",
		Tier:          "free",
		Retention:     "7 days",
		PollPerMinute: 30,
		Features:      mailFeatures{CustomUsername: true, PollInbox: true, Attachments: true},
		APIBase:       "https://api.catchmail.io",
		Domains:       mailDomainsConfig{Mode: "static", List: []string{"catchmail.io", "mailistry.com", "zeppost.com"}},
		Create:        mailCreateConfig{Skip: true},
		Messages: mailMessagesConfig{
			Request:    mailRequest{Path: "/api/v1/mailbox?address={{address}}", Method: "GET", AuthFrom: "provider"},
			ResultPath: "messages",
			Mapping:    mailMapping{ID: "id", From: "from", Subject: "subject", Excerpt: "subject", ReceivedAt: "date"},
		},
		MessageDetail: mailMessageDetailConfig{
			Request: mailRequest{Path: "/api/v1/message/{{messageId}}?mailbox={{address}}", Method: "GET", AuthFrom: "provider"},
			Mapping: mailMapping{ID: "id", From: "from", Subject: "subject", Text: "body.text", HTML: "body.html", ReceivedAt: "date"},
		},
	}),
	newMailProvider(mailProviderConfig{
		Name:          "nimail-cn",
		DisplayName:   "NiMail.cn",
		Tier:          "free",
		Retention:     "10min (resets on each request)",
		PollPerMinute: 30,
		Features:      mailFeatures{CustomUsername: true, PollInbox: true},
		APIBase:       "https://www.nimail.cn",
		ExtraHeaders:  map[string]string{"Origin": "https://www.nimail.cn", "Referer": "https://www.nimail.cn/"},
		Domains:       mailDomainsConfig{Mode: "static", List: []string{"nimail.cn"}},
		Create: mailCreateConfig{
			Request:     mailRequest{Path: "/api/applymail", Method: "POST", Body: map[string]any{"mail": "{{address}}"}, BodyType: "form"},
			AddressPath: "user",
			ExpiresIn:   600,
		},
		Messages: mailMessagesConfig{
			Request:    mailRequest{Path: "/api/getmails", Method: "POST", Body: map[string]any{"mail": "{{address}}", "time": "0"}, BodyType: "form", AuthFrom: "provider"},
			ResultPath: "mail",
			Mapping:    mailMapping{ID: "id", From: "from", Subject: "subject", Excerpt: "text", ReceivedAt: "time", Text: "text", HTML: "html"},
		},
		MessageDetail: mailMessageDetailConfig{
			FromList: true,
			Mapping:  mailMapping{ID: "id", From: "from", Subject: "subject", Text: "text", HTML: "html", ReceivedAt: "time"},
		},
	}),
}

// mailTemplatedConfig 是 mail.tm / mail.gw 共用的声明式配置，两者只有域名和限额不同。
func mailTemplatedConfig(name, displayName, apiBase, retention string, pollPerMinute int) mailProviderConfig {
	return mailProviderConfig{
		Name:          name,
		DisplayName:   displayName,
		Tier:          "free",
		Retention:     retention,
		PollPerMinute: pollPerMinute,
		Features:      mailFeatures{CustomUsername: true, PollInbox: true},
		APIBase:       apiBase,
		ExtraHeaders:  map[string]string{"Accept": "application/ld+json"},
		Domains: mailDomainsConfig{
			Mode:         "endpoint",
			Request:      mailRequest{Path: "/domains", Method: "GET", AuthFrom: "provider"},
			ResultPath:   "hydra:member",
			DomainField:  "domain",
			FilterField:  "isActive",
			FilterEquals: true,
		},
		Create: mailCreateConfig{
			Request: mailRequest{
				Path: "/accounts", Method: "POST",
				Body: map[string]any{"address": "{{address}}", "password": "{{password}}"},
			},
			AddressPath: "address",
			AuthPaths:   map[string]string{"accountId": "id"},
			ExpiresIn:   86400,
		},
		PostCreate: &mailPostCreateConfig{
			Request: mailRequest{
				Path: "/token", Method: "POST",
				Body: map[string]any{"address": "{{address}}", "password": "{{password}}"},
			},
			AuthPaths: map[string]string{"token": "token"},
		},
		Messages: mailMessagesConfig{
			Request:    mailRequest{Path: "/messages", Method: "GET", AuthFrom: "inbox", AuthField: "token"},
			ResultPath: "hydra:member",
			Mapping:    mailMapping{ID: "id", From: "from.address", Subject: "subject", Excerpt: "intro", ReceivedAt: "createdAt"},
		},
		MessageDetail: mailMessageDetailConfig{
			Request: mailRequest{Path: "/messages/{{messageId}}", Method: "GET", AuthFrom: "inbox", AuthField: "token"},
			Mapping: mailMapping{ID: "id", From: "from.address", Subject: "subject", Text: "text", HTML: "html", ReceivedAt: "createdAt"},
		},
		Delete: &mailRequest{Path: "/accounts/{{accountId}}", Method: "DELETE", AuthFrom: "inbox", AuthField: "token"},
	}
}
