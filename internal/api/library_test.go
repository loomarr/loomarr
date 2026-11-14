package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	mptmdb "github.com/loomarr/loomarr/internal/library/metadataprovider/tmdb"
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
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, Library: api.LibraryOptions{Roots: lib},
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
		PathMap: func() mediaserver.PathMap { return nil },
		Disk:    scan.OSDisk{MountInfo: filepath.Join(t.TempDir(), "no-mounts")},
		Now:     time.Now,
	}
	// The Library's rows follow every inventory write, as the app wires it: a file's title is a
	// foreign key to its row (#2793).
	st.ProjectLibraryWith(scan.ProjectLibraryItem)
	inv := inventory.New(st)
	// The catalog the app wires; it is never asked anything here, only what it fetches by.
	keys := mptmdb.New(nil, nil).Capabilities().Resolves
	return &scan.Library{Roots: roots, ItemStore: st, NeedStore: st, Orders: st, CollectionStore: st, Records: st, Keys: keys, Inventory: inv, Scanner: scan.NewScanner(&scan.Importer{
		Store: st, Inventory: inv, Disk: roots.Disk,
		Resolver: resolver, Keys: keys, Enabled: func() bool { return true },
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
	// A folder that holds something: an empty one reports folder_empty, not ok (#2151).
	if err := os.WriteFile(filepath.Join(tv, "Pilot.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
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
		Step       string     `json:"step"`
		LastScanAt *time.Time `json:"lastScanAt"`
	} `json:"scan"`
	Files int `json:"files"`
	Held  []struct {
		Path       string   `json:"path"`
		Movies     int      `json:"movies"`
		Episodes   int      `json:"episodes"`
		Other      int      `json:"other"`
		Titles     []string `json:"titles"`
		TitleCount int      `json:"titleCount"`
	} `json:"held"`
	Unreadable []string `json:"unreadable"`
	Skipped    []struct {
		Reason string   `json:"reason"`
		Total  int      `json:"total"`
		Paths  []string `json:"paths"`
	} `json:"skipped"`
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

// The files a scan skips reach the status by why, each with its path: a shortcut whose file is
// gone, and one to a file outside the media folders.
func TestLibraryScan_StatusListsSkippedFiles(t *testing.T) {
	h, lib := newScanHarness(t)
	movies, elsewhere := t.TempDir(), t.TempDir()
	_ = decodeLibraryRoots(t, h.Do(http.MethodPost, "/v1/library/roots", adminToken, `{"path":"`+movies+`"}`))
	if err := os.WriteFile(filepath.Join(movies, "Kept (2001).mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(elsewhere, "Away (2002).mkv")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken, away := filepath.Join(movies, "Gone (2003).mkv"), filepath.Join(movies, "Away (2002).mkv")
	if err := os.Symlink(filepath.Join(elsewhere, "nothing.mkv"), broken); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, away); err != nil {
		t.Fatal(err)
	}
	st := scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	if st.Skipped == nil || len(st.Skipped) != 0 {
		t.Fatalf("before a scan skipped = %+v, want an empty list", st.Skipped)
	}
	if err := lib.Scanner.Importer.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	st = scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	got := map[string][]string{}
	for _, s := range st.Skipped {
		if s.Total != len(s.Paths) {
			t.Errorf("%s total = %d, paths %q", s.Reason, s.Total, s.Paths)
		}
		got[s.Reason] = s.Paths
	}
	if !slices.Equal(got["broken_link"], []string{broken}) || !slices.Equal(got["link_outside"], []string{away}) || len(got) != 2 {
		t.Fatalf("skipped = %+v", st.Skipped)
	}
}

// The folder browser lists from the folders the apps name, down, never outside them, for an
// admin only; a folder overlapping one in use is refused with 409, and replace takes the outer one.
func TestLibraryBrowse_ListsDownFromTheAppsFoldersAndRefusesOverlaps(t *testing.T) {
	share := t.TempDir()
	for _, dir := range []string{"Movies/Wrenfield (2004)", "TV", "@eaDir"} {
		if err := os.MkdirAll(filepath.Join(share, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := newScanHarness(t, share)
	if resp := h.Do(http.MethodPost, "/v1/library/roots/discover", adminToken, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("discover: %d", resp.StatusCode)
	}
	type listing struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Folders []struct {
			Path   string   `json:"path"`
			Name   string   `json:"name"`
			Named  bool     `json:"named"`
			InUse  bool     `json:"inUse"`
			Within string   `json:"within"`
			Holds  []string `json:"holds"`
		} `json:"folders"`
	}
	browse := func(path string, want int) listing {
		t.Helper()
		resp := h.Do(http.MethodGet, "/v1/library/browse?path="+url.QueryEscape(path), adminToken, "")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != want {
			t.Fatalf("browse %q: %d, want %d", path, resp.StatusCode, want)
		}
		var l listing
		if want == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
				t.Fatal(err)
			}
		}
		return l
	}

	top := browse("", http.StatusOK)
	if len(top.Folders) != 1 || top.Folders[0].Path != share || !top.Folders[0].Named || top.Folders[0].Holds == nil {
		t.Fatalf("top = %+v, want Sonarr's folder alone", top.Folders)
	}
	in := browse(share, http.StatusOK)
	if len(in.Folders) != 2 || in.Folders[0].Name != "Movies" || in.Folders[1].Name != "TV" || in.Parent != "" {
		t.Fatalf("in the share = %+v, want Movies and TV under the top", in)
	}
	browse(filepath.Dir(share), http.StatusForbidden)
	browse("relative", http.StatusUnprocessableEntity)
	if resp := h.Do(http.MethodGet, "/v1/library/browse", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a member browsing: %d, want 403", resp.StatusCode)
	}

	movies := filepath.Join(share, "Movies")
	add := func(body string, want int) {
		t.Helper()
		if resp := h.Do(http.MethodPost, "/v1/library/roots", adminToken, body); resp.StatusCode != want {
			t.Fatalf("add %s: %d, want %d", body, resp.StatusCode, want)
		}
	}
	add(`{"path":"`+movies+`"}`, http.StatusOK)
	add(`{"path":"`+filepath.Join(movies, "Wrenfield (2004)")+`"}`, http.StatusConflict)
	add(`{"path":"`+share+`"}`, http.StatusConflict)
	if f := browse(movies, http.StatusOK).Folders; len(f) != 1 || f[0].Within != movies {
		t.Fatalf("in Movies = %+v, want its film's folder within Movies", f)
	}
	add(`{"path":"`+share+`","replace":true}`, http.StatusOK)
	roots := decodeLibraryRoots(t, h.Do(http.MethodGet, "/v1/library/roots", adminToken, ""))
	for _, r := range roots.Roots {
		if r.InUse != (r.Path == share) {
			t.Errorf("%s in use = %v after replace", r.Path, r.InUse)
		}
	}
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
	if !st.Scan.Enabled || st.Scan.Step != "idle" || st.Scan.LastScanAt == nil || st.Files != 5 || len(st.Held) != 0 ||
		st.Unreadable == nil || len(st.Unreadable) != 0 {
		t.Fatalf("after first scan %+v", st)
	}

	for _, p := range paths[:2] {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(movies, "Foxtrot \xe9.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lib.Scanner.Importer.Run(ctx); err != nil {
		t.Fatal(err)
	}
	st = scanStatus(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), http.StatusOK)
	if len(st.Held) != 1 || st.Held[0].Path != movies || st.Held[0].Movies != 2 || st.Held[0].Episodes != 0 {
		t.Fatalf("held %+v", st.Held)
	}
	// A name that is not UTF-8 is listed for renaming, as near its name as text allows.
	if want := []string{filepath.Join(movies, "Foxtrot �.mkv")}; !slices.Equal(st.Unreadable, want) {
		t.Fatalf("unreadable = %q, want %q", st.Unreadable, want)
	}
	// The maintainer sees which titles they would let go.
	if got := st.Held[0]; strings.Join(got.Titles, ",") != "Alpha,Bravo" || got.TitleCount != 2 {
		t.Fatalf("held titles = %q of %d, want Alpha and Bravo", got.Titles, got.TitleCount)
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
