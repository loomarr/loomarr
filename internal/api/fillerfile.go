package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Filing decisions — the Incoming tab's verbs (§10 V38).
//
// A held clip is recorded but not in the playable catalog. These two routes are the human half of
// the lifecycle: file (admit it) and hold (send it back). The machine half is the tag job's
// auto-file, which uses the same store writer.
//
// ⚠ **`file-as-suggested` is a THIRD verb, not a flag on the first**, because it answers a
// different question. Filing takes a clip as it stands; filing-as-suggested first CONFIRMS the
// tagger's ungrounded era guess — committing each clip's OWN `suggestedEra` rather than one value
// the operator picked. `bulk/tag` cannot express that: it applies a single era to the whole
// selection, which is the opposite of "each of these, as proposed".

type fileFillerInput struct {
	Body struct {
		Paths []string `json:"paths" minItems:"1" doc:"Clip identities (paths relative to FILLER_DIR)"`
		// AsSuggested confirms each clip's OWN ungrounded era guess as it files (§10 V34/V38).
		//
		// ⚠ Per clip, never one value across the selection. The mock's "File all as suggested" is
		// exactly this: N clips, N different proposed eras, one action.
		AsSuggested bool `json:"asSuggested,omitempty" doc:"Confirm each clip's own suggested era as it files"`
	}
}

type holdFillerInput struct {
	Body struct {
		Paths []string `json:"paths" minItems:"1" doc:"Clip identities (paths relative to FILLER_DIR)"`
	}
}

func (s *Server) registerFillerFile(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "file-filler-clips", Method: http.MethodPost, Path: "/v1/filler/file",
		Summary: "File held clips into the catalog",
		Description: "Admin only (§10 V38). Admits held clips to the playable catalog — they become matchable " +
			"into pods. With `asSuggested`, each clip's OWN ungrounded era guess is confirmed as it files " +
			"(the mock's \"File all as suggested\"), which `bulk/tag` cannot express because it applies one " +
			"era to the whole selection. Filing by hand CLEARS the auto-filed marker: a human looked.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.fileFillerClips)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "hold-filler-clips", Method: http.MethodPost, Path: "/v1/filler/hold",
		Summary: "Send clips back to the review queue",
		Description: "Admin only (§10 V38). The undo for auto-filing: a clip Loomarr filed without asking goes " +
			"back to Incoming and stops being matched into pods. ⚠ It does NOT remove the clip or touch the " +
			"file — that is \"Remove from catalog\", a different action with a different promise.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.holdFillerClips)
}

func (s *Server) fileFillerClips(ctx context.Context, in *fileFillerInput) (*bulkResultOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	now := time.Now().UTC()

	// Confirming the suggestions happens BEFORE filing, so a failure leaves the clips held with
	// their guesses intact rather than filed with unconfirmed tags. The safe half-state is "still
	// waiting", never "in the catalog claiming something nobody agreed to".
	if in.Body.AsSuggested {
		for _, path := range in.Body.Paths {
			c, err := s.store.GetClip(ctx, path)
			if err != nil {
				// A stale selection races a re-scan; skipping one row beats failing the batch,
				// the same call bulk/tag makes.
				continue
			}
			if c.SuggestedEra == 0 {
				continue
			}
			// ⚠ Writing `era` is what CONFIRMS: the store clears `suggested_era` in the same
			// statement (see UpdateClipTags), so the question cannot outlive its answer. The other
			// two tags are passed through unchanged because that write sets all three columns —
			// sending only the era would blank audience and category.
			if err := s.store.UpdateClipTags(ctx, path, c.SuggestedEra, string(c.Audience), c.Category, 0, c.AITagged, now); err != nil {
				return nil, huma.Error500InternalServerError("confirm suggested era", err)
			}
		}
	}

	// ⚠ `autoFiled: false`. A human filed these, so the marker that says "nobody looked" must be
	// cleared — it is the only thing that can answer "which of these did I never see?", and
	// leaving it set would make a reviewed clip indistinguishable from an unreviewed one.
	updated, err := s.store.SetClipsHeld(ctx, in.Body.Paths, false, false, now)
	if err != nil {
		return nil, huma.Error500InternalServerError("file clips", err)
	}
	out := &bulkResultOutput{}
	out.Body.Updated = updated
	out.Body.Missing = len(in.Body.Paths) - updated
	return out, nil
}

func (s *Server) holdFillerClips(ctx context.Context, in *holdFillerInput) (*bulkResultOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	// ⚠ `autoFiled: false` here too, and for a subtler reason than above: a clip sent back is no
	// longer filed at all, so a marker saying "filed without a human" would describe a state it
	// is not in. Re-filing it later re-decides that flag from scratch.
	updated, err := s.store.SetClipsHeld(ctx, in.Body.Paths, true, false, time.Now().UTC())
	if err != nil {
		return nil, huma.Error500InternalServerError("hold clips", err)
	}
	out := &bulkResultOutput{}
	out.Body.Updated = updated
	out.Body.Missing = len(in.Body.Paths) - updated
	return out, nil
}
