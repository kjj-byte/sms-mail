package identity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdatePricesMergesWebSnapshotBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	catalog := Catalog{SchemaVersion: 1, Providers: []Provider{{ID: "longsms", Name: "LongSMS", URL: "https://longsms.net/", Hosts: []string{"longsms.net"}, Sources: []string{"https://longsms.net/"}, Verified: "2026-11-07"}}}
	b, _ := json.Marshal(catalog)
	if e := os.WriteFile(catalogPath, b, 0600); e != nil {
		t.Fatal(e)
	}
	snapshotPath := filepath.Join(dir, "web-prices.json")
	snapshot := WebPriceSnapshot{SchemaVersion: 1, CapturedAt: time.Now().Format(time.RFC3339), Offers: []WebPriceOffer{{Provider: "longsms", Product: "longsms-short", Country: "美国", Service: "Telegram", Term: "20 分钟", Scope: "service_code=tg", Amount: "0.20", Currency: "USD", Unit: "每次激活", Source: "https://longsms.net/api/pricing?product_line=short&service_code=tg&country_hint_id=1"}}}
	b, _ = json.Marshal(snapshot)
	if e := os.WriteFile(snapshotPath, b, 0600); e != nil {
		t.Fatal(e)
	}
	cfg := PriceConfig{SchemaVersion: 1, WebPrices: snapshotPath}
	info, e := updatePrices(context.Background(), cfg, catalogPath, nil, time.Now())
	if e != nil || !info.Saved || info.UpdatedOffers != 1 || info.UpdatedServices != 1 {
		t.Fatalf("info=%+v err=%v", info, e)
	}
	loaded, e := LoadCatalog(catalogPath)
	if e != nil {
		t.Fatal(e)
	}
	if len(loaded.Products) != 1 || len(loaded.Offers) != 1 || len(loaded.ServiceListings) != 1 {
		t.Fatalf("products=%d offers=%d listings=%d", len(loaded.Products), len(loaded.Offers), len(loaded.ServiceListings))
	}
}

func TestMergeWebPricesCreatesProductsAndReplacesOnlyWebOffers(t *testing.T) {
	c := &Catalog{SchemaVersion: 1, Providers: []Provider{
		{ID: "longsms", Name: "LongSMS", URL: "https://longsms.net/", Hosts: []string{"longsms.net"}, Sources: []string{"https://longsms.net/"}, Verified: "2026-11-07"},
		{ID: "haozhuma", Name: "豪猪", URL: "https://my.haozhuma.com/", Hosts: []string{"my.haozhuma.com", "h5.haozhuma.com"}, Sources: []string{"https://my.haozhuma.com/"}, Verified: "2026-11-07"},
	}}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	s := WebPriceSnapshot{SchemaVersion: 1, CapturedAt: now.Format(time.RFC3339), Offers: []WebPriceOffer{
		{Provider: "longsms", Product: "longsms-short", Country: "美国", Service: "Telegram", NumberType: "虚拟号", Term: "20 分钟", Scope: "service_code=tg", Amount: "0.20", Currency: "USD", Unit: "每次激活", Source: "https://longsms.net/api/pricing?product_line=short&service_code=tg&country_hint_id=1", Evidence: `{"service":"Telegram"}`},
		{Provider: "haozhuma", Product: "haozhuma-once", Country: "美国", Service: "Google", Term: "single activation", Scope: "project_id=1", Amount: "0.50", Currency: "CNY", Unit: "每次激活", Source: "https://h5.haozhuma.com/", Evidence: `{"project":"Google"}`},
		{Provider: "haozhuma", Product: "haozhuma-once", Country: "美国", Service: "Bad", Term: "single activation", Scope: "project_id=2", Amount: "0.10", Currency: "CNY", Unit: "每次激活", Source: "https://evil.example/", Evidence: `{"project":"Bad"}`},
	}}
	offers, listings, warnings, e := mergeWebPrices(c, s, now)
	if e != nil {
		t.Fatal(e)
	}
	if offers != 2 || listings != 2 || len(warnings) != 1 {
		t.Fatalf("offers=%d listings=%d warnings=%v", offers, listings, warnings)
	}
	if len(c.Products) != 2 || len(c.Offers) != 2 || len(c.ServiceListings) != 2 {
		t.Fatalf("products=%d offers=%d listings=%d", len(c.Products), len(c.Offers), len(c.ServiceListings))
	}
	for _, o := range c.Offers {
		if o.Method != "web_api" || o.Status != "verified" || o.Kind != "exact" || o.ServiceID == "" {
			t.Fatalf("bad web offer %+v", o)
		}
		if o.Product == "longsms-short" && (o.Amount == nil || *o.Amount != "0.20" || o.Term != "20 分钟") {
			t.Fatalf("LongSMS short offer %+v", o)
		}
	}
	// A second snapshot without offers must not delete the existing web rows.
	if _, _, _, e := mergeWebPrices(c, WebPriceSnapshot{SchemaVersion: 1, CapturedAt: now.Format(time.RFC3339)}, now); e != nil {
		t.Fatal(e)
	}
	if len(c.Offers) != 2 {
		t.Fatal("empty snapshot deleted valid web offers")
	}
	bad := &Catalog{SchemaVersion: 1, Providers: c.Providers}
	_, _, _, e = mergeWebPrices(bad, WebPriceSnapshot{SchemaVersion: 1, CapturedAt: now.Format(time.RFC3339), Offers: []WebPriceOffer{{Provider: "haozhuma", Product: "haozhuma-once", Country: "美国", Service: "Bad", Term: "single activation", Amount: "0.10", Currency: "CNY", Unit: "每次激活", Source: "https://evil.example/"}}}, now)
	if e != nil || len(bad.Products) != 0 || len(bad.Offers) != 0 {
		t.Fatalf("invalid-only snapshot left products: products=%d offers=%d err=%v", len(bad.Products), len(bad.Offers), e)
	}
}
