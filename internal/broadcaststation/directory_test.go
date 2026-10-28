package broadcaststation

import "testing"

func shippedDirectory(t *testing.T) *Directory {
	t.Helper()
	revisions := SeedRevisions()
	stations, err := revisions[len(revisions)-1].Load()
	if err != nil {
		t.Fatal(err)
	}
	return NewDirectory(stations)
}

// Title shapes from the household rotation that reported the bug (#2059), plus the reused call signs
// the date has to settle. The stations are the real shipped data under test; the advertised
// products are invented (#1592).
func TestIdentifyArchiveTitles(t *testing.T) {
	d := shippedDirectory(t)
	for title, want := range map[string]struct{ station, sign, airDate string }{
		"WEWS NewsChannel 5 News Update 03/10/1991":                          {"fcc:59441", "WEWS", "1991-03-10"},
		"Brambly Lotion Commercial CBS/WJW 8 1993-12-22":                     {"fcc:73150", "WJW", "1993-12-22"},
		"Tax Reform Update - News Clip ABC/WAKR 23 1986-07-04":               {"fcc:70491", "WAKR", "1986-07-04"},
		"Weather Forecast Update with the Meteorologist on CBS/WOIO 19 1997": {"fcc:39746", "WOIO", ""},
		"Larkspur House Charity Auction PSA ABC/WABC 7 2000-01-25":           {"fcc:1328", "WABC", "2000-01-25"},
		"KYW 3 Eyewitness News 1962":                                         {"fcc:73195", "KYW", ""},
		"KYW 3 Eyewitness News 1975":                                         {"fcc:25453", "KYW", ""},
		"WOR-TV Million Dollar Movie 1984":                                   {"fcc:74197", "WOR", ""},
	} {
		got, ok := d.Identify(title, "", "", "")
		if !ok || got.Station.ID != want.station || got.CallSign != want.sign || got.AirDate != want.airDate {
			t.Errorf("%q = %+v (%v), want %s %s %q", title, got, ok, want.station, want.sign, want.airDate)
		}
	}
}

func TestIdentifyAbstains(t *testing.T) {
	d := shippedDirectory(t)
	for _, title := range []string{
		"Brambly Imports - Christmas (1997) TV Commercial", // no call sign
		"wjw news update",                 // lowercase is never a call sign
		"WJW News Update 1993",            // a bare capitalised word is not a cue
		"BRAMBLY BURGER KING 2 FOR 1",     // all-capitals: a channel number is not enough
		"WOR-TV 2010 rerun",               // WOR was WWOR by then
		"WEWS 5 and WJW 8 news swap 1991", // two stations
		"The WEEK 1 Recap",                // a number after a word, with nothing about broadcasting
		"WISH 3 Reasons",
	} {
		if got, ok := d.Identify(title, "", "", ""); ok {
			t.Errorf("%q resolved to %+v; want no station", title, got)
		}
	}
	// On-screen text is all capitals: a channel number is not a cue there.
	if got, ok := d.Identify("Brambly Lotion Commercial", "", "", "WJW 8 NEWS"); ok {
		t.Errorf("on-screen WJW 8 resolved to %+v; want no station", got)
	}
}

func TestIdentifyFilenamesAndOnScreenText(t *testing.T) {
	d := shippedDirectory(t)
	for _, c := range []struct{ originalName, visibleText, station string }{
		{"WEWS_NewsChannel_5_News_Update_1991.mp4", "", "fcc:59441"},
		{"CBS_WJW_8_promo.mpg", "", "fcc:73150"},
		{"KYW_3_news-1962.mp4", "", "fcc:73195"}, // a hyphen before a year does not make it a phone number
		{"tape04.mpg", "WEWS-TV", "fcc:59441"},
	} {
		got, ok := d.Identify("", c.originalName, "", c.visibleText)
		if !ok || got.Station.ID != c.station {
			t.Errorf("%q / %q = %+v (%v), want %s", c.originalName, c.visibleText, got, ok, c.station)
		}
	}
}

// A description names the station but its dates and phone numbers are not the broadcast's.
func TestIdentifyDatesComeFromTheTitle(t *testing.T) {
	d := shippedDirectory(t)
	got, ok := d.Identify("KYW 3 Eyewitness News 1962", "", "Digitised 1975 from a home tape. Call 1-800-555-2020.", "")
	if !ok || got.Station.ID != "fcc:73195" {
		t.Errorf("got %+v (%v), want the 1962 Cleveland KYW fcc:73195", got, ok)
	}
	got, ok = d.Identify("Brambly Lotion Commercial 1993", "", "Recorded off CBS/WJW 8. Call 1-800-555-2020.", "")
	if !ok || got.Station.ID != "fcc:73150" {
		t.Errorf("got %+v (%v), want WJW fcc:73150 from the description", got, ok)
	}
}
