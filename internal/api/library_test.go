package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/scan"
)

type libraryRootsBody struct {
	Roots []struct {
		Path        string     `json:"path"`
		InUse       bool       `json:"inUse"`
		Readable    bool       `json:"readable"`
		CheckedAt   *time.Time `json:"checkedAt"`
		AddedByHand bool       `json:"addedByHand"`
		FoundIn     []struct {
			Authority string `json:"authority"`
			Path      string `json:"path"`
		} `json:"foundIn"`
	} `json:"roots"`
	PathMap []struct {
		Authority     string `json:"authority"`
		AuthorityPath string `json:"authorityPath"`
		LocalPath     string `json:"localPath"`
		ByHand        bool   `json:"byHand"`
	} `json:"pathMap"`
	Sources []struct {
		Authority string `json:"authority"`
		State     string `json:"state"`
	} `json:"sources"`
}

// newLibraryHarness serves the real scan.Roots over the harness store and the real disk: the
// sources name folders that exist in a temp dir.
func newLibraryHarness(t *testing.T, sonarrFolders ...string) *apiHarness {
	t.Helper()
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		roots := &scan.Roots{
			Store: defaults.Store,
			Sources: func() []scan.Source {
				return []scan.Source{
					{Authority: "sonarr", Folders: func(context.Context) ([]string, error) { return sonarrFolders, nil }},
					{Authority: "radarr", Folders: func(context.Context) ([]string, error) { return nil, scan.ErrSkipped }},
				}
			},
			PathMap: func() library.PathMap { return nil },
			Disk:    scan.OSDisk{MountInfo: filepath.Join(t.TempDir(), "no-mounts")},
			Now:     time.Now,
		}
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, LibraryRoots: roots,
		})
	})
}

func decodeLibraryRoots(t *testing.T, resp *http.Response) libraryRootsBody {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body libraryRootsBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func libraryStatus(t *testing.T, h *apiHarness) (health string, inUse, offline, toConfirm int) {
	t.Helper()
	resp := h.Do(http.MethodGet, "/v1/library/status", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Health    string `json:"health"`
		InUse     int    `json:"inUse"`
		Offline   int    `json:"offline"`
		ToConfirm int    `json:"toConfirm"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Health, body.InUse, body.Offline, body.ToConfirm
}

// Discover → confirm → status: the path the Settings → Library page drives.
func TestLibraryRoots_DiscoverConfirmAndStatus(t *testing.T) {
	tv := t.TempDir()
	h := newLibraryHarness(t, tv, "/not/on/this/machine")

	if health, _, _, _ := libraryStatus(t, h); health != "needs_folders" {
		t.Fatalf("fresh health = %q", health)
	}
	body := decodeLibraryRoots(t, h.Do(http.MethodPost, "/v1/library/roots/discover", adminToken, ""))
	if len(body.Roots) != 1 || body.Roots[0].Path != tv || body.Roots[0].InUse || !body.Roots[0].Readable ||
		body.Roots[0].CheckedAt == nil || len(body.Roots[0].FoundIn) != 1 || body.Roots[0].FoundIn[0].Authority != "sonarr" {
		t.Fatalf("roots %+v", body.Roots)
	}
	if len(body.PathMap) != 2 || len(body.Sources) != 2 || body.Sources[1].State != "skipped" {
		t.Fatalf("map %+v sources %+v", body.PathMap, body.Sources)
	}
	if _, _, _, toConfirm := libraryStatus(t, h); toConfirm != 1 {
		t.Fatalf("toConfirm = %d", toConfirm)
	}

	body = decodeLibraryRoots(t, h.Do(http.MethodPut, "/v1/library/roots/in-use", adminToken,
		`{"path":"`+tv+`","inUse":true}`))
	if !body.Roots[0].InUse || len(body.Sources) != 0 {
		t.Fatalf("after confirm %+v", body)
	}
	if health, inUse, _, _ := libraryStatus(t, h); health != "ok" || inUse != 1 {
		t.Fatalf("health %q inUse %d", health, inUse)
	}
}

// Hand edits: an unreadable or relative folder is refused in words; an unknown folder is 404;
// members never see folders.
func TestLibraryRoots_HandEditsAndAccess(t *testing.T) {
	h := newLibraryHarness(t, "/srv/tv")
	for _, path := range []string{"relative", filepath.Join(t.TempDir(), "missing")} {
		resp := h.Do(http.MethodPost, "/v1/library/roots", adminToken, `{"path":"`+path+`"}`)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("add %q: status %d", path, resp.StatusCode)
		}
	}
	extra := t.TempDir()
	body := decodeLibraryRoots(t, h.Do(http.MethodPost, "/v1/library/roots", adminToken, `{"path":"`+extra+`"}`))
	if len(body.Roots) != 1 || !body.Roots[0].InUse || !body.Roots[0].AddedByHand {
		t.Fatalf("added %+v", body.Roots)
	}

	resp := h.Do(http.MethodPut, "/v1/library/roots/in-use", adminToken, `{"path":"/nope","inUse":true}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown root: %d", resp.StatusCode)
	}

	_ = decodeLibraryRoots(t, h.Do(http.MethodPost, "/v1/library/roots/discover", adminToken, ""))
	shows := t.TempDir()
	body = decodeLibraryRoots(t, h.Do(http.MethodPut, "/v1/library/path-map", adminToken,
		`{"authority":"sonarr","authorityPath":"/srv/tv","localPath":"`+shows+`"}`))
	var mapped bool
	for _, row := range body.PathMap {
		mapped = mapped || (row.AuthorityPath == "/srv/tv" && row.LocalPath == shows && row.ByHand)
	}
	if !mapped {
		t.Fatalf("path map %+v", body.PathMap)
	}

	for _, route := range []string{"/v1/library/roots", "/v1/library/status"} {
		resp := h.Do(http.MethodGet, route, memberToken, "")
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("member %s: %d", route, resp.StatusCode)
		}
	}
}
