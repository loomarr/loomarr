package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/filler"
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
		Paths []string `json:"paths" minItems:"1" doc:"Clip identities (paths relative to FILLER_DIR)"`
		// Each field is optional: the bulk bar has three independent dropdowns, and an
		// operator setting only the audience must not blank the other two.
		Era      *int    `json:"era,omitempty" doc:"Set the era on every selected clip. Omit to leave it alone."`
		Audience *string `json:"audience,omitempty" enum:"kids,family,general,late_night" doc:"Omit to leave it alone."`
		Category *string `json:"category,omitempty" doc:"Omit to leave it alone."`
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
	for _, path := range in.Body.Paths {
		clip, err := s.store.GetClip(ctx, path)
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
		if in.Body.Category != nil {
			category = *in.Body.Category
		}

		// ⚠ Setting an era CONFIRMS an outstanding suggestion, and the existing single-clip
		// path already encodes that (§10 V34: writing an era clears the suggestion). Routed
		// through the same store method so a bulk edit and a single edit cannot disagree about
		// what confirming means — a second rule here is how the grounding invariant rots.
		suggested := clip.SuggestedEra
		if in.Body.Era != nil {
			suggested = 0
		}
		// aiTagged=false: a human just made this decision, so it is no longer an AI tag.
		if err := s.store.UpdateClipTags(ctx, path, era, string(audience), category, suggested, false, now); err != nil {
			return nil, huma.Error500InternalServerError("retag clips", err)
		}
		out.Body.Updated++
	}
	return out, nil
}

type bulkRemoveFillerInput struct {
	Body struct {
		Paths   []string `json:"paths" minItems:"1"`
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

	// The zero time is the restore value: `removed_at = 0` is what "present" means, so undo
	// needs no second method and cannot drift from the removal path.
	at := time.Now().UTC()
	if in.Body.Restore {
		at = time.Time{}
	}
	n, err := s.store.SetClipsRemoved(ctx, in.Body.Paths, at)
	if err != nil {
		return nil, huma.Error500InternalServerError("remove clips from the catalog", err)
	}
	out := &bulkResultOutput{}
	out.Body.Updated = n
	out.Body.Missing = len(in.Body.Paths) - n
	return out, nil
}
