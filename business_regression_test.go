package identity

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestSMSManRejectsMismatchedAndMissingResponseIDs(t *testing.T) {
	for _, ids := range []string{
		`"application_id":"3","country_id":"44"`,
		`"application_id":"9","country_id":"5"`,
		`"application_id":"3"`,
		`"country_id":"5"`,
	} {
		t.Run(ids, func(t *testing.T) {
			d := directFixture("sms-man")
			p := Provider{ID: "sms-man", Hosts: []string{"sms-man.com"}, APIHosts: []string{"api.sms-man.com"}}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/services":
					return reply(200, `[{"id":3,"title":"Telegram","code":"tg"}]`), nil
				case "/api/countries":
					return reply(200, `{"5":{"id":5,"title":"United States","code":"US"},"6":{"id":6,"title":"Malaysia","code":"MY"}}`), nil
				default:
					if r.URL.Query().Get("country_id") == "6" {
						return reply(200, `{"3":{"cost":"0.42","count":10,"application_id":"3","country_id":"6","country":"Malaysia"}}`), nil
					}
					return reply(200, `{"3":{"cost":"0.01","count":10,`+ids+`}}`), nil
				}
			})}
			if err := collectSMSMan(context.Background(), client, p, d, nil, "test-site-key"); err != nil {
				t.Fatal(err)
			}
			_, offers := d.values()
			if d.CountriesRead != 1 || len(d.warnings) != 1 || len(offers) != 2 {
				t.Fatal("invalid country did not produce partial coverage", d.CountriesRead, d.warnings, offers)
			}
			for _, o := range offers {
				if o.Country == "美国" || *o.Amount != "0.42" || !bytes.Contains([]byte(o.Evidence), []byte(`"country":"Malaysia"`)) {
					t.Fatal("mismatched price published or response evidence lost", o)
				}
			}
		})
	}
}

func TestCancelDuringFinalSourceDoesNotCommit(t *testing.T) {
	for _, duringRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "before first source", true: "during final source"}[duringRequest], func(t *testing.T) {
			path := priceCatalog(t)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !duringRequest {
				cancel()
			}
			cfg := PriceConfig{Sources: []PriceSource{{Provider: "smsfast", Product: "smsfast-once", URL: "https://smsfast.com/", Currency: "USD"}}}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				cancel()
				return reply(200, "SMS Verification From $0.21 per activation"), nil
			})}
			info, err := updatePrices(ctx, cfg, path, client, time.Now())
			if err == nil || info.Saved {
				t.Fatal("canceled update committed", info, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("canceled update changed catalog", err)
			}
		})
	}
}
