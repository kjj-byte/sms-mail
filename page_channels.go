package identity

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func collectHeroState(raw []byte, d *directPrices) error {
	m := nuxtData.FindSubmatch(raw)
	if len(m) < 2 {
		return errors.New("HeroSMS 页面缺少公开 Nuxt 状态")
	}
	var pool []json.RawMessage
	if json.Unmarshal(m[1], &pool) != nil || len(pool) > 50000 {
		return errors.New("HeroSMS 状态结构变化")
	}
	value := func(i int) json.RawMessage {
		if i < 0 || i >= len(pool) {
			return nil
		}
		return pool[i]
	}
	for _, item := range pool {
		var refs map[string]int
		if json.Unmarshal(item, &refs) != nil {
			continue
		}
		var name, code string
		if _, ok := refs["directoryName"]; ok {
			json.Unmarshal(value(refs["directoryName"]), &name)
			json.Unmarshal(value(refs["service"]), &code)
			if code == "full" {
				continue
			}
		} else if _, ok := refs["short_name"]; ok {
			var image string
			json.Unmarshal(value(refs["img"]), &image)
			if !strings.Contains(image, "/service/") && !strings.Contains(image, "/services/") {
				continue
			}
			json.Unmarshal(value(refs["short_name"]), &name)
		} else {
			continue
		}
		if name == "" {
			continue
		}
		evidence := map[string]any{"service": code, "name": name}
		s, e := d.target(name, d.source.URL, compactEvidence(evidence))
		if e != nil {
			continue
		}
		pi, pok := refs["priceMinAvailable"]
		ci, cok := refs["count"]
		if !pok || !cok {
			continue
		}
		var count json.Number
		if json.Unmarshal(value(ci), &count) != nil {
			continue
		}
		stock, se := strconv.ParseInt(string(count), 10, 64)
		amount, ae := decimalRaw(value(pi))
		if se != nil || stock <= 0 || ae != nil {
			continue
		}
		evidence["priceMinAvailable"] = value(pi)
		evidence["count"] = count
		d.minimum(s, "未知", amount, "USD", d.source.URL, "page_data", compactEvidence(evidence))
	}
	return nil
}

var grizzlyService = regexp.MustCompile(`(?is)<span\b[^>]*class=["']--service["'][^>]*>(.*?)</span>`)
var grizzlyCountry = regexp.MustCompile(`(?is)<span\b[^>]*class=["']--country["'][^>]*>(.*?)</span>`)
var grizzlyPrice = regexp.MustCompile(`data-info-price-country-service=["']([A-Za-z0-9]+)_([0-9]+)_([0-9]+(?:\.[0-9]+)?)["']`)
var dollarPrice = regexp.MustCompile(`\$\s*([0-9]+(?:\.[0-9]+)?)`)

func pageText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(s, " "))), " ")
}
func collectGrizzlyCards(raw []byte, d *directPrices) {
	for _, card := range strings.Split(string(raw), `class="service-card"`)[1:] {
		if i := strings.Index(card, "</a>"); i >= 0 {
			card = card[:i+4]
		} else {
			continue
		}
		n, c, p := grizzlyService.FindStringSubmatch(card), grizzlyCountry.FindStringSubmatch(card), grizzlyPrice.FindStringSubmatch(card)
		if len(n) != 2 || len(c) != 2 || len(p) != 4 {
			continue
		}
		amount := dollarPrice.FindStringSubmatch(pageText(card))
		if len(amount) != 2 {
			continue
		}
		a, _ := new(big.Rat).SetString(amount[1])
		b, _ := new(big.Rat).SetString(p[3])
		if a.Cmp(b) != 0 {
			continue
		}
		name, country := pageText(n[1]), pageText(c[1])
		ev := compactEvidence(map[string]string{"service": name, "country": country, "price": p[3], "service_code": p[1], "country_id": p[2], "currency": "USD"})
		s, e := d.target(name, d.source.URL, ev)
		if e == nil {
			d.minimum(s, country, p[3], "USD", d.source.URL, "page_text", ev)
		}
	}
}

var smsVerificationPrice = regexp.MustCompile(`(?i)SMS Verification\s+\$([0-9]+(?:\.[0-9]+)?)\s+per verification\b`)
var targetHeading = regexp.MustCompile(`(?is)<h2[^>]*>([^<]+)</h2>`)

func collectTextverifiedPages(ctx context.Context, client *http.Client, p Provider, raw []byte, d *directPrices, focus []string) {
	seen := map[string]bool{}
	// shortcut: fetch detail pages only for price_targets, expand after upstream batching is available.
	for _, m := range serviceAnchor.FindAllSubmatch(raw, -1) {
		name, endpoint := pageText(string(m[2])), html.UnescapeString(string(m[1]))
		s, ok := d.names[targetNameKey(name)]
		if !ok || !detailedTarget(s, focus) || seen[endpoint] {
			continue
		}
		seen[endpoint] = true
		u, e := url.Parse(endpoint)
		if e != nil || u.Host != "app.textverified.com" || !strings.HasPrefix(u.Path, "/services/") {
			continue
		}
		src := d.source
		src.URL = endpoint
		body, ct, e := fetchPage(ctx, client, src, p)
		if e != nil {
			d.warnings = append(d.warnings, "Textverified 对象价格页："+e.Error())
			continue
		}
		headings := targetHeading.FindAllSubmatch(body, -1)
		matches := false
		for _, h := range headings {
			if targetNameKey(pageText(string(h[1]))) == targetNameKey(name) {
				matches = true
				break
			}
		}
		if !matches {
			d.warnings = append(d.warnings, "Textverified 对象页名称未匹配")
			continue
		}
		text, e := sourceText(body, ct)
		if e != nil {
			d.warnings = append(d.warnings, e.Error())
			continue
		}
		match := smsVerificationPrice.FindStringSubmatchIndex(text)
		if match == nil {
			continue
		}
		end := match[1] + 700
		if end > len(text) {
			end = len(text)
		}
		fragment := text[match[0]:end]
		country := "未知"
		if strings.Contains(fragment, "US-based non-VoIP") {
			country = "US"
		}
		evidence := compactEvidence(map[string]string{"service": name, "sms_verification": text[match[0]:match[1]], "country_basis": country})
		d.minimum(s, country, text[match[2]:match[3]], "USD", endpoint, "page_text", evidence)
	}
}

func pageFailureReason(provider string) string {
	switch provider {
	case "sms-man", "sv-number", "tiger-sms", "smsfast", "grizzlysms", "smspva":
		return "页面未包含可解析的对象报价；官方价格 API 需该平台独立密钥"
	case "goinsms", "sms24", "quackr", "receive-smss", "smsonline", "onlinesmsbox":
		return "公开页面是共享号码/短信列表，未提供可验证的接码对象目录或付费对象报价"
	case "haozhuma":
		return "公开首页未含对象与价格，目录/报价需登录或平台认证接口"
	default:
		return "页面未含可解析的接码对象目录或报价，可能为动态页面或来源地址失效"
	}
}
