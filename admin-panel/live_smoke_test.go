//go:build live

package main

// 联网冒烟测试：go test -tags live -run TestLiveMail -v
// 只验证真实上游可达与完整 HTTP 链路；不进入常规测试。

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func liveApp(t *testing.T) *App {
	t.Helper()
	a := &App{token: "csrf-test", addr: host, mail: newTestMailService(t.TempDir())}
	for _, p := range mailProviders {
		a.mail.channels = append(a.mail.channels, p)
	}
	return a
}

func TestLiveMailEveryProvider(t *testing.T) {
	a := liveApp(t)
	for _, p := range mailProviders {
		name := p.cfg.Name
		t.Run(name, func(t *testing.T) {
			w := mailCall(a, "POST", "/inbox", fmt.Sprintf(`{"for":"example.com","provider":%q}`, name), origin, a.token)
			if w.Code != 201 {
				t.Fatalf("create: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			var created map[string]any
			json.Unmarshal(w.Body.Bytes(), &created)
			id, _ := created["id"].(string)
			t.Cleanup(func() { mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token) })
			t.Logf("created %v (%v)", created["address"], created["provider"])

			w = mailCall(a, "GET", "/inbox/"+id+"/messages?includeCode=true", "", origin, a.token)
			if w.Code != 200 {
				t.Fatalf("messages: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			if !strings.Contains(w.Body.String(), `"verification"`) || !strings.Contains(w.Body.String(), `"pollAfter"`) {
				t.Fatalf("缺少工作台轮询字段: %s", w.Body.String())
			}
			t.Logf("messages: %s", strings.TrimSpace(w.Body.String()))

			w = mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token)
			if w.Code != 200 {
				t.Fatalf("code: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			t.Logf("code: %s", strings.TrimSpace(w.Body.String()))

			if w = mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 {
				t.Fatalf("delete: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
		})
	}
}

func TestLiveMailAttachDomains(t *testing.T) {
	a := liveApp(t)
	domains := a.mail.attachDomains()
	if len(domains) == 0 {
		t.Fatal("没有可接入域名")
	}
	t.Logf("attach domains: %v", domains)
	local := fmt.Sprintf("smoke%d", time.Now().UnixNano()%1e9)
	for _, domain := range domains {
		t.Run(domain, func(t *testing.T) {
			address := local + "@" + domain
			w := mailCall(a, "POST", "/inbox", fmt.Sprintf(`{"for":"example.com","address":%q}`, address), origin, a.token)
			if w.Code != 201 {
				t.Fatalf("attach: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			var created map[string]any
			json.Unmarshal(w.Body.Bytes(), &created)
			if created["address"] != address {
				t.Fatalf("地址被改写：%v", created["address"])
			}
			id := created["id"].(string)
			if w = mailCall(a, "GET", "/inbox/"+id+"/messages", "", origin, a.token); w.Code != 200 {
				t.Fatalf("messages: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			t.Logf("messages: %s", strings.TrimSpace(w.Body.String()))
			if w = mailCall(a, "GET", "/inbox/"+id+"/code", "", origin, a.token); w.Code != 200 {
				t.Fatalf("code: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
			}
			if w = mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 {
				t.Fatalf("delete: %d", w.Code)
			}
		})
	}
}

// TestLiveMailImapTLS 只验证真机能否完成 TLS + IMAP 协议握手：用错误凭据，
// 期望在 LOGIN 阶段被拒绝（说明前面的连接与协议都通了）。
func TestLiveMailImapTLS(t *testing.T) {
	c := newMailImapChannel()
	for _, address := range []string{"identity.smoke@gmail.com", "identity.smoke@qq.com"} {
		t.Run(address, func(t *testing.T) {
			if _, err := c.Connect(address, "definitely-not-the-password", ""); err == nil {
				t.Fatal("错误凭据竟然登录成功")
			} else {
				t.Logf("按预期拒绝：%v", err)
			}
		})
	}
}

func TestLiveMailAutoDispatch(t *testing.T) {
	a := liveApp(t)
	w := mailCall(a, "POST", "/inbox", `{"for":"example.com"}`, origin, a.token)
	if w.Code != 201 {
		t.Fatalf("auto create: %d %s", w.Code, strings.TrimSpace(w.Body.String()))
	}
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	t.Logf("auto dispatch picked %v: %v", created["provider"], created["address"])
	if w = mailCall(a, "GET", "/inboxes?status=active&pageSize=100", "", origin, a.token); !strings.Contains(w.Body.String(), created["address"].(string)) {
		t.Fatalf("list: %s", strings.TrimSpace(w.Body.String()))
	}
	id, _ := created["id"].(string)
	if w = mailCall(a, "DELETE", "/inbox/"+id, "", origin, a.token); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
}
