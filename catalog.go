package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
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
	// 渠道级充值资料，展示在报价卡片右下角小字。DepositNote 是整句说明（含最低金额或“免费/待核实”）。
	DepositNote     string   `json:"deposit_note,omitempty"`
	PaymentMethods  []string `json:"payment_methods,omitempty"`
	PaymentVerified string   `json:"payment_verified,omitempty"`
}
type Product struct {
	ID         string   `json:"id"`
	Provider   string   `json:"provider"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	URL        string   `json:"url"`
	ManageURL  string   `json:"manage_url"`
	Duration   string   `json:"duration"`
	Starts     string   `json:"starts"`
	Repeat     string   `json:"repeat"`
	Reuse      string   `json:"reuse"`
	Renew      string   `json:"renew"`
	AutoRenew  string   `json:"auto_renew"`
	Restore    string   `json:"restore"`
	Online     string   `json:"online"`
	Refund     string   `json:"refund"`
	Conditions string   `json:"conditions"`
	Sources    []string `json:"sources"`
	Verified   string   `json:"verified"`
}
type Offer struct {
	ID            string  `json:"id"`
	Product       string  `json:"product"`
	Country       string  `json:"country"`
	Service       string  `json:"service"`
	NumberType    string  `json:"number_type"`
	Term          string  `json:"term"`
	Scope         string  `json:"scope"`
	Amount        *string `json:"amount"`
	RenewalAmount *string `json:"renewal_amount"`
	Currency      string  `json:"currency"`
	Unit          string  `json:"unit"`
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	Source        string  `json:"source"`
	Verified      string  `json:"verified"`
	ServiceID     string  `json:"service_id,omitempty"`
	CheckedAt     string  `json:"checked_at,omitempty"`
	Evidence      string  `json:"evidence,omitempty"`
	Method        string  `json:"method,omitempty"`
}
type Service struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}
type ServiceListing struct {
	Provider  string `json:"provider"`
	Product   string `json:"product,omitempty"`
	ServiceID string `json:"service_id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Evidence  string `json:"evidence"`
	CheckedAt string `json:"checked_at"`
	Status    string `json:"status"`
}
type UpdateFailure struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}
type PriceUpdate struct {
	Saved           bool               `json:"-"`
	AttemptedAt     string             `json:"attempted_at"`
	CompletedAt     string             `json:"completed_at"`
	NextUpdateAt    string             `json:"next_update_at"`
	UpdatedOffers   int                `json:"updated_offers"`
	UpdatedServices int                `json:"updated_services,omitempty"`
	PriceWarnings   []UpdateFailure    `json:"price_warnings,omitempty"`
	Failures        []UpdateFailure    `json:"failures"`
	Sources         []SourceCollection `json:"sources,omitempty"`
}
type SourceCollection struct {
	Provider          string            `json:"provider"`
	Name              string            `json:"name"`
	ListedTargets     int               `json:"listed_targets"`
	PricedTargets     int               `json:"priced_targets"`
	Quotes            int               `json:"quotes"`
	CountriesExpected int               `json:"countries_expected,omitempty"`
	CountriesRead     int               `json:"countries_read,omitempty"`
	QuoteAttempts     map[string]string `json:"quote_attempts,omitempty"`
}
type Candidate struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Note string `json:"note"`
}
type Catalog struct {
	SchemaVersion   int              `json:"schema_version"`
	Providers       []Provider       `json:"providers"`
	Products        []Product        `json:"products"`
	Offers          []Offer          `json:"offers"`
	Candidates      []Candidate      `json:"candidates"`
	Services        []Service        `json:"services,omitempty"`
	ServiceListings []ServiceListing `json:"service_listings,omitempty"`
	PriceUpdate     *PriceUpdate     `json:"price_update,omitempty"`
}
type Referrals struct {
	SchemaVersion int               `json:"schema_version"`
	Invitations   map[string]string `json:"invitations"`
}

// StrictJSON rejects duplicate keys before decoding, including keys inside nested objects.
func StrictJSON(data []byte, dst any) error {
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
				k, e := d.Token()
				if e != nil {
					return e
				}
				s := k.(string)
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
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
func ValidateURL(raw string, hosts []string) error {
	if len(raw) == 0 || len(raw) > 4096 || strings.ContainsAny(raw, "\\ ") {
		return errors.New("链接长度或字符无效")
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

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func amountOK(v *string) bool { return v == nil || decimal.MatchString(*v) }
func validDate(s string) bool { _, e := time.Parse("2006-01-02", s); return e == nil }
func sourceOK(s string) bool {
	u, e := url.Parse(s)
	return e == nil && ValidateURL(s, []string{strings.ToLower(u.Hostname())}) == nil
}
func LoadCatalog(path string) (Catalog, error) {
	var c Catalog
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	if e = StrictJSON(b, &c); e != nil {
		return c, e
	}
	return c, c.Validate()
}
func (c Catalog) Validate() error {
	bad := func(s string) error { return fmt.Errorf("目录校验失败：%s", s) }
	if c.SchemaVersion != 1 || len(c.Providers) == 0 {
		return bad("版本或平台为空")
	}
	providers := map[string]Provider{}
	products := map[string]Product{}
	ids := map[string]bool{}
	for _, p := range c.Providers {
		if !identifier.MatchString(p.ID) || providers[p.ID].ID != "" || p.Name == "" || !validDate(p.Verified) || len(p.Sources) == 0 || ValidateURL(p.URL, p.Hosts) != nil {
			return bad("平台 " + p.ID)
		}
		for _, h := range append(append([]string{}, p.Hosts...), p.APIHosts...) {
			if h == "" || h != strings.ToLower(h) || strings.ContainsAny(h, "/:* ") {
				return bad("域名白名单")
			}
		}
		for _, s := range p.Sources {
			if !sourceOK(s) {
				return bad("平台来源")
			}
		}
		if (p.DepositNote != "" || len(p.PaymentMethods) > 0) && !validDate(p.PaymentVerified) {
			return bad("平台充值资料 " + p.ID)
		}
		providers[p.ID] = p
	}
	states := []string{"supported", "unsupported", "conditional", "unknown", "conflicting"}
	services := map[string]Service{}
	for _, s := range c.Services {
		if !identifier.MatchString(s.ID) || s.Name == "" || len(s.Name) > 160 || services[s.ID].ID != "" {
			return bad("服务目录")
		}
		services[s.ID] = s
	}
	listingIDs := map[string]bool{}
	for _, l := range c.ServiceListings {
		v, ok := providers[l.Provider]
		key := l.Provider + "/" + l.Product + "/" + l.ServiceID
		_, stampErr := time.Parse(time.RFC3339, l.CheckedAt)
		if !ok || services[l.ServiceID].ID == "" || l.Name == "" || len(l.Name) > 160 || l.Evidence == "" || len(l.Evidence) > 1600 || stampErr != nil || listingIDs[key] || ValidateURL(l.Source, priceHosts(v)) != nil || !slices.Contains([]string{"verified", "unavailable"}, l.Status) {
			return bad("接码对象来源")
		}
		if l.Product != "" {
			found := false
			for _, p := range c.Products {
				if p.ID == l.Product && p.Provider == l.Provider {
					found = true
					break
				}
			}
			if !found {
				return bad("接码对象产品关联")
			}
		}
		listingIDs[key] = true
	}
	for _, p := range c.Products {
		provider, ok := providers[p.Provider]
		if !ok || !identifier.MatchString(p.ID) || products[p.ID].ID != "" || p.Name == "" || !slices.Contains([]string{"free", "once", "rental"}, p.Kind) || p.Duration == "" || p.Starts == "" || !validDate(p.Verified) || len(p.Sources) == 0 {
			return bad("产品 " + p.ID)
		}
		for _, v := range []string{p.Repeat, p.Reuse, p.Renew, p.AutoRenew, p.Restore, p.Online} {
			if !slices.Contains(states, v) {
				return bad("能力状态 " + p.ID)
			}
		}
		if !slices.Contains([]string{"refundable", "nonrefundable", "conditional", "unknown", "conflicting", "na"}, p.Refund) {
			return bad("退款状态")
		}
		for _, s := range []string{p.URL, p.ManageURL} {
			if s != "" && ValidateURL(s, provider.Hosts) != nil {
				return bad("产品链接")
			}
		}
		for _, s := range p.Sources {
			if !sourceOK(s) {
				return bad("产品来源")
			}
		}
		products[p.ID] = p
	}
	for _, o := range c.Offers {
		if o.ServiceID != "" {
			s, ok := services[o.ServiceID]
			if !ok || s.Name != o.Service {
				return bad("服务关联 " + o.ID)
			}
		}
		if o.CheckedAt != "" {
			if _, e := time.Parse(time.RFC3339, o.CheckedAt); e != nil {
				return bad("价格采集时间")
			}
		}
		p, ok := products[o.Product]
		if !ok || !identifier.MatchString(o.ID) || ids[o.ID] || !amountOK(o.Amount) || !amountOK(o.RenewalAmount) || !validDate(o.Verified) || !sourceOK(o.Source) || o.Unit == "" || o.Currency == "" || o.Country == "" || o.Service == "" || o.Term == "" || o.Scope == "" || o.NumberType == "" {
			return bad("报价 " + o.ID)
		}
		if !slices.Contains([]string{"exact", "starting", "unknown", "login"}, o.Kind) || !slices.Contains([]string{"verified", "conflicting", "unavailable"}, o.Status) {
			return bad("报价类型")
		}
		if (o.Kind == "exact" || o.Kind == "starting") != (o.Amount != nil) {
			return bad("报价金额和类型不一致")
		}
		if o.Kind == "exact" && p.Kind != "free" && (o.Country == "未知" || o.Service == "未知" || o.Term == "未知") {
			return bad("精确报价缺少适用条件")
		}
		ids[o.ID] = true
	}
	for _, p := range c.Products {
		found := false
		for _, o := range c.Offers {
			if o.Product == p.ID {
				found = true
				break
			}
		}
		if !found {
			return bad("产品缺少报价状态 " + p.ID)
		}
	}
	for _, v := range c.Candidates {
		if v.Name == "" || v.Note == "" || !sourceOK(v.URL) {
			return bad("候选来源")
		}
	}
	if c.PriceUpdate != nil {
		for _, v := range c.PriceUpdate.Sources {
			if _, ok := providers[v.Provider]; !ok || v.ListedTargets < 0 || v.PricedTargets < 0 || v.PricedTargets > v.ListedTargets || v.Quotes < v.PricedTargets || v.CountriesRead < 0 || v.CountriesExpected < v.CountriesRead {
				return bad("来源采集统计")
			}
		}
	}
	return nil
}
func LoadReferrals(path string, providers []Provider) (Referrals, error) {
	r := Referrals{SchemaVersion: 1, Invitations: map[string]string{}}
	if path == "" {
		return r, nil
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	if e = StrictJSON(b, &r); e != nil {
		return r, e
	}
	if r.SchemaVersion != 1 || r.Invitations == nil {
		return r, errors.New("邀请文件版本或结构无效")
	}
	for id, raw := range r.Invitations {
		var hosts []string
		for _, p := range providers {
			if p.ID == id {
				hosts = p.Hosts
				break
			}
		}
		if e = ValidateURL(raw, hosts); e != nil {
			return r, fmt.Errorf("邀请配置 %s 无效：%w", id, e)
		}
	}
	return r, nil
}
func PurchaseURL(p Product, provider Provider, r Referrals) string {
	if raw := r.Invitations[provider.ID]; raw != "" {
		return raw
	}
	if p.URL != "" {
		return p.URL
	}
	return provider.URL
}
func Group(o Offer, p Product) string {
	return strings.Join(groupFields(o, p), " / ")
}
func groupFields(o Offer, p Product) []string {
	return []string{p.Kind, o.Country, o.Service, o.NumberType, o.Term, o.Currency, o.Unit, o.Scope, o.Kind}
}
func SortOffers(offers []Offer, products map[string]Product) {
	// Parse each distinct price once; comparisons must retain exact decimal precision.
	amounts := map[string]*big.Rat{}
	for _, o := range offers {
		if o.Amount != nil && amounts[*o.Amount] == nil {
			amounts[*o.Amount], _ = new(big.Rat).SetString(*o.Amount)
		}
	}
	slices.SortStableFunc(offers, func(a, b Offer) int {
		if c := strings.Compare(products[a.Product].Kind, products[b.Product].Kind); c != 0 {
			return c
		}
		// Priced offers first (ascending), then unpriced; matches the page's price sort.
		if (a.Amount == nil) != (b.Amount == nil) {
			if a.Amount == nil {
				return 1
			}
			return -1
		}
		// Keep currencies and units separate; within one, ascending by price.
		if c := natRank(a.Currency) - natRank(b.Currency); c != 0 {
			return c
		}
		if c := slices.Compare([]string{a.Currency, a.Unit, a.Term, a.Kind, a.NumberType, a.Scope}, []string{b.Currency, b.Unit, b.Term, b.Kind, b.NumberType, b.Scope}); c != 0 {
			return c
		}
		if a.Amount != nil && b.Amount != nil {
			ar, br := amounts[*a.Amount], amounts[*b.Amount]
			if c := ar.Cmp(br); c != 0 {
				return c
			}
		}
		// Same price: keep a stable, readable grouping.
		if c := slices.Compare(groupFields(a, products[a.Product]), groupFields(b, products[b.Product])); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}
