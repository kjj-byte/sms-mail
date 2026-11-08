package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// mailEngineProvider 复制内置渠道配置并改指 httptest，这样测的是真实声明式配置而不只是引擎骨架。
func mailEngineProvider(t *testing.T, name, apiBase string) *mailProvider {
	t.Helper()
	source := mailProviderByName(name)
	if source == nil {
		t.Fatalf("内置渠道 %q 不存在", name)
	}
	provider := newMailProvider(source.cfg)
	provider.cfg.APIBase = apiBase
	return provider
}

func mailTestJSONBody(t *testing.T, r *http.Request) map[string]any {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("读取请求体失败: %v", err)
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Errorf("请求体不是 JSON: %v", err)
		return nil
	}
	return body
}

func TestMailProviderMailTMLifecycle(t *testing.T) {
	var domainCalls, tokenCalls, deleteCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			domainCalls++
			if r.Header.Get("Accept") != "application/ld+json" {
				t.Errorf("域名请求缺少 Accept 头: %q", r.Header.Get("Accept"))
			}
			io.WriteString(w, `{"hydra:member":[{"domain":"active.test","isActive":true},{"domain":"dead.test","isActive":false}]}`)
		case "/accounts":
			body := mailTestJSONBody(t, r)
			address, _ := body["address"].(string)
			if !strings.HasSuffix(address, "@active.test") {
				t.Errorf("创建地址未使用活跃域名: %q", address)
			}
			if password, _ := body["password"].(string); len(password) < 16 {
				t.Errorf("创建密码太弱: %q", password)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"acc1","address":"`+address+`"}`)
		case "/token":
			tokenCalls++
			body := mailTestJSONBody(t, r)
			if body["address"] == "" || body["password"] == "" {
				t.Errorf("换 token 请求缺少地址或密码: %v", body)
			}
			io.WriteString(w, `{"token":"tok123"}`)
		case "/messages":
			if r.Header.Get("Authorization") != "Bearer tok123" {
				t.Errorf("列表取件未带 Bearer token: %q", r.Header.Get("Authorization"))
			}
			io.WriteString(w, `{"hydra:member":[{"id":"m1","from":{"address":"sender@x.test"},"subject":"验证码","intro":"code 482910","createdAt":"2026-01-02T03:04:05Z"}]}`)
		case "/messages/m1":
			if r.Header.Get("Authorization") != "Bearer tok123" {
				t.Errorf("详情取件未带 Bearer token: %q", r.Header.Get("Authorization"))
			}
			io.WriteString(w, `{"id":"m1","from":{"address":"sender@x.test"},"subject":"验证码","text":"验证码 482910","html":"<b>482910</b>","createdAt":"2026-01-02T03:04:05Z"}`)
		case "/accounts/acc1":
			deleteCalls++
			if r.Method != http.MethodDelete {
				t.Errorf("删除方法错误: %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "mailtm", upstream.URL)
	if domains := provider.Domains(); len(domains) != 1 || domains[0] != "active.test" {
		t.Fatalf("域名过滤失败: %v", domains)
	}
	provider.Domains()
	if domainCalls != 1 {
		t.Fatalf("域名成功结果未缓存，调用了 %d 次", domainCalls)
	}

	inbox, err := provider.Create("", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Address != "alice@active.test" || inbox.AuthData["accountId"] != "acc1" || inbox.AuthData["token"] != "tok123" {
		t.Fatalf("创建结果映射错误: %+v", inbox)
	}
	if inbox.Provider != "mailtm" || inbox.APIBase != upstream.URL {
		t.Fatalf("收件箱元数据错误: %+v", inbox)
	}
	if until := time.Until(inbox.ExpiresAt); until <= 0 || until > 25*time.Hour {
		t.Fatalf("过期时间错误: %v", inbox.ExpiresAt)
	}
	if tokenCalls != 1 {
		t.Fatalf("换 token 请求次数错误: %d", tokenCalls)
	}

	messages, err := provider.Messages(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("邮件数错误: %d", len(messages))
	}
	got := messages[0]
	if got.ID != "m1" || got.From != "sender@x.test" || got.Subject != "验证码" || got.Excerpt != "code 482910" || got.ReceivedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("列表映射错误: %+v", got)
	}

	detail, err := provider.Message(inbox, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Text != "验证码 482910" || detail.HTML != "<b>482910</b>" || detail.ReceivedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("详情映射错误: %+v", detail)
	}

	if err := provider.Close(inbox); err != nil {
		t.Fatalf("关闭收件箱失败: %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("删除上游收件箱次数错误: %d", deleteCalls)
	}
}

func TestMailProviderTempMailLOLFromCreateUnixMilliseconds(t *testing.T) {
	createCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/inbox/create":
			createCalls++
			body := mailTestJSONBody(t, r)
			if body["community"] != true {
				t.Errorf("community 参数丢失: %v", body["community"])
			}
			if prefix, _ := body["prefix"].(string); prefix == "" {
				t.Errorf("prefix 参数丢失: %v", body["prefix"])
			}
			if _, ok := body["domain"]; !ok {
				t.Error("domain 字段缺失")
			}
			io.WriteString(w, `{"address":"abc@tempmail.lol","token":"loltok"}`)
		case "/inbox":
			if r.URL.Query().Get("token") != "loltok" {
				t.Errorf("token 查询参数缺失: %s", r.URL.RawQuery)
			}
			if r.Header.Get("Authorization") != "Bearer loltok" {
				t.Errorf("缺少 Bearer token: %q", r.Header.Get("Authorization"))
			}
			io.WriteString(w, `{"emails":[{"date":1735689600000,"from":"s@x.test","subject":"验证码 482910","body":"验证码 482910","html":"<p>482910</p>"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "tempmail-lol", upstream.URL)
	inbox, err := provider.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Address != "abc@tempmail.lol" || inbox.AuthData["token"] != "loltok" {
		t.Fatalf("创建结果映射错误: %+v", inbox)
	}
	if until := time.Until(inbox.ExpiresAt); until <= 0 || until > time.Hour+time.Minute {
		t.Fatalf("过期时间错误: %v", inbox.ExpiresAt)
	}

	messages, err := provider.Messages(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("邮件数错误: %d", len(messages))
	}
	if messages[0].ReceivedAt != "2025-01-01T00:00:00Z" {
		t.Fatalf("毫秒时间戳转换错误: %q", messages[0].ReceivedAt)
	}
	if messages[0].ID != "1735689600000" {
		t.Fatalf("数字 ID 转换错误: %q", messages[0].ID)
	}

	detail, err := provider.Message(inbox, messages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Text != "验证码 482910" || detail.HTML != "<p>482910</p>" || detail.ReceivedAt != "2025-01-01T00:00:00Z" {
		t.Fatalf("fromList 详情映射错误: %+v", detail)
	}

	// from_create 渠道只能先真的创建一个收件箱才知道域名；5 分钟内重复查询走缓存。
	before := createCalls
	domains := provider.Domains()
	provider.Domains()
	if len(domains) != 1 || domains[0] != "tempmail.lol" {
		t.Fatalf("from_create 域名推导错误: %v", domains)
	}
	if createCalls != before+1 {
		t.Fatalf("from_create 域名结果未缓存: %d", createCalls-before)
	}
}

func TestMailProviderMaildropSkipCreateGraphQL(t *testing.T) {
	var requests []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		body := mailTestJSONBody(t, r)
		query, _ := body["query"].(string)
		switch {
		case strings.Contains(query, "inbox(mailbox"):
			if !strings.Contains(query, `mailbox:"alice"`) {
				t.Errorf("列表查询没有用户名: %s", query)
			}
			io.WriteString(w, `{"data":{"inbox":[{"id":"m1","headerfrom":"s@x.test","subject":"验证码","date":"2026-01-02T03:04:05Z"},{"id":"m2","headerfrom":"a@b","subject":"`+strings.Repeat("长", 300)+`","date":""}]}}`)
		case strings.Contains(query, "message(mailbox"):
			if !strings.Contains(query, `id:"m1"`) {
				t.Errorf("详情查询缺少邮件 id: %s", query)
			}
			io.WriteString(w, `{"data":{"message":{"id":"m1","headerfrom":"s@x.test","subject":"验证码","date":"2026-01-02T03:04:05Z","data":"482910 是您的验证码","html":"<p>482910</p>"}}}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "maildrop", upstream.URL)
	inbox, err := provider.Create("", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Address != "alice@maildrop.cc" {
		t.Fatalf("skip 渠道地址合成错误: %q", inbox.Address)
	}
	if len(requests) != 0 {
		t.Fatalf("skip 渠道不应请求上游: %v", requests)
	}

	messages, err := provider.Messages(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("邮件数错误: %d", len(messages))
	}
	if messages[0].ID != "m1" || messages[0].From != "s@x.test" || messages[0].ReceivedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("列表映射错误: %+v", messages[0])
	}
	if messages[1].ReceivedAt != "" {
		t.Fatalf("未知时间应为空: %q", messages[1].ReceivedAt)
	}
	if count := utf8.RuneCountInString(messages[1].Excerpt); count != 200 {
		t.Fatalf("摘要截断错误: %d", count)
	}

	detail, err := provider.Message(inbox, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Text != "482910 是您的验证码" || detail.HTML != "<p>482910</p>" || detail.ID != "m1" {
		t.Fatalf("详情映射错误: %+v", detail)
	}
}

func TestMailProviderNiMailFormBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://www.nimail.cn" || r.Header.Get("Referer") != "https://www.nimail.cn/" {
			t.Errorf("附加请求头丢失: %v", r.Header)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("表单内容类型错误: %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("解析表单失败: %v", err)
			return
		}
		switch r.URL.Path {
		case "/api/applymail":
			if r.PostForm.Get("mail") != "bob@nimail.cn" {
				t.Errorf("创建表单参数错误: %v", r.PostForm)
			}
			io.WriteString(w, `{"user":"bob@nimail.cn"}`)
		case "/api/getmails":
			if r.PostForm.Get("mail") != "bob@nimail.cn" || r.PostForm.Get("time") != "0" {
				t.Errorf("列表表单参数错误: %v", r.PostForm)
			}
			io.WriteString(w, `{"mail":[{"id":"1","from":"s@x.test","subject":"验证码","text":"482910","html":"<p>482910</p>","time":"2026-01-02 03:04:05"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "nimail-cn", upstream.URL)
	inbox, err := provider.Create("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Address != "bob@nimail.cn" {
		t.Fatalf("创建返回地址映射错误: %q", inbox.Address)
	}
	messages, err := provider.Messages(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Excerpt != "482910" || messages[0].Subject != "验证码" {
		t.Fatalf("列表映射错误: %+v", messages)
	}
	if messages[0].ReceivedAt != "" {
		t.Fatalf("无时区的上游时间应保持未知: %q", messages[0].ReceivedAt)
	}
	detail, err := provider.Message(inbox, "1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Text != "482910" || detail.Excerpt != "482910" {
		t.Fatalf("fromList 详情映射错误: %+v", detail)
	}
}

func TestMailProviderGuerrillaMailQueryAPI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax.php" {
			t.Errorf("请求路径错误: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Query().Get("f") {
		case "get_email_address":
			io.WriteString(w, `{"email_addr":"xyz@sharklasers.com","sid_token":"sid1"}`)
		case "check_email":
			if r.URL.Query().Get("sid_token") != "sid1" {
				t.Errorf("sid 未带: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"list":[{"mail_id":"m1","mail_from":"s@x.test","mail_subject":"hi","mail_excerpt":"e","mail_timestamp":1700000000}]}`)
		case "fetch_email":
			if r.URL.Query().Get("email_id") != "m1" || r.URL.Query().Get("sid_token") != "sid1" {
				t.Errorf("详情参数错误: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"mail_id":"m1","mail_from":"s@x.test","mail_subject":"hi","mail_text":"code 123456","mail_body":"<p>123456</p>","mail_timestamp":1700000000}`)
		case "forget_me":
			if r.URL.Query().Get("email_addr") != "xyz@sharklasers.com" {
				t.Errorf("删除参数错误: %s", r.URL.RawQuery)
			}
			io.WriteString(w, `{"success":true}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "guerrillamail", upstream.URL+"/ajax.php")
	inbox, err := provider.Create("", "")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Address != "xyz@sharklasers.com" || inbox.AuthData["sid"] != "sid1" {
		t.Fatalf("创建结果映射错误: %+v", inbox)
	}
	if inbox.AuthData["username"] == "" {
		t.Fatal("随机用户名缺失")
	}
	messages, err := provider.Messages(inbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ReceivedAt != "2023-11-14T22:13:20Z" {
		t.Fatalf("列表映射错误: %+v", messages)
	}
	detail, err := provider.Message(inbox, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Text != "code 123456" || detail.HTML != "<p>123456</p>" {
		t.Fatalf("详情映射错误: %+v", detail)
	}
	if err := provider.Close(inbox); err != nil {
		t.Fatalf("关闭收件箱失败: %v", err)
	}
}

func TestMailProviderDomainFailureCacheAndNoLeak(t *testing.T) {
	const secret = "SECRET-UPSTREAM-DETAIL"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, secret)
	}))
	defer upstream.Close()

	provider := mailEngineProvider(t, "mailtm", upstream.URL)
	if domains := provider.Domains(); domains != nil {
		t.Fatalf("失败时不应返回域名: %v", domains)
	}
	if domains := provider.Domains(); domains != nil {
		t.Fatalf("失败时不应返回域名: %v", domains)
	}
	if calls != 1 {
		t.Fatalf("域名失败结果未缓存，请求了 %d 次", calls)
	}

	_, err := provider.Create("active.test", "alice")
	if err == nil {
		t.Fatal("非 2xx 创建应当失败")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("错误泄露了上游响应体: %v", err)
	}
	if !strings.Contains(err.Error(), "mailtm") || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("错误不可读: %v", err)
	}
}

func TestMailProviderDoesNotFollowRedirect(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		io.WriteString(w, `{"hydra:member":[]}`)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirector.Close()

	provider := mailEngineProvider(t, "mailtm", redirector.URL)
	if domains := provider.Domains(); domains != nil {
		t.Fatalf("重定向应视为失败: %v", domains)
	}
	if targetHits != 0 {
		t.Fatal("跟随了上游重定向")
	}
}
