package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const PriceInterval = 24 * time.Hour

type PriceSource struct {
	Provider string `json:"provider"`
	Product  string `json:"product"`
	URL      string `json:"url"`
	Currency string `json:"currency,omitempty"`
}
type PriceConfig struct {
	SchemaVersion int                           `json:"schema_version"`
	Sources       []PriceSource                 `json:"sources"`
	PriceTargets  []string                      `json:"price_targets,omitempty"`
	ProviderKeys  string                        `json:"provider_keys,omitempty"`
	WebPrices     string                        `json:"web_prices,omitempty"`
	Credentials   map[string]ProviderCredential `json:"-"`
}

func LoadPriceConfig(path string) (PriceConfig, error) {
	var c PriceConfig
	b, e := os.ReadFile(path)
	if e != nil {
		return c, errors.New("无法读取价格更新配置")
	}
	if e = StrictJSON(b, &c); e != nil {
		return c, errors.New("价格更新配置 JSON 无效")
	}
	if c.SchemaVersion != 1 || len(c.Sources) == 0 {
		return c, errors.New("价格更新配置版本或来源无效")
	}
	if c.ProviderKeys != "" {
		keyPath := c.ProviderKeys
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(filepath.Dir(path), keyPath)
		}
		c.Credentials, e = loadProviderKeys(keyPath)
		if e != nil {
			return c, e
		}
	}
	if c.WebPrices != "" {
		if len(c.WebPrices) > 4096 || strings.ContainsAny(c.WebPrices, "\r\n") {
			return c, errors.New("网页报价快照路径无效")
		}
		if !filepath.IsAbs(c.WebPrices) {
			c.WebPrices = filepath.Join(filepath.Dir(path), c.WebPrices)
		}
	}
	return c, nil
}
func boundedBody(r io.Reader, limit int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, errors.New("响应读取失败或超出大小限制")
	}
	return b, nil
}

var nonContent = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>|<!--.*?-->`)
var tags = regexp.MustCompile(`<[^>]+>`)

func sourceText(raw []byte, contentType string) (string, error) {
	// shortcut: reads server-rendered HTML/JSON, use verified read-only endpoints for JavaScript-only directories.
	if !utf8.Valid(raw) {
		return "", errors.New("来源不是 UTF-8")
	}
	text := string(raw)
	if strings.Contains(strings.ToLower(contentType), "html") {
		text = html.UnescapeString(tags.ReplaceAllString(nonContent.ReplaceAllString(text, " "), " "))
	}
	text = strings.Join(strings.Fields(text), " ")
	text = smsCode.ReplaceAllString(text, smsRedaction)
	if len(text) < 10 || len(text) > 180000 {
		return "", errors.New("价格页为空或过大，请配置更具体的只读价格来源")
	}
	return text, nil
}
func fetchSource(ctx context.Context, client *http.Client, s PriceSource, p Provider) (string, error) {
	b, contentType, e := fetchPage(ctx, client, s, p)
	if e != nil {
		return "", e
	}
	return sourceText(b, contentType)
}
func fetchPage(ctx context.Context, client *http.Client, s PriceSource, p Provider) ([]byte, string, error) {
	if ValidateURL(s.URL, priceHosts(p)) != nil {
		return nil, "", errors.New("来源不在官方域名白名单")
	}
	safeClient := *client
	safeClient.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || ValidateURL(r.URL.String(), priceHosts(p)) != nil {
			return errors.New("来源重定向不允许")
		}
		return nil
	}
	var body []byte
	var ctype string
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, "", errors.New("官方目录请求超时（25 秒）")
			case <-time.After(2 * time.Second):
			}
		}
		body, ctype, lastErr = fetchPageOnce(ctx, &safeClient, s.URL)
		if lastErr == nil {
			return body, ctype, nil
		}
	}
	return nil, "", lastErr
}

func fetchPageOnce(ctx context.Context, safeClient *http.Client, rawURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if e != nil {
		return nil, "", errors.New("来源请求无效")
	}
	req.Header.Set("User-Agent", "identity-PriceReader/1.0")
	req.Header.Set("Accept", "text/html, application/json")
	response, e := safeClient.Do(req)
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) {
			return nil, "", errors.New("官方目录请求超时（25 秒）")
		}
		return nil, "", errors.New("官方价格页请求失败")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", fmt.Errorf("官方价格页 HTTP %d", response.StatusCode)
	}
	b, e := boundedBody(response.Body, 4<<20)
	if e != nil {
		return nil, "", e
	}
	return b, response.Header.Get("Content-Type"), nil
}

func atomicJSON(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".prices-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

func UpdatePrices(ctx context.Context, configPath, catalogPath string) (PriceUpdate, error) {
	cfg, e := LoadPriceConfig(configPath)
	if e != nil {
		return PriceUpdate{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	return updatePrices(ctx, cfg, catalogPath, &http.Client{Timeout: 180 * time.Second}, time.Now())
}
func updatePrices(ctx context.Context, cfg PriceConfig, catalogPath string, client *http.Client, now time.Time) (PriceUpdate, error) {
	info := PriceUpdate{AttemptedAt: now.Format(time.RFC3339), NextUpdateAt: now.Add(PriceInterval).Format(time.RFC3339), Failures: []UpdateFailure{}}
	lock, e := os.OpenFile(catalogPath+".update-lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return info, errors.New("已有价格更新任务或存在未清理的更新锁")
	}
	lock.Close()
	defer os.Remove(catalogPath + ".update-lock")
	original, e := os.ReadFile(catalogPath)
	if e != nil {
		return info, e
	}
	var catalog Catalog
	if e = StrictJSON(original, &catalog); e != nil {
		return info, e
	}
	if e = catalog.Validate(); e != nil {
		return info, e
	}
	if cfg.WebPrices != "" {
		snapshot, se := loadWebPrices(cfg.WebPrices)
		if se != nil {
			if !os.IsNotExist(se) {
				info.PriceWarnings = append(info.PriceWarnings, UpdateFailure{Provider: "web", Reason: "网页报价快照未使用：" + se.Error()})
			}
		} else {
			offers, listings, warnings, we := mergeWebPrices(&catalog, snapshot, now)
			if we != nil {
				return info, we
			}
			info.UpdatedOffers += offers
			info.UpdatedServices += listings
			for _, w := range warnings {
				info.PriceWarnings = append(info.PriceWarnings, UpdateFailure{Provider: "web", Reason: w})
			}
		}
	}
	providers := map[string]Provider{}
	products := map[string]Product{}
	for _, v := range catalog.Providers {
		providers[v.ID] = v
	}
	for _, v := range catalog.Products {
		products[v.ID] = v
	}
	seen := map[string]bool{}
	// Validate every source before any HTTP request or API charge.
	for _, s := range cfg.Sources {
		p, ok := products[s.Product]
		v := providers[s.Provider]
		key := s.Provider + "/" + s.Product
		if (s.Product != "" && (!ok || p.Provider != s.Provider)) || seen[key] || ValidateURL(s.URL, v.Hosts) != nil {
			return info, errors.New("抓取来源的产品、平台、域名或唯一性无效")
		}
		if s.Currency != "" && !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(s.Currency) {
			return info, errors.New("来源币种无效")
		}
		seen[key] = true
	}
	for sourceIndex, s := range cfg.Sources {
		if errors.Is(ctx.Err(), context.Canceled) {
			return info, errors.New("价格更新已取消，原目录未修改")
		}
		fmt.Printf("[%d/%d] %s：拉取接码对象与公开报价\n", sourceIndex+1, len(cfg.Sources), providers[s.Provider].Name)
		var err error
		var fresh []Offer
		var listings []ServiceListing
		var issues []string
		direct := newDirect(&catalog, s, products[s.Product], now)
		if ctx.Err() != nil {
			err = errors.New("本轮 25 分钟运行预算已用完，该来源未更新")
		} else {
			listings, fresh, issues, err = collectPriceSource(ctx, client, providers[s.Provider], direct, cfg)
		}
		priced := map[string]bool{}
		for _, o := range fresh {
			if o.ServiceID != "" {
				priced[o.ServiceID] = true
			}
		}
		info.Sources = append(info.Sources, SourceCollection{Provider: s.Provider, Name: providers[s.Provider].Name, ListedTargets: len(listings), PricedTargets: len(priced), Quotes: len(fresh), CountriesExpected: direct.CountriesExpected, CountriesRead: direct.CountriesRead, QuoteAttempts: direct.QuoteAttempts})
		if err == nil && len(listings) == 0 && len(fresh) == 0 {
			err = errors.New("未取得有原文依据的接码对象或价格；可能需登录、浏览器渲染或专用只读目录接口")
		}
		if err != nil {
			info.Failures = append(info.Failures, UpdateFailure{s.Provider, err.Error()})
			mergeTargetResults(&catalog, s, nil, nil, false)
			fmt.Printf("  未更新：%s\n", err.Error())
			continue
		}
		mergeTargetResults(&catalog, s, listings, fresh, len(issues) == 0)
		if len(fresh) == 0 {
			issues = append(issues, "接码对象已拉取，未取得可核验的公开价格")
		}
		if len(issues) > 0 {
			summary := strings.Join(issues, "；")
			info.PriceWarnings = append(info.PriceWarnings, UpdateFailure{s.Provider, summary})
			fmt.Printf("  采集说明：%s\n", summary)
		}
		info.UpdatedOffers += len(fresh)
		info.UpdatedServices += len(listings)
		fmt.Printf("  已拉取 %d 个接码对象，%d 条报价\n", len(listings), len(fresh))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return info, errors.New("价格更新已取消，原目录未修改")
	}
	info.CompletedAt = time.Now().Format(time.RFC3339)
	catalog.PriceUpdate = &info
	if e = catalog.Validate(); e != nil {
		return info, errors.New("采集后目录校验失败，未写入")
	}
	latest, e := os.ReadFile(catalogPath)
	if e != nil || !bytes.Equal(latest, original) {
		return info, errors.New("目录已被其他程序修改，未覆盖；请重试")
	}
	// Keep the catalog compact: large logged-in web snapshots must still fit the 32 MiB read limit.
	b, e := json.Marshal(catalog)
	if e != nil {
		return info, e
	}
	if len(b)+1 > 32<<20 {
		// shortcut: one catalog is capped at 32 MiB, split listings by provider when verified data exceeds it.
		return info, errors.New("对象和报价目录超过 32 MiB 限制，未覆盖旧目录；请缩小来源范围")
	}
	if e = atomicJSON(catalogPath+".backup", original); e != nil {
		return info, errors.New("无法保存旧目录备份，未写入新价格")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return info, errors.New("价格更新已取消，原目录未修改")
	}
	if e = atomicJSON(catalogPath, append(b, '\n')); e != nil {
		return info, errors.New("目录写入失败，原价格保留")
	}
	info.Saved = true
	if info.UpdatedServices == 0 && info.UpdatedOffers == 0 {
		return info, errors.New("所有来源均未取得有效接码对象；旧列表、旧价格与采集时间保留")
	}
	return info, nil
}
