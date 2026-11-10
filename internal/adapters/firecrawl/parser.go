package firecrawl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"prism/internal/domain"
)

// Supported contracts are deliberately narrow. Never select the first of several
// properties, or use an LLM response as a replacement for source text.
func parseDocument(markdown, document string) (extracted, error) {
	return parseDocumentForSource(markdown, document, "")
}
func parseDocumentForSource(markdown, document, sourceURL string) (extracted, error) {
	nodes := []map[string]any{}
	listings := []map[string]any{}
	if document != "" {
		root, err := html.Parse(strings.NewReader(document))
		if err != nil {
			return extracted{}, err
		}
		var scan func(*html.Node) error
		scan = func(n *html.Node) error {
			if n.Type == html.ElementNode && n.Data == "script" {
				for _, a := range n.Attr {
					if a.Key == "type" && strings.EqualFold(strings.TrimSpace(a.Val), "application/ld+json") {
						var data any
						if n.FirstChild == nil || json.Unmarshal([]byte(n.FirstChild.Data), &data) != nil {
							return fmt.Errorf("invalid JSON-LD; no fallback across malformed structured data")
						}
						if hasCollection(data) {
							return fmt.Errorf("structured collection/search page is not a single listing")
						}
						collectApartments(data, &nodes)
						collectScopedListings(data, sourceURL, &listings)
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if err := scan(c); err != nil {
					return err
				}
			}
			return nil
		}
		if err := scan(root); err != nil {
			return extracted{}, err
		}
	}
	if len(listings) > 1 {
		return extracted{}, fmt.Errorf("multiple scoped RealEstateListing entities")
	}
	if len(listings) == 1 {
		parent := listings[0]
		child := object(parent["mainEntity"])
		merged := map[string]any{}
		for key, value := range child {
			merged[key] = value
		}
		if merged["offers"] == nil {
			merged["offers"] = parent["offers"]
		}
		if merged["datePublished"] == nil {
			merged["datePublished"] = parent["datePosted"]
		}
		offer := object(merged["offers"])
		if offer != nil && stringValue(offer["businessFunction"]) == "" {
			copyOffer := map[string]any{}
			for key, value := range offer {
				copyOffer[key] = value
			}
			action := object(parent["potentialAction"])
			if isType(action["@type"], "BuyAction") {
				copyOffer["businessFunction"] = "Sell"
			}
			if isType(action["@type"], "RentAction") {
				copyOffer["businessFunction"] = "LeaseOut"
			}
			merged["offers"] = copyOffer
		}
		f, err := parseApartment(merged)
		if err != nil {
			return f, err
		}
		f.method = "jsonld_scoped_listing_v1"
		f.extraWarnings = append(f.extraWarnings, "Offer is scoped through the publisher RealEstateListing mainEntity and matching page URL; it does not establish a completed transaction.")
		return f, nil
	}
	if len(nodes) > 1 {
		return extracted{}, fmt.Errorf("multiple Apartment entities; a single-property page is required")
	}
	if len(nodes) == 1 {
		return parseApartment(nodes[0])
	}
	return parseLabels(markdown)
}

func collectScopedListings(v any, source string, out *[]map[string]any) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			collectScopedListings(item, source, out)
		}
	case map[string]any:
		if isType(x["@type"], "RealEstateListing") && isType(object(x["mainEntity"])["@type"], "Apartment") && matchingPage(stringValue(x["url"]), source) {
			*out = append(*out, x)
		}
		for _, item := range x {
			collectScopedListings(item, source, out)
		}
	}
}
func matchingPage(raw, source string) bool {
	if source == "" {
		return false
	}
	a, err := url.Parse(raw)
	if err != nil {
		return false
	}
	b, err := url.Parse(source)
	if err != nil {
		return false
	}
	return (a.Scheme == "http" || a.Scheme == "https") && a.User == nil && strings.EqualFold(a.Host, b.Host) && a.EscapedPath() == b.EscapedPath() && a.RawQuery == b.RawQuery
}

func stringValue(v any) string    { s, _ := v.(string); return strings.TrimSpace(s) }
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func isType(v any, want string) bool {
	if a, ok := v.([]any); ok {
		for _, x := range a {
			if isType(x, want) {
				return true
			}
		}
		return false
	}
	s := stringValue(v)
	return s == want || s == "https://schema.org/"+want || s == "http://schema.org/"+want
}
func collectApartments(v any, nodes *[]map[string]any) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			collectApartments(item, nodes)
		}
	case map[string]any:
		if isType(x["@type"], "Apartment") {
			*nodes = append(*nodes, x)
		}
		for _, item := range x {
			collectApartments(item, nodes)
		}
	}
}

func hasCollection(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if hasCollection(item) {
				return true
			}
		}
	case map[string]any:
		if isType(x["@type"], "ItemList") || isType(x["@type"], "CollectionPage") || isType(x["@type"], "SearchResultsPage") {
			return true
		}
		for _, item := range x {
			if hasCollection(item) {
				return true
			}
		}
	}
	return false
}
func numericText(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return stringValue(v)
}

var pilotLocality = regexp.MustCompile(`(?i)\b(baner|kharadi)\b`)

func parseApartment(n map[string]any) (extracted, error) {
	b, _ := json.Marshal(n)
	f := extracted{PropertyType: "apartment", AreaBasis: "unknown", sourceText: string(b), method: "jsonld_apartment_v1"}
	a := object(n["address"])
	if a == nil {
		return f, fmt.Errorf("Apartment requires a structured PostalAddress")
	}
	street, city := stringValue(a["streetAddress"]), stringValue(a["addressLocality"])
	f.Address = strings.TrimSpace(strings.Join([]string{street, city, stringValue(a["addressRegion"])}, ", "))
	if street == "" {
		f.Address = strings.Trim(strings.TrimSpace(city+", "+stringValue(a["addressRegion"])), ", ")
		f.extraWarnings = append(f.extraWarnings, "Publisher supplies a locality-level address only; street address and physical unit identity are unknown.")
	}
	f.State = stringValue(a["addressRegion"])
	if strings.EqualFold(f.State, "Pune") && pilotLocality.MatchString(city) {
		f.State = "Maharashtra"
		f.District = "Pune"
		f.extraWarnings = append(f.extraWarnings, "State is derived from the explicit Baner/Kharadi and Pune address tuple using the pilot administrative mapping, not a publisher-supplied state field.")
	}
	country := stringValue(a["addressCountry"])
	if country == "" {
		country = stringValue(object(a["addressCountry"])["name"])
	}
	if country != "" && !strings.EqualFold(country, "IN") && !strings.EqualFold(country, "India") {
		return f, fmt.Errorf("non-India addressCountry")
	}
	if strings.EqualFold(city, "Pune") {
		f.District = "Pune"
	}
	localities := map[string]bool{}
	for _, s := range pilotLocality.FindAllString(street+" "+city, -1) {
		localities[strings.ToLower(s)] = true
	}
	if len(localities) == 1 {
		for s := range localities {
			f.Locality = s
		}
	}
	if f.Locality == "" {
		return f, fmt.Errorf("address does not establish one pilot locality")
	}
	o := object(n["offers"])
	if o == nil {
		return f, fmt.Errorf("Apartment requires one directly associated Offer with explicit sale/rent purpose")
	}
	function := stringValue(o["businessFunction"])
	switch {
	case function == "Sell" || strings.HasSuffix(function, "/Sell") || strings.HasSuffix(function, "#Sell"):
		f.Purpose = "sale"
	case function == "LeaseOut" || strings.HasSuffix(function, "/LeaseOut") || strings.HasSuffix(function, "#LeaseOut"):
		f.Purpose = "rent"
	default:
		return f, fmt.Errorf("Offer has no explicit supported businessFunction")
	}
	f.PublishedDate = stringValue(n["datePublished"])
	price := numericText(o["price"])
	currency := stringValue(o["priceCurrency"])
	if f.Purpose == "rent" {
		spec := object(o["priceSpecification"])
		period := strings.ToLower(stringValue(spec["unitText"]))
		if period != "month" && period != "monthly" {
			price = ""
		}
	}
	if currency == "INR" && price != "" {
		if value, ok := parsePrice(price); ok {
			if f.Purpose == "sale" {
				if _, valid := parseListingPrice(price, "sale"); !valid {
					return f, fmt.Errorf("monthly price on sale Offer")
				}
			}
			kind := "asking_price"
			if f.Purpose == "rent" {
				kind = "monthly_rent"
			}
			f.parsedPrice = &domain.Price{Amount: value, Currency: "INR", Type: kind}
		}
	}
	size := object(n["floorSize"])
	unit := stringValue(size["unitCode"])
	if unit == "MTK" {
		unit = "sqm"
	} else if unit == "FTK" {
		unit = "sqft"
	} else {
		unit = stringValue(size["unitText"])
	}
	areaText := numericText(size["value"]) + " " + unit
	basis := "unknown"
	label := stringValue(size["name"])
	for _, proposed := range []string{"carpet", "built_up", "super_built_up"} {
		if sourceAreaBasis(label+" "+areaText, areaText, proposed) == proposed {
			basis = proposed
		}
	}
	f.parsedArea = parseArea(areaText, basis)
	return f, nil
}

func parseLabels(markdown string) (extracted, error) {
	f := extracted{AreaBasis: "unknown", sourceText: markdown, method: "labeled_markdown_v1"}
	labels := map[string]string{}
	headings := 0
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			headings++
		}
		// Support explicit key:value fields and two-column Markdown tables only.
		var key, value string
		if strings.HasPrefix(line, "|") {
			cells := strings.Split(strings.Trim(line, "|"), "|")
			if len(cells) != 2 {
				continue
			}
			key, value = cells[0], cells[1]
		} else {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key, value = parts[0], parts[1]
		}
		key = strings.ToLower(strings.Trim(strings.TrimSpace(key), "*- "))
		value = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(value), "*"))
		switch key {
		case "property type", "transaction type", "address", "locality", "district", "state", "price", "rent", "carpet area", "built-up area", "super built-up area", "area", "published date", "project", "unit id":
			if _, exists := labels[key]; exists {
				return f, fmt.Errorf("repeated %s label; ambiguous document", key)
			}
			labels[key] = value
		}
	}
	if headings != 1 {
		return f, fmt.Errorf("unsupported page: require one main heading and explicit property labels or Apartment JSON-LD")
	}
	typeName := strings.ToLower(labels["property type"])
	if typeName == "apartment" || typeName == "flat" {
		f.PropertyType = "apartment"
	}
	switch strings.ToLower(labels["transaction type"]) {
	case "sale", "for sale":
		f.Purpose = "sale"
	case "rent", "for rent":
		f.Purpose = "rent"
	}
	f.Address, f.Locality, f.District, f.State = labels["address"], labels["locality"], labels["district"], labels["state"]
	f.Project, f.UnitID, f.PublishedDate = labels["project"], labels["unit id"], labels["published date"]
	f.PriceText = labels["price"]
	if labels["rent"] != "" {
		if f.PriceText != "" {
			return f, fmt.Errorf("multiple price labels")
		}
		f.PriceText = labels["rent"]
	}
	for _, area := range []struct{ label, basis string }{{"carpet area", "carpet"}, {"built-up area", "built_up"}, {"super built-up area", "super_built_up"}, {"area", "unknown"}} {
		if text := labels[area.label]; text != "" {
			if f.AreaText != "" {
				return f, fmt.Errorf("multiple area measurements; require explicit review")
			}
			f.AreaText, f.AreaBasis = text, area.basis
		}
	}
	return f, nil
}
