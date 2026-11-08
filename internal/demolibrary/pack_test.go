package demolibrary

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Every file in the committed pack is pinned and licensed for a public, all-ages site.
func TestOpenPackItemsArePinnedLicensedAndAllAges(t *testing.T) {
	p, err := EmbeddedPack()
	if err != nil {
		t.Fatal(err)
	}
	files := p.files()
	var total int64
	for _, f := range files {
		m := f.media
		total += m.Size
		if _, ok := Licences[m.Licence]; !ok || m.Licence == "" {
			t.Errorf("%s: licence %q is not CC BY or public domain", f.itemID, m.Licence)
		}
		if strings.Contains(m.Licence, "-nc") || strings.Contains(m.Licence, "-nd") || strings.Contains(m.Licence, "-sa") {
			t.Errorf("%s: licence %s carries NC, ND or SA", f.itemID, m.Licence)
		}
		if !sha256Hex.MatchString(m.SHA256) || m.Size <= 0 || !m.AllAges || m.Source == "" || m.Credit == "" {
			t.Errorf("%s: needs a sha256, a size, a source, a credit and the all-ages flag: %+v", f.itemID, m)
		}
	}
	if total > MaxPackBytes {
		t.Errorf("pack downloads %d bytes, over the %d cap", total, int64(MaxPackBytes))
	}

	programmes, longestSeries := 0, 0
	for _, pt := range p.Titles {
		if pt.Kind == Movie {
			programmes++
			continue
		}
		programmes += len(pt.Episodes)
		longestSeries = max(longestSeries, len(pt.Episodes))
	}
	if programmes < 8 || programmes > 12 || longestSeries < 3 {
		t.Errorf("pack = %d programmes, longest series %d episodes; want 8–12 with a series of several", programmes, longestSeries)
	}
	if n := len(p.Fillers); n < 10 || n > 20 {
		t.Errorf("pack = %d filler clips, want 10–20", n)
	}
	// The filler pipeline enrols a clip by its name's kind token and year (filler.KindFromName,
	// filler.EraFromName); a clip without them would sit unclassified and never reach a break.
	convention := regexp.MustCompile(`^(commercial|trailer|psa)-[a-z0-9-]+-(19[3-9]\d|20[0-3]\d)$`)
	for _, f := range p.Fillers {
		if !convention.MatchString(f.ID) {
			t.Errorf("filler id %q does not follow the <kind>-<name>-<year> convention", f.ID)
		}
	}

	lib := p.Library()
	for _, ti := range lib.Titles {
		if !strings.Contains(ti.Overview, "Licence: ") || !strings.HasPrefix(ti.ID(), "open-") {
			t.Errorf("%s %q: overview must credit its licence and the id must be open-", ti.ID(), ti.Name)
		}
	}
}

// validPack is a minimal pack that passes, for the rejection cases to break one field at a time.
func validPack() Pack {
	m := PackMedia{URL: "https://example.org/a.mp4", SHA256: strings.Repeat("ab", 32), Size: 10, Seconds: 30,
		Width: 640, Height: 480, Codec: "h264", Audio: "aac", Licence: "https://creativecommons.org/licenses/by/4.0/",
		Source: "https://example.org/a", Credit: "Someone.", AllAges: true}
	return Pack{
		Version: PackVersion,
		Titles: []PackTitle{
			{N: 1, Kind: Movie, Name: "A Film", Year: 2000, Rating: "G", Genres: []string{"Family"}, Overview: "x", Media: &m, PosterAt: 5, BackdropAt: 10},
			{N: 1, Kind: Series, Name: "A Show", Year: 1950, Rating: "TV-G", Genres: []string{"Animation"}, Overview: "x",
				Episodes: []PackEpisode{{Name: "One", Year: 1950, Overview: "x", Media: m}}, PosterAt: 1, BackdropAt: 2},
		},
		Fillers:  []PackClip{{ID: "commercial-soap-1950", Name: "Soap", Media: m}},
		Channels: []Channel{{Number: 1, Name: "Ch", Titles: []string{"open-film-01", "open-series-01"}}},
	}
}

func TestPackValidationRejectsWhatThePublicSiteMustNotServe(t *testing.T) {
	if err := func() *Pack { p := validPack(); return &p }().Validate(); err != nil {
		t.Fatalf("the valid pack fails: %v", err)
	}
	for name, tc := range map[string]struct {
		breakIt func(p *Pack)
		want    string
	}{
		"non-commercial licence": {func(p *Pack) { p.Titles[0].Media.Licence = "https://creativecommons.org/licenses/by-nc/4.0/" }, "not CC BY or public domain"},
		"no-derivatives licence": {func(p *Pack) { p.Fillers[0].Media.Licence = "https://creativecommons.org/licenses/by-nd/4.0/" }, "not CC BY or public domain"},
		"missing sha256":         {func(p *Pack) { p.Titles[1].Episodes[0].Media.SHA256 = "" }, "sha256"},
		"missing size":           {func(p *Pack) { p.Fillers[0].Media.Size = 0 }, "size"},
		"not all-ages":           {func(p *Pack) { p.Titles[0].Media.AllAges = false }, "all-ages"},
		"plain http":             {func(p *Pack) { p.Titles[0].Media.URL = "http://example.org/a.mp4" }, "https"},
		"filler under the floor": {func(p *Pack) { p.Fillers[0].Media.Seconds = 8 }, "10-second floor"},
		"art past the end":       {func(p *Pack) { p.Titles[0].BackdropAt = 30 }, "backdropAt"},
		"unknown channel title":  {func(p *Pack) { p.Channels[0].Titles = append(p.Channels[0].Titles, "open-film-09") }, "unknown title"},
		"duplicate title":        {func(p *Pack) { p.Titles = append(p.Titles, p.Titles[0]) }, "duplicate"},
		"over the size cap":      {func(p *Pack) { p.Titles[0].Media.Size = MaxPackBytes }, "exceeds"},
	} {
		t.Run(name, func(t *testing.T) {
			p := validPack()
			// The media is shared by value except the film's pointer; give each case its own.
			film := *p.Titles[0].Media
			p.Titles[0].Media = &film
			tc.breakIt(&p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestParsePackRejectsUnknownFields(t *testing.T) {
	p := validPack()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePack(raw); err != nil {
		t.Fatalf("round trip fails: %v", err)
	}
	typo := strings.Replace(string(raw), `"licence"`, `"license"`, 1)
	if _, err := ParsePack([]byte(typo)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("a misspelt licence field parsed: %v", err)
	}
}
