package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SV-Number (sms-verification-number.com) read-only price collection.
// API is sms-activate compatible but requires per-country calls:
//
//	getCountryAndOperators -> [{id,name}]
//	getServicesAndCost&country=ID -> [{id,name,price,quantity}]
//
// lang=en => USD. Read-only; no order/number calls.
const svNumberBase = "https://sms-verification-number.com/stubs/handler_api"

func svNumberGet(ctx context.Context, client *http.Client, p Provider, key, params string, dst any) error {
	q, err := url.ParseQuery(params)
	if err != nil || len(q["action"]) != 1 || (q.Get("action") != "getCountryAndOperators" && q.Get("action") != "getServicesAndCost") {
		return errors.New("SV-Number 仅允许目录和价格只读方法")
	}
	for name, values := range q {
		if len(values) != 1 || (name != "action" && name != "country") {
			return errors.New("SV-Number 请求参数无效")
		}
	}
	if q.Get("action") == "getServicesAndCost" {
		if _, err := strconv.ParseUint(q.Get("country"), 10, 64); err != nil {
			return errors.New("SV-Number 国家编号无效")
		}
	}
	q.Set("api_key", key)
	q.Set("lang", "en")
	u := svNumberBase + "?" + q.Encode()
	if ValidateURL(u, priceHosts(p)) != nil {
		return errors.New("SV-Number 接口域名未登记")
	}
	var body []byte
	var lastErr error
	safe := *client
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return errors.New("SV-Number 请求超时")
			case <-time.After(2 * time.Second):
			}
		}
		reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "identity-PriceReader/1.0")
		r, e := safe.Do(req)
		if e != nil {
			cancel()
			lastErr = errors.New("SV-Number 请求失败或超时")
			continue
		}
		if r.StatusCode != http.StatusOK {
			r.Body.Close()
			cancel()
			return fmt.Errorf("SV-Number 接口 HTTP %d（未跟随重定向）", r.StatusCode)
		}
		b, be := boundedBody(r.Body, 4<<20)
		r.Body.Close()
		cancel()
		if be != nil {
			lastErr = be
			continue
		}
		body, lastErr = b, nil
		break
	}
	if lastErr != nil {
		return lastErr
	}
	var check any
	if StrictJSON(body, &check) != nil {
		return errors.New("SV-Number 响应不是完整有效 JSON")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return errors.New("SV-Number 响应结构变化")
	}
	return nil
}

func collectSVNumber(ctx context.Context, client *http.Client, p Provider, d *directPrices, key string, focus []string) error {
	var countries []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	}
	if e := svNumberGet(ctx, client, p, key, "action=getCountryAndOperators", &countries); e != nil {
		return e
	}
	if len(countries) == 0 {
		return errors.New("SV-Number 国家目录为空")
	}
	d.CountriesExpected = len(countries)
	read := 0
	priced := 0
	for _, c := range countries {
		cid := string(c.ID)
		if cid == "" {
			continue
		}
		var rows []struct {
			ID       string          `json:"id"`
			Name     string          `json:"name"`
			Price    json.RawMessage `json:"price"`
			Quantity json.Number     `json:"quantity"`
		}
		if e := svNumberGet(ctx, client, p, key, "action=getServicesAndCost&country="+cid, &rows); e != nil {
			continue
		}
		read++
		for _, row := range rows {
			stock, se := row.Quantity.Int64()
			if se != nil || stock <= 0 || row.ID == "" || row.Name == "" {
				continue
			}
			amount, ae := decimalRaw(row.Price)
			if ae != nil {
				continue
			}
			s, e := d.target(row.Name, svNumberBase, compactEvidence(map[string]any{"code": row.ID, "name": row.Name, "country_id": cid}))
			if e != nil {
				continue
			}
			// Always record the cheapest-across-countries "starting" price (country 未知) so the
			// catalog stays small; only focus targets get per-country rows for exact comparison.
			evStart := compactEvidence(map[string]any{"service": row.Name, "code": row.ID, "cost": row.Price, "count": stock})
			d.minimum(s, "未知", amount, "USD", svNumberBase, "authenticated_api", evStart)
			priced++
			if detailedTarget(s, focus) {
				ev := compactEvidence(map[string]any{"service": row.Name, "code": row.ID, "country": c.Name, "cost": row.Price, "count": stock})
				d.minimum(s, c.Name, amount, "USD", svNumberBase, "authenticated_api", ev)
			}
		}
	}
	d.CountriesRead = read
	if read < len(countries) {
		d.warnings = append(d.warnings, fmt.Sprintf("SV-Number 完成 %d/%d 个国家，未完成国家的旧价格保留并标记", read, len(countries)))
	}
	if priced == 0 {
		return fmt.Errorf("SV-Number 目录可用，但价格接口无可用库存（余额可能不足）")
	}
	return nil
}
