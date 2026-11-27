package nfo_test

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/nfo"
)

// FuzzParse reads any document as each kind of NFO and holds what comes out to what a title page
// and the catalog can trust, however broken the sidecar a scraper, a person or a disk left:
//
//   - Nothing panics, and the same document reads the same way twice.
//   - A document is read as the kind asked for, every record in it too.
//   - Every id is well formed, one per namespace.
//   - Text is valid UTF-8, trimmed, on one line of single spaces, and holds no control character.
//   - A year is a real one, a date is a real day, a runtime is a real length, a score is out of ten,
//     and an episode's numbering is not negative.
func FuzzParse(f *testing.F) {
	for _, name := range []string{"movie.nfo", "tvshow.nfo", "episode-multi.nfo"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint8(0), data)
		f.Add(uint8(1), data)
		f.Add(uint8(2), data)
	}
	for _, doc := range []string{
		`<movie><title> Ashgrove </title><year>2015</year><runtime>112 min</runtime></movie>`,
		`<movie><year>99999</year><runtime>99999999999</runtime><rating>NaN</rating></movie>`,
		`<movie><ratings><rating max="Inf"><value>Inf</value></rating></ratings><rating>-1</rating></movie>`,
		`<movie><premiered>2015-02-30</premiered><aired>0000-00-00</aired></movie>`,
		`<tvshow><uniqueid type="tvdb" default="true">90123456</uniqueid><uniqueid type="tvdb">2</uniqueid></tvshow>`,
		`<tvshow><uniqueid type="IMDB">tt0000001</uniqueid><uniqueid type="">x</uniqueid></tvshow>`,
		`<episodedetails><season>0</season></episodedetails>`,
		`<episodedetails><season>-1</season><episode>2</episode></episodedetails><episodedetails><season>1</season><episode>3</episode></episodedetails>`,
		`<?xml version="1.0" encoding="windows-1252"?><movie><title>Caf` + "\xe9" + `</title></movie>`,
		`<movie><title>a` + "\x00\xe2\x80\x8b\u0007\t\n" + `b</title></movie>`,
		`<!DOCTYPE movie [<!ENTITY x "y">]><movie><title>&x;</title></movie>`,
		`<movie><title>&eacute;&amp;</title></movie>https://scraper.invalid/123`,
		`<movie></movie><movie>`,
		``,
	} {
		f.Add(uint8(0), []byte(doc))
		f.Add(uint8(1), []byte(doc))
		f.Add(uint8(2), []byte(doc))
	}

	kinds := []mp.Kind{mp.KindMovie, mp.KindSeries, mp.KindEpisode}
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		kind := kinds[int(which)%len(kinds)]
		rec, err := nfo.Parse(kind, data)
		again, againErr := nfo.Parse(kind, data)
		if (err == nil) != (againErr == nil) {
			t.Fatalf("read twice, the document read differently: %v / %v", err, againErr)
		}
		if err != nil {
			return
		}
		checkRecord(t, kind, rec)
		for _, more := range rec.More {
			checkRecord(t, kind, more)
		}
		// Compared once the record holds no NaN, which never equals itself.
		if !t.Failed() && !reflect.DeepEqual(rec, again) {
			t.Fatalf("read twice, the document read differently")
		}
	})
}

var (
	fuzzNamespace = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	fuzzValue     = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
)

func checkRecord(t *testing.T, kind mp.Kind, rec mp.Record) {
	t.Helper()
	if rec.Kind != kind {
		t.Errorf("read as %v, asked for %v", rec.Kind, kind)
	}
	seen := map[mp.Namespace]bool{}
	for _, id := range rec.IDs {
		if !fuzzNamespace.MatchString(string(id.Namespace)) || !fuzzValue.MatchString(id.Value) {
			t.Errorf("id %q:%q is not well formed", id.Namespace, id.Value)
		}
		if seen[id.Namespace] {
			t.Errorf("two %s ids", id.Namespace)
		}
		seen[id.Namespace] = true
	}
	if n := rec.Numbering; n != nil && (n.Season < 0 || n.Episode < 0 || n.EpisodeEnd < 0) {
		t.Errorf("numbering %+v is negative", *n)
	}
	d := rec.Descriptive
	if d == nil {
		return
	}
	for field, s := range map[string]string{
		"title": d.Title, "original title": d.OriginalTitle, "sort title": d.SortTitle, "overview": d.Overview,
		"status": d.Status, "tagline": d.Tagline, "edition": d.Edition, "trailer": d.Trailer,
	} {
		checkText(t, field, s)
	}
	for field, list := range map[string][]string{
		"genre": d.Genres, "studio": d.Studios, "keyword": d.Keywords, "country": d.Countries,
	} {
		for _, s := range list {
			if s == "" {
				t.Errorf("an empty %s", field)
			}
			checkText(t, field, s)
		}
	}
	for _, p := range d.People {
		checkText(t, "person", p.Name)
		checkText(t, "character", p.Character)
	}
	if d.Year != 0 && (d.Year < 1870 || d.Year > 2200) {
		t.Errorf("year %d", d.Year)
	}
	if d.PremiereDate != "" {
		if _, err := time.Parse(time.DateOnly, d.PremiereDate); err != nil {
			t.Errorf("premiere %q is not a day", d.PremiereDate)
		}
	}
	if d.Runtime < 0 || d.Runtime > 100*time.Hour {
		t.Errorf("runtime %v", d.Runtime)
	}
	if r := d.CommunityRating; math.IsNaN(r) || r < 0 || r > 10 {
		t.Errorf("score %v is not out of ten", r)
	}
}

func checkText(t *testing.T, field, s string) {
	t.Helper()
	if !utf8.ValidString(s) {
		t.Errorf("%s %q is not UTF-8", field, s)
	}
	if s != strings.TrimSpace(s) || strings.Contains(s, "  ") {
		t.Errorf("%s %q is not trimmed to single spaces", field, s)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			t.Errorf("%s %q holds control character %U", field, s, r)
		}
	}
}
