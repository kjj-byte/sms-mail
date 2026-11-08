package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Row struct {
	Provider    Provider
	Product     Product
	Offer       Offer
	Purchase    string
	Group       string
	Search      string
	Stale       bool
	Invited     bool
	ListingOnly bool
}
type TargetOption struct {
	Name     string
	Channels int
}
type Page struct {
	Rows        []Row
	Pending     []Row
	Candidates  []Candidate
	Count       int
	PriceUpdate *PriceUpdate
	Targets     []TargetOption
	RowsJSON    template.JS // compact serialized rows for client-side card rendering
}

// prodJS holds product-level fields shared by every offer of that product, so the
// per-row payload stays small (tens of thousands of offers share only ~32 products).
type prodJS struct {
	PN string   `json:"pn"`           // product name
	D  string   `json:"d"`            // duration
	Rp string   `json:"rp"`           // repeat label
	Rn string   `json:"rn"`           // renew label
	Ru string   `json:"ru"`           // reuse label
	Co string   `json:"co,omitempty"` // conditions
	St string   `json:"st,omitempty"` // start point
	Ar string   `json:"ar,omitempty"` // auto renew label
	Rs string   `json:"rs,omitempty"` // restore label
	On string   `json:"on,omitempty"` // online label
	Ps []string `json:"ps,omitempty"` // product rule source URLs
	M  string   `json:"m,omitempty"`  // manage URL
	Pv string   `json:"pv,omitempty"` // product verified date
}

// channelJS holds channel recharge facts shared by every card of that channel.
type channelJS struct {
	D  string   `json:"d,omitempty"`  // deposit note (minimum amount / free / unverified)
	Pm []string `json:"pm,omitempty"` // payment methods
	V  string   `json:"v,omitempty"`  // payment info verified date
}

// rowJS is the minimal per-card payload the client uses to build a card on demand.
type rowJS struct {
	P   string `json:"p"`             // provider name
	K   string `json:"k"`             // product kind
	Ok  string `json:"ok"`            // offer kind
	N   int    `json:"n"`             // number type (pool index)
	Sc  int    `json:"sc"`            // scope (pool index)
	Pid string `json:"pid"`           // product id -> lookup
	C   string `json:"c"`             // country
	S   string `json:"s"`             // service
	T   int    `json:"t"`             // term (pool index)
	R   string `json:"r"`             // refund state
	U   string `json:"u"`             // unit
	Cu  string `json:"cu"`            // currency
	A   string `json:"a,omitempty"`   // amount; empty = unpriced
	Pi  string `json:"pi"`            // provider id (purchase key)
	H   int    `json:"h"`             // purchase URL (pool index)
	Q   string `json:"q"`             // search text (lower)
	Me  int    `json:"me"`            // method label (pool index)
	Os  int    `json:"os"`            // offer source (pool index)
	Ov  int    `json:"ov"`            // offer verified date (pool index)
	Ca  int    `json:"ca"`            // offer checked-at (pool index)
	Ra  string `json:"ra,omitempty"`  // renewal amount
	Oa  string `json:"oa,omitempty"`  // original amount before conversion
	Oc  string `json:"oc,omitempty"`  // original currency
	Ora string `json:"ora,omitempty"` // original renewal amount
	Sm  string `json:"sm,omitempty"`  // status label
	Lf  bool   `json:"lf,omitempty"`  // listing only
	Iv  bool   `json:"iv,omitempty"`  // invited
	W   bool   `json:"w,omitempty"`   // stale
}

// money renders an amount string as a readable price without changing its value:
// trailing zeros trimmed, but genuine zero stays "0".
func money(s string) string {
	if s == "" {
		return s
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func label(s string) string {
	switch s {
	case "public_api":
		return "官方公开 API"
	case "authenticated_api":
		return "官方认证价格 API"
	case "page_data":
		return "页面结构化数据"
	case "page_text":
		return "价格页文本"
	case "page_analysis":
		return "页面分析"
	}
	if s == "pending" {
		return "产品类型待核实"
	}
	m := map[string]string{"free": "免费公共号码", "once": "一次性接码", "rental": "可续费号码", "supported": "支持", "unsupported": "不支持", "conditional": "有条件", "unknown": "未知", "conflicting": "资料冲突待核实", "refundable": "未收码可退", "nonrefundable": "不可退", "na": "不适用", "exact": "具体报价", "starting": "官网宣传起价", "login": "登录官网查看价格", "unavailable": "来源上次访问失败", "verified": "人工核验"}
	if v, ok := m[s]; ok {
		return v
	}
	return s
}

// umbrellaTargets are different platforms' spellings of the same "any other /
// not-listed" fallback service. They are merged into one canonical target so a
// keyword search shows a single object instead of several duplicates.
func umbrellaKey(name string) bool {
	switch targetNameKey(strings.TrimSpace(name)) {
	case "anyother", "any other", "anyother/другой", "любой другой", "not listed/other/any", "other", "other (no guarantee)", "service not listed":
		return true
	}
	return false
}

const umbrellaID = "target-other-any"

func mergeUmbrellaTargets(c *Catalog) {
	var kept []Service
	merged := false
	variants := map[string]bool{}
	for _, s := range c.Services {
		if umbrellaKey(s.Name) {
			merged = true
			variants[s.ID] = true
			continue
		}
		kept = append(kept, s)
	}
	if !merged {
		return
	}
	// Canonical service keeps the alias list so lookups still resolve.
	canonical := Service{ID: umbrellaID, Name: "其他/未列出 (Any Other)", Aliases: []string{"AnyOther", "Any other", "AnyOther/Другой", "Любой другой", "Not Listed / Other / Any", "other"}}
	kept = append(kept, canonical)
	c.Services = kept
	for i := range c.Offers {
		if variants[c.Offers[i].ServiceID] {
			c.Offers[i].ServiceID = umbrellaID
			c.Offers[i].Service = canonical.Name
		}
	}
	for i := range c.ServiceListings {
		if variants[c.ServiceListings[i].ServiceID] {
			c.ServiceListings[i].ServiceID = umbrellaID
		}
	}
}

func Generate(catalogPath, referralsPath, out string) error {
	c, e := LoadCatalog(catalogPath)
	if e != nil {
		return e
	}
	mergeUmbrellaTargets(&c)
	index := targetIndex(c.Services)
	for i, o := range c.Offers {
		if s, ok := index[targetNameKey(o.Service)]; ok && o.ServiceID != "" {
			c.Offers[i].ServiceID = s.ID
			c.Offers[i].Service = s.Name
		}
	}
	for i, l := range c.ServiceListings {
		if s, ok := index[targetNameKey(l.Name)]; ok {
			c.ServiceListings[i].ServiceID = s.ID
		}
	}
	originalPrices := map[string]Offer{}
	for _, o := range c.Offers {
		if o.Amount != nil && !isUSD(o.Currency) {
			originalPrices[o.ID] = o
		}
	}
	unifyCurrency(&c)
	r, e := LoadReferrals(referralsPath, c.Providers)
	if e != nil {
		return e
	}
	page := Page{Count: len(c.Providers), Candidates: c.Candidates, PriceUpdate: c.PriceUpdate}
	providers := map[string]Provider{}
	products := map[string]Product{}
	used := map[string]bool{}
	for _, p := range c.Providers {
		providers[p.ID] = p
	}
	for _, p := range c.Products {
		products[p.ID] = p
		used[p.Provider] = true
	}
	services := map[string]Service{}
	serviceSearch := map[string]string{}
	for _, s := range c.Services {
		services[s.ID] = s
		serviceSearch[s.Name] = s.Name + " " + strings.Join(s.Aliases, " ")
	}
	sourceNames := map[string]string{}
	for _, l := range c.ServiceListings {
		if messageEvidence(l.Evidence) {
			continue
		}
		sourceNames[l.Provider+"/"+l.ServiceID] += " " + l.Name
	}
	quoted := map[string]bool{}
	quotedProviders := map[string]bool{}
	for _, o := range c.Offers {
		p := products[o.Product]
		if o.ServiceID != "" {
			quoted[targetKey(p.Provider, p.ID, o.ServiceID)] = true
			quotedProviders[p.Provider+"/"+o.ServiceID] = true
		}
	}
	for _, l := range c.ServiceListings {
		if messageEvidence(l.Evidence) {
			continue
		}
		if quoted[targetKey(l.Provider, l.Product, l.ServiceID)] || l.Product == "" && quotedProviders[l.Provider+"/"+l.ServiceID] {
			continue
		}
		p := products[l.Product]
		if l.Product == "" {
			p = Product{ID: "listing-product-" + l.Provider, Provider: l.Provider, Name: "接码对象（产品规则待核实）", Kind: "pending", Duration: "未知", Starts: "未知", Repeat: "unknown", Reuse: "unknown", Renew: "unknown", AutoRenew: "unknown", Restore: "unknown", Online: "unknown", Refund: "unknown", Conditions: "已拉取到该渠道列出的接码对象；产品类型、价格与可用库存仍需官网确认。", Sources: []string{l.Source}, Verified: strings.Split(l.CheckedAt, "T")[0]}
			products[p.ID] = p
		}
		c.Offers = append(c.Offers, Offer{ID: "listing-" + l.Provider + "-" + l.ServiceID + "-" + p.ID, Product: p.ID, ServiceID: l.ServiceID, Service: services[l.ServiceID].Name, Country: "未知", NumberType: "未注明", Scope: "未注明", Term: "未知", Currency: "未知", Unit: "官网查看", Kind: "unknown", Status: l.Status, Source: l.Source, Verified: strings.Split(l.CheckedAt, "T")[0], CheckedAt: l.CheckedAt, Evidence: l.Evidence})
	}
	for i := range c.Offers {
		c.Offers[i].Country = normalizedCountry(c.Offers[i].Country)
	}
	SortOffers(c.Offers, products)
	targetChannels := map[string]map[string]bool{}
	for _, o := range c.Offers {
		if messageEvidence(o.Evidence) {
			continue
		}
		p := products[o.Product]
		v := providers[p.Provider]
		date, _ := time.Parse("2006-01-02", o.Verified)
		pd, _ := time.Parse("2006-01-02", p.Verified)
		search := o.Service
		search += " " + serviceSearch[o.Service] + sourceNames[p.Provider+"/"+o.ServiceID]
		// Deduplicate search terms once: repeated alias words across 17k offers inflate the page by tens of MB.
		seen := map[string]bool{}
		var uniq []string
		for _, w := range strings.Fields(search) {
			if !seen[w] {
				seen[w] = true
				uniq = append(uniq, w)
			}
		}
		search = strings.Join(uniq, " ")
		if o.ServiceID != "" {
			if targetChannels[o.Service] == nil {
				targetChannels[o.Service] = map[string]bool{}
			}
			targetChannels[o.Service][v.ID] = true
		}
		stale := time.Since(date) > 7*24*time.Hour || time.Since(pd) > 7*24*time.Hour
		if o.CheckedAt != "" {
			checked, _ := time.Parse(time.RFC3339, o.CheckedAt)
			stale = stale || time.Since(checked) > 24*time.Hour
		}
		group := Group(o, p)
		page.Rows = append(page.Rows, Row{Provider: v, Product: p, Offer: o, Purchase: PurchaseURL(p, v, r), Group: group, Search: strings.ToLower(search), Stale: stale, Invited: r.Invitations[v.ID] != "", ListingOnly: strings.HasPrefix(o.ID, "listing-")})
	}
	for name, channels := range targetChannels {
		page.Targets = append(page.Targets, TargetOption{name, len(channels)})
	}
	slices.SortFunc(page.Targets, func(a, b TargetOption) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, p := range c.Providers {
		if !used[p.ID] {
			page.Pending = append(page.Pending, Row{Provider: p, Purchase: PurchaseURL(Product{}, p, r), Invited: r.Invitations[p.ID] != ""})
		}
	}
	jsProducts := map[string]prodJS{}
	channels := map[string]channelJS{}
	for _, p := range c.Providers {
		if p.DepositNote != "" {
			channels[p.ID] = channelJS{D: p.DepositNote, Pm: p.PaymentMethods, V: p.PaymentVerified}
		}
	}
	jsRows := make([]rowJS, 0, len(page.Rows))
	// String pool for high-repetition fields (term/method/source/date/purchase URL)
	// so 29k rows store small integer indexes instead of repeated long strings.
	pool := []string{""}
	poolIdx := map[string]int{"": 0}
	intern := func(s string) int {
		if s == "" {
			return 0
		}
		if i, ok := poolIdx[s]; ok {
			return i
		}
		poolIdx[s] = len(pool)
		pool = append(pool, s)
		return len(pool) - 1
	}
	for _, row := range page.Rows {
		// product-level fields go into the lookup once per product id.
		if _, ok := jsProducts[row.Product.ID]; !ok {
			pj := prodJS{
				PN: row.Product.Name, D: row.Product.Duration,
				Rp: label(row.Product.Repeat), Rn: label(row.Product.Renew), Ru: label(row.Product.Reuse),
				Co: row.Product.Conditions, St: row.Product.Starts,
				Ar: label(row.Product.AutoRenew), Rs: label(row.Product.Restore), On: label(row.Product.Online),
				Ps: row.Product.Sources, M: row.Product.ManageURL, Pv: row.Product.Verified,
			}
			jsProducts[row.Product.ID] = pj
		}
		j := rowJS{
			P: row.Provider.Name, K: row.Product.Kind, Pid: row.Product.ID,
			Ok: row.Offer.Kind, N: intern(row.Offer.NumberType), Sc: intern(row.Offer.Scope),
			C: row.Offer.Country, S: row.Offer.Service, T: intern(row.Offer.Term),
			R: row.Product.Refund, U: row.Offer.Unit, Cu: row.Offer.Currency,
			Pi: row.Provider.ID, H: intern(row.Purchase),
			Q: row.Search, Os: intern(row.Offer.Source),
			Ov: intern(row.Offer.Verified), Ca: intern(row.Offer.CheckedAt),
			Lf: row.ListingOnly, Iv: row.Invited, W: row.Stale,
		}
		if row.Offer.Amount != nil {
			j.A = money(*row.Offer.Amount)
		}
		if row.Offer.RenewalAmount != nil {
			j.Ra = money(*row.Offer.RenewalAmount)
		}
		if original, ok := originalPrices[row.Offer.ID]; ok && original.Currency != row.Offer.Currency {
			j.Oa, j.Oc = money(*original.Amount), original.Currency
			if original.RenewalAmount != nil {
				j.Ora = money(*original.RenewalAmount)
			}
		}
		if row.Offer.Method != "" {
			j.Me = intern(label(row.Offer.Method))
		}
		if row.Offer.Status != "verified" {
			j.Sm = label(row.Offer.Status)
		}
		jsRows = append(jsRows, j)
	}
	if b, err := json.Marshal(struct {
		Products map[string]prodJS    `json:"products"`
		Channels map[string]channelJS `json:"channels"`
		Pool     []string             `json:"pool"`
		Rows     []rowJS              `json:"rows"`
		RateDate string               `json:"rate_date"`
	}{jsProducts, channels, pool, jsRows, rateTimestamp}); err == nil {
		page.RowsJSON = template.JS(b)
	} else {
		page.RowsJSON = `{"products":{},"channels":{},"pool":[""],"rows":[]}`
	}
	t, e := template.New("index.gohtml").Funcs(template.FuncMap{"label": label, "join": strings.Join, "money": money, "toJSON": func(v any) template.JS {
		b, err := json.Marshal(v)
		if err != nil {
			return "[]"
		}
		return template.JS(b)
	}}).ParseFS(assets, "web/index.gohtml")
	if e != nil {
		return e
	}
	var html bytes.Buffer
	if e = t.Execute(&html, page); e != nil {
		return e
	}
	abs, e := filepath.Abs(out)
	if e != nil {
		return e
	}
	if abs == filepath.VolumeName(abs)+string(os.PathSeparator) {
		return errors.New("不能输出到磁盘根目录")
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0755); e != nil {
		return e
	}
	lock, e := os.OpenFile(abs+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return errors.New("导航正在生成或存在未清理的发布锁")
	}
	lock.Close()
	defer os.Remove(abs + ".lock")
	if entries, err := os.ReadDir(abs); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || (entry.Name() != "index.html" && entry.Name() != "styles.css" && entry.Name() != "app.js" && entry.Name() != "rows.json") {
				return errors.New("输出目录包含非导航文件，请选择专用空目录")
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0755); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(filepath.Dir(abs), ".identity-stage-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	if e = os.WriteFile(filepath.Join(stage, "index.html"), html.Bytes(), 0644); e != nil {
		return e
	}
	// Price rows are served as a separate file so index.html stays small and opens fast.
	if e = os.WriteFile(filepath.Join(stage, "rows.json"), []byte(page.RowsJSON), 0644); e != nil {
		return e
	}
	for _, name := range []string{"styles.css", "app.js"} {
		b, e := assets.ReadFile("web/" + name)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(stage, name), b, 0644); e != nil {
			return e
		}
	}
	// A sibling backup allows rollback if publication fails, including Windows replacement failures.
	backup := abs + ".previous"
	if _, e = os.Lstat(backup); e == nil {
		return errors.New("检测到上次发布备份，请先检查并移走 .previous 目录")
	}
	hadOld := false
	if info, err := os.Lstat(abs); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("输出必须是普通目录")
		}
		if e = os.Rename(abs, backup); e != nil {
			return e
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if e = os.Rename(stage, abs); e != nil {
		if hadOld {
			if rollback := os.Rename(backup, abs); rollback != nil {
				return errors.Join(e, rollback)
			}
		}
		return e
	}
	if hadOld {
		return os.RemoveAll(backup)
	}
	return nil
}
