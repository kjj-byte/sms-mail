package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFallbackTargetAlwaysKeepsCountryPrices(t *testing.T) {
	for _, name := range []string{"AnyOther", "Any other", "Not Listed / Other / Any", "Любой другой", "other"} {
		if !detailedTarget(Service{Name: name}, []string{"Google"}) {
			t.Fatalf("fallback target %q lost per-country prices", name)
		}
	}
	if detailedTarget(Service{Name: "Discord"}, []string{"Google"}) {
		t.Fatal("non-focus target should keep only its global starting price")
	}
}

func TestCountryReadsBoundConcurrencyAndHonorCancellation(t *testing.T) {
	var active, maximum, calls atomic.Int32
	codes := []string{"1", "2", "3", "4", "5", "6"}
	results := readItems(context.Background(), codes, func(code string) (string, error) {
		n := active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		for {
			old := maximum.Load()
			if n <= old || maximum.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		return code, nil
	})
	if calls.Load() != 6 || maximum.Load() > 3 {
		t.Fatal("country worker bound lost")
	}
	for i, r := range results {
		if r.Err != nil || r.Code != codes[i] || r.Value != codes[i] || !r.Started {
			t.Fatal("country results crossed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls.Store(0)
	results = readItems(ctx, codes, func(code string) (string, error) { calls.Add(1); return code, nil })
	if calls.Load() != 0 {
		t.Fatal("cancelled job made upstream requests")
	}
	for _, r := range results {
		if r.Err == nil || r.Started {
			t.Fatal("cancelled country reported complete")
		}
	}
}

func TestOnlineSIMScansEnabledCountriesAndKeepsTrueMinimum(t *testing.T) {
	d := directFixture("onlinesim")
	p := Provider{ID: "onlinesim", Hosts: []string{"onlinesim.io"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("country") == "49" {
			return reply(200, `{"response":"1","country":49,"services":{"go":{"service":"Google","price":"0.05","count":2}},"end":true}`), nil
		}
		if r.URL.Query().Get("country") != "" {
			t.Fatal("disabled country queried")
		}
		return reply(200, `{"response":"1","country":1,"countries":{"_1":{"name":"USA","enable":true},"_49":{"name":"Germany","enable":true},"_33":{"name":"France","enable":false}},"services":{"go":{"service":"Google","price":"0.10","count":2}},"end":true}`), nil
	})}
	if e := collectOnlineSIM(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	_, offers := d.values()
	if len(offers) != 3 || d.CountriesExpected != 2 || d.CountriesRead != 2 {
		t.Fatal("missing country coverage")
	}
	for _, o := range offers {
		if o.Country == "未知" && *o.Amount != "0.05" {
			t.Fatal("global minimum not selected across countries")
		}
	}
}

func TestOnlineSIMCountryMismatchCannotPublishForeignPrices(t *testing.T) {
	d := directFixture("onlinesim")
	p := Provider{ID: "onlinesim", Hosts: []string{"onlinesim.io"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("country") == "49" {
			return reply(200, `{"response":"1","country":1,"services":{"go":{"service":"Google","price":"0.001","count":2}},"end":true}`), nil
		}
		return reply(200, `{"response":"1","country":1,"countries":{"_1":{"name":"USA"},"_49":{"name":"Germany"}},"services":{"go":{"service":"Google","price":"0.10","count":2}},"end":true}`), nil
	})}
	if e := collectOnlineSIM(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	_, offers := d.values()
	if d.CountriesRead != 1 || len(d.warnings) == 0 {
		t.Fatal("partial country scan not disclosed")
	}
	for _, o := range offers {
		if *o.Amount != "0.10" {
			t.Fatal("wrong-country quote published")
		}
	}
}

func TestSMSPVAUsesAllCountryMetadataAndRetainsValidCountry(t *testing.T) {
	d := directFixture("smspva")
	d.source.URL = "https://smspva.com/"
	p := Provider{ID: "smspva", Hosts: []string{"smspva.com"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return reply(200, `<script>window.DATA={allServices:[{"service_opt":"opt1","service_name":"Google"}],allCountries:[{"shortname":"US","title_country":"USA"},{"shortname":"FR","title_country":"France"}]}</script>`), nil
		}
		r.ParseForm()
		if r.Form.Get("country") == "FR" {
			return reply(429, "rate limit"), nil
		}
		if r.Form.Get("method") == "GET_PRICES_BY_COUNTRY" {
			return reply(200, `{"response":{"res":true,"data":{"opt1":{"stock":"0.20","empty":"0.001"}}}}`), nil
		}
		return reply(200, `{"response":{"res":true,"data":{"amount":{"opt1":{"stock":3,"empty":0}}}}}`), nil
	})}
	if e := collectSMSPVA(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	_, offers := d.values()
	if len(offers) != 2 || d.CountriesRead != 1 || d.CountriesExpected != 2 || !strings.Contains(strings.Join(d.warnings, " "), "1/2") {
		t.Fatal("partial country scan lost valid data or warning")
	}
	for _, o := range offers {
		if *o.Amount != "0.20" {
			t.Fatal("out-of-stock country price included")
		}
	}
}

func TestPageJSONFieldRejectsDuplicateKeys(t *testing.T) {
	var data []map[string]string
	if e := pageJSONField([]byte(`allServices:[{"name":"Google","name":"Telegram"}]`), "allServices", &data); e == nil {
		t.Fatal("duplicate metadata keys accepted")
	}
}

func TestSatsScansNonFocusTargetsAndTreatsUnavailableAsComplete(t *testing.T) {
	d := directFixture("sms4sats")
	p := Provider{ID: "sms4sats", APIHosts: []string{"api.sms4sats.com"}}
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Method == "GET" {
			return reply(200, `{"type":"receive-sms","services":[{"code":"go","name":"Google"},{"code":"tg","name":"Telegram"},{"code":"wa","name":"WhatsApp"}]}`), nil
		}
		var body struct {
			Service string `json:"service"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch body.Service {
		case "go":
			return reply(200, `{"type":"receive-sms","service":"go","country":"39","priceSats":1000,"availability":3}`), nil
		case "tg":
			return reply(200, `{"type":"receive-sms","service":"tg","country":"39","priceSats":2000,"availability":3}`), nil
		default:
			return reply(409, `{"error":"UNAVAILABLE"}`), nil
		}
	})}
	if e := collectSMS4Sats(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 3 || len(o) != 2 || requests.Load() != 4 || len(d.warnings) != 0 {
		t.Fatal("non-focus target skipped or no inventory treated as a broken scan")
	}
	if len(d.QuoteAttempts) != 3 {
		t.Fatal("completed and unavailable quote attempts were not retained")
	}
}

func TestQuoteProgressResumesFromSourceStateAndExistingEvidence(t *testing.T) {
	c := &Catalog{PriceUpdate: &PriceUpdate{Sources: []SourceCollection{{Provider: "sms4sats", QuoteAttempts: map[string]string{"tg": "2026-11-06T20:00:00+08:00"}}}}, Offers: []Offer{{Product: "sms4sats-once", Method: "public_api", CheckedAt: "2026-11-06T19:00:00+08:00", Evidence: `{"code":"go"}`}}}
	d := newDirect(c, PriceSource{Provider: "sms4sats", Product: "sms4sats-once"}, Product{ID: "sms4sats-once", Kind: "once"}, time.Now())
	if len(d.QuoteAttempts) != 2 || d.QuoteAttempts["tg"] != "2026-11-06T20:00:00+08:00" || d.QuoteAttempts["go"] != "2026-11-06T19:00:00+08:00" {
		t.Fatal("partial run progress not resumed")
	}
}

func TestMaintainedAliasOverridesLegacyDuplicateWithoutMergingVoice(t *testing.T) {
	c := &Catalog{Services: []Service{{ID: "google", Name: "Google", Aliases: []string{"Google,youtube,Gmail"}}, {ID: "legacy-google", Name: "Google,youtube,Gmail"}, {ID: "voice", Name: "Google Voice"}}}
	d := newDirect(c, PriceSource{Provider: "fixture"}, Product{}, time.Now())
	if d.names[targetNameKey("Google,youtube,Gmail")].ID != "google" || d.names[targetNameKey("Google Voice")].ID != "voice" {
		t.Fatal("known alias shadowed or unrelated product merged")
	}
}

func TestCountryAliasesKeepChannelFiltersComparable(t *testing.T) {
	for _, pair := range [][2]string{{"Britain", "GB"}, {"Unt. Kingdom", "UK"}, {"France ", "FR"}, {"Russian Federation", "RU"}, {"Brazil", "BR"}} {
		if normalizedCountry(pair[0]) != normalizedCountry(pair[1]) {
			t.Fatal("equivalent country names split", pair)
		}
	}
	if normalizedCountry("USA (virtual)") == normalizedCountry("USA") {
		t.Fatal("virtual route distinction removed")
	}
}
