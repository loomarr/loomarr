package installationlocation

import (
	"net/netip"
	"testing"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	index, err := NewIndex([]Place{
		{ID: "country-US", Name: "United States", Country: "US", CountryName: "United States"},
		{ID: "country-GB", Name: "United Kingdom", Country: "GB", CountryName: "United Kingdom"},
		{ID: "5128581", Name: "New York City", Market: "New York", Country: "US", CountryName: "United States", Latitude: 40.71427, Longitude: -74.00597, Population: 8_804_190},
		{ID: "2643743", Name: "London", Market: "London", Country: "GB", CountryName: "United Kingdom", Latitude: 51.50853, Longitude: -0.12574, Population: 8_961_989},
	}, []IPRange{
		{From: "198.51.100.0", To: "198.51.100.255", Country: "US"},
		{From: "2001:db8::", To: "2001:db8::ffff", Country: "GB"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func TestSearchFindsPlacesAndCountriesWithoutExposingStorageShape(t *testing.T) {
	index := testIndex(t)

	got := index.Search("new york", 8)
	if len(got) == 0 || got[0].Label != "New York, United States" || got[0].Country != "US" || got[0].Market != "New York" {
		t.Fatalf("Search(new york) = %#v, want New York, United States with its setting values", got)
	}

	got = index.Search("united kingdom", 8)
	if len(got) == 0 || got[0].Label != "United Kingdom" || got[0].Country != "GB" || got[0].Market != "" {
		t.Fatalf("Search(country) = %#v, want a valid country-only selection", got)
	}
}

func TestGeneratedIndexContainsTheSupportedWorldData(t *testing.T) {
	index, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	got := index.Search("New York City", 1)
	if len(got) != 1 || got[0].Country != "US" || got[0].Market != "New York City" {
		t.Fatalf("generated Search(New York City) = %#v", got)
	}
	place, ok := index.LookupCountry(netip.MustParseAddr("1.1.1.1"))
	if !ok || place.Country != "AU" {
		t.Fatalf("generated LookupCountry(1.1.1.1) = %#v, %v; want AU from pinned DB-IP data", place, ok)
	}
}

func TestGeneratedIndexFindsPopulatedMunicipalities(t *testing.T) {
	index, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	got := index.Search("North Greenbush", 8)
	if len(got) == 0 || got[0].Country != "US" || got[0].Market != "North Greenbush" {
		t.Fatalf("generated Search(North Greenbush) = %#v; want North Greenbush, New York, United States", got)
	}
	if got[0].Label != "North Greenbush, United States" || got[0].Region != "New York" {
		t.Fatalf("generated North Greenbush label = %q", got[0].Label)
	}
}

func TestSearchCollapsesEquivalentCityAndMunicipalityRecords(t *testing.T) {
	index, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	got := index.Search("Westerly", 8)
	matches := 0
	for _, place := range got {
		if place.Country == "US" && place.Region == "Rhode Island" && place.Market == "Westerly" {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("generated Search(Westerly) returned %d equivalent Westerly, Rhode Island choices: %#v", matches, got)
	}
}

func TestResolveUsesCoordinatesRatherThanBrowserLanguage(t *testing.T) {
	index := testIndex(t)

	got, ok := index.Resolve(40.7128, -74.0060)
	if !ok || got.Country != "US" || got.Market != "New York" {
		t.Fatalf("Resolve(NYC) = %#v, %v; want New York US independent of browser locale", got, ok)
	}

	if _, ok := index.Resolve(91, 0); ok {
		t.Fatal("invalid latitude resolved to a place")
	}
}

func TestLookupCountryUsesPublicIPv4AndIPv6Only(t *testing.T) {
	index := testIndex(t)

	for raw, want := range map[string]string{
		"198.51.100.42": "US",
		"2001:db8::42":  "GB",
	} {
		got, ok := index.LookupCountry(netip.MustParseAddr(raw))
		if !ok || got.Country != want || got.Market != "" {
			t.Fatalf("LookupCountry(%s) = %#v, %v; want country-only %s", raw, got, ok, want)
		}
	}

	for _, raw := range []string{"127.0.0.1", "10.0.0.4", "192.168.1.9", "::1", "fd00::1"} {
		if _, ok := index.LookupCountry(netip.MustParseAddr(raw)); ok {
			t.Fatalf("private address %s produced a location suggestion", raw)
		}
	}
}

func TestSearchIsBounded(t *testing.T) {
	index := testIndex(t)
	if got := index.Search("un", 1); len(got) > 1 {
		t.Fatalf("Search returned %d results with limit 1", len(got))
	}
}
