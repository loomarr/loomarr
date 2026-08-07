package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The Catalog tab's bulk bar (§10 V35): retag a selection, or remove it from the catalog.
//
// ⚠ **"Remove from catalog" is a TOMBSTONE, not a delete, and not a file operation.** `clips` is a
// synced cache of `FILLER_DIR` (migration `00013`), so deleting the row would let the next scan
// find the file and put it straight back — the operator removes a clip, watches it reappear
// fifteen minutes later, and concludes the button is broken. And deleting the FILE is not on the
// table: nothing in Loomarr deletes an operator's media (disabling a source keeps its clips;
// deleting a source keeps its clips). The action says remove from the *catalog*, and that is
// exactly what it does.

// bulkTagFillerInput retags a selection in one request.
type bulkTagFillerInput struct {
	Body struct {
		Hashes []string `json:"hashes" minItems:"1" doc:"Clip identities (content hashes, §10 V45a)"`
		// Each field is optional: the bulk bar has three independent dropdowns, and an
		// operator setting only the audience must not blank the other two.
		Era      *int    `json:"era,omitempty" doc:"Set the era on every selected clip. Omit to leave it alone."`
		Audience *string `json:"audience,omitempty" enum:"kids,family,general,late_night" doc:"Omit to leave it alone."`
		// Tags REPLACES the old flat category (§10 V45a): the taxonomy tag set applied to every
		// selected clip. Grounded against the live vocabulary (an unknown slug 422s the whole batch);
		// each clip's `category` shadow is derived. Omit to leave tags alone; send [] to clear them.
		Tags *[]string `json:"tags,omitempty" doc:"Taxonomy tags to set on every selected clip. Grounded on write; category derived. Omit to leave alone (§10 V45a)."`
	}
}

type bulkResultOutput struct {
	Body struct {
		Updated int `json:"updated" doc:"How many clips actually changed"`
		// Missing counts selected clips that no longer exist. Reported rather than failing the
		// batch: a selection made minutes ago races a re-scan, and refusing the whole request
		// for one stale row would be worse than applying the rest.
		Missing int `json:"missing"`
	}
}

func (s *Server) registerFillerBulk(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "bulk-tag-filler", Method: http.MethodPost, Path: "/v1/filler/bulk/tag",
		Summary: "Retag a selection of clips",
		Description: "Admin only (§10 V35) — the Catalog tab's bulk bar. Each tag field is INDEPENDENT: omitting " +
			"one leaves it alone, so setting only the audience never blanks the era. Selected clips that no longer " +
			"exist are counted as `missing` rather than failing the batch, because a selection races a re-scan.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.bulkTagFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "bulk-remove-filler", Method: http.MethodPost, Path: "/v1/filler/bulk/remove",
		Summary: "Remove a selection from the catalog",
		Description: "Admin only (§10 V35). ⚠ A TOMBSTONE, not a delete: the clip stops appearing in the catalog " +
			"and stops being used in commercial breaks, and **the file is left exactly where it is**. Nothing in " +
			"Loomarr deletes an operator's media. It survives re-scans, which a row delete could not — the next " +
			"scan would find the file and put it back. Pass `restore:true` to undo.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.bulkRemoveFiller)
}

func (s *Server) bulkTagFiller(ctx context.Context, in *bulkTagFillerInput) (*bulkResultOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}

	out := &bulkResultOutput{}
	now := time.Now().UTC()

	// Ground the requested tag set ONCE against the live taxonomy (§10 V45a) — an unknown slug 422s
	// the whole batch before any clip is touched (all-or-nothing on a bad request, like the single
	// PATCH). `leaves` is the grounded, canonical, de-duplicated set applied to every selected clip;
	// nil `Tags` means "leave each clip's tags alone".
	var leaves []string
	var forest *taxonomy.Forest
	if in.Body.Tags != nil {
		taxa, err := s.store.ListTaxa(ctx)
		if err != nil {
			return nil, err
		}
		forest = taxonomy.New(taxa)
		seen := map[string]bool{}
		for _, raw := range *in.Body.Tags {
			slug, ok := forest.Resolve(raw)
			if !ok {
				return nil, errUnprocessable("Unknown tag",
					fmt.Sprintf("The tag %q is not in the taxonomy vocabulary. Add it under Filler → Taxonomy first, or choose an existing tag.", raw))
			}
			if !seen[slug] {
				seen[slug] = true
				leaves = append(leaves, slug)
			}
		}
		sort.Strings(leaves)
	}

	for _, hash := range in.Body.Hashes {
		// The selection carries HASHES (§10 V45a) — the wire identity. GetClip is hash-keyed, so a
		// stale hash (a clip removed by a re-sync between selection and apply) counts as missing.
		clip, err := s.store.GetClip(ctx, hash)
		if err != nil {
			out.Body.Missing++
			continue
		}
		era, audience, category := clip.Era, clip.Audience, clip.Category
		if in.Body.Era != nil {
			era = *in.Body.Era
		}
		if in.Body.Audience != nil {
			audience = filler.Audience(*in.Body.Audience)
		}
		// Tags: persist the grounded set and DERIVE category from it (never take category from the
		// client). Only when Tags was sent — otherwise the clip's existing tags and category shadow
		// are left untouched.
		if in.Body.Tags != nil {
			if err := s.store.SetClipTags(ctx, clip.Hash, leaves, forest, now); err != nil {
				return nil, huma.Error500InternalServerError("retag clips", err)
			}
			category = forest.PrimaryProductLeaf(leaves)
		}

		// ⚠ Setting an era CONFIRMS an outstanding suggestion, and the existing single-clip
		// path already encodes that (§10 V34: writing an era clears the suggestion). Routed
		// through the same store method so a bulk edit and a single edit cannot disagree about
		// what confirming means — a second rule here is how the grounding invariant rots.
		suggested := clip.SuggestedEra
		if in.Body.Era != nil {
			suggested = 0
		}
		// aiTagged=false: a human just made this decision, so it is no longer an AI tag. Hash-keyed.
		if err := s.store.UpdateClipTags(ctx, clip.Hash, era, string(audience), category, suggested, false, now); err != nil {
			return nil, huma.Error500InternalServerError("retag clips", err)
		}
		out.Body.Updated++
	}
	return out, nil
}

type bulkRemoveFillerInput struct {
	Body struct {
		Hashes  []string `json:"hashes" minItems:"1" doc:"Clip identities (content hashes, §10 V45a)"`
		Restore bool     `json:"restore,omitempty" doc:"Put the clips back in the catalog instead of removing them"`
	}
}

func (s *Server) bulkRemoveFiller(ctx context.Context, in *bulkRemoveFillerInput) (*bulkResultOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}

	// ⚠ The selection carries HASHES (§10 V45a), but SetClipsRemoved is PATH-keyed by design (V38 —
	// the tombstone tracks the file on disk by location). Resolve hash → path at the boundary, so the
	// store keeps its path key and the wire keeps its hash identity. A hash that no longer resolves (a
	// clip removed by a re-sync between selection and apply) is dropped and reported as missing.
	paths := make([]string, 0, len(in.Body.Hashes))
	for _, hash := range in.Body.Hashes {
		clip, err := s.store.GetClip(ctx, hash)
		if err != nil {
			continue // stale hash — counted as missing below
		}
		paths = append(paths, clip.Path)
	}

	// The zero time is the restore value: `removed_at = 0` is what "present" means, so undo
	// needs no second method and cannot drift from the removal path.
	at := time.Now().UTC()
	if in.Body.Restore {
		at = time.Time{}
	}
	n, err := s.store.SetClipsRemoved(ctx, paths, at)
	if err != nil {
		return nil, huma.Error500InternalServerError("remove clips from the catalog", err)
	}
	out := &bulkResultOutput{}
	out.Body.Updated = n
	// Missing = requested hashes that did not resolve OR did not match a removable row.
	out.Body.Missing = len(in.Body.Hashes) - n
	return out, nil
}
