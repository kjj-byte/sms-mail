package identity

import (
	"context"
	"encoding/json"
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
func TestUnsafeSourceDoesNotSendRequests(t *testing.T) {
	path := priceCatalog(t)
	before, _ := os.ReadFile(path)
	cfg := PriceConfig{SchemaVersion: 1, Sources: []PriceSource{{Provider: "fivesim", Product: "fivesim-once", URL: "https://5sim.net/prices", Currency: "USD"}, {Provider: "smspool", Product: "smspool-once", URL: "https://www.smspool.net/", Currency: "USD"}}}
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
func TestBodyAndSourceTextLimits(t *testing.T) {
	if _, e := boundedBody(strings.NewReader("12345"), 4); e == nil {
		t.Fatal("oversize accepted")
	}
	text, e := sourceText([]byte(`<script>fake secret</script><p>Google USA USD 0.12 activation</p>`), "text/html")
	if e != nil || strings.Contains(text, "secret") {
		t.Fatal("script text not stripped")
	}
}
