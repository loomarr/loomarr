package requester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
)

// arrStub is a fake Sonarr/Radarr: it answers lookup/qualityprofile/rootfolder/queue and
// records the add POST + any DELETE, so a test can assert the exact request Loomarr made.
type arrStub struct {
	server      *httptest.Server
	addBody     map[string]any // the last POST /api/v3/{kind} body
	addStatus   int            // status to return from the add (default 201)
	lookupID    int            // the arr's internal id returned by lookup (for queue matching)
	deletedID   int            // the queue id DELETEd by Cancel, if any
	queueForID  int            // a queue record's movieId/seriesId to expose
	queueRecID  int            // that queue record's own id
	profileName string         // name of the single quality profile (id 7)
	lookupTerm  string         // the last lookup ?term= (proves tmdb/tvdb routing)
}

func newArrStub(t *testing.T, kind string) *arrStub {
	t.Helper()
	s := &arrStub{addStatus: http.StatusCreated, lookupID: 42, profileName: "HD-1080p"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/"+kind+"/lookup", func(w http.ResponseWriter, r *http.Request) {
		s.lookupTerm = r.URL.Query().Get("term")
		// One match, carrying the arr's internal id (0 until added; we expose lookupID so
		// queue correlation works in the Cancel test).
		_ = json.NewEncoder(w).Encode([]map[string]any{{"title": "The Matrix", "id": s.lookupID}})
	})
	mux.HandleFunc("/api/v3/qualityprofile", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "name": s.profileName}})
	})
	mux.HandleFunc("/api/v3/rootfolder", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"path": "/data/media"}})
	})
	mux.HandleFunc("/api/v3/"+kind, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&s.addBody)
			w.WriteHeader(s.addStatus)
			_, _ = w.Write([]byte(`{"id":42}`))
		}
	})
	mux.HandleFunc("/api/v3/queue", func(w http.ResponseWriter, r *http.Request) {
		field := "movieId"
		if kind == "series" {
			field = "seriesId"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": []map[string]any{{"id": s.queueRecID, field: s.queueForID}},
		})
	})
	mux.HandleFunc("/api/v3/queue/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			// path is /api/v3/queue/<id> (strip any ?query first)
			idStr := strings.TrimPrefix(r.URL.Path, "/api/v3/queue/")
			s.deletedID, _ = strconv.Atoi(idStr)
			w.WriteHeader(http.StatusOK)
		}
	})
	mux.HandleFunc("/api/v3/system/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"version":"4.0"}`))
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

// arrFor builds an Arr whose Radarr (movies) or Sonarr (series) points at the stub; the other
// arr is left unconfigured. Overrides optional.
func arrFor(kind string, url string, qp, root string) *Arr {
	c := ArrConns{
		Sonarr: func() (string, string) { return "", "" },
		Radarr: func() (string, string) { return "", "" },
	}
	if kind == "movie" {
		c.Radarr = func() (string, string) { return url, "key" }
		c.RadarrQualityProfile = func() string { return qp }
		c.RadarrRootFolder = func() string { return root }
	} else {
		c.Sonarr = func() (string, string) { return url, "key" }
		c.SonarrQualityProfile = func() string { return qp }
		c.SonarrRootFolder = func() string { return root }
	}
	return NewArr(c)
}

func TestArr_RequestMovie_LooksUpAndAdds(t *testing.T) {
	stub := newArrStub(t, "movie")
	a := arrFor("movie", stub.server.URL, "", "")

	err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	if stub.addBody == nil {
		t.Fatal("no add POST recorded")
	}
	// Auto-picked profile (id 7) + root, monitored, with a search trigger.
	if qp, _ := toInt(stub.addBody["qualityProfileId"]); qp != 7 {
		t.Errorf("qualityProfileId = %v, want 7", stub.addBody["qualityProfileId"])
	}
	if stub.addBody["rootFolderPath"] != "/data/media" {
		t.Errorf("rootFolderPath = %v", stub.addBody["rootFolderPath"])
	}
	if stub.addBody["monitored"] != true {
		t.Errorf("monitored = %v, want true", stub.addBody["monitored"])
	}
}

func TestArr_RequestSeries_RoutesToSonarrByTVDB(t *testing.T) {
	stub := newArrStub(t, "series")
	a := arrFor("series", stub.server.URL, "", "")

	// TVDB present + TMDB present: Sonarr must look up by tvdb (its native id), not tmdb.
	err := a.Request(context.Background(), provision.Title{MediaType: provision.Series, TVDBID: 81189, TMDBID: 999, Name: "Breaking Bad"})
	if err != nil {
		t.Fatal(err)
	}
	if stub.addBody == nil {
		t.Fatal("series add not recorded (should route to Sonarr)")
	}
	if stub.lookupTerm != "tvdb:81189" {
		t.Errorf("lookup term = %q, want tvdb:81189", stub.lookupTerm)
	}
}

func TestArr_RequestMovie_AlreadyAddedIsSuccess(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusConflict} {
		stub := newArrStub(t, "movie")
		stub.addStatus = code
		a := arrFor("movie", stub.server.URL, "", "")
		if err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err != nil {
			t.Errorf("add status %d should be success (already added), got %v", code, err)
		}
	}
}

func TestArr_RequestMovie_ServerErrorFails(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.addStatus = http.StatusInternalServerError
	a := arrFor("movie", stub.server.URL, "", "")
	if err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err == nil {
		t.Error("500 on add should fail (record stays wanted for retry)")
	}
}

func TestArr_QualityProfileOverride_ByName(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.profileName = "4K-UHD"
	a := arrFor("movie", stub.server.URL, "4K-UHD", "")
	if err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err != nil {
		t.Fatal(err)
	}
	if qp, _ := toInt(stub.addBody["qualityProfileId"]); qp != 7 {
		t.Errorf("name override should resolve to id 7, got %v", stub.addBody["qualityProfileId"])
	}
}

func TestArr_RootFolderOverride(t *testing.T) {
	stub := newArrStub(t, "movie")
	a := arrFor("movie", stub.server.URL, "", "/custom/movies")
	if err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err != nil {
		t.Fatal(err)
	}
	if stub.addBody["rootFolderPath"] != "/custom/movies" {
		t.Errorf("rootFolderPath = %v, want the override", stub.addBody["rootFolderPath"])
	}
}

func TestArr_Cancel_DeletesQueueRecord(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.lookupID = 42   // the movie's arr id
	stub.queueForID = 42 // a queue record for that movie…
	stub.queueRecID = 99 // …with queue id 99
	a := arrFor("movie", stub.server.URL, "", "")

	if err := a.Cancel(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err != nil {
		t.Fatal(err)
	}
	if stub.deletedID != 99 {
		t.Errorf("deleted queue id = %d, want 99", stub.deletedID)
	}
}

func TestArr_Cancel_NoQueueRecordIsNoop(t *testing.T) {
	stub := newArrStub(t, "movie")
	stub.lookupID = 42
	stub.queueForID = 1 // a different movie is queued; ours isn't
	stub.queueRecID = 5
	a := arrFor("movie", stub.server.URL, "", "")
	if err := a.Cancel(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 603, Name: "M"}); err != nil {
		t.Fatal(err)
	}
	if stub.deletedID != 0 {
		t.Errorf("nothing should be deleted, got %d", stub.deletedID)
	}
}

func TestArr_Reachable(t *testing.T) {
	stub := newArrStub(t, "movie")
	a := arrFor("movie", stub.server.URL, "", "")
	if err := a.Reachable(context.Background()); err != nil {
		t.Errorf("Reachable = %v, want nil", err)
	}

	// Unconfigured → a clear error, not a false pass.
	empty := NewArr(ArrConns{Sonarr: func() (string, string) { return "", "" }, Radarr: func() (string, string) { return "", "" }})
	if err := empty.Reachable(context.Background()); err == nil {
		t.Error("no arr configured should be unreachable")
	}
}

func TestArr_UnconfiguredArrForMediaTypeFails(t *testing.T) {
	// Only Sonarr configured; a movie request needs Radarr → error (stays wanted).
	a := NewArr(ArrConns{
		Sonarr: func() (string, string) { return "http://sonarr", "k" },
		Radarr: func() (string, string) { return "", "" },
	})
	if err := a.Request(context.Background(), provision.Title{MediaType: provision.Movie, TMDBID: 1, Name: "M"}); err == nil {
		t.Error("movie with no Radarr should fail")
	}
}
