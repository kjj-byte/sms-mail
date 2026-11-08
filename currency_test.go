package identity

import "testing"

func TestToUSD(t *testing.T) {
	cases := []struct {
		amount, cur, want string
		ok                bool
	}{
		{"1.50", "USD", "1.50", true},
		{"1.50", "$", "1.50", true},
		{"100000000", "sats", "75854.000000", true}, // exactly 1 BTC
		{"1000", "sats", "0.758540", true},
		{"6.6713", "CNY", "1.000028", true},
		{"5", "RUB", "", false},
		{"bad", "USD", "", false},
	}
	for _, c := range cases {
		got, ok := toUSD(c.amount, c.cur)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("toUSD(%q,%q)=(%q,%v) want (%q,%v)", c.amount, c.cur, got, ok, c.want, c.ok)
		}
	}
}

func TestUnifyCurrencyKeepsOriginalInEvidence(t *testing.T) {
	amt := "1000"
	c := &Catalog{Offers: []Offer{{ID: "x", Amount: &amt, Currency: "sats", Evidence: "orig"}}}
	unifyCurrency(c)
	o := c.Offers[0]
	if o.Currency != "USD" || *o.Amount != "0.758540" {
		t.Fatalf("not converted: %s %s", *o.Amount, o.Currency)
	}
	if !contains(o.Evidence, "原价 1000 sats") || !contains(o.Evidence, "orig") {
		t.Fatalf("evidence lost original: %q", o.Evidence)
	}
}

func TestUnifyCurrencyPrefersDollarOverConverted(t *testing.T) {
	usd, cny := "0.21", "1.50"
	c := &Catalog{Offers: []Offer{
		{ID: "converted", Amount: &cny, Currency: "CNY"},
		{ID: "dollar", Amount: &usd, Currency: "$"},
	}}
	unifyCurrency(c)
	mine, other := c.Offers[0], c.Offers[1]
	if mine.Currency != "USD" || other.Currency != "USD" {
		t.Fatalf("not unified: %s %s", mine.Currency, other.Currency)
	}
	// The page's own dollar price must not be reset or give way to a conversion.
	if *other.Amount != "0.21" || other.Evidence != "" {
		t.Fatalf("dollar price was re-converted: %s %q", *other.Amount, other.Evidence)
	}
	// Only one currency remains, and native dollar sorts ahead of converted.
	SortOffers(c.Offers, map[string]Product{})
	if c.Offers[0].ID != "dollar" {
		t.Fatalf("native dollar should sort first, got %s", c.Offers[0].ID)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
