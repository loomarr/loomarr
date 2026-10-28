package main

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/broadcaststation"
)

type span = broadcaststation.CallSign

// Rows as CDBS and the Wikidata snapshot record them for the stations the epic (#2059) names.
func TestCallSignHistory(t *testing.T) {
	// WWOR: CDBS's first row is dated 1978, when CDBS began tracking it, not when WOR-TV started.
	wwor := spansFromHistory([]historyRow{
		{seq: 3, sign: "WWOR-TV", begin: "1987-12-21"}, {seq: 1, sign: "WOR-TV", begin: "1978-08-16"}, {seq: 2, sign: "WWOR", begin: "1987-04-29"},
	}, "WWOR-TV")
	wwor = mergeWikidata(wwor, []wikidataRow{{sign: "WOR-TV", from: "1949-01-01", to: "1987-01-01"}})
	if want := []span{{Sign: "WOR", From: "1949-01-01", To: "1987-04-29"}, {Sign: "WWOR", From: "1987-04-29"}}; !reflect.DeepEqual(wwor, want) {
		t.Fatalf("WWOR = %+v, want %+v", wwor, want)
	}

	// WKYC: CDBS knows only WKYC; Wikidata supplies WNBK and KYW before it.
	wkyc := spansFromHistory([]historyRow{{seq: 1, sign: "WKYC"}, {seq: 2, sign: "WKYC-TV", begin: "1979-06-04"}, {seq: 3, sign: "WKYC", begin: "2009-06-16"}}, "WKYC")
	wkyc = mergeWikidata(wkyc, []wikidataRow{
		{sign: "WKYC", from: "2009-01-01"}, {sign: "KYW-TV", from: "1956-01-01", to: "1965-01-01"},
		{sign: "WKYC-TV", from: "1965-01-01", to: "2009-01-01"}, {sign: "WNBK", from: "1948-01-01", to: "1956-01-01"},
	})
	if want := []span{{Sign: "WNBK", From: "1948-01-01", To: "1956-01-01"}, {Sign: "KYW", From: "1956-01-01", To: "1965-01-01"}, {Sign: "WKYC", From: "1965-01-01"}}; !reflect.DeepEqual(wkyc, want) {
		t.Fatalf("WKYC = %+v, want %+v", wkyc, want)
	}

	// Philadelphia's KYW has no recorded start; Cleveland held the call sign until 1965.
	kyw := spansFromHistory([]historyRow{{seq: 1, sign: "KYW"}, {seq: 2, sign: "KYW-TV", begin: "1979-06-15"}}, "KYW-TV")
	stations := []broadcaststation.Station{{ID: "fcc:25453", CallSigns: kyw}, {ID: "fcc:73195", CallSigns: wkyc}}
	clipReusedCallSigns(stations)
	if want := []span{{Sign: "KYW", From: "1965-01-01"}}; !reflect.DeepEqual(stations[0].CallSigns, want) {
		t.Fatalf("Philadelphia KYW = %+v, want %+v", stations[0].CallSigns, want)
	}

	// A facility with no history keeps its current call sign, open at both ends.
	if got, want := spansFromHistory(nil, "WPIX"), []span{{Sign: "WPIX"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("no history = %+v, want %+v", got, want)
	}
}

// Review findings on the first seed (#2060): shared tenures the generator must settle.
func TestSharedTenures(t *testing.T) {
	// WWNY-CD shares WWNY-TV's letters; the full-power station keeps them.
	stations := []broadcaststation.Station{
		{ID: "fcc:68851", CallSigns: []span{{Sign: "WWNY"}}},
		{ID: "fcc:16744", CallSigns: []span{{Sign: "WXXX", To: "2014-01-01"}, {Sign: "WWNY", From: "2014-01-01"}}},
	}
	facilities := map[string]facility{"68851": {}, "16744": {classA: true}}
	preferFullPower(stations, facilities)
	if want := []span{{Sign: "WXXX", To: "2014-01-01"}}; !reflect.DeepEqual(stations[1].CallSigns, want) {
		t.Fatalf("Class A after preferring full power = %+v, want %+v", stations[1].CallSigns, want)
	}
	if n := concurrentHolders(stations); n != 0 {
		t.Fatalf("%d concurrent holders remain", n)
	}

	// Wikidata dates KOY 1953-1954 and KOOL 1953-1982 for one station: KOOL keeps 1954-1982.
	got := mergeWikidata(nil, []wikidataRow{{sign: "KOOL", from: "1953-01-01", to: "1982-01-01"}, {sign: "KOY", from: "1953-01-01", to: "1954-01-01"}})
	if want := []span{{Sign: "KOY", From: "1953-01-01", To: "1954-01-01"}, {Sign: "KOOL", From: "1954-01-01", To: "1982-01-01"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("contradictory spans = %+v, want %+v", got, want)
	}

	// A wd: item matching one FCC station by call sign and years merges rather than duplicating it.
	fcc := []span{{Sign: "KSAZ"}}
	wd := []span{{Sign: "KTSP", From: "1982-01-01", To: "1994-01-01"}, {Sign: "KSAZ", From: "1994-01-01"}}
	if !sharesTenure(fcc, wd) || sharesTenure([]span{{Sign: "KSAZ", To: "1990-01-01"}}, []span{{Sign: "KSAZ", From: "1994-01-01"}}) {
		t.Fatal("sharesTenure misjudges overlapping tenures")
	}
}

func TestNormalizeCity(t *testing.T) {
	for fcc, place := range map[string]string{"NEW YORK": "New York City", "ST. LOUIS": "Saint Louis", "SECAUCUS": "Secaucus"} {
		if normalizeCity(fcc) != normalizeCity(place) {
			t.Errorf("%q does not match place %q", fcc, place)
		}
	}
}
