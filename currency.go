package identity

import (
	"math/big"
	"strings"
)

// 统一币种为 USD。非 USD 报价按此快照汇率折算，原价与原币种保留在证据里，便于追溯。
// 网页本身就以美元（USD/$）出示的价格原样保留，不再参与折算，避免被换算值覆盖。
// 汇率来源与时间：BTC/USD=75854（coingecko 2026-11-06）；1 USD=6.6713 CNY（frankfurter 2026-11-06 UTC）。
// 更新汇率只需改这里并注明时间；采集端继续保存原币种，避免把波动汇率固化进源头数据。
const (
	rateTimestamp = "2026-11-06"
	btcUSD        = "75854"   // 1 BTC 的美元价
	satsPerBTC    = "100000000"
	usdPerCNY     = "0.14990" // 1 CNY ≈ 1/6.6713 USD
)

// toUSD converts an amount string in cur to a USD decimal string.
// ok=false means the currency is unknown and cannot be converted honestly.
func toUSD(amount, cur string) (string, bool) {
	amt, ok := new(big.Rat).SetString(amount)
	if !ok {
		return "", false
	}
	switch strings.ToUpper(cur) {
	case "USD", "$", "US$":
		return amount, true
	case "CNY", "RMB":
		r, _ := new(big.Rat).SetString(usdPerCNY)
		return new(big.Rat).Mul(amt, r).FloatString(6), true
	case "SATS", "SAT":
		btc, _ := new(big.Rat).SetString(btcUSD)
		per, _ := new(big.Rat).SetString(satsPerBTC)
		usd := new(big.Rat).Mul(amt, btc)
		usd.Quo(usd, per)
		return usd.FloatString(6), true
	}
	return "", false
}

// isUSD reports whether a currency label is already in US dollars.
func isUSD(cur string) bool {
	switch strings.ToUpper(cur) {
	case "USD", "$", "US$":
		return true
	}
	return false
}

// natRank orders offers by currency-originalness: prices the source itself showed in
// dollars stay in dollars (0) and are never overwritten by a conversion (1).
func natRank(cur string) int {
	if isUSD(cur) {
		return 0
	}
	return 1
}

// unifyCurrency rewrites every offer's Amount (and RenewalAmount) to USD,
// preserving the original unit in Evidence.
func unifyCurrency(c *Catalog) {
	for i := range c.Offers {
		o := &c.Offers[i]
		if o.Amount == nil {
			continue
		}
		origCur := o.Currency
		usd, ok := toUSD(*o.Amount, origCur)
		if !ok {
			continue
		}
		if !isUSD(origCur) {
			orig := "原价 " + *o.Amount + " " + origCur + " ≈ USD " + usd + "（" + rateTimestamp + " 汇率折算）"
			if o.Evidence == "" {
				o.Evidence = orig
			} else {
				o.Evidence = orig + "；" + o.Evidence
			}
		}
		if o.RenewalAmount != nil {
			if r, ok2 := toUSD(*o.RenewalAmount, origCur); ok2 {
				o.RenewalAmount = &r
			}
		}
		o.Amount = &usd
		o.Currency = "USD"
	}
}
