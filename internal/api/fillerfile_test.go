package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// Filing admits a held clip to the catalog — where "in the catalog" means matchable into a pod,
// which is what the default ListClips filter answers.
func TestFileFillerClips_AdmitsAHeldClipToTheCatalog(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Path: "held.mp4", Name: "held.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1990, Audience: filler.Kids, Category: "toys", Held: true,
	})

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/file", `{"paths":["held.mp4"]}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got, err := st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "held.mp4" {
		t.Fatalf("catalog = %+v, want the filed clip", got)
	}
	if got[0].Held {
		t.Error("still held after filing")
	}
}

// ⚠ Filing BY HAND clears the auto-filed marker. That flag answers "which of these did I never
// see?" — leaving it set on a clip a human just reviewed makes a reviewed clip indistinguishable
// from an unreviewed one, which is the one question the flag exists for.
func TestFileFillerClips_ClearsTheAutoFiledMarkerBecauseAHumanLooked(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Path: "auto.mp4", Name: "auto.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1990, Audience: filler.Kids, Category: "toys", Held: true, AutoFiled: true,
	})

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/file",
		`{"paths":["auto.mp4"]}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	c, err := st.GetClip(ctx, clipHashFor("auto.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if c.AutoFiled {
		t.Error("auto_filed survived a HUMAN filing the clip — a reviewed clip now looks unreviewed")
	}
}

// ⚠ "File all as suggested" commits each clip's OWN proposed era. `bulk/tag` cannot express this:
// it applies one operator-chosen era to the whole selection, which for a queue of clips with
// DIFFERENT guesses is the wrong answer for all but one of them.
func TestFileFillerClips_AsSuggestedConfirmsEachClipsOwnEra(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Path: "a.mp4", Name: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Audience: filler.Kids, Category: "toys", SuggestedEra: 1985, Held: true,
	})
	putClip(t, st, filler.Clip{
		Path: "b.mp4", Name: "b.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Audience: filler.Kids, Category: "cereal", SuggestedEra: 1992, Held: true,
	})

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/file",
		`{"paths":["a.mp4","b.mp4"],"asSuggested":true}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	for path, wantEra := range map[string]int{"a.mp4": 1985, "b.mp4": 1992} {
		// ⚠ By HASH. The request above sends PATHS (that is the route's contract), but the store
		// is keyed by hash — the exact split this test's own sibling at line 107 records as a
		// shipped bug. Passing `path` here read as correct only because the fixture used to set
		// hash = path.
		c, err := st.GetClip(ctx, clipHashFor(path))
		if err != nil {
			t.Fatal(err)
		}
		if c.Era != wantEra {
			t.Errorf("%s era = %d, want %d — each clip's OWN suggestion", path, c.Era, wantEra)
		}
		// Confirming answers the question, so the question must not outlive it.
		if c.SuggestedEra != 0 {
			t.Errorf("%s still carries suggestedEra %d after confirmation", path, c.SuggestedEra)
		}
		// ⚠ The other classification facts survive. Taxonomy and category are owned by their own
		// transaction, so confirming an era cannot overwrite them.
		if c.Audience == "" || c.Category == "" {
			t.Errorf("%s lost its other tags: audience=%q category=%q", path, c.Audience, c.Category)
		}
	}
}

// ⚠ **The same claim as the test above, made with identity and location as DIFFERENT strings.**
//
// `putClip` defaults `Hash = Path` so fixtures can name clips readably, which means every other
// test in this package is blind to a route confusing the two — and this one did. `asSuggested`
// looked each clip up with `GetClip(ctx, path)`, but `GetClip` is `WHERE hash = ?` (V38c split
// identity from location and added `GetClipByPath` for exactly this). In production the lookup
// missed, hit the `continue`, and **every suggested era was silently dropped**: "File all as
// suggested" filed the clips and applied none of the suggestions it is named after.
//
// The fixture is the whole point of the test. Give a clip a content-hash-shaped id that is not
// its path and the bug is visible; equate them and it is not.
func TestFileFillerClips_AsSuggestedResolvesByPathNotIdentity(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	const (
		hash = "sha256:9f2c1e" // a content hash, as V38c made it
		path = "2026/03/promo.mp4"
	)
	putClip(t, st, filler.Clip{
		Hash: hash, Path: path, Name: "promo.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Audience: filler.Kids, Category: "toys", SuggestedEra: 1985, Held: true,
	})

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/file",
		`{"paths":["`+path+`"],"asSuggested":true}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	c, err := st.GetClip(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if c.Era != 1985 {
		t.Errorf("era = %d, want 1985 — asSuggested filed the clip but dropped its suggestion", c.Era)
	}
	if c.SuggestedEra != 0 {
		t.Errorf("still carries suggestedEra %d after confirmation", c.SuggestedEra)
	}
	// The other classification facts and taxonomy shadow survive confirmation.
	if c.Audience == "" || c.Category == "" {
		t.Errorf("lost its other tags: audience=%q category=%q", c.Audience, c.Category)
	}
}

// The undo for auto-filing: back to the queue, and OUT of pod matching.
func TestHoldFillerClips_SendsAnAutoFiledClipBackAndOutOfMatching(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	putClip(t, st, filler.Clip{
		Path: "auto.mp4", Name: "auto.mp4", Kind: filler.Commercial, DurationMs: 30_000,
		Era: 1990, Audience: filler.Kids, Category: "toys", AutoFiled: true,
	})

	if res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/hold",
		`{"paths":["auto.mp4"]}`, adminToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	// Gone from the catalog read pod assembly uses...
	catalog, err := st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 0 {
		t.Errorf("a held-back clip is still in the catalog (%+v) — it would keep airing", catalog)
	}
	// ...but NOT deleted. Holding is not removing: the file and the row both stay.
	c, err := st.GetClip(ctx, clipHashFor("auto.mp4"))
	if err != nil {
		t.Fatalf("holding DELETED the clip: %v — that is 'Remove from catalog', a different promise", err)
	}
	if !c.Held {
		t.Error("not held")
	}
}

// §19 negatives: filing changes what plays on a channel.
func TestFileAndHoldRoutes_RequireAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)
	for _, path := range []string{"/v1/filler/file", "/v1/filler/hold"} {
		res := sourceReq(t, http.MethodPost, srv.URL+path, `{"paths":["x.mp4"]}`, "")
		if res.StatusCode == http.StatusOK {
			t.Errorf("%s succeeded with no credential", path)
		}
	}
}
