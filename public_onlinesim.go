package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type onlineTariffs struct {
	Response  string      `json:"response"`
	Country   json.Number `json:"country"`
	Countries map[string]struct {
		Name   string `json:"name"`
		Enable *bool  `json:"enable"`
	} `json:"countries"`
	Services map[string]struct {
		Service string          `json:"service"`
		Price   json.RawMessage `json:"price"`
		Count   json.Number     `json:"count"`
	} `json:"services"`
	End bool `json:"end"`
}
type onlinePage struct {
	Data onlineTariffs
	URL  string
}

func onlineCountryPages(ctx context.Context, client *http.Client, p Provider, code string, initial *onlinePage) ([]onlinePage, error) {
	var pages []onlinePage
	for page := 1; page <= 20; page++ {
		endpoint := fmt.Sprintf("https://onlinesim.io/api/getTariffs.php?lang=en&count=5000&page=%d&country=%s", page, code)
		var data onlineTariffs
		if page == 1 && initial != nil {
			data = initial.Data
			endpoint = initial.URL
		} else if e := publicJSON(ctx, client, p, endpoint, &data); e != nil {
			return nil, e
		}
		if data.Response != "1" {
			return nil, errors.New("OnlineSIM 国家价格接口未返回成功")
		}
		if string(data.Country) != code {
			return nil, errors.New("OnlineSIM 返回国家与请求不符，未交叉拼接价格")
		}
		pages = append(pages, onlinePage{data, endpoint})
		if data.End || len(data.Services) == 0 {
			return pages, nil
		}
	}
	return nil, errors.New("OnlineSIM 单个国家超过 20 页，未发布不完整分页价格")
}

func collectOnlineSIM(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	endpoint := "https://onlinesim.io/api/getTariffs.php?lang=en&count=5000&page=1"
	var initial onlineTariffs
	if e := publicJSON(ctx, client, p, endpoint, &initial); e != nil {
		return e
	}
	if initial.Response != "1" || len(initial.Countries) == 0 {
		return errors.New("OnlineSIM 国家目录为空或未成功")
	}
	names := map[string]string{}
	for key, v := range initial.Countries {
		code := strings.TrimPrefix(key, "_")
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(code) || v.Name == "" || (v.Enable != nil && !*v.Enable) {
			continue
		}
		names[code] = v.Name
	}
	if names[string(initial.Country)] == "" {
		return errors.New("OnlineSIM 默认国家不在公开目录")
	}
	codes := sortedKeys(names)
	d.CountriesExpected = len(codes)
	fmt.Printf("  OnlineSIM：读取 %d 个可用国家的分页价格（最多 3 个并发）\n", len(codes))
	results := readItems(ctx, codes, func(code string) ([]onlinePage, error) {
		var first *onlinePage
		if code == string(initial.Country) {
			first = &onlinePage{initial, endpoint}
		}
		return onlineCountryPages(ctx, client, p, code, first)
	})
	completed := 0
	var failures []string
	for _, result := range results {
		if result.Err != nil {
			failures = append(failures, result.Code+"："+result.Err.Error())
			continue
		}
		completed++
		for _, page := range result.Value {
			for _, code := range sortedKeys(page.Data.Services) {
				v := page.Data.Services[code]
				evidence := compactEvidence(map[string]any{"service": v.Service, "slug": code, "country": names[result.Code], "country_code": result.Code, "price": v.Price, "count": v.Count})
				s, e := d.target(strings.TrimSpace(v.Service), page.URL, evidence)
				if e != nil {
					continue
				}
				amount, ae := decimalRaw(v.Price)
				stock, se := v.Count.Int64()
				if ae != nil || se != nil || stock <= 0 {
					continue
				}
				d.minimum(s, "未知", amount, "USD", page.URL, "public_api", evidence)
				if detailedTarget(s, focus) {
					d.minimum(s, names[result.Code], amount, "USD", page.URL, "public_api", evidence)
				}
			}
		}
	}
	fmt.Printf("  OnlineSIM：完整读取 %d/%d 个国家\n", completed, len(codes))
	d.CountriesRead = completed
	if completed == 0 {
		return errors.New("OnlineSIM 所有国家价格均未完整取得")
	}
	if len(failures) > 0 {
		if len(failures) > 3 {
			failures = failures[:3]
		}
		d.warnings = append(d.warnings, fmt.Sprintf("OnlineSIM 完成 %d/%d 个国家，未完成国家的旧价格保留并标记；%s", completed, len(codes), strings.Join(failures, "；")))
	}
	return nil
}
