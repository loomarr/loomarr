package clipfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Discovery is tested against the PINNED fixture, served by a local stub — never the live API
// (CLAUDE.md: unit tests never touch the network). The fixture is a real capture, and it keeps
// all four doc shapes the live API returns; see fixtures/archive/FINDINGS.md.
func discoverServer(t *testing.T, onQuery func(url.Values)) *httptest.Server {
	t.Helper()
	raw, err := os.ReadFile("../testkit/fixtures/archive/collection_search.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		if onQuery != nil {
			onQuery(r.URL.Query())
		}
		_, _ = w.Write(raw)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func discoverer(t *testing.T, base string) *ArchiveDownloader {
	t.Helper()
	return &ArchiveDownloader{client: newArchiveClient(base, nil, diskSink{})}
}

// ⚠ The headline behaviour: a listing must NOT download anything. An operator browsing to
// decide whether a collection is worth having cannot be made to fetch gigabytes to find out.
// Asserted by serving ONLY the search endpoint — any metadata or file request 404s, and the
// walk would fail rather than succeed quietly.
func TestDiscoverCollection_DownloadsNothing(t *testing.T) {
	var hits int
	srv := discoverServer(t, func(url.Values) { hits++ })

	res, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "classic_tv_commercials", 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("made %d search requests, want exactly 1 — a listing is one question", hits)
	}
	if len(res.Items) == 0 {
		t.Fatal("no items")
	}
}

// ⚠ Total is the COLLECTION's size, not the page's. Reporting len(Items) would tell an operator
// that an 8362-item collection holds 5 — and that number is exactly what they use to decide.
func TestDiscoverCollection_ReportsTheFullCollectionSize(t *testing.T) {
	srv := discoverServer(t, nil)

	res, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "classic_tv_commercials", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 8362 {
		t.Errorf("Total = %d, want 8362 (numFound, not the page length)", res.Total)
	}
	if res.Total == len(res.Items) {
		t.Error("Total equals the page length — it is reporting the page, not the collection")
	}
}

// The fixture deliberately mixes shapes because the live API does: Solr omits absent fields
// entirely. A parser assuming title/licenceurl/year is always present passes on a tidy fixture
// and fails on the first real collection.
func TestDiscoverCollection_HandlesTheShapesTheAPIActuallySends(t *testing.T) {
	srv := discoverServer(t, nil)

	res, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "classic_tv_commercials", 0)
	if err != nil {
		t.Fatal(err)
	}

	var withLicence, withoutLicence, withoutYear int
	for _, it := range res.Items {
		if it.ID == "" {
			t.Error("an item has no identifier — nothing could download it")
		}
		if it.License != "" {
			withLicence++
		} else {
			withoutLicence++
		}
		if it.Year == 0 {
			withoutYear++
		}
	}
	// Both licence states must be represented, or this test is not exercising the ~92%-absent
	// case that the whole "empty means unknown" rule exists for.
	if withLicence == 0 || withoutLicence == 0 {
		t.Errorf("fixture no longer covers both licence states (%d with, %d without) — "+
			"re-capture it with a mix, or this stops testing the absent case",
			withLicence, withoutLicence)
	}
	if withoutYear == 0 {
		t.Error("fixture no longer covers a doc with no year")
	}
}

// A caller may paste a URL, a /details/ path, or a bare id — the same spellings Download
// accepts. Making an operator know which form a field wants is a papercut with no upside.
func TestDiscoverCollection_AcceptsTheSpellingsDownloadDoes(t *testing.T) {
	var gotQuery string
	srv := discoverServer(t, func(q url.Values) { gotQuery = q.Get("q") })

	for _, ref := range []string{
		"classic_tv_commercials",
		"https://archive.org/details/classic_tv_commercials",
		"archive.org/details/classic_tv_commercials",
	} {
		if _, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), ref, 0); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		if !strings.Contains(gotQuery, "classic_tv_commercials") {
			t.Errorf("%s → query %q, which does not name the collection", ref, gotQuery)
		}
	}
}

// ⚠ The identifier is a VALUE in a Solr query, so it must be quoted — otherwise an id with a
// colon or a space changes the query's MEANING rather than being searched for.
func TestDiscoverCollection_QuotesTheIdentifier(t *testing.T) {
	var gotQuery string
	srv := discoverServer(t, func(q url.Values) { gotQuery = q.Get("q") })

	if _, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "odd id:with colon", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, `collection:"odd id:with colon"`) {
		t.Errorf("query = %q, want the identifier quoted as a single value", gotQuery)
	}
}

func TestDiscoverCollection_RejectsAnUnusableRef(t *testing.T) {
	srv := discoverServer(t, nil)

	if _, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "", 0); err == nil {
		t.Error("an empty ref was accepted")
	}
}

// The row cap is a decision aid, not a browser: an operator judges a source from a handful of
// titles. An unbounded limit would spend the operator's latency and archive.org's bandwidth on
// rows nobody reads.
func TestDiscoverCollection_CapsRows(t *testing.T) {
	var gotRows string
	srv := discoverServer(t, func(q url.Values) { gotRows = q.Get("rows") })

	if _, err := discoverer(t, srv.URL).DiscoverCollection(context.Background(), "c", 10_000); err != nil {
		t.Fatal(err)
	}
	if gotRows != "25" {
		t.Errorf("rows = %s, want the 25 cap", gotRows)
	}
}

// --- keyword search across archive.org (V33) ---

func searchServer(t *testing.T, onQuery func(url.Values)) *httptest.Server {
	t.Helper()
	raw, err := os.ReadFile("../testkit/fixtures/archive/keyword_search.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/advancedsearch.php", func(w http.ResponseWriter, r *http.Request) {
		if onQuery != nil {
			onQuery(r.URL.Query())
		}
		_, _ = w.Write(raw)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearch_ReturnsItemsWithTheirTotal(t *testing.T) {
	srv := searchServer(t, nil)

	res, err := discoverer(t, srv.URL).Search(context.Background(), "1980s cereal commercial", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no items")
	}
	// numFound from the real capture — the count an operator judges the search by, not the
	// page length.
	if res.Total != 54 {
		t.Errorf("Total = %d, want 54", res.Total)
	}
	for _, it := range res.Items {
		if it.ID == "" {
			t.Error("an item has no identifier — nothing could download it")
		}
	}
}

// ⚠ Scoped to mediatype:movies. Without it the same words return texts, audio and software —
// real results for the query, useless for a clip catalog.
func TestSearch_ScopesToVideo(t *testing.T) {
	var got string
	srv := searchServer(t, func(q url.Values) { got = q.Get("q") })

	if _, err := discoverer(t, srv.URL).Search(context.Background(), "cereal advert", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "mediatype:movies") {
		t.Errorf("query = %q, want it scoped to mediatype:movies", got)
	}
}

// ⚠ PARENTHESISED, not quoted. Quoting forces an exact-phrase match, so a three-word search
// would find only items containing that literal string — almost nothing. The parentheses also
// stop `AND mediatype:movies` binding to just the last word.
func TestSearch_DoesNotForceAnExactPhrase(t *testing.T) {
	var got string
	srv := searchServer(t, func(q url.Values) { got = q.Get("q") })

	if _, err := discoverer(t, srv.URL).Search(context.Background(), "1980s cereal commercial", 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"1980s cereal commercial"`) {
		t.Errorf("query = %q — quoting forces an exact phrase and finds nothing", got)
	}
	if !strings.Contains(got, "(1980s cereal commercial)") {
		t.Errorf("query = %q, want the words parenthesised", got)
	}
}

// A person pasting a URL or typing a colon must not accidentally write a Solr field query or a
// range — they would get a syntax error with no visible cause.
func TestSearch_StripsSolrSyntaxFromUserWords(t *testing.T) {
	var got string
	srv := searchServer(t, func(q url.Values) { got = q.Get("q") })

	if _, err := discoverer(t, srv.URL).Search(context.Background(), `collection:foo [a TO z] "x"`, 0); err != nil {
		t.Fatal(err)
	}
	// The only colon left is the one WE added for mediatype.
	if strings.Count(got, ":") != 1 {
		t.Errorf("query = %q, want the user's colons stripped", got)
	}
	if strings.ContainsAny(strings.TrimSuffix(got, ") AND mediatype:movies"), `[]"`) {
		t.Errorf("query = %q, want brackets and quotes stripped", got)
	}
}

func TestSearch_RejectsAnEmptyQuery(t *testing.T) {
	srv := searchServer(t, nil)

	if _, err := discoverer(t, srv.URL).Search(context.Background(), "   ", 0); err == nil {
		t.Error("an all-whitespace query was accepted")
	}
}
