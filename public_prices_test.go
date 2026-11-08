package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func directFixture(provider string) *directPrices {
	return newDirect(&Catalog{Services: []Service{{ID: "google", Name: "Google", Aliases: []string{"google", "Google/Gmail"}}, {ID: "telegram", Name: "Telegram", Aliases: []string{"telegram"}}}}, PriceSource{Provider: provider, Product: provider + "-once", URL: "https://example.test/", Currency: "USD"}, Product{ID: provider + "-once", Provider: provider, Kind: "once"}, time.Now())
}

func TestFiveSIMPublicPricesUseDecimalsAndSkipOutOfStock(t *testing.T) {
	d := directFixture("fivesim")
	p := Provider{ID: "fivesim", Hosts: []string{"5sim.net"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("analysis key sent to public API")
		}
		switch r.URL.Path {
		case "/v1/guest/products/any/any":
			return reply(200, `{"google":{"Category":"activation","Qty":50,"Price":0.100000000000000001},"facebook":{"Category":"activation","Qty":0,"Price":0},"1day":{"Category":"hosting","Qty":1,"Price":1}}`), nil
		case "/v1/guest/countries":
			return reply(200, `{"usa":{"iso":{"us":1},"text_en":"United States"}}`), nil
		case "/v1/guest/prices":
			return reply(200, `{"google":{"usa":{"stock":{"cost":"0.100000000000000002","count":1},"empty":{"cost":"0.01","count":0}}}}`), nil
		}
		t.Fatal("unexpected public API route", r.URL.Path)
		return nil, nil
	})}
	if e := collectFiveSIM(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	listings, offers := d.values()
	if len(listings) != 2 || len(offers) != 2 {
		t.Fatal("lost products or invented prices")
	}
	for _, o := range offers {
		if o.Country == "美国" && *o.Amount != "0.100000000000000002" {
			t.Fatal("wrong decimal/stock handling")
		}
		if o.Method != "public_api" || o.Kind != "starting" {
			t.Fatal("API minimum mislabelled")
		}
	}
}

func TestLongSMSLongPricesStayExactAndHideOutOfStockCountries(t *testing.T) {
	d := newDirect(&Catalog{}, PriceSource{Provider: "longsms", Product: "longsms-long", URL: "https://longsms.net/api/countries", Currency: "USD"}, Product{ID: "longsms-long", Provider: "longsms", Kind: "rental"}, time.Now())
	p := Provider{ID: "longsms", Hosts: []string{"longsms.net"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
			t.Fatal("secret sent to public LongSMS price API")
		}
		switch r.URL.Path {
		case "/api/countries":
			return reply(200, `{"success":true,"data":[{"id":1,"name":"美国","card_type":"virtual","stock":5,"is_active":true},{"id":2,"name":"加拿大","card_type":"virtual","stock":0,"is_active":true}]}`), nil
		case "/api/pricing":
			if r.URL.Query().Get("product_line") != "long" {
				t.Fatal("wrong product line")
			}
			if r.URL.Query().Get("country_id") == "1" {
				return reply(200, `{"success":true,"data":[{"days":7,"price":"0.550","first_activation_fee":"0","active":true,"product_line":"long","long_project_code":"other"},{"days":30,"price":1.95,"first_activation_fee":1.18,"active":true,"product_line":"long","long_project_code":"full_pack"}]}`), nil
			}
			return reply(200, `{"success":true,"data":[{"days":7,"price":"0.5","first_activation_fee":"0","active":true,"product_line":"long","long_project_code":"other"}]}`), nil
		}
		t.Fatal("unexpected LongSMS route", r.URL.Path)
		return nil, nil
	})}
	if handled, e := collectPublicAPI(context.Background(), client, p, d, nil); !handled || e != nil {
		t.Fatalf("LongSMS rental dispatch handled=%v err=%v", handled, e)
	}
	listings, offers := d.values()
	if len(listings) != 2 || len(offers) != 2 {
		t.Fatalf("longsms listings=%d offers=%d", len(listings), len(offers))
	}
	if d.CountriesExpected != 2 || d.CountriesRead != 2 {
		t.Fatalf("country coverage %d/%d", d.CountriesRead, d.CountriesExpected)
	}
	for _, o := range offers {
		if o.Kind != "exact" || o.Currency != "USD" || o.Country != "美国" || o.Method != "public_api" || o.ServiceID == "" {
			t.Fatalf("bad longsms offer %+v", o)
		}
		if o.Term == "7 days" && (o.Service != "长期项目 other（未映射应用）" || *o.Amount != "0.55" || o.Unit != "每个号码 / 7 天") {
			t.Fatalf("7-day offer lost exact terms: %+v", o)
		}
		if o.Term == "30 days" && (*o.Amount != "3.13" || o.Unit != "每个号码 / 30 天（含首次激活费 1.18 USD）") {
			t.Fatalf("activation fee not included: %+v", o)
		}
	}
}

func TestSMSFastSitewidePriceReplacesManualOffer(t *testing.T) {
	manual := Offer{ID: "smsfast-once-offer", Product: "smsfast-once", Country: "未知", Service: "未知", NumberType: "按官网套餐", Term: "未知", Scope: "按官网套餐", Currency: "USD", Unit: "激活起", Kind: "unknown", Status: "verified", Source: "https://smsfast.com/", Verified: "2026-11-06"}
	c := &Catalog{Offers: []Offer{manual}}
	d := newDirect(c, PriceSource{Provider: "smsfast", Product: "smsfast-once", URL: "https://smsfast.com/", Currency: "USD"}, Product{ID: "smsfast-once", Provider: "smsfast", Kind: "once"}, time.Now())
	raw := []byte(`<html><body><h1>Buy a number</h1><p>From 0.21</p><p>From 5</p></body></html>`)
	text, e := sourceText(raw, "text/html")
	if e != nil {
		t.Fatal(e)
	}
	if m := sitewidePrice.FindStringSubmatch(text); len(m) != 2 || m[1] != "0.21" {
		t.Fatalf("sitewide regex missed %q", text)
	}
	if e := collectPagePrices(raw, "text/html", d); e != nil {
		t.Fatal(e)
	}
	if len(d.offers) != 1 {
		t.Fatalf("sitewide offers = %d", len(d.offers))
	}
	for _, o := range d.offers {
		if o.Amount == nil || *o.Amount != "0.21" || o.Evidence != "From 0.21" || o.Method != "page_text" || o.ServiceID != "" || o.Kind != "starting" {
			t.Fatalf("bad sitewide offer %+v", o)
		}
	}
	if len(c.Offers) != 0 {
		t.Fatal("manual product-level offer was not replaced")
	}
}
func TestSMSPoolPublicStreamMapsServicesAndCountries(t *testing.T) {
	d := directFixture("smspool")
	p := Provider{ID: "smspool", Hosts: []string{"www.smspool.net"}, APIHosts: []string{"api.smspool.net"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("key sent upstream")
		}
		if r.URL.Path == "/service/retrieve_all" {
			return reply(200, `[{"ID":395,"name":"Google/Gmail"},{"ID":907,"name":"Telegram"}]`), nil
		}
		if r.URL.Path == "/request/pricing" {
			return reply(200, `[{"service":395,"service_name":"Google/Gmail","country_name":"United States","short_name":"US","pool":1,"price":"0.20"},{"service":395,"service_name":"Google/Gmail","country_name":"United States","short_name":"US","pool":2,"price":"0.21"},{"service":395,"service_name":"Google/Gmail","country_name":"United Kingdom","short_name":"GB","pool":3,"price":"0.05"}]`), nil
		}
		t.Fatal("unexpected route")
		return nil, nil
	})}
	if e := collectSMSPool(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 2 || len(o) != 3 {
		t.Fatal("missing global/country minima")
	}
	for _, v := range o {
		if v.ServiceID != "google" {
			t.Fatal("numeric service ID guessed as a name")
		}
		if v.Country == "美国" && *v.Amount != "0.20" {
			t.Fatal("wrong pool price")
		}
		if v.Country == "未知" && *v.Amount != "0.05" {
			t.Fatal("wrong global minimum")
		}
	}
	if ValidateURL("https://api.smspool.net/register", p.Hosts) == nil {
		t.Fatal("API hosts accidentally expanded invite whitelist")
	}
}

func TestPageEmbeddedPricesUseRetailCost(t *testing.T) {
	d := directFixture("sms-bus")
	pool := []any{map[string]int{"id": 1, "title": 2, "code": 3, "keywords": 4, "cost": 5, "ws_cost": 6, "total_count": 7}, 1, "Google", "go", "google", json.Number("0.200000000000000001"), json.Number("0.01"), 20}
	raw, _ := json.Marshal(pool)
	body := []byte(`<script type="application/json" id="__NUXT_DATA__">` + string(raw) + `</script>`)
	if e := collectSMSBusState(body, d); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 1 || len(o) != 1 || *o[0].Amount != "0.200000000000000001" || o[0].Method != "page_data" {
		t.Fatal("page price lost or wholesale discount used")
	}
}

func TestPageTextDoesNotCrossJoinServicesAndPrices(t *testing.T) {
	d := directFixture("unknown")
	d.source.Currency = "USD"
	d.target("Google", d.source.URL, "Google")
	d.target("Telegram", d.source.URL, "Telegram")
	raw := []byte(`<div>Google</div><div>Choose a service</div><div>Telegram 120 pcs from $0.12</div><footer>Numbers from $0.01</footer>`)
	if e := collectPagePrices(raw, "text/html", d); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if len(l) != 2 || len(o) != 1 || o[0].ServiceID != "telegram" || *o[0].Amount != "0.12" {
		t.Fatal("page minimum applied to unrelated service")
	}
}

func TestPageCountryPricesAreNotServicePrices(t *testing.T) {
	d := directFixture("unknown")
	d.catalog.Services = append(d.catalog.Services, Service{ID: "country-shaped-app", Name: "France"})
	if e := collectPagePrices([]byte(`<a href="/country/france">France from $0.10</a>`), "text/html", d); e != nil {
		t.Fatal(e)
	}
	_, offers := d.values()
	if len(offers) != 0 {
		t.Fatal("country from-price assigned to an unrelated service")
	}
}

func TestTruncatedAPIPriceStreamIsNotPublished(t *testing.T) {
	d := directFixture("smspool")
	d.source.URL = "https://www.smspool.net/"
	p := Provider{ID: "smspool", Hosts: []string{"www.smspool.net"}, APIHosts: []string{"api.smspool.net"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/service/retrieve_all":
			return reply(200, `[{"ID":395,"name":"Google/Gmail"}]`), nil
		case "/request/pricing":
			return reply(200, `[{"service":395,"service_name":"Google/Gmail","country_name":"USA","short_name":"US","pool":1,"price":"0.01"},`), nil
		default:
			return reply(200, "<p>No public prices</p>"), nil
		}
	})}
	l, o, w, e := collectPriceSource(context.Background(), client, p, d, PriceConfig{})
	if e != nil || len(l) != 1 || len(o) != 0 || len(w) == 0 {
		t.Fatal("partial price response was accepted or valid metadata was lost")
	}
	if !strings.Contains(strings.Join(w, " "), "未完整") {
		t.Fatal("stream failure not explained")
	}
}

func TestOnlineSIMPagesAndStock(t *testing.T) {
	d := directFixture("onlinesim")
	p := Provider{ID: "onlinesim", Hosts: []string{"onlinesim.io"}}
	pages := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		pages++
		if r.URL.Query().Get("page") == "1" {
			return reply(200, `{"response":"1","country":1,"countries":{"_1":{"name":"USA"}},"services":{"go":{"service":"Google","price":"0.10","count":2},"tg":{"service":"Telegram","price":"0.01","count":0}},"end":false}`), nil
		}
		return reply(200, `{"response":"1","country":1,"countries":{"_1":{"name":"USA"}},"services":{},"end":true}`), nil
	})}
	if e := collectOnlineSIM(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	l, o := d.values()
	if pages != 2 || len(l) != 2 || len(o) != 2 || o[0].ServiceID != "google" {
		t.Fatal("pagination, country or stock lost")
	}
}

func TestSMSAKMapsServiceCodesAndCountries(t *testing.T) {
	d := directFixture("smsak")
	d.source.Currency = "RUB"
	p := Provider{ID: "smsak", Hosts: []string{"sms-acktiwator.ru"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Query().Get("action") {
		case "getServicesList":
			return reply(200, `{"status":"success","services":[{"code":"go","name":"Google"}]}`), nil
		case "getCountries":
			return reply(200, `{"1":{"eng":"USA"}}`), nil
		default:
			return reply(200, `{"1":{"go":{"cost":"0.20","count":1},"unknown":{"cost":"0.01","count":1}}}`), nil
		}
	})}
	if e := collectSMSAK(context.Background(), client, p, d, []string{"Google"}); e != nil {
		t.Fatal(e)
	}
	_, o := d.values()
	if len(o) != 2 {
		t.Fatal("missing global or country price")
	}
	for _, v := range o {
		if v.ServiceID != "google" || *v.Amount != "0.20" || v.Currency != "RUB" {
			t.Fatal("incorrect code or currency mapping")
		}
	}
}

func TestSMSManPublicPricesFromSiteKey(t *testing.T) {
	d := directFixture("sms-man")
	p := Provider{ID: "sms-man", Hosts: []string{"sms-man.com"}, APIHosts: []string{"api.sms-man.com"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		// Catalogue endpoints use api_key; price endpoint uses token. Both must be the public site key.
		key := q.Get("api_key")
		if q.Get("token") != "" {
			key = q.Get("token")
		}
		if key != smsmanPublicKey {
			t.Fatal("wrong public key sent")
		}
		if r.URL.Query().Get("currency") != "USD" && strings.HasPrefix(r.URL.Path, "/control/get-prices") {
			t.Fatal("price call not forced to USD")
		}
		switch r.URL.Path {
		case "/api/services":
			return reply(200, `[{"id":3,"title":"Telegram","code":"tg"},{"id":5,"title":"Instagram","code":"ig"}]`), nil
		case "/api/countries":
			return reply(200, `{"5":{"id":5,"title":"United States/USA","code":"US"},"6":{"id":6,"title":"Malaysia","code":"MY"}}`), nil
		case "/control/get-prices":
			cid := q.Get("country_id")
			if cid == "5" {
				return reply(200, `{"3":{"cost":"1.80","count":333243,"application_id":"3","application":"Telegram","country_id":"5","country":"United States"}}`), nil
			}
			return reply(200, `{"3":{"cost":"0.42","count":322847,"application_id":"3","application":"Telegram","country_id":"6","country":"Malaysia"},"5":{"cost":"0.10","count":0,"application_id":"5","application":"Instagram","country_id":"6","country":"Malaysia"}}`), nil
		}
		t.Fatal("unexpected sms-man route", r.URL.Path)
		return nil, nil
	})}
	if e := collectSMSMan(context.Background(), client, p, d, []string{"Telegram"}); e != nil {
		t.Fatal(e)
	}
	_, o := d.values()
	if len(o) == 0 {
		t.Fatal("no sms-man prices collected")
	}
	var usTelegram, myTelegram bool
	for _, v := range o {
		if v.Currency != "USD" || v.Method != "public_api" {
			t.Fatal("wrong currency or method")
		}
		if v.ServiceID == "telegram" && v.Country == "美国" && *v.Amount == "1.80" {
			usTelegram = true
		}
		if v.ServiceID == "telegram" && v.Country == "马来西亚" && *v.Amount == "0.42" {
			myTelegram = true
		}
	}
	if !usTelegram || !myTelegram {
		t.Fatal("country-specific Telegram prices missing", len(o))
	}
	// Out-of-stock Instagram (count 0) must be skipped.
	for _, v := range o {
		if v.ServiceID == "instagram" {
			t.Fatal("out-of-stock price was published")
		}
	}
}
