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

// The seeded titles a member must never read: each is waiting in Needs you, ignored, or an
// episode still named by its file, and each is named from its file or folder.
const (
	unidentifiedMovie  = "90000005" // named from its folder, nobody has answered it
	ignoredMovie       = "90000007" // identified, but the admin said it is not a title
	unidentifiedSeries = "90000008" // named from its folder
	unidentifiedEp     = "90000081" // a numbered episode of the unidentified series
	fileNamedEpisode   = "90000012" // an identified series' episode still named by its file
	unmarkedEpisode    = "90000013" // numbered and not marked, but its name is still its file's
	unnamedMovie       = "90000009" // identified, its catalogue record not read yet: its file's name
	answeredMovie      = "90000006" // nothing identifies it, but the admin answered it
	readyMadeSet       = "90000811" // a ready-made set holding the identified movie and hidden ones
)

// seedMemberLibrary stores an identified movie whose folder name differs from its title, with the
// copy that plays (its subtitle in a file beside it), an extra copy, a file whose header couldn't
// be read and a held removal; an identified series with one episode in a season folder and one
// still named by its file; and the titles waiting in Needs you, ignored, or answered (above).
func seedMemberLibrary(t *testing.T, st records.Store) {
	t.Helper()
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	const dir = "/media/Alpha Folder (2001)/"
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	grounded := librarystore.LibraryMatch{Basis: "embedded_id"}
	named := func(provider string) librarystore.LibraryFrom { return librarystore.LibraryFrom{Name: provider} }
	set := librarystore.LibraryCatalogKey{Provider: "tmdb_collection", Kind: "collection", ExternalID: readyMadeSet}
	streams := []librarystore.LibraryStream{
		{Index: 0, Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, HDR: "HDR10"},
		{Index: 1, Kind: "audio", Codec: "eac3", Channels: 6, Language: "en", Default: true},
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"},
		{Index: 3, Kind: "subtitle", Codec: "subrip", Language: "es", ExternalRef: dir + "alpha.es.srt"},
		{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "en", ExternalRef: dir + "alpha.en.sdh.srt"},
	}
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie", Year: 2001, Overview: "A story.",
			Origin: origin(dir + "alpha.mkv"), Match: grounded, Set: set,
			From: librarystore.LibraryFrom{Name: "tmdb", Year: "nfo"},
			Sources: []librarystore.LibrarySource{
				{Path: dir + "alpha.mkv", Plays: true, DurationMs: 5_400_000, Streams: streams},
				{Path: dir + "alpha.copy.mkv", Plays: false, DurationMs: 5_400_000},
			}},
		{ID: "90000002", Kind: "series", Name: "Bravo Show", Origin: origin("/media/Bravo Folder"), Match: grounded, From: named("tvdb")},
		{ID: "90000011", Kind: "episode", Name: "Bravo E1", SeriesID: "90000002", Season: 1, Episode: 1, From: named("tvdb"),
			Sources: []librarystore.LibrarySource{{Path: "/media/Bravo Folder/Season 01/bravo.e1.mkv", Plays: true,
				Streams: streams[:2]}}},
		{ID: fileNamedEpisode, Kind: "episode", Name: "bravo.e2 From File", SeriesID: "90000002", Season: 1, Episode: 2,
			Unnumbered: true},
		{ID: unmarkedEpisode, Kind: "episode", Name: "bravo.e3 Unmarked File", SeriesID: "90000002", Season: 1, Episode: 3},
		{ID: unidentifiedMovie, Kind: "movie", Name: "Charlie Folder Extras", Origin: origin("/media/Charlie Folder Extras/cast.mkv"), Set: set},
		{ID: answeredMovie, Kind: "movie", Name: "Delta Answered", Origin: origin("/media/delta.mkv"), From: named("tmdb")},
		{ID: ignoredMovie, Kind: "movie", Name: "Echo Folder Ignored", Origin: origin("/media/Echo Folder Ignored/echo.mkv"),
			Match: grounded, From: named("tmdb"), Set: set},
		{ID: unnamedMovie, Kind: "movie", Name: "Golf Folder Unnamed", Origin: origin("/media/Golf Folder Unnamed/golf.mkv"),
			Match: grounded, Set: set},
		{ID: unidentifiedSeries, Kind: "series", Name: "Foxtrot Folder", Origin: origin("/media/Foxtrot Folder")},
		{ID: unidentifiedEp, Kind: "episode", Name: "Foxtrot Numbered", SeriesID: unidentifiedSeries, Season: 1, Episode: 1,
			From: named("tvdb")},
	}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{set}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveLibraryCatalog(ctx, set, `{"Kind":"collection","Descriptive":{"Title":"The Hotel Set"},"Children":[
		{"Kind":"movie","IDs":[{"Namespace":"tmdb","Value":"90000812"}]},
		{"Kind":"movie","IDs":[{"Namespace":"tmdb","Value":"90000813"}]}]}`, 0, "en-US", at); err != nil {
		t.Fatal(err)
	}
	for _, answer := range []librarystore.LibraryAnswer{
		{Origin: origin("/media/delta.mkv"), Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000206"}, AnsweredAt: at},
		{Origin: origin("/media/Echo Folder Ignored/echo.mkv"), Kind: "movie", Ignored: true, AnsweredAt: at},
	} {
		if err := st.SaveLibraryAnswer(ctx, answer); err != nil {
			t.Fatal(err)
		}
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
		file("/media/Bravo Folder/Season 01/bravo.e2.mkv", fileNamedEpisode, "90000002", librarystore.LibraryHeaderRead, false),
		file("/media/Bravo Folder/Season 01/bravo.e3.mkv", unmarkedEpisode, "90000002", librarystore.LibraryHeaderRead, false),
		file("/media/Golf Folder Unnamed/golf.mkv", unnamedMovie, "", librarystore.LibraryHeaderRead, false),
		file("/media/Charlie Folder Extras/cast.mkv", unidentifiedMovie, "", librarystore.LibraryHeaderRead, false),
		file("/media/delta.mkv", answeredMovie, "", librarystore.LibraryHeaderRead, false),
		file("/media/Echo Folder Ignored/echo.mkv", ignoredMovie, "", librarystore.LibraryHeaderRead, false),
		file("/media/Foxtrot Folder/e1.mkv", unidentifiedEp, unidentifiedSeries, librarystore.LibraryHeaderRead, false),
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

// memberLeaks are pieces of the seeded files' paths and the names files and folders gave the
// titles a member doesn't read: none may reach a member in any field.
var memberLeaks = []string{"/media", "Alpha Folder", "Bravo Folder", "Season 01", "alpha.", "bravo.", ".mkv", ".srt",
	"From File", "Unmarked File", "Charlie Folder", "Echo Folder", "Foxtrot", "Golf Folder", unidentifiedMovie, ignoredMovie,
	unidentifiedSeries, unidentifiedEp, fileNamedEpisode, unmarkedEpisode, unnamedMovie}

// adminOnlyKeys are the JSON keys a member's Library responses leave out: a title's file count
// and files list (both "files"), its match and sources, and the Needs you counts and marks.
var adminOnlyKeys = []string{`"files":`, `"match":`, `"detailsFrom":`, `"unplayable":`, `"extraCopies":`, `"fromFile":`, `"path":`}

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

// A member reads only identified titles (#4476 security review): one waiting in Needs you, one
// the admin ignored, and an episode still named by its file are absent from the list, its search
// and total, and the collections, and opening one by id answers exactly what an unknown id does.
// An answered title is read. An admin reads them all, as before.
func TestMemberNeverReadsATitleWaitingInNeedsYou(t *testing.T) {
	h := newLibraryHarness(t)
	seedMemberLibrary(t, h.Store)
	type page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	listed := func(token, query string) ([]string, int) {
		t.Helper()
		var p page
		decode(t, h.Do(http.MethodGet, "/v1/library/items"+query, token, ""), &p)
		out := []string{}
		for _, it := range p.Items {
			out = append(out, it.ID)
		}
		slices.Sort(out)
		return out, p.Total
	}
	for _, tc := range []struct {
		token, query string
		want         []string
	}{
		{memberToken, "", []string{"90000001", "90000002", answeredMovie}},
		{memberToken, "?limit=1", []string{"90000001"}},
		{memberToken, "?search=folder", []string{}},
		{memberToken, "?kind=series", []string{"90000002"}},
		{adminToken, "", []string{"90000001", "90000002", unidentifiedMovie, answeredMovie, unidentifiedSeries, unnamedMovie}},
		{adminToken, "?search=folder", []string{unidentifiedMovie, unidentifiedSeries, unnamedMovie}},
	} {
		got, total := listed(tc.token, tc.query)
		wantTotal := len(tc.want)
		if tc.query == "?limit=1" {
			wantTotal = 3
		}
		if !slices.Equal(got, tc.want) || total != wantTotal {
			t.Errorf("%s list%s = %v (total %d), want %v (total %d)", tc.token, tc.query, got, total, tc.want, wantTotal)
		}
	}

	// A hidden title's 404 is the unknown id's, word for word.
	problemOf := func(token, id string) (int, string) {
		t.Helper()
		resp := h.Do(http.MethodGet, "/v1/library/items/"+id, token, "")
		defer func() { _ = resp.Body.Close() }()
		var p struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, p.Title + " / " + p.Detail
	}
	unknownStatus, unknown := problemOf(memberToken, "90000099")
	if unknownStatus != http.StatusNotFound {
		t.Fatalf("unknown id = %d", unknownStatus)
	}
	for _, id := range []string{unidentifiedMovie, ignoredMovie, unidentifiedSeries, unidentifiedEp, fileNamedEpisode,
		unmarkedEpisode, unnamedMovie} {
		if status, body := problemOf(memberToken, id); status != unknownStatus || body != unknown {
			t.Errorf("member opens %s = %d %q, want the unknown id's %d %q", id, status, body, unknownStatus, unknown)
		}
		if resp := h.Do(http.MethodGet, "/v1/library/items/"+id, adminToken, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("admin opens %s = %d, want 200", id, resp.StatusCode)
		}
	}

	// The identified series stays, its season without the episodes their files still name: one
	// marked unnumbered, one the scanner left unmarked.
	var series struct {
		Seasons []struct {
			Episodes []struct {
				ID string `json:"id"`
			} `json:"episodes"`
		} `json:"seasons"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items/90000002", memberToken, ""), &series)
	if len(series.Seasons) != 1 || len(series.Seasons[0].Episodes) != 1 || series.Seasons[0].Episodes[0].ID != "90000011" {
		t.Errorf("member series seasons = %+v", series.Seasons)
	}
	admin := okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000002", adminToken, ""))
	if !strings.Contains(admin, `"id":"`+fileNamedEpisode+`"`) || !strings.Contains(admin, `"fromFile":true`) ||
		!strings.Contains(admin, `"id":"`+unmarkedEpisode+`"`) {
		t.Errorf("admin series lacks the file-named episodes: %s", admin)
	}

	// A ready-made set holds only the member's titles; the admin's keeps the ones they don't read
	// (an ignored title is left out of every list, the admin's too).
	setTitles := func(token string) []string {
		t.Helper()
		var got struct {
			Sets []struct {
				ID     string `json:"id"`
				Titles []struct {
					ID string `json:"id"`
				} `json:"titles"`
			} `json:"sets"`
		}
		decode(t, h.Do(http.MethodGet, "/v1/collections", token, ""), &got)
		if len(got.Sets) != 1 || got.Sets[0].ID != readyMadeSet {
			t.Fatalf("%s sets = %+v, want the one set", token, got.Sets)
		}
		ids := []string{}
		for _, title := range got.Sets[0].Titles {
			ids = append(ids, title.ID)
		}
		slices.Sort(ids)
		return ids
	}
	if got, want := setTitles(memberToken), []string{"90000001"}; !slices.Equal(got, want) {
		t.Errorf("member set titles = %v, want %v", got, want)
	}
	if got, want := setTitles(adminToken), []string{"90000001", unidentifiedMovie, unnamedMovie}; !slices.Equal(got, want) {
		t.Errorf("admin set titles = %v, want %v", got, want)
	}
}

// A member reads every title fact and the one line about the copy that plays, and nothing that
// names a file or that a fix needs: not on the list, not on the title, not on an episode's own
// page, not through a filter, and not through the collections.
func TestMemberReadsLibraryWithoutFilesOrFixFacts(t *testing.T) {
	h := newLibraryHarness(t)
	seedMemberLibrary(t, h.Store)
	if resp := h.Do(http.MethodPost, "/v1/collections", adminToken,
		`{"name":"Favourites","titles":["90000001","90000002","`+unidentifiedMovie+`","`+ignoredMovie+`","`+
			unidentifiedSeries+`","`+answeredMovie+`"]}`); resp.StatusCode != http.StatusCreated {
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
		"/v1/library/items?search=folder", "/v1/library/items?kind=series", "/v1/library/items?sort=recent&limit=1&offset=1",
		"/v1/library/items/90000001", "/v1/library/items/90000002", "/v1/library/items/90000011",
		"/v1/library/items/" + answeredMovie,
		"/v1/collections", "/v1/collections/" + made.Collections[0].ID,
	} {
		assertMemberSafe(t, route, okBody(t, h.Do(http.MethodGet, route, memberToken, "")))
	}

	// The member's title keeps its facts and readiness and gains the playing copy's one line.
	var movie struct {
		Item struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
		} `json:"item"`
		Overview string          `json:"overview"`
		Playing  json.RawMessage `json:"playing"`
	}
	if err := json.Unmarshal([]byte(okBody(t, h.Do(http.MethodGet, "/v1/library/items/90000001", memberToken, ""))), &movie); err != nil {
		t.Fatal(err)
	}
	const playing = `{"picture":{"width":3840,"height":2160,"hdr":true},"audioChannels":6,"subtitleLanguages":["en","es"]}`
	if movie.Item.Name != "Alpha Movie" || movie.Overview != "A story." || !movie.Item.Ready || string(movie.Playing) != playing {
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
	if !strings.Contains(list, `"unplayable":0`) || !strings.Contains(list, `"extraCopies":0`) || !strings.Contains(list, `"files":3`) {
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

// newLibrarySessionHarness serves the real Library behind the production session and device
// authorizer, with an admin ("boss") and a member ("kid") who sign in and pair devices.
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
		devices := auth.NewDeviceManager(defaults.Store, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", identitystore.RoleAdmin)
		seedImported(t, defaults.Store, "u-kid", "kid", identitystore.RoleMember)
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Log: defaults.Log, CookieSecure: "false",
			Auth: api.NewSessionAuthorizerCurrent(mgr, devices, func(context.Context) (string, error) { return "break-glass-token", nil }),
			People: api.PeopleOptions{
				Login:     auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
				Sessions:  mgr,
				Devices:   devices,
				Passwords: auth.NewPasswordService(defaults.Store, func() string { return "u-new" }, time.Now),
				UserSync:  auth.NewUserSync(lib, defaults.Store, time.Now),
			},
			Library: api.LibraryOptions{Roots: newScanLibrary(t, defaults.Store)},
		})
	})
}

// §19: a signed-in member, and a device they paired, read the Library and are refused its fixes;
// once an admin disables them, neither the session nor the device reads any of the
// member-readable routes.
func TestDisabledMemberLosesTheLibraryOnSessionAndDevice(t *testing.T) {
	h := newLibrarySessionHarness(t)
	seedMemberLibrary(t, h.Store)
	srv := h.Server
	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")
	tv := pairDevice(t, srv, member)
	code, made := deviceCall(t, srv, http.MethodPost, "/v1/collections", admin, "", `{"name":"Favourites","titles":["90000001"]}`)
	if code != http.StatusCreated {
		t.Fatalf("make collection = %d (%v)", code, made)
	}
	routes := []string{"/v1/library/items", "/v1/library/items/90000001", "/v1/library/genres", "/v1/collections",
		"/v1/collections/" + made["id"].(string)}

	for _, route := range routes {
		assertMemberSafe(t, route, okBody(t, authed(t, http.MethodGet, srv.URL+route, member, "")))
		if code, body := deviceCall(t, srv, http.MethodGet, route, nil, tv, ""); code != http.StatusOK {
			t.Errorf("member's device %s = %d (%v), want 200", route, code, body)
		}
	}
	resp := authed(t, http.MethodPost, srv.URL+"/v1/library/items/90000001/check", member, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("signed-in member check = %d, want 403", resp.StatusCode)
	}
	if code, _ := deviceCall(t, srv, http.MethodPost, "/v1/library/items/90000001/check", nil, tv, ""); code != http.StatusForbidden {
		t.Errorf("member's device check = %d, want 403", code)
	}

	resp = authed(t, http.MethodPatch, srv.URL+"/v1/users/u-kid", admin, `{"disabled":true}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable = %d", resp.StatusCode)
	}
	for _, route := range routes {
		resp := authed(t, http.MethodGet, srv.URL+route, member, "")
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("disabled member %s = %d, want 401", route, resp.StatusCode)
		}
		if code, _ := deviceCall(t, srv, http.MethodGet, route, nil, tv, ""); code != http.StatusUnauthorized {
			t.Errorf("disabled member's device %s = %d, want 401", route, code)
		}
	}
}
