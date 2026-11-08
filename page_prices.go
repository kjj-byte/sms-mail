package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var nuxtData = regexp.MustCompile(`(?is)<script\b[^>]*\bid=["']__NUXT_DATA__["'][^>]*>(.*?)</script>`)
var serviceAnchor = regexp.MustCompile(`(?is)<a\b[^>]*href=["'](https://app\.textverified\.com/services/[^"']+)["'][^>]*>(.*?)</a>`)
var sitewidePrice = regexp.MustCompile(`(?i)\bfrom\s*\$?\s*(0\.[0-9]{2}|[1-9][0-9]*\.[0-9]{2})\b`)

func collectPagePrices(raw []byte, contentType string, d *directPrices) error {
	switch d.source.Provider {
	case "sms-bus":
		if e := collectSMSBusState(raw, d); e != nil {
			d.warnings = append(d.warnings, e.Error())
		}
	case "textverified":
		for _, m := range serviceAnchor.FindAllSubmatch(raw, -1) {
			name := strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(string(m[2]), " ")))
			if name != "" {
				d.target(name, d.source.URL, compactEvidence(map[string]string{"service": name, "service_page": string(m[1])}))
			}
		}
	case "herosms":
		if e := collectHeroState(raw, d); e != nil {
			d.warnings = append(d.warnings, e.Error())
		}
	case "grizzlysms":
		collectGrizzlyCards(raw, d)
	}
	text, e := sourceText(raw, contentType)
	if e != nil {
		return e
	}
	if d.source.Provider == "smsfast" {
		if m := sitewidePrice.FindStringSubmatch(text); len(m) == 2 {
			d.sitewide(m[1], d.source.Currency, d.source.URL, m[0])
		}
	}
	// Only a target immediately followed by an explicit from/起价 amount is a page-price match.
	localNames := map[string][]string{}
	for _, l := range d.catalog.ServiceListings {
		if l.Provider == d.source.Provider && !messageEvidence(l.Evidence) {
			localNames[l.ServiceID] = append(localNames[l.ServiceID], l.Name)
		}
	}
	for _, l := range d.listings {
		localNames[l.ServiceID] = append(localNames[l.ServiceID], l.Name)
	}
	for _, o := range d.catalog.Offers {
		if o.Product == d.source.Product && o.ServiceID != "" {
			localNames[o.ServiceID] = append(localNames[o.ServiceID], o.Service)
		}
	}
	priced := map[string]bool{}
	for _, o := range d.offers {
		priced[o.ServiceID] = true
	}
	for _, s := range d.catalog.Services {
		if priced[s.ID] || len(localNames[s.ID]) == 0 {
			continue
		}
		for _, name := range localNames[s.ID] {
			if slices.Contains([]string{"sms", "otp", "number", "numbers", "service", "services", "other", "anyother"}, strings.ToLower(name)) {
				continue
			}
			if name == "" || !strings.Contains(strings.ToLower(text), strings.ToLower(name)) {
				continue
			}
			pattern := `(?i)(?:^|[^\pL\pN_])(` + regexp.QuoteMeta(name) + `\s+(?:[0-9][0-9 ,.'\x{00A0}]*\s+(?:pcs|numbers|шт\.?)\s+)?(?:from|starting at|起价|低至)\s*(?:\$|USD\s*)([0-9]+(?:\.[0-9]+)?))`
			m := regexp.MustCompile(pattern).FindStringSubmatch(text)
			if len(m) < 3 || messageEvidence(m[1]) {
				continue
			}
			currency := d.source.Currency
			if currency == "" {
				if strings.Contains(strings.ToUpper(m[1]), "USD") {
					currency = "USD"
				} else {
					currency = "$"
				}
			}
			service, e := d.target(name, d.source.URL, m[1])
			if e == nil {
				d.minimum(service, "未知", m[2], currency, d.source.URL, "page_text", m[1])
			}
		}
	}
	return nil
}

func collectSMSBusState(raw []byte, d *directPrices) error {
	match := nuxtData.FindSubmatch(raw)
	if len(match) < 2 {
		return errors.New("SMS-BUS 页面未含可解析的公开价格状态")
	}
	var pool []json.RawMessage
	if json.Unmarshal(match[1], &pool) != nil || len(pool) > 50000 {
		return errors.New("SMS-BUS 页面价格状态格式变化")
	}
	value := func(index int) json.RawMessage {
		if index < 0 || index >= len(pool) {
			return nil
		}
		return pool[index]
	}
	for _, item := range pool {
		var refs map[string]int
		if json.Unmarshal(item, &refs) != nil {
			continue
		}
		title, hasName := refs["title"]
		_, hasCode := refs["code"]
		_, hasKeywords := refs["keywords"]
		if !hasName || !hasCode || !hasKeywords {
			continue
		}
		var name, code string
		if json.Unmarshal(value(title), &name) != nil || json.Unmarshal(value(refs["code"]), &code) != nil {
			continue
		}
		evidence := map[string]any{"title": name, "code": code}
		costIndex, hasPrice := refs["cost"]
		if hasPrice {
			evidence["cost"] = value(costIndex)
		}
		listing, e := d.target(name, d.source.URL, compactEvidence(evidence))
		if e != nil || !hasPrice {
			continue
		}
		amount, e := decimalRaw(value(costIndex))
		if e != nil {
			continue
		}
		if idx, ok := refs["total_count"]; ok {
			var count json.Number
			if json.Unmarshal(value(idx), &count) != nil {
				continue
			}
			n, e := strconv.ParseInt(string(count), 10, 64)
			if e != nil || n <= 0 {
				continue
			}
			evidence["total_count"] = count
		}
		currency := d.source.Currency
		if currency == "" {
			currency = "$"
		}
		d.minimum(listing, "未知", amount, currency, d.source.URL, "page_data", compactEvidence(evidence))
	}
	return nil
}

func collectPriceSource(ctx context.Context, client *http.Client, p Provider, d *directPrices, cfg PriceConfig) ([]ServiceListing, []Offer, []string, error) {
	handled, apiErr := false, error(nil)
	if p.ID == "sv-number" && cfg.Credentials[p.ID].APIKey != "" {
		// SV-Number is sms-activate compatible but needs an explicit per-country
		// getServicesAndCost loop, so it has its own collector.
		handled = true
		apiErr = collectSVNumber(ctx, client, p, d, cfg.Credentials[p.ID].APIKey, cfg.PriceTargets)
	} else {
		handled, apiErr = collectAuthenticatedPrices(ctx, client, p, d, cfg)
	}
	if !handled {
		handled, apiErr = collectPublicAPI(ctx, client, p, d, cfg.PriceTargets)
	}
	if handled {
		if apiErr != nil {
			d.warnings = append(d.warnings, apiErr.Error()+"；改抓价格页")
			d.offers = map[string]Offer{}
		} else {
			if cfg.Credentials[p.ID].APIKey != "" {
				fmt.Println("  读取方式：官方认证价格 API")
			} else {
				fmt.Println("  读取方式：官方公开价格 API")
			}
		}
	}
	if !handled || apiErr != nil || len(d.offers) == 0 {
		raw, contentType, pageErr := fetchPage(ctx, client, d.source, p)
		if pageErr != nil {
			d.warnings = append(d.warnings, pageErr.Error())
		} else {
			if e := collectPagePrices(raw, contentType, d); e != nil {
				d.warnings = append(d.warnings, e.Error())
			}
			if p.ID == "textverified" {
				collectTextverifiedPages(ctx, client, p, raw, d, cfg.PriceTargets)
			}
		}
	}
	l, o := d.values()
	if len(l) == 0 && len(o) == 0 {
		reason := strings.Join(d.warnings, "；")
		if reason == "" {
			reason = pageFailureReason(p.ID)
		}
		return l, o, d.warnings, fmt.Errorf("未取得有依据的目录或价格：%s", reason)
	}
	if len(o) == 0 {
		d.warnings = append(d.warnings, "目录已拉取，但 API 或页面未提供可核验价格")
	}
	return l, o, d.warnings, nil
}
