package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// The source ROLL-UP (§10 V51c): three archive.org collections stop being three sibling rows and
// become one Archive.org row with the collections beneath it.
//
// ⚠ **The grouping is DERIVED from `kind` at read time — no column, no table, no migration** — so
// these tests are the whole specification of it. There is nothing persisted to inspect.

// sourcesFrom reads the sources read-model off a server built by newFillerServer.
func sourcesFrom(t *testing.T, srv *httptest.Server) []api.FillerSourceDTO {
	t.Helper()
	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/sources", "", adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET sources → %d, want 200", res.StatusCode)
	}
	defer func() { _ = res.Body.Close() }()
	var body struct {
		Sources []api.FillerSourceDTO `json:"sources"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Sources
}

// addArchive registers one archive collection.
func addArchive(t *testing.T, st store.Store, id, label string, fetched time.Time) {
	t.Helper()
	src := store.NewFillerSource(id, "archive", id, label, time.Unix(1_700_000_000, 0).UTC())
	src.LastFetchedAt = fetched
	if err := st.UpsertFillerSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
}

// indexOf returns a row's position in the flat array, or -1.
func indexOf(rows []api.FillerSourceDTO, id string) int {
	for i, r := range rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// ⚠ **THE wire contract: flat and PRE-ORDERED**, each group node immediately followed by its own
// children. A nested `children: []` would generate a recursive type through orval (which it
// handles badly) and the frontend has no tree primitive — a twirl-down renders from exactly this
// shape by hiding rows whose parent is collapsed. If the ordering breaks, the UI interleaves one
// provider's collections under another's header with nothing failing.
func TestSourceGroups_ChildrenFollowTheirGroupInOrder(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	addArchive(t, st, "classic", "Classic TV", time.Time{})
	addArchive(t, st, "vhs", "VHS Vault", time.Time{})

	rows := sourcesFrom(t, srv)
	group := indexOf(rows, "provider:archive")
	if group < 0 {
		t.Fatalf("no Archive.org group node in %+v", rows)
	}
	if !rows[group].Group {
		t.Error("the group node is not marked as a group")
	}
	for i, want := range []string{"classic", "vhs"} {
		at := group + 1 + i
		if at >= len(rows) || rows[at].ID != want {
			t.Fatalf("row %d = %q, want %q immediately under its group", at, rowID(rows, at), want)
		}
		if rows[at].ParentID != "provider:archive" {
			t.Errorf("%s.parentId = %q, want provider:archive", want, rows[at].ParentID)
		}
		if rows[at].Group {
			t.Errorf("%s is marked as a group — it is a leaf", want)
		}
	}
}

func rowID(rows []api.FillerSourceDTO, i int) string {
	if i < 0 || i >= len(rows) {
		return "<out of range>"
	}
	return rows[i].ID
}

// ⚠ **A provider is emitted even with ZERO children**, which is what keeps the top-level row count
// stable when an operator deletes their last collection: the service becomes an empty state
// inviting another, rather than vanishing and raising "where did Archive.org go?".
//
// `configured` is the field that distinguishes the two, and it is the reason `Configured()` was
// extracted: an empty provider is an INVITATION, a malformed remote is a FAULT, and the tab
// renders them differently.
func TestSourceGroups_EmptyProviderIsStillEmittedAsAnInvitation(t *testing.T) {
	srv, _, _ := newFillerServer(t) // no sources at all
	rows := sourcesFrom(t, srv)

	for _, id := range []string{"provider:archive", "provider:youtube"} {
		i := indexOf(rows, id)
		if i < 0 {
			t.Fatalf("%s is missing — a provider with no children must still be listed", id)
		}
		if rows[i].Configured {
			t.Errorf("%s.configured = true with no children; an empty provider is an invitation", id)
		}
		if rows[i].Count != 0 || rows[i].Enabled {
			t.Errorf("%s reports count=%d enabled=%v with no children", id, rows[i].Count, rows[i].Enabled)
		}
	}
}

// ⚠ **A group carries no switch and cannot be removed** — the opinionated call of the phase.
// Cascade-on-write would destroy each child's own choice, which the store forbids in as many
// words ("Disabling is not deleting… switching it back on restores what was there").
func TestSourceGroups_GroupOffersNoControls(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	addArchive(t, st, "classic", "Classic TV", time.Time{})

	rows := sourcesFrom(t, srv)
	g := rows[indexOf(rows, "provider:archive")]
	if g.Switchable {
		t.Error("the group is switchable — a master switch has to be a visible bulk write, not a cascade")
	}
	if g.Removable {
		t.Error("the group is removable — deleting a provider would silently delete every child")
	}
	if g.Fetchable || g.Searchable {
		t.Errorf("group fetchable=%v searchable=%v; a provider has no URI of its own", g.Fetchable, g.Searchable)
	}
}

// The rollups: count SUMS, lastFetchedAt is the MAX, enabled is ANY.
//
// ⚠ The count is honest arithmetic over what the children claim and never invents attribution.
// Nothing records which SOURCE a downloaded clip came from today, so children legitimately report
// 0 — and the group must report 0 too, rather than a plausible-looking total.
func TestSourceGroups_RollsUpCountLastFetchedAndEnabled(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	older := time.Unix(1_700_000_000, 0).UTC()
	newer := time.Unix(1_800_000_000, 0).UTC()
	addArchive(t, st, "classic", "Classic TV", older)
	addArchive(t, st, "vhs", "VHS Vault", newer)

	// One child switched off: the provider still reads as doing work, because the other is on.
	res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic", `{"enabled":false}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH child → %d, want 200", res.StatusCode)
	}

	rows := sourcesFrom(t, srv)
	g := rows[indexOf(rows, "provider:archive")]
	if !g.Enabled {
		t.Error("group reads as off while a child is still on — `enabled` is ANY, not ALL")
	}
	if g.LastFetchedAt != newer.Format(time.RFC3339) {
		t.Errorf("lastFetchedAt = %q, want the MAX over children (%q)", g.LastFetchedAt, newer.Format(time.RFC3339))
	}

	// And with every child off, the provider reads as dormant rather than as running.
	if r := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/vhs", `{"enabled":false}`, adminToken); r.StatusCode != http.StatusOK {
		t.Fatalf("PATCH second child → %d", r.StatusCode)
	}
	if g := sourcesFrom(t, srv)[indexOf(sourcesFrom(t, srv), "provider:archive")]; g.Enabled {
		t.Error("group reads as on with every child off")
	}
}

// ⚠ **A group id is not addressable for writes.** One PREFIX guard covers every provider that
// exists now and every one added later — a case-per-provider is a list that drifts.
func TestSourceGroups_WritesToAGroupAreRefused(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	addArchive(t, st, "classic", "Classic TV", time.Time{})

	for _, tc := range []struct {
		name, method, body string
	}{
		{"switch", http.MethodPatch, `{"enabled":false}`},
		{"delete", http.MethodDelete, ""},
	} {
		res := sourceReq(t, tc.method, srv.URL+"/v1/filler/sources/provider:archive", tc.body, adminToken)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s a group → %d, want 409", tc.name, res.StatusCode)
		}
	}

	// ⚠ And the child is untouched by the refused write — a 409 that half-applied would be worse
	// than one that failed loudly.
	got := registeredSources(t, st)
	if len(got) != 1 || !got[0].Enabled {
		t.Errorf("registered = %+v, want the child still registered and still on", got)
	}
}

// ⚠ The seeded blank-URI `youtube` row (migration 00034) becomes the provider's EMPTY STATE rather
// than a peer. It was always shaped like a provider root — no URI, nothing to fetch — and rendered
// as a row with a blank target; V51c is what makes skipping it correct rather than a loss.
func TestSourceGroups_SeededBlankProviderRowIsNotAPeer(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	blank := store.NewFillerSource("yt-seed", "youtube", "", "", time.Unix(1_700_000_000, 0).UTC())
	if err := st.UpsertFillerSource(context.Background(), blank); err != nil {
		t.Fatal(err)
	}

	rows := sourcesFrom(t, srv)
	if i := indexOf(rows, "yt-seed"); i >= 0 {
		t.Errorf("the blank-URI seeded row is listed as a peer at %d — one source would appear twice", i)
	}
	yt := indexOf(rows, "provider:youtube")
	if yt < 0 {
		t.Fatal("the YouTube provider node is missing")
	}
	if rows[yt].Configured {
		t.Error("the YouTube provider reads as configured on the strength of a blank seeded row")
	}
}

// ⚠ **`folder` and `library` deliberately do NOT group.** A twirl-down exists because ONE SERVICE
// offers many targets; two watched folders are unrelated directories with no service in common, so
// a "Folders" container would be a row that dims and changes nothing — §10's forbidden shape.
func TestSourceGroups_FoldersDoNotRollUp(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	extra := store.NewFillerSource("extra", "folder", "/mnt/more-ads", "More ads", time.Unix(1_700_000_000, 0).UTC())
	if err := st.UpsertFillerSource(context.Background(), extra); err != nil {
		t.Fatal(err)
	}

	rows := sourcesFrom(t, srv)
	for _, r := range rows {
		if r.Group && r.Kind == "folder" {
			t.Errorf("a folder group node was emitted (%s) — folders share no service", r.ID)
		}
	}
	i := indexOf(rows, "extra")
	if i < 0 {
		t.Fatal("the added folder is missing from the list")
	}
	if rows[i].ParentID != "" {
		t.Errorf("added folder parentId = %q, want top-level", rows[i].ParentID)
	}
}
