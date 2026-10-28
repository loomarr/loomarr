package app

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/broadcaststation"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// A curated source names stations by hand, so a typo would switch it on for nobody. Every id must be
// in the newest shipped directory and in the source's country.
func TestShippedCuratedStationsAreInTheShippedDirectory(t *testing.T) {
	revisions := broadcaststation.SeedRevisions()
	stations, err := revisions[len(revisions)-1].Load()
	if err != nil {
		t.Fatal(err)
	}
	directory := broadcaststation.NewDirectory(stations)
	list, err := filler.CuratedSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range list.Sources {
		for _, id := range src.Stations {
			station, ok := directory.Station(id)
			if !ok {
				t.Errorf("%s: station %s is not in the shipped directory", src.Label, id)
			} else if station.Country != src.Country {
				t.Errorf("%s: station %s is in %s, the source in %q", src.Label, id, station.Country, src.Country)
			}
		}
	}
}

type fixedDirectory struct{ *broadcaststation.Directory }

func (d fixedDirectory) get(context.Context) (*broadcaststation.Directory, error) {
	return d.Directory, nil
}

// The open-search gate admits a title naming a station near some current place, held on the
// title's date (#2059 decision 11). Invented stations: fcc:1 near "harbor" was WAAA until
// 1987-04-29 and WBBB after; fcc:3 near a channel's "lakeside" is WCCC; fcc:9 is WZZZ, near nothing.
func TestOpenSearchGateAdmitsTheStationsNearEveryPlace(t *testing.T) {
	directory := broadcaststation.NewDirectory([]broadcaststation.Station{
		{ID: "fcc:1", Country: "US", CallSigns: []broadcaststation.CallSign{
			{Sign: "WAAA", To: "1987-04-29"}, {Sign: "WBBB", From: "1987-04-29"}}},
		{ID: "fcc:3", Country: "US", CallSigns: []broadcaststation.CallSign{{Sign: "WCCC"}}},
		{ID: "fcc:9", Country: "US", CallSigns: []broadcaststation.CallSign{{Sign: "WZZZ"}}},
	})
	search := openArchiveSearch{
		channels: &retirementFake{channels: []store.Channel{channelAt("US", "lakeside")}},
		policy:   retirementPolicy("US"), stations: fixedDirectory{directory},
		languages: func(context.Context) (string, error) { return "en", nil },
	}
	gate, err := search.Gate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	eras := []filler.EraRange{{From: 1980, To: 1999}}
	for title, want := range map[string]string{
		"WAAA-TV station ID 1984": "WAAA",
		"WBBB-TV station ID 1990": "WBBB",
		"WCCC-TV news open 1992":  "WCCC",
		"WBBB-TV station ID 1984": "",
		"WZZZ-TV station ID 1984": "",
		"Fizzwell Cola ad, 1984":  "",
	} {
		if got, _ := gate.Admit(filler.DiscoveredRef{Title: title}, eras); got != want {
			t.Errorf("%q admitted by %q, want %q", title, got, want)
		}
	}
}

// Invented stations: fcc:1 was WAAA until 1987-04-29 and WBBB after; fcc:2 has always been WCCC;
// fcc:3 is out of reach.
func TestNearbyCallSignsAreThoseHeldDuringTheEraGaps(t *testing.T) {
	directory := broadcaststation.NewDirectory([]broadcaststation.Station{
		{ID: "fcc:1", Country: "US", CallSigns: []broadcaststation.CallSign{
			{Sign: "WAAA", To: "1987-04-29"}, {Sign: "WBBB", From: "1987-04-29"}}},
		{ID: "fcc:2", Country: "US", CallSigns: []broadcaststation.CallSign{{Sign: "WCCC"}}},
		{ID: "fcc:3", Country: "US", CallSigns: []broadcaststation.CallSign{{Sign: "WDDD"}}},
	})
	near := map[string]bool{"fcc:1": true, "fcc:2": true, "fcc:404": true}
	for _, c := range []struct {
		name string
		eras []filler.EraRange
		want []string
	}{
		{"before the change", []filler.EraRange{{From: 1980, To: 1986}}, []string{"WAAA", "WCCC"}},
		{"after it", []filler.EraRange{{From: 1990, To: 1999}}, []string{"WBBB", "WCCC"}},
		{"across it", []filler.EraRange{{From: 1985, To: 1989}}, []string{"WAAA", "WBBB", "WCCC"}},
		{"no era gap", nil, []string{"WAAA", "WBBB", "WCCC"}},
	} {
		if got := nearbyCallSigns(near, c.eras, directory); !slices.Equal(got, c.want) {
			t.Errorf("%s: call signs %v, want %v", c.name, got, c.want)
		}
	}
}
