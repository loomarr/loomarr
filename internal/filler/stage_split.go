package filler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The SPLIT stage (§10 V51b): a compilation is cut into the adverts it holds.
//
// Detection is `Splitter.Propose` and the gate is `AutoCuttable` (#4409). The probe rung decides a
// clip is a candidate (it sets `is_composite`), and the pieces are SPAWNED, so each segment runs the
// whole ladder for itself instead of waiting for cron jobs to notice it.
//
// ⚠ **`filler.autosplit.enabled` defaults ON** (maintainer decision, V51b). Each cut it drops
// carries `AutoSplitReject`'s reason, so "why was this not cut?" is answerable without reading the
// log.
//
// ⚠ **The gate is PER SEGMENT (V54).** One doubtful cut in 52 drops that cut, never the other 51.

// SplitStageStore is the slice of the store the split stage reads.
//
// ⚠ One read of the pending queue rather than a per-clip existence check — the queue is a review
// backlog a human is expected to clear, so it is small by construction, and re-detecting over a
// proposal an operator is halfway through editing is the thing being prevented.
type SplitStageStore interface {
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
}

// SplitStage proposes (and where allowed, confirms) the cuts inside a compilation.
type SplitStage struct {
	splitter *Splitter
	store    SplitStageStore
	// autoConfirm is the opt-in gate. nil ⇒ propose only, every reel waits for a human.
	autoConfirm *AutoSplitPolicy
	// minClipDuration is `filler.min_duration`, the SAME floor the scan boundary rejects on.
	minClipDuration func() time.Duration
	// vision grounds proposed segments from their own frames so the gate has data (§10 V54).
	// nil ⇒ propose only.
	vision *SegmentVision
	// log reports what a grounding pass actually did (§10 V54b). nil is tolerated everywhere.
	log *slog.Logger
}

// NewSplitStage builds the stage. Without `WithAutoConfirm` it PROPOSES ONLY, which is the safe
// default: proposing writes no clips and consumes no file, so the waiting is removed without the
// deciding.
func NewSplitStage(splitter *Splitter, store SplitStageStore) *SplitStage {
	return &SplitStage{splitter: splitter, store: store}
}

// WithLogger attaches the logger the grounding pass reports through (§10 V54b).
//
// ⚠ **Grounding had SEVEN distinct silent outcomes and no way to tell them apart.** Not wired,
// zero budget, unreadable taxonomy, no keyframes, a provider that refused, an unparseable answer,
// and — the one that actually bit — a provider that answered every time and yielded nothing
// usable. All seven produced the same observable: segments left ungrounded, the gate refusing with
// "a segment could not be classified", and not one line anywhere saying which.
func (s *SplitStage) WithLogger(log *slog.Logger) *SplitStage {
	s.log = log
	return s
}

// WithAutoConfirm attaches the auto-confirm policy and the clip floor it must respect.
func (s *SplitStage) WithAutoConfirm(pol AutoSplitPolicy, minClipDuration func() time.Duration) *SplitStage {
	s.autoConfirm = &pol
	s.minClipDuration = minClipDuration
	return s
}

// WithSegmentVision attaches the split-time grounder that answers the auto-confirm gate (§10 V54).
//
// ⚠ **Without this, `filler.autosplit.enabled` is default-ON and cannot fire.** `AutoCuttable`
// refuses any cut whose look did not see one whole piece of filler, and the split-time grounder is
// the only thing that looks: text classification at split time was removed in V51g (§10) because
// it cost 3× the pass budget and classified nothing but a generated name.
//
// ⚠ **It grounds from PIXELS, not from the segment's name, and that is the whole difference.** The
// removed classifier read `"… part 7"` and grounded nothing, which is why speeding it up would not
// have helped. Frames from the segment's own span exist at split time and carry a real signal — the
// same signal the `vision` rung reads, through the same prompt and the same `groundVisionTags`, so
// the gate is fed by the vocabulary that judges it.
//
// nil ⇒ propose only, exactly as a nil `autoConfirm` does. A missing grounder must never mean
// "confirm without one".
func (s *SplitStage) WithSegmentVision(v *SegmentVision) *SplitStage {
	s.vision = v
	return s
}

// SegmentRoleEscalator is the split stage's complete interface to temporal model evidence.
// The implementation owns derivative extraction, request limits, strict decoding, and evidence
// attribution; the caller knows only that an exact unresolved span may gain one role observation.
type SegmentRoleEscalator interface {
	EscalateRole(context.Context, split.SourceAsset, string, split.Segment, time.Time) (*split.RoleEvidence, error)
}

// SegmentVision grounds proposed segments from their own frames so the auto-confirm gate has
// something to judge.
type SegmentVision struct {
	Tools    tools.MediaTools
	Provider llm.VisionProvider
	// RoleEscalator inspects an exact bounded video only when frame evidence cannot establish
	// a role. nil leaves the span unresolved; it never weakens the publication gate.
	RoleEscalator SegmentRoleEscalator
	Taxa          TaxaLister
	// Loudness measures a cut's sound (#4409). nil measures nothing, and an unmeasured cut is
	// never made unattended.
	Loudness LoudnessMeter
	ClipDir  string
	// Budget caps how many segments ONE pass will look at (`filler.pipeline.max_split_vision`).
	//
	// ⚠ It is a per-pass cost bound, and §10 V51g is why it is not optional: a rung's cost must
	// stay inside its pass budget.
	//
	// ⚠ **A RATE, not a ceiling (V54).** Reels run to 82–303 segments against a default of 60, so
	// grounding persists and resumes and a large reel is judged over several passes; a segment
	// still ungrounded when the gate runs is held back on its own rather than sinking the reel.
	Budget func() int
}

// LoudnessMeter is the RMS level, in dB, of each winMs window of a file's audio over a span.
type LoudnessMeter interface {
	LoudnessWindows(ctx context.Context, file string, startMs, endMs, winMs int64) ([]float64, error)
}

// measureSound sets the sound of each cut that has none. It is cheap (one audio decode of the cut
// and a little either side) and not part of the vision budget. A cut whose sound cannot be read
// stays unmeasured, which never cuts; the next pass tries again.
func (s *SplitStage) measureSound(ctx context.Context, file string, segs []split.Segment) (measured, failed int) {
	if s.vision == nil || s.vision.Loudness == nil {
		return 0, 0
	}
	for i := range segs {
		if segs[i].Sound != nil || segs[i].EndMs <= segs[i].StartMs || ctx.Err() != nil {
			continue
		}
		origin := max(0, segs[i].StartMs-split.SoundPadMs)
		levels, err := s.vision.Loudness.LoudnessWindows(ctx, file, origin, segs[i].EndMs+split.SoundPadMs, split.SoundWindowMs)
		if err != nil {
			failed++
			continue
		}
		sound := split.MeasureCutSound(levels, origin, segs[i].StartMs, segs[i].EndMs)
		segs[i].Sound = &sound
		measured++
	}
	return measured, failed
}

// TaxaLister reads the taxonomy the grounder resolves categories against — the SAME read the
// `vision` and `tag` rungs make, so all three ground against one vocabulary.
type TaxaLister interface {
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

// groundFromSource stamps Category/Era onto each segment it can see, in place.
//
// Best-effort throughout: a frame-extraction or model failure leaves that segment ungrounded,
// which the gate then refuses — a reel that could not be judged goes to review rather than being
// confirmed on missing data. That direction is the safety property, so every error path here
// degrades toward review and never toward confirm.
func (s *SplitStage) groundFromSource(ctx context.Context, c clipstore.Clip, source split.SourceAsset, segs []split.Segment) groundPass {
	if s.vision == nil {
		return s.groundAt(ctx, c, "", source, segs)
	}
	_, file, err := resolveSplitSource(ctx, s.vision.ClipDir, c, source)
	if err != nil {
		s.groundSkipped(ctx, c, "the proposal's evidence derivative is unavailable", "err", err)
		return groundPass{Pending: countPendingGrounding(segs)}
	}
	return s.groundAt(ctx, c, file, source, segs)
}

func countPendingGrounding(segs []split.Segment) int { return countPending(segs) }

// needsLook reports whether a segment still needs the split look. A segment looked at before the
// look judged the whole cut (#4409) has no Whole answer, and is looked at again.
func needsLook(s split.Segment) bool { return !s.Looked || s.Whole == "" }

func (s *SplitStage) groundAt(ctx context.Context, c clipstore.Clip, file string, source split.SourceAsset, segs []split.Segment) groundPass {
	pending := func() int {
		return countPendingGrounding(segs)
	}
	if s.vision == nil || s.vision.Provider == nil || s.vision.Tools == nil {
		s.groundSkipped(ctx, c, "vision is not wired on this install")
		return groundPass{Pending: pending()}
	}
	measured, failed := s.measureSound(ctx, file, segs)
	if failed > 0 && s.log != nil {
		s.log.Warn("filler split: some cuts' sound could not be measured; they are not cut until it is",
			"clip", c.Hash, "cuts", failed)
	}
	// ⚠ The budget bounds ATTEMPTS THIS PASS, not an index into the segment list. It was
	// `if i >= budget { return }`, which is absolute — with resume that stops at the same segment
	// on every pass forever, so a 142-segment reel would ground its first 60 and then loop.
	budget := len(segs)
	if s.vision.Budget != nil {
		if b := s.vision.Budget(); b >= 0 && b < budget {
			budget = b
		}
	}
	if budget == 0 {
		s.groundSkipped(ctx, c, "the per-pass vision budget is zero (filler.pipeline.max_split_vision)")
		return groundPass{Pending: pending(), Measured: measured}
	}
	var forest *taxonomy.Forest
	if s.vision.Taxa != nil {
		taxa, err := s.vision.Taxa.ListTaxa(ctx)
		if err != nil {
			// No vocabulary to ground against ⇒ ground nothing, and the gate refuses. Reported as
			// zero looked, which is what stops the caller deferring on a pass that achieved nothing.
			s.groundSkipped(ctx, c, "the taxonomy could not be read", "err", err)
			return groundPass{Pending: pending(), Measured: measured}
		}
		forest = taxonomy.New(taxa)
	}
	looked := 0
	// The pass tally. Counted rather than logged per segment: 60 lines a pass is noise, and the
	// question an operator has is about the pass, not any one cut.
	var noFrames, unreadable, learned, roles, unresolvedRoles, videoAttempts, videoRoles, videoErrors int
	var providerErr error
	for i := range segs {
		if looked >= budget {
			break
		}
		if !needsLook(segs[i]) {
			continue // ⚠ THE RESUME: an earlier pass already spent a look on this one.
		}
		frames, err := s.vision.Tools.VisionKeyframesIn(ctx, file, segs[i].StartMs, segs[i].EndMs, split.WholeCutKeyframes)
		if err != nil || len(frames) == 0 {
			// No frames, no judgement of the whole cut: a doubt, and doubt never cuts.
			segs[i].Looked, segs[i].Whole, looked = true, split.WholeUnsure, looked+1
			noFrames++
			if evidence, attempted, escalationErr := s.escalateSegmentRole(ctx, source, file, segs[i]); attempted {
				videoAttempts++
				if escalationErr != nil {
					videoErrors++
					unresolvedRoles++
				} else if evidence != nil {
					segs[i].RoleEvidence = evidence
					roles, videoRoles, learned = roles+1, videoRoles+1, learned+1
				} else {
					unresolvedRoles++
				}
			} else {
				unresolvedRoles++
			}
			continue
		}
		prompt := splitVisionPrompt(forest)
		resp, err := s.vision.Provider.AskAboutImages(llm.WithCallSite(ctx, "filler.split_vision"), prompt, frames)
		if err != nil {
			// A separately routed direct-video model may still settle the temporal role. Only a
			// successful escalation marks the span looked; if both routes fail, the old retry
			// behavior remains and this pass stops without burning the rest of the reel.
			if evidence, attempted, escalationErr := s.escalateSegmentRole(ctx, source, file, segs[i]); attempted {
				videoAttempts++
				if escalationErr != nil {
					videoErrors++
					providerErr = errors.Join(err, escalationErr)
					break
				}
				// The video route settles the role only; the whole cut was not judged, so it is a
				// doubt. Marking it keeps the resume bounded while the image route keeps failing.
				segs[i].Looked, segs[i].Whole, looked = true, split.WholeUnsure, looked+1
				if evidence != nil {
					segs[i].RoleEvidence = evidence
					segs[i].LookedRole = evidence.Role
					roles, videoRoles, learned = roles+1, videoRoles+1, learned+1
				} else {
					unresolvedRoles++
				}
				continue
			}
			providerErr = err
			break
		}
		segs[i].Looked, looked = true, looked+1
		var out visionOutput
		parsed := json.Unmarshal([]byte(llm.ExtractJSONObject(resp.Content)), &out) == nil
		if !parsed {
			unreadable++
		}
		// An unreadable answer parses as WholeUnsure: doubt never cuts.
		segs[i].Whole = split.ParseWholeCut(out.Whole)
		segs[i].LookedRole = split.LookRole(out.Role)
		v := groundVisionTags(out, forest)
		var roleEvidence *split.RoleEvidence
		var roleErr error
		if parsed {
			roleEvidence, roleErr = structureRoleEvidenceFromVision(source, segs[i], splitVisionPromptVersion, prompt, frames, resp, out, s.structureAssessedAt())
		}
		if roleEvidence == nil {
			if evidence, attempted, escalationErr := s.escalateSegmentRole(ctx, source, file, segs[i]); attempted {
				videoAttempts++
				if escalationErr != nil {
					videoErrors++
				} else if evidence != nil {
					roleEvidence = evidence
					roleErr = nil
					videoRoles++
				}
			}
		}
		if roleErr == nil && roleEvidence != nil {
			segs[i].RoleEvidence = roleEvidence
			segs[i].LookedRole = roleEvidence.Role // the validated role wins over the raw answer
			roles++
		} else {
			unresolvedRoles++
		}
		if len(v.Tags) > 0 || v.Era > 0 || roleEvidence != nil {
			learned++
		}
		segs[i].Tags = unionLeaves(segs[i].Tags, v.Tags)
		segs[i].Category = v.Category
		segs[i].Era = v.Era
		if segs[i].NameOrigin == SplitNameFallback {
			if name, evidence, ok := groundSegmentName(out.ProposedTitle, v.VisibleText); ok {
				segs[i].Name = name
				segs[i].NameOrigin = SplitNameModelProposed
				segs[i].NameEvidence = evidence
			}
		}
		// ⚠ `SuggestedEra` is deliberately NOT stamped here, though the vision RUNG does stamp it.
		// The gate treats a suggested era as an automatic refusal at every threshold — "an era
		// Loomarr GUESSED is exactly the case a human should see" — so writing one would make this
		// grounder guarantee the rejection it exists to lift. The child clip's own vision pass
		// still raises the suggestion afterwards, where an operator can act on it.
	}
	s.groundReport(ctx, c, groundTally{
		looked: looked, pending: pending(), learned: learned,
		noFrames: noFrames, unreadable: unreadable, roles: roles, unresolvedRoles: unresolvedRoles,
		videoAttempts: videoAttempts, videoRoles: videoRoles, videoErrors: videoErrors,
		providerErr: providerErr,
	})
	return groundPass{Looked: looked, Pending: pending(), Measured: measured}
}

// groundTally is one pass's outcome, counted so the pass can be reported in a single line.
type groundTally struct {
	looked, pending, learned, noFrames, unreadable, roles, unresolvedRoles int
	videoAttempts, videoRoles, videoErrors                                 int
	providerErr                                                            error
}

func (s *SplitStage) escalateSegmentRole(ctx context.Context, source split.SourceAsset, file string, segment split.Segment) (*split.RoleEvidence, bool, error) {
	if s.vision == nil || s.vision.RoleEscalator == nil || source.Validate() != nil {
		return nil, false, nil
	}
	evidence, err := s.vision.RoleEscalator.EscalateRole(ctx, source, file, segment, s.structureAssessedAt())
	return evidence, true, err
}

func (s *SplitStage) structureAssessedAt() time.Time {
	if s.splitter != nil && s.splitter.now != nil {
		return s.splitter.now().UTC()
	}
	return time.Now().UTC()
}

// groundSkipped reports a pass that never began. Each caller passes a DIFFERENT reason, which is
// the entire point: these were four indistinguishable early returns.
func (s *SplitStage) groundSkipped(ctx context.Context, c clipstore.Clip, why string, args ...any) {
	if s.log == nil {
		return
	}
	s.log.WarnContext(ctx, "split grounding did not run: "+why, append([]any{"clip", c.Hash}, args...)...)
}

// groundReport reports what a pass achieved.
//
// ⚠ **The `learned == 0` case is the one this exists for, and it is not an error anywhere.** A
// model can answer every single call, parse cleanly, and still ground nothing — `llava:7b`
// (measured 2026-08-13) echoed the prompt's own placeholder list back as its answer, so
// `"category":"toys|cereal|cars|..."` arrived as well-formed JSON and `groundVisionTags` correctly
// dropped every field. Thirty-seven segments, thirty-seven successful calls, one usable category,
// and the reel refused as untagged. Every counter along that path says "fine": no provider error,
// no parse error, `Looked` incremented 37 times. Only `learned` tells the truth, and only by being
// compared against `looked`.
func (s *SplitStage) groundReport(ctx context.Context, c clipstore.Clip, t groundTally) {
	if s.log == nil || (t.looked == 0 && t.providerErr == nil) {
		return
	}
	args := []any{
		"clip", c.Hash, "looked", t.looked, "learned", t.learned,
		"pending", t.pending, "noFrames", t.noFrames, "unreadable", t.unreadable,
		"roles", t.roles, "unresolvedRoles", t.unresolvedRoles,
		"videoAttempts", t.videoAttempts, "videoRoles", t.videoRoles, "videoErrors", t.videoErrors,
	}
	switch {
	case t.providerErr != nil:
		s.log.WarnContext(ctx, "split grounding stopped: the vision provider refused",
			append(args, "err", t.providerErr)...)
	case t.looked > 0 && t.learned == 0:
		s.log.WarnContext(ctx, "split grounding read every cut and learned nothing — check filler.vision.model can follow a JSON prompt", args...)
	default:
		s.log.InfoContext(ctx, "split grounding pass", args...)
	}
}

// pendingFor returns the proposal already waiting for this clip, or nil.
//
// ⚠ One read of the whole pending list rather than a per-clip lookup, unchanged from V51b: the
// queue is a review backlog a human is expected to clear, so it is small by construction.
func (s *SplitStage) pendingFor(ctx context.Context, hash string) (*SplitProposal, error) {
	pending, err := s.store.ListSplitProposals(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pending {
		if pending[i].ClipHash == hash {
			return &pending[i], nil
		}
	}
	return nil, nil
}

// countPending counts segments the grounder has never looked at.
func countPending(segs []split.Segment) int {
	n := 0
	for i := range segs {
		if needsLook(segs[i]) {
			n++
		}
	}
	return n
}

// groundPass is what ONE grounding pass achieved: how many segments it looked at, and how many
// have still never been looked at.
//
// ⚠ `Looked == 0 && Pending > 0` is the termination signal, and the difference matters: it means
// the pass could not make progress (vision off, no vocabulary, the provider down at the first
// segment), so the caller must NOT defer — it falls through to the gate and the reel parks with a
// reason. Deferring on a pass that achieved nothing is an infinite loop.
type groundPass struct {
	Looked  int
	Pending int
	// Measured counts cuts whose sound this pass measured. It is persisted like a look, but it is
	// not progress toward a verdict: only looks bound the deferral.
	Measured int
}

type splitDiscards struct {
	duplicates int
	short      int
}

// discardDeterministic removes outcomes that cannot become useful new clips. Ambiguous or
// unsplittable spans remain for review; known duplicates and below-floor fragments do not.
func discardDeterministic(segs []split.Segment, minClipDuration time.Duration) ([]split.Segment, splitDiscards) {
	kept := make([]split.Segment, 0, len(segs))
	var discarded splitDiscards
	for _, seg := range segs {
		if seg.DupOf != "" {
			discarded.duplicates++
			continue
		}
		span := time.Duration(seg.EndMs-seg.StartMs) * time.Millisecond
		if minClipDuration > 0 && span < minClipDuration {
			discarded.short++
			continue
		}
		seg.Index = len(kept)
		kept = append(kept, seg)
	}
	return kept, discarded
}

func discardNote(d splitDiscards) string {
	switch {
	case d.duplicates > 0 && d.short > 0:
		return fmt.Sprintf("discarded %d duplicate(s) and %d short fragment(s)", d.duplicates, d.short)
	case d.duplicates > 0:
		return fmt.Sprintf("discarded %d duplicate(s)", d.duplicates)
	case d.short > 0:
		return fmt.Sprintf("discarded %d short fragment(s)", d.short)
	default:
		return ""
	}
}

func (s *SplitStage) ID() pipelinestate.StageID { return pipelinestate.StageSplit }
func (s *SplitStage) Cost() StageCost           { return CostSplit }

// Applies to a composite, and nothing else.
//
// ⚠ **The candidate decision is the probe rung's, not this one's.** Probe measures the file and
// sets `is_composite`; this rung acts on that mark. Splitting the question that way is what lets
// the expensive boundary scan be gated on duration in ONE place, and it is why a 16-minute
// recording is excluded from pods the moment it is measured rather than only once someone splits
// it — the exact bug §10 V45 describes.
func (s *SplitStage) Applies(_ context.Context, c clipstore.Clip) (bool, string) {
	if s.splitter == nil {
		return false, "splitting is not available on this install"
	}
	if !c.IsComposite {
		return false, "not a compilation"
	}
	return true, ""
}

// Run detects the cuts and either makes them or leaves them for a human.
func (s *SplitStage) Run(ctx context.Context, c clipstore.Clip) (StageResult, error) {
	// Already review-pending: re-detecting would replace a proposal an operator may be halfway
	// through editing. ⚠ Keyed by HASH on both sides — the identity `Propose` takes.
	// ⚠ An existing proposal is RE-GROUNDED, never re-detected. Returning outright would look like
	// "leave the operator's cut list alone" but is a dead end: the row goes to `review`,
	// `ListPipelineWork` only claims `running`, and nothing would ever reach that reel again.
	//
	// Re-detection stays the operator's call (`POST /v1/filler/split`): a rung must not redraw a
	// cut list a human may have open. Grounding is additive and touches no boundary.
	p, err := s.pendingFor(ctx, c.Hash)
	if err != nil {
		return StageResult{}, err
	}
	if p == nil || !p.Ready() {
		var complete bool
		if p, complete, err = s.splitter.advanceProposal(ctx, c.Hash, p); err != nil {
			return StageResult{}, err
		}
		if !complete {
			return StageResult{}, ErrDeferred
		}
	}
	// Representative frames are presentation evidence, not detector authority. Prepare them after
	// the proposal is reviewable, persist a bounded batch, and resume long reels on later passes.
	// A failed frame is marked unavailable inside prepareArtwork so it cannot trap the pipeline.
	if !p.ArtworkPrepared {
		prepared, pending, artworkErr := s.splitter.prepareArtwork(ctx, *p)
		if artworkErr != nil {
			return StageResult{}, artworkErr
		}
		p = &prepared
		if pending > 0 {
			return StageResult{Verdict: pipelinestate.VerdictDefer, Note: fmt.Sprintf(
				"prepared previews for %d of %d clips", len(p.Segments)-pending, len(p.Segments))}, nil
		}
	}
	// A segment whose duplicate check could not run is checked again on every pass, before the gate
	// reads it: the tooling may be back. One that still cannot be checked is held back below, never
	// accepted, because an aired duplicate cannot be recalled while a stalled split can.
	if countDedupUnchecked(p.Segments) > 0 {
		remaining, recheckErr := s.splitter.RecheckDedup(ctx, p)
		if errors.Is(recheckErr, ErrProposalGone) {
			return StageResult{Verdict: pipelinestate.VerdictContinue, Note: "already resolved"}, nil
		}
		if recheckErr != nil {
			return StageResult{}, recheckErr
		}
		if remaining > 0 && s.log != nil {
			s.log.WarnContext(ctx, "split auto-confirm: segments held back because they could not be checked for duplicates",
				"clip", c.Hash, "held", remaining, "segments", len(p.Segments))
		}
	}
	// Deterministic outcomes are curation, not decisions. Persist them before any model work so a
	// restart and the review UI see the same smaller reel.
	kept, discarded := discardDeterministic(p.Segments, s.minClipFloor())
	if len(kept) != len(p.Segments) {
		p.Segments = kept
		if err := s.splitter.saveProposal(ctx, *p); err != nil {
			return StageResult{}, err
		}
	}
	if len(p.Segments) == 0 {
		if err := s.splitter.resolveEmpty(ctx, p.ID); err != nil {
			return StageResult{}, err
		}
		note := ""
		if excluded := len(p.LanguageExclusions); excluded > 0 {
			clipWord := "clips"
			if excluded == 1 {
				clipWord = "clip"
			}
			note = fmt.Sprintf("left out %d %s spoken in another language", excluded, clipWord)
		} else {
			note = discardNote(discarded)
		}
		if note == "" {
			note = "no usable adverts remained"
		}
		return StageResult{Verdict: pipelinestate.VerdictContinue, Note: note}, nil
	}

	// Ground the segments from their own frames BEFORE the gate reads them (§10 V54).
	pass := s.groundFromSource(ctx, c, p.Source, p.Segments)

	// Persist what this pass learned, so the next one resumes instead of starting over.
	if pass.Looked > 0 || pass.Measured > 0 {
		merged, rerr := s.splitter.Reground(ctx, p.ID, p.Segments)
		switch {
		case errors.Is(rerr, ErrProposalGone):
			// Confirmed or rejected under us while we were grounding. Nothing to write and nothing
			// to decide — the clip's disposition is already someone else's answer.
			return StageResult{Verdict: pipelinestate.VerdictContinue, Note: "already resolved"}, nil
		case rerr != nil:
			return StageResult{}, rerr
		}
		p = &merged
		pass = groundPass{Looked: pass.Looked, Pending: countPending(p.Segments)}
	}

	// ⚠ Partly grounded is UNFINISHED WORK, not a refusal. `filler.pipeline.max_split_vision`
	// bounds ONE PASS; a 142-segment reel is judged over several. Deferring requires that this
	// pass achieved something (`Looked > 0`), which is what guarantees termination: every look
	// marks a segment, so `Pending` strictly decreases and the reel reaches a verdict.
	if pass.Pending > 0 && pass.Looked > 0 {
		return StageResult{Verdict: pipelinestate.VerdictDefer, Note: fmt.Sprintf(
			"looked at %d of %d cuts so far", len(p.Segments)-pass.Pending, len(p.Segments))}, nil
	}
	// A pass that looked at nothing while cuts wait, on an install that HAS a vision model, met a
	// failing model or a missing derivative: the reel waits for it rather than dropping every cut
	// it could not judge. Without a vision model nothing is waited for, and every unjudged cut is
	// dropped below: no AI, no compilations (#3838 decision 11).
	if pass.Pending > 0 && s.visionWired() {
		return StageResult{Verdict: pipelinestate.VerdictDefer, Note: fmt.Sprintf(
			"waiting for the vision model to look at %s", pluralizeCuts(pass.Pending))}, nil
	}

	// ⚠ The cut is decided HERE, not inside `Propose`. The manual path runs the same Propose from a
	// button an operator just pressed — a human is already present, so cutting under them would take
	// the decision they came to make.
	//
	// ⚠ **PER SEGMENT (V54).** One doubtful cut in 52 drops that cut, never the other 51.
	part := AutoCuttable(*p, s.autoConfirm, s.minClipFloor())
	if part.Reject != AutoSplitOK {
		// A whole-proposal refusal: splitting is off or nothing was detected. The held cuts keep
		// their reason, so the proposal under review says why it was not cut.
		if len(part.Hold) > 0 {
			if err := s.persistHeldReasons(ctx, p.ID, part.Hold); err != nil {
				if errors.Is(err, ErrProposalGone) {
					return StageResult{Verdict: pipelinestate.VerdictContinue, Note: "already resolved"}, nil
				}
				return StageResult{}, err
			}
		}
		note := string(part.Reject)
		if d := discardNote(discarded); d != "" {
			note += "; " + d
		}
		return StageResult{Verdict: pipelinestate.VerdictReview, Note: note}, nil
	}
	// The dropped cuts, in aggregate: "3 of 7 cuts: the cut is not one whole spot". Recorded on the
	// ladder so what a reel lost is readable without a review screen (#3838 decision 10).
	var lost []string
	if held := holdNote(part); held != "" {
		lost = append(lost, held)
	}
	if d := discardNote(discarded); d != "" {
		lost = append(lost, d)
	}
	dropped := strings.Join(lost, "; ")
	if len(part.Confirm) == 0 {
		// Nothing is one clean spot: the reel is settled with nothing cut, never held for a person.
		// It stays a non-airable compilation in the catalog, as a reel whose cuts were all
		// discarded deterministically does.
		p.Segments = nil
		if err := s.splitter.saveProposal(ctx, *p); err != nil {
			return StageResult{}, err
		}
		if err := s.splitter.resolveEmpty(ctx, p.ID); err != nil {
			return StageResult{}, err
		}
		return StageResult{Verdict: pipelinestate.VerdictContinue, Note: "dropped " + dropped}, nil
	}

	// The cuts that are one clean spot are cut; nothing is left behind for a person (no hold).
	spawned, err := s.splitter.ConfirmSome(ctx, p.ID, part.Confirm, nil)
	if err != nil {
		// Returned as an error so the runner retries with backoff; the proposal is intact.
		//
		// ⚠ Exhausting the retries does NOT leave the pipeline row in review: `split` is not in
		// `fatalStages`, so `onFailure` resolves and the row STEPS ONWARD, through rungs the
		// composite guard skips, landing at `complete`.
		return StageResult{}, err
	}
	pipelinestate.ReportProgress(ctx, pipelinestate.StageSplit, 100)

	// ⚠ The segments are SPAWNED, so each one is enrolled at `probe` and runs the whole ladder for
	// itself — transcoded, heard, transcribed, tagged and scored like any other arrival.
	note := fmt.Sprintf("cut into %d adverts", len(spawned))
	if len(spawned) == 1 {
		note = "cut into one advert"
	}
	if dropped != "" {
		note += "; dropped " + dropped
	}
	return StageResult{Verdict: pipelinestate.VerdictContinue, Spawned: spawned, Note: note}, nil
}

// visionWired reports whether this install can look at a cut at all. Without it no cut can be
// judged one whole spot, so no compilation is cut (#3838 decision 11).
func (s *SplitStage) visionWired() bool {
	if s.vision == nil || s.vision.Provider == nil || s.vision.Tools == nil {
		return false
	}
	// The budget is a per-pass setting (filler.pipeline.max_split_vision), not a daily allowance:
	// zero is the operator switching split looks off, never a spent quota.
	return s.vision.Budget == nil || s.vision.Budget() > 0
}

func (s *SplitStage) persistHeldReasons(ctx context.Context, proposalID string, held []split.Segment) error {
	_, err := s.splitter.Reground(ctx, proposalID, held)
	return err
}

// minClipFloor resolves the clip-duration floor, defaulting to zero (no floor check).
func (s *SplitStage) minClipFloor() time.Duration {
	if s.minClipDuration == nil {
		return 0
	}
	return s.minClipDuration()
}

// resumableReviewHashes finds older review rows that the current stage can improve without a
// person: incomplete detection, deterministic discards, or unlooked segments with runnable vision.
func (s *SplitStage) resumableReviewHashes(ctx context.Context) (map[string]struct{}, error) {
	return s.reviewHashes(ctx, false)
}

// uncheckedReviewHashes finds the review rows holding a segment whose duplicate check could not run,
// so a later pass can check it again once the tooling is back.
func (s *SplitStage) uncheckedReviewHashes(ctx context.Context) (map[string]struct{}, error) {
	return s.reviewHashes(ctx, true)
}

func (s *SplitStage) reviewHashes(ctx context.Context, uncheckedOnly bool) (map[string]struct{}, error) {
	proposals, err := s.store.ListSplitProposals(ctx)
	if err != nil {
		return nil, err
	}
	if uncheckedOnly {
		out := make(map[string]struct{})
		for _, p := range proposals {
			if p.Ready() && countDedupUnchecked(p.Segments) > 0 {
				out[p.ClipHash] = struct{}{}
			}
		}
		return out, nil
	}
	autoOn := s.autoConfirm != nil && s.autoConfirm.Enabled != nil && s.autoConfirm.Enabled()
	out := make(map[string]struct{})
	for _, p := range proposals {
		if !p.Ready() {
			out[p.ClipHash] = struct{}{}
			continue
		}
		if !p.ArtworkPrepared && s.splitter != nil && s.splitter.artwork != nil {
			out[p.ClipHash] = struct{}{}
			continue
		}
		kept, discarded := discardDeterministic(p.Segments, s.minClipFloor())
		if len(kept) != len(p.Segments) || discarded.duplicates > 0 || discarded.short > 0 {
			out[p.ClipHash] = struct{}{}
			continue
		}
		if countDedupUnchecked(p.Segments) > 0 {
			out[p.ClipHash] = struct{}{}
			continue
		}
		// With splitting on, the rung decides every reel that has cuts: it cuts, drops or waits for
		// the vision model, and never holds one for a person (#4409). So a reel left in review by
		// the old rule, or one with a cut still to look at, goes back to the rung.
		if autoOn && len(p.Segments) > 0 {
			out[p.ClipHash] = struct{}{}
		}
	}
	return out, nil
}
