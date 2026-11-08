package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePreservesOfferKindsConditionsAndOriginalPrices(t *testing.T) {
	c, err := LoadCatalog("data/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.Products {
		if p.ID == "fivesim-once" {
			c.Products = []Product{p}
			break
		}
	}
	for _, p := range c.Providers {
		if p.ID == "fivesim" {
			c.Providers = []Provider{p}
			break
		}
	}
	c.Services = []Service{{ID: "google", Name: "Google"}}
	c.ServiceListings, c.PriceUpdate, c.Candidates = nil, nil, nil
	c.Offers = nil
	amount, renewal := "1000", "2000"
	for _, kind := range []string{"exact", "starting", "unknown", "login"} {
		o := Offer{ID: kind, Product: "fivesim-once", ServiceID: "google", Service: "Google", Country: "美国", NumberType: "mobile", Scope: "one service", Term: "single activation", Unit: "每次激活", Currency: "sats", Kind: kind, Status: "verified", Source: "https://5sim.net/prices", Verified: "2026-11-06", CheckedAt: "2026-11-06T12:00:00+08:00", Method: "public_api"}
		if kind == "exact" || kind == "starting" {
			o.Amount, o.RenewalAmount = &amount, &renewal
		}
		c.Offers = append(c.Offers, o)
	}
	dir := t.TempDir()
	path, out := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "dist")
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Generate(path, "", out); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	_, data, found := strings.Cut(string(page), `id="rows-data">`)
	if !found {
		t.Fatal("missing rows")
	}
	data, _, _ = strings.Cut(data, "</script>")
	var payload struct {
		Products map[string]prodJS
		Pool     []string
		Rows     []rowJS
		RateDate string `json:"rate_date"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Rows) != 4 || payload.RateDate != rateTimestamp {
		t.Fatal("missing rows or rate date")
	}
	seen := map[string]bool{}
	for _, r := range payload.Rows {
		seen[r.Ok] = true
		if payload.Pool[r.T] != "single activation" || payload.Pool[r.N] != "mobile" || payload.Pool[r.Sc] != "one service" || payload.Pool[r.Os] != "https://5sim.net/prices" || payload.Pool[r.Me] != "官方公开 API" || payload.Pool[r.Ca] != "2026-11-06T12:00:00+08:00" || payload.Pool[r.Ov] != "2026-11-06" || payload.Pool[r.H] != c.Products[0].URL {
			t.Fatal("pooled fields changed", r)
		}
		if r.Ok == "exact" || r.Ok == "starting" {
			if r.A != "0.75854" || r.Cu != "USD" || r.Oa != "1000" || r.Oc != "sats" || r.Ora != "2000" || r.Ra != "1.51708" {
				t.Fatal("original prices lost", r)
			}
		} else if r.A != "" || r.Oc != "" {
			t.Fatal("unpriced row acquired a price", r)
		}
	}
	if len(seen) != 4 || payload.Products["fivesim-once"].D != c.Products[0].Duration {
		t.Fatal("offer kinds or product fields lost")
	}
}

func TestSortOffersKeepsComparisonDimensionsSeparate(t *testing.T) {
	low, high := "0.01", "9.00"
	base := Offer{ID: "base", Product: "p", Currency: "USD", Unit: "activation", Term: "single activation", Kind: "exact", NumberType: "mobile", Scope: "one service", Amount: &high}
	for _, change := range []func(*Offer){
		func(o *Offer) { o.Currency = "ZZZ" }, func(o *Offer) { o.Unit = "z" }, func(o *Offer) { o.Term = "z" },
		func(o *Offer) { o.Kind = "starting" }, func(o *Offer) { o.NumberType = "voip" }, func(o *Offer) { o.Scope = "z" },
	} {
		other := base
		other.ID = "other"
		other.Amount = &low
		change(&other)
		offers := []Offer{other, base}
		SortOffers(offers, map[string]Product{"p": {Kind: "once"}})
		if offers[0].ID != "base" {
			t.Fatal("different conditions ranked by amount", offers)
		}
	}
}
