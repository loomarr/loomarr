package nfo_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/metadataprovider/providertest"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileRef(kind mp.Kind, path string) mp.Ref {
	return mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceFile, Value: path}}
}

// library lays out a small Kodi-convention library and returns its root.
func library(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "movies", "The Copper Orchard (2016)", "movie.nfo"), testdata(t, "movie.nfo"))
	write(t, filepath.Join(root, "tv", "The Lantern Office", "tvshow.nfo"), testdata(t, "tvshow.nfo"))
	write(t, filepath.Join(root, "tv", "The Lantern Office", "Season 02", "The Lantern Office - S02E03-E04.nfo"), testdata(t, "episode-multi.nfo"))
	return root
}

func TestNFO_Conformance(t *testing.T) {
	root := library(t)
	providertest.Run(t, nfo.New(), providertest.Case{
		Held: []mp.Ref{
			fileRef(mp.KindMovie, filepath.Join(root, "movies", "The Copper Orchard (2016)", "The Copper Orchard (2016).mkv")),
			fileRef(mp.KindSeries, filepath.Join(root, "tv", "The Lantern Office")),
			fileRef(mp.KindEpisode, filepath.Join(root, "tv", "The Lantern Office", "Season 02", "The Lantern Office - S02E03-E04.mkv")),
		},
		Missing: []mp.Ref{
			fileRef(mp.KindEpisode, filepath.Join(root, "tv", "The Lantern Office", "Season 02", "No Sidecar.mkv")),
			fileRef(mp.KindSeries, filepath.Join(root, "tv", "Absent Show")),
		},
	})
}

func TestParse_Movie(t *testing.T) {
	rec, err := nfo.Parse(mp.KindMovie, testdata(t, "movie.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	want := mp.Record{
		Source: mp.Source{Provider: mp.IDNFO, Basis: mp.BasisSidecar, Catalog: mp.NamespaceTMDB},
		Kind:   mp.KindMovie,
		// The repeated tmdb uniqueid and the malformed tvdb one are dropped.
		IDs: []mp.ExternalID{{Namespace: mp.NamespaceIMDB, Value: "tt90000601"}, {Namespace: mp.NamespaceTMDB, Value: "90000501"}},
		Descriptive: &mp.Descriptive{
			Title: "The Copper Orchard", OriginalTitle: "Le Verger de Cuivre", SortTitle: "Copper Orchard",
			Overview:     "A clockmaker inherits an orchard whose trees keep perfect time.",
			PremiereDate: "2016-05-20", Year: 2016, Genres: []string{"Drama", "Fantasy"},
			Network: "Ironbell Pictures", Runtime: 112 * time.Minute,
		},
		Certification: &mp.Certification{Rating: "Rated PG-13"},
	}
	if !reflect.DeepEqual(rec, want) {
		t.Errorf("record =\n%+v\nwant\n%+v", rec, want)
	}
}

func TestParse_SeriesIgnoresTheScraperURLAfterTheDocument(t *testing.T) {
	rec, err := nfo.Parse(mp.KindSeries, testdata(t, "tvshow.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Source.Catalog != mp.NamespaceTVDB {
		t.Errorf("catalog = %q, want the default uniqueid's namespace", rec.Source.Catalog)
	}
	if rec.Descriptive.Title != "The Lantern Office" || rec.Descriptive.Status != "Continuing" || rec.Descriptive.Year != 2019 {
		t.Errorf("descriptive = %+v", *rec.Descriptive)
	}
	if *rec.Certification != (mp.Certification{Rating: "TV-PG"}) {
		t.Errorf("certification = %+v", *rec.Certification)
	}
	if rec.Numbering != nil {
		t.Errorf("a series carries numbering %+v", rec.Numbering)
	}
}

func TestParse_MultiEpisodeFile(t *testing.T) {
	rec, err := nfo.Parse(mp.KindEpisode, testdata(t, "episode-multi.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	if *rec.Numbering != (mp.Numbering{Season: 2, Episode: 3, EpisodeEnd: 4}) {
		t.Errorf("numbering = %+v, want S02E03-E04", *rec.Numbering)
	}
	if !reflect.DeepEqual(rec.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90001023"}}) {
		t.Errorf("ids = %+v, want the first episode's own TVDB id", rec.IDs)
	}
	if *rec.Certification != (mp.Certification{Rating: "TV-MA", Country: "US"}) {
		t.Errorf("certification = %+v", *rec.Certification)
	}
	if rec.Descriptive.PremiereDate != "2020-10-01" {
		t.Errorf("premiere = %q", rec.Descriptive.PremiereDate)
	}
}

func TestParse_KeepsARatingsCountry(t *testing.T) {
	for raw, want := range map[string]mp.Certification{
		"gb:15":    {Rating: "15", Country: "GB"},
		"US : R":   {Rating: "R", Country: "US"},
		"TV-Y7":    {Rating: "TV-Y7"},
		"Rated NR": {Rating: "Rated NR"},
	} {
		rec, err := nfo.Parse(mp.KindMovie, []byte("<movie><mpaa>"+raw+"</mpaa></movie>"))
		if err != nil {
			t.Fatal(err)
		}
		if rec.Certification == nil || *rec.Certification != want {
			t.Errorf("mpaa %q = %+v, want %+v", raw, rec.Certification, want)
		}
	}
	rec, err := nfo.Parse(mp.KindMovie, []byte("<movie><mpaa> </mpaa></movie>"))
	if err != nil || rec.Certification != nil {
		t.Errorf("an empty mpaa = %+v, %v; want no certification", rec.Certification, err)
	}
}

func TestParse_RefusesUnsafeOrMalformedDocuments(t *testing.T) {
	cases := map[string]string{
		"external entity": `<?xml version="1.0"?><!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/hostname">]><movie><title>&x;</title></movie>`,
		"internal entity": `<!DOCTYPE movie [<!ENTITY a "aaaaaaaaaa">]><movie><title>&a;&a;</title></movie>`,
		"undefined entity": `<movie><title>&undefined;</title></movie>`,
		"wrong root":       `<tvshow><title>Elsewhere</title></tvshow>`,
		"unclosed":         `<movie><title>Half`,
	}
	for name, doc := range cases {
		if _, err := nfo.Parse(mp.KindMovie, []byte(doc)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	if _, err := nfo.Parse(mp.KindMovie, []byte("https://www.invented-scraper.example/movie/90000501")); !errors.Is(err, mp.ErrNotFound) {
		t.Errorf("a URL-only NFO = %v, want ErrNotFound", err)
	}
}

func TestParse_DropsMalformedIDs(t *testing.T) {
	doc := `<movie>
		<uniqueid type="tmdb">0</uniqueid>
		<uniqueid type="imdb">90000601</uniqueid>
		<uniqueid type="file">/etc/hostname</uniqueid>
		<uniqueid type="Invented Scraper">x</uniqueid>
		<uniqueid type="anidb">90000801</uniqueid>
	</movie>`
	rec, err := nfo.Parse(mp.KindMovie, []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if want := []mp.ExternalID{{Namespace: "anidb", Value: "90000801"}}; !reflect.DeepEqual(rec.IDs, want) {
		t.Errorf("ids = %+v, want only the well-formed open namespace %+v", rec.IDs, want)
	}
	if rec.Source.Catalog != "anidb" {
		t.Errorf("catalog = %q, want the only id's namespace", rec.Source.Catalog)
	}
}

func TestFetch_PrefersTheFilesOwnNFOOverMovieNFO(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "The Copper Orchard (2016)")
	write(t, filepath.Join(dir, "movie.nfo"), []byte("<movie><title>From movie.nfo</title></movie>"))
	write(t, filepath.Join(dir, "The Copper Orchard (2016).nfo"), []byte("<movie><title>From the file's NFO</title></movie>"))
	rec, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(dir, "The Copper Orchard (2016).mkv")))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Descriptive.Title != "From the file's NFO" {
		t.Errorf("title = %q", rec.Descriptive.Title)
	}
}

func TestFetch_RefusesAnOversizedOrLinkedSidecar(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "Big.nfo")
	write(t, big, []byte("<movie><title>"+strings.Repeat("x", nfo.MaxSize)+"</title></movie>"))
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(dir, "Big.mkv"))); err == nil || errors.Is(err, mp.ErrNotFound) {
		t.Errorf("oversized NFO = %v, want a size error", err)
	}

	target := filepath.Join(dir, "elsewhere.xml")
	write(t, target, []byte("<movie><title>Linked</title></movie>"))
	if err := os.Symlink(target, filepath.Join(dir, "Linked.nfo")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(dir, "Linked.mkv"))); err == nil || errors.Is(err, mp.ErrNotFound) {
		t.Errorf("symlinked NFO = %v, want it refused", err)
	}
}

func TestFetch_RefusesARelativePath(t *testing.T) {
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, "movies/Film.mkv")); !errors.Is(err, mp.ErrUnsupported) {
		t.Errorf("relative path = %v, want ErrUnsupported", err)
	}
}
