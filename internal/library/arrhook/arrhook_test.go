package arrhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/library/arrhook"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, body, typ string
		folders         []string
	}{
		{
			name: "sonarr download names the episode's folder",
			body: `{"eventType":"Download","isUpgrade":false,"series":{"path":"/tv/Lima"},
				"episodeFile":{"relativePath":"Season 01/Lima S01E02.mkv","path":"/tv/Lima/Season 01/Lima S01E02.mkv"}}`,
			typ: "Download", folders: []string{"/tv/Lima/Season 01"},
		},
		{
			name: "a relative path alone is joined to the series folder",
			body: `{"eventType":"Download","series":{"path":"/tv/Lima"},"episodeFile":{"relativePath":"Season 02/Lima S02E01.mkv"}}`,
			typ:  "Download", folders: []string{"/tv/Lima/Season 02"},
		},
		{
			name: "an upgrade names the deleted file's folder too",
			body: `{"eventType":"Download","isUpgrade":true,"movie":{"folderPath":"/movies/Mike (2001)"},
				"movieFile":{"path":"/movies/Mike (2001)/Mike (2001) 2160p.mkv"},
				"deletedFiles":[{"path":"/movies/Mike (2001)/old/Mike (2001) 1080p.mkv"}]}`,
			typ: "Download", folders: []string{"/movies/Mike (2001)", "/movies/Mike (2001)/old"},
		},
		{
			name: "a rename names both ends and the series folder",
			body: `{"eventType":"Rename","series":{"path":"/tv/November"},"renamedEpisodeFiles":[
				{"path":"/tv/November/Season 01/November S01E01.mkv","previousPath":"/tv/November/S1/n101.mkv"}]}`,
			typ: "Rename", folders: []string{"/tv/November", "/tv/November/S1", "/tv/November/Season 01"},
		},
		{
			name: "a movie delete names its folder",
			body: `{"eventType":"MovieDelete","movie":{"folderPath":"/movies/Oscar (1999)"}}`,
			typ:  "MovieDelete", folders: []string{"/movies/Oscar (1999)"},
		},
		{
			// A delete's deletedFiles says whether the files went too: a bool, where an upgrade's is
			// a list (Sonarr's WebhookSeriesDeletePayload, Radarr's WebhookMovieDeletePayload).
			name: "a series delete that took its files names its folder",
			body: `{"eventType":"SeriesDelete","series":{"path":"/tv/Romeo","tvdbId":90081202},"deletedFiles":true}`,
			typ:  "SeriesDelete", folders: []string{"/tv/Romeo"},
		},
		{
			name: "a movie delete that kept its files names its folder",
			body: `{"eventType":"MovieDelete","movie":{"folderPath":"/movies/Sierra (2003)"},"deletedFiles":false}`,
			typ:  "MovieDelete", folders: []string{"/movies/Sierra (2003)"},
		},
		{
			name: "a Windows arr's backslashes become slashes",
			body: `{"eventType":"EpisodeFileDelete","series":{"path":"D:\\tv\\Papa"},"episodeFile":{"path":"D:\\tv\\Papa\\Season 01\\Papa S01E01.mkv"}}`,
			typ:  "EpisodeFileDelete", folders: []string{"D:/tv/Papa/Season 01"},
		},
		{
			name: "a grab changes no file",
			body: `{"eventType":"Grab","series":{"path":"/tv/Quebec"}}`,
			typ:  "Grab", folders: []string{},
		},
		{name: "test", body: `{"eventType":"Test"}`, typ: "Test", folders: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := arrhook.Parse([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if ev.Type != tc.typ || !slices.Equal(ev.Folders, tc.folders) {
				t.Errorf("event = %+v, want %s %v", ev, tc.typ, tc.folders)
			}
		})
	}
	// The item an event is about rides along, so the scanner reads just that one again.
	show, err := arrhook.Parse([]byte(`{"eventType":"Download","series":{"path":"/tv/Lima","tvdbId":90081201},
		"episodeFile":{"path":"/tv/Lima/Season 01/Lima S01E02.mkv"}}`))
	if err != nil || show.SeriesTVDB != 90081201 || show.MovieTMDB != 0 {
		t.Errorf("series event = %+v, %v", show, err)
	}
	film, err := arrhook.Parse([]byte(`{"eventType":"MovieDelete","movie":{"folderPath":"/movies/Oscar","tmdbId":90000801}}`))
	if err != nil || film.MovieTMDB != 90000801 || film.SeriesTVDB != 0 {
		t.Errorf("movie event = %+v, %v", film, err)
	}
	for _, bad := range []string{`not json`, `{"series":{}}`} {
		if _, err := arrhook.Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) = nil error", bad)
		}
	}
}

// fakeArr keeps notifications the way Sonarr does: it masks passwords on read and refuses to
// save a connection whose test delivery fails.
type fakeArr struct {
	mu      sync.Mutex
	saved   []map[string]any
	writes  int
	refuse  bool
	tests   int
	sawKeys []string
}

func (a *fakeArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sawKeys = append(a.sawKeys, r.Header.Get("X-Api-Key"))
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification/schema":
		writeJSON(w, []any{
			map[string]any{"implementation": "Slack", "fields": []any{}},
			map[string]any{
				"implementation": "Webhook", "configContract": "WebhookSettings", "name": "",
				"onGrab": false, "supportsOnGrab": true, "onDownload": false, "supportsOnDownload": true,
				"onUpgrade": false, "supportsOnUpgrade": true, "onRename": false, "supportsOnRename": true,
				"onSeriesDelete": false, "supportsOnSeriesDelete": true,
				"onEpisodeFileDelete": false, "supportsOnEpisodeFileDelete": true,
				"onHealthIssue": false, "supportsOnHealthIssue": true,
				"onMovieDelete": false, "supportsOnMovieDelete": false,
				"fields": []any{
					map[string]any{"name": "url", "value": ""}, map[string]any{"name": "method", "value": 1},
					map[string]any{"name": "username", "value": ""}, map[string]any{"name": "password", "value": ""},
					map[string]any{"name": "headers", "value": []any{}},
				},
			},
		})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification":
		out := []any{map[string]any{"id": 90000002.0, "name": "Discord", "fields": []any{}}}
		for _, n := range a.saved {
			out = append(out, masked(n))
		}
		writeJSON(w, out)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/notification/test":
		a.tests++
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/notification",
		r.Method == http.MethodPut && r.URL.Path == "/api/v3/notification/90000001":
		if a.refuse {
			http.Error(w, `[{"errorMessage":"Unable to send test message"}]`, http.StatusBadRequest)
			return
		}
		var n map[string]any
		_ = json.NewDecoder(r.Body).Decode(&n)
		a.writes++
		if r.Method == http.MethodPost {
			n["id"] = 90000001.0
			a.saved = append(a.saved, n)
		} else {
			a.saved[0] = n
		}
		writeJSON(w, n)
	default:
		http.NotFound(w, r)
	}
}

func masked(n map[string]any) map[string]any {
	raw, _ := json.Marshal(n)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	for _, f := range out["fields"].([]any) {
		if field := f.(map[string]any); field["name"] == "password" && field["value"] != "" {
			field["value"] = "********"
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestRegister(t *testing.T) {
	arr := &fakeArr{}
	srv := httptest.NewServer(arr)
	defer srv.Close()
	ctx := context.Background()
	c := arrhook.Client{Conn: func() (string, string) { return srv.URL + "/", "key-90000002" }}
	const hook = "http://loomarr.test/v1/library/hooks/sonarr"

	changed, err := c.Register(ctx, hook, "secret-90000003")
	if err != nil || !changed {
		t.Fatalf("first Register = %v, %v; want a new connection", changed, err)
	}
	n := arr.saved[0]
	if n["name"] != arrhook.Name || n["implementation"] != "Webhook" {
		t.Fatalf("saved = %v", n)
	}
	want := map[string]bool{
		"onGrab": false, "onDownload": true, "onUpgrade": true, "onRename": true, "onSeriesDelete": true,
		"onEpisodeFileDelete": true, "onHealthIssue": false, "onMovieDelete": false, // not offered by this arr
	}
	for k, v := range want {
		if n[k] != v {
			t.Errorf("%s = %v, want %v", k, n[k], v)
		}
	}
	fields := map[string]any{}
	for _, f := range n["fields"].([]any) {
		field := f.(map[string]any)
		fields[field["name"].(string)] = field["value"]
	}
	if fields["url"] != hook || fields["username"] != arrhook.Username || fields["password"] != "secret-90000003" || fields["method"] != 1.0 {
		t.Errorf("fields = %v", fields)
	}
	if _, kept := fields["headers"]; !kept {
		t.Error("a field Loomarr does not set was dropped")
	}

	if changed, err := c.Register(ctx, hook, "secret-90000003"); err != nil || changed {
		t.Fatalf("second Register = %v, %v; want no write (the masked password matches)", changed, err)
	}
	if changed, err := c.Register(ctx, hook+"?moved", "secret-90000003"); err != nil || !changed || arr.writes != 2 {
		t.Fatalf("Register with a new address = %v, %v, %d writes; want an update", changed, err, arr.writes)
	}
	if len(arr.saved) != 1 {
		t.Fatalf("connections = %d, want 1", len(arr.saved))
	}
	if err := c.Test(ctx); err != nil || arr.tests != 1 {
		t.Fatalf("Test = %v (tests %d)", err, arr.tests)
	}
	for _, k := range arr.sawKeys {
		if k != "key-90000002" {
			t.Fatalf("a call carried API key %q", k)
		}
	}
}

func TestRegisterReportsAnArrThatCannotReachLoomarr(t *testing.T) {
	arr := &fakeArr{refuse: true}
	srv := httptest.NewServer(arr)
	defer srv.Close()
	c := arrhook.Client{Conn: func() (string, string) { return srv.URL, "key" }}
	if _, err := c.Register(context.Background(), "http://unreachable.test/hook", "s"); err == nil {
		t.Fatal("Register = nil error, want the arr's refusal")
	}
	unset := arrhook.Client{Conn: func() (string, string) { return "", "" }}
	if _, err := unset.Register(context.Background(), "http://x/hook", "s"); !errors.Is(err, arrhook.ErrNotConfigured) {
		t.Fatalf("Register with no address = %v, want ErrNotConfigured", err)
	}
}
