package library_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/testkit"
)

// These tests pin the Live TV wiring adapter against the Phase-10 live capture
// (Emby 4.10.0.17) fixtures in testkit/fixtures/livetv/. They prove the adapter
// speaks the shapes the real media server accepted — enumerate, add, and the
// guide-refresh task-id-by-Key resolution (finding 4).

func newLiveTVClient(url string) *library.Client {
	return library.New(library.Emby, url, "test-token", "dev-1")
}

// The enumerate-first read goes through GET /System/Configuration/livetv, which is the
// ONLY listing endpoint both flavors serve. Emby also answers the lineage
// GET /LiveTv/{TunerHosts,ListingProviders}, but Jellyfin makes those write-only and
// returns 405 — so reading through them broke the idempotency check on every Jellyfin
// install (see fixtures/livetv/FINDINGS.md, Jellyfin capture).
//
// This asserts the PATH as well as the parse: a regression back to the lineage endpoint
// would still pass a body-shape-only test while being broken against half the supported
// media servers.
func serveLiveTVConfig(t *testing.T) (*httptest.Server, *library.Client) {
	t.Helper()
	cfg := testkit.Fixture(t, "livetv/livetv_config_jellyfin.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Configuration/livetv" {
			t.Errorf("enumerate hit %s — Jellyfin 405s the /LiveTv/* listing endpoints", r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(cfg)
	}))
	t.Cleanup(srv.Close)
	return srv, library.New(library.Emby, srv.URL, "test-token", "dev-1")
}

func TestTunerRegistered_MatchesByURL(t *testing.T) {
	_, c := serveLiveTVConfig(t)

	// The captured config holds one m3u tuner at this exact URL.
	got, err := c.TunerRegistered(context.Background(), "http://192.168.1.79:8001/api/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("the captured M3U tuner should report registered")
	}

	// A URL that is absent reports false — this is what drives the enumerate-first
	// idempotency, so getting it wrong means either a duplicate tuner or a skipped wiring.
	got, err = c.TunerRegistered(context.Background(), "http://elsewhere:8000/api/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("an unregistered M3U URL should report false")
	}
}

func TestListingRegistered_MatchesByPath(t *testing.T) {
	_, c := serveLiveTVConfig(t)

	// The xmltv provider's URL lives in `Path`, not `Url` (Phase-10 finding 1).
	got, err := c.ListingRegistered(context.Background(), "http://192.168.1.79:8001/api/xmltv.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("the captured xmltv provider should report registered")
	}

	got, err = c.ListingRegistered(context.Background(), "http://elsewhere:8000/api/xmltv.xml")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("an unregistered xmltv URL should report false")
	}
}

// RescanTuner re-POSTs the matching host VERBATIM, so the media server treats it as an
// update rather than a second registration. Fields we don't model (Id, TunerCount,
// IgnoreDts…) must survive the round trip.
func TestRescanTuner_Reposts_TheWholeHost(t *testing.T) {
	cfg := testkit.Fixture(t, "livetv/livetv_config_jellyfin.json")
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/System/Configuration/livetv" {
			_, _ = w.Write(cfg)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/LiveTv/TunerHosts" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &posted)
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	if err := c.RescanTuner(context.Background(), "http://192.168.1.79:8001/api/channels.m3u"); err != nil {
		t.Fatal(err)
	}
	if posted["Id"] != "f31d60f93a5d4affa67b67c8a51174cc" {
		t.Errorf("re-POST lost the host Id (%v) — the server would create a SECOND tuner", posted["Id"])
	}
	if posted["FriendlyName"] != "loomarr" {
		t.Errorf("re-POST dropped unmodeled fields: %v", posted)
	}

	// An unregistered URL is a no-op, not an error, and must not POST anything.
	posted = nil
	if err := c.RescanTuner(context.Background(), "http://nope:8000/api/channels.m3u"); err != nil {
		t.Fatal(err)
	}
	if posted != nil {
		t.Error("rescanning an unwired tuner must not write")
	}
}

func TestAddTuner_SendsM3UShape(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/LiveTv/TunerHosts" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testkit.Fixture(t, "livetv/tuner_add_response.json"))
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	if err := c.AddTuner(context.Background(), "http://TUNARR_HOST:8000/api/channels.m3u"); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Type string `json:"Type"`
		URL  string `json:"Url"`
	}
	_ = json.Unmarshal(body, &sent)
	if sent.Type != "m3u" {
		t.Errorf("tuner Type = %q, want m3u (capture)", sent.Type)
	}
	if sent.URL != "http://TUNARR_HOST:8000/api/channels.m3u" {
		t.Errorf("tuner Url = %q", sent.URL)
	}
}

func TestAddListingProvider_SendsXMLTVPathShape(t *testing.T) {
	// Phase-10 finding 1: the xmltv provider URL field is Path (confirmed 200).
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/LiveTv/ListingProviders" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testkit.Fixture(t, "livetv/listing_add_response.json"))
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	if err := c.AddListingProvider(context.Background(), "http://TUNARR_HOST:8000/api/xmltv.xml"); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Type string `json:"Type"`
		Path string `json:"Path"`
	}
	_ = json.Unmarshal(body, &sent)
	if sent.Type != "xmltv" || sent.Path != "http://TUNARR_HOST:8000/api/xmltv.xml" {
		t.Errorf("provider body = %+v, want xmltv with Path (capture finding 1)", sent)
	}
}

func TestRefreshGuide_ResolvesTaskIDByKey(t *testing.T) {
	// Phase-10 finding 4: the run endpoint takes the per-install Id, resolved from
	// the stable Key "RefreshGuide". The adapter must GET /ScheduledTasks, find the
	// task by Key, then POST /ScheduledTasks/Running/<id>.
	const realID = "9492d30c70f7f1bec3757c9d0a4feb45"
	var ranPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			// Return a task list where RefreshGuide has the per-install id.
			_, _ = w.Write([]byte(`[
				{"Id":"aaaa","Key":"RefreshChannels","Name":"Refresh Channels"},
				{"Id":"` + realID + `","Key":"RefreshGuide","Name":"Refresh Guide"}
			]`))
		case r.Method == http.MethodPost && r.URL.Path == "/ScheduledTasks/Running/"+realID:
			ranPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	if err := c.RefreshGuide(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ranPath != "/ScheduledTasks/Running/"+realID {
		t.Errorf("guide refresh ran %q, want the id resolved by Key", ranPath)
	}
}

func TestRefreshGuide_ErrorsWhenTaskAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"x","Key":"SomethingElse"}]`))
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	if err := c.RefreshGuide(context.Background()); err == nil {
		t.Fatal("expected an error when the RefreshGuide task is absent")
	}
}
