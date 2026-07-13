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

func TestTunerRegistered_MatchesByURL(t *testing.T) {
	list := testkit.Fixture(t, "livetv/tuner_hosts_list.json") // has the HDHomeRun, not our M3U
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(list)
	}))
	defer srv.Close()
	c := newLiveTVClient(srv.URL)

	// The fixture's only tuner is an HDHomeRun at "192.168.0.11" — our M3U URL is
	// absent, so TunerRegistered is false (drives the enumerate-first idempotency).
	got, err := c.TunerRegistered(context.Background(), "http://TUNARR_HOST:8000/api/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("M3U not in the fixture; TunerRegistered should be false")
	}
	// A URL that IS present reports true.
	got, _ = c.TunerRegistered(context.Background(), "10.0.0.11")
	if !got {
		t.Error("existing HDHomeRun URL should report registered")
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
