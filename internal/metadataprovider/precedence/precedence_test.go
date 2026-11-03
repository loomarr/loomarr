package precedence_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
)

// Sources as the rules see them. Only measured, header, embedded-id and link carry no provider:
// they are Loomarr's own evidence. The catalog sources stand in for providers to come (TMDB,
// TVDB); the rules rank them by basis and catalog, so no provider needs to exist.
var (
	measured   = mp.Source{Basis: mp.BasisMeasured}
	header     = mp.Source{Basis: mp.BasisHeader}
	embedded   = mp.Source{Basis: mp.BasisEmbeddedID}
	link       = mp.Source{Basis: mp.BasisLink}
	sonarr     = mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager, Catalog: mp.NamespaceTVDB}
	radarr     = mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager, Catalog: mp.NamespaceTMDB}
	nfoTMDB    = mp.Source{Provider: mp.IDNFO, Basis: mp.BasisSidecar, Catalog: mp.NamespaceTMDB}
	nfoTVDB    = mp.Source{Provider: mp.IDNFO, Basis: mp.BasisSidecar, Catalog: mp.NamespaceTVDB}
	nfoUnknown = mp.Source{Provider: mp.IDNFO, Basis: mp.BasisSidecar}
	tmdbCat    = mp.Source{Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
	tvdbCat    = mp.Source{Basis: mp.BasisCatalog, Catalog: mp.NamespaceTVDB}
)

func resolver(t *testing.T) *precedence.Resolver {
	t.Helper()
	r, err := precedence.New("us")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func desc(src mp.Source, kind mp.Kind, title, overview string) mp.Record {
	return mp.Record{Source: src, Kind: kind, Descriptive: &mp.Descriptive{Title: title, Overview: overview}}
}

func tech(src mp.Source, d time.Duration, codec string, hdr bool) mp.Record {
	return mp.Record{Source: src, Kind: mp.KindMovie, Technical: &mp.Technical{Duration: d, VideoCodec: codec, HDR: hdr}}
}

func cert(src mp.Source, kind mp.Kind, rating, country string) mp.Record {
	return mp.Record{Source: src, Kind: kind, Certification: &mp.Certification{Rating: rating, Country: country}}
}

func ident(src mp.Source, kind mp.Kind, ids ...mp.ExternalID) mp.Record {
	return mp.Record{Source: src, Kind: kind, IDs: ids}
}

func tmdbID(v string) mp.ExternalID { return mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: v} }
func tvdbID(v string) mp.ExternalID { return mp.ExternalID{Namespace: mp.NamespaceTVDB, Value: v} }

func TestNew_RefusesACountryWithoutALadder(t *testing.T) {
	for _, c := range []string{"GB", "de", "", "USA"} {
		if _, err := precedence.New(c); !errors.Is(err, precedence.ErrCountryUnsupported) {
			t.Errorf("New(%q) = %v, want ErrCountryUnsupported", c, err)
		}
	}
	for _, c := range []string{"US", " us "} {
		if _, err := precedence.New(c); err != nil {
			t.Errorf("New(%q) = %v", c, err)
		}
	}
}

func TestResolve_TechnicalFacts(t *testing.T) {
	cases := []struct {
		name      string
		records   []mp.Record
		duration  time.Duration
		durFrom   mp.Source
		codec     string
		codecFrom mp.Source
		hdr       bool
		hdrFrom   mp.Source
	}{
		{
			name:     "measured beats header beats manager",
			records:  []mp.Record{tech(radarr, 3*time.Minute, "x264", true), tech(header, 2*time.Minute, "h264", false), tech(measured, time.Minute, "h264", false)},
			duration: time.Minute, durFrom: measured, codec: "h264", codecFrom: measured, hdr: false, hdrFrom: measured,
		},
		{
			name:     "header beats manager",
			records:  []mp.Record{tech(radarr, 3*time.Minute, "x264", true), tech(header, 2*time.Minute, "hevc", false)},
			duration: 2 * time.Minute, durFrom: header, codec: "hevc", codecFrom: header, hdr: false, hdrFrom: header,
		},
		{
			name:     "a lower source fills a gap but a false flag is an answer",
			records:  []mp.Record{tech(radarr, 3*time.Minute, "x264", true), tech(header, 2*time.Minute, "", false)},
			duration: 2 * time.Minute, durFrom: header, codec: "x264", codecFrom: radarr, hdr: false, hdrFrom: header,
		},
		{
			name:     "the manager's summary when nothing was read",
			records:  []mp.Record{tech(radarr, 3*time.Minute, "x264", true)},
			duration: 3 * time.Minute, durFrom: radarr, codec: "x264", codecFrom: radarr, hdr: true, hdrFrom: radarr,
		},
		{
			name:    "a sidecar or catalog is never a technical source",
			records: []mp.Record{tech(nfoTMDB, time.Hour, "vp9", true), tech(tmdbCat, time.Hour, "av1", true)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolver(t).Resolve(precedence.Input{Kind: mp.KindMovie, Records: tc.records}).Technical
			if got.Duration.Value != tc.duration || got.Duration.From != tc.durFrom {
				t.Errorf("duration = %v from %+v, want %v from %+v", got.Duration.Value, got.Duration.From, tc.duration, tc.durFrom)
			}
			if got.VideoCodec.Value != tc.codec || got.VideoCodec.From != tc.codecFrom {
				t.Errorf("codec = %q from %+v, want %q from %+v", got.VideoCodec.Value, got.VideoCodec.From, tc.codec, tc.codecFrom)
			}
			if got.HDR.Value != tc.hdr || got.HDR.From != tc.hdrFrom {
				t.Errorf("hdr = %v from %+v, want %v from %+v", got.HDR.Value, got.HDR.From, tc.hdr, tc.hdrFrom)
			}
		})
	}
}

func TestResolve_DimensionsComeFromOneSource(t *testing.T) {
	widthOnly := mp.Record{Source: header, Kind: mp.KindMovie, Technical: &mp.Technical{Width: 1920}}
	both := mp.Record{Source: radarr, Kind: mp.KindMovie, Technical: &mp.Technical{Width: 1280, Height: 720}}
	got := resolver(t).Resolve(precedence.Input{Kind: mp.KindMovie, Records: []mp.Record{widthOnly, both}}).Technical
	if got.Width.Value != 1280 || got.Height.Value != 720 || got.Width.From != radarr {
		t.Errorf("dimensions = %dx%d from %+v, want 1280x720 from one source", got.Width.Value, got.Height.Value, got.Width.From)
	}
}

func TestResolve_IdentityAndNumbering(t *testing.T) {
	cases := []struct {
		name    string
		records []mp.Record
		want    []precedence.Picked[mp.ExternalID]
	}{
		{
			name:    "manager beats embedded id beats sidecar beats link",
			records: []mp.Record{ident(link, mp.KindMovie, tmdbID("4")), ident(nfoTMDB, mp.KindMovie, tmdbID("3")), ident(embedded, mp.KindMovie, tmdbID("2")), ident(radarr, mp.KindMovie, tmdbID("1"))},
			want:    []precedence.Picked[mp.ExternalID]{{Value: tmdbID("1"), From: radarr}},
		},
		{
			name:    "embedded id beats sidecar",
			records: []mp.Record{ident(nfoTMDB, mp.KindMovie, tmdbID("3")), ident(embedded, mp.KindMovie, tmdbID("2"))},
			want:    []precedence.Picked[mp.ExternalID]{{Value: tmdbID("2"), From: embedded}},
		},
		{
			name:    "sidecar beats a confirmed link",
			records: []mp.Record{ident(link, mp.KindMovie, tmdbID("4")), ident(nfoTMDB, mp.KindMovie, tmdbID("3"))},
			want:    []precedence.Picked[mp.ExternalID]{{Value: tmdbID("3"), From: nfoTMDB}},
		},
		{
			name: "each namespace is taken from the best source that has it",
			records: []mp.Record{
				ident(nfoTMDB, mp.KindMovie, tmdbID("3"), mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: "tt90000003"}),
				ident(radarr, mp.KindMovie, tmdbID("1")),
			},
			want: []precedence.Picked[mp.ExternalID]{
				{Value: tmdbID("1"), From: radarr},
				{Value: mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: "tt90000003"}, From: nfoTMDB},
			},
		},
		{
			name:    "a catalog or header never asserts identity",
			records: []mp.Record{ident(tmdbCat, mp.KindMovie, tmdbID("9")), ident(header, mp.KindMovie, tmdbID("8"))},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolver(t).Resolve(precedence.Input{Kind: mp.KindMovie, Records: tc.records}).IDs
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ids = %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("numbering and files move whole from the best source", func(t *testing.T) {
		fromNFO := mp.Record{Source: nfoTVDB, Kind: mp.KindEpisode, Numbering: &mp.Numbering{Season: 2, Episode: 3, EpisodeEnd: 4}, Files: []mp.FileRef{{Path: "/b.mkv"}}}
		fromSonarr := mp.Record{Source: sonarr, Kind: mp.KindEpisode, Numbering: &mp.Numbering{Season: 0, Episode: 1, AirsBeforeSeason: 1, AirsBeforeEpisode: 2}, Files: []mp.FileRef{{Path: "/a.mkv"}}}
		got := resolver(t).Resolve(precedence.Input{Kind: mp.KindEpisode, Records: []mp.Record{fromNFO, fromSonarr}})
		if got.Numbering.Value != *fromSonarr.Numbering || got.Numbering.From != sonarr {
			t.Errorf("numbering = %+v from %+v, want Sonarr's whole, including its air order", got.Numbering.Value, got.Numbering.From)
		}
		if !reflect.DeepEqual(got.Files.Value, fromSonarr.Files) {
			t.Errorf("files = %+v, want Sonarr's", got.Files.Value)
		}
	})

	t.Run("an episode's own ids and its parent's resolve apart", func(t *testing.T) {
		ep := mp.Record{Source: sonarr, Kind: mp.KindEpisode, IDs: []mp.ExternalID{tvdbID("90001001")}, Parent: []mp.ExternalID{tvdbID("90000101")}}
		got := resolver(t).Resolve(precedence.Input{Kind: mp.KindEpisode, Records: []mp.Record{ep}})
		if len(got.IDs) != 1 || got.IDs[0].Value != tvdbID("90001001") || len(got.Parent) != 1 || got.Parent[0].Value != tvdbID("90000101") {
			t.Errorf("ids = %+v, parent = %+v", got.IDs, got.Parent)
		}
	})
}

func TestResolve_DescriptiveText(t *testing.T) {
	cases := []struct {
		name         string
		kind         mp.Kind
		records      []mp.Record
		title        string
		titleFrom    mp.Source
		overview     string
		overviewFrom mp.Source
	}{
		{
			name:    "a movie takes TMDB's text over its manager's and sidecar's",
			kind:    mp.KindMovie,
			records: []mp.Record{desc(nfoUnknown, mp.KindMovie, "N", "n"), desc(radarr, mp.KindMovie, "R", "r"), desc(tmdbCat, mp.KindMovie, "T", "t")},
			title:   "T", titleFrom: tmdbCat, overview: "t", overviewFrom: tmdbCat,
		},
		{
			name:    "without a TMDB catalog, Radarr's TMDB-sourced text wins",
			kind:    mp.KindMovie,
			records: []mp.Record{desc(nfoUnknown, mp.KindMovie, "N", "n"), desc(radarr, mp.KindMovie, "R", "r")},
			title:   "R", titleFrom: radarr, overview: "r", overviewFrom: radarr,
		},
		{
			name:    "a TMDB-scraped tvshow.nfo never beats Sonarr",
			kind:    mp.KindSeries,
			records: []mp.Record{desc(nfoTMDB, mp.KindSeries, "N", "n"), desc(sonarr, mp.KindSeries, "S", "s")},
			title:   "S", titleFrom: sonarr, overview: "s", overviewFrom: sonarr,
		},
		{
			name:    "a TMDB-scraped NFO never beats a TVDB catalog",
			kind:    mp.KindSeries,
			records: []mp.Record{desc(nfoTMDB, mp.KindSeries, "N", "n"), desc(tvdbCat, mp.KindSeries, "V", "v")},
			title:   "V", titleFrom: tvdbCat, overview: "v", overviewFrom: tvdbCat,
		},
		{
			name:    "among sidecars, the preferred catalog's scrape wins",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(nfoTMDB, mp.KindEpisode, "M", "m"), desc(nfoTVDB, mp.KindEpisode, "V", "v")},
			title:   "V", titleFrom: nfoTVDB, overview: "v", overviewFrom: nfoTVDB,
		},
		{
			name:    "a series takes TMDB's text over Sonarr's",
			kind:    mp.KindSeries,
			records: []mp.Record{desc(sonarr, mp.KindSeries, "S", "s"), desc(tmdbCat, mp.KindSeries, "T", "t")},
			title:   "T", titleFrom: tmdbCat, overview: "t", overviewFrom: tmdbCat,
		},
		{
			name:    "an episode takes Sonarr's TVDB text over TMDB's",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(tmdbCat, mp.KindEpisode, "T", "t"), desc(sonarr, mp.KindEpisode, "S", "s")},
			title:   "S", titleFrom: sonarr, overview: "s", overviewFrom: sonarr,
		},
		{
			name:    "a lower source fills only the gap",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(tmdbCat, mp.KindEpisode, "T", "t"), desc(sonarr, mp.KindEpisode, "S", "")},
			title:   "S", titleFrom: sonarr, overview: "t", overviewFrom: tmdbCat,
		},
		{
			name:    "the sidecar when nothing else speaks",
			kind:    mp.KindMovie,
			records: []mp.Record{desc(nfoUnknown, mp.KindMovie, "N", "n")},
			title:   "N", titleFrom: nfoUnknown, overview: "n", overviewFrom: nfoUnknown,
		},
		{
			name:    "records of another kind are ignored",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(sonarr, mp.KindSeries, "Series", "s")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolver(t).Resolve(precedence.Input{Kind: tc.kind, Records: tc.records})
			if got.Title.Value != tc.title || got.Title.From != tc.titleFrom {
				t.Errorf("title = %q from %+v, want %q from %+v", got.Title.Value, got.Title.From, tc.title, tc.titleFrom)
			}
			if got.Overview.Value != tc.overview || got.Overview.From != tc.overviewFrom {
				t.Errorf("overview = %q from %+v, want %q from %+v", got.Overview.Value, got.Overview.From, tc.overview, tc.overviewFrom)
			}
		})
	}
}

func TestResolve_Certification(t *testing.T) {
	cases := []struct {
		name      string
		kind      mp.Kind
		records   []mp.Record
		series    []mp.Record
		rating    string
		from      mp.Source
		inherited bool
	}{
		{
			name:    "the strictest grounded value wins, whoever states it",
			kind:    mp.KindMovie,
			records: []mp.Record{cert(radarr, mp.KindMovie, "PG-13", "US"), cert(nfoTMDB, mp.KindMovie, "Rated R", ""), cert(tmdbCat, mp.KindMovie, "PG", "US")},
			rating:  "R", from: nfoTMDB,
		},
		{
			name:    "TV and film systems share one ladder",
			kind:    mp.KindSeries,
			records: []mp.Record{cert(sonarr, mp.KindSeries, "TV-PG", "US"), cert(nfoTVDB, mp.KindSeries, "PG-13", "")},
			rating:  "PG-13", from: nfoTVDB,
		},
		{
			name:    "a tie keeps the higher-ranked source",
			kind:    mp.KindMovie,
			records: []mp.Record{cert(nfoTMDB, mp.KindMovie, "NC-17", ""), cert(radarr, mp.KindMovie, "R", "US")},
			rating:  "R", from: radarr,
		},
		{
			name:    "another country's rating grounds nothing",
			kind:    mp.KindMovie,
			records: []mp.Record{cert(nfoTMDB, mp.KindMovie, "18", "GB"), cert(radarr, mp.KindMovie, "R", "DE"), cert(tmdbCat, mp.KindMovie, "PG", "us")},
			rating:  "PG", from: tmdbCat,
		},
		{
			name:    "an unrated value grounds nothing",
			kind:    mp.KindMovie,
			records: []mp.Record{cert(nfoTMDB, mp.KindMovie, "Not Rated", ""), cert(radarr, mp.KindMovie, "NR", "US")},
		},
		{
			name:    "Loomarr's own evidence is not a rating source",
			kind:    mp.KindMovie,
			records: []mp.Record{cert(header, mp.KindMovie, "TV-MA", "US"), cert(embedded, mp.KindMovie, "TV-MA", "US"), cert(link, mp.KindMovie, "TV-MA", "US")},
		},
		{
			name:    "an unrated episode inherits its series' strictest",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(sonarr, mp.KindEpisode, "E", "")},
			series:  []mp.Record{cert(sonarr, mp.KindSeries, "TV-14", "US"), cert(nfoTVDB, mp.KindSeries, "TV-MA", "")},
			rating:  "TV-MA", from: nfoTVDB, inherited: true,
		},
		{
			name:    "an episode rated below its series keeps its own rating",
			kind:    mp.KindEpisode,
			records: []mp.Record{cert(nfoTVDB, mp.KindEpisode, "TV-PG", "")},
			series:  []mp.Record{cert(sonarr, mp.KindSeries, "TV-MA", "US")},
			rating:  "TV-PG", from: nfoTVDB,
		},
		{
			name:    "an episode rated above its series keeps its own rating",
			kind:    mp.KindEpisode,
			records: []mp.Record{cert(tvdbCat, mp.KindEpisode, "TV-MA", "US")},
			series:  []mp.Record{cert(sonarr, mp.KindSeries, "TV-PG", "US")},
			rating:  "TV-MA", from: tvdbCat,
		},
		{
			name:    "an episode whose only rating is ungrounded still inherits",
			kind:    mp.KindEpisode,
			records: []mp.Record{cert(nfoTVDB, mp.KindEpisode, "15", "GB")},
			series:  []mp.Record{cert(sonarr, mp.KindSeries, "TV-14", "US")},
			rating:  "TV-14", from: sonarr, inherited: true,
		},
		{
			name:    "an episode never inherits from records that are not its series",
			kind:    mp.KindEpisode,
			records: []mp.Record{desc(sonarr, mp.KindEpisode, "E", "")},
			series:  []mp.Record{cert(sonarr, mp.KindEpisode, "TV-MA", "US")},
		},
		{
			name:    "a movie never inherits",
			kind:    mp.KindMovie,
			records: []mp.Record{desc(radarr, mp.KindMovie, "M", "")},
			series:  []mp.Record{cert(sonarr, mp.KindSeries, "TV-MA", "US")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolver(t).Resolve(precedence.Input{Kind: tc.kind, Records: tc.records, Series: tc.series}).Certification
			want := precedence.Picked[precedence.Certification]{Value: precedence.Certification{Rating: tc.rating, Inherited: tc.inherited}, From: tc.from}
			if got != want {
				t.Errorf("certification = %+v, want %+v", got, want)
			}
			if got.Known() != (tc.rating != "") {
				t.Errorf("Known() = %v", got.Known())
			}
		})
	}
}

func TestResolve_Artwork(t *testing.T) {
	art := func(src mp.Source, refs ...mp.ArtworkRef) mp.Record {
		return mp.Record{Source: src, Kind: mp.KindMovie, Artwork: refs}
	}
	sidecar := mp.Source{Basis: mp.BasisSidecar}
	got := resolver(t).Resolve(precedence.Input{Kind: mp.KindMovie, Records: []mp.Record{
		art(radarr, mp.ArtworkRef{Kind: "poster", Ref: "https://m/p"}, mp.ArtworkRef{Kind: "fanart", Ref: "https://m/f"}, mp.ArtworkRef{Kind: "clearlogo", Ref: "https://m/l"}),
		art(tmdbCat, mp.ArtworkRef{Kind: "poster", Ref: "https://c/p"}, mp.ArtworkRef{Kind: "fanart", Ref: "https://c/f"}),
		art(sidecar, mp.ArtworkRef{Kind: "poster", Ref: "/media/poster.jpg"}),
	}}).Artwork
	want := []precedence.Picked[mp.ArtworkRef]{
		{Value: mp.ArtworkRef{Kind: "poster", Ref: "/media/poster.jpg"}, From: sidecar},
		{Value: mp.ArtworkRef{Kind: "fanart", Ref: "https://c/f"}, From: tmdbCat},
		{Value: mp.ArtworkRef{Kind: "clearlogo", Ref: "https://m/l"}, From: radarr},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("artwork = %+v, want %+v", got, want)
	}
}

func TestResolve_NothingKnownIsUnknown(t *testing.T) {
	got := resolver(t).Resolve(precedence.Input{Kind: mp.KindMovie})
	if got.Title.Known() || got.Certification.Known() || got.Technical.Duration.Known() || got.IDs != nil || got.Numbering.Known() {
		t.Errorf("an item with no records resolved to %+v", got)
	}
}
