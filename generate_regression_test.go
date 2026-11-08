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
	if strings.Contains(string(page), `id="rows-data"`) {
		t.Fatal("rows still inlined into index.html; expected separate rows.json")
	}
	data, err := os.ReadFile(filepath.Join(out, "rows.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Products map[string]prodJS
		Pool     []string
		Rows     []rowJS
		RateDate string `json:"rate_date"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
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

func TestNoPublicDeposit(t *testing.T) {
	// Hidden: only a login/unverified placeholder, no concrete amount → card must
	// not show the "需登录查看" wording.
	for _, note := range []string{
		"最低充值金额需登录查看", "最低充值以官网充值页为准（需登录）",
		"最低充值金额未公开核实；登录后在 Balance → Refill 按支付方式查看限额",
		"最低充值金额需登录查看（官网无公开充值说明）",
		"最低充值按所选支付方式显示（仅支持加密货币）",
		"最低充值金额需登录查看（官网说明取决于所选支付方式）",
	} {
		if !noPublicDeposit(note) {
			t.Errorf("expected hidden: %q", note)
		}
	}
	// Shown: a concrete amount, free numbers, or pay-per-order stay verbatim.
	for _, note := range []string{
		"最低充值 3 USD（快捷档 5/10/20/50/100/200）", "最低充值 2 USDT",
		"AnyPay/SBP 渠道官网明示最低 1000 RUB", "尼日利亚银行卡渠道官网明示最低 5 USD",
		"公开预设最低 $25 起（官网未标为最低）", "免费公共号码，无需充值",
		"无账户余额，按单支付（闪电网络）",
	} {
		if noPublicDeposit(note) {
			t.Errorf("expected shown: %q", note)
		}
	}
}
