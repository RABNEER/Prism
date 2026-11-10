package firecrawl

import (
	"encoding/json"
	"strings"
	"testing"
)

func apartmentFixture() map[string]any {
	return map[string]any{"@type": "Apartment", "address": map[string]any{"streetAddress": "Test Building, Baner", "addressLocality": "Pune", "addressRegion": "Maharashtra", "addressCountry": "IN"}, "offers": map[string]any{"@type": "Offer", "businessFunction": "http://purl.org/goodrelations/v1#Sell", "price": 12000000, "priceCurrency": "INR"}, "floorSize": map[string]any{"value": 100, "unitCode": "MTK", "name": "Carpet area"}, "datePublished": "2026-10-01"}
}
func jsonLD(v any) string {
	b, _ := json.Marshal(v)
	return `<script type="application/ld+json">` + string(b) + `</script>`
}

func TestStructuredApartmentAndScopedOffers(t *testing.T) {
	n := apartmentFixture()
	// Schema businessFunction uses both URI fragments and simple names.
	n["offers"].(map[string]any)["businessFunction"] = "Sell"
	f, err := parseDocument("An apartment page", jsonLD(n))
	if err != nil {
		t.Fatal(err)
	}
	if f.parsedPrice == nil || f.parsedPrice.Amount != 12000000 || f.parsedPrice.Type != "asking_price" || f.parsedArea == nil || f.parsedArea.Basis != "carpet" || f.Locality != "baner" {
		t.Fatalf("bad extraction: %+v", f)
	}
	delete(n, "offers")
	if _, err := parseDocument("", jsonLD([]any{n, map[string]any{"@type": "Offer", "businessFunction": "Sell", "price": 999}})); err == nil {
		t.Fatal("unrelated offer borrowed")
	}
	n = apartmentFixture()
	n["offers"].(map[string]any)["businessFunction"] = "Sell"
	if _, err := parseDocument("", jsonLD([]any{n, n})); err == nil {
		t.Fatal("multiple apartment entities accepted")
	}
	if _, err := parseDocument("", jsonLD(map[string]any{"@type": "ItemList", "itemListElement": []any{n}})); err == nil {
		t.Fatal("one-result search page accepted")
	}
}

func TestRentalFrequencyCurrencyAndUnknownArea(t *testing.T) {
	n := apartmentFixture()
	offer := n["offers"].(map[string]any)
	offer["businessFunction"] = "LeaseOut"
	offer["price"] = 50000
	n["floorSize"].(map[string]any)["name"] = "Floor area"
	f, err := parseDocument("", jsonLD(n))
	if err != nil || f.parsedPrice != nil || f.parsedArea.Basis != "unknown" {
		t.Fatal("rent period or area basis inferred", f, err)
	}
	offer["priceSpecification"] = map[string]any{"unitText": "month"}
	f, err = parseDocument("", jsonLD(n))
	if err != nil || f.parsedPrice == nil || f.parsedPrice.Type != "monthly_rent" {
		t.Fatal("explicit monthly rent rejected", f, err)
	}
	offer["priceCurrency"] = "USD"
	f, err = parseDocument("", jsonLD(n))
	if err != nil || f.parsedPrice != nil {
		t.Fatal("foreign currency relabeled INR")
	}
	n["address"].(map[string]any)["addressCountry"] = "US"
	if _, err = parseDocument("", jsonLD(n)); err == nil {
		t.Fatal("foreign address accepted")
	}
}

func TestLabelsAmbiguityAndNoGuessedFields(t *testing.T) {
	page := "# Apartment\nProperty type: apartment\nTransaction type: sale\nAddress: Building, Baner, Pune\nLocality: Baner\nDistrict: Pune\nState: Maharashtra\nPrice: Call for price\nArea: 100 sqm"
	f, err := parseDocument(page, "")
	if err != nil || f.AreaBasis != "unknown" || f.Project != "" || f.UnitID != "" || f.PublishedDate != "" {
		t.Fatal("absent fields inferred", f, err)
	}
	for _, bad := range []string{page + "\nAddress: Another apartment", page + "\n# Another apartment", strings.TrimPrefix(page, "# Apartment\n"), page + "\nCarpet area: 80 sqm"} {
		if _, err := parseDocument(bad, ""); err == nil {
			t.Fatal("ambiguous page accepted")
		}
	}
	if _, err := parseDocument(page, `<script type="application/ld+json">broken</script>`); err == nil {
		t.Fatal("malformed structured data silently bypassed")
	}
}

func TestScopedListingWithQualifiedLocalityAddress(t *testing.T) {
	child := map[string]any{"@type": "Apartment", "address": map[string]any{"addressLocality": "Baner", "addressRegion": "Pune", "addressCountry": "IN"}}
	parent := map[string]any{"@type": []string{"Product", "RealEstateListing"}, "url": "http://listing.example/property/one", "mainEntity": child, "offers": map[string]any{"price": 39525000, "priceCurrency": "INR"}, "potentialAction": map[string]any{"@type": "BuyAction"}}
	f, err := parseDocumentForSource("", jsonLD(parent), "https://listing.example/property/one")
	if err != nil || f.parsedPrice == nil || f.parsedPrice.Amount != 39525000 || f.State != "Maharashtra" || f.Address != "Baner, Pune" || len(f.extraWarnings) < 2 || f.parsedArea != nil || f.PublishedDate != "" {
		t.Fatal("scoped locality extraction failed", f, err)
	}
	if _, err := parseDocumentForSource("", jsonLD(parent), "https://listing.example/property/two"); err == nil {
		t.Fatal("offer borrowed from another page")
	}
	delete(parent, "potentialAction")
	if _, err := parseDocumentForSource("", jsonLD(parent), "https://listing.example/property/one"); err == nil {
		t.Fatal("purpose inferred from discovery instead of source")
	}
}
