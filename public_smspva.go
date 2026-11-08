package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func pageJSONField(raw []byte, field string, dst any) error {
	match := regexp.MustCompile(`\b` + regexp.QuoteMeta(field) + `\s*:\s*`).FindIndex(raw)
	if match == nil {
		return fmt.Errorf("页面缺少 %s 元数据", field)
	}
	var value json.RawMessage
	if json.NewDecoder(bytes.NewReader(raw[match[1]:])).Decode(&value) != nil {
		return errors.New("页面元数据不是完整 JSON")
	}
	var check any
	if StrictJSON(value, &check) != nil || json.Unmarshal(value, dst) != nil {
		return errors.New("页面元数据结构无效或含重复键")
	}
	return nil
}

type smspvaCountryData struct {
	Prices map[string]map[string]json.RawMessage
	Stock  map[string]map[string]json.Number
}

func collectSMSPVA(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	raw, _, e := fetchPage(ctx, client, d.source, p)
	if e != nil {
		return e
	}
	var services []struct {
		Code string `json:"service_opt"`
		Name string `json:"service_name"`
	}
	if e := pageJSONField(raw, "allServices", &services); e != nil {
		return e
	}
	var countries []struct {
		Code string `json:"shortname"`
		Name string `json:"title_country"`
	}
	if e := pageJSONField(raw, "allCountries", &countries); e != nil {
		return e
	}
	if len(services) == 0 || len(services) > 5000 || len(countries) == 0 || len(countries) > 500 {
		return errors.New("SMSPVA 服务或国家目录数量无效")
	}
	ids := map[string]Service{}
	for _, v := range services {
		if v.Code == "" {
			continue
		}
		s, e := d.target(strings.TrimSpace(v.Name), d.source.URL, compactEvidence(v))
		if e == nil {
			ids[v.Code] = s
		}
	}
	names := map[string]string{}
	for _, v := range countries {
		if v.Code != "" && strings.TrimSpace(v.Name) != "" {
			names[v.Code] = strings.TrimSpace(v.Name)
		}
	}
	codes := sortedKeys(names)
	d.CountriesExpected = len(codes)
	pricesURL := "https://smspva.com/sms_api/orders.php?n=7-3"
	fmt.Printf("  SMSPVA：读取 %d 个国家的价格和库存（最多 3 个并发）\n", len(codes))
	results := readItems(ctx, codes, func(code string) (smspvaCountryData, error) {
		var result smspvaCountryData
		if e := smspvaGuestData(ctx, client, p, pricesURL, "GET_PRICES_BY_COUNTRY", code, &result.Prices); e != nil {
			return result, e
		}
		var inventory struct {
			Amount map[string]map[string]json.Number `json:"amount"`
		}
		if e := smspvaGuestData(ctx, client, p, "https://smspva.com/sms_api/orders.php?n=6-3", "GET_AMOUNT_BY_COUNTRY", code, &inventory); e != nil {
			return result, e
		}
		result.Stock = inventory.Amount
		return result, nil
	})
	completed := 0
	var failures []string
	for _, result := range results {
		if result.Err != nil {
			failures = append(failures, result.Code+"："+result.Err.Error())
			continue
		}
		completed++
		for code, operators := range result.Value.Prices {
			s, ok := ids[code]
			if !ok {
				continue
			}
			for operator, value := range operators {
				stock, e := result.Value.Stock[code][operator].Int64()
				if e != nil || stock <= 0 {
					continue
				}
				amount, e := decimalRaw(value)
				if e != nil {
					continue
				}
				ev := compactEvidence(map[string]any{"service": s.Name, "service_code": code, "country": names[result.Code], "country_code": result.Code, "operator": operator, "price": value, "count": stock, "currency": "USD"})
				d.minimum(s, "未知", amount, "USD", pricesURL, "public_api", ev)
				if detailedTarget(s, focus) {
					d.minimum(s, names[result.Code], amount, "USD", pricesURL, "public_api", ev)
				}
			}
		}
	}
	fmt.Printf("  SMSPVA：完整读取 %d/%d 个国家\n", completed, len(codes))
	d.CountriesRead = completed
	if completed == 0 {
		return errors.New("SMSPVA 所有国家的价格或库存均未完整取得")
	}
	if len(failures) > 0 {
		if len(failures) > 3 {
			failures = failures[:3]
		}
		d.warnings = append(d.warnings, fmt.Sprintf("SMSPVA 完成 %d/%d 个国家，未完成国家的旧价格保留并标记；%s", completed, len(codes), strings.Join(failures, "；")))
	}
	return nil
}

func smspvaGuestData(ctx context.Context, client *http.Client, p Provider, endpoint, method, country string, dst any) error {
	body := url.Values{"method": {method}, "country": {country}}
	r, e := publicRequest(ctx, client, p, http.MethodPost, endpoint, strings.NewReader(body.Encode()), "application/x-www-form-urlencoded")
	if e != nil {
		return e
	}
	defer r.Body.Close()
	raw, e := boundedBody(r.Body, 4<<20)
	if e != nil {
		return e
	}
	var check any
	if StrictJSON(raw, &check) != nil {
		return errors.New("SMSPVA 匿名接口不是完整 JSON")
	}
	var envelope struct {
		Response struct {
			OK   bool            `json:"res"`
			Data json.RawMessage `json:"data"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Response.OK || json.Unmarshal(envelope.Response.Data, dst) != nil {
		return errors.New("SMSPVA 匿名页面接口拒绝请求或结构变化")
	}
	return nil
}
