package identity

import (
	"slices"
	"testing"
)

func TestSortOffersRepeatedPricesAndChangedAmounts(t *testing.T) {
	price, equal, smaller := "0.100000000000000002", "0.1000000000000000020", "0.100000000000000001"
	offers := []Offer{
		{ID: "c", Amount: &price},
		{ID: "b", Amount: &equal},
		{ID: "a", Amount: &price},
		{ID: "small", Amount: &smaller},
		{ID: "unknown"},
	}
	check := func(want []string) {
		t.Helper()
		SortOffers(offers, nil)
		for i, id := range want {
			if offers[i].ID != id {
				t.Fatalf("position %d: got %s, want %s", i, offers[i].ID, id)
			}
		}
	}
	check([]string{"small", "a", "b", "c", "unknown"})
	price = "0"
	check([]string{"a", "c", "small", "b", "unknown"})
}

func BenchmarkSortOffers(b *testing.B) {
	c, err := LoadCatalog("data/catalog.json")
	if err != nil {
		b.Fatal(err)
	}
	products := make(map[string]Product, len(c.Products))
	for _, p := range c.Products {
		products[p.ID] = p
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		SortOffers(slices.Clone(c.Offers), products)
	}
}
