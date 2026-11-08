package identity

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCountryNormalization(t *testing.T) {
	cases := map[string]string{
		"US": "美国", "United States/USA": "美国", "usa": "美国",
		"MY": "马来西亚", "Malaysia": "马来西亚", "AU": "澳大利亚",
		"Argentina": "阿根廷", "Argentinas": "阿根廷", "Nambia": "纳米比亚", "Sierre Lyone": "塞拉利昂",
		"Bos. and Herz.": "波黑", "Hong Kong (China)": "中国香港", "Timor-Leste": "东帝汶",
		"USA(Virtual)": "美国（虚拟）", "Australia (Virtual)": "澳大利亚（虚拟）", "USA (real)": "美国（实体）",
		"USA (2)": "美国（虚拟）",
		"西班牙":     "西班牙", "未知": "未知", "": "",
	}
	for in, want := range cases {
		if got := normalizedCountry(in); got != want {
			t.Errorf("normalizedCountry(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"schema_version":1,"schema_version":1}`, `{"schema_version":1,"invitations":{"a":"x","a":"y"}}`, `{"schema_version":1,"extra":true}`, `{} {}`, `{"invitations":[]}`} {
		var r Referrals
		if StrictJSON([]byte(raw), &r) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestURLs(t *testing.T) {
	if ValidateURL("https://5sim.net:", []string{"5sim.net"}) == nil {
		t.Fatal("empty explicit port accepted")
	}
	good := "https://5sim.net/register?b=2&b=1&value=%26%2F#/?invite=test"
	if e := ValidateURL(good, []string{"5sim.net"}); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{"http://5sim.net/", "https://5sim.net.evil.test/", "https://evil5sim.net/", "https://sub.5sim.net/", "https://user@5sim.net/", "https://5sim.net:80/", "https://5sim.net/\n", "https://5sim.net/%0d", strings.Repeat("a", 4097), "javascript:alert(1)", "https://5sim.net./", "https://5sim.net\\@evil.test/"} {
		if ValidateURL(raw, []string{"5sim.net"}) == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
func TestCatalogAndSorting(t *testing.T) {
	c, e := LoadCatalog("data/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Providers) != 22 || len(c.Candidates) != 3 {
		t.Fatal("provider counts")
	}
	if slices.ContainsFunc(c.Providers, func(p Provider) bool { return p.ID == "daisysms" || p.ID == "eomsg" || p.ID == "d1jiema" }) || slices.ContainsFunc(c.Candidates, func(p Candidate) bool { return p.Name == "SMS-Activate" || p.Name == "Truverifi" }) {
		t.Fatal("removed unavailable sites returned")
	}
	zero, small, large := "0", "0.100000000000000001", "0.100000000000000002"
	p := Product{ID: "p", Kind: "rental"}
	products := map[string]Product{"p": p}
	base := Offer{Product: "p", Country: "US", Service: "service", Term: "1 month", Currency: "USD", Unit: "number/month", Kind: "exact", Scope: "one service", NumberType: "mobile"}
	offers := []Offer{}
	for i, v := range []*string{nil, &large, &zero, &small} {
		o := base
		o.ID = string(rune('a' + i))
		o.Amount = v
		offers = append(offers, o)
	}
	SortOffers(offers, products)
	if offers[0].Amount != &zero || offers[1].Amount != &small || offers[2].Amount != &large || offers[3].Amount != nil {
		t.Fatal("decimal ordering lost precision or confused nil with zero")
	}
	for _, change := range []func(*Offer){func(o *Offer) { o.Term = "30 days" }, func(o *Offer) { o.Country = "GB" }, func(o *Offer) { o.Service = "other" }, func(o *Offer) { o.Currency = "CNY" }, func(o *Offer) { o.Unit = "SMS" }, func(o *Offer) { o.Scope = "all" }, func(o *Offer) { o.Kind = "starting" }, func(o *Offer) { o.NumberType = "voip" }} {
		other := base
		change(&other)
		if Group(base, p) == Group(other, p) {
			t.Fatal("incompatible groups merged")
		}
	}
	for _, v := range []*string{nil, &zero, &small} {
		if !amountOK(v) {
			t.Fatal("valid amount rejected")
		}
	}
	for _, v := range []string{"-1", "1e2", "NaN", ".1", "01", "1/2"} {
		if amountOK(&v) {
			t.Fatal("invalid amount accepted", v)
		}
	}
	if !slices.ContainsFunc(c.Products, func(p Product) bool {
		return p.Provider == "sms-man" && p.Kind == "rental" && p.Refund == "nonrefundable"
	}) {
		t.Fatal("rental refund rule lost")
	}
	c.Offers[0].Product = "missing"
	if c.Validate() == nil {
		t.Fatal("invalid reference accepted")
	}
}
func TestProviderPaymentFacts(t *testing.T) {
	c, e := LoadCatalog("data/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range c.Providers {
		if p.DepositNote == "" || p.PaymentVerified == "" {
			t.Errorf("provider %s missing deposit facts", p.ID)
		}
		// A minimum only goes on the page when the provider's own page states it;
		// otherwise the note must say the amount needs a login (or is unverified).
		if strings.Contains(p.DepositNote, "最低充值与支付方式") {
			t.Errorf("provider %s hides the payment method although only the minimum needs a login", p.ID)
		}
	}
	c.Providers[0].PaymentVerified = "2026-13-01"
	if c.Validate() == nil {
		t.Fatal("invalid payment verified date accepted")
	}
}

func TestGenerateLinksAndPreserveOld(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "dist")
	refs := filepath.Join(dir, "referrals.json")
	raw := "https://5sim.net/register?z=2&z=1&value=%26%2F#/?invite=fixture"
	data, _ := json.Marshal(Referrals{1, map[string]string{"fivesim": raw, "haozhuma": "https://my.haozhuma.com/?fixture=1", "sms-man": "https://sms-man.com/?fixture=1"}})
	os.WriteFile(refs, data, 0600)
	if e := Generate("data/catalog.json", refs, out); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(out, "index.html"))
	if e != nil {
		t.Fatal(e)
	}
	// Cards are now rendered client-side from embedded JSON, so verify the data
	// payload (the source of truth) rather than pre-rendered HTML.
	rowsJSON := extractRowsJSON(t, string(b))
	all := strings.Join(rowsJSON, "\n") + "\n" + html.UnescapeString(string(b))
	for _, value := range []string{raw, "https://my.haozhuma.com/?fixture=1", "https://sms-man.com/rent", "https://5sim.net/rules"} {
		if !strings.Contains(all, value) {
			t.Fatal("missing preserved URL", value)
		}
	}
	if !strings.Contains(all, `"channels":{`) {
		t.Fatal("missing channel deposit payload")
	}
	os.WriteFile(refs, []byte(`{"schema_version":1,"invitations":{"bogus":"https://evil.test/"}}`), 0600)
	if Generate("data/catalog.json", refs, out) == nil {
		t.Fatal("invalid referral accepted")
	}
	after, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if string(after) != string(b) {
		t.Fatal("failed build changed previous page")
	}
	if Generate("data/catalog.json", filepath.Join(dir, "missing"), out) == nil {
		t.Fatal("explicit missing config ignored")
	}
	if e = Generate("data/catalog.json", "", out); e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(html.UnescapeString(strings.Join(extractRowsJSON(t, string(after)), "\n")), raw) {
		t.Fatal("restored URL still invited")
	}
	os.WriteFile(filepath.Join(out, "private.txt"), []byte("keep"), 0600)
	if Generate("data/catalog.json", "", out) == nil {
		t.Fatal("would delete unrelated files")
	}
	if _, e = os.Stat(filepath.Join(out, "private.txt")); e != nil {
		t.Fatal("unrelated file lost")
	}
}

// extractRowsJSON returns all string values from the embedded #rows-data payload,
// JSON-decoded (so URLs keep their real &, #, % characters).
func extractRowsJSON(t *testing.T, page string) []string {
	t.Helper()
	const tag = `<script type="application/json" id="rows-data">`
	i := strings.Index(page, tag)
	if i < 0 {
		t.Fatal("rows-data script missing")
	}
	rest := page[i+len(tag):]
	j := strings.Index(rest, "</script>")
	if j < 0 {
		t.Fatal("rows-data script not closed")
	}
	var payload struct {
		Products map[string]map[string]any `json:"products"`
		Pool     []string                  `json:"pool"`
		Rows     []map[string]any          `json:"rows"`
	}
	if err := json.Unmarshal([]byte(rest[:j]), &payload); err != nil {
		t.Fatalf("rows-data invalid JSON: %v", err)
	}
	var out []string
	out = append(out, payload.Pool...)
	collect := func(m map[string]any) {
		for _, v := range m {
			switch val := v.(type) {
			case string:
				out = append(out, val)
			case []any:
				for _, it := range val {
					if s, ok := it.(string); ok {
						out = append(out, s)
					}
				}
			}
		}
	}
	for _, p := range payload.Products {
		collect(p)
	}
	for _, r := range payload.Rows {
		collect(r)
	}
	return out
}

func TestTemplateEscapingAndDates(t *testing.T) {
	dir := t.TempDir()
	c, e := LoadCatalog("data/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	c.Providers[0].Name = `<script>alert("x")</script>`
	for i := range c.Offers {
		c.Offers[i].Verified = "2020-01-01"
	}
	b, _ := json.Marshal(c)
	path := filepath.Join(dir, "catalog.json")
	os.WriteFile(path, b, 0600)
	out := filepath.Join(dir, "dist")
	if e = Generate(path, "", out); e != nil {
		t.Fatal(e)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(string(page), `<script>alert("x")</script>`) {
		t.Fatal("unescaped HTML")
	}
	// Stale/date facts live in the embedded rows JSON now; the "stale" badge text
	// is emitted client-side, so assert the data payload + renderer template.
	if !strings.Contains(string(page), `"w":true`) {
		t.Fatal("stale facts not flagged in data")
	}
	if !strings.Contains(strings.Join(extractRowsJSON(t, string(page)), "\n"), "2020-01-01") {
		t.Fatal("build refreshed fact date")
	}
}
