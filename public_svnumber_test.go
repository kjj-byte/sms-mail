package identity

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSVNumberRejectsRedirectsBadStatusAndDuplicateJSON(t *testing.T) {
	const secret = "fixture+key&not=a-query#fragment"
	p := Provider{Hosts: []string{"sms-verification-number.com"}}
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"redirect", 302, `[]`},
		{"bad status", 503, `[]`},
		{"duplicate keys", 200, `[{"id":1,"name":"first","name":"second"}]`},
		{"truncated", 200, `[{"id":1`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != p.Hosts[0] || r.URL.Query().Get("api_key") != secret {
					t.Fatal("request escaped the endpoint or key was not encoded")
				}
				response := reply(test.status, test.body)
				response.Header.Set("Location", "https://outside.example/collect")
				return response, nil
			})}
			var dst any
			err := svNumberGet(context.Background(), client, p, secret, "action=getCountryAndOperators", &dst)
			if err == nil || strings.Contains(err.Error(), secret) || calls != 1 {
				t.Fatal("unsafe response accepted or retried", err, calls)
			}
		})
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid method made an HTTP request")
		return nil, nil
	})}
	for _, params := range []string{"action=getNumber", "action=getServicesAndCost&country=1%26action=getNumber", "action=getCountries&api_key=x"} {
		var dst any
		if svNumberGet(context.Background(), client, p, secret, params, &dst) == nil {
			t.Fatal("invalid parameters accepted")
		}
	}
}

func TestSVNumberPreservesPricesAndWarnsOnPartialCountries(t *testing.T) {
	d := directFixture("sv-number")
	p := Provider{Hosts: []string{"sms-verification-number.com"}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("action") == "getCountryAndOperators" {
			return reply(200, `[{"id":1,"name":"United States"},{"id":2,"name":"United Kingdom"}]`), nil
		}
		if r.URL.Query().Get("country") == "2" {
			return reply(503, `[]`), nil
		}
		return reply(200, `[{"id":"go","name":"Google","price":"0.20","quantity":3}]`), nil
	})}
	if err := collectSVNumber(context.Background(), client, p, d, "fixture", nil); err != nil {
		t.Fatal(err)
	}
	_, offers := d.values()
	if len(offers) != 2 || d.CountriesRead != 1 || d.CountriesExpected != 2 || len(d.warnings) != 1 {
		t.Fatal("partial coverage or valid prices lost", d.CountriesRead, len(offers), d.warnings)
	}
}
