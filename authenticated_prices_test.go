package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticatedPricesMapMetadataAndNeverPublishKey(t *testing.T) {
	for _, id := range []string{"tiger-sms", "grizzlysms", "sms-man"} {
		t.Run(id, func(t *testing.T) {
			d := directFixture(id)
			host := "api." + id + ".com"
			p := Provider{ID: id, APIHosts: []string{host}}
			key := "fixture-upstream-secret"
			cfg := PriceConfig{Credentials: map[string]ProviderCredential{id: {APIKey: key, Currency: "USD"}}, PriceTargets: []string{"Google"}}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				q := r.URL.Query()
				method := q.Get("action")
				credential := q.Get("api_key")
				if id == "sms-man" {
					method = strings.TrimPrefix(r.URL.Path, "/control/")
					credential = q.Get("token")
				}
				if r.URL.Host != host || credential != key || r.Header.Get("Authorization") != "" {
					t.Fatal("wrong auth host or key")
				}
				switch method {
				case "getServicesList":
					return reply(200, `{"status":"success","services":[{"code":"go","name":"Google"}]}`), nil
				case "applications":
					return reply(200, `[{"id":"go","name":"Google"}]`), nil
				case "getCountries", "countries":
					return reply(200, `[{"id":187,"eng":"USA","title":"USA"}]`), nil
				case "getPrices", "get-prices":
					return reply(200, `{"187":{"go":{"cost":"0.100000000000000001","count":2},"unknown":{"cost":"0.01","count":2}}}`), nil
				default:
					t.Fatal("non-read-only method", method)
					return nil, nil
				}
			})}
			// SMS-Man uses numeric application IDs, never borrowed service shortcodes.
			if id == "sms-man" {
				client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Query().Get("token") != key {
						t.Fatal("missing token")
					}
					switch r.URL.Path {
					case "/control/applications":
						return reply(200, `[{"id":"12","name":"Google","code":"go"}]`), nil
					case "/control/countries":
						return reply(200, `[{"id":187,"title":"USA"}]`), nil
					case "/control/get-prices":
						return reply(200, `{"187":{"12":{"cost":"0.100000000000000001","count":2}}}`), nil
					default:
						t.Fatal("wrong SMS-Man route")
						return nil, nil
					}
				})
			}
			handled, e := collectAuthenticatedPrices(context.Background(), client, p, d, cfg)
			if e != nil || !handled {
				t.Fatal(e)
			}
			l, o := d.values()
			if calls != 3 || len(l) != 1 || len(o) != 2 {
				t.Fatal("missing mapped quote")
			}
			for _, v := range o {
				if v.Method != "authenticated_api" || *v.Amount != "0.100000000000000001" {
					t.Fatal("incorrect quote")
				}
			}
			b, _ := json.Marshal([]any{l, o})
			if strings.Contains(string(b), key) || strings.Contains(string(b), "api_key=") || strings.Contains(string(b), "token=") {
				t.Fatal("credential published")
			}
		})
	}
}

func TestAuthenticatedPricesBlockRedirectsAndWriteMethods(t *testing.T) {
	p := Provider{ID: "tiger-sms", APIHosts: []string{"api.tiger-sms.com"}}
	endpoint := "https://api.tiger-sms.com/stubs/handler_api.php"
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		res := reply(302, "")
		res.Header.Set("Location", "https://api.tiger-sms.com/leak")
		return res, nil
	})}
	var raw any
	e := authenticatedJSON(context.Background(), client, p, endpoint, "fixture-secret", url.Values{"action": {"getPrices"}}, &raw)
	if e == nil || calls != 1 || strings.Contains(e.Error(), "fixture-secret") {
		t.Fatal("redirect leaked credentials")
	}
	e = authenticatedJSON(context.Background(), client, p, endpoint, "fixture-secret", url.Values{"action": {"getNumber"}}, &raw)
	if e == nil || calls != 1 {
		t.Fatal("purchase method allowed")
	}
}

func TestProviderKeyFileValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-keys.json")
	for _, body := range []string{`{"schema_version":1,"providers":{"sms-man":{"api_key":"x","currency":""}}}`, `{"schema_version":1,"providers":{"unknown":{"api_key":"","currency":""}}}`, `{"schema_version":1,"providers":{"sms-man":{"api_key":"x\n","currency":"USD"}}}`} {
		os.WriteFile(path, []byte(body), 0600)
		if _, e := loadProviderKeys(path); e == nil {
			t.Fatal("invalid credential config accepted")
		}
	}
	os.WriteFile(path, []byte(`{"schema_version":1,"providers":{"sms-man":{"api_key":"","currency":""}}}`), 0600)
	if _, e := loadProviderKeys(path); e != nil {
		t.Fatal(e)
	}
}

func TestPriceConfigLoadsSeparateCredentialsRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keys.json"), []byte(`{"schema_version":1,"providers":{"sms-man":{"api_key":"fixture-platform-key","currency":"RUB"}}}`), 0600)
	cfg := PriceConfig{SchemaVersion: 1, ProviderKeys: "keys.json", Sources: []PriceSource{{Provider: "sms-man", URL: "https://sms-man.com/"}}}
	raw, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "prices.json")
	os.WriteFile(path, raw, 0600)
	loaded, e := LoadPriceConfig(path)
	if e != nil || loaded.Credentials["sms-man"].APIKey != "fixture-platform-key" {
		t.Fatal("separate credential file not loaded", e)
	}
	encoded, _ := json.Marshal(loaded)
	if strings.Contains(string(encoded), "fixture-platform-key") {
		t.Fatal("loaded keys serialized into price configuration")
	}
}
