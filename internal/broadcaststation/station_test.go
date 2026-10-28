package broadcaststation

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
)

func TestNormalizeCallSign(t *testing.T) {
	for in, want := range map[string]string{
		"WEWS-TV": "WEWS", "wjw": "WJW", " KYW-DT ": "KYW", "WABC-CD": "WABC",
		"NEW": "", "VACANT": "", "CBLFT-5": "", "XHFI": "", "W1ABC": "", "": "",
	} {
		if got := NormalizeCallSign(in); got != want {
			t.Errorf("NormalizeCallSign(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSeedRoundTripsAndRejectsInvalidStations(t *testing.T) {
	stations := []Station{
		{ID: "fcc:74197", Country: "US", City: "Secaucus", Region: "NJ", Latitude: 40.78096, Longitude: -74.06624,
			CallSigns: []CallSign{{Sign: "WOR", From: "1949-01-01", To: "1987-04-29"}, {Sign: "WWOR", From: "1987-04-29"}}},
		{ID: "wd:Q1", Country: "US", Latitude: 1, Longitude: 2, CallSigns: []CallSign{{Sign: "KAAA"}}},
	}
	var buf bytes.Buffer
	if err := EncodeSeed(&buf, stations); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeSeed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, stations) {
		t.Fatalf("round trip = %+v, want %+v", got, stations)
	}

	for name, bad := range map[string]Station{
		"unprefixed id":  {ID: "74197", Country: "US", CallSigns: []CallSign{{Sign: "WOR"}}},
		"suffixed sign":  {ID: "fcc:1", Country: "US", CallSigns: []CallSign{{Sign: "WOR-TV"}}},
		"inverted span":  {ID: "fcc:1", Country: "US", CallSigns: []CallSign{{Sign: "WOR", From: "1990-01-01", To: "1980-01-01"}}},
		"no call signs":  {ID: "fcc:1", Country: "US"},
		"lowercase land": {ID: "fcc:1", Country: "us", CallSigns: []CallSign{{Sign: "WOR"}}},
	} {
		if err := EncodeSeed(&bytes.Buffer{}, []Station{bad}); err == nil {
			t.Errorf("%s: encoded an invalid station", name)
		}
	}
}

// The shipped seed loads, and carries the history the reported Cleveland and New York cases need.
func TestShippedSeed(t *testing.T) {
	revisions := SeedRevisions()
	stations, err := revisions[len(revisions)-1].Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) < 1500 {
		t.Fatalf("the shipped seed has %d stations; full-power US television alone is about 1,700", len(stations))
	}
	byID := map[string]Station{}
	for _, s := range stations {
		byID[s.ID] = s
	}
	for id, want := range map[string]struct {
		city string
		sign CallSign
	}{
		"fcc:59441": {"Cleveland", CallSign{Sign: "WEWS"}},
		"fcc:73195": {"Cleveland", CallSign{Sign: "KYW", From: "1956-01-01", To: "1965-01-01"}},
		"fcc:25453": {"Philadelphia", CallSign{Sign: "KYW", From: "1965-01-01"}},
		"fcc:74197": {"Secaucus", CallSign{Sign: "WOR", From: "1949-01-01", To: "1987-04-29"}},
		"fcc:1328":  {"New York City", CallSign{Sign: "WABC", From: "1953-01-01"}},
	} {
		s, ok := byID[id]
		if !ok {
			t.Errorf("%s is missing from the shipped seed", id)
			continue
		}
		if s.City != want.city || !slices.Contains(s.CallSigns, want.sign) {
			t.Errorf("%s = %s %+v, want %s with %+v", id, s.City, s.CallSigns, want.city, want.sign)
		}
	}
}
