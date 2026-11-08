package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// WebPriceSnapshot is written by tools/web-prices.mjs from a logged-in browser
// session. It only carries normalized offers; cookies and tokens stay in the
// ignored Chrome profile.
type WebPriceSnapshot struct {
	SchemaVersion int             `json:"schema_version"`
	CapturedAt    string          `json:"captured_at"`
	Offers        []WebPriceOffer `json:"offers"`
	Warnings      []string        `json:"warnings,omitempty"`
}
type WebPriceOffer struct {
	Provider   string `json:"provider"`
	Product    string `json:"product"`
	Country    string `json:"country"`
	Service    string `json:"service"`
	NumberType string `json:"number_type,omitempty"`
	Term       string `json:"term"`
	Scope      string `json:"scope,omitempty"`
	Amount     string `json:"amount"`
	Currency   string `json:"currency"`
	Unit       string `json:"unit"`
	Kind       string `json:"kind,omitempty"`
	Source     string `json:"source"`
	Evidence   string `json:"evidence,omitempty"`
}

func webProduct(id string, now time.Time) Product {
	switch id {
	case "longsms-short":
		return Product{
			ID: id, Provider: "longsms", Name: "短期验证号码", Kind: "once",
			URL:      "https://longsms.net/app/shop",
			Duration: "20 分钟（短期验证号码）", Starts: "购买后按官网规则起算",
			Repeat: "unknown", Reuse: "unknown", Renew: "unknown", AutoRenew: "unknown", Restore: "unknown", Online: "unknown", Refund: "unknown",
			Conditions: "短期号码目录与报价来自网页登录后的 shop API；有效期、可复用范围与退款规则仍待核实。",
			Sources:    []string{"https://longsms.net/app/shop"},
			Verified:   now.Format("2006-01-02"),
		}
	case "haozhuma-once":
		return Product{
			ID: id, Provider: "haozhuma", Name: "单次号码", Kind: "once",
			URL:      "https://my.haozhuma.com/",
			Duration: "未注明", Starts: "购买后按官网规则起算",
			Repeat: "unknown", Reuse: "unknown", Renew: "unknown", AutoRenew: "unknown", Restore: "unknown", Online: "unknown", Refund: "unknown",
			Conditions: "项目名称与价格来自网页登录后的项目列表；时长、复用与退款规则仍待核实。",
			Sources:    []string{"https://my.haozhuma.com/"},
			Verified:   now.Format("2006-01-02"),
		}
	}
	return Product{}
}

func loadWebPrices(path string) (WebPriceSnapshot, error) {
	var s WebPriceSnapshot
	b, e := os.ReadFile(path)
	if e != nil {
		return s, e
	}
	if len(b) > 8<<20 || json.Unmarshal(b, &s) != nil || s.SchemaVersion != 1 || len(s.Offers) > 20000 {
		return s, errors.New("网页报价快照格式或版本无效")
	}
	if _, e = time.Parse(time.RFC3339, s.CapturedAt); e != nil {
		return s, errors.New("网页报价快照时间无效")
	}
	return s, nil
}

func (d *directPrices) onceWeb(service Service, country, numberType, term, scope, amount, currency, unit, kind, url, method, evidence string) {
	if d.product.Kind != "once" || !decimal.MatchString(amount) || currency == "" || term == "" || scope == "" || unit == "" || len(evidence) > 1600 || messageEvidence(evidence) {
		return
	}
	if kind != "starting" {
		kind = "exact"
	}
	if kind == "exact" && country == "" {
		return
	}
	if country == "" {
		country = "未知"
	}
	country = normalizedCountry(country)
	key, _ := json.Marshal([]string{d.source.Provider, d.product.ID, service.ID, country, numberType, term, scope, currency, unit, kind})
	sum := sha256.Sum256(key)
	id := "web-" + hex.EncodeToString(sum[:12])
	d.offers[id] = Offer{ID: id, Product: d.product.ID, ServiceID: service.ID, Service: service.Name, Country: country, Amount: &amount, Currency: currency, Unit: unit, NumberType: numberType, Scope: scope, Term: term, Kind: kind, Status: "verified", Source: url, Verified: d.now.Format("2006-01-02"), CheckedAt: d.now.Format(time.RFC3339), Evidence: evidence, Method: method}
}

// mergeWebPrices adds a successful snapshot to the catalog. Old web offers for
// the same product are replaced; a snapshot with no valid offers is a no-op.
func mergeWebPrices(c *Catalog, s WebPriceSnapshot, now time.Time) (int, int, []string, error) {
	captured, e := time.Parse(time.RFC3339, s.CapturedAt)
	if e != nil {
		return 0, 0, nil, errors.New("网页报价快照时间无效")
	}
	if now.Sub(captured) > 48*time.Hour {
		return 0, 0, []string{"网页报价快照超过 48 小时，未合并"}, nil
	}
	providers := map[string]Provider{}
	for _, p := range c.Providers {
		providers[p.ID] = p
	}
	products := map[string]Product{}
	for _, p := range c.Products {
		products[p.ID] = p
	}
	created := map[string]bool{}
	direct := map[string]*directPrices{}
	warnings := append([]string{}, s.Warnings...)
	for _, w := range s.Offers {
		p, productExists := products[w.Product]
		if !productExists {
			p = webProduct(w.Product, captured)
			if p.ID == "" {
				warnings = append(warnings, "未知网页产品："+w.Product)
				continue
			}
		}
		provider, providerOK := providers[w.Provider]
		if !providerOK || p.Provider != w.Provider {
			warnings = append(warnings, "网页报价平台/产品不匹配："+w.Provider+"/"+w.Product)
			continue
		}
		if ValidateURL(w.Source, priceHosts(provider)) != nil {
			warnings = append(warnings, "网页报价来源不在官方域名："+w.Source)
			continue
		}
		if w.Country == "" || w.Service == "" || w.Term == "" || w.Unit == "" || !decimal.MatchString(w.Amount) || !regexpCurrency(w.Currency) {
			warnings = append(warnings, "网页报价字段无效："+w.Provider+"/"+w.Service)
			continue
		}
		if !productExists {
			c.Products = append(c.Products, p)
			products[w.Product] = p
			created[w.Product] = true
		}
		d := direct[w.Product]
		if d == nil {
			d = newDirect(c, PriceSource{Provider: w.Provider, Product: w.Product, URL: p.URL}, p, captured)
			direct[w.Product] = d
		}
		numberType := w.NumberType
		if numberType == "" {
			numberType = "未注明"
		}
		scope := w.Scope
		if scope == "" {
			scope = "按官网套餐"
		}
		evidence := w.Evidence
		if evidence == "" {
			evidence = compactEvidence(w)
		}
		serviceName := truncateUTF8(w.Service, 160)
		service, e := d.target(serviceName, w.Source, compactEvidence(map[string]string{"service": serviceName, "source": w.Source}))
		if e != nil {
			warnings = append(warnings, "网页服务名称无效："+w.Service)
			continue
		}
		d.onceWeb(service, w.Country, numberType, w.Term, scope, w.Amount, w.Currency, w.Unit, w.Kind, w.Source, "web_api", evidence)
	}
	offers, listings := 0, 0
	for id, d := range direct {
		l, o := d.values()
		if len(o) == 0 {
			warnings = append(warnings, "网页产品没有有效报价："+id)
			continue
		}
		p := products[id]
		c.Offers = slices.DeleteFunc(c.Offers, func(v Offer) bool { return v.Product == p.ID && strings.HasPrefix(v.ID, "web-") })
		c.ServiceListings = slices.DeleteFunc(c.ServiceListings, func(v ServiceListing) bool { return v.Provider == p.Provider && v.Product == p.ID })
		c.Offers = append(c.Offers, o...)
		c.ServiceListings = append(c.ServiceListings, l...)
		offers += len(o)
		listings += len(l)
	}
	for id := range created {
		if slices.ContainsFunc(c.Offers, func(o Offer) bool { return o.Product == id }) {
			continue
		}
		c.Products = slices.DeleteFunc(c.Products, func(p Product) bool { return p.ID == id })
		c.ServiceListings = slices.DeleteFunc(c.ServiceListings, func(l ServiceListing) bool { return l.Product == id })
	}
	if offers == 0 {
		return 0, 0, warnings, nil
	}
	if e := c.Validate(); e != nil {
		return 0, 0, warnings, fmt.Errorf("网页报价合并后目录校验失败：%w", e)
	}
	return offers, listings, warnings, nil
}

func regexpCurrency(v string) bool {
	if len(v) != 3 {
		return false
	}
	for _, r := range v {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for i := range s {
		if i >= max {
			return s[:i]
		}
	}
	return s[:max]
}
