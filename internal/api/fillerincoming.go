package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// The Incoming tab — the ingest conveyor (§10 V35).
//
// What has been downloaded but is not yet filed, in two shapes:
//
//   - **asks**: clips whose tags need a human. Today that means an AI-proposed era the tagger
//     could NOT ground in the clip's text (V34's rule: an era is persisted as fact only when the
//     year appears literally in the filename, sidecar or transcript), plus commercials with no
//     match tags at all.
//   - **reels**: compilations mid-split — the persisted split proposals V34 already writes.
//
// ⚠ **One read behind the tab, not a fan-out the client assembles.** The two halves answer one
// question ("what is waiting on me?"), and a client that fetched them separately would render a
// half-empty queue whenever one call was slower — which is exactly when the queue matters.
//
// ⚠ **Nothing here invents a confidence score.** The mock draws a per-ask confidence bar and a
// rationale; the tagger does not record either. `reason` is therefore derived from the real
// state (an ungrounded era, or no tags at all), and there is no numeric confidence field —
// putting a number in front of an operator that nothing measured is the failure the estimate
// field on a pull is also written to avoid. `filler.autofile.*` is the knob that WOULD need one;
// it is not built yet, and this DTO deliberately does not pretend otherwise.

// IncomingAskDTO is one clip waiting on a human decision about its tags.
type IncomingAskDTO struct {
	Path string `json:"path" doc:"Clip identity — the path relative to FILLER_DIR"`
	Name string `json:"name"`
	// From is where the clip came from, so an operator reviewing forty of them can tell which
	// source is producing junk.
	From       string `json:"from,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Thumbnail  string `json:"thumbnail,omitempty"`
	// Kind is what Loomarr already believes; Era/Audience/Category are its current tags.
	Kind     string `json:"kind"`
	Era      int    `json:"era,omitempty"`
	Audience string `json:"audience,omitempty"`
	Category string `json:"category,omitempty"`
	// SuggestedEra is an era the tagger proposed but could NOT ground in the clip's text
	// (§10 V34). Confirming it is `PATCH /v1/filler/{id}` setting `era`.
	SuggestedEra int `json:"suggestedEra,omitempty"`
	// Reason is why this clip is in the queue, in the operator's terms. Derived from real
	// state, never generated prose.
	Reason string `json:"reason"`
}

// IncomingReelDTO is one compilation mid-split.
type IncomingReelDTO struct {
	ProposalID string `json:"proposalId"`
	ClipPath   string `json:"clipPath"`
	Segments   int    `json:"segments" doc:"How many clips the detector found"`
	// NeedsAttention counts segments the operator cannot simply accept — an unsplittable
	// stretch, or one flagged as a duplicate of something already in the catalog.
	NeedsAttention int    `json:"needsAttention"`
	CreatedAt      string `json:"createdAt" doc:"RFC3339"`
}

type fillerIncomingOutput struct {
	Body struct {
		Asks  []IncomingAskDTO  `json:"asks"`
		Reels []IncomingReelDTO `json:"reels"`
		Total int               `json:"total" doc:"Everything waiting on a human — asks plus reels"`
	}
}

func (s *Server) registerFillerIncoming(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-incoming", Method: http.MethodGet, Path: "/v1/filler/incoming",
		Summary: "What has been downloaded but isn't filed yet",
		Description: "Admin only (§10 V35) — the Filler page's Incoming tab. Two halves in ONE read: clips whose " +
			"tags need a human (an AI era the tagger could not ground in the clip's text, or a commercial with no " +
			"match tags), and compilations mid-split. ⚠ No confidence score is returned, because nothing measures " +
			"one — `reason` reports the real state instead.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.fillerIncoming)
}

func (s *Server) fillerIncoming(ctx context.Context, _ *struct{}) (*fillerIncomingOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}

	clips, err := s.store.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, huma.Error500InternalServerError("list clips", err)
	}

	out := &fillerIncomingOutput{}
	out.Body.Asks = make([]IncomingAskDTO, 0)
	for _, c := range clips {
		reason, ok := askReason(c)
		if !ok {
			continue
		}
		out.Body.Asks = append(out.Body.Asks, IncomingAskDTO{
			Path: c.Path, Name: c.Name, From: c.Source, DurationMs: c.DurationMs,
			Thumbnail: c.Thumbnail, Kind: string(c.Kind), Era: c.Era,
			Audience: string(c.Audience), Category: c.Category,
			SuggestedEra: c.SuggestedEra, Reason: reason,
		})
	}
	// An ungrounded era sorts ahead of a bare untagged clip: it is a decision with a proposed
	// answer (one click), where an untagged clip needs the operator to supply everything.
	sort.SliceStable(out.Body.Asks, func(i, j int) bool {
		a, b := out.Body.Asks[i], out.Body.Asks[j]
		if (a.SuggestedEra > 0) != (b.SuggestedEra > 0) {
			return a.SuggestedEra > 0
		}
		return a.Path < b.Path
	})

	// ⚠ A split-proposal read failure is NOT fatal to the whole tab. The asks half is the part
	// an operator can always act on, and losing the page because a secondary list did not load
	// would be a poor trade — the same call the sources read-model makes about its remotes.
	out.Body.Reels = make([]IncomingReelDTO, 0)
	if proposals, err := s.store.ListSplitProposals(ctx); err != nil {
		s.log.Warn("list split proposals for incoming", "err", err)
	} else {
		for _, p := range proposals {
			out.Body.Reels = append(out.Body.Reels, IncomingReelDTO{
				ProposalID: p.ID, ClipPath: p.ClipPath, Segments: len(p.Segments),
				NeedsAttention: segmentsNeedingAttention(p),
				CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
	}

	out.Body.Total = len(out.Body.Asks) + len(out.Body.Reels)
	return out, nil
}

// askReason reports why a clip is waiting on a human, and whether it is waiting at all.
//
// ⚠ The two cases are deliberately NOT merged into "needs tags". They are different asks: an
// ungrounded era has a proposed answer the operator confirms or rejects, while an untagged
// commercial has nothing to confirm. Collapsing them would put one button on two questions.
func askReason(c store.Clip) (string, bool) {
	if c.SuggestedEra > 0 {
		// V34's grounding rule, in the operator's terms rather than the validator's.
		return "The year isn't written anywhere in this clip's name or description, so Loomarr guessed it.", true
	}
	// Only commercials: bumpers and station IDs do their bookend job without era/audience/
	// category, so queueing them would fill the review with work that changes nothing. Same
	// rule the AI-tagging job applies (store/clips.go).
	if c.Kind == filler.Commercial && (c.Era == 0 || c.Audience == "" || c.Category == "") {
		return "Loomarr couldn't work out what this is, so it will only match broadly.", true
	}
	return "", false
}

// segmentsNeedingAttention counts the segments an operator cannot simply accept.
//
// Unsplittable and duplicate are first-class outcomes of V34's pipeline, not errors — the point
// of surfacing the count is that a reel of twelve clean segments and a reel with three problems
// are different amounts of work, and the queue should say which is which before it is opened.
func segmentsNeedingAttention(p filler.SplitProposal) int {
	n := 0
	for _, seg := range p.Segments {
		if seg.Unsplittable || seg.DupOf != "" {
			n++
		}
	}
	return n
}
