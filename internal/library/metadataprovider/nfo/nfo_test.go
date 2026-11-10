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

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/nfo"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/providertest"
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
			Studios: []string{"Ironbell Pictures"}, Runtime: 112 * time.Minute,
			Keywords: []string{"Clockwork"}, Tagline: "Every tree keeps time.", Countries: []string{"France"},
			// Kodi's YouTube add-on link reads as the page it plays.
			Trailer: "https://www.youtube.com/watch?v=AbCdEfGhIjK", Edition: "Director's Cut",
			// The default rating wins over the first one listed.
			CommunityRating: 7.2,
			Collection: &mp.CollectionRef{
				ID:   mp.ExternalID{Namespace: mp.NamespaceTMDBCollection, Value: "90000701"},
				Name: "The Copper Orchard Collection",
			},
			// The cast in billing order, the actor with none last; then the crew.
			People: []mp.Person{
				{Name: "Invented Actor", Role: mp.PersonActor, Character: "The Clockmaker"},
				{Name: "Invented Costar", Role: mp.PersonActor, Character: "The Gardener", ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: "90000802"},
					Photo: "https://artwork.example/person/90000802.jpg"},
				{Name: "Invented Extra", Role: mp.PersonActor, Character: "Passer-by"},
				{Name: "Invented Director", Role: mp.PersonDirector},
				{Name: "Invented Writer", Role: mp.PersonWriter},
			},
		},
		Certification: &mp.Certification{Rating: "Rated PG-13"},
	}
	if !reflect.DeepEqual(rec, want) {
		t.Errorf("record =\n%+v\nwant\n%+v", rec, want)
	}
}

// An older scraper's NFO names its title with id tags instead of uniqueid, rates it with a bare
// <rating> and names its set as text. A bare <id> is IMDb's when it looks like it, and otherwise
// the kind's default catalog: TMDB for a movie, TVDB for a show. A uniqueid still wins its
// namespace, and a trailer that is not a web page is dropped.
func TestParse_OlderScraperTags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    mp.Kind
		doc     string
		ids     []mp.ExternalID
		catalog mp.Namespace
	}{
		{"movie by imdb id", mp.KindMovie, `<movie><id>tt90000611</id><tmdbid>90000511</tmdbid></movie>`,
			[]mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000511"}, {Namespace: mp.NamespaceIMDB, Value: "tt90000611"}}, ""},
		{"movie by a bare number", mp.KindMovie, `<movie><id>90000512</id></movie>`,
			[]mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000512"}}, mp.NamespaceTMDB},
		{"show by a bare number", mp.KindSeries, `<tvshow><id>90000513</id></tvshow>`,
			[]mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90000513"}}, mp.NamespaceTVDB},
		{"a uniqueid wins its namespace", mp.KindSeries,
			`<tvshow><uniqueid type="tvdb" default="true">90000514</uniqueid><id>90000999</id><imdbid>tt90000614</imdbid></tvshow>`,
			[]mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90000514"}, {Namespace: mp.NamespaceIMDB, Value: "tt90000614"}}, mp.NamespaceTVDB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := nfo.Parse(tc.kind, []byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rec.IDs, tc.ids) || rec.Source.Catalog != tc.catalog {
				t.Errorf("ids %v, catalog %q; want %v, %q", rec.IDs, rec.Source.Catalog, tc.ids, tc.catalog)
			}
		})
	}

	rec, err := nfo.Parse(mp.KindMovie, []byte(`<movie><rating>6.4</rating><set>The Old Set</set>`+
		`<trailer>/srv/trailers/old.mkv</trailer><ratings><rating max="10"><value>0</value></rating></ratings></movie>`))
	if err != nil {
		t.Fatal(err)
	}
	d := rec.Descriptive
	if d.CommunityRating != 6.4 || d.Trailer != "" || d.Collection == nil || *d.Collection != (mp.CollectionRef{Name: "The Old Set"}) {
		t.Errorf("rating %v, trailer %q, set %+v; want 6.4 (a zero score is no votes), none, the set by name", d.CommunityRating, d.Trailer, d.Collection)
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
	if len(rec.More) != 1 {
		t.Fatalf("further episodes = %d, want 1: each episodedetails is its own record", len(rec.More))
	}
	second := rec.More[0]
	for _, tc := range []struct {
		got                mp.Record
		episode            int
		tvdb, title, aired string
	}{
		{rec, 3, "90001023", "The Tide Ledger (1)", "2020-10-01"},
		{second, 4, "90001024", "The Tide Ledger (2)", "2020-10-08"},
	} {
		if *tc.got.Numbering != (mp.Numbering{Season: 2, Episode: tc.episode}) {
			t.Errorf("numbering = %+v, want S02E%02d", *tc.got.Numbering, tc.episode)
		}
		if !reflect.DeepEqual(tc.got.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: tc.tvdb}}) {
			t.Errorf("ids = %+v, want the episode's own TVDB id %s (decision 15)", tc.got.IDs, tc.tvdb)
		}
		if tc.got.Descriptive.Title != tc.title || tc.got.Descriptive.PremiereDate != tc.aired {
			t.Errorf("descriptive = %+v, want its own title and air date", *tc.got.Descriptive)
		}
	}
	if *rec.Certification != (mp.Certification{Rating: "TV-MA", Country: "US"}) {
		t.Errorf("certification = %+v", *rec.Certification)
	}
	if second.Certification != nil || second.Descriptive.Overview != "The storm arrives." {
		t.Errorf("the second episode borrowed the first's fields: %+v %+v", second.Certification, *second.Descriptive)
	}
}

func TestParse_UnderstandsHTMLEntities(t *testing.T) {
	rec, err := nfo.Parse(mp.KindMovie, []byte("<movie><title>Caf&eacute; &amp; Co&nbsp;</title></movie>"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Descriptive.Title != "Café & Co" {
		t.Errorf("title = %q", rec.Descriptive.Title)
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
	cases := map[string]struct {
		doc  string
		want error
	}{
		"external entity":  {`<?xml version="1.0"?><!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/hostname">]><movie><title>&x;</title></movie>`, nfo.ErrDirective},
		"internal entity":  {`<!DOCTYPE movie [<!ENTITY a "aaaaaaaaaa">]><movie><title>&a;&a;</title></movie>`, nfo.ErrDirective},
		"undefined entity": {`<movie><title>&undefined;</title></movie>`, nfo.ErrMalformed},
		"wrong root":       {`<tvshow><title>Elsewhere</title></tvshow>`, nfo.ErrMalformed},
		"unclosed":         {`<movie><title>Half`, nfo.ErrMalformed},
	}
	for name, tc := range cases {
		if _, err := nfo.Parse(mp.KindMovie, []byte(tc.doc)); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
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

func TestParse_ReadsLegacySingleByteEncodings(t *testing.T) {
	for label, title := range map[string][]byte{
		"ISO-8859-1":   []byte("Caf\xe9 Lumi\xe8re"),
		"windows-1252": []byte("Caf\xe9 \x93Lumi\xe8re\x94"),
	} {
		doc := append([]byte(`<?xml version="1.0" encoding="`+label+`"?><movie><title>`), title...)
		doc = append(doc, []byte("</title></movie>")...)
		rec, err := nfo.Parse(mp.KindMovie, doc)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		want := map[string]string{"ISO-8859-1": "Café Lumière", "windows-1252": "Café “Lumière”"}[label]
		if rec.Descriptive.Title != want {
			t.Errorf("%s title = %q, want %q", label, rec.Descriptive.Title, want)
		}
	}
	if _, err := nfo.Parse(mp.KindMovie, []byte(`<?xml version="1.0" encoding="shift_jis"?><movie/>`)); err == nil {
		t.Error("an unsupported encoding parsed")
	}
}

func TestParse_RuntimeToleratesAMinutesUnit(t *testing.T) {
	for raw, want := range map[string]time.Duration{"112": 112 * time.Minute, "112 min": 112 * time.Minute, "1h 52min": 0, "abc": 0} {
		rec, err := nfo.Parse(mp.KindMovie, []byte("<movie><runtime>"+raw+"</runtime></movie>"))
		if err != nil {
			t.Fatal(err)
		}
		if rec.Descriptive.Runtime != want {
			t.Errorf("runtime %q = %v, want %v", raw, rec.Descriptive.Runtime, want)
		}
	}
}

func TestParse_ABrokenSecondEpisodeFailsTheFile(t *testing.T) {
	doc := `<episodedetails><season>1</season><episode>1</episode></episodedetails>
<episodedetails><season>1</season><episode>2`
	if _, err := nfo.Parse(mp.KindEpisode, []byte(doc)); err == nil {
		t.Error("a truncated second episode was dropped silently")
	}
}

func TestFetch_MovieNFODescribesOnlyASingleMovieFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Movies")
	write(t, filepath.Join(dir, "movie.nfo"), []byte("<movie><title>Folder</title></movie>"))
	write(t, filepath.Join(dir, "First (2001).mkv"), nil)
	write(t, filepath.Join(dir, "Second (2002).mp4"), nil)
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(dir, "First (2001).mkv"))); !errors.Is(err, mp.ErrNotFound) {
		t.Errorf("a shared folder's movie.nfo = %v, want ErrNotFound", err)
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
	big := filepath.Join(t.TempDir(), "Big (2001)")
	write(t, filepath.Join(big, "Big (2001).nfo"), []byte("<movie><title>"+strings.Repeat("x", nfo.MaxSize)+"</title></movie>"))
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(big, "Big (2001).mkv"))); !errors.Is(err, nfo.ErrTooLarge) {
		t.Errorf("oversized NFO = %v, want ErrTooLarge", err)
	}

	linked := filepath.Join(t.TempDir(), "Linked (2002)")
	target := filepath.Join(linked, "elsewhere.xml")
	write(t, target, []byte("<movie><title>Linked</title></movie>"))
	if err := os.Symlink(target, filepath.Join(linked, "Linked (2002).nfo")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, filepath.Join(linked, "Linked (2002).mkv"))); !errors.Is(err, nfo.ErrNotRegularFile) {
		t.Errorf("symlinked NFO = %v, want ErrNotRegularFile", err)
	}

	// A refused <file>.nfo falls through to the folder's movie.nfo.
	write(t, filepath.Join(big, "movie.nfo"), []byte("<movie><title>From movie.nfo</title></movie>"))
	write(t, filepath.Join(linked, "movie.nfo"), []byte("<movie><title>From movie.nfo</title></movie>"))
	for _, media := range []string{filepath.Join(big, "Big (2001).mkv"), filepath.Join(linked, "Linked (2002).mkv")} {
		rec, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, media))
		if err != nil || rec.Descriptive.Title != "From movie.nfo" {
			t.Errorf("%s = %+v, %v; want movie.nfo after the refused sidecar", filepath.Base(media), rec.Descriptive, err)
		}
	}
}

func TestFetch_RefusesARelativePath(t *testing.T) {
	if _, err := nfo.New().Fetch(context.Background(), fileRef(mp.KindMovie, "movies/Film.mkv")); !errors.Is(err, mp.ErrUnsupported) {
		t.Errorf("relative path = %v, want ErrUnsupported", err)
	}
}
