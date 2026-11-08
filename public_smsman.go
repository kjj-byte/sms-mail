package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// SMS-Man public read-only price collection.
//
// The account API (https://api.sms-man.com/control/*) normally needs a paid
// token, but the public website itself calls the same endpoints with a
// publishable site key embedded in its page bootstrap. That key exposes the
// public catalogue and per-country prices (currency=USD) without login or the
// account-balance gate. Read-only; never buys numbers.
//
//	GET /api/services?api_key=KEY&l=en        -> [{id,title,code}]
//	GET /api/countries?api_key=KEY&l=en       -> {id:{id,title,code}}
//	GET /control/get-prices?token=KEY&country_id=ID&currency=USD
//	     -> {appID:{cost,count,application_id,country_id,country,application}}
const smsmanBase = "https://api.sms-man.com"

// Public publishable site key from sms-man.com page bootstrap (not an account
// token, and not a model key). Exposes the public catalogue and per-country
// prices read-only; it is published on the public website.
const smsmanPublicKey = "ksYFPry6RvHBDMnKUjBFfzTPZbPJsADv2TGxbTny"

func smsmanGet(ctx context.Context, client *http.Client, p Provider, path string, dst any) error {
	u := smsmanBase + path
	if ValidateURL(u, priceHosts(p)) != nil {
		return errors.New("SMS-Man 接口域名未登记")
	}
	r, e := publicRequest(ctx, client, p, http.MethodGet, u, nil, "")
	if e != nil {
		return e
	}
	defer r.Body.Close()
	b, e := boundedBody(r.Body, 16<<20)
	if e != nil {
		return e
	}
	var check any
	if StrictJSON(b, &check) != nil {
		return errors.New("SMS-Man 接口不是完整 JSON")
	}
	if json.Unmarshal(b, dst) != nil {
		return errors.New("SMS-Man 接口结构变化")
	}
	return nil
}

func collectSMSMan(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	var services []struct {
		ID    json.Number `json:"id"`
		Title string      `json:"title"`
		Code  string      `json:"code"`
	}
	if e := smsmanGet(ctx, client, p, "/api/services?api_key="+smsmanPublicKey+"&l=en", &services); e != nil {
		return e
	}
	if len(services) == 0 || len(services) > 9000 {
		return errors.New("SMS-Man 服务目录为空或异常")
	}
	var countriesMap map[string]struct {
		ID    json.Number `json:"id"`
		Title string      `json:"title"`
		Code  string      `json:"code"`
	}
	if e := smsmanGet(ctx, client, p, "/api/countries?api_key="+smsmanPublicKey+"&l=en", &countriesMap); e != nil {
		return e
	}
	if len(countriesMap) == 0 || len(countriesMap) > 500 {
		return errors.New("SMS-Man 国家目录为空或异常")
	}

	ids := map[string]Service{} // application id -> service
	for _, v := range services {
		name := strings.TrimSpace(v.Title)
		if name == "" || string(v.ID) == "" {
			continue
		}
		s, e := d.target(name, smsmanBase+"/api/services", compactEvidence(map[string]string{"id": string(v.ID), "name": name, "code": v.Code}))
		if e == nil {
			ids[string(v.ID)] = s
		}
	}

	type country struct{ id, name string }
	var list []country
	for _, v := range countriesMap {
		name := strings.TrimSpace(v.Title)
		if name == "" || string(v.ID) == "" || string(v.ID) == "0" {
			continue
		}
		// Prefer the ISO code for a stable Chinese name; fall back to the display
		// title (also normalized, since titles can be "United States/USA").
		label := normalizedCountry(v.Code)
		if label == "" || label == v.Code {
			if t := normalizedCountry(name); t != "" {
				label = t
			}
		}
		list = append(list, country{string(v.ID), label})
	}
	d.CountriesExpected = len(list)
	fmt.Printf("  SMS-Man：读取 %d 个国家的公开价格（最多 3 个并发）\n", len(list))

	type row struct {
		Cost      json.RawMessage `json:"cost"`
		Count     json.Number     `json:"count"`
		AppID     string          `json:"application_id"`
		App       string          `json:"application"`
		CountryID string          `json:"country_id"`
		Country   string          `json:"country"`
	}
	codes := make([]string, len(list))
	for i, c := range list {
		codes[i] = c.id
	}
	results := readItems(ctx, codes, func(code string) (map[string]row, error) {
		var data map[string]row
		path := "/control/get-prices?token=" + smsmanPublicKey + "&currency=USD&country_id=" + code
		if e := smsmanGet(ctx, client, p, path, &data); e != nil {
			return nil, e
		}
		for appID, r := range data {
			if r.CountryID != code || r.AppID != appID {
				return nil, errors.New("SMS-Man 返回国家或应用 ID 与请求不符，未发布该国家价格")
			}
		}
		return data, nil
	})

	completed := 0
	var failures []string
	priceURL := smsmanBase + "/control/get-prices"
	for i, result := range results {
		if result.Err != nil {
			failures = append(failures, list[i].id+"："+result.Err.Error())
			continue
		}
		completed++
		cname := list[i].name
		for appID, r := range result.Value {
			s, ok := ids[appID]
			if !ok {
				continue
			}
			stock, se := r.Count.Int64()
			if se != nil || stock <= 0 {
				continue
			}
			amount, ae := decimalRaw(r.Cost)
			if ae != nil {
				continue
			}
			ev := compactEvidence(map[string]any{"service": s.Name, "application_id": r.AppID, "application": r.App, "country": r.Country, "country_id": r.CountryID, "cost": r.Cost, "count": r.Count, "currency": "USD"})
			d.minimum(s, "未知", amount, "USD", priceURL, "public_api", ev)
			if detailedTarget(s, focus) {
				d.minimum(s, cname, amount, "USD", priceURL, "public_api", ev)
			}
		}
	}
	fmt.Printf("  SMS-Man：完整读取 %d/%d 个国家\n", completed, len(list))
	d.CountriesRead = completed
	if completed == 0 {
		return errors.New("SMS-Man 所有国家价格均未取得")
	}
	if len(failures) > 0 {
		if len(failures) > 3 {
			failures = failures[:3]
		}
		d.warnings = append(d.warnings, fmt.Sprintf("SMS-Man 完成 %d/%d 个国家，未完成国家旧价格保留并标记；%s", completed, len(list), strings.Join(failures, "；")))
	}
	return nil
}
