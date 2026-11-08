package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func collectSMS4Sats(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	endpoint := "https://api.sms4sats.com/v2/receive-sms/catalog"
	var catalog struct {
		Type     string                        `json:"type"`
		Services []struct{ Code, Name string } `json:"services"`
	}
	if e := publicJSON(ctx, client, p, endpoint, &catalog); e != nil {
		return e
	}
	if catalog.Type != "receive-sms" || len(catalog.Services) == 0 {
		return errors.New("SMS4Sats v2 接码目录无效")
	}
	type target struct {
		service Service
		name    string
	}
	targets := map[string]target{}
	var codes []string
	for _, v := range catalog.Services {
		v.Name = strings.TrimSpace(v.Name)
		s, e := d.target(v.Name, endpoint, compactEvidence(v))
		if e != nil || v.Code == "" {
			continue
		}
		if old, ok := targets[v.Code]; ok {
			if old.name != v.Name {
				return errors.New("SMS4Sats 对象代码存在冲突")
			}
			continue
		}
		targets[v.Code] = target{s, v.Name}
		codes = append(codes, v.Code)
	}
	sort.SliceStable(codes, func(i, j int) bool {
		a, b := detailedTarget(targets[codes[i]].service, focus), detailedTarget(targets[codes[j]].service, focus)
		if a != b {
			return a
		}
		x, _ := time.Parse(time.RFC3339, d.QuoteAttempts[codes[i]])
		y, _ := time.Parse(time.RFC3339, d.QuoteAttempts[codes[j]])
		return x.Before(y)
	})
	fmt.Printf("  SMS4Sats：询价全部 %d 个目录对象，重点对象优先（最多 3 个并发）\n", len(codes))
	quoteURL := "https://api.sms4sats.com/v2/receive-sms/quote"
	results := readItems(ctx, codes, func(code string) ([]byte, error) {
		requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		// A quote is read-only: it creates neither an invoice nor a number reservation.
		payload, _ := json.Marshal(map[string]string{"country": "auto", "service": code})
		r, e := publicRequest(requestCtx, client, p, http.MethodPost, quoteURL, bytes.NewReader(payload), "application/json")
		if e != nil {
			return nil, e
		}
		body, e := boundedBody(r.Body, 64<<10)
		r.Body.Close()
		return body, e
	})
	completed, unavailable, failed := 0, 0, 0
	for _, result := range results {
		if result.Started {
			d.QuoteAttempts[result.Code] = d.now.Format(time.RFC3339)
		}
		if result.Err != nil {
			if strings.Contains(result.Err.Error(), "HTTP 409") {
				unavailable++
			} else {
				failed++
			}
			continue
		}
		body := result.Value
		v := targets[result.Code]
		var quote struct {
			Type      string          `json:"type"`
			Service   string          `json:"service"`
			Country   string          `json:"country"`
			Price     json.RawMessage `json:"priceSats"`
			Available json.Number     `json:"availability"`
		}
		// Additional documented response fields are allowed, duplicate keys are not.
		var raw any
		if StrictJSON(body, &raw) != nil || json.Unmarshal(body, &quote) != nil || quote.Type != "receive-sms" || quote.Service != result.Code {
			failed++
			continue
		}
		amount, e := decimalRaw(quote.Price)
		if e != nil || strings.Contains(amount, ".") {
			failed++
			continue
		}
		stock, se := quote.Available.Int64()
		if se != nil || stock < 0 {
			failed++
			continue
		}
		if stock == 0 {
			unavailable++
			continue
		}
		completed++
		d.minimum(v.service, "未知", amount, "sats", quoteURL, "public_api", compactEvidence(map[string]any{"service": v.name, "code": quote.Service, "country_id": quote.Country, "availability": quote.Available, "priceSats": quote.Price, "country": "报价路线未分配国家"}))
	}
	fmt.Printf("  SMS4Sats：%d 个对象有价，%d 个暂无库存，%d 个未完成\n", completed, unavailable, failed)
	if failed > 0 {
		d.warnings = append(d.warnings, fmt.Sprintf("SMS4Sats 询价 %d/%d 个对象完成（含 %d 个暂无库存），%d 个因超时、限流或响应无效未完成，历史报价保留并标记", completed+unavailable, len(codes), unavailable, failed))
	}
	return nil
}
