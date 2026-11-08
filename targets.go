package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

var slashSpacing = regexp.MustCompile(`\s*/\s*`)

func targetNameKey(name string) string {
	return strings.ToLower(slashSpacing.ReplaceAllString(strings.Join(strings.Fields(name), " "), "/"))
}

func targetIndex(services []Service) map[string]Service {
	index := map[string]Service{}
	for _, s := range services {
		index[targetNameKey(s.Name)] = s
	}
	// Maintained aliases take precedence over legacy duplicate names.
	for _, s := range services {
		for _, name := range s.Aliases {
			index[targetNameKey(name)] = s
		}
	}
	return index
}

func resolveTarget(c *Catalog, name, evidence, text string) (Service, error) {
	if name == "" || len(name) > 160 || strings.TrimSpace(name) != name || len(evidence) < 2 || len(evidence) > 1600 || messageEvidence(evidence) || !strings.Contains(text, evidence) || !containsEvidence(evidence, name) {
		return Service{}, errors.New("接码对象名称缺少原文证据")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return Service{}, errors.New("接码对象名称含控制字符")
		}
	}
	for _, s := range c.Services {
		for _, alias := range append([]string{s.Name}, s.Aliases...) {
			if targetNameKey(alias) == targetNameKey(name) {
				return s, nil
			}
		}
	}
	sum := sha256.Sum256([]byte(targetNameKey(name)))
	s := Service{ID: "target-" + hex.EncodeToString(sum[:10]), Name: name, Aliases: []string{}}
	if slices.ContainsFunc(c.Services, func(v Service) bool { return v.ID == s.ID }) {
		return Service{}, errors.New("接码对象名称标识冲突")
	}
	c.Services = append(c.Services, s)
	return s, nil
}

func extractTargets(c *Catalog, result PriceExtraction, text string, source PriceSource, p Product, now time.Time) ([]ServiceListing, []Offer, []string) {
	listings := map[string]ServiceListing{}
	var offers []Offer
	var issues []string
	add := func(service Service, name, evidence string) {
		listings[service.ID] = ServiceListing{Provider: source.Provider, Product: source.Product, ServiceID: service.ID, Name: name, Source: source.URL, Evidence: evidence, CheckedAt: now.Format(time.RFC3339), Status: "verified"}
	}
	for _, target := range result.Services {
		s, e := resolveTarget(c, target.Name, target.Evidence, text)
		if e != nil {
			issues = append(issues, e.Error())
			continue
		}
		add(s, target.Name, target.Evidence)
	}
	seenOffers := map[string]int{}
	for _, x := range result.Offers {
		if p.ID == "" {
			issues = append(issues, "产品类型尚未核实，接码对象已保留但不录入报价")
			continue
		}
		var service Service
		if x.ServiceName != "" {
			s, e := resolveTarget(c, x.ServiceName, x.Evidence, text)
			if e != nil {
				issues = append(issues, e.Error())
				continue
			}
			service = s
			x.ServiceID = s.ID
		} else {
			for _, s := range c.Services {
				if s.ID == x.ServiceID {
					service = s
					break
				}
			}
		}
		offer, e := validateExtraction(x, text, source, p, c.Services, now)
		if e != nil {
			issues = append(issues, e.Error())
			continue
		}
		name := x.ServiceName
		if name == "" {
			name = service.Name
		}
		add(service, name, x.Evidence)
		if index, ok := seenOffers[offer.ID]; ok {
			offers[index].Status = "conflicting"
			issues = append(issues, "同条件报价冲突，历史价格将保留")
			continue
		}
		seenOffers[offer.ID] = len(offers)
		offers = append(offers, offer)
	}
	valid := offers[:0]
	for _, o := range offers {
		if o.Status != "conflicting" {
			valid = append(valid, o)
		}
	}
	resultListings := make([]ServiceListing, 0, len(listings))
	for _, l := range listings {
		resultListings = append(resultListings, l)
	}
	slices.SortFunc(resultListings, func(a, b ServiceListing) int { return strings.Compare(a.ServiceID, b.ServiceID) })
	return resultListings, valid, issues
}

func targetKey(provider, product, service string) string {
	return provider + "/" + product + "/" + service
}

func mergeTargetResults(c *Catalog, s PriceSource, listings []ServiceListing, fresh []Offer, prune bool) {
	newIDs := map[string]bool{}
	for _, o := range fresh {
		newIDs[o.ID] = true
	}
	// A complete refresh replaces stale rows; a failed or partial one keeps them labelled.
	succeeded := prune && len(fresh) > 0
	retained := c.Offers[:0]
	for _, o := range c.Offers {
		if o.Product == s.Product && strings.HasPrefix(o.ID, "auto-") {
			if newIDs[o.ID] || succeeded {
				continue
			}
			o.Status = "unavailable"
		}
		retained = append(retained, o)
	}
	c.Offers = append(retained, fresh...)
	newListings := map[string]bool{}
	for _, l := range listings {
		newListings[l.ServiceID] = true
	}
	listingsSucceeded := prune && len(listings) > 0
	kept := c.ServiceListings[:0]
	for _, l := range c.ServiceListings {
		if l.Provider == s.Provider && l.Product == s.Product {
			if newListings[l.ServiceID] || listingsSucceeded {
				continue
			}
			l.Status = "unavailable"
		}
		kept = append(kept, l)
	}
	c.ServiceListings = append(kept, listings...)
}
