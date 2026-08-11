package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

type bulkBody struct {
	Updated int `json:"updated"`
	Missing int `json:"missing"`
}

func postBulk(t *testing.T, srv, path, body, token string) (*http.Response, bulkBody) {
	t.Helper()
	res := sourceReq(t, http.MethodPost, srv+path, body, token)
	var b bulkBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
			t.Fatal(err)
		}
	}
	return res, b
}

func listPaths(t *testing.T, st store.Store, f store.ClipFilter) []string {
	t.Helper()
	clips, err := st.ListClips(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(clips))
	for _, c := range clips {
		out = append(out, c.Path)
	}
	return out
}

// The bulk bar has three INDEPENDENT dropdowns. Setting only the audience must not blank the era
// an operator (or the tagger) already established.
func TestBulkTagFiller_OmittedFieldsAreLeftAlone(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1992, Audience: filler.General, Category: "cars",
	})

	res, body := postBulk(t, srv.URL, "/v1/filler/bulk/tag",
		`{"hashes":["`+clipHashFor("a.mp4")+`"],"audience":"kids"}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body.Updated != 1 {
		t.Errorf("updated = %d, want 1", body.Updated)
	}

	got, err := st.GetClip(context.Background(), clipHashFor("a.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Audience != filler.Kids {
		t.Errorf("audience = %q, want kids", got.Audience)
	}
	if got.Era != 1992 || got.Category != "cars" {
		t.Errorf("era/category = %d/%q — an omitted field was blanked", got.Era, got.Category)
	}
}

// ⚠ Setting an era CONFIRMS an outstanding suggestion (§10 V34). Bulk and single-clip editing
// must agree about what confirming means, or the grounding invariant has two rules.
func TestBulkTagFiller_SettingAnEraConfirmsTheSuggestion(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{
		Path: "guess.mp4", Name: "guess.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Audience: filler.Kids, Category: "toys", SuggestedEra: 1988,
	})

	postBulk(t, srv.URL, "/v1/filler/bulk/tag",
		`{"hashes":["`+clipHashFor("guess.mp4")+`"],"era":1988}`, adminToken)

	got, err := st.GetClip(context.Background(), clipHashFor("guess.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Era != 1988 {
		t.Errorf("era = %d, want 1988", got.Era)
	}
	if got.SuggestedEra != 0 {
		t.Errorf("suggestedEra = %d, want 0 — writing an era must clear the suggestion", got.SuggestedEra)
	}
}

// A selection made minutes ago races a re-scan. Failing the whole batch for one stale row would
// be worse than applying the rest, so a missing clip is counted rather than fatal.
func TestBulkTagFiller_CountsMissingRatherThanFailing(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000})

	_, body := postBulk(t, srv.URL, "/v1/filler/bulk/tag",
		`{"hashes":["`+clipHashFor("a.mp4")+`","`+clipHashFor("gone.mp4")+`"],"tags":["toys"]}`, adminToken)

	if body.Updated != 1 || body.Missing != 1 {
		t.Errorf("updated/missing = %d/%d, want 1/1", body.Updated, body.Missing)
	}
}

// Removing hides the clip from the catalog. The file is NOT touched — nothing in Loomarr deletes
// an operator's media — and the row survives so a restore can put it back.
func TestBulkRemoveFiller_HidesTheClipButKeepsTheRow(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000})
	putClip(t, st, filler.Clip{Path: "b.mp4", Name: "b.mp4", Kind: filler.Commercial, DurationMs: 30_000})

	res, body := postBulk(t, srv.URL, "/v1/filler/bulk/remove", `{"hashes":["`+clipHashFor("a.mp4")+`"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body.Updated != 1 {
		t.Errorf("updated = %d, want 1", body.Updated)
	}

	if got := listPaths(t, st, store.ClipFilter{}); len(got) != 1 || got[0] != "b.mp4" {
		t.Errorf("catalog = %v, want only b.mp4", got)
	}
	// The row is still there, which is what makes restore possible and what makes the
	// tombstone survive a re-scan.
	if got := listPaths(t, st, store.ClipFilter{IncludeRemoved: true}); len(got) != 2 {
		t.Errorf("with IncludeRemoved = %v, want both rows", got)
	}
}

// ⚠ THE property a row DELETE cannot give. `clips` is a synced cache, so the next scan finds the
// file still on disk and upserts it. If the tombstone rode along in that upsert it would reset to
// zero and the clip would silently reappear — the operator's removal quietly undone.
func TestBulkRemoveFiller_SurvivesAReScan(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000})
	postBulk(t, srv.URL, "/v1/filler/bulk/remove", `{"hashes":["`+clipHashFor("a.mp4")+`"]}`, adminToken)

	// Exactly what a scan does when it finds the file again: upsert the row from the
	// filesystem's view, which knows nothing about the tombstone.
	// ⚠ Carries the same HASH as the seeded row, or this upsert inserts a SECOND clip instead of
	// updating the tombstoned one — and the test would pass for the wrong reason (the tombstone
	// survives because nothing touched it). Identity is the hash since V38c.
	if err := st.UpsertClip(ctx, store.Clip{
		Clip:      filler.Clip{Hash: clipHashFor("a.mp4"), Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000},
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if got := listPaths(t, st, store.ClipFilter{}); len(got) != 0 {
		t.Errorf("catalog = %v after a re-scan — the removal was silently undone", got)
	}
}

// Restore is the same write with the zero time, so undo cannot drift from removal.
func TestBulkRemoveFiller_RestorePutsItBack(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000})
	postBulk(t, srv.URL, "/v1/filler/bulk/remove", `{"hashes":["`+clipHashFor("a.mp4")+`"]}`, adminToken)
	if got := listPaths(t, st, store.ClipFilter{}); len(got) != 0 {
		t.Fatalf("setup: clip not removed, catalog = %v", got)
	}

	postBulk(t, srv.URL, "/v1/filler/bulk/remove",
		`{"hashes":["`+clipHashFor("a.mp4")+`"],"restore":true}`, adminToken)

	if got := listPaths(t, st, store.ClipFilter{}); len(got) != 1 {
		t.Errorf("catalog = %v after restore, want the clip back", got)
	}
}

// §19 negatives: both routes edit the catalog.
func TestFillerBulkRoutes_RequireAdmin(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000})

	for _, path := range []string{"/v1/filler/bulk/tag", "/v1/filler/bulk/remove"} {
		body := `{"hashes":["` + clipHashFor("a.mp4") + `"],"tags":["toys"]}`
		if res, _ := postBulk(t, srv.URL, path, body, ""); res.StatusCode == http.StatusOK {
			t.Errorf("%s succeeded with no credential", path)
		}
	}
	if got := listPaths(t, st, store.ClipFilter{}); len(got) != 1 {
		t.Errorf("an unauthenticated caller changed the catalog: %v", got)
	}
}
