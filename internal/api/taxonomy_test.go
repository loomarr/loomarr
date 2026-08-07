package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// The taxonomy CRUD API (§10 V45a): read the graph, and let an ADMIN edit it — every write reindexing
// the derived clip rollups. The auth negatives (§19) are the load-bearing part: reads are member-open,
// writes are admin-only.

func TestTaxonomy_ListIsSeededAndMemberReadable(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	// A member (not admin) may READ the vocabulary — the catalog UI shows tags.
	resp := do(t, srv, http.MethodGet, "/v1/taxonomy", memberToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member GET /v1/taxonomy → %d, want 200 (reads are member-open)", resp.StatusCode)
	}
	var body struct {
		Taxa []struct {
			Slug string `json:"slug"`
			Axis string `json:"axis"`
		} `json:"taxa"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Taxa) < 40 {
		t.Fatalf("taxonomy not seeded: %d taxa, want the default forest (~55)", len(body.Taxa))
	}
	var sawBeer bool
	for _, tx := range body.Taxa {
		if tx.Slug == "beer" && tx.Axis == "product" {
			sawBeer = true
		}
	}
	if !sawBeer {
		t.Error("seeded vocabulary missing the product leaf 'beer'")
	}
}

// ⚠ §19 auth negatives: EVERY taxonomy write is admin-only. A member must be refused on create,
// update, and delete — the tag vocabulary is an operator concern.
func TestTaxonomy_WritesRequireAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/v1/taxonomy", `{"slug":"energy-drink","label":"Energy drink","axis":"product","parent":"drinks"}`},
		{http.MethodPut, "/v1/taxonomy/beer", `{"label":"Beer","axis":"product","parent":"alcohol"}`},
		{http.MethodDelete, "/v1/taxonomy/beer", ""},
	}
	for _, c := range cases {
		resp := do(t, srv, c.method, c.path, memberToken, c.body)
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s %s as member → 200, want a refusal (writes are admin-only)", c.method, c.path)
		}
	}
}

// Create a taxon, then confirm it is queryable AND that a clip tagged under it rolls up through the
// new node — i.e. the create REINDEXED. This is the whole point of the write path.
func TestTaxonomy_CreateReindexesRollups(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()

	// A clip tagged `energy-drink` before the taxon exists cannot be tagged — so create it first.
	resp := do(t, srv, http.MethodPost, "/v1/taxonomy", adminToken,
		`{"slug":"energy-drink","label":"Energy drink","axis":"product","parent":"drinks"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin create taxon → %d, want 200", resp.StatusCode)
	}

	// Seed a real clip and tag it `energy-drink` via the clip PATCH — grounding must now accept it.
	seedClip(t, st, "e1", filler.Commercial, 1999, filler.General, "")
	patch := do(t, srv, http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"e1","tags":["energy-drink"]}`)
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("tag clip energy-drink → %d, want 200 (the taxon now exists)", patch.StatusCode)
	}
	// The clip's rollups must include the ancestors of the NEW node: energy-drink → drinks.
	got, err := st.GetClip(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasTag(got.Tags, "energy-drink") || !hasTag(got.Tags, "drinks") {
		t.Errorf("clip tags = %v, want to roll up energy-drink → drinks", got.Tags)
	}
}

// ⚠ A parent that does not exist is rejected (an orphaned taxon would emit a dangling rollup).
func TestTaxonomy_CreateRejectsUnknownParent(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/taxonomy", adminToken,
		`{"slug":"widget","label":"Widget","axis":"product","parent":"does-not-exist"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("create with unknown parent → %d, want 422", resp.StatusCode)
	}
}

// ⚠ Tagging a clip with an off-vocabulary slug is REJECTED (the grounding gate at the API boundary):
// the taxonomy is the one vocabulary, and a slug not in it is never silently persisted.
func TestTaxonomy_ClipPatchRejectsUnknownTag(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "u1", filler.Commercial, 1990, filler.General, "")
	resp := do(t, srv, http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"u1","tags":["cryptocurrency"]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("patch with off-vocabulary tag → %d, want 422 (grounding gate)", resp.StatusCode)
	}
}

// Delete a MIDDLE taxon: its children reparent to the grandparent (not orphaned), and a clip tagged
// under a survivor rolls up correctly against the shrunk graph. Exercises delete + reindex + reparent.
func TestTaxonomy_DeleteReparentsAndReindexes(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()

	// Insert energy-drink under drinks, tag a clip, then delete drinks' child chain is not needed —
	// delete `alcohol` (a real middle node: beer → alcohol → drinks) and confirm beer reparents to drinks.
	seedClip(t, st, "b1", filler.Commercial, 1994, filler.General, "")
	if p := do(t, srv, http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"b1","tags":["beer"]}`); p.StatusCode != http.StatusOK {
		t.Fatalf("tag beer → %d", p.StatusCode)
	}
	// Before delete: beer rolls up through alcohol.
	got, _ := st.GetClip(ctx, "b1")
	if !hasTag(got.Tags, "alcohol") {
		t.Fatalf("precondition: beer should roll up to alcohol, got %v", got.Tags)
	}
	// Delete the middle taxon.
	if d := do(t, srv, http.MethodDelete, "/v1/taxonomy/alcohol", adminToken, ""); d.StatusCode != http.StatusNoContent {
		t.Fatalf("delete alcohol → %d, want 204", d.StatusCode)
	}
	// After delete+reindex: beer's asserted leaf survives, alcohol is GONE (reparented away, not a
	// phantom), and beer now rolls up straight to drinks (the grandparent).
	got, _ = st.GetClip(ctx, "b1")
	if hasTag(got.Tags, "alcohol") {
		t.Errorf("clip still rolls up to deleted 'alcohol' after reindex: %v", got.Tags)
	}
	if !hasTag(got.Tags, "beer") || !hasTag(got.Tags, "drinks") {
		t.Errorf("clip tags = %v, want beer → drinks after reparent", got.Tags)
	}
}

// Deleting a taxon that does not exist is a 404.
func TestTaxonomy_DeleteMissingIs404(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	resp := do(t, srv, http.MethodDelete, "/v1/taxonomy/no-such-taxon", adminToken, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing taxon → %d, want 404", resp.StatusCode)
	}
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
