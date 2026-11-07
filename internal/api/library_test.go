package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
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

// newLibraryHarness serves the real scan.Library over the harness store and the real disk: the
// sources name folders that exist in a temp dir.
func newLibraryHarness(t *testing.T, sonarrFolders ...string) *apiHarness {
	t.Helper()
	h, _ := newScanHarness(t, sonarrFolders...)
	return h
}

// newScanHarness is newLibraryHarness with its scanner, whose passes a test runs itself; the
// scanner's own loop never starts.
func newScanHarness(t *testing.T, sonarrFolders ...string) (*apiHarness, *scan.Library) {
	t.Helper()
	var lib *scan.Library
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib = newScanLibrary(t, defaults.Store, sonarrFolders...)
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, LibraryRoots: lib,
		})
	})
	return h, lib
}

// newScanLibrary is the real scan.Library over st, with Sonarr naming sonarrFolders and Radarr
// not set up.
func newScanLibrary(t *testing.T, st records.Store, sonarrFolders ...string) *scan.Library {
	t.Helper()
	resolver, err := precedence.New("US")
	if err != nil {
		t.Fatal(err)
	}
	roots := &scan.Roots{
		Store: st,
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
	inv := inventory.New(st)
	return &scan.Library{Roots: roots, ItemStore: st, NeedStore: st, CollectionStore: st, SetRecords: st, Inventory: inv, Scanner: scan.NewScanner(&scan.Importer{
		Store: st, Inventory: inv, Disk: roots.Disk,
		Resolver: resolver, Enabled: func() bool { return true },
	}, 0)}
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

type libraryScanStatus struct {
	Scan struct {
		Enabled    bool       `json:"enabled"`
		Stage      string     `json:"stage"`
		LastScanAt *time.Time `json:"lastScanAt"`
	} `json:"scan"`
	Files int `json:"files"`
	Held  []struct {
		Path     string `json:"path"`
		Movies   int    `json:"movies"`
		Episodes int    `json:"episodes"`
		Other    int    `json:"other"`
	} `json:"held"`
}

func scanStatus(t *testing.T, resp *http.Response, want int) libraryScanStatus {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	var body libraryScanStatus
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// The scanner's state reaches the status strip, a held removal is listed by kind and applied on
// confirmation, and "Check again" starts a scan.
func TestLibraryScan_StatusHeldAndConfirm(t *testing.T) {
	h, lib := newScanHarness(t)
	movies := t.TempDir()
	_ = decodeLibraryRoots(t, h.Do(http.MethodPost, "/v1/library/roots", adminToken, `{"path":"`+movies+`"}`))
	var paths []string
	for _, name := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"} {
		dir := filepath.Join(movies, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name+".mkv")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	ctx := context.Background()
	if err := lib.Scanner.Importer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	st := scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	if !st.Scan.Enabled || st.Scan.Stage != "idle" || st.Scan.LastScanAt == nil || st.Files != 5 || len(st.Held) != 0 {
		t.Fatalf("after first scan %+v", st)
	}

	for _, p := range paths[:2] {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := lib.Scanner.Importer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	st = scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	if len(st.Held) != 1 || st.Held[0].Path != movies || st.Held[0].Movies != 2 || st.Held[0].Episodes != 0 {
		t.Fatalf("held %+v", st.Held)
	}

	// The confirmation is recorded at once and applied by the next pass, which it asks for.
	_ = scanStatus(t, h.Do(http.MethodPost, "/v1/library/roots/confirm-removal", adminToken, `{"path":"`+movies+`"}`), http.StatusAccepted)
	if err := lib.Scanner.Importer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	st = scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	if len(st.Held) != 0 || st.Files != 3 {
		t.Fatalf("after confirm %+v", st)
	}
	resp := h.Do(http.MethodPost, "/v1/library/roots/confirm-removal", adminToken, `{"path":"`+movies+`"}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second confirm: %d", resp.StatusCode)
	}
	_ = scanStatus(t, h.Do(http.MethodPost, "/v1/library/scan", adminToken, ""), http.StatusAccepted)

	lib.Scanner.Importer.Enabled = func() bool { return false }
	resp = h.Do(http.MethodPost, "/v1/library/scan", adminToken, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("scan while off: %d", resp.StatusCode)
	}
	for _, route := range []string{"/v1/library/scan", "/v1/library/roots/confirm-removal"} {
		resp := h.Do(http.MethodPost, route, memberToken, `{"path":"/x"}`)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("member %s: %d", route, resp.StatusCode)
		}
	}
}
