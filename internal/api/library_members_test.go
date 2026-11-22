package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// A household member browses the Library read-only (#4476, library.md "Who reads the Library").
// These pin the §19 negatives for it: a member is refused every other Library route, reads no
// file, path or fix fact, and loses the Library the moment they are disabled.

// libraryMemberReads are the Library operations a member may call. Every other operation under
// /v1/library or /v1/collections is admin-only, except the public hook receiver.
var libraryMemberReads = []string{"getCollection", "getLibraryItem", "listCollections", "listLibraryCollections", "listLibraryItems"}

// libraryPublic is the Library's one public operation: an *arr's webhook, which answers only its
// app's shared secret.
var libraryPublic = []string{"receiveLibraryHook"}

func libraryOperations(t *testing.T) []api.RegisteredOperation {
	t.Helper()
	ops, err := api.RegisteredOperations()
	if err != nil {
		t.Fatal(err)
	}
	var out []api.RegisteredOperation
	for _, op := range ops {
		if strings.HasPrefix(op.Path, "/v1/library") || strings.HasPrefix(op.Path, "/v1/collections") {
			out = append(out, op)
		}
	}
	if len(out) < 30 {
		t.Fatalf("found %d Library operations — the enumeration broke, not the API", len(out))
	}
	return out
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// Every Library operation is either one a member reads, the public hook, or admin-only, and a
// member is refused each admin-only one. The routes come from the registry, so a route added
// later as member-readable or public fails here until it is named above, and one added as admin
// is probed for its 403.
func TestMemberIsRefusedOnEveryAdminLibraryRoute(t *testing.T) {
	h := newLibraryHarness(t)
	var reads, public []string
	refused := 0
	for _, op := range libraryOperations(t) {
		switch op.Role {
		case httpcore.RoleMember:
			reads = append(reads, op.ID)
			continue
		case httpcore.RolePublic:
			public = append(public, op.ID)
			continue
		case httpcore.RoleAdmin:
		default:
			t.Errorf("%s %s requires %q", op.Method, op.Path, op.Role)
			continue
		}
		path := pathParam.ReplaceAllString(op.Path, "90000001")
		body := ""
		if op.Method != http.MethodGet && op.Method != http.MethodDelete {
			body = `{}`
		}
		resp := h.Do(op.Method, path, memberToken, body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("member %s %s (%s) = %d, want 403", op.Method, path, op.ID, resp.StatusCode)
		}
		refused++
	}
	slices.Sort(reads)
	slices.Sort(public)
	if !slices.Equal(reads, libraryMemberReads) {
		t.Errorf("member-readable Library operations = %v, want %v", reads, libraryMemberReads)
	}
	if !slices.Equal(public, libraryPublic) {
		t.Errorf("public Library operations = %v, want %v", public, libraryPublic)
	}
	if refused < 20 {
		t.Errorf("probed %d admin-only Library operations, want every one", refused)
	}
}

// The title list filters only on what a member reads: a filter on an admin fact (unplayable
// files, extra copies) would let a member find them by elimination.
func TestLibraryListFiltersRevealNoAdminFact(t *testing.T) {
	for _, op := range libraryOperations(t) {
		if op.ID != "listLibraryItems" {
			continue
		}
		if want := []string{"kind", "limit", "offset", "ready", "search", "sort"}; !slices.Equal(op.Query, want) {
			t.Errorf("listLibraryItems filters = %v, want %v: decide whether a member may filter on it", op.Query, want)
		}
		return
	}
	t.Fatal("listLibraryItems is not registered")
}

// seedMemberLibrary stores a movie whose folder name differs from its title, with the copy that
// plays (its subtitle in a file beside it), an extra copy, a file whose header couldn't be read
// and a held removal, and a series with one episode in a season folder.
func seedMemberLibrary(t *testing.T, st records.Store) {
	t.Helper()
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	const dir = "/media/Alpha Folder (2001)/"
	streams := []librarystore.LibraryStream{
		{Index: 0, Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, HDR: "HDR10"},
		{Index: 1, Kind: "audio", Codec: "eac3", Channels: 6, Language: "en", Default: true},
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"},
		{Index: 3, Kind: "subtitle", Codec: "subrip", Language: "es", ExternalRef: dir + "alpha.es.srt"},
		{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "en", ExternalRef: dir + "alpha.en.sdh.srt"},
	}
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie", Year: 2001, Overview: "A story.",
			From: librarystore.LibraryFrom{Year: "nfo"},
			Sources: []librarystore.LibrarySource{
				{Path: dir + "alpha.mkv", Plays: true, DurationMs: 5_400_000, Streams: streams},
				{Path: dir + "alpha.copy.mkv", Plays: false, DurationMs: 5_400_000},
			}},
		{ID: "90000002", Kind: "series", Name: "Bravo Show"},
		{ID: "90000011", Kind: "episode", Name: "Bravo E1", SeriesID: "90000002", Season: 1, Episode: 1,
			Sources: []librarystore.LibrarySource{{Path: "/media/Bravo Folder/Season 01/bravo.e1.mkv", Plays: true,
				Streams: streams[:2]}}},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string, held bool) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 7, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series, Held: held}
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file(dir+"alpha.mkv", "90000001", "", librarystore.LibraryHeaderRead, false),
		file(dir+"alpha.copy.mkv", "90000001", "", librarystore.LibraryHeaderRead, false),
		file(dir+"alpha.broken.mkv", "90000001", "", librarystore.LibraryHeaderFailed, false),
		file(dir+"alpha.held.mkv", "90000001", "", librarystore.LibraryHeaderRead, true),
		file("/media/Bravo Folder/Season 01/bravo.e1.mkv", "90000011", "90000002", librarystore.LibraryHeaderRead, false),
	}); err != nil {
		t.Fatal(err)
	}
}

// okBody is a response's body, failing unless it is a 200.
func okBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}
	return string(b)
}

// memberLeaks are pieces of the seeded files' paths: none may reach a member in any field.
var memberLeaks = []string{"/media", "Alpha Folder", "Bravo Folder", "Season 01", "alpha.", "bravo.", ".mkv", ".srt"}

// adminOnlyKeys are the JSON keys a member's Library responses leave out. A title's file count
// is `"files":<n>` and stays; its files list is `"files":[`.
var adminOnlyKeys = []string{`"files":[`, `"match":`, `"detailsFrom":`, `"unplayable":`, `"extraCopies":`, `"fromFile":`, `"path":`}

func assertMemberSafe(t *testing.T, route, body string) {
	t.Helper()
	for _, leak := range memberLeaks {
		if strings.Contains(body, leak) {
			t.Errorf("member %s carries %q: %s", route, leak, body)
		}
	}
	for _, key := range adminOnlyKeys {
		if strings.Contains(body, key) {
			t.Errorf("member %s carries the admin-only %s: %s", route, key, body)
		}
	}
}

// A member reads every title fact and the one line about the copy that plays, and nothing that
// names a file or that a fix needs: not on the list, not on the title, not on an episode's own
// page, not through a filter, and not through the collections.
func TestMemberReadsLibraryWithoutFilesOrFixFacts(t *testing.T) {
	h := newLibraryHarness(t)
	seedMemberLibrary(t, h.Store)
	if resp := h.Do(http.MethodPost, "/v1/collections", adminToken,
		`{"name":"Favourites","titles":["90000001","90000002"]}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("make collection = %d", resp.StatusCode)
	}
	var made struct {
		Collections []struct {
			ID string `json:"id"`
		} `json:"collections"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/collections", adminToken, ""), &made)
	if len(made.Collections) != 1 {
		t.Fatalf("collections = %+v", made)
	}

	for _, route := range []string{
		"/v1/library/items", "/v1/library/items?kind=movie&search=alpha&ready=true&sort=recent",
		"/v1/library/items/90000001", "/v1/library/items/90000002", "/v1/library/items/90000011",
		"/v1/collections", "/v1/collections/" + made.Collections[0].ID,
	} {
		assertMemberSafe(t, route, okBody(t, h.Do(http.MethodGet, route, memberToken, "")))
	}

	// The member's title keeps its facts and readiness and gains the playing copy's one line.
	var movie struct {
		Item struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
			Files int    `json:"files"`
		} `json:"item"`
		Overview string          `json:"overview"`
		Playing  json.RawMessage `json:"playing"`
	}
	if err := json.Unmarshal([]byte(okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000001", memberToken, ""))), &movie); err != nil {
		t.Fatal(err)
	}
	const playing = `{"picture":{"width":3840,"height":2160,"hdr":true},"audioChannels":6,"subtitleLanguages":["en","es"]}`
	if movie.Item.Name != "Alpha Movie" || movie.Overview != "A story." || movie.Item.Files == 0 || string(movie.Playing) != playing {
		t.Errorf("member movie = %+v, playing %s", movie, movie.Playing)
	}
	var episode struct {
		Playing json.RawMessage `json:"playing"`
	}
	if err := json.Unmarshal([]byte(okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000011", memberToken, ""))), &episode); err != nil {
		t.Fatal(err)
	}
	if want := `{"picture":{"width":3840,"height":2160,"hdr":true},"audioChannels":6,"subtitleLanguages":[]}`; string(episode.Playing) != want {
		t.Errorf("member episode playing = %s, want %s", episode.Playing, want)
	}

	// An admin reads the same title with its files and fix facts, and the same playing line.
	admin := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000001", adminToken, ""))), &admin); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"files", "match", "detailsFrom"} {
		if len(admin[key]) == 0 {
			t.Errorf("admin title has no %s", key)
		}
	}
	if string(admin["playing"]) != playing {
		t.Errorf("admin playing = %s, want %s", admin["playing"], playing)
	}
	var adminItem struct {
		Unplayable  *int `json:"unplayable"`
		ExtraCopies *int `json:"extraCopies"`
	}
	if err := json.Unmarshal(admin["item"], &adminItem); err != nil {
		t.Fatal(err)
	}
	if adminItem.Unplayable == nil || *adminItem.Unplayable != 1 || adminItem.ExtraCopies == nil || *adminItem.ExtraCopies != 1 {
		t.Errorf("admin item = %s", admin["item"])
	}
	var adminFiles []struct {
		Path  string `json:"path"`
		Extra bool   `json:"extra"`
	}
	if err := json.Unmarshal(admin["files"], &adminFiles); err != nil {
		t.Fatal(err)
	}
	extra := false
	for _, f := range adminFiles {
		extra = extra || (f.Extra && strings.HasSuffix(f.Path, "alpha.copy.mkv"))
	}
	if len(adminFiles) < 3 || !extra {
		t.Errorf("admin files = %s", admin["files"])
	}
}

// An admin's counts and marks stay on the wire at zero: a title with nothing wrong still says
// so, and a series lists its files as [] and each episode's fromFile as false.
func TestAdminLibraryResponsesKeepZeroFixFacts(t *testing.T) {
	h := newLibraryHarness(t)
	seedMemberLibrary(t, h.Store)
	list := okBody(t, h.Do(http.MethodGet, "/v1/library/items?kind=series", adminToken, ""))
	if !strings.Contains(list, `"unplayable":0`) || !strings.Contains(list, `"extraCopies":0`) {
		t.Errorf("admin series list = %s", list)
	}
	series := okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000002", adminToken, ""))
	for _, want := range []string{`"files":[]`, `"fromFile":false`, `"match":{`, `"detailsFrom":{`} {
		if !strings.Contains(series, want) {
			t.Errorf("admin series page lacks %s: %s", want, series)
		}
	}
	if strings.Contains(series, `"playing":`) {
		t.Errorf("a series has a playing copy: %s", series)
	}
}

// newLibrarySessionHarness serves the real Library behind the production session authorizer,
// with an admin ("boss") and a member ("kid") who sign in.
func newLibrarySessionHarness(t *testing.T) *apiHarness {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid", IsAdmin: false},
	}
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := mediaserver.New(mediaserver.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", identitystore.RoleAdmin)
		seedImported(t, defaults.Store, "u-kid", "kid", identitystore.RoleMember)
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: sessionAuth(mgr, "break-glass-token"), Log: defaults.Log, CookieSecure: "false",
			People: api.PeopleOptions{
				Login:     auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
				Sessions:  mgr,
				Passwords: auth.NewPasswordService(defaults.Store, func() string { return "u-new" }, time.Now),
				UserSync:  auth.NewUserSync(lib, defaults.Store, time.Now),
			},
			Library: api.LibraryOptions{Roots: newScanLibrary(t, defaults.Store)},
		})
	})
}

// §19: a signed-in member reads the Library and is refused its fixes; once an admin disables
// them, their session reads nothing, the list or a title.
func TestDisabledMembersSessionLosesTheLibrary(t *testing.T) {
	h := newLibrarySessionHarness(t)
	seedMemberLibrary(t, h.Store)
	srv := h.Server
	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")

	assertMemberSafe(t, "/v1/library/items/90000001", okBody(t, authed(t, http.MethodGet, srv.URL+"/v1/library/items/90000001", member, "")))
	resp := authed(t, http.MethodPost, srv.URL+"/v1/library/items/90000001/check", member, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("signed-in member check = %d, want 403", resp.StatusCode)
	}

	resp = authed(t, http.MethodPatch, srv.URL+"/v1/users/u-kid", admin, `{"disabled":true}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable = %d", resp.StatusCode)
	}
	for _, route := range []string{"/v1/library/items", "/v1/library/items/90000001", "/v1/collections"} {
		resp := authed(t, http.MethodGet, srv.URL+route, member, "")
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("disabled member %s = %d, want 401", route, resp.StatusCode)
		}
	}
}
