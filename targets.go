package identity

import (
	"regexp"
	"strings"
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
