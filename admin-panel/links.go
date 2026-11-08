package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
)

type Provider struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases"`
	URL      string   `json:"url"`
	Hosts    []string `json:"hosts"`
	APIHosts []string `json:"api_hosts,omitempty"`
	Note     string   `json:"note"`
	Sources  []string `json:"sources"`
	Verified string   `json:"verified"`
}
type Catalog struct {
	SchemaVersion   int             `json:"schema_version"`
	Providers       []Provider      `json:"providers"`
	Products        json.RawMessage `json:"products"`
	Offers          json.RawMessage `json:"offers"`
	Candidates      json.RawMessage `json:"candidates"`
	Services        json.RawMessage `json:"services"`
	ServiceListings json.RawMessage `json:"service_listings"`
	PriceUpdate     json.RawMessage `json:"price_update"` // 目录仍含采集状态；管理端只读平台字段。
}
type Referrals struct {
	SchemaVersion int               `json:"schema_version"`
	Invitations   map[string]string `json:"invitations"`
}

// strictJSON 拒绝未知字段；catalog 用 strictJSONTolerant，目录新增字段不该让管理端启动失败。
func strictJSON(data []byte, dst any) error { return decodeJSON(data, dst, true) }

func strictJSONTolerant(data []byte, dst any) error { return decodeJSON(data, dst, false) }

func decodeJSON(data []byte, dst any, strict bool) error {
	if len(data) > 32<<20 {
		return errors.New("JSON 文件过大")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				s := key.(string)
				if seen[s] {
					return errors.New("JSON 存在重复键")
				}
				seen[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = walk(); e != nil {
					return e
				}
			}
		default:
			return errors.New("JSON 结构无效")
		}
		_, e = d.Token()
		return e
	}
	if e := walk(); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("JSON 尾部有多余内容")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	if strict {
		d.DisallowUnknownFields()
	}
	return d.Decode(dst)
}
func validateURL(raw string, hosts []string) error {
	if len(raw) == 0 || len(raw) > 4096 || strings.ContainsAny(raw, "\\ ") {
		return errors.New("链接长度或字符无效，最多 4096 字节")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("链接包含空白或控制字符")
		}
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return errors.New("请填写无用户名密码的完整 HTTPS 链接")
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("不允许此端口")
	}
	if u.Host != u.Hostname() && u.Host != u.Hostname()+":443" {
		return errors.New("主机或端口格式无效")
	}
	if strings.Contains(u.Host, "%") || !slices.Contains(hosts, strings.ToLower(u.Hostname())) {
		return errors.New("域名不在该平台官方白名单")
	}
	decoded, e := url.PathUnescape(raw)
	if e != nil {
		return errors.New("链接编码无效")
	}
	for _, r := range decoded {
		if unicode.IsControl(r) {
			return errors.New("链接包含控制字符")
		}
	}
	return nil
}
func loadProviders(path string) ([]Provider, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c Catalog
	if e = strictJSONTolerant(b, &c); e != nil {
		return nil, e
	}
	if c.SchemaVersion != 1 || len(c.Providers) == 0 {
		return nil, errors.New("目录版本或平台列表无效")
	}
	seen := map[string]bool{}
	for _, p := range c.Providers {
		if p.ID == "" || seen[p.ID] || p.Name == "" || validateURL(p.URL, p.Hosts) != nil {
			return nil, errors.New("目录平台或白名单无效")
		}
		for _, h := range p.Hosts {
			if h == "" || h != strings.ToLower(h) || strings.ContainsAny(h, "/:* ") {
				return nil, errors.New("域名白名单无效")
			}
		}
		seen[p.ID] = true
	}
	return c.Providers, nil
}

type Store struct {
	mu        sync.Mutex
	path      string
	providers []Provider
}

func (s *Store) provider(id string) (Provider, bool) {
	for _, p := range s.providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
func revision(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *Store) load() (Referrals, string, error) {
	var r Referrals
	b, e := os.ReadFile(s.path)
	if e != nil {
		return r, "", e
	}
	if e = strictJSON(b, &r); e != nil {
		return r, "", e
	}
	if r.SchemaVersion != 1 || r.Invitations == nil {
		return r, "", errors.New("邀请配置结构或版本无效")
	}
	for id, raw := range r.Invitations {
		p, ok := s.provider(id)
		if !ok {
			return r, "", errors.New("邀请配置包含未知平台")
		}
		if e = validateURL(raw, p.Hosts); e != nil {
			return r, "", e
		}
	}
	return r, revision(b), nil
}
func writeAtomic(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".referrals-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}

var errConflict = errors.New("另一页面或文件已修改配置，请刷新后重试；本次输入未保存")

func (s *Store) update(id, raw, rev string, restore bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.provider(id)
	if !ok {
		return errors.New("未知平台")
	}
	if !restore {
		if e := validateURL(raw, p.Hosts); e != nil {
			return e
		}
	}
	r, current, e := s.load()
	if e != nil {
		return e
	}
	if current != rev {
		return errConflict
	}
	if restore {
		delete(r.Invitations, id)
	} else {
		r.Invitations[id] = raw
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(b, '\n'))
}
