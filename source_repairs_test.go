package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHeroUsesAvailablePriceAndExcludesRentAndCountries(t *testing.T) {
	d := directFixture("herosms")
	pool := []any{map[string]int{"directoryName": 1, "service": 2, "priceMinAvailable": 3, "priceMinDefault": 4, "count": 5}, "Google", "go", json.Number("0.006"), json.Number("0.02"), 100, map[string]int{"directoryName": 7, "service": 8, "priceMinAvailable": 3, "count": 5}, "Full rent", "full", map[string]int{"short_name": 10, "img": 11}, "Brazil", "https://cdn.hero-sms.com/assets/img/country/73.svg"}
	b, _ := json.Marshal(pool)
	if e := collectHeroState([]byte(`<script id="__NUXT_DATA__">`+string(b)+`</script>`), d); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 1 || len(o) != 1 || *o[0].Amount != "0.006" {
		t.Fatal("wrong price or rental/country classified as target")
	}
}

func TestGrizzlyKeepsCountryAndRejectsConflictingRenderedPrice(t *testing.T) {
	d := directFixture("grizzlysms")
	card := `<div class="service-card"><span class="--service">Google</span><span class="--country">USA</span><button data-info-price-country-service="go_12_0.0500">$ 0.05</button><a href="/google">Google USA</a></div>`
	collectGrizzlyCards([]byte(card+strings.ReplaceAll(card, "$ 0.05", "$ 0.01")), d)
	l, o := d.values()
	if len(l) != 1 || len(o) != 1 || o[0].Country != "美国" || *o[0].Amount != "0.0500" {
		t.Fatal("card fields crossed or displayed price mismatch accepted")
	}
}

func TestTextverifiedOnlyFetchesMatchedTargetAndSMSPrice(t *testing.T) {
	d := directFixture("textverified")
	d.target("Google", d.source.URL, "Google")
	p := Provider{ID: "textverified", Hosts: []string{"www.textverified.com"}, APIHosts: []string{"app.textverified.com"}}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/services/google" {
			t.Fatal("unrequested target fetched")
		}
		return reply(200, `<h2>Google</h2><h3>SMS Verification</h3><h2>$0.75</h2><span>per verification</span><p>Numbers are US-based non-VoIP.</p><h3>Voice Verification</h3><p>$0.01 per verification</p>`), nil
	})}
	collectTextverifiedPages(context.Background(), client, p, []byte(`<a href="https://app.textverified.com/services/google">Google</a><a href="https://app.textverified.com/services/telegram">Telegram</a>`), d, []string{"Google"})
	_, o := d.values()
	if calls != 1 || len(o) != 1 || *o[0].Amount != "0.75" || o[0].Country != "美国" {
		t.Fatal("SMS and voice/rental prices mixed")
	}
}

func TestSatsV2QuoteDoesNotCreateOrderOrConvertCurrency(t *testing.T) {
	d := directFixture("sms4sats")
	p := Provider{ID: "sms4sats", Hosts: []string{"sms4sats.com"}, APIHosts: []string{"api.sms4sats.com"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credential leak")
		}
		switch r.URL.Path {
		case "/v2/receive-sms/catalog":
			return reply(200, `{"type":"receive-sms","services":[{"code":"go","name":"Google"},{"code":"tg","name":"Telegram"}]}`), nil
		case "/v2/receive-sms/quote":
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
				t.Fatal("quote format")
			}
			return reply(200, `{"type":"receive-sms","service":"go","country":"39","countryName":null,"availability":3,"priceSats":1000}`), nil
		default:
			t.Fatal("non-read-only endpoint called")
			return nil, nil
		}
	})}
	if e := collectSMS4Sats(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 2 || len(o) != 1 || o[0].Currency != "sats" || *o[0].Amount != "1000" || o[0].Country != "未知" {
		t.Fatal("lost catalog, converted sats, or guessed allocated country")
	}
}

func TestSMSPVAPairsOperatorPriceWithStock(t *testing.T) {
	d := directFixture("smspva")
	d.source.URL = "https://smspva.com/"
	p := Provider{ID: "smspva", Hosts: []string{"smspva.com"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return reply(200, `<script>window.DATA={allServices:[{"service_opt":"opt1","service_name":"Google"}],allCountries:[{"shortname":"US","title_country":"USA"}],minPrices:{"US":"0.001"}}</script>`), nil
		}
		r.ParseForm()
		switch r.Form.Get("method") {
		case "GET_PRICES_BY_COUNTRY":
			return reply(200, `{"response":{"res":true,"data":{"opt1":{"empty":"0.001","stock":"0.20"}}}}`), nil
		case "GET_AMOUNT_BY_COUNTRY":
			return reply(200, `{"response":{"res":true,"data":{"amount":{"opt1":{"empty":0,"stock":5}}}}}`), nil
		default:
			t.Fatal("wrong guest method")
			return nil, nil
		}
	})}
	if e := collectSMSPVA(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	_, o := d.values()
	if len(o) != 2 || *o[0].Amount != "0.20" {
		t.Fatal("country or unavailable operator minimum used")
	}
}

func TestSourceFailurePreservesHTTPReason(t *testing.T) {
	d := directFixture("unknown")
	d.source.URL = "https://example.test/"
	p := Provider{ID: "unknown", Hosts: []string{"example.test"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return reply(403, "challenge"), nil })}
	_, _, _, e := collectPriceSource(context.Background(), client, p, d, PriceConfig{})
	if e == nil || !strings.Contains(e.Error(), "HTTP 403") {
		t.Fatal("specific HTTP error hidden")
	}
}
