package identity

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}}
}
func extraction(amount, country string) ExtractedPrice {
	return ExtractedPrice{ServiceID: "google", Country: country, Amount: amount, Currency: "USD", Unit: "activation", NumberType: "mobile", Term: "single activation", Scope: "SMS", Kind: "exact", Evidence: "Google " + country + " USD " + amount + " activation mobile SMS"}
}
func chatReply(x PriceExtraction) *http.Response {
	content, _ := json.Marshal(x)
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}, "finish_reason": "stop"}}})
	r := reply(200, string(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}
func priceCatalog(t *testing.T) string {
	t.Helper()
	c, e := LoadCatalog("data/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	c.Offers = slices.DeleteFunc(c.Offers, func(o Offer) bool { return strings.HasPrefix(o.ID, "auto-") || strings.HasPrefix(o.ID, "web-") })
	c.ServiceListings = nil
	c.PriceUpdate = nil
	// Some products are refreshed entirely by one API and have no manual seed.
	// Keep a valid fixture offer so validation exercises the post-update shape.
	for _, p := range c.Products {
		if slices.ContainsFunc(c.Offers, func(o Offer) bool { return o.Product == p.ID }) {
			continue
		}
		c.Offers = append(c.Offers, Offer{ID: p.ID + "-fixture-offer", Product: p.ID, Country: "未知", Service: "未知", NumberType: "按官网套餐", Term: "未知", Scope: "按官网套餐", Currency: "未知", Unit: "按官网套餐", Kind: "unknown", Status: "verified", Source: p.Sources[0], Verified: p.Verified})
	}
	b, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	return path
}
func testPriceConfig() PriceConfig {
	return PriceConfig{SchemaVersion: 1, BaseURL: "https://analysis.example/v1", APIKey: "fixture-secret-never-publish", Model: "fixture-model", AnalysisFallback: true, Sources: []PriceSource{{Provider: "fivesim", Product: "fivesim-once", URL: "https://5sim.net/prices", Currency: "USD"}, {Provider: "smspool", Product: "smspool-once", URL: "https://www.smspool.net/", Currency: "USD"}}}
}
func TestPriceUpdateToGoogleSearchAndPriceOrder(t *testing.T) {
	path := priceCatalog(t)
	cfg := testPriceConfig()
	requests := 0
	now := time.Now().Truncate(time.Second)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host == "analysis.example" {
			if r.Header.Get("Authorization") != "Bearer "+cfg.APIKey || r.URL.Path != "/v1/chat/completions" {
				t.Fatal("wrong analysis endpoint/auth")
			}
			body, _ := io.ReadAll(r.Body)
			var request struct {
				Model    string `json:"model"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if json.Unmarshal(body, &request) != nil || request.Model != cfg.Model {
				t.Fatal("wrong model request")
			}
			amount := "0.02"
			country := "USA"
			if strings.Contains(request.Messages[1].Content, "Pool source") {
				amount = "0.1"
				country = "United States"
			}
			return chatReply(PriceExtraction{Offers: []ExtractedPrice{extraction(amount, country)}}), nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("API key sent to SMS source")
		}
		amount, country, marker := "0.02", "USA", "Five source"
		if r.URL.Host == "www.smspool.net" {
			amount, country, marker = "0.1", "United States", "Pool source"
		}
		return reply(200, "<html><script>Google USA USD 0.001 activation</script><p>"+marker+" "+extraction(amount, country).Evidence+"</p></html>"), nil
	})}
	info, e := updatePrices(context.Background(), cfg, path, client, now)
	if e != nil {
		t.Fatal(e)
	}
	if requests != 6 || info.UpdatedOffers != 2 || len(info.Failures) != 0 {
		t.Fatal("unexpected update summary", info)
	}
	if len(info.Sources) != 2 || info.Sources[0].ListedTargets != 1 || info.Sources[0].PricedTargets != 1 || info.Sources[0].Quotes != 1 {
		t.Fatal("source coverage did not distinguish targets and quotes")
	}
	if info.NextUpdateAt != now.Add(24*time.Hour).Format(time.RFC3339) {
		t.Fatal("not a 24h schedule")
	}
	c, e := LoadCatalog(path)
	if e != nil {
		t.Fatal(e)
	}
	products := map[string]Product{}
	for _, p := range c.Products {
		products[p.ID] = p
	}
	SortOffers(c.Offers, products)
	var google []Offer
	for _, o := range c.Offers {
		if o.ServiceID == "google" {
			google = append(google, o)
		}
	}
	if len(google) != 2 || *google[0].Amount != "0.02" || *google[1].Amount != "0.1" || google[0].Country != "美国" || google[1].Country != "美国" {
		t.Fatal("same-country Google prices not normalized/sorted", google)
	}
	out := filepath.Join(t.TempDir(), "dist")
	if e = Generate(path, "", out); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(out, "index.html"))
	page := html.UnescapeString(string(b))
	if !strings.Contains(page, "谷歌") || !strings.Contains(page, "Google") || strings.Index(page, "USD 0.02") > strings.Index(page, "USD 0.1") {
		t.Fatal("Google search data or price order missing")
	}
	if strings.Contains(page, cfg.APIKey) || strings.Contains(page, cfg.BaseURL) || strings.Contains(page, cfg.Model) {
		t.Fatal("private API settings leaked to public page")
	}
	if _, e = os.Stat(path + ".backup"); e != nil {
		t.Fatal("no last-good backup")
	}
	before := google[0].CheckedAt
	failing := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return reply(403, "blocked"), nil })}
	failedInfo, e := updatePrices(context.Background(), cfg, path, failing, now.Add(24*time.Hour))
	if e == nil || len(failedInfo.Failures) != 2 {
		t.Fatal("failure reported as success")
	}
	c, _ = LoadCatalog(path)
	kept := 0
	for _, o := range c.Offers {
		if o.ServiceID == "google" {
			kept++
			if o.Status != "unavailable" || o.CheckedAt != before || o.Amount == nil {
				t.Fatal("old prices erased or falsely refreshed")
			}
		}
	}
	if kept != 2 {
		t.Fatal("old prices lost")
	}
}
func TestExtractionRejectsInventedAndMismatchedPrices(t *testing.T) {
	cfg := testPriceConfig()
	p := Product{Kind: "once"}
	services := []Service{{ID: "google", Name: "Google", Aliases: []string{"谷歌"}}}
	valid := extraction("0.12", "USA")
	text := valid.Evidence
	if _, e := validateExtraction(valid, text, cfg.Sources[0], p, services, time.Now()); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*ExtractedPrice){func(x *ExtractedPrice) { x.Amount = "0.1" }, func(x *ExtractedPrice) { x.ServiceID = "invented" }, func(x *ExtractedPrice) { x.Country = "India" }, func(x *ExtractedPrice) { x.Currency = "EUR" }, func(x *ExtractedPrice) { x.Unit = "month" }, func(x *ExtractedPrice) { x.Term = "30 days" }, func(x *ExtractedPrice) { x.Amount = "-1" }, func(x *ExtractedPrice) { x.Evidence = "Google USA USD 0.12 activation invented" }} {
		x := valid
		change(&x)
		if _, e := validateExtraction(x, text, cfg.Sources[0], p, services, time.Now()); e == nil {
			t.Fatal("invented dimension accepted", x)
		}
	}
	from := valid
	from.Evidence = "Google USA from USD 0.12 activation mobile SMS"
	if _, e := validateExtraction(from, from.Evidence, cfg.Sources[0], p, services, time.Now()); e == nil {
		t.Fatal("starting price treated as exact")
	}
	rental := valid
	rental.Term = "1 month"
	rental.Evidence += " 1 month"
	o, e := validateExtraction(rental, rental.Evidence, cfg.Sources[0], Product{Kind: "rental"}, services, time.Now())
	if e != nil || o.Term != "1 month" {
		t.Fatal("month normalization changed duration")
	}
	if containsEvidence("GoogleAnalytics USA USD 0.12", "Google") || containsEvidence("Google USA USD 0.123", "0.12") {
		t.Fatal("partial token accepted as evidence")
	}
	starting := valid
	starting.Kind = "starting"
	starting.Country = "未知"
	starting.Unit = "未注明"
	starting.NumberType = "未注明"
	starting.Scope = "未注明"
	starting.Evidence = "Google from USD 0.12"
	if o, e := validateExtraction(starting, starting.Evidence, cfg.Sources[0], p, services, time.Now()); e != nil || o.Unit != "未注明" || o.Kind != "starting" {
		t.Fatal("explicit starting price with unknown unit was lost", e)
	}
}
func TestPriceConfigAndUnsafeSourceDoNotSendRequests(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(config, []byte(`{"schema_version":1,"baseurl":"","api_key":"","model":"","analysis_fallback":true,"sources":[{}]}`), 0600)
	if _, e := LoadPriceConfig(config); e == nil {
		t.Fatal("empty credentials accepted")
	}
	for _, raw := range []string{"http://remote.example/v1", "https://name:password@api.example/v1", "https://api.example/v1?key=secret", "https://api.example/v1#fragment"} {
		if _, e := analysisEndpoint(raw); e == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, raw := range []string{"https://api.example/v1", "http://127.0.0.1:1234/v1", "https://api.example/v1/chat/completions"} {
		if endpoint, e := analysisEndpoint(raw); e != nil || !strings.HasSuffix(endpoint, "/chat/completions") {
			t.Fatal("valid endpoint rejected")
		}
	}
	path := priceCatalog(t)
	before, _ := os.ReadFile(path)
	cfg := testPriceConfig()
	cfg.Sources[1].URL = "https://www.smspool.net.evil.example/"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe configuration caused a request")
		return nil, nil
	})}
	if _, e := updatePrices(context.Background(), cfg, path, client, time.Now()); e == nil {
		t.Fatal("unsafe source accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("unsafe config modified catalog")
	}
}
func TestAnalysisFailuresAndBodyLimits(t *testing.T) {
	cfg := testPriceConfig()
	p := Product{Kind: "once"}
	responses := []*http.Response{reply(401, cfg.APIKey), reply(200, `{"choices":[],"choices":[]}`), reply(200, `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`), reply(200, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"offers\":null}"}}]}`)}
	for _, response := range responses {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response, nil })}
		_, e := analyzeSource(context.Background(), client, cfg, cfg.Sources[0], p, nil, "public price text")
		if e == nil || strings.Contains(e.Error(), cfg.APIKey) {
			t.Fatal("bad response accepted or key leaked")
		}
	}
	if _, e := boundedBody(strings.NewReader("12345"), 4); e == nil {
		t.Fatal("oversize accepted")
	}
	text, e := sourceText([]byte(`<script>fake secret</script><p>Google USA USD 0.12 activation</p>`), "text/html")
	if e != nil || strings.Contains(text, "secret") {
		t.Fatal("script text not stripped")
	}
}
