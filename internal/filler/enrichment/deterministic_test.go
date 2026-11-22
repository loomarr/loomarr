package enrichment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/station"
)

// These tests pin the trusted brand dictionary in deterministic.go, whose entries are real brands
// (HP Sauce, Tootsie Pop), against the sidecars in testdata. A dictionary phrase is the only text
// that fires a trusted product mapping, so this file keeps the real names on purpose (#1592) and
// the demolibrary guard lists it in realDataGo. Every other enrichment test uses invented
// brands.

type sidecarFixture struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	UploadDate  string `json:"upload_date"`
	Loomarr     struct {
		OriginalName string `json:"originalName"`
		SourceID     string `json:"sourceId"`
	} `json:"loomarr"`
}

func readSignals(t *testing.T, name, hash string) enrichment.Signals {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture sidecarFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return enrichment.Signals{ClipHash: hash, Kind: "commercial", Title: fixture.Title,
		Description: fixture.Description, OriginalName: fixture.Loomarr.OriginalName,
		UploadDate: fixture.UploadDate, SourceID: fixture.Loomarr.SourceID, ObservedAt: time.Unix(100, 0).UTC()}
}

func TestAnalyzeDeterministic_TootsieUsesItemTextButNotUploadDate(t *testing.T) {
	got := statesByAxis(enrichment.AnalyzeDeterministic(readSignals(t, "tootsie-pop.info.json", "tootsie")))
	if got[enrichment.AxisRole].Value.Text != "commercial" {
		t.Fatalf("kind = %+v", got[enrichment.AxisRole])
	}
	if got[enrichment.AxisBrand].Value.Text != "Tootsie Pop" {
		t.Fatalf("brand = %+v", got[enrichment.AxisBrand])
	}
	if tags := got[enrichment.AxisProduct].Value.Tags; len(tags) != 1 || tags[0] != "candy" {
		t.Fatalf("product = %+v", got[enrichment.AxisProduct])
	}
	if got[enrichment.AxisEra].Value.Year != 0 || got[enrichment.AxisEra].Status != enrichment.StatusComplete {
		t.Fatalf("upload date became era or deterministic check was not recorded: %+v", got[enrichment.AxisEra])
	}
	// The fixture's source declares no country: the pass records an empty withdrawal, never a
	// completed location, so later passes still look for one.
	if geo, exists := got[enrichment.AxisGeography]; exists &&
		(geo.Status == enrichment.StatusComplete || geo.Value.Geography != (enrichment.Geography{})) {
		t.Fatalf("unknown geography was marked complete: %+v", geo)
	}
}

// An ad that aired on Five is located in GB by that, but it is not Five's own, so it records no
// network: a network limits where a clip airs (decision 0050).
func TestAnalyzeDeterministic_HPKeepsBroadcastYearAndTrustedCountry(t *testing.T) {
	got := statesByAxis(enrichment.AnalyzeDeterministic(readSignals(t, "hp-sauce.info.json", "hp")))
	if got[enrichment.AxisEra].Value.Year != 1999 || got[enrichment.AxisBrand].Value.Text != "HP Sauce" {
		t.Fatalf("HP facts = era %+v, brand %+v", got[enrichment.AxisEra], got[enrichment.AxisBrand])
	}
	product := got[enrichment.AxisProduct]
	if len(product.Value.Tags) != 1 || product.Value.Tags[0] != "condiments" {
		t.Fatalf("product = %+v", product)
	}
	geo := got[enrichment.AxisGeography]
	if geo.Value.Geography.Country != "GB" || geo.Value.Geography.Network != "" || geo.Evidence.Kind != enrichment.EvidenceTrustedMap {
		t.Fatalf("geography = %+v", geo)
	}
}

// A network is recorded only on a piece that presents it (#4432, decision 0050): its ID, bumper or
// promo, by kind or by wording, naming exactly one network in the network's own form. An ad taped
// off an affiliate, a call sign holding the letters, a lower-case word and a title naming two
// networks record none. The network names here are real because the rule matches real ones.
func TestAnalyzeDeterministic_ANetworkIsRecordedOnlyOnItsOwnPiece(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		kind, title, want string
	}{
		{"commercial", "Saturday Morning Lineup Promo CBS/WJW 8 1993-12-02", "CBS"},
		{"commercial", "Winter Games CBS Promo 1994", "CBS"},
		{"station_id", "NBC Station 1988", "NBC"},
		{"interstitial", "CBS Television Network-Seasons Greetings", "CBS"},
		{"commercial", "Kids' WB promo 1999", "The WB"},
		{"bumper", "Fox Kids bumper", "FOX"},
		{"commercial", "Relief Tablets Commercial CBS/WJW 8 1993-12-02", ""},
		{"commercial", "Brambly Job Growth Ad CBS/WOIO 19 1997", ""},
		{"station_id", "WCBS-TV Channel 2 sign-on", ""},
		{"commercial", "fox and hound toy promo", ""},
		{"commercial", "Promos from CBS and NBC, 1990", ""},
		// The WB by its network/call-sign form; a bare WB is also Warner Bros.
		{"unclassified", "Two Hour Event Promo WB/WPIX 11 2000-11-13", "The WB"},
		{"commercial", "Brambly Cartoon Commercial WB VHS", ""},
		// An ad or a trailer is the advertiser's, even when it calls itself a promo.
		{"commercial", "Brambly Movie Promo Commercial WB/WPIX 11 2000-11-16", ""},
		{"trailer", "Brambly Movie Trailer WB/WPIX 11 2000-11-09", ""},
		// A kids block's name is the network's branding without a cue, but not its ads.
		{"commercial", "PBS KIDS thirteen", "PBS"},
		{"commercial", "FOX Kids Commercials 1994", ""},
		{"commercial", "CBSN Brambly special promo", ""},
	} {
		state := enrichment.AnalyzeDeterministic(enrichment.Signals{ClipHash: "c", Kind: tc.kind, Title: tc.title, ObservedAt: at})
		got := ""
		for _, s := range state {
			if s.Axis == enrichment.AxisGeography {
				got = s.Value.Geography.Network
				if got != "" && (s.Value.Geography.Country != "US" || s.Evidence.Kind != enrichment.EvidenceTrustedMap) {
					t.Errorf("%q: geography = %+v (%s), want a trusted US network claim", tc.title, s.Value.Geography, s.Evidence.Kind)
				}
			}
		}
		if got != tc.want {
			t.Errorf("%s %q: network = %q, want %q", tc.kind, tc.title, got, tc.want)
		}
	}

	// An affiliate's station ID naming its network stays local to the station and carries both.
	match := &station.Match{Station: station.Station{ID: "fcc:59441", Country: "US"}, CallSign: "WJW"}
	for _, s := range enrichment.AnalyzeDeterministic(enrichment.Signals{ClipHash: "c", Kind: "station_id",
		Title: "WJW 8 CBS station ID 1993", Station: match, ObservedAt: at}) {
		if s.Axis == enrichment.AxisGeography && (s.Value.Geography.Scope != "local" ||
			s.Value.Geography.Station != "fcc:59441" || s.Value.Geography.Network != "CBS") {
			t.Errorf("affiliate station ID geography = %+v, want local to its station, network CBS", s.Value.Geography)
		}
	}
}
