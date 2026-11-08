package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTargetDiscoveryIncludesObjectsWithoutPrices(t *testing.T) {
	path := priceCatalog(t)
	cfg := testPriceConfig()
	cfg.Sources = cfg.Sources[:1]
	now := time.Now().Truncate(time.Second)
	body := "Supported SMS targets: Amazon Discord TikTok."
	response := PriceExtraction{Services: []ExtractedService{{Name: "Amazon", Evidence: body}, {Name: "Discord", Evidence: body}, {Name: "TikTok", Evidence: body}}, Offers: []ExtractedPrice{}}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "analysis.example" {
			return chatReply(response), nil
		}
		return reply(200, "<p>"+body+"</p>"), nil
	})}
	info, e := updatePrices(context.Background(), cfg, path, client, now)
	if e != nil {
		t.Fatal(e)
	}
	if info.UpdatedServices != 3 || info.UpdatedOffers != 0 || len(info.Failures) != 0 || len(info.PriceWarnings) != 1 {
		t.Fatal("object-only discovery counted as source failure", info)
	}
	c, e := LoadCatalog(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(c.ServiceListings) != 3 {
		t.Fatal("target listings not persisted")
	}
	out := filepath.Join(t.TempDir(), "dist")
	if e = Generate(path, "", out); e != nil {
		t.Fatal(e)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	rendered := string(page)
	_, data, found := strings.Cut(rendered, `id="obj-data">`)
	if !found {
		t.Fatal("target data missing")
	}
	data, _, _ = strings.Cut(data, "</script>")
	var targets []TargetOption
	if err := json.Unmarshal([]byte(data), &targets); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Amazon", "Discord", "TikTok"} {
		if !slices.Contains(targets, TargetOption{Name: name, Channels: 1}) {
			t.Fatal("discovered target missing from client data", name)
		}
		if strings.Contains(rendered, `<option value="`+name+`">`) {
			t.Fatal("target options should mount on demand", name)
		}
	}
	if !strings.Contains(rendered, `"lf":true`) {
		t.Fatal("unpriced target listing rows missing from data")
	}
	originalID := c.ServiceListings[0].ServiceID
	failed := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return reply(403, "blocked"), nil })}
	if _, e = updatePrices(context.Background(), cfg, path, failed, now.Add(24*time.Hour)); e == nil {
		t.Fatal("source failure reported as fresh result")
	}
	c, _ = LoadCatalog(path)
	if len(c.ServiceListings) != 3 || c.ServiceListings[0].ServiceID != originalID || c.ServiceListings[0].CheckedAt != now.Format(time.RFC3339) || c.ServiceListings[0].Status != "unavailable" {
		t.Fatal("failure cleared targets or refreshed timestamps")
	}
}

func TestTargetNamesHaveEvidenceAndStableIDs(t *testing.T) {
	c := Catalog{Services: []Service{{ID: "google", Name: "Google", Aliases: []string{"Gmail"}}}}
	a, e := resolveTarget(&c, "Discord", "Supports Discord SMS", "Supports Discord SMS")
	if e != nil {
		t.Fatal(e)
	}
	b, e := resolveTarget(&c, "discord", "Supports discord SMS", "Supports discord SMS")
	if e != nil || b.ID != a.ID {
		t.Fatal("target ID unstable across case variants")
	}
	google, e := resolveTarget(&c, "Gmail", "Supports Gmail SMS", "Supports Gmail SMS")
	if e != nil || google.ID != "google" {
		t.Fatal("known exact alias mapping lost")
	}
	c.Services[0].Aliases = append(c.Services[0].Aliases, "Google/YouTube")
	spaced, e := resolveTarget(&c, "Google / YouTube", "Google / YouTube SMS", "Google / YouTube SMS")
	if e != nil || spaced.ID != "google" {
		t.Fatal("slash formatting split an explicitly known alias")
	}
	for _, entry := range []struct{ name, evidence, text string }{{"Facebook", "Supports Discord SMS", "Supports Discord SMS"}, {"Discord", "invented Discord", "Supports Discord SMS"}, {"Google", "GoogleAnalytics SMS", "GoogleAnalytics SMS"}} {
		if _, e := resolveTarget(&c, entry.name, entry.evidence, entry.text); e == nil {
			t.Fatal("invented or partial name accepted")
		}
	}
}

func TestProductlessTargetSourceCannotInventProductQuote(t *testing.T) {
	path := priceCatalog(t)
	cfg := testPriceConfig()
	cfg.Sources = []PriceSource{{Provider: "longsms", URL: "https://longsms.net/app"}}
	text := "Supported SMS apps: Discord, Amazon."
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "analysis.example" {
			return chatReply(PriceExtraction{Services: []ExtractedService{{"Discord", text}}, Offers: []ExtractedPrice{extraction("0.1", "USA")}}), nil
		}
		return reply(200, text), nil
	})}
	info, e := updatePrices(context.Background(), cfg, path, client, time.Now())
	if e != nil || info.UpdatedServices != 1 || info.UpdatedOffers != 0 {
		t.Fatal("unknown product incorrectly produced quote", info, e)
	}
	out := filepath.Join(t.TempDir(), "dist")
	if e = Generate(path, "", out); e != nil {
		t.Fatal(e)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(page), "接码对象（产品规则待核实）") {
		t.Fatal("unknown product target row missing")
	}
}

func TestUnseenNamedTargetCanReceiveValidatedPrice(t *testing.T) {
	c, e := LoadCatalog(priceCatalog(t))
	if e != nil {
		t.Fatal(e)
	}
	c.Services = slices.DeleteFunc(c.Services, func(s Service) bool { return strings.EqualFold(s.Name, "Discord") })
	x := extraction("0.2", "USA")
	x.ServiceID = ""
	x.ServiceName = "Discord"
	x.Evidence = strings.Replace(x.Evidence, "Google", "Discord", 1)
	p := Product{ID: "fivesim-once", Provider: "fivesim", Kind: "once"}
	targets, prices, issues := extractTargets(&c, PriceExtraction{Services: []ExtractedService{{"Discord", x.Evidence}}, Offers: []ExtractedPrice{x}}, x.Evidence, testPriceConfig().Sources[0], p, time.Now())
	if len(targets) != 1 || len(prices) != 1 || len(issues) != 0 || prices[0].Service != "Discord" {
		t.Fatal("unseeded target rejected", targets, prices, issues)
	}
	c.ServiceListings = append(c.ServiceListings, targets...)
	c.Offers = append(c.Offers, prices...)
	b, _ := json.Marshal(c)
	var roundTrip Catalog
	if StrictJSON(b, &roundTrip) != nil || roundTrip.Validate() != nil {
		t.Fatal("new target broke catalog contract")
	}
}

func TestInvalidPriceDoesNotDiscardValidTarget(t *testing.T) {
	c, e := LoadCatalog(priceCatalog(t))
	if e != nil {
		t.Fatal(e)
	}
	text := "Supported SMS targets: Discord. Google USA USD 0.2 activation mobile SMS."
	invalid := extraction("99", "USA")
	invalid.Evidence = "Google USA USD 0.2 activation mobile SMS."
	listings, offers, issues := extractTargets(&c, PriceExtraction{Services: []ExtractedService{{"Discord", "Supported SMS targets: Discord."}}, Offers: []ExtractedPrice{invalid}}, text, testPriceConfig().Sources[0], Product{ID: "fivesim-once", Provider: "fivesim", Kind: "once"}, time.Now())
	if len(listings) != 1 || len(offers) != 0 || len(issues) == 0 {
		t.Fatal("bad price discarded a supported target or got accepted")
	}
}

func TestExpiredBatchBudgetDoesNotMakeMoreAPIRequests(t *testing.T) {
	path := priceCatalog(t)
	cfg := testPriceConfig()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("expired batch made a network/API request")
		return nil, nil
	})}
	info, e := updatePrices(ctx, cfg, path, client, time.Now())
	if e == nil || !info.Saved || len(info.Failures) != len(cfg.Sources) {
		t.Fatal("batch deadline did not save an explicit failure summary", info, e)
	}
	c, e := LoadCatalog(path)
	if e != nil || c.PriceUpdate == nil {
		t.Fatal("failure summary not persisted", e)
	}
}

func TestRefreshReplacesStaleRowsOnlyWhenComplete(t *testing.T) {
	old := func() *Catalog {
		return &Catalog{
			Offers:          []Offer{{ID: "auto-old", Product: "p", Status: "verified"}, {ID: "manual", Product: "p", Status: "verified"}},
			ServiceListings: []ServiceListing{{Provider: "x", Product: "p", ServiceID: "a", Status: "verified"}},
		}
	}
	fresh := []Offer{{ID: "auto-new", Product: "p", Status: "verified"}}
	listing := []ServiceListing{{Provider: "x", Product: "p", ServiceID: "a", Status: "verified"}}
	c := old()
	mergeTargetResults(c, PriceSource{Provider: "x", Product: "p"}, listing, fresh, true)
	foundNew, foundOld := false, false
	for _, o := range c.Offers {
		foundNew = foundNew || o.ID == "auto-new"
		foundOld = foundOld || o.ID == "auto-old"
	}
	if len(c.Offers) != 2 || !foundNew || foundOld {
		t.Fatal("complete refresh kept stale auto offers", c.Offers)
	}
	c = old()
	mergeTargetResults(c, PriceSource{Provider: "x", Product: "p"}, nil, fresh, false)
	if len(c.Offers) != 3 || c.Offers[0].Status != "unavailable" || c.Offers[0].ID != "auto-old" {
		t.Fatal("partial refresh dropped historical offers", c.Offers)
	}
	if len(c.ServiceListings) != 1 || c.ServiceListings[0].Status != "unavailable" {
		t.Fatal("partial refresh dropped historical listings", c.ServiceListings)
	}
}
func TestInboxMessagesCannotBecomeTargetCatalogEvidence(t *testing.T) {
	for _, text := range []string{"Netflix Your Netflix verification code is 123456.", "【应用】您的验证码为：123456。", "App code: 123456", "123456 is your sign in code", "WeChat Verification code (123456) may only be used once", "LINE Please enter 123456 into LINE", "Steam Use code 123456 to confirm", "【应用】您登录系统的动态码为：123456"} {
		if !messageEvidence(text) {
			t.Fatal("SMS example not recognized")
		}
		clean, e := sourceText([]byte("<p>"+text+"</p>"), "text/html")
		if e != nil || strings.Contains(clean, "123456") {
			t.Fatal("SMS code would be sent to analysis API", e)
		}
		c := Catalog{}
		if _, e = resolveTarget(&c, "Netflix", text, text); e == nil {
			t.Fatal("inbox message accepted as supported target")
		}
	}
	if messageEvidence("Netflix 120000 numbers from USD 0.12") {
		t.Fatal("inventory mistaken for verification code")
	}
}
