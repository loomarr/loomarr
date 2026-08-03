package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

func sourceReq(t *testing.T, method, url, body, token string) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// Registering a source records that it EXISTS and is allowed. It must not download anything —
// that is the ingest path, and a composed pull goes through the approval gate.
func TestAddFillerSource_RegistersEnabledAndDownloadsNothing(t *testing.T) {
	srv, st, ff := newFillerServer(t)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
		`{"kind":"archive","uri":"https://archive.org/details/classic_tv_commercials"}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "archive:classic_tv_commercials" {
		t.Errorf("id = %q, want the parsed identifier", body.ID)
	}
	// ⚠ A Go bool zero-values to false, so a source registered "off" is the failure mode this
	// asserts against: it would sit in the UI switched off, never fetch, and look like a bug.
	if !body.Enabled {
		t.Error("a newly registered source is switched off")
	}
	if ff.syncs != 0 {
		t.Errorf("registering triggered %d syncs — it must download nothing", ff.syncs)
	}

	// ⚠ Counted over the REGISTERED rows, not the whole table. V37's migration seeds the two
	// config-backed singletons (`folder`, `library`) so the flat list can still say "not
	// configured", so a total-row count would be asserting on the migration rather than on what
	// this request did.
	got := registeredSources(t, st)
	if len(got) != 1 {
		t.Fatalf("store holds %d registered sources, want 1", len(got))
	}
	if !got[0].Enabled {
		t.Error("persisted source is disabled")
	}
}

// registeredSources returns the DOWNLOADABLE rows — archive collections and YouTube playlists —
// skipping the folder/library rows migration 00029 materialises on every install, so a test
// asserting "this request registered one source" is not really asserting on the migration.
//
// ⚠ Since V38c an operator can ADD folders and libraries too (§10), so this no longer means
// "everything the operator added". A test about those kinds must read the whole table; see
// TestAddFillerSource_AcceptsFoldersAndLibraries.
func registeredSources(t *testing.T, st store.Store) []store.FillerSource {
	t.Helper()
	all, err := st.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []store.FillerSource
	for _, s := range all {
		if s.Kind != "folder" && s.Kind != "library" {
			out = append(out, s)
		}
	}
	return out
}

// An unparseable paste must be refused, not turned into a row that can never fetch.
func TestAddFillerSource_RefusesSomethingThatIsNotACollection(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
		`{"kind":"archive","uri":"https://example.com/some/page"}`, adminToken)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if got := registeredSources(t, st); len(got) != 0 {
		t.Errorf("stored %d sources for an unparseable uri", len(got))
	}
}

// V37: YouTube is a registrable source kind, not only an ingest URL. The pre-V37 handler
// hardcoded kind="archive" and ran every URI through `archiveIdentifier`, which rejects anything
// containing a dot — so a playlist URL 400'd with a message about archive.org collections.
func TestAddFillerSource_RegistersAYouTubePlaylist(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
		`{"kind":"youtube","uri":"https://www.youtube.com/playlist?list=PL123"}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got := registeredSources(t, st)
	if len(got) != 1 {
		t.Fatalf("registered %d sources, want 1", len(got))
	}
	if got[0].Kind != "youtube" {
		t.Errorf("kind = %q, want youtube", got[0].Kind)
	}
	// ⚠ The row id is namespaced by kind. With one table holding both vocabularies, a bare
	// identifier lets a YouTube row silently UPSERT over an archive row that parsed to the
	// same string.
	if got[0].ID == "" || got[0].ID[:8] != "youtube:" {
		t.Errorf("id = %q, want a youtube:-namespaced id", got[0].ID)
	}
}

// ⚠ Registration is STRICTER than ingest, deliberately. `clipfetch.KindForURL` defaults an
// unknown host to YouTube because yt-dlp handles the widest set of sites — right for an admin
// pasting one URL and watching it work, wrong for a row that persists and fetches unattended.
//
// The host check is exact for a security reason as well as a correctness one: "youtube.com.evil
// .test" CONTAINS the string, and a substring check would register an attacker-chosen host.
func TestAddFillerSource_RefusesANearMissYouTubeHost(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	for _, uri := range []string{
		"https://youtube.com.evil.test/playlist?list=PL1",
		"https://notyoutube.com/playlist?list=PL1",
		"https://example.com/watch?v=abc",
		// A bare host identifies nothing to fetch.
		"https://youtube.com",
	} {
		res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
			`{"kind":"youtube","uri":"`+uri+`"}`, adminToken)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", uri, res.StatusCode)
		}
	}
	if got := registeredSources(t, st); len(got) != 0 {
		t.Errorf("stored %d sources for URLs that are not YouTube", len(got))
	}
}

// Folders and libraries are ADDABLE (§10 V38c). ⚠ This replaces
// TestAddFillerSource_RefusesTheConfigBackedKinds, which pinned the opposite rule: both kinds
// were singletons materialised from configuration and 409'd here. V38c allows many of each, and
// the maintainer's 2026-08-02 decision returned `library` to being scanned for real — so the
// dialog offering three kinds must not be refused by two of them.
func TestAddFillerSource_AcceptsFoldersAndLibraries(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	for _, tc := range []struct{ kind, uri, want string }{
		{"folder", "/data/other", "/data/other"},
		// Cleaned, so a trailing slash does not create a second row watching one directory —
		// the duplicate the dropped singleton index no longer prevents.
		{"folder", "/mnt/ads/", "/mnt/ads"},
		{"library", "Commercials", "Commercials"},
	} {
		res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
			`{"kind":"`+tc.kind+`","uri":"`+tc.uri+`"}`, adminToken)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s %q: status = %d, want 200", tc.kind, tc.uri, res.StatusCode)
		}
	}

	// ⚠ Read the WHOLE table, not registeredSources — that helper filters out `folder` and
	// `library` as config-backed singletons, which is the very assumption this test disproves.
	all, err := st.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var folders, libraries int
	for _, s := range all {
		switch s.Kind {
		case "folder":
			folders++
		case "library":
			libraries++
		}
	}
	// Two ADDED folders plus whatever migration 00029 seeded — the point is that a second folder
	// is now possible at all, which the singleton index used to forbid.
	if folders < 2 {
		t.Errorf("folder rows = %d, want at least the 2 just added — many folders is the V38c change", folders)
	}
	if libraries < 1 {
		t.Errorf("library rows = %d, want at least the 1 just added", libraries)
	}
}

// A folder path must be ABSOLUTE and must not need cleaning. A relative path resolves against
// Loomarr's working directory — an implementation detail that differs between a container and a
// `go run` — so "ads" would silently mean different directories in dev and in production, and the
// only symptom would be an empty catalog.
func TestAddFillerSource_RefusesUnusableFolderPaths(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	before := len(registeredSources(t, st))
	for _, uri := range []string{
		"ads",          // relative
		"data/filler",  // relative
		"/data/../etc", // needs cleaning; not what the operator typed
		"/",            // scanning the filesystem root
		"   ",          // blank
	} {
		res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
			`{"kind":"folder","uri":"`+uri+`"}`, adminToken)
		if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%q: status = %d, want 400", uri, res.StatusCode)
		}
	}
	if got := len(registeredSources(t, st)); got != before {
		t.Errorf("registered sources went %d → %d — an unusable folder path was stored", before, got)
	}
}

// The switch flips the column, and — the part worth pinning — leaves the clips alone. The
// Sources tab tells the operator "clips already in the catalog stay put"; that is a promise.
func TestSetFillerSourceEnabled_DisablingKeepsTheClips(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx, store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	seedClip(t, st, "classic/ad.mp4", "commercial", 1992, "kids", "toys")

	res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic", `{"enabled":false}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	got := registeredSources(t, st)
	if len(got) != 1 {
		t.Fatalf("registered sources = %d, want 1", len(got))
	}
	if got[0].Enabled {
		t.Error("source still enabled after being switched off")
	}
	if _, err := st.GetClip(ctx, "classic/ad.mp4"); err != nil {
		t.Errorf("disabling a source removed its clip: %v — the switch is not a delete", err)
	}
}

// A row with no work behind it gets no switch, and asking for one is refused rather than
// silently accepted. Storing a flag nothing reads is how a control that changes nothing ships.
//
// ⚠ `library` USED TO BE IN THIS LIST and no longer is. V35 refused it because nothing scanned a
// media-server library; V38c restored the scan (§10), so the row has real work to stop and a real
// switch. `remote` stays refused for a different reason that has not changed: it is a container
// whose children carry the switches.
func TestSetFillerSourceEnabled_RefusesRowsWithNothingToStop(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, id := range []string{"remote"} {
		res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/"+id, `{"enabled":false}`, adminToken)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", id, res.StatusCode)
		}
	}
}

// Per-source fetch overrides (§10 V38c). ⚠ THREE distinct states, all reachable, because a busy
// archive collection and a small playlist want different numbers and one global figure serves
// neither. The store column is nullable for this reason; this test proves the HTTP layer can
// actually express what the column can hold — the columns shipped in an earlier V38c step with
// no route reaching them, which is the declared-but-unconsumed shape §15 forbids.
func TestSetFillerSourceFetchPolicy_ThreeStatesAllReachable(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx,
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	sourceByID := func(id string) store.FillerSource {
		t.Helper()
		all, err := st.ListFillerSources(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range all {
			if s.ID == id {
				return s
			}
		}
		t.Fatalf("source %q not found", id)
		return store.FillerSource{}
	}

	// 1. A positive interval — poll this source on its own schedule.
	res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic",
		`{"enabled":true,"fetchEverySeconds":900,"fetchMaxPerRun":5}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got := sourceByID("classic")
	if got.FetchEverySeconds == nil || *got.FetchEverySeconds != 900 {
		t.Errorf("FetchEverySeconds = %v, want 900", got.FetchEverySeconds)
	}
	if got.FetchMaxPerRun == nil || *got.FetchMaxPerRun != 5 {
		t.Errorf("FetchMaxPerRun = %v, want 5", got.FetchMaxPerRun)
	}
	// The resolved behaviour, not just the column: 900s wins over the global.
	if every, ok := got.FetchEvery(time.Hour); !ok || every != 15*time.Minute {
		t.Errorf("FetchEvery = %v/%v, want 15m and pollable", every, ok)
	}

	// 2. ZERO — never auto-fetch this source. ⚠ Distinct from "unset": a plain int could not
	// tell these apart, and conflating them would read every untouched source as switched off.
	res = sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic",
		`{"enabled":true,"fetchEverySeconds":0}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got = sourceByID("classic")
	if got.FetchEverySeconds == nil || *got.FetchEverySeconds != 0 {
		t.Fatalf("FetchEverySeconds = %v, want a stored 0 (never)", got.FetchEverySeconds)
	}
	if _, ok := got.FetchEvery(time.Hour); ok {
		t.Error("a source set to never-fetch is still pollable — 0 was read as 'inherit'")
	}
	// ⚠ maxPerRun was OMITTED on that request, which means "inherit" — it must have been cleared
	// rather than left at 5. A partial write that keeps stale values is how an operator ends up
	// with tuning they cannot see and did not ask for.
	if got.FetchMaxPerRun != nil {
		t.Errorf("FetchMaxPerRun = %v, want nil — an omitted field means inherit", *got.FetchMaxPerRun)
	}

	// 3. Cleared back to inheriting the global. This is a real action an operator takes, so it
	// must be expressible — an override that can be set but never removed is a one-way door.
	res = sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic",
		`{"enabled":true}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got = sourceByID("classic")
	if got.FetchEverySeconds != nil || got.FetchMaxPerRun != nil {
		t.Errorf("overrides = %v/%v, want both nil — the source cannot be returned to the global",
			got.FetchEverySeconds, got.FetchMaxPerRun)
	}
	if every, ok := got.FetchEvery(time.Hour); !ok || every != time.Hour {
		t.Errorf("FetchEvery = %v/%v, want the global hour back", every, ok)
	}
}

// ⚠ `fetchMaxPerRun: 0` is REFUSED, not stored. "Fetch nothing per run" is what
// fetchEverySeconds=0 already says, and letting it be said twice invites the two to disagree —
// a source scheduled to poll but capped at nothing looks enabled and does nothing.
func TestSetFillerSourceFetchPolicy_RefusesAZeroCap(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx,
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic",
		`{"enabled":true,"fetchMaxPerRun":0}`, adminToken)
	if res.StatusCode != http.StatusUnprocessableEntity && res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want a validation refusal", res.StatusCode)
	}
}

// Deleting forgets the registration. ⚠ It must NOT take the clips: they are real files, already
// tagged and possibly pinned into a channel.
func TestDeleteFillerSource_ForgetsTheSourceNotTheClips(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx, store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	seedClip(t, st, "classic/ad.mp4", "commercial", 1992, "kids", "toys")

	if res := sourceReq(t, http.MethodDelete, srv.URL+"/v1/filler/sources/classic", "", adminToken); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if got := registeredSources(t, st); len(got) != 0 {
		t.Errorf("%d registered sources remain", len(got))
	}
	if _, err := st.GetClip(ctx, "classic/ad.mp4"); err != nil {
		t.Errorf("deleting a source removed its clip: %v", err)
	}
}

// The derived rows describe configuration; deleting one would have to mean "unset filler.dir",
// which belongs in Settings where the consequence is legible.
func TestDeleteFillerSource_RefusesTheDerivedRows(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	// ⚠ `remote` is NOT in this list any more (V37). It was the CONTAINER row the archive
	// collections nested under; the flat list has no container, so there is no such id to
	// protect — it 404s as an unknown source, which is the honest answer.
	for _, id := range []string{"folder", "library"} {
		res := sourceReq(t, http.MethodDelete, srv.URL+"/v1/filler/sources/"+id, "", adminToken)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", id, res.StatusCode)
		}
	}
}

// §19 negatives: these routes name filesystem paths and change what gets downloaded.
func TestFillerSourceRoutes_RequireAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/v1/filler/sources", `{"uri":"classic"}`},
		{http.MethodPatch, "/v1/filler/sources/classic", `{"enabled":false}`},
		{http.MethodDelete, "/v1/filler/sources/classic", ""},
	} {
		res := sourceReq(t, tc.method, srv.URL+tc.path, tc.body, "")
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent {
			t.Errorf("%s %s succeeded with no credential (%d)", tc.method, tc.path, res.StatusCode)
		}
	}
}

// An operator-added folder or library RENDERS as its own row (§10 V38c).
//
// ⚠ The list used to skip the `folder` and `library` KINDS wholesale, because migration 00029
// materialises one seeded row of each whose live state comes from configuration — emitting the
// stored copy too would list the drop-folder twice. Correct while there was exactly one of each;
// once V38c made them addable, that filter hid every source an operator added. They would POST,
// get a 200, and never see the row again.
func TestListFillerSources_ShowsOperatorAddedFoldersAndLibraries(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, body := range []string{
		`{"kind":"folder","uri":"/mnt/extra-ads"}`,
		`{"kind":"library","uri":"Commercials"}`,
	} {
		if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources", body, adminToken); res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", body, res.StatusCode)
		}
	}

	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/sources", "", adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Sources []struct {
			ID         string `json:"id"`
			Kind       string `json:"kind"`
			Target     string `json:"target"`
			Switchable bool   `json:"switchable"`
			Removable  bool   `json:"removable"`
		} `json:"sources"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	byTarget := map[string]bool{}
	for _, s := range body.Sources {
		byTarget[s.Target] = true
	}
	if !byTarget["/mnt/extra-ads"] {
		t.Errorf("the added folder is missing from %d rows — an operator adds a source and never sees it", len(body.Sources))
	}
	if !byTarget["Commercials"] {
		t.Error("the added library is missing from the list")
	}

	// ⚠ The configured drop-folder must still appear EXACTLY ONCE. The skip exists to stop the
	// seeded row double-listing it, and narrowing that skip must not have reopened it.
	seen := 0
	for _, s := range body.Sources {
		if s.ID == "folder" || s.Target == "/data/filler" {
			seen++
		}
	}
	if seen > 1 {
		t.Errorf("the drop-folder appears %d times — the seeded row is double-listing", seen)
	}

	// An added library is a real source doing real work, so it has a switch and can be forgotten.
	for _, s := range body.Sources {
		if s.Target == "Commercials" {
			if !s.Switchable {
				t.Error("an added library has no switch — V38c made it scanned, so there is work to stop")
			}
			if !s.Removable {
				t.Error("an added library cannot be removed")
			}
		}
	}
}
