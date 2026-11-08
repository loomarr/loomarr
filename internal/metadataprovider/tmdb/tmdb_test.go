package tmdb_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/metadataprovider/providertest"
	provider "github.com/loomarr/loomarr/internal/metadataprovider/tmdb"
	"github.com/loomarr/loomarr/internal/testkit"
	tmdbclient "github.com/loomarr/loomarr/internal/tmdb"
)

const apiKey = "test-key"

// fakeTMDB serves the recorded-shape fixtures and remembers every request it saw.
type fakeTMDB struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []*http.Request
}

func newFakeTMDB(t *testing.T) *fakeTMDB {
	t.Helper()
	f := &fakeTMDB{}
	serve := func(w http.ResponseWriter, name string) {
		_, _ = w.Write(testkit.Fixture(t, "tmdb/"+name))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/movie/90000601", func(w http.ResponseWriter, _ *http.Request) { serve(w, "provider_movie_90000601.json") })
	mux.HandleFunc("/tv/90000701", func(w http.ResponseWriter, _ *http.Request) { serve(w, "provider_tv_90000701.json") })
	mux.HandleFunc("/tv/90000701/season/1", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, "provider_tv_90000701_season_1.json")
	})
	mux.HandleFunc("/collection/90000611", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, "provider_collection_90000611.json")
	})
	mux.HandleFunc("/find/90001201", func(w http.ResponseWriter, _ *http.Request) { serve(w, "provider_find_tvdb_episode_90001201.json") })
	mux.HandleFunc("/find/90009998", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"movie_results":[],"person_results":[],"tv_results":[],"tv_episode_results":[],"tv_season_results":[]}`))
	})
	mux.HandleFunc("/movie/changes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			serve(w, "provider_movie_changes_page2.json")
			return
		}
		serve(w, "provider_movie_changes_page1.json")
	})
	mux.HandleFunc("/tv/changes", func(w http.ResponseWriter, _ *http.Request) { serve(w, "provider_tv_changes.json") })
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(context.Background()))
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Errorf("%s: bearer auth missing (the key must not ride in the URL): %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if strings.Contains(r.URL.RawQuery, apiKey) {
			t.Errorf("%s: the key is in the query string", r.URL.Path)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDB) last(t *testing.T) *http.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("TMDB saw no request")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeTMDB) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeTMDB) provider(locale provider.Locale) *provider.Provider {
	return provider.New(tmdbclient.NewWithBase(f.srv.URL, apiKey), locale)
}

func ref(kind mp.Kind, ns mp.Namespace, value string) mp.Ref {
	return mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: ns, Value: value}}
}

var (
	movieRef      = ref(mp.KindMovie, mp.NamespaceTMDB, "90000601")
	seriesRef     = ref(mp.KindSeries, mp.NamespaceTMDB, "90000701")
	episodeRef    = ref(mp.KindEpisode, mp.NamespaceTVDB, "90001201")
	collectionRef = ref(mp.KindCollection, mp.NamespaceTMDBCollection, "90000611")
)

func TestProvider_Conformance(t *testing.T) {
	providertest.Run(t, newFakeTMDB(t).provider(nil), providertest.Case{
		Held: []mp.Ref{movieRef, seriesRef, episodeRef, collectionRef},
		Missing: []mp.Ref{
			ref(mp.KindMovie, mp.NamespaceTMDB, "90009999"),
			ref(mp.KindCollection, mp.NamespaceTMDBCollection, "90009997"),
			ref(mp.KindEpisode, mp.NamespaceTVDB, "90009998"),
		},
	})
}

func TestProvider_IsInTheClosedSet(t *testing.T) {
	p := provider.New(nil, nil)
	if got, err := mp.ParseID(string(p.ID())); err != nil || got != mp.IDTMDB {
		t.Fatalf("ParseID(%q) = %q, %v", p.ID(), got, err)
	}
	caps := p.Capabilities()
	if caps.Basis != mp.BasisCatalog || caps.Catalog != mp.NamespaceTMDB {
		t.Errorf("basis/catalog = %s/%s, want catalog/tmdb", caps.Basis, caps.Catalog)
	}
	if caps.Supplies(mp.FamilyTechnical) {
		t.Error("a catalog has not seen the file, so it supplies no technical facts")
	}
	if caps.Rate.MaxConcurrent == 0 || caps.Rate.MinInterval == 0 {
		t.Errorf("rate hint %+v declares no limit; TMDB documents one", caps.Rate)
	}
}

func TestFetchMovie(t *testing.T) {
	f := newFakeTMDB(t)
	rec, err := f.provider(func() (string, string) { return "en-US", "us" }).Fetch(context.Background(), movieRef)
	if err != nil {
		t.Fatal(err)
	}

	q := f.last(t).URL.Query()
	if got := q.Get("append_to_response"); got != "external_ids,release_dates,keywords,images,videos,credits" {
		t.Errorf("append_to_response = %q: the detail call must carry everything the record stores", got)
	}
	if q.Get("language") != "en-US" || q.Get("include_image_language") != "en,null" {
		t.Errorf("language = %q, include_image_language = %q", q.Get("language"), q.Get("include_image_language"))
	}

	wantIDs := []mp.ExternalID{
		{Namespace: mp.NamespaceTMDB, Value: "90000601"},
		{Namespace: mp.NamespaceIMDB, Value: "tt9000601"},
	}
	if !reflect.DeepEqual(rec.IDs, wantIDs) {
		t.Errorf("ids = %+v, want %+v", rec.IDs, wantIDs)
	}
	d := rec.Descriptive
	if d.Title != "The Lantern Archive" || d.Year != 2019 || d.PremiereDate != "2019-06-14" || d.Runtime != 112*time.Minute {
		t.Errorf("descriptive = %+v", d)
	}
	if !reflect.DeepEqual(d.Studios, []string{"Harbor Lamp Pictures", "Second Shift Films"}) ||
		!reflect.DeepEqual(d.Genres, []string{"Adventure", "Fantasy"}) {
		t.Errorf("studios = %q, genres = %v", d.Studios, d.Genres)
	}
	if d.Tagline != "Every light is accounted for." ||
		!reflect.DeepEqual(d.Countries, []string{"United States of America", "Canada"}) {
		t.Errorf("tagline = %q, countries = %v", d.Tagline, d.Countries)
	}
	// The official YouTube trailer wins over a fan cut listed first, a teaser and another site's.
	if d.Trailer != "https://www.youtube.com/watch?v=official900" {
		t.Errorf("trailer = %q", d.Trailer)
	}
	// The cast in billing order, then the key crew once each; an editor is not key crew.
	wantPeople := []mp.Person{
		{Name: "Top Billed", Role: mp.PersonActor, Character: "Night Clerk", TMDB: "90003001",
			Photo: "https://image.tmdb.org/t/p/original/person-90003001.jpg"},
		{Name: "Second Billed", Role: mp.PersonActor, Character: "The Archivist", TMDB: "90003002"},
		{Name: "Lamp Director", Role: mp.PersonDirector, TMDB: "90003101"},
		{Name: "Ledger Writer", Role: mp.PersonWriter, TMDB: "90003102"},
	}
	if !reflect.DeepEqual(d.People, wantPeople) {
		t.Errorf("people = %+v\nwant     %+v", d.People, wantPeople)
	}
	if !reflect.DeepEqual(d.Keywords, []string{"night shift", "small town", "ledger"}) {
		t.Errorf("keywords = %v", d.Keywords)
	}
	if d.CommunityRating != 7.346 {
		t.Errorf("community rating = %v, want TMDB's vote average", d.CommunityRating)
	}
	wantCollection := &mp.CollectionRef{ID: mp.ExternalID{Namespace: mp.NamespaceTMDBCollection, Value: "90000801"}, Name: "The Lantern Archive Collection"}
	if !reflect.DeepEqual(d.Collection, wantCollection) {
		t.Errorf("collection = %+v, want %+v", d.Collection, wantCollection)
	}
	if rec.Certification == nil || rec.Certification.Rating != "PG-13" || rec.Certification.Country != "US" {
		t.Errorf("certification = %+v, want the strictest US release rating (PG-13), not GB's 15 or the edited PG", rec.Certification)
	}
	wantArt := []mp.ArtworkRef{
		{Kind: "poster", Ref: "https://image.tmdb.org/t/p/original/img-poster-en-90000601.jpg"},
		{Kind: "fanart", Ref: "https://image.tmdb.org/t/p/original/img-backdrop-textless-90000601.jpg"},
		{Kind: "clearlogo", Ref: "https://image.tmdb.org/t/p/original/img-logo-en-90000601.png"},
	}
	if !reflect.DeepEqual(rec.Artwork, wantArt) {
		t.Errorf("artwork = %+v, want a language poster, a textless backdrop and a language logo", rec.Artwork)
	}
	if rec.Technical != nil || rec.Numbering != nil || len(rec.Files) > 0 {
		t.Error("a catalog record carries no technical facts, numbering or files")
	}
}

// A collection is its name, its artwork and its movies in release order, an unreleased one last;
// a part with no id is left out.
func TestFetchCollection(t *testing.T) {
	f := newFakeTMDB(t)
	rec, err := f.provider(nil).Fetch(context.Background(), collectionRef)
	if err != nil {
		t.Fatal(err)
	}
	if f.last(t).URL.Path != "/collection/90000611" {
		t.Errorf("asked %s", f.last(t).URL.Path)
	}
	if rec.Descriptive == nil || rec.Descriptive.Title != "The Lantern Keeper Collection" {
		t.Fatalf("descriptive %+v", rec.Descriptive)
	}
	if len(rec.Artwork) == 0 {
		t.Error("no artwork")
	}
	var got []string
	for _, c := range rec.Children {
		got = append(got, c.IDs[0].Value+" "+c.Descriptive.Title)
	}
	want := []string{
		"90000612 The Lantern Keeper", "90000613 The Lantern Keeper Returns",
		"90000614 The Lantern Keeper Forever", "90000615 The Lantern Keeper: Dawn",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parts = %q, want %q", got, want)
	}
	if rec.Children[0].Descriptive.Year != 2001 || rec.Children[0].Kind != mp.KindMovie {
		t.Errorf("first part %+v", rec.Children[0])
	}
}

func TestFetchSeries(t *testing.T) {
	rec, err := newFakeTMDB(t).provider(nil).Fetch(context.Background(), seriesRef)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []mp.ExternalID{
		{Namespace: mp.NamespaceTMDB, Value: "90000701"},
		{Namespace: mp.NamespaceTVDB, Value: "90000711"},
		{Namespace: mp.NamespaceIMDB, Value: "tt9000701"},
	}
	if !reflect.DeepEqual(rec.IDs, wantIDs) {
		t.Errorf("ids = %+v, want %+v", rec.IDs, wantIDs)
	}
	d := rec.Descriptive
	if d.Title != "Tidewater Station" || !reflect.DeepEqual(d.Studios, []string{"Northlight Network"}) ||
		d.Runtime != 42*time.Minute || d.Tagline != "" || d.Trailer != "" {
		t.Errorf("descriptive = %+v", d)
	}
	if want := []mp.Person{
		{Name: "Station Lead", Role: mp.PersonActor, Character: "Keeper", TMDB: "90003202"},
		{Name: "Station Creator", Role: mp.PersonCreator, TMDB: "90003201",
			Photo: "https://image.tmdb.org/t/p/original/person-90003201.jpg"},
	}; !reflect.DeepEqual(d.People, want) {
		t.Errorf("series people = %+v", d.People)
	}
	if d.Status != "continuing" {
		t.Errorf("status = %q, want Sonarr's spelling of a returning series", d.Status)
	}
	// The layout says how big each season is; an announced season with no episodes yet is left out.
	wantSeasons := []mp.SeasonSize{{Number: 0, Episodes: 2}, {Number: 1, Episodes: 10}, {Number: 2, Episodes: 8}, {Number: 3, Episodes: 6}}
	if !reflect.DeepEqual(d.Seasons, wantSeasons) {
		t.Errorf("seasons = %+v, want %+v", d.Seasons, wantSeasons)
	}
	if !reflect.DeepEqual(d.Keywords, []string{"lighthouse", "harbor"}) {
		t.Errorf("keywords = %v: a series reads keywords from `results`", d.Keywords)
	}
	if rec.Certification == nil || rec.Certification.Rating != "TV-14" {
		t.Errorf("certification = %+v, want the US content rating, not DE's", rec.Certification)
	}
	var logo string
	var seasonPosters []mp.ArtworkRef
	for _, a := range rec.Artwork {
		switch a.Kind {
		case "clearlogo":
			logo = a.Ref
		case mp.KindSeasonPoster:
			seasonPosters = append(seasonPosters, a)
		}
	}
	if logo != "https://image.tmdb.org/t/p/original/img-logo-en-90000701.png" {
		t.Errorf("series logo = %q: logos are captured", logo)
	}
	wantPosters := []mp.ArtworkRef{{Kind: mp.KindSeasonPoster, Season: 1, Ref: "https://image.tmdb.org/t/p/original/img-season-90000722.jpg"}}
	if !reflect.DeepEqual(seasonPosters, wantPosters) {
		t.Errorf("season posters = %+v, want only season 1's, the one season with a poster", seasonPosters)
	}
	if len(rec.Children) != 0 {
		t.Error("TMDB's season layout differs from TVDB's, so a series carries no episodes; they are fetched by their own ids")
	}
	if d.Orderings == nil || *d.Orderings != 1 {
		t.Errorf("orderings = %v, want the one episode group", d.Orderings)
	}
}

// A season lists its episodes in TMDB's numbering; an episode TMDB has no id for is left out, and
// anything but "<series>/<season>" is refused.
func TestFetchSeason(t *testing.T) {
	p := newFakeTMDB(t).provider(nil)
	rec, err := p.Fetch(context.Background(), ref(mp.KindSeason, mp.NamespaceTMDB, "90000701/1"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != mp.KindSeason || len(rec.Children) != 2 {
		t.Fatalf("season = %+v, want two numbered episodes", rec)
	}
	e := rec.Children[1]
	if e.Numbering == nil || e.Numbering.Season != 1 || e.Numbering.Episode != 2 ||
		e.Descriptive.Title != "Low Water" || e.Descriptive.Runtime != 41*time.Minute ||
		!reflect.DeepEqual(e.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000792"}}) ||
		!reflect.DeepEqual(e.Parent, []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000701"}}) {
		t.Errorf("episode 2 = %+v", e)
	}
	for _, bad := range []string{"90000701", "x/1", "90000701/-1"} {
		if _, err := p.Fetch(context.Background(), ref(mp.KindSeason, mp.NamespaceTMDB, bad)); !errors.Is(err, mp.ErrUnsupported) {
			t.Errorf("%q = %v, want unsupported", bad, err)
		}
	}
}

func TestFetchEpisode_BridgesByItsOwnTVDBID(t *testing.T) {
	f := newFakeTMDB(t)
	rec, err := f.provider(nil).Fetch(context.Background(), episodeRef)
	if err != nil {
		t.Fatal(err)
	}
	req := f.last(t)
	if req.URL.Path != "/find/90001201" || req.URL.Query().Get("external_source") != "tvdb_id" {
		t.Errorf("request = %s, want /find by tvdb_id and no season/episode path", req.URL)
	}
	wantIDs := []mp.ExternalID{
		{Namespace: mp.NamespaceTVDB, Value: "90001201"},
		{Namespace: mp.NamespaceTMDB, Value: "90001301"},
	}
	if !reflect.DeepEqual(rec.IDs, wantIDs) {
		t.Errorf("ids = %+v, want %+v", rec.IDs, wantIDs)
	}
	if want := []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000701"}}; !reflect.DeepEqual(rec.Parent, want) {
		t.Errorf("parent = %+v, want the series' own TMDB id %+v", rec.Parent, want)
	}
	if rec.Numbering != nil {
		t.Errorf("numbering = %+v: TMDB's S/E can differ from the file's layout, so the record carries none", rec.Numbering)
	}
	d := rec.Descriptive
	if d.Title != "Slack Water" || d.PremiereDate != "2022-04-12" || d.Runtime != 43*time.Minute || d.CommunityRating != 7.8 {
		t.Errorf("descriptive = %+v: the episode's vote average is its community rating", d)
	}
	if rec.Certification != nil {
		t.Error("an episode takes its series' rating; TMDB's find answer carries none")
	}
	if want := []mp.ArtworkRef{{Kind: "thumb", Ref: "https://image.tmdb.org/t/p/original/episode-still-90001301.jpg"}}; !reflect.DeepEqual(rec.Artwork, want) {
		t.Errorf("artwork = %+v, want the episode's own still", rec.Artwork)
	}
}

// One language is spelled one way: a bare code takes the country as its region, none is English,
// and the record says the language it was read in.
func TestFetch_ReadsInTheHouseholdLanguage(t *testing.T) {
	f := newFakeTMDB(t)
	for _, tc := range []struct{ language, country, want, images string }{
		{"fr", "", "fr-US", "fr,null"},
		{"PT-br", "us", "pt-BR", "pt,null"},
		{"", "", "en-US", "en,null"},
	} {
		p := f.provider(func() (string, string) { return tc.language, tc.country })
		rec, err := p.Fetch(context.Background(), movieRef)
		if err != nil {
			t.Fatal(err)
		}
		q := f.last(t).URL.Query()
		if q.Get("language") != tc.want || q.Get("include_image_language") != tc.images || rec.Language != tc.want {
			t.Errorf("%q, %q: asked language=%q images=%q, record says %q; want %q", tc.language, tc.country,
				q.Get("language"), q.Get("include_image_language"), rec.Language, tc.want)
		}
		if got, err := p.Language(); err != nil || got != tc.want {
			t.Errorf("%q, %q: Language() = %q, %v; want %q", tc.language, tc.country, got, err, tc.want)
		}
	}
}

func TestFetch_RefusesWhatItCannotAnswer(t *testing.T) {
	f := newFakeTMDB(t)
	ctx := context.Background()

	t.Run("a country without a rating ladder", func(t *testing.T) {
		_, err := f.provider(func() (string, string) { return "en-US", "GB" }).Fetch(ctx, movieRef)
		if !errors.Is(err, mp.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
	})
	t.Run("a series by a TVDB id is outside the capabilities", func(t *testing.T) {
		_, err := f.provider(nil).Fetch(ctx, ref(mp.KindSeries, mp.NamespaceTVDB, "90000711"))
		if !errors.Is(err, mp.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
	})
	t.Run("an id that is not a positive integer", func(t *testing.T) {
		for _, v := range []string{"", "abc", "-4", "0"} {
			if _, err := f.provider(nil).Fetch(ctx, ref(mp.KindMovie, mp.NamespaceTMDB, v)); !errors.Is(err, mp.ErrUnsupported) {
				t.Errorf("id %q: err = %v, want ErrUnsupported", v, err)
			}
		}
	})
	t.Run("no key", func(t *testing.T) {
		p := provider.New(tmdbclient.NewWithBase(f.srv.URL, ""), nil)
		before := f.count()
		if _, err := p.Fetch(ctx, movieRef); !errors.Is(err, mp.ErrNotConfigured) {
			t.Fatalf("err = %v, want ErrNotConfigured", err)
		}
		if f.count() != before {
			t.Error("a request was sent without a key")
		}
	})
}

func TestFetch_ReadsTheLocaleOncePerOperation(t *testing.T) {
	f := newFakeTMDB(t)
	calls := 0
	p := f.provider(func() (string, string) { calls++; return "fr-FR", "US" })
	if _, err := p.Fetch(context.Background(), movieRef); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("locale read %d times for one fetch", calls)
	}
	q := f.last(t).URL.Query()
	if q.Get("language") != "fr-FR" || q.Get("include_image_language") != "fr,null" {
		t.Errorf("language = %q, include_image_language = %q", q.Get("language"), q.Get("include_image_language"))
	}
}

func TestChanges(t *testing.T) {
	f := newFakeTMDB(t)
	p := f.provider(nil)
	now := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	provider.SetClock(p, func() time.Time { return now })

	if p.Window() != 14*24*time.Hour {
		t.Fatalf("window = %s, want TMDB's 14 days", p.Window())
	}
	var feed mp.ChangeFeed = p

	refs, err := feed.Changes(context.Background(), now.Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []mp.Ref{
		ref(mp.KindMovie, mp.NamespaceTMDB, "90000601"),
		ref(mp.KindMovie, mp.NamespaceTMDB, "90000602"),
		ref(mp.KindMovie, mp.NamespaceTMDB, "90000603"),
		ref(mp.KindSeries, mp.NamespaceTMDB, "90000701"),
	}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("refs = %+v\nwant   %+v\n(pages are followed and an id on two pages appears once)", refs, want)
	}
	q := f.last(t).URL.Query()
	if q.Get("start_date") != "2026-11-01" || q.Get("end_date") != "2026-11-03" {
		t.Errorf("range = %s..%s, want 2026-11-01..2026-11-03", q.Get("start_date"), q.Get("end_date"))
	}

	t.Run("older than the window is not answered", func(t *testing.T) {
		before := f.count()
		_, err := feed.Changes(context.Background(), now.Add(-15*24*time.Hour))
		if !errors.Is(err, mp.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported so the caller refetches", err)
		}
		if f.count() != before {
			t.Error("a request was sent for a range TMDB rejects")
		}
	})
	t.Run("exactly the window is answered", func(t *testing.T) {
		if _, err := feed.Changes(context.Background(), now.Add(-14*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if q := f.last(t).URL.Query(); q.Get("start_date") != "2026-10-20" {
			t.Errorf("start_date = %s, want 2026-10-20", q.Get("start_date"))
		}
	})
}

// TestResolved_TMDBTextWinsForMoviesAndFeedsTheNewFields runs the provider's records through the
// resolver: TMDB's text outranks a Radarr record, and the keywords, rating and collection resolve
// with their provenance.
func TestResolved_TMDBTextWinsForMoviesAndFeedsTheNewFields(t *testing.T) {
	rec, err := newFakeTMDB(t).provider(nil).Fetch(context.Background(), movieRef)
	if err != nil {
		t.Fatal(err)
	}
	radarr := mp.Record{
		Source:      mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager, Catalog: mp.NamespaceTMDB},
		Kind:        mp.KindMovie,
		Descriptive: &mp.Descriptive{Title: "Radarr title", Overview: "Radarr overview"},
	}
	r, err := precedence.New("US")
	if err != nil {
		t.Fatal(err)
	}
	got := r.Resolve(precedence.Input{Kind: mp.KindMovie, Records: []mp.Record{radarr, rec}})
	if got.Title.Value != "The Lantern Archive" || got.Title.From != rec.Source {
		t.Errorf("title = %+v, want TMDB's", got.Title)
	}
	if len(got.Keywords.Value) != 3 || got.CommunityRating.Value != 7.346 || got.Collection.Value.Name != "The Lantern Archive Collection" {
		t.Errorf("keywords = %v, rating = %v, collection = %+v", got.Keywords.Value, got.CommunityRating.Value, got.Collection.Value)
	}
	if got.Collection.From != rec.Source || got.Certification.Value.Rating != "PG-13" {
		t.Errorf("collection from %+v, certification %+v", got.Collection.From, got.Certification)
	}
}
