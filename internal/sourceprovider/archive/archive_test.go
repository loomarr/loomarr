package archive_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/sourceprovider"
	"github.com/loomarr/loomarr/internal/sourceprovider/archive"
)

// fakeCatalog lists items without runtimes, as archive.org's one-request search does, and fills
// them in only when asked for per-item metadata.
type fakeCatalog struct {
	listed    []string
	runtimes  map[string]int
	hits      map[string][]string
	searchErr map[string]error

	listLimit    int
	searched     []string
	searchLimits []int
	enriched     int
	// allowed is how long the enrichment was given before its deadline.
	allowed time.Duration
}

func (f *fakeCatalog) EnumerateCollection(_ context.Context, _ string, limit int) (clipfetch.DiscoveryResult, error) {
	f.listLimit = limit
	return discovered(f.listed...), nil
}

func (f *fakeCatalog) SearchCollection(_ context.Context, ref, query string, limit int) (clipfetch.DiscoveryResult, error) {
	f.searched = append(f.searched, ref+"?"+query)
	f.searchLimits = append(f.searchLimits, limit)
	return discovered(f.hits[query]...), f.searchErr[query]
}

func (f *fakeCatalog) Enrich(ctx context.Context, items []clipfetch.DiscoveredItem) {
	f.enriched++
	if deadline, ok := ctx.Deadline(); ok {
		f.allowed = time.Until(deadline)
	}
	for i := range items {
		items[i].DurationMS = f.runtimes[items[i].ID]
	}
}

func discovered(ids ...string) clipfetch.DiscoveryResult {
	var res clipfetch.DiscoveryResult
	for _, id := range ids {
		res.Items = append(res.Items, clipfetch.DiscoveredItem{ID: id, Title: "title " + id, Year: 1990, Height: 480})
	}
	res.Total = len(res.Items)
	return res
}

func ids(items []sourceprovider.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func list(t *testing.T, p *archive.Provider, cp sourceprovider.Checkpoint, req sourceprovider.ListRequest) sourceprovider.Listing {
	t.Helper()
	listing, err := p.List(t.Context(), sourceprovider.Source{ID: "s", URI: "coll", Checkpoint: cp}, req)
	if err != nil {
		t.Fatal(err)
	}
	return listing
}

func TestProvider_IsFetchedAndSearchable(t *testing.T) {
	p := archive.New(archive.Config{Catalog: &fakeCatalog{}})
	if p.ID() != "archive" || !p.Describe().Fetched {
		t.Fatalf("provider = %q %+v, want the fetched archive provider", p.ID(), p.Describe())
	}
	var _ sourceprovider.Lister = p
	var _ sourceprovider.Searcher = p
}

// A page full of items already held must still yield new ones, so a check reads several times the
// budget it will take; the budget before the pool's shortfall cut sizes it.
func TestList_OverListsFromTheBudgetBeforeTheShortfallCut(t *testing.T) {
	catalog := &fakeCatalog{listed: []string{"a", "b"}}
	got := list(t, archive.New(archive.Config{Catalog: catalog}), sourceprovider.Checkpoint{}, sourceprovider.ListRequest{Take: 1, List: 2})
	if catalog.listLimit != 8 {
		t.Fatalf("read %d items, want 4 times the 2-item budget", catalog.listLimit)
	}
	if !reflect.DeepEqual(ids(got.Items), []string{"a", "b"}) {
		t.Fatalf("offered %v, want the whole page: choosing is the selection's", ids(got.Items))
	}
	if got.Items[0].URL != "https://archive.org/details/a" || got.Items[0].ObservedYear != 1990 || got.Items[0].Height != 480 {
		t.Fatalf("item = %+v, want its details page and catalogued metadata", got.Items[0])
	}
}

// The provider does not resume: it hands its checkpoint back, and each check starts a new summary.
func TestList_DoesNotResume(t *testing.T) {
	cp := sourceprovider.Checkpoint{Cursor: "c", Watermark: "w", PendingWatermark: "p"}
	got := list(t, archive.New(archive.Config{Catalog: &fakeCatalog{listed: []string{"a"}}}), cp, sourceprovider.ListRequest{Take: 1, List: 1})
	if got.Checkpoint != cp || !got.ReplaceOutcomes {
		t.Fatalf("checkpoint %+v replace=%v, want the given checkpoint back and a new summary", got.Checkpoint, got.ReplaceOutcomes)
	}
}

// Only the caller's rejection applies: another provider's metadata screen does not, so a title that
// names its length is an ordinary item here.
func TestList_AppliesOnlyTheCallersRejection(t *testing.T) {
	catalog := &fakeCatalog{listed: []string{"reel", "spot"}}
	got := list(t, archive.New(archive.Config{Catalog: catalog}), sourceprovider.Checkpoint{}, sourceprovider.ListRequest{
		Take: 5, List: 5, Reject: func(item sourceprovider.Item) (sourceprovider.Outcome, bool) {
			return sourceprovider.OutcomeTooLong, item.ID == "reel"
		},
	})
	if !reflect.DeepEqual(ids(got.Items), []string{"spot"}) || got.Outcomes[sourceprovider.OutcomeTooLong] != 1 {
		t.Fatalf("got %v %v", ids(got.Items), got.Outcomes)
	}
}

// A short pass searches the source for the call signs it is short of and takes the hits ahead of
// the listing, each marked with the call sign that found it; a hit also in the listing is offered
// once.
func TestList_PutsStationHitsAheadOfTheListing(t *testing.T) {
	catalog := &fakeCatalog{
		listed: []string{"listed", "waaa-news"},
		hits:   map[string][]string{"WAAA": {"waaa-news", "waaa-ident"}, "WBBB": {"waaa-ident"}},
	}
	got := list(t, archive.New(archive.Config{Catalog: catalog}), sourceprovider.Checkpoint{}, sourceprovider.ListRequest{
		Take: 3, List: 3, Stations: []string{"WAAA", "WBBB"}, Now: time.Unix(0, 0),
	})
	if !reflect.DeepEqual(ids(got.Items), []string{"waaa-news", "waaa-ident", "listed"}) {
		t.Fatalf("offered %v, want the hits ahead of the listing, none twice", ids(got.Items))
	}
	stations := map[string]string{}
	for _, item := range got.Items {
		stations[item.ID] = item.Station
	}
	if !reflect.DeepEqual(stations, map[string]string{"waaa-news": "WAAA", "waaa-ident": "WAAA", "listed": ""}) {
		t.Fatalf("stations = %v, want the first call sign to find a hit credited", stations)
	}
	if !reflect.DeepEqual(catalog.searchLimits, []int{3, 3}) {
		t.Fatalf("search limits = %v, want the budget", catalog.searchLimits)
	}
}

// The call signs rotate by the hour and are bounded per source, so a long list is worked through
// across passes rather than in one burst.
func TestList_RotatesAndBoundsItsStationSearches(t *testing.T) {
	signs := []string{"W1", "W2", "W3", "W4", "W5", "W6"}
	for _, tc := range []struct {
		hour int64
		want []string
	}{
		{0, []string{"coll?W1", "coll?W2", "coll?W3", "coll?W4"}},
		{4, []string{"coll?W5", "coll?W6", "coll?W1", "coll?W2"}},
	} {
		catalog := &fakeCatalog{}
		list(t, archive.New(archive.Config{Catalog: catalog}), sourceprovider.Checkpoint{}, sourceprovider.ListRequest{
			Take: 1, List: 1, Stations: signs, Now: time.Unix(tc.hour*3600, 0),
		})
		if !reflect.DeepEqual(catalog.searched, tc.want) {
			t.Errorf("hour %d searched %v, want %v", tc.hour, catalog.searched, tc.want)
		}
	}
}

// A search that fails costs only its hits: the listing still runs the pass, and the failure is
// logged.
func TestList_ASearchFailureCostsOnlyItsHits(t *testing.T) {
	var logged bytes.Buffer
	catalog := &fakeCatalog{
		listed:    []string{"listed"},
		hits:      map[string][]string{"WBBB": {"wbbb-1"}},
		searchErr: map[string]error{"WAAA": errors.New("archive.org down")},
	}
	p := archive.New(archive.Config{Catalog: catalog, Log: slog.New(slog.NewTextHandler(&logged, nil))})
	got := list(t, p, sourceprovider.Checkpoint{}, sourceprovider.ListRequest{
		Take: 2, List: 2, Stations: []string{"WAAA", "WBBB"}, Now: time.Unix(0, 0),
	})
	if !reflect.DeepEqual(ids(got.Items), []string{"wbbb-1", "listed"}) {
		t.Fatalf("offered %v, want the surviving search's hit and the listing", ids(got.Items))
	}
	if !strings.Contains(logged.String(), "WAAA") {
		t.Fatalf("log = %q, want the failed call sign reported", logged.String())
	}
}

func TestList_WithoutStationsSearchesNothing(t *testing.T) {
	catalog := &fakeCatalog{listed: []string{"a"}}
	list(t, archive.New(archive.Config{Catalog: catalog}), sourceprovider.Checkpoint{}, sourceprovider.ListRequest{Take: 1, List: 1})
	if len(catalog.searched) != 0 {
		t.Fatalf("searched %v, want none", catalog.searched)
	}
}

// #1773: with compilations held back, a listing is enriched with runtimes before selection, since
// the search carries none and the gate can only hold back what it can measure. With compilations
// taken, the listing stays one request.
func TestList_ReadsRuntimesOnlyWhileCompilationsWait(t *testing.T) {
	for _, hold := range []bool{true, false} {
		catalog := &fakeCatalog{listed: []string{"reel", "spot"}, runtimes: map[string]int{"reel": 30 * 60 * 1000, "spot": 45 * 1000}}
		p := archive.New(archive.Config{Catalog: catalog, Runtimes: func() bool { return hold }})
		got := list(t, p, sourceprovider.Checkpoint{}, sourceprovider.ListRequest{Take: 5, List: 5})
		runtimes := map[string]int{}
		for _, item := range got.Items {
			runtimes[item.ID] = item.DurationMS
		}
		if hold && (catalog.enriched != 1 || runtimes["reel"] != 30*60*1000 || runtimes["spot"] != 45*1000) {
			t.Errorf("compilations held back: enriched %d times, runtimes %v; want one enrichment and both runtimes", catalog.enriched, runtimes)
		}
		if !hold && catalog.enriched != 0 {
			t.Errorf("compilations taken: enriched %d times, want the listing left at one request", catalog.enriched)
		}
	}
}

// Reading runtimes never spends more than half of what is left of the caller's deadline, so a
// planned pull over many sources can't fail its own deadline on runtimes.
func TestList_RuntimesLeaveTheCallerTime(t *testing.T) {
	catalog := &fakeCatalog{listed: []string{"a"}}
	p := archive.New(archive.Config{Catalog: catalog, Runtimes: func() bool { return true }})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := p.List(ctx, sourceprovider.Source{URI: "coll"}, sourceprovider.ListRequest{Take: 1, List: 1}); err != nil {
		t.Fatal(err)
	}
	if catalog.allowed <= 0 || catalog.allowed > 5*time.Second {
		t.Fatalf("runtimes were given %s of a 10 s deadline, want at most half", catalog.allowed)
	}
}

func TestSearch_FindsItemsInsideOneSource(t *testing.T) {
	catalog := &fakeCatalog{hits: map[string][]string{"WAAA": {"hit"}}}
	got, err := archive.New(archive.Config{Catalog: catalog}).Search(t.Context(), sourceprovider.Source{URI: "coll"}, "WAAA", 7)
	if err != nil || !reflect.DeepEqual(ids(got), []string{"hit"}) || !reflect.DeepEqual(catalog.searchLimits, []int{7}) {
		t.Fatalf("got %v, %v (limits %v)", ids(got), err, catalog.searchLimits)
	}
}

func TestProvider_IsDiscoverableAndAddable(t *testing.T) {
	var p sourceprovider.Provider = archive.New(archive.Config{})
	for name, ok := range map[string]bool{
		"Finder":    func() bool { _, ok := p.(sourceprovider.Finder); return ok }(),
		"Addresser": func() bool { _, ok := p.(sourceprovider.Addresser); return ok }(),
		"Searcher":  func() bool { _, ok := p.(sourceprovider.Searcher); return ok }(),
	} {
		if !ok {
			t.Errorf("the archive provider does not implement %s", name)
		}
	}
	if p.Describe().BadAddress.Title == "" {
		t.Error("the archive provider does not say what address it accepts")
	}
}

// What an operator pastes into "Add a source" is not consistent, and the three shapes below are
// all things people really paste. Anything else must be REFUSED rather than guessed at: a row
// built from an unparseable string is a source that can never fetch and never explains why.
func TestProvider_AddressReadsTheShapesAnOperatorPastes(t *testing.T) {
	p := archive.New(archive.Config{})
	for _, tc := range []struct{ in, want string }{
		{"classic_tv_commercials", "classic_tv_commercials"},
		{"https://archive.org/details/classic_tv_commercials", "classic_tv_commercials"},
		{"http://archive.org/details/classic_tv_commercials/", "classic_tv_commercials"},
		{"archive.org/details/classic_tv_commercials?sort=date", "classic_tv_commercials"},
		{"/details/classic_tv_commercials", "classic_tv_commercials"},
		{"  classic_tv_commercials  ", "classic_tv_commercials"},

		// Refused. Each of these would otherwise become a row that looks registered.
		{"", ""},
		{"https://example.com/some/page", ""},
		{"not an id", ""},
		{"https://archive.org/", ""},
		{"archive.org/details/", ""},
	} {
		if got := p.Address(tc.in); got != tc.want {
			t.Errorf("Address(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A finder given an address checks it; given words it searches. A bare word is a valid
// identifier and also a search, so only a separated slug or a URL counts as an address.
func TestProvider_RecognizesAddressesNotWords(t *testing.T) {
	p := archive.New(archive.Config{})
	for in, want := range map[string]bool{
		"https://archive.org/details/classic_tv_commercials": true,
		"archive.org/metadata/classic_tv":                    true,
		"classic_tv_commercials":                             true,
		"classic-tv":                                         true,
		"cartoons":                                           false,
		"classic tv commercials":                             false,
		"https://example.com/details/x_y":                    false,
	} {
		if got := p.Recognizes(in); got != want {
			t.Errorf("Recognizes(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestProvider_LinksToItsOwnPages(t *testing.T) {
	p := archive.New(archive.Config{})
	for in, want := range map[string]string{
		"classic_tv":                    "https://archive.org/details/classic_tv",
		"https://archive.org/details/x": "https://archive.org/details/x",
		"  ":                            "",
	} {
		if got := p.SourceURL(in); got != want {
			t.Errorf("SourceURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := p.EmbedURL("https://archive.org/details/break_one"); got != "https://archive.org/embed/break_one?autoplay=1" {
		t.Errorf("EmbedURL = %q", got)
	}
	for _, refused := range []string{"https://archive.org/details/a/b", "https://evil.test/details/a", "https://archive.org/search?q=x"} {
		if got := p.EmbedURL(refused); got != "" {
			t.Errorf("EmbedURL(%q) = %q, want none", refused, got)
		}
	}
}
