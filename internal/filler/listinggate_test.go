package filler

import (
	"strings"
	"testing"
)

// Invented stations: WAAA is near the place, WZZZ is not.
func TestListingGateAdmitsOnlyANearbyStationDatedInTheEra(t *testing.T) {
	gate := ListingGate{Languages: "en", Station: func(title string) (string, bool) {
		if strings.Contains(title, "WAAA") {
			return "WAAA", true
		}
		return "", false
	}}
	eras := []EraRange{{From: 1980, To: 1989}}
	for _, c := range []struct {
		title   string
		sign    string
		refused string
	}{
		{"WAAA-TV Channel 3 station ID 1984", "WAAA", ""},
		{"WAAA-TV commercial break, March 1986 (Fizzwell Cola)", "WAAA", ""},
		{"WZZZ-TV station ID 1984", "", ListingNoStation},
		{"WAAA-TV station ID", "", ListingUndated},
		{"WAAA-TV station ID 1975", "", ListingOffEra},
		{"WAAA-TV 1984 news open, digitised 2011", "", ListingOffEra},
		{"WAAA-TV 1984 uncensored late show", "", ListingUnsafe},
		{"WAAA-TV comerciales en español 1984", "", ListingLanguage},
		{"WAAA-TV Spanish commercials 1984", "", ListingLanguage},
		{"WAAA-TV реклама 1984", "", ListingLanguage},
		{"WAAA-TV English and French promos 1984", "", ListingLanguage},
	} {
		sign, refused := gate.Admit(DiscoveredRef{Title: c.title}, eras)
		if sign != c.sign || refused != c.refused {
			t.Errorf("%q: admitted by %q, refused %q; want %q, %q", c.title, sign, refused, c.sign, c.refused)
		}
	}
}

func TestListingGateAdmitsALanguageAChannelAirs(t *testing.T) {
	near := func(string) (string, bool) { return "WAAA", true }
	eras := []EraRange{{From: 1980, To: 1989}}
	for _, c := range []struct {
		languages string
		title     string
		want      string
	}{
		{"en,es", "WAAA-TV comerciales en español 1984", ""},
		{"", "WAAA-TV реклама 1984", ""},
		{"ja", "WAAA-TV 広告 1984", ""},
		{"zh", "WAAA-TV 広告 1984", ""},
		{"ko", "WAAA-TV 広告 1984", ListingLanguage},
	} {
		gate := ListingGate{Languages: c.languages, Station: near}
		if _, refused := gate.Admit(DiscoveredRef{Title: c.title}, eras); refused != c.want {
			t.Errorf("airing %q, %q: refused %q, want %q", c.languages, c.title, refused, c.want)
		}
	}
}
