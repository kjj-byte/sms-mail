package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func testApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	catalog, e := filepath.Abs("../data/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	a, e := newApp(catalog, filepath.Join(dir, "referrals.json"), filepath.Join(dir, "missing.exe"), filepath.Join(dir, "dist"))
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func currentRevision(t *testing.T, a *App) string {
	t.Helper()
	_, rev, e := a.store.load()
	if e != nil {
		t.Fatal(e)
	}
	return rev
}
func request(a *App, method, path string, values url.Values, requestHost, requestOrigin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, origin+path, strings.NewReader(values.Encode()))
	r.Host = requestHost
	r.Header.Set("Origin", requestOrigin)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Invite-Admin", a.inviteKey)
	w := httptest.NewRecorder()
	a.handler(w, r)
	return w
}
func post(a *App, path string, v url.Values) *httptest.ResponseRecorder {
	v.Set("csrf", a.token)
	w := request(a, "POST", path, v, host, origin)
	if w.Code == 303 {
		return request(a, "GET", w.Header().Get("Location"), nil, host, origin)
	}
	return w
}

type pageData struct {
	Products map[string]struct {
		M  string   `json:"m"`
		Ps []string `json:"ps"`
	} `json:"products"`
	Pool []string `json:"pool"`
	Rows []struct {
		Pi string `json:"pi"`
		H  int    `json:"h"`
	} `json:"rows"`
}

func embeddedRows(t *testing.T, page string) pageData {
	t.Helper()
	m := regexp.MustCompile(`(?s)<script type="application/json" id="rows-data">(.*?)</script>`).FindStringSubmatch(page)
	if len(m) != 2 {
		t.Fatal("rows-data missing")
	}
	var d pageData
	if e := json.Unmarshal([]byte(m[1]), &d); e != nil {
		t.Fatal(e)
	}
	return d
}
func TestPublicInvitationManagementIsClosed(t *testing.T) {
	a := testApp(t)
	for _, path := range []string{"/save", "/restore", "/build", "/backup", "/styles.css", "/admin.js"} {
		for _, method := range []string{"GET", "POST"} {
			r := httptest.NewRequest(method, origin+path, strings.NewReader("csrf="+a.token))
			r.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			a.handler(w, r)
			if w.Code != 404 {
				t.Fatalf("%s %s exposed: %d", method, path, w.Code)
			}
		}
	}
	r := httptest.NewRequest("GET", origin+"/", nil)
	w := httptest.NewRecorder()
	a.handler(w, r)
	if w.Code != 303 || w.Header().Get("Location") != "/preview/" {
		t.Fatal("public root must open user workspace")
	}
	r = httptest.NewRequest("GET", origin+"/backup", nil)
	r.Header.Set("X-Invite-Admin", "wrong")
	w = httptest.NewRecorder()
	a.handler(w, r)
	if w.Code != 404 {
		t.Fatal("incorrect capability accepted")
	}
	r=httptest.NewRequest("GET",origin+"/backup",nil);r.Header.Set("X-Invite-Admin",a.token);w=httptest.NewRecorder();a.handler(w,r)
	if w.Code!=404||a.token==a.inviteKey{t.Fatal("public mail CSRF must not grant invitation access")}
}

func TestURLAndJSONBoundaries(t *testing.T) {
	if validateURL("https://5sim.net:", []string{"5sim.net"}) == nil {
		t.Fatal("empty explicit port accepted")
	}
	for _, raw := range []string{"http://5sim.net/", "https://5sim.net.evil.test/", "https://sub.5sim.net/", "https://name@5sim.net/", "https://5sim.net:8080/", "https://5sim.net/%00", "https://5sim.net/\r", strings.Repeat("x", 4097), "https://5sim.net\\@evil.test/"} {
		if validateURL(raw, []string{"5sim.net"}) == nil {
			t.Fatal("unsafe URL accepted", raw)
		}
	}
	for _, raw := range []string{`{"schema_version":1,"invitations":{},"extra":1}`, `{"schema_version":1,"invitations":{"x":"a","x":"b"}}`, `{"schema_version":1,"schema_version":1}`, `{} []`} {
		var r Referrals
		if strictJSON([]byte(raw), &r) == nil {
			t.Fatal("unsafe JSON accepted")
		}
	}
}
func TestSaveRestartConflictAndRestore(t *testing.T) {
	a := testApp(t)
	rev := currentRevision(t, a)
	raw := "https://5sim.net/register?b=2&b=1&x=%26%2F#/?fixture=1"
	w := post(a, "/save", url.Values{"id": {"fivesim"}, "url": {raw}, "revision": {rev}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "尚未更新导航") {
		t.Fatal(w.Code, w.Body.String())
	}
	b, e := newApp(a.catalog, a.store.path, a.builder, a.out)
	if e != nil {
		t.Fatal(e)
	}
	r, _, e := b.store.load()
	if e != nil || r.Invitations["fivesim"] != raw {
		t.Fatal("save not persistent", e)
	}
	w = post(a, "/save", url.Values{"id": {"smspool"}, "url": {"https://www.smspool.net/?fixture=1"}, "revision": {rev}})
	if w.Code != 409 {
		t.Fatal("stale page not rejected", w.Code)
	}
	w = post(a, "/save", url.Values{"id": {"smspool"}, "url": {"https://www.smspool.net/?fixture=1"}, "revision": {currentRevision(t, a)}})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = post(a, "/restore", url.Values{"id": {"fivesim"}, "revision": {currentRevision(t, a)}})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	r, _, _ = a.store.load()
	if r.Invitations["fivesim"] != "" || r.Invitations["smspool"] == "" {
		t.Fatal("restore changed other provider")
	}
	w = post(a, "/save", url.Values{"id": {"fivesim"}, "url": {"https://evil.test/?input=preserved"}, "revision": {currentRevision(t, a)}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "input=preserved") {
		t.Fatal("invalid input lost")
	}
	before, _ := os.ReadFile(a.store.path)
	a.store.path = filepath.Join(a.store.path, "impossible.json")
	if a.store.update("fivesim", raw, "", false) == nil {
		t.Fatal("write error ignored")
	}
	after, _ := os.ReadFile(b.store.path)
	if string(before) != string(after) {
		t.Fatal("write failure changed configuration")
	}
}
func TestHTTPGuardsAndPreview(t *testing.T) {
	a := testApp(t)
	values := url.Values{"csrf": {a.token}, "id": {"fivesim"}, "url": {"https://5sim.net/"}, "revision": {currentRevision(t, a)}}
	cases := []struct {
		method, path, h, o string
		v                  url.Values
		code               int
	}{
		{"GET", "/", "evil.test", origin, nil, 403}, {"GET", "/save", host, origin, values, 405}, {"POST", "/save", host, "https://evil.test", values, 403}, {"POST", "/save", host, "", values, 403}, {"POST", "/save", host, origin, url.Values{}, 403}, {"POST", "/save", host, origin, url.Values{"csrf": {a.token}, "id": {"bogus"}}, 400}, {"POST", "/save", host, origin, url.Values{"csrf": {a.token}, "command": {"whoami"}}, 400}, {"POST", "/save", host, origin, url.Values{"csrf": {a.token}, "url": {strings.Repeat("x", 20000)}}, 413}, {"GET", "/preview/../data/referrals.json", host, origin, nil, 404}, {"POST", "/preview/index.html", host, origin, nil, 405},
	}
	for _, c := range cases {
		w := request(a, c.method, c.path, c.v, c.h, c.o)
		if w.Code != c.code {
			t.Errorf("%s %s got %d want %d", c.method, c.path, w.Code, c.code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS enabled")
		}
	}
	os.MkdirAll(a.out, 0755)
	os.WriteFile(filepath.Join(a.out, "index.html"), []byte("old site"), 0644)
	w := post(a, "/build", url.Values{})
	if w.Code != 500 || !strings.Contains(w.Body.String(), "仍显示上一版") {
		t.Fatal("fake build success", w.Code)
	}
	b, _ := os.ReadFile(filepath.Join(a.out, "index.html"))
	if string(b) != "old site" {
		t.Fatal("build failure lost old site")
	}
	w = request(a, "GET", "/preview/", nil, host, origin)
	if w.Code != 200 || w.Body.String() != "old site" {
		t.Fatal("preview failed")
	}
	a.building.Lock()
	w = post(a, "/build", url.Values{})
	a.building.Unlock()
	if w.Code != 409 {
		t.Fatal("concurrent build allowed")
	}
}
func TestCorruptConfigNeverReset(t *testing.T) {
	a := testApp(t)
	for _, raw := range []string{"broken", `{"schema_version":2,"invitations":{}}`, `{"schema_version":1,"invitations":{"bogus":"https://5sim.net/"}}`} {
		os.WriteFile(a.store.path, []byte(raw), 0600)
		if _, e := newApp(a.catalog, a.store.path, a.builder, a.out); e == nil {
			t.Fatal("corrupt config silently accepted")
		}
		b, _ := os.ReadFile(a.store.path)
		if string(b) != raw {
			t.Fatal("corrupt file reset")
		}
	}
}
func TestNoPriceUpdateUI(t *testing.T) {
	a := testApp(t)
	w := request(a, "GET", "/", nil, host, origin)
	if body := w.Body.String(); strings.Contains(body, "/prices") || strings.Contains(body, "price-status") {
		t.Fatal("price update UI still rendered")
	}
	os.MkdirAll(a.out, 0755)
	os.WriteFile(filepath.Join(a.out, "index.html"), []byte("old site"), 0644)
	w = request(a, "GET", "/preview/", nil, host, origin)
	if strings.Contains(w.Body.String(), "preview-admin-bar") {
		t.Fatal("preview admin bar still injected")
	}
	if w = post(a, "/prices", url.Values{}); w.Code != 404 {
		t.Fatal("price update endpoint still served", w.Code)
	}
}
func TestAdminToGeneratedLinks(t *testing.T) {
	a := testApp(t)
	exe := "sitegen"
	goexe := "go"
	if runtime.GOOS == "windows" {
		exe += ".exe"
		goexe += ".exe"
	}
	a.builder = filepath.Join(t.TempDir(), exe)
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", goexe), "build", "-o", a.builder, "./cmd/sitegen")
	cmd.Dir = ".."
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	raw := "https://5sim.net/register?b=2&b=1&encoded=%26%2F#/?fixture=1"
	for id, value := range map[string]string{"fivesim": raw, "sms-man": "https://sms-man.com/?fixture=1"} {
		w := post(a, "/save", url.Values{"id": {id}, "url": {value}, "revision": {currentRevision(t, a)}})
		if w.Code != 200 {
			t.Fatal("save failed", w.Body.String())
		}
	}
	w := post(a, "/build", url.Values{})
	if w.Code != 200 {
		t.Fatal("build failed", w.Body.String())
	}
	w = request(a, "GET", "/preview/", nil, host, origin)
	data := embeddedRows(t, w.Body.String())
	purchase, rules := false, false
	for _, r := range data.Rows {
		if r.Pi == "fivesim" && r.H >= 0 && r.H < len(data.Pool) && data.Pool[r.H] == raw {
			purchase = true
		}
	}
	for _, s := range data.Products["fivesim-once"].Ps {
		if s == "https://5sim.net/rules" {
			rules = true
		}
	}
	if !purchase || data.Products["sms-man-rental"].M != "https://sms-man.com/rent" || !rules {
		t.Fatal("purchase, manage or source URL wrong")
	}
	w = post(a, "/restore", url.Values{"id": {"fivesim"}, "revision": {currentRevision(t, a)}})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if e := a.build(context.Background()); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(a.out, "index.html"))
	restored := embeddedRows(t, string(b))
	for _, v := range restored.Pool {
		if v == raw {
			t.Fatal("restore didn't reach site")
		}
	}
	foundSMS := false
	for _, v := range restored.Pool {
		if v == "https://sms-man.com/?fixture=1" {
			foundSMS = true
		}
	}
	if !foundSMS {
		t.Fatal("other provider changed")
	}
	w = request(a, "GET", "/backup", nil, host, origin)
	var refs Referrals
	if json.Unmarshal(w.Body.Bytes(), &refs) != nil || refs.Invitations["sms-man"] == "" {
		t.Fatal("backup mismatch")
	}
}
