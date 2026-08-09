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
// What has been downloaded but is not yet filed:
//
//   - **clips**: the conveyor (§10 V51e) — one row per clip, whether the machine is still
//     preparing it or has finished and handed it over. `needsDecision` says which end it is at.
//   - **reels**: compilations mid-split — the persisted split proposals V34 already writes.
//   - **rejected** / **recentlyFiled**: the two audit halves — what was refused, and what was
//     filed with nobody looking.
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

// IncomingClipDTO is one clip on the Incoming conveyor — being prepared, or waiting on a person.
//
// ⚠ **ONE type for both, because they are one belt.** It was `IncomingAskDTO` (retired-ok), in `asks`
// beside a separate `pipeline` array, and the two overlapped almost completely: the runner enrols
// every clip at `probe/queued` on scan while `asks` asked the older question "is it held and
// untagged?", so on a fresh catalog **84 of 85 clips were in both lists** — one row asking for a
// decision above another saying nothing needed the operator. A clip is at one place on the belt;
// the DTO now says which via `NeedsDecision`.
type IncomingClipDTO struct {
	// NeedsDecision is whether the MACHINE is finished and a person is required.
	//
	// ⚠ Derived from the pipeline's `review` disposition — V51b's own word for the handoff, which
	// the old `asks` query predated and never consulted. The held-and-untagged test survives only
	// as the fallback below for a clip catalogued before V51b: it has no pipeline row at all, and
	// treating "no row" as "still working" would make it invisible forever.
	NeedsDecision bool `json:"needsDecision,omitempty"`
	// Pipeline is where the machine has got to. Absent for a clip with no pipeline row.
	Pipeline *IncomingPipelineDTO `json:"pipeline,omitempty"`
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

// IncomingPipelineDTO is one clip still moving through ingest (§10 V51b).
//
// ⚠ **It carries the WHOLE ladder, not just the current rung**, because this GET is the truth on
// a cold load and on reconnect. The `filler_clip` SSE frames are a latency optimisation that the
// bus drops under load, so a client that built the ladder from frames would render a queue
// silently missing steps.
// ⚠ It carries NO identity and no display fields. It is nested inside the clip it describes, which
// already has the hash, the name, the duration and the thumbnail — and the version of this struct
// that repeated them also performed a `GetClip` PER ROW to fill them in, 85 extra reads to render
// one page. A nested type that re-answers its parent's questions is two sources for one fact.
type IncomingPipelineDTO struct {
	// Stage/Status/Progress are where it is now. Progress is **-1 when the stage cannot measure
	// itself** — a sentinel to render as an indeterminate spinner, never as 0%.
	Stage    string `json:"stage"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Attempts int    `json:"attempts,omitempty" doc:"Retries spent on the current stage; absent on the first try"`
	// Stages is the finished ladder — what ran, what was skipped, and why.
	Stages    []IncomingStageDTO `json:"stages"`
	UpdatedAt string             `json:"updatedAt" doc:"RFC3339"`
}

// IncomingStageDTO is one rung of a clip's ladder.
type IncomingStageDTO struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
	// Note is the operator-facing sentence for a skip or a failure ("vision tagging is off").
	// Empty for an ordinary success — a rung that needs no explanation gets none.
	Note string `json:"note,omitempty"`
	At   string `json:"at" doc:"RFC3339"`
}

// IncomingRejectDTO is one clip ingest refused, and why (§10 V51b).
//
// ⚠ **Not optional, and not telemetry.** `filler.reject.unidentified` is ON by default, so clips
// begin leaving the catalog unattended — including wordless station idents, which §10 calls some
// of the best filler there is. The same rule the auto-file audit list is built on applies with
// more force here: an unattended decision that cannot be found is not one an appliance gets to
// make. This list is how an operator finds it and puts it back.
type IncomingRejectDTO struct {
	Hash string `json:"hash"`
	Name string `json:"name,omitempty"`
	// Reason is a stable CODE, never prose — the frontend owns the wording (the §11 refusal-code
	// precedent `FitReason` follows).
	Reason string `json:"reason"`
	// Detail is the measured fact behind it — "8.2s; the floor is 10.0s" — which is what makes a
	// reject arguable rather than an assertion.
	Detail string `json:"detail,omitempty"`
	// Restorable reports whether an operator may override this one. A HARD reject (no audio, no
	// video, unreadable) offers no override, because that is a control that could not work:
	// restoring a clip with no audio puts silence in a break.
	Restorable bool   `json:"restorable"`
	Stage      string `json:"stage" doc:"Which stage refused it"`
	At         string `json:"at" doc:"RFC3339"`
}

type fillerIncomingOutput struct {
	Body struct {
		// Clips is the whole conveyor, in one list: what is being prepared and what is waiting on
		// a person, ordered decisions-first.
		//
		// ⚠ This replaced `asks` + `pipeline`. They were separate arrays over overlapping
		// populations, and the operator saw the same clip twice — once demanded of them, once
		// described as "just working". One belt, one row per clip, `needsDecision` says which end.
		Clips []IncomingClipDTO `json:"clips"`
		Reels []IncomingReelDTO `json:"reels"`
		// Rejected is the audit half of refusal, the sibling of RecentlyFiled.
		Rejected []IncomingRejectDTO `json:"rejected"`
		// StageOrder is the whole ladder, in order — the answer to "which rungs are still ahead
		// of this clip", which no per-clip row can give.
		//
		// ⚠ **`IncomingPipelineDTO.Stages` is the VISITED ladder, not the whole one.** A clip
		// sitting at `split` carries three records; a strip that draws one pip per record would
		// grow as the clip advances, so the operator could never see how far there is left to go.
		// The remaining rungs have to come from somewhere, and the only honest source is the
		// server: `filler.StageOrder` is the runner's own sequence, and rendering from it means a
		// rung added to the pipeline appears in the UI without a second edit.
		//
		// ⚠ It is NOT filtered by what applies to this install. A disabled stage still runs as a
		// `skipped` record with its reason attached — "vision tagging is off" is a fact an
		// operator needs, and a ladder that silently omitted the rung would present a shorter
		// pipeline as if that were the whole of it. Applicability is per clip and re-evaluated
		// every pass (§10 V51b); the ladder is fixed.
		StageOrder []string `json:"stageOrder" doc:"Every pipeline stage id, in run order"`
		// RecentlyFiled is what Loomarr filed WITHOUT asking (§10 V38) — the audit half of
		// auto-filing.
		//
		// ⚠ It is not decoration and not telemetry. Auto-filing is on by default, so on an
		// upgraded install clips begin entering the catalog unattended; an operator who did not
		// expect that must be able to see exactly what was filed and send any of it back. An
		// unattended decision that cannot be found is not one an appliance gets to make (§10).
		RecentlyFiled []IncomingClipDTO `json:"recentlyFiled"`
		Total         int               `json:"total" doc:"Everything waiting on a human — asks plus reels"`
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

	// The conveyor, assembled from two reads that used to be two LISTS (§10 V51e).
	//
	// ⚠ A pipeline read failure degrades the belt to the held clips rather than emptying the tab:
	// the same non-fatal treatment the split proposals and the audit list get. Every clip then
	// reports `needsDecision` through the legacy fallback, which is the honest answer when the
	// pipeline's state could not be read at all.
	pipeByHash := map[string]filler.ClipPipeline{}
	if rows, perr := s.store.ListClipPipelines(ctx, filler.PipelineFilter{ConveyorOnly: true, Limit: incomingPipelineLimit}); perr != nil {
		s.log.Warn("list pipeline rows for incoming", "err", perr)
	} else {
		for _, r := range rows {
			pipeByHash[r.ClipHash] = r
		}
	}

	out.Body.Clips = make([]IncomingClipDTO, 0, len(held))
	seen := make(map[string]bool, len(held))
	for _, c := range held {
		seen[c.Hash] = true
		out.Body.Clips = append(out.Body.Clips, conveyorDTO(c, pipeByHash[c.Hash]))
	}
	// ⚠ A clip on the belt that is NOT held still belongs here. Holding and enrolment are set by
	// different writers, so "enrolled but not held" is reachable — and a clip the machine is
	// visibly working on must not vanish from the one surface that reports on it.
	for hash, row := range pipeByHash {
		if seen[hash] {
			continue
		}
		clip, cerr := s.store.GetClip(ctx, hash)
		if cerr != nil {
			continue
		}
		out.Body.Clips = append(out.Body.Clips, conveyorDTO(clip, row))
	}

	// ⚠ Decisions first: work the operator OWES outranks work they merely watch. Within the
	// decisions an ungrounded era sorts ahead of a bare untagged clip — it is one click, where an
	// untagged clip needs everything supplied. Within the in-flight half, most recently moved
	// first, so the rows that are actually changing sit where the eye already is.
	sort.SliceStable(out.Body.Clips, func(i, j int) bool {
		a, b := out.Body.Clips[i], out.Body.Clips[j]
		if a.NeedsDecision != b.NeedsDecision {
			return a.NeedsDecision
		}
		if a.NeedsDecision {
			if (a.SuggestedEra > 0) != (b.SuggestedEra > 0) {
				return a.SuggestedEra > 0
			}
			return a.Path < b.Path
		}
		ap, bp := a.Pipeline, b.Pipeline
		if ap != nil && bp != nil && ap.UpdatedAt != bp.UpdatedAt {
			return ap.UpdatedAt > bp.UpdatedAt
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
	out.Body.RecentlyFiled = make([]IncomingClipDTO, 0)
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

	// The audit half of refusal (§10 V51b). ⚠ Deliberately NOT on the conveyor: a refused clip has
	// left the belt, and folding it back in would make "what is Loomarr doing" and "what did
	// Loomarr decide without me" the same list again — the merge this phase just undid.
	// ⚠ Derived from `filler.StageOrder` rather than listed here. A second copy of the sequence
	// is the drift class this codebase keeps finding: the runner would advance through one list
	// and the UI would draw another, and the only symptom would be a pip in the wrong place.
	out.Body.StageOrder = make([]string, 0, len(filler.StageOrder))
	for _, id := range filler.StageOrder {
		out.Body.StageOrder = append(out.Body.StageOrder, string(id))
	}

	out.Body.Rejected = make([]IncomingRejectDTO, 0)
	if rows, rerr := s.store.ListClipPipelines(ctx, filler.PipelineFilter{RejectedOnly: true, Limit: incomingRejectLimit}); rerr != nil {
		s.log.Warn("list rejected clips for incoming", "err", rerr)
	} else {
		for _, r := range rows {
			out.Body.Rejected = append(out.Body.Rejected, rejectDTO(ctx, s, r))
		}
	}

	// ⚠ Total counts what is WAITING, so recentlyFiled is deliberately excluded: it is an audit
	// list, not work. Including it would put a badge on the tab that never clears, and a count
	// that cannot reach zero stops being read.
	//
	// ⚠ **Clips still being PREPARED and `rejected` are excluded for the same reason, arrived at
	// differently.** A clip mid-ingest is not waiting on a person — it is waiting on the machine,
	// and badging it would make the tab count tick up every time a download starts. A reject is
	// an audit row exactly like an auto-filed one. Neither is work the operator owes.
	//
	// ⚠ This used to read `len(out.Body.Asks)`, which was the same number by accident and the
	// wrong rule by construction: `asks` included clips the machine had not finished with, so the
	// badge counted work nobody owed. Counting `NeedsDecision` makes the number and the list agree
	// — they disagreed before, and the list was the honest one.
	for _, c := range out.Body.Clips {
		if c.NeedsDecision {
			out.Body.Total++
		}
	}
	out.Body.Total += len(out.Body.Reels)
	return out, nil
}

// The two lists are BOUNDED, and each bound is a different judgement.
//
// `pipeline` is naturally small — the runner advances one clip at a time and a pass is budgeted —
// so this cap only matters during a large import, where showing the first hundred movers is as
// useful as showing four hundred. `rejected` is an audit feed ordered newest-first, so the cap is
// a page rather than a filter: what an operator is looking for is what was just refused.
const (
	incomingPipelineLimit = 100
	incomingRejectLimit   = 100
)

// pipelineDTO renders one in-flight clip, resolving its display name.
//
// ⚠ A missing clip is not an error. A row whose clip has gone is what a pruned catalog looks like
// mid-pass, and falling back to the identity is more honest than a blank row.
// conveyorDTO renders one clip on the belt, with its pipeline state attached when it has one.
//
// ⚠ **`needsDecision` is the pipeline's answer, not a second opinion derived from tags.** The
// zero-value `row` (no pipeline row at all) is the only case that falls back to the V38 test, and
// it exists for one population: clips catalogued before V51b. Treating "no row" as "still being
// prepared" would strand them in a section that says nothing needs the operator, forever.
func conveyorDTO(c store.Clip, row filler.ClipPipeline) IncomingClipDTO {
	dto := incomingDTO(c, askReasonFor(c))
	if row.ClipHash == "" {
		dto.NeedsDecision = true
		return dto
	}
	p := pipelineDTO(row)
	dto.Pipeline = &p
	dto.NeedsDecision = row.Disposition == filler.DispositionReview
	return dto
}

func pipelineDTO(r filler.ClipPipeline) IncomingPipelineDTO {
	dto := IncomingPipelineDTO{
		Stage: string(r.Stage), Status: string(r.Status),
		Progress: r.Progress, Attempts: r.Attempts,
		Stages:    make([]IncomingStageDTO, 0, len(r.Stages)),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	for _, rec := range r.Stages {
		dto.Stages = append(dto.Stages, IncomingStageDTO{
			Stage: string(rec.Stage), Status: string(rec.Status), Note: rec.Note,
			At: rec.At.UTC().Format(time.RFC3339),
		})
	}
	return dto
}

// rejectDTO renders one refusal.
func rejectDTO(ctx context.Context, s *Server, r filler.ClipPipeline) IncomingRejectDTO {
	dto := IncomingRejectDTO{
		Hash: r.ClipHash, Reason: string(r.RejectReason), Detail: r.RejectDetail,
		// ⚠ `Soft()` owns which reasons a human may overturn, so the button's presence and the
		// endpoint's behaviour cannot disagree. Deriving it here from a second list would be the
		// drift class this codebase keeps finding.
		Restorable: r.RejectReason.Soft(),
		Stage:      string(r.Stage),
		At:         r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if clip, err := s.store.GetClip(ctx, r.ClipHash); err == nil && clip.Name != "" {
		dto.Name = clip.Name
	}
	return dto
}

// incomingDTO renders one clip for either half of the tab — shared so an ask and an audit row
// cannot drift into describing the same clip differently.
func incomingDTO(c store.Clip, reason string) IncomingClipDTO {
	return IncomingClipDTO{
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
