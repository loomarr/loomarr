package arr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/arr"
	"github.com/loomarr/loomarr/internal/metadataprovider/providertest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func conn(url string) arr.Conn {
	return func() (string, string) { return url, testkit.ArrLibraryAPIKey }
}

func series(tvdb string) mp.Ref {
	return mp.Ref{Kind: mp.KindSeries, ID: mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: tvdb}}
}

func movie(tmdb string) mp.Ref {
	return mp.Ref{Kind: mp.KindMovie, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: tmdb}}
}

func TestSonarr_Conformance(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	providertest.Run(t, arr.NewSonarr(conn(lib.URL), nil), providertest.Case{
		Held:    []mp.Ref{series("90000101"), series("90000102")},
		Missing: []mp.Ref{series("90000999")},
	})
}

func TestRadarr_Conformance(t *testing.T) {
	lib := testkit.NewRadarrLibrary(t)
	providertest.Run(t, arr.NewRadarr(conn(lib.URL), nil), providertest.Case{
		Held:    []mp.Ref{movie("90000501"), movie("90000502")},
		Missing: []mp.Ref{movie("90000999")},
	})
}

func TestSonarr_FetchReturnsTheSeriesWithItsEpisodes(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	rec, err := arr.NewSonarr(conn(lib.URL), nil).Fetch(context.Background(), series("90000101"))
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []mp.ExternalID{
		{Namespace: mp.NamespaceTVDB, Value: "90000101"},
		{Namespace: mp.NamespaceTMDB, Value: "90000201"},
		{Namespace: mp.NamespaceIMDB, Value: "tt90000301"},
	}
	if !reflect.DeepEqual(rec.IDs, wantIDs) {
		t.Errorf("series ids = %+v, want %+v", rec.IDs, wantIDs)
	}
	wantDesc := mp.Descriptive{
		Title: "The Lantern Office", SortTitle: "lantern office",
		Overview:     "A night clerk at a seaside lighthouse registry keeps the lamps of a town that no longer has ships.",
		PremiereDate: "2019-09-12", Year: 2019, Genres: []string{"Drama", "Mystery"},
		Network: "Harbor Light Network", Status: "continuing", Runtime: 44 * time.Minute,
	}
	if !reflect.DeepEqual(*rec.Descriptive, wantDesc) {
		t.Errorf("series descriptive = %+v, want %+v", *rec.Descriptive, wantDesc)
	}
	if rec.Certification == nil || *rec.Certification != (mp.Certification{Rating: "TV-14", Country: "US"}) {
		t.Errorf("series certification = %+v, want TV-14 US", rec.Certification)
	}
	wantArt := []mp.ArtworkRef{
		{Kind: "poster", Ref: "https://artwork.example/series/90000101/poster.jpg"},
		{Kind: "fanart", Ref: "https://artwork.example/series/90000101/fanart.jpg"},
	}
	if !reflect.DeepEqual(rec.Artwork, wantArt) {
		t.Errorf("artwork = %+v, want %+v (the banner has only a key-bound local URL)", rec.Artwork, wantArt)
	}
	if len(rec.Children) != 4 {
		t.Fatalf("episodes = %d, want 4 (series 2's episode must not leak in)", len(rec.Children))
	}

	first := rec.Children[0]
	if !reflect.DeepEqual(first.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90001001"}}) {
		t.Errorf("episode ids = %+v, want its own TVDB episode id", first.IDs)
	}
	if !reflect.DeepEqual(first.Parent, wantIDs) {
		t.Errorf("episode parent = %+v, want the series ids", first.Parent)
	}
	if *first.Numbering != (mp.Numbering{Season: 1, Episode: 1, Absolute: 1}) {
		t.Errorf("numbering = %+v", *first.Numbering)
	}
	if first.Descriptive.Title != "First Light" || first.Descriptive.PremiereDate != "2019-09-12" || first.Descriptive.Runtime != 45*time.Minute {
		t.Errorf("episode descriptive = %+v", *first.Descriptive)
	}
	wantFile := []mp.FileRef{{Path: "/data/tv/The Lantern Office/Season 01/The Lantern Office - S01E01 - First Light.mkv", SizeBytes: 1610612736}}
	if !reflect.DeepEqual(first.Files, wantFile) {
		t.Errorf("files = %+v, want %+v", first.Files, wantFile)
	}
	wantTech := mp.Technical{
		Duration: 44*time.Minute + 12*time.Second, SizeBytes: 1610612736, VideoCodec: "x265",
		Width: 1920, Height: 1080, HDR: true, AudioCodec: "EAC3", AudioChannels: 5.1,
		AudioLanguages: []string{"eng"}, SubtitleLanguages: []string{"eng", "spa"},
	}
	if !reflect.DeepEqual(*first.Technical, wantTech) {
		t.Errorf("technical = %+v, want %+v", *first.Technical, wantTech)
	}
	if first.Certification != nil {
		t.Errorf("Sonarr rates no episode, got %+v", first.Certification)
	}

	special := rec.Children[2]
	if *special.Numbering != (mp.Numbering{Season: 0, Episode: 1, AirsBeforeSeason: 1, AirsBeforeEpisode: 2}) {
		t.Errorf("special numbering = %+v, want its air-order position (decision 38)", *special.Numbering)
	}
	if len(special.Files) != 1 || special.Technical != nil {
		t.Errorf("special without a media summary: files %+v, technical %+v; want the file and no technical facts", special.Files, special.Technical)
	}

	missing := rec.Children[3]
	if missing.Files != nil || missing.Technical != nil {
		t.Errorf("an episode with no file has files %+v, technical %+v", missing.Files, missing.Technical)
	}
}

func TestSonarr_SeriesWithoutRatingOrTMDBIDLeavesThemOut(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	rec, err := arr.NewSonarr(conn(lib.URL), nil).Fetch(context.Background(), series("90000102"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Certification != nil {
		t.Errorf("certification = %+v, want none for an unrated series", rec.Certification)
	}
	if !reflect.DeepEqual(rec.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90000102"}}) {
		t.Errorf("ids = %+v, want only the TVDB id (tmdbId 0 and no imdbId)", rec.IDs)
	}
	tech := rec.Children[0].Technical
	if tech.Duration != 22*time.Minute+31*time.Second || !tech.Interlaced || tech.Width != 640 || tech.AudioLanguages != nil || tech.HDR {
		t.Errorf("technical = %+v", *tech)
	}
}

func TestSonarr_ListPullsEverySeriesOnOneConnection(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	calls := 0
	resolve := func() (string, string) {
		calls++
		return lib.URL, testkit.ArrLibraryAPIKey
	}
	records, err := arr.NewSonarr(resolve, nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || len(records[0].Children) != 4 || len(records[1].Children) != 1 {
		t.Fatalf("listed %d series", len(records))
	}
	if calls != 1 {
		t.Errorf("connection resolved %d times in one operation, want 1", calls)
	}
	want := []string{
		"/api/v3/series",
		"/api/v3/episode?seriesId=90000011", "/api/v3/episodefile?seriesId=90000011",
		"/api/v3/episode?seriesId=90000012", "/api/v3/episodefile?seriesId=90000012",
	}
	if got := lib.Requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %v, want %v", got, want)
	}
}

func TestSonarr_ListSkipsASeriesDeletedMidPull(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	lib.Fail("/api/v3/episode?seriesId=90000011", http.StatusNotFound)
	records, err := arr.NewSonarr(conn(lib.URL), nil).List(context.Background())
	if err != nil {
		t.Fatalf("a series deleted mid-pull failed the pull: %v", err)
	}
	if len(records) != 1 || records[0].IDs[0].Value != "90000102" {
		t.Errorf("listed %+v, want only the series that still exists", records)
	}
}

func TestSonarr_ListKeepsGoingPastAFailedSeriesAndReportsIt(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	lib.Fail("/api/v3/episodefile?seriesId=90000011", http.StatusInternalServerError)
	records, err := arr.NewSonarr(conn(lib.URL), nil).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "series 90000011") {
		t.Fatalf("err = %v, want one naming the failed series", err)
	}
	if len(records) != 1 || records[0].IDs[0].Value != "90000102" {
		t.Errorf("listed %+v, want the series that read", records)
	}
}

func TestRadarr_FetchReturnsTheMovie(t *testing.T) {
	lib := testkit.NewRadarrLibrary(t)
	rec, err := arr.NewRadarr(conn(lib.URL), nil).Fetch(context.Background(), movie("90000501"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.IDs, []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000501"}, {Namespace: mp.NamespaceIMDB, Value: "tt90000601"}}) {
		t.Errorf("ids = %+v", rec.IDs)
	}
	wantDesc := mp.Descriptive{
		Title: "The Copper Orchard", OriginalTitle: "Le Verger de Cuivre", SortTitle: "copper orchard",
		Overview:     "A clockmaker inherits an orchard whose trees keep perfect time.",
		PremiereDate: "2016-05-20", Year: 2016, Genres: []string{"Drama", "Fantasy"},
		Network: "Ironbell Pictures", Runtime: 112 * time.Minute,
	}
	if !reflect.DeepEqual(*rec.Descriptive, wantDesc) {
		t.Errorf("descriptive = %+v, want %+v", *rec.Descriptive, wantDesc)
	}
	if rec.Certification == nil || *rec.Certification != (mp.Certification{Rating: "PG-13", Country: "US"}) {
		t.Errorf("certification = %+v, want PG-13 in Radarr's configured country", rec.Certification)
	}
	if rec.Technical.Duration != time.Hour+52*time.Minute+7*time.Second || rec.Technical.Width != 1920 || rec.Technical.Height != 800 {
		t.Errorf("technical = %+v", *rec.Technical)
	}
	if !reflect.DeepEqual(rec.Technical.AudioLanguages, []string{"fre", "eng"}) {
		t.Errorf("audio languages = %v", rec.Technical.AudioLanguages)
	}
	if len(rec.Files) != 1 || !strings.HasSuffix(rec.Files[0].Path, "[Bluray-1080p].mkv") {
		t.Errorf("files = %+v", rec.Files)
	}
	if len(rec.Artwork) != 1 || rec.Artwork[0].Kind != "poster" {
		t.Errorf("artwork = %+v, want the https poster only", rec.Artwork)
	}
}

func TestRadarr_MovieWithoutFileHasNoFileOrTechnicalFacts(t *testing.T) {
	lib := testkit.NewRadarrLibrary(t)
	rec, err := arr.NewRadarr(conn(lib.URL), nil).Fetch(context.Background(), movie("90000502"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Files != nil || rec.Technical != nil || rec.Descriptive.PremiereDate != "" {
		t.Errorf("files %+v, technical %+v, premiere %q", rec.Files, rec.Technical, rec.Descriptive.PremiereDate)
	}
}

func TestRadarr_RatingsAreLeftOutWhenRadarrDoesNotSayItsCountry(t *testing.T) {
	lib := testkit.NewRadarrLibrary(t)
	lib.Fail("/api/v3/config/metadata", http.StatusNotFound)
	rec, err := arr.NewRadarr(conn(lib.URL), nil).Fetch(context.Background(), movie("90000501"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Certification != nil {
		t.Errorf("certification = %+v, want none from a Radarr whose rating country is unknown", rec.Certification)
	}
}

func TestRadarr_ListReturnsEveryMovie(t *testing.T) {
	lib := testkit.NewRadarrLibrary(t)
	records, err := arr.NewRadarr(conn(lib.URL), nil).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("listed %d movies, want 2", len(records))
	}
}

func TestArr_UnconfiguredIsNotConfigured(t *testing.T) {
	empty := func() (string, string) { return " ", "" }
	if _, err := arr.NewSonarr(empty, nil).Fetch(context.Background(), series("90000101")); !errors.Is(err, mp.ErrNotConfigured) {
		t.Errorf("Sonarr = %v, want ErrNotConfigured", err)
	}
	if _, err := arr.NewRadarr(nil, nil).List(context.Background()); !errors.Is(err, mp.ErrNotConfigured) {
		t.Errorf("Radarr = %v, want ErrNotConfigured", err)
	}
}

func TestArr_RejectedKeyIsAnErrorThatCarriesNoKey(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	const secret = "not-the-key-0123456789"
	_, err := arr.NewSonarr(func() (string, string) { return lib.URL, secret }, nil).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("err = %v, want a rejected-key error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("the error carries the API key")
	}
}

func TestArr_RefusesARedirectSoTheKeyNeverLeavesTheApp(t *testing.T) {
	leaked := make(chan string, 1)
	elsewhere := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		leaked <- r.Header.Get("X-Api-Key")
	}))
	t.Cleanup(elsewhere.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	_, err := arr.NewSonarr(conn(redirector.URL), &http.Client{}).List(context.Background())
	if err == nil {
		t.Fatal("a redirect was followed to an answer")
	}
	select {
	case key := <-leaked:
		t.Fatalf("the redirect target received a request (key %t)", key != "")
	default:
	}
}

func TestArr_ServerErrorFailsTheOperation(t *testing.T) {
	lib := testkit.NewSonarrLibrary(t)
	lib.Fail("/api/v3/episodefile", http.StatusInternalServerError)
	if _, err := arr.NewSonarr(conn(lib.URL), nil).Fetch(context.Background(), series("90000101")); err == nil {
		t.Fatal("a failed episode-file read was swallowed")
	}
}

func TestArr_RefusesAMalformedID(t *testing.T) {
	for _, value := range []string{"", "0", "-4", "tt123", "12a"} {
		if _, err := arr.NewSonarr(conn("http://unused.invalid"), nil).Fetch(context.Background(), series(value)); !errors.Is(err, mp.ErrUnsupported) {
			t.Errorf("tvdb %q = %v, want ErrUnsupported", value, err)
		}
	}
}
