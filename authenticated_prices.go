package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type ProviderCredential struct {
	APIKey   string `json:"api_key"`
	Currency string `json:"currency"`
}

func loadProviderKeys(path string) (map[string]ProviderCredential, error) {
	var config struct {
		SchemaVersion int                           `json:"schema_version"`
		Providers     map[string]ProviderCredential `json:"providers"`
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, errors.New("无法读取本地平台密钥配置")
	}
	if len(raw) > 128<<10 || StrictJSON(raw, &config) != nil || config.SchemaVersion != 1 || config.Providers == nil {
		return nil, errors.New("平台密钥配置格式或版本无效")
	}
	for id, c := range config.Providers {
		if id != "sms-man" && id != "tiger-sms" && id != "grizzlysms" && id != "herosms" && id != "sv-number" {
			return nil, errors.New("平台密钥配置包含未支持的平台 ID")
		}
		if len(c.APIKey) > 4096 || strings.TrimSpace(c.APIKey) != c.APIKey {
			return nil, errors.New("平台密钥格式无效")
		}
		for _, r := range c.APIKey {
			if unicode.IsControl(r) {
				return nil, errors.New("平台密钥含控制字符")
			}
		}
		if c.APIKey != "" && !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(c.Currency) {
			return nil, errors.New("已填写平台密钥时须填写报价原币种（如 USD 或 RUB），不可猜测币种")
		}
	}
	return config.Providers, nil
}

// Only fixed catalog and price methods are permitted; redirects never forward query credentials.
func authenticatedJSON(ctx context.Context, client *http.Client, p Provider, endpoint, key string, params url.Values, dst any) error {
	if ValidateURL(endpoint, priceHosts(p)) != nil {
		return errors.New("认证价格接口不在官方域名白名单")
	}
	allowed := false
	switch p.ID {
	case "sms-man":
		allowed = endpoint == "https://api.sms-man.com/control/countries" || endpoint == "https://api.sms-man.com/control/applications" || endpoint == "https://api.sms-man.com/control/get-prices"
	case "tiger-sms":
		allowed = endpoint == "https://api.tiger-sms.com/stubs/handler_api.php"
	case "grizzlysms":
		allowed = endpoint == "https://api.grizzlysms.com/stubs/handler_api.php"
	case "herosms":
		allowed = endpoint == "https://hero-sms.com/stubs/handler_api.php"
	}
	if !allowed {
		return errors.New("认证价格接口地址不允许")
	}
	q := url.Values{}
	for k, v := range params {
		q[k] = append([]string{}, v...)
	}
	if p.ID == "sms-man" {
		q.Set("token", key)
	} else {
		switch q.Get("action") {
		case "getServicesList", "getCountries", "getPrices":
		default:
			return errors.New("仅允许目录和价格只读方法")
		}
		q.Set("api_key", key)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "identity-PriceReader/1.0")
	safe := *client
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// Large price dumps (e.g. GrizzlySMS ~4MB) sometimes drop mid-stream; retry transient read/network failures.
	var raw []byte
	var readErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return errors.New("认证价格接口超时（地址和密钥已隐藏）")
			case <-time.After(2 * time.Second):
			}
		}
		r, e := safe.Do(req.Clone(ctx))
		if e != nil {
			readErr = errors.New("认证价格接口网络失败或超时（地址和密钥已隐藏）")
			continue
		}
		if r.StatusCode != 200 {
			r.Body.Close()
			return fmt.Errorf("认证价格接口 HTTP %d（未跟随重定向）", r.StatusCode)
		}
		body, be := boundedBody(r.Body, 16<<20)
		r.Body.Close()
		if be != nil {
			readErr = be
			continue
		}
		raw, readErr = body, nil
		break
	}
	if readErr != nil {
		return readErr
	}
	text := strings.TrimSpace(string(raw))
	switch text {
	case "BAD_KEY", "NO_KEY", "INVALID_API_KEY":
		return errors.New("平台密钥无效或缺失")
	case "BAD_ACTION":
		return errors.New("该平台不支持此目录或价格方法")
	}
	var check any
	if StrictJSON(raw, &check) != nil {
		return errors.New("认证价格接口未返回完整 JSON，原始响应已隐藏")
	}
	var failure struct {
		ErrorCode string `json:"error_code"`
		Status    string `json:"status"`
	}
	json.Unmarshal(raw, &failure)
	if failure.ErrorCode != "" || failure.Status == "error" {
		return errors.New("平台认证或请求被拒绝，请检查平台密钥、API 权限与账户币种")
	}
	if json.Unmarshal(raw, dst) != nil {
		return errors.New("认证价格接口数据结构变化")
	}
	return nil
}

func collectAuthenticatedPrices(ctx context.Context, client *http.Client, p Provider, d *directPrices, cfg PriceConfig) (bool, error) {
	credential := cfg.Credentials[p.ID]
	if credential.APIKey == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if d.product.Kind != "once" {
		return true, errors.New("认证价格接口未绑定单次接码产品")
	}
	endpoint := "https://api." + p.ID + ".com/stubs/handler_api.php"
	if p.ID == "sms-man" {
		endpoint = "https://api.sms-man.com/control/"
	}
	if p.ID == "herosms" {
		endpoint = "https://hero-sms.com/stubs/handler_api.php"
	}
	request := func(method string, dst any) error {
		target := endpoint
		params := url.Values{}
		if p.ID == "sms-man" {
			target += method
		} else {
			params.Set("action", method)
			if method == "getServicesList" {
				params.Set("lang", "en")
			}
		}
		return authenticatedJSON(ctx, client, p, target, credential.APIKey, params, dst)
	}
	var services []struct {
		ID   json.Number `json:"id"`
		Code string      `json:"code"`
		Name string      `json:"name"`
	}
	serviceMethod, countryMethod, priceMethod := "getServicesList", "getCountries", "getPrices"
	if p.ID == "sms-man" {
		serviceMethod, countryMethod, priceMethod = "applications", "countries", "get-prices"
	}
	var raw json.RawMessage
	if e := request(serviceMethod, &raw); e != nil {
		return true, e
	}
	if json.Unmarshal(raw, &services) != nil {
		var wrapper struct {
			Status   string          `json:"status"`
			Services json.RawMessage `json:"services"`
		}
		if json.Unmarshal(raw, &wrapper) != nil || wrapper.Status != "success" || json.Unmarshal(wrapper.Services, &services) != nil {
			return true, errors.New("平台接码对象目录结构无效")
		}
	}
	if len(services) == 0 {
		return true, errors.New("平台接码对象目录为空")
	}
	source := endpoint
	listingSource := endpoint
	if p.ID == "sms-man" {
		source += priceMethod
		listingSource += serviceMethod
	} else {
		source += "?action=getPrices"
		listingSource += "?action=getServicesList"
	}
	ids := map[string]Service{}
	for _, v := range services {
		code := v.Code
		if p.ID == "sms-man" {
			code = string(v.ID)
		}
		if code == "" {
			continue
		}
		s, e := d.target(strings.TrimSpace(v.Name), listingSource, compactEvidence(map[string]string{"code": code, "name": v.Name}))
		if e == nil {
			ids[code] = s
		}
	}
	if e := request(countryMethod, &raw); e != nil {
		return true, e
	}
	type country struct {
		ID      json.Number `json:"id"`
		English string      `json:"eng"`
		Title   string      `json:"title"`
	}
	var list []country
	if json.Unmarshal(raw, &list) != nil {
		var keyed map[string]country
		if json.Unmarshal(raw, &keyed) != nil {
			return true, errors.New("平台国家目录结构无效")
		}
		for _, v := range keyed {
			list = append(list, v)
		}
	}
	countries := map[string]string{}
	for _, v := range list {
		name := v.English
		if name == "" {
			name = v.Title
		}
		if name != "" {
			countries[string(v.ID)] = name
		}
	}
	if len(countries) == 0 {
		return true, errors.New("平台国家目录为空")
	}
	var prices map[string]map[string]struct {
		Cost  json.RawMessage `json:"cost"`
		Count json.Number     `json:"count"`
	}
	if e := request(priceMethod, &prices); e != nil {
		return true, e
	}
	for countryID, rows := range prices {
		country := countries[countryID]
		if country == "" {
			continue
		}
		for code, row := range rows {
			s, ok := ids[code]
			if !ok {
				continue
			}
			amount, e := decimalRaw(row.Cost)
			stock, se := row.Count.Int64()
			if e != nil || se != nil || stock <= 0 {
				continue
			}
			ev := compactEvidence(map[string]any{"service": s.Name, "code": code, "country": country, "cost": row.Cost, "count": stock, "currency": credential.Currency})
			d.minimum(s, "未知", amount, credential.Currency, source, "authenticated_api", ev)
			if detailedTarget(s, cfg.PriceTargets) {
				d.minimum(s, country, amount, credential.Currency, source, "authenticated_api", ev)
			}
		}
	}
	if len(d.offers) == 0 {
		return true, errors.New("平台目录可用，但价格接口没有可用库存或有效金额")
	}
	return true, nil
}
