package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func priceHosts(p Provider) []string { return append(append([]string{}, p.Hosts...), p.APIHosts...) }

type directPrices struct {
	listings          map[string]ServiceListing
	offers            map[string]Offer
	warnings          []string
	catalog           *Catalog
	source            PriceSource
	product           Product
	now               time.Time
	names             map[string]Service
	CountriesExpected int
	CountriesRead     int
	QuoteAttempts     map[string]string
}

func newDirect(c *Catalog, s PriceSource, p Product, now time.Time) *directPrices {
	d := &directPrices{listings: map[string]ServiceListing{}, offers: map[string]Offer{}, catalog: c, source: s, product: p, now: now, names: targetIndex(c.Services)}
	if s.Provider == "sms4sats" {
		d.QuoteAttempts = map[string]string{}
		if c.PriceUpdate != nil {
			for _, v := range c.PriceUpdate.Sources {
				if v.Provider == s.Provider {
					for code, checked := range v.QuoteAttempts {
						d.QuoteAttempts[code] = checked
					}
				}
			}
		}
		for _, o := range c.Offers {
			if o.Product != s.Product || o.Method != "public_api" || o.CheckedAt == "" {
				continue
			}
			var ev struct {
				Code string `json:"code"`
			}
			if json.Unmarshal([]byte(o.Evidence), &ev) == nil && ev.Code != "" && d.QuoteAttempts[ev.Code] == "" {
				d.QuoteAttempts[ev.Code] = o.CheckedAt
			}
		}
	}
	return d
}
func decimalRaw(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if strings.HasPrefix(value, `"`) {
		if json.Unmarshal(raw, &value) != nil {
			return "", errors.New("金额格式无效")
		}
	}
	if !decimal.MatchString(value) {
		return "", errors.New("金额不是非负十进制数")
	}
	return value, nil
}

// decimalAdd sums finite decimal strings without converting to float.
func decimalAdd(values ...string) (string, error) {
	sum := new(big.Rat)
	for _, value := range values {
		if !decimal.MatchString(value) {
			return "", errors.New("金额不是非负十进制数")
		}
		n, ok := new(big.Rat).SetString(value)
		if !ok {
			return "", errors.New("金额格式无效")
		}
		sum.Add(sum, n)
	}
	s := sum.FloatString(18)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0", nil
	}
	return s, nil
}
func compactEvidence(record any) string { b, _ := json.Marshal(record); return string(b) }
func (d *directPrices) target(name, url, evidence string) (Service, error) {
	// Names come from documented metadata fields, not guessed from numeric service codes.
	if name == "" || len(name) > 160 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\r\n\x00") {
		return Service{}, errors.New("接码对象名称无效")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return Service{}, errors.New("目录名称含控制字符")
		}
	}
	key := targetNameKey(name)
	service, exists := d.names[key]
	if !exists {
		sum := sha256.Sum256([]byte(key))
		service = Service{ID: "target-" + hex.EncodeToString(sum[:10]), Name: name, Aliases: []string{}}
		d.catalog.Services = append(d.catalog.Services, service)
		d.names[key] = service
	}
	if len(evidence) > 1600 || messageEvidence(evidence) {
		return service, errors.New("目录证据无效")
	}
	d.listings[service.ID] = ServiceListing{Provider: d.source.Provider, Product: d.source.Product, ServiceID: service.ID, Name: name, Source: url, Evidence: evidence, CheckedAt: d.now.Format(time.RFC3339), Status: "verified"}
	return service, nil
}
func (d *directPrices) minimum(service Service, country, amount, currency, url, method, evidence string) {
	if d.product.ID == "" || d.product.Kind != "once" || !decimal.MatchString(amount) || currency == "" || country == "" || len(evidence) > 1600 || messageEvidence(evidence) {
		return
	}
	country = normalizedCountry(country)
	key, _ := json.Marshal([]string{d.source.Provider, d.product.ID, service.ID, country, currency, "每次激活", "single activation", "未注明", "单服务；号池未指定", "starting"})
	sum := sha256.Sum256(key)
	id := "auto-" + hex.EncodeToString(sum[:12])
	if old, ok := d.offers[id]; ok {
		a, _ := new(big.Rat).SetString(amount)
		b, _ := new(big.Rat).SetString(*old.Amount)
		if a.Cmp(b) >= 0 {
			return
		}
	}
	d.offers[id] = Offer{ID: id, Product: d.product.ID, ServiceID: service.ID, Service: service.Name, Country: country, Amount: &amount, Currency: currency, Unit: "每次激活", NumberType: "未注明", Scope: "单服务；号池未指定", Term: "single activation", Kind: "starting", Status: "verified", Source: url, Verified: d.now.Format("2006-01-02"), CheckedAt: d.now.Format(time.RFC3339), Evidence: evidence, Method: method}
}

// rental records one exact rental package. LongSMS prices are per
// country/project/duration, so they must not be flattened into a global minimum.
func (d *directPrices) rental(service Service, country, numberType, term, scope, amount, currency, unit, url, method, evidence string) {
	if d.product.ID == "" || d.product.Kind != "rental" || !decimal.MatchString(amount) || currency == "" || country == "" || term == "" || scope == "" || unit == "" || len(evidence) > 1600 || messageEvidence(evidence) {
		return
	}
	country = normalizedCountry(country)
	key, _ := json.Marshal([]string{d.source.Provider, d.product.ID, service.ID, country, numberType, term, scope, currency, unit, "exact"})
	sum := sha256.Sum256(key)
	id := "auto-" + hex.EncodeToString(sum[:12])
	d.offers[id] = Offer{ID: id, Product: d.product.ID, ServiceID: service.ID, Service: service.Name, Country: country, Amount: &amount, Currency: currency, Unit: unit, NumberType: numberType, Scope: scope, Term: term, Kind: "exact", Status: "verified", Source: url, Verified: d.now.Format("2006-01-02"), CheckedAt: d.now.Format(time.RFC3339), Evidence: evidence, Method: method}
}

// sitewide records a product-level advertised starting price when the source
// publishes no per-target prices (SMSFAST). It replaces the old manual offer.
func (d *directPrices) sitewide(amount, currency, url, evidence string) {
	if d.product.ID == "" || d.product.Kind != "once" || !decimal.MatchString(amount) || currency == "" || len(evidence) > 1600 || messageEvidence(evidence) {
		return
	}
	key, _ := json.Marshal([]string{d.source.Provider, d.product.ID, "sitewide", currency})
	sum := sha256.Sum256(key)
	id := "auto-" + hex.EncodeToString(sum[:12])
	d.offers[id] = Offer{ID: id, Product: d.product.ID, Country: "未知", Service: "未知", NumberType: "按官网套餐", Term: "未知", Scope: "按官网套餐", Amount: &amount, Currency: currency, Unit: "激活起", Kind: "starting", Status: "verified", Source: url, Verified: d.now.Format("2006-01-02"), CheckedAt: d.now.Format(time.RFC3339), Evidence: evidence, Method: "page_text"}
	kept := d.catalog.Offers[:0]
	for _, o := range d.catalog.Offers {
		if o.Product == d.product.ID && o.ID == d.product.ID+"-offer" {
			continue
		}
		kept = append(kept, o)
	}
	d.catalog.Offers = kept
}
func (d *directPrices) values() ([]ServiceListing, []Offer) {
	l := make([]ServiceListing, 0, len(d.listings))
	o := make([]Offer, 0, len(d.offers))
	for _, v := range d.listings {
		l = append(l, v)
	}
	for _, v := range d.offers {
		o = append(o, v)
	}
	slices.SortFunc(l, func(a, b ServiceListing) int { return strings.Compare(a.ServiceID, b.ServiceID) })
	slices.SortFunc(o, func(a, b Offer) int { return strings.Compare(a.ID, b.ID) })
	return l, o
}
func detailedTarget(s Service, focus []string) bool {
	// The fallback "any other" target is shown as one object across channels; only
	// its per-country prices answer "how much in this country", so always keep them.
	if umbrellaKey(s.Name) {
		return true
	}
	if len(focus) == 0 {
		focus = []string{"Google", "Telegram", "WhatsApp", "OpenAI"}
	}
	for _, name := range append([]string{s.Name}, s.Aliases...) {
		for _, want := range focus {
			if targetNameKey(name) == targetNameKey(want) {
				return true
			}
		}
	}
	return false
}
func publicResponse(ctx context.Context, client *http.Client, p Provider, url string) (*http.Response, error) {
	return publicRequest(ctx, client, p, http.MethodGet, url, nil, "")
}
func publicRequest(ctx context.Context, client *http.Client, p Provider, method, url string, body io.Reader, contentType string) (*http.Response, error) {
	if ValidateURL(url, priceHosts(p)) != nil {
		return nil, errors.New("公开价格 API 域名未登记")
	}
	req, e := http.NewRequestWithContext(ctx, method, url, body)
	if e != nil {
		return nil, errors.New("价格 API 请求格式无效")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "identity-PriceReader/1.0")
	safe := *client
	safe.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || ValidateURL(r.URL.String(), priceHosts(p)) != nil {
			return errors.New("价格 API 重定向不允许")
		}
		return nil
	}
	r, e := safe.Do(req)
	if e != nil {
		return nil, errors.New("公开价格 API 请求失败或超时")
	}
	if r.StatusCode != 200 {
		r.Body.Close()
		return nil, fmt.Errorf("公开价格 API HTTP %d", r.StatusCode)
	}
	return r, nil
}
func publicJSON(ctx context.Context, client *http.Client, p Provider, url string, dst any) error {
	// Retry transient read/network failures; large/slow dumps occasionally drop mid-stream.
	var b []byte
	var readErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return errors.New("公开价格 API 请求失败或超时")
			case <-time.After(2 * time.Second):
			}
		}
		r, e := publicResponse(ctx, client, p, url)
		if e != nil {
			readErr = e
			continue
		}
		body, be := boundedBody(r.Body, 4<<20)
		r.Body.Close()
		if be != nil {
			readErr = be
			continue
		}
		b, readErr = body, nil
		break
	}
	if readErr != nil {
		return readErr
	}
	var raw any
	if StrictJSON(b, &raw) != nil {
		return errors.New("公开价格 API 未返回有效 JSON")
	}
	if json.Unmarshal(b, dst) != nil {
		return errors.New("公开价格 API 结构发生变化")
	}
	return nil
}

func collectPublicAPI(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) (bool, error) {
	if d.product.Kind != "once" && !(p.ID == "longsms" && d.product.Kind == "rental") {
		return false, nil
	}
	timeout := 3 * time.Minute
	if p.ID == "smspva" || p.ID == "onlinesim" || p.ID == "sms4sats" || p.ID == "sms-man" {
		timeout = 6 * time.Minute
	}
	if p.ID == "smsak" {
		timeout = 7 * time.Minute
		copy := *client
		copy.Timeout = 6 * time.Minute
		client = &copy
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch p.ID {
	case "fivesim":
		return true, collectFiveSIM(ctx, client, p, d, focus)
	case "smspool":
		return true, collectSMSPool(ctx, client, p, d, focus)
	case "onlinesim":
		return true, collectOnlineSIM(ctx, client, p, d, focus)
	case "smsak":
		return true, collectSMSAK(ctx, client, p, d, focus)
	case "sms4sats":
		return true, collectSMS4Sats(ctx, client, p, d, focus)
	case "smspva":
		return true, collectSMSPVA(ctx, client, p, d, focus)
	case "sms-man":
		return true, collectSMSMan(ctx, client, p, d, focus)
	case "longsms":
		return true, collectLongSMS(ctx, client, p, d)
	}
	return false, nil
}

// LongSMS publishes long-term packages without authentication. The client API
// key only covers long-term order operations and has no price-list action.
func collectLongSMS(ctx context.Context, client *http.Client, p Provider, d *directPrices) error {
	if d.product.Kind != "rental" {
		return errors.New("LongSMS 长期价格只能绑定租赁产品")
	}
	var countries struct {
		Success bool `json:"success"`
		Data    []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			CardType string `json:"card_type"`
			Stock    int    `json:"stock"`
			IsActive bool   `json:"is_active"`
		} `json:"data"`
	}
	endpoint := "https://longsms.net/api/countries"
	if e := publicJSON(ctx, client, p, endpoint, &countries); e != nil {
		return e
	}
	if !countries.Success || len(countries.Data) == 0 {
		return errors.New("LongSMS 国家目录为空")
	}
	d.CountriesExpected = len(countries.Data)
	for _, country := range countries.Data {
		if !country.IsActive {
			continue
		}
		priceURL := fmt.Sprintf("https://longsms.net/api/pricing?country_id=%d&product_line=long", country.ID)
		var prices struct {
			Success bool `json:"success"`
			Data    []struct {
				Days               int             `json:"days"`
				Price              json.RawMessage `json:"price"`
				FirstActivationFee json.RawMessage `json:"first_activation_fee"`
				Active             bool            `json:"active"`
				ProductLine        string          `json:"product_line"`
				LongProjectCode    string          `json:"long_project_code"`
			} `json:"data"`
		}
		if e := publicJSON(ctx, client, p, priceURL, &prices); e != nil {
			d.warnings = append(d.warnings, fmt.Sprintf("LongSMS 国家 %s 价格：%s", country.Name, e.Error()))
			continue
		}
		d.CountriesRead++
		if !prices.Success {
			d.warnings = append(d.warnings, fmt.Sprintf("LongSMS 国家 %s 价格接口未成功", country.Name))
			continue
		}
		// stock_zero_action=hide upstream; do not publish a country the site hides.
		if country.Stock <= 0 {
			continue
		}
		numberType := longSMSNumberType(country.CardType)
		for _, row := range prices.Data {
			if !row.Active || row.Days <= 0 || row.ProductLine != "long" {
				continue
			}
			price, e := decimalRaw(row.Price)
			if e != nil {
				continue
			}
			fee := "0"
			if value, feeErr := decimalRaw(row.FirstActivationFee); feeErr == nil {
				fee = value
			}
			amount, e := decimalAdd(price, fee)
			if e != nil {
				continue
			}
			serviceName := longSMSProjectName(row.LongProjectCode)
			service, e := d.target(serviceName, priceURL, compactEvidence(map[string]string{"country_id": fmt.Sprint(country.ID), "long_project_code": row.LongProjectCode}))
			if e != nil {
				continue
			}
			unit := fmt.Sprintf("每个号码 / %d 天", row.Days)
			if fee != "0" {
				unit += fmt.Sprintf("（含首次激活费 %s USD）", fee)
			}
			evidence := compactEvidence(map[string]any{"country_id": country.ID, "country": country.Name, "card_type": country.CardType, "stock": country.Stock, "days": row.Days, "price": row.Price, "first_activation_fee": row.FirstActivationFee, "total": amount, "long_project_code": row.LongProjectCode, "currency": "USD"})
			term := fmt.Sprintf("%d days", row.Days)
			d.rental(service, country.Name, numberType, term, "long_project_code="+row.LongProjectCode, amount, "USD", unit, priceURL, "public_api", evidence)
		}
	}
	if len(d.offers) == 0 {
		return errors.New("LongSMS 价格接口没有可用库存或有效金额")
	}
	return nil
}

func longSMSProjectName(code string) string {
	switch code {
	case "other":
		return "长期项目 other（未映射应用）"
	case "hosting_other":
		return "长期项目 hosting_other（未映射应用）"
	case "hosting_full":
		return "长期项目 hosting_full（未映射应用）"
	case "full_pack":
		return "长期项目 full_pack（未映射应用）"
	default:
		return "长期项目 " + code
	}
}

func longSMSNumberType(cardType string) string {
	switch strings.ToLower(cardType) {
	case "virtual":
		return "虚拟号"
	case "mobile":
		return "移动号"
	}
	if cardType != "" {
		return cardType
	}
	return "未注明"
}

func collectFiveSIM(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	endpoint := "https://5sim.net/v1/guest/products/any/any"
	var data map[string]struct {
		Category string
		Qty      json.Number
		Price    json.RawMessage
	}
	if e := publicJSON(ctx, client, p, endpoint, &data); e != nil {
		return e
	}
	if len(data) == 0 {
		return errors.New("5SIM 产品列表为空")
	}
	var detail map[string]Service = map[string]Service{}
	currency := d.source.Currency
	if currency == "" {
		currency = "5SIM 原币种"
	}
	for _, name := range sortedKeys(data) {
		v := data[name]
		if v.Category != "activation" {
			continue
		}
		evidence := compactEvidence(map[string]any{"product": name, "Category": v.Category, "Price": v.Price, "Qty": v.Qty})
		s, e := d.target(name, endpoint, evidence)
		if e != nil {
			continue
		}
		if detailedTarget(s, focus) {
			detail[name] = s
		}
		price, e := decimalRaw(v.Price)
		qty, e2 := strconv.ParseInt(string(v.Qty), 10, 64)
		if e == nil && e2 == nil && qty > 0 {
			d.minimum(s, "未知", price, currency, endpoint, "public_api", evidence)
		}
	}
	var countryMeta map[string]struct {
		ISO  map[string]json.Number `json:"iso"`
		Name string                 `json:"text_en"`
	}
	if len(detail) > 0 {
		if e := publicJSON(ctx, client, p, "https://5sim.net/v1/guest/countries", &countryMeta); e != nil {
			d.warnings = append(d.warnings, "5SIM 国家映射未更新，部分国家保留官网代码")
		}
	}
	for _, name := range sortedKeys(detail) {
		url := "https://5sim.net/v1/guest/prices?product=" + url.QueryEscape(name)
		var table map[string]map[string]map[string]struct {
			Cost  json.RawMessage `json:"cost"`
			Count json.Number     `json:"count"`
		}
		if e := publicJSON(ctx, client, p, url, &table); e != nil {
			d.warnings = append(d.warnings, "5SIM 分国家价格未取得，保留该对象全球起价")
			continue
		}
		countries, ok := table[name]
		if !ok {
			d.warnings = append(d.warnings, "5SIM 分国家价格结构变化")
			continue
		}
		for country, pools := range countries {
			countryName := country
			if meta, ok := countryMeta[country]; ok {
				if len(meta.ISO) == 1 {
					for iso := range meta.ISO {
						countryName = strings.ToUpper(iso)
					}
				} else if meta.Name != "" {
					countryName = meta.Name
				}
			}
			for pool, row := range pools {
				price, e := decimalRaw(row.Cost)
				stock, se := strconv.ParseInt(string(row.Count), 10, 64)
				if e != nil || se != nil || stock <= 0 {
					continue
				}
				record := map[string]any{"product": name, "country": country, "operator": pool, "cost": row.Cost, "count": row.Count, "selection": "同国家未指定运营商时的最低公开报价"}
				d.minimum(detail[name], countryName, price, currency, url, "public_api", compactEvidence(record))
			}
		}
	}
	return nil
}

func collectSMSPool(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	endpoint := "https://api.smspool.net/service/retrieve_all"
	var services []struct {
		ID   json.Number `json:"ID"`
		Name string      `json:"name"`
	}
	if e := publicJSON(ctx, client, p, endpoint, &services); e != nil {
		return e
	}
	ids := map[string]Service{}
	for _, v := range services {
		s, e := d.target(v.Name, endpoint, compactEvidence(v))
		if e == nil {
			ids[string(v.ID)] = s
		}
	}
	prices := "https://api.smspool.net/request/pricing"
	r, e := publicResponse(ctx, client, p, prices)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	limited := &io.LimitedReader{R: r.Body, N: 64 << 20}
	dec := json.NewDecoder(limited)
	dec.UseNumber()
	start, e := dec.Token()
	if e != nil || start != json.Delim('[') {
		return errors.New("SMSPool 价格响应不是数组")
	}
	for dec.More() {
		var row struct {
			Service     json.Number     `json:"service"`
			ServiceName string          `json:"service_name"`
			CountryName string          `json:"country_name"`
			CountryISO  string          `json:"short_name"`
			Pool        json.Number     `json:"pool"`
			Price       json.RawMessage `json:"price"`
		}
		if dec.Decode(&row) != nil {
			return errors.New("SMSPool 价格响应未完整读取")
		}
		s, ok := ids[string(row.Service)]
		if !ok || row.ServiceName == "" || row.CountryName == "" {
			continue
		}
		amount, e := decimalRaw(row.Price)
		if e != nil {
			continue
		}
		evidence := compactEvidence(row)
		d.minimum(s, "未知", amount, "USD", prices, "public_api", evidence)
		if detailedTarget(s, focus) {
			country := row.CountryName
			if len(row.CountryISO) == 2 {
				country = strings.ToUpper(row.CountryISO)
			}
			d.minimum(s, country, amount, "USD", prices, "public_api", evidence)
		}
	}
	if _, e = dec.Token(); e != nil || limited.N <= 0 {
		return errors.New("SMSPool 价格响应超出大小限制或不完整")
	}
	if _, e = dec.Token(); e != io.EOF {
		return errors.New("SMSPool 价格响应尾部无效")
	}
	return nil
}

func collectSMSAK(ctx context.Context, client *http.Client, p Provider, d *directPrices, focus []string) error {
	endpoint := "https://sms-acktiwator.ru/api/otp/?action=getServicesList"
	var services struct {
		Status   string `json:"status"`
		Services []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"services"`
	}
	if e := publicJSON(ctx, client, p, endpoint, &services); e != nil {
		return e
	}
	if services.Status != "success" {
		return errors.New("SMSAK 对象目录未成功")
	}
	ids := map[string]Service{}
	for _, v := range services.Services {
		s, e := d.target(v.Name, endpoint, compactEvidence(v))
		if e == nil {
			ids[v.Code] = s
		}
	}
	var countries map[string]struct {
		English string `json:"eng"`
	}
	if e := publicJSON(ctx, client, p, "https://sms-acktiwator.ru/api/otp/?action=getCountries", &countries); e != nil {
		return e
	}
	prices := "https://sms-acktiwator.ru/api/otp/?action=getPrices"
	fmt.Println("  SMSAK 完整价格响应较大，下载上限 6 分钟；完整校验后才发布")
	r, e := publicResponse(ctx, client, p, prices)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	limited := &io.LimitedReader{R: r.Body, N: 64 << 20}
	dec := json.NewDecoder(limited)
	dec.UseNumber()
	start, e := dec.Token()
	if e != nil || start != json.Delim('{') {
		return errors.New("SMSAK 价格响应不是国家映射")
	}
	currency := d.source.Currency
	if currency == "" {
		currency = "SMSAK 原币种"
	}
	for dec.More() {
		token, e := dec.Token()
		if e != nil {
			return errors.New("SMSAK 价格响应读取失败")
		}
		countryID, ok := token.(string)
		if !ok {
			return errors.New("SMSAK 国家编号无效")
		}
		var rows map[string]struct {
			Cost  json.RawMessage `json:"cost"`
			Count json.Number     `json:"count"`
		}
		if e := dec.Decode(&rows); e != nil {
			if errors.Is(e, context.DeadlineExceeded) {
				return errors.New("SMSAK 价格响应下载超时，截断价格未发布")
			}
			return errors.New("SMSAK 价格响应 JSON 结构无效或未完整读取")
		}
		country := countries[countryID].English
		if country == "" {
			continue
		}
		for code, row := range rows {
			s, ok := ids[code]
			if !ok {
				continue
			}
			amount, e := decimalRaw(row.Cost)
			stock, se := strconv.ParseInt(string(row.Count), 10, 64)
			if e != nil || se != nil || stock <= 0 {
				continue
			}
			evidence := compactEvidence(map[string]any{"code": code, "service": s.Name, "country": country, "cost": row.Cost, "count": row.Count})
			d.minimum(s, "未知", amount, currency, prices, "public_api", evidence)
			if detailedTarget(s, focus) {
				d.minimum(s, country, amount, currency, prices, "public_api", evidence)
			}
		}
	}
	if _, e = dec.Token(); e != nil || limited.N <= 0 {
		return errors.New("SMSAK 价格响应超出大小限制或不完整")
	}
	if _, e = dec.Token(); e != io.EOF {
		return errors.New("SMSAK 价格响应尾部无效")
	}
	return nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
