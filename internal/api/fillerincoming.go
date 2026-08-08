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
// ⚠ **The "nothing here invents a confidence score" rule is RETIRED (V38), not weakened.** It
// stood for two phases and was right the whole time: the mock drew a confidence bar, the tagger
// recorded nothing, and rendering a number nothing measured would have been the same failure as a
// fabricated pull estimate. V38 built the thing the rule was waiting for — a **grounding-capped**
// score (`filler.TagSuggestion.Score`), where the model may only lower a ceiling set by what
// could actually be verified in the clip's own text. So `confidence` is now a real measurement,
// and `filler.autofile.*` — the knob that rule named as needing one — is registered and read.
//
// ⚠ The bar the rule set still applies to everything else here: `reason` stays derived from real
// state and is never generated prose.

// IncomingAskDTO is one clip waiting on a human decision about its tags.
type IncomingAskDTO struct {
	// Hash is the clip's identity (V45a) — used by the single-clip tag PATCH (`/v1/filler/tags`).
	Hash string `json:"hash" doc:"Clip identity — its content hash"`
	// Path is retained for the ARRAY-keyed ops (hold/file/remove take `paths: []` — SetClipsHeld/
	// SetClipsRemoved are path-keyed by design, V38). Not the tag-edit identity; that is Hash.
	Path string `json:"path" doc:"The clip's disk path — used by the array-keyed hold/file/remove ops"`
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
	// Confidence (0-100) is the grounding-capped tagging score (§10 V38) — how sure Loomarr is
	// that these tags are right, and what the auto-file threshold compares against.
	//
	// ⚠ NEVER the model's self-assessment. Grounding facts cap it and the model may only lower
	// that cap, which is why an ungrounded era sits low here however certain the model claimed
	// to be. 0 = never scored (a clip catalogued before V38, or one the tagger has not reached).
	Confidence int `json:"confidence,omitempty"`
	// AutoFiled marks a clip already filed WITHOUT a human — shown so an operator who did not
	// expect auto-filing can find what happened and send it back.
	//
	// ⚠ Asks are held clips, so this is false on all of them. It carries meaning on the
	// `recentlyFiled` half below, and lives on the shared DTO so both halves render identically.
	AutoFiled bool `json:"autoFiled,omitempty"`
}

// IncomingReelDTO is one compilation mid-split.
type IncomingReelDTO struct {
	ProposalID string `json:"proposalId"`
	// ClipHash is the compilation's identity (§10 V38c). ⚠ Was `clipPath` and carried the shard
	// path; the two look alike on screen (`a3/f9/<hash>.mp4` is mostly the hash) which is part of
	// why the mismatch behind it went unnoticed for so long.
	ClipHash string `json:"clipHash"`
	// ClipName is what the operator recognises — the compilation's catalog name.
	//
	// ⚠ Added with the identity rename, because without it this row's title becomes 64 hex
	// characters. The old `clipPath` was no better in production (a filed clip's path IS
	// `a3/f9/<hash>.mp4`); it only LOOKED acceptable because the test fixture used a friendly
	// filename no real catalog contains. Renaming the field made that flattery visible in the
	// visual baseline, which is the honest moment to fix it rather than defer it.
	//
	// Falls back to the hash when the clip has gone — a reel whose compilation was deleted is a
	// real state, and rendering nothing there would hide it.
	ClipName string `json:"clipName"`
	Segments int    `json:"segments" doc:"How many clips the detector found"`
	// NeedsAttention counts segments the operator cannot simply accept — an unsplittable
	// stretch, or one flagged as a duplicate of something already in the catalog.
	NeedsAttention int    `json:"needsAttention"`
	CreatedAt      string `json:"createdAt" doc:"RFC3339"`
}

type fillerIncomingOutput struct {
	Body struct {
		Asks  []IncomingAskDTO  `json:"asks"`
		Reels []IncomingReelDTO `json:"reels"`
		// RecentlyFiled is what Loomarr filed WITHOUT asking (§10 V38) — the audit half of
		// auto-filing.
		//
		// ⚠ It is not decoration and not telemetry. Auto-filing is on by default, so on an
		// upgraded install clips begin entering the catalog unattended; an operator who did not
		// expect that must be able to see exactly what was filed and send any of it back. An
		// unattended decision that cannot be found is not one an appliance gets to make (§10).
		RecentlyFiled []IncomingAskDTO `json:"recentlyFiled"`
		Total         int              `json:"total" doc:"Everything waiting on a human — asks plus reels"`
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

	// ⚠ HELD clips are the queue (§10 V38). Before the lifecycle existed this read the whole
	// catalog and inferred "waiting" from missing tags; now waiting is a STATE, so the queue is
	// simply what is held. `HeldOnly` is required — the default filter excludes exactly these.
	held, err := s.store.ListClips(ctx, store.ClipFilter{HeldOnly: true})
	if err != nil {
		return nil, huma.Error500InternalServerError("list clips", err)
	}

	out := &fillerIncomingOutput{}
	out.Body.Asks = make([]IncomingAskDTO, 0, len(held))
	for _, c := range held {
		out.Body.Asks = append(out.Body.Asks, incomingDTO(c, askReasonFor(c)))
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
			// One read per pending reel to resolve a display name. ⚠ Bounded by design: a
			// proposal exists only while a compilation is waiting on a human, so this list is
			// the review queue, not the catalog. A missing clip is not an error — the reel
			// simply falls back to its identity, which is what a deleted compilation should
			// look like rather than a blank row.
			name := p.ClipHash
			if clip, cerr := s.store.GetClip(ctx, p.ClipHash); cerr == nil && clip.Name != "" {
				name = clip.Name
			}
			out.Body.Reels = append(out.Body.Reels, IncomingReelDTO{
				ProposalID: p.ID, ClipHash: p.ClipHash, ClipName: name,
				Segments:       len(p.Segments),
				NeedsAttention: segmentsNeedingAttention(p),
				CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
	}

	// The audit half: what was filed with nobody looking. ⚠ A read failure here is NOT fatal —
	// same call as the split proposals above. Losing the whole tab because the audit list did not
	// load would take away the queue an operator CAN act on.
	out.Body.RecentlyFiled = make([]IncomingAskDTO, 0)
	// ⚠ Narrowed in SQL. This loaded the WHOLE catalog and discarded all but the auto-filed rows
	// in Go — on an install with thousands of clips, to render a handful of audit cards.
	if filed, ferr := s.store.ListClips(ctx, store.ClipFilter{AutoFiledOnly: true}); ferr != nil {
		s.log.Warn("list auto-filed clips for incoming", "err", ferr)
	} else {
		for _, c := range filed {
			out.Body.RecentlyFiled = append(out.Body.RecentlyFiled, incomingDTO(c, autoFiledReason(c)))
		}
		// Highest confidence last: the ones worth a second look are the ones Loomarr was least
		// sure about, so they sort to the top where an operator scanning the list meets them first.
		sort.SliceStable(out.Body.RecentlyFiled, func(i, j int) bool {
			return out.Body.RecentlyFiled[i].Confidence < out.Body.RecentlyFiled[j].Confidence
		})
	}

	// ⚠ Total counts what is WAITING, so recentlyFiled is deliberately excluded: it is an audit
	// list, not work. Including it would put a badge on the tab that never clears, and a count
	// that cannot reach zero stops being read.
	out.Body.Total = len(out.Body.Asks) + len(out.Body.Reels)
	return out, nil
}

// incomingDTO renders one clip for either half of the tab — shared so an ask and an audit row
// cannot drift into describing the same clip differently.
func incomingDTO(c store.Clip, reason string) IncomingAskDTO {
	return IncomingAskDTO{
		Hash: c.Hash, Path: c.Path, Name: c.Name, From: c.Source, DurationMs: c.DurationMs,
		Thumbnail: c.Thumbnail, Kind: string(c.Kind), Era: c.Era,
		Audience: string(c.Audience), Category: c.Category,
		SuggestedEra: c.SuggestedEra, Reason: reason,
		Confidence: c.Confidence, AutoFiled: c.AutoFiled,
	}
}

// autoFiledReason says why Loomarr filed this without asking, in the operator's terms.
func autoFiledReason(c store.Clip) string {
	if c.Confidence > 0 {
		return "Loomarr was confident enough about these tags to file it without asking."
	}
	return "Filed automatically."
}

// askReasonFor reports why a HELD clip is still waiting, in the operator's terms.
//
// ⚠ **It no longer decides WHETHER a clip is waiting** (§10 V38) — being held is that answer, and
// it is a state rather than something inferred from missing tags. The previous version returned
// `(string, bool)` and the bool was the queue's membership test; a clip that happened to be fully
// tagged could not be queued at all, which the lifecycle needs (a downloaded clip is held whether
// or not the tagger has reached it yet).
//
// ⚠ The cases stay distinct rather than collapsing into "needs tags". An ungrounded era has a
// proposed answer to confirm or reject; an untagged commercial has nothing to confirm; a clip
// below the auto-file bar has tags that simply were not trusted. One button on three questions
// would be wrong for two of them.
func askReasonFor(c store.Clip) string {
	if c.SuggestedEra > 0 {
		// V34's grounding rule, in the operator's terms rather than the validator's.
		return "The year isn't written anywhere in this clip's name or description, so Loomarr guessed it."
	}
	// Only commercials: bumpers and station IDs do their bookend job without era/audience/
	// category, so flagging them would fill the review with work that changes nothing. Same
	// rule the AI-tagging job applies (store/clips.go).
	if c.Kind == filler.Commercial && (c.Era == 0 || c.Audience == "" || c.Category == "") {
		return "Loomarr couldn't work out what this is, so it will only match broadly."
	}
	if c.Confidence > 0 {
		return "Loomarr tagged this but wasn't sure enough to file it without checking."
	}
	// Held, tagged, unscored — the tagger has not reached it yet. Honest about the wait rather
	// than inventing a fault: nothing is wrong with this clip, it is simply in the queue.
	return "Downloaded and waiting to be checked."
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
