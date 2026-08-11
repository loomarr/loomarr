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
	if in.Body.Restore {
		s.clearPipelineRejects(ctx, in.Body.Hashes)
	} else {
		// ⚠ **"Don't use it" has to move the pipeline row, not just the tombstone** (§10 V54).
		// `SetClipsRemoved` writes `removed_at` and `GetClip` carries no `removed_at` predicate, so
		// the Incoming belt's fallback loop re-resolved a dismissed clip and put it straight back on
		// the queue. Settling the row is what actually takes it off the belt: `dismissed` is not in
		// `ConveyorOnly`'s `running|review` set, so the row is never fetched to be re-resolved.
		//
		// From `review` OR `filed`: this route also serves the Catalog tab's bulk bar, where the
		// clip being removed was filed long ago.
		s.settlePipeline(ctx, in.Body.Hashes, filler.DispositionDismissed,
			filler.DispositionReview, filler.DispositionFiled)
	}
	out := &bulkResultOutput{}
	out.Body.Updated = n
	// Missing = requested hashes that did not resolve OR did not match a removable row.
	out.Body.Missing = len(in.Body.Hashes) - n
	return out, nil
}

// clearPipelineRejects turns a restored clip's pipeline row from `rejected` back to `review`
// (§10 V51b) — the other half of an undo.
//
// ⚠ **Restore is ONE endpoint, not two.** The obvious alternative was a dedicated
// `POST /v1/filler/clips/{hash}/restore` for pipeline rejects beside this one for catalog
// removals. They would then be two ways to un-remove a clip that could disagree — one clearing
// the tombstone, one clearing the reason — and an operator hitting the wrong one would see the
// clip return to the catalog while Incoming still called it rejected, or the reverse. Two places,
// one truth: `removed_at` is WHETHER, the pipeline row is WHY, and undoing has to move both.
//
// ⚠ **It does NOT re-run the pipeline**, and that is the point of a restore rather than a rewind.
// The same rule that refused the clip would refuse it again in seconds, so re-running would make
// the button a two-second round trip to nowhere. `review` is the honest destination: the machine
// has done everything it can and is waiting on a person, which is exactly the state a restored
// clip is in. An operator who genuinely wants it re-examined asks for that separately.
//
// Best-effort: the clip IS back in the catalog by this point (the tombstone is cleared and
// airability is what matters), so failing the request over the bookkeeping half would report a
// failure for an undo that mostly worked. A stale `rejected` row shows a wrong reason on the audit
// list until the next write — visible and harmless, unlike a clip that is silently still hidden.
// ⚠ It clears an operator DISMISSAL too (§10 V54), not only a machine rejection. Both are terminal
// refusals with the same undo, and restore is one endpoint by the argument above — a restore that
// un-removed a dismissed clip from the catalog while leaving Incoming calling it dismissed would be
// the exact disagreement that argument exists to prevent.
func (s *Server) clearPipelineRejects(ctx context.Context, hashes []string) {
	for _, hash := range hashes {
		row, found, err := s.store.GetClipPipeline(ctx, hash)
		if err != nil || !found {
			continue
		}
		if row.Disposition != filler.DispositionRejected && row.Disposition != filler.DispositionDismissed {
			continue
		}
		row.Disposition = filler.DispositionReview
		row.RejectReason, row.RejectDetail = "", ""
		row.UpdatedAt = time.Now().UTC()
		if err := s.store.UpsertClipPipeline(ctx, row); err != nil {
			s.log.Warn("could not clear a restored clip's rejection", "clip", hash, "err", err)
		}
	}
}

// settlePipeline moves each clip's pipeline row to the terminal disposition the OPERATOR chose
// (§10 V54) — the writer `review → filed | dismissed` never had.
//
// ⚠ **The whole class of defect this fixes:** `filed` and `rejected` were only ever written by
// `filler.Pipeline` itself, so every operator path moved `clips` and left the pipeline row alone.
// The row kept saying `review`, so the clip kept saying it needed a decision. Three of the four
// decision buttons therefore did not stick.
//
// ⚠ **Guarded on the row's CURRENT disposition, with the allowed origins passed explicitly.** A
// clip the pipeline is still working (`running`) is not settled by an operator verb: it finishes
// its ladder and settles itself, which is what keeps "the operator filed it early" from abandoning
// the transcribe and tag rungs. An unlisted origin is skipped, never coerced.
//
// Best-effort, matching `clearPipelineRejects`: the catalog half has already landed by the time
// this runs and `removed_at`/`held` are what decide airability, so failing the request over the
// bookkeeping half would report a failure for a decision that took effect.
func (s *Server) settlePipeline(ctx context.Context, hashes []string, to filler.Disposition, from ...filler.Disposition) {
	for _, hash := range hashes {
		row, found, err := s.store.GetClipPipeline(ctx, hash)
		if err != nil || !found {
			continue
		}
		allowed := false
		for _, d := range from {
			if row.Disposition == d {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		row.Disposition = to
		if to.Terminal() {
			// Zero the schedule so the work list cannot re-pick a settled row on a clock skew —
			// the same rule `Pipeline.save` applies to the rows it settles itself.
			row.NextRun = time.Time{}
		}
		row.UpdatedAt = time.Now().UTC()
		if err := s.store.UpsertClipPipeline(ctx, row); err != nil {
			s.log.Warn("could not record the operator's decision", "clip", hash, "to", string(to), "err", err)
		}
	}
}
