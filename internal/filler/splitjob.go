package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"path/filepath"

	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// ErrSplitValidation marks a rejected confirm edit (out-of-clip bounds,
// overlaps, zero segments, slivers) so the API can render 422 rather than 500 —
// the operator's edit was wrong, not the server.
var ErrSplitValidation = errors.New("invalid split segments")

// ErrProposalGone means the proposal was confirmed or rejected while a rung was working on it.
//
// ⚠ A DOMAIN sentinel, translated from the store's ErrNotFound by the adapter, because
// `internal/filler` is the pure domain and does not import `internal/platform/store` (Tier 3). It exists
// for one race: split-time grounding is a read-modify-write spanning minutes of vision calls, and
// a `Confirm` landing inside that window must not be undone by the write that follows.
var ErrProposalGone = errors.New("filler: the split proposal was resolved while it was being grounded")

// ErrProposalClaimed means another process owns the reviewed proposal's publication lease.
// Confirm callers must leave the proposal and filesystem untouched and retry after that owner
// releases the lease or its durable deadline passes.
var ErrProposalClaimed = errors.New("filler: the split proposal is already being confirmed")

// ErrSplitParentNotHeld means the proposal's parent is not held for split review: it is already
// filed (a playable clip, or a composite whose cuts were confirmed), so the proposal cannot be
// confirmed (#1761). It is distinct from a missing proposal, which the API answers with 404.
var ErrSplitParentNotHeld = errors.New("filler: the split's parent clip is already filed, not held for split review")

// ErrConditioningOwnershipMismatch means durable conditioning publication evidence no longer
// belongs to the catalog transition it claims. This is a safe review outcome; infrastructure
// failures must remain errors so the pipeline retries them.
var ErrConditioningOwnershipMismatch = errors.New("filler: conditioning publication ownership mismatch")

// The splitter (§10, V34): turns a compilation clip into a REVIEWED set of
// clips. Propose runs detection and persists a SplitProposal; Confirm writes
// the operator's edited cut list to the catalog and removes the compilation.
// NOTHING enters the catalog from Propose — review is not optional, because
// detection quality is a property of the source (measured 69–100%, plan §6.4).

// SplitStore is the slice of the store the splitter needs (mirrors sync.go's
// pattern — declared here so filler doesn't import store; app bridges them).
type SplitStore interface {
	GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error)
	ListClips(ctx context.Context) ([]clipstore.Clip, error) // the dedup candidate set
	ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error)
	UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error
	UpsertClip(ctx context.Context, c clipstore.Clip) error
	// RecordFillerEraObservation carries a segment's guessed era onto its child as an uncited
	// Context suggestion (#2323).
	RecordFillerEraObservation(ctx context.Context, clipHash string, era int, source research.ObservationSource, at time.Time) error
	GetClipTags(ctx context.Context, clipHash string, leavesOnly bool) ([]string, error)
	SetClipTags(ctx context.Context, clipHash string, leaves []string) error
	// UpsertClipPipeline durably enrolls a reviewed child before its media name is published.
	UpsertClipPipeline(ctx context.Context, row pipelinestate.ClipPipeline) error
	// ReplaceSplitChildren atomically makes keepHashes the airable generation for a parent.
	// Superseded rows are tombstoned, never deleted; channel-pinned children are retained.
	ReplaceSplitChildren(ctx context.Context, parentHash string, keepHashes []string, at time.Time) (int, error)
	// SetClipComposite marks the parent as a composite on confirm (§10 V45) — the parent is KEPT,
	// not deleted, so its segments can point back at it and a re-split stays possible.
	SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error
	// ReleaseCompositeHolds exposes the fully resolved parent as a non-airable lineage container.
	ReleaseCompositeHolds(ctx context.Context, paths []string, at time.Time) (int, error)
	// MarkPipelineComplete makes full confirmation the terminal owner of the parent pipeline row.
	// It belongs inside the confirmation saga so the application cannot fail after commit.
	MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error
	// CompleteSplitConfirmation atomically consumes one fully reviewed proposal, transitions its
	// retained parent and replacement pipelines, and selects the replacement generation.
	CompleteSplitConfirmation(ctx context.Context, completion SplitCompletion) (int, error)
	// ListTaxa is the taxonomy path (§10 V45a): classify serves this vocabulary to the model and
	// grounds the answer against it.
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	UpsertSplitProposal(ctx context.Context, p SplitProposal) error
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
	GetSplitProposal(ctx context.Context, id string) (SplitProposal, error)
	AcquireSplitProposalClaim(ctx context.Context, id, token string, at, expiresAt time.Time) (SplitProposal, error)
	RenewSplitProposalClaim(ctx context.Context, id, token string, expiresAt time.Time) error
	ReleaseSplitProposalClaim(ctx context.Context, id, token string) error
	DeleteSplitProposal(ctx context.Context, id string) error
	// UpdateSplitProposal writes grounding and partial-confirm state onto an existing proposal without
	// re-detecting. Must NOT insert — a write landing after Confirm would resurrect the proposal.
	UpdateSplitProposal(ctx context.Context, p SplitProposal) error
	CompletePartialSplitConfirmation(ctx context.Context, completion SplitPartialCompletion) error
}

// SplitCompletion is the closed durable commit for one fully reviewed split generation. Media and
// sidecars are already published reversibly; every field here becomes visible in one transaction.
type SplitCompletion struct {
	ProposalID     string
	ClaimToken     string
	ParentHash     string
	ChildHashes    []string
	ActivateHashes []string
	At             time.Time
}

// SplitPartialCompletion is the durable boundary for one partial reviewed generation. Its claim
// token fences the proposal document update after reversible media publication.
type SplitPartialCompletion struct {
	Proposal       SplitProposal
	ClaimToken     string
	ActivateHashes []string
	At             time.Time
}

// Splitter runs compilation splitting. provider may be nil: rescue and
// classification then simply don't run (over-long segments come out
// Unsplittable and tags empty) — an install without an LLM still gets the
// coarse split, which is the honest degradation.
type Splitter struct {
	store    SplitStore
	tools    tools.MediaTools
	provider llm.Provider
	dropDir  string // FILLER_DIR — clip paths are relative to it (§10)
	// minClipDuration is `filler.min_duration`, read live so it hot-applies. Composed with
	// MinSegmentMs into the detection floor — see segmentFloor for why that composition exists.
	minClipDuration func() time.Duration
	now             func() time.Time
	newID           func() string
	log             *slog.Logger
	storage         *storagegovernor.Governor
	segmentLanguage *SegmentLanguagePolicy
	artwork         SplitArtworkWriter
}

// SplitArtworkWriter adopts one representative JPEG into the shared image service. The splitter
// owns when and from which exact span the bytes were extracted; the adapter owns image storage,
// responsive renditions, and proposal-reference lifetime.
type SplitArtworkWriter interface {
	IngestSplitArtwork(ctx context.Context, proposalID string, segment split.Segment, jpeg []byte) (string, error)
}

// SegmentLanguagePolicy applies the intake languages (language.IntakeSet) to each detected span before
// the split becomes reviewable. Budget is the existing per-pass whisper allowance, not another user
// knob.
type SegmentLanguagePolicy struct {
	Detector language.Detector
	Want     func(context.Context) (string, error)
	Budget   func() int
}

// NewSplitter builds the splitter. dropDir is the filler drop-folder root. minClipDuration may be
// nil, which leaves MinSegmentMs as the only floor.
func NewSplitter(store SplitStore, tools tools.MediaTools, provider llm.Provider, dropDir string, minClipDuration func() time.Duration, newID func() string, now func() time.Time, log *slog.Logger) *Splitter {
	if now == nil {
		now = time.Now
	}
	return &Splitter{store: store, tools: tools, provider: provider, dropDir: dropDir, minClipDuration: minClipDuration, newID: newID, now: now, log: log}
}

// WithStorageGovernor protects the peak split working set: the immutable source snapshot, every
// staged cut, and the published children coexist until the catalog transaction commits.
func (sp *Splitter) WithStorageGovernor(governor *storagegovernor.Governor) *Splitter {
	if sp != nil {
		sp.storage = governor
	}
	return sp
}

// WithSegmentLanguage attaches the intake language check to proposal preparation. Keeping
// it on Splitter makes scheduled and explicitly requested proposals use the same path.
func (sp *Splitter) WithSegmentLanguage(policy SegmentLanguagePolicy) *Splitter {
	if sp != nil {
		sp.segmentLanguage = &policy
	}
	return sp
}

// WithSplitArtwork attaches the presentation-only representative-frame sink. Missing artwork is
// an honest unavailable state and never changes cut or admission authority.
func (sp *Splitter) WithSplitArtwork(writer SplitArtworkWriter) *Splitter {
	if sp != nil {
		sp.artwork = writer
	}
	return sp
}

// splitArtworkPerPass bounds image work independently of reel size. It is implementation policy,
// not a user-facing knob: a long proposal resumes through the existing split rung.
const splitArtworkPerPass = 12

func (sp *Splitter) prepareArtwork(ctx context.Context, p SplitProposal) (SplitProposal, int, error) {
	if p.ArtworkPrepared {
		return p, 0, nil
	}
	// A splitter without an image sink (unit tests and deliberately partial embeddings) degrades
	// in one step. It must not make presentation support a prerequisite for split progress.
	if sp.artwork == nil || sp.tools == nil {
		for i := range p.Segments {
			p.Segments[i].ArtworkChecked = true
		}
		p.ArtworkPrepared = true
		if err := sp.saveProposal(ctx, p); err != nil {
			return p, 0, err
		}
		return p, 0, nil
	}
	clip, found, err := sp.store.GetClip(ctx, p.ClipHash)
	if err != nil {
		return p, 0, err
	}
	if !found {
		return p, 0, fmt.Errorf("clip %s not found", p.ClipHash)
	}
	_, file, err := resolveSplitSource(ctx, sp.dropDir, clip, p.Source)
	if err != nil {
		return p, 0, err
	}

	attempted, unavailable := 0, 0
	for i := range p.Segments {
		if p.Segments[i].ArtworkChecked || attempted >= splitArtworkPerPass {
			continue
		}
		attempted++
		segment := &p.Segments[i]
		segment.ArtworkChecked = true
		frames, frameErr := sp.tools.KeyframesIn(ctx, file, segment.StartMs, segment.EndMs, 1)
		if frameErr != nil || len(frames) == 0 {
			unavailable++
			continue
		}
		hash, ingestErr := sp.artwork.IngestSplitArtwork(ctx, p.ID, *segment, frames[0])
		if ingestErr != nil || hash == "" {
			unavailable++
			continue
		}
		segment.ArtworkImageHash = hash
	}

	pending := 0
	for i := range p.Segments {
		if !p.Segments[i].ArtworkChecked {
			pending++
		}
	}
	p.ArtworkPrepared = pending == 0
	if err := sp.saveProposal(ctx, p); err != nil {
		return p, pending, err
	}
	if sp.log != nil && attempted > 0 {
		sp.log.Info("split: prepared representative frames", "proposal", p.ID,
			"prepared", attempted-unavailable, "unavailable", unavailable, "pending", pending)
	}
	return p, pending, nil
}

func (sp *Splitter) startSegmentLanguage(ctx context.Context, p *SplitProposal) (bool, error) {
	policy := sp.segmentLanguage
	if policy == nil || policy.Detector == nil || policy.Want == nil {
		return false, nil
	}
	raw, err := policy.Want(ctx)
	if err != nil {
		// Not "no language": that would present every segment as reviewable under no policy.
		return false, fmt.Errorf("read the channels' commercial languages: %w", err)
	}
	want := language.NormalizeSet(raw)
	if want == "" {
		return false, nil
	}
	p.LanguagePreference = want
	p.Language = &SplitLanguageProgress{Want: p.LanguagePreference}
	return true, nil
}

func (sp *Splitter) advanceSegmentLanguage(ctx context.Context, file string, p *SplitProposal) (bool, error) {
	policy := sp.segmentLanguage
	if p.Language == nil {
		return true, nil
	}
	if policy == nil || policy.Detector == nil {
		p.Language = nil
		return true, nil
	}
	limit := len(p.Segments) - p.Language.Next
	if policy.Budget != nil {
		limit = max(0, min(limit, policy.Budget()))
	}
	unavailable := policy.Detector.UnavailableReason()
	if unavailable != "" || limit == 0 {
		note := unavailable
		reason := SplitLanguageUnavailable
		if note == "" {
			note = "Language checks are paused by the processing limit"
			reason = SplitLanguagePaused
		}
		for i := p.Language.Next; i < len(p.Segments); i++ {
			p.Segments[i].LanguageChecked = true
			p.Segments[i].LanguageReason = reason
			p.Segments[i].LanguageNote = note
		}
		p.Language.Next = len(p.Segments)
	} else {
		endIndex := min(len(p.Segments), p.Language.Next+limit)
		for i := p.Language.Next; i < endIndex; i++ {
			segment := &p.Segments[i]
			segment.LanguageChecked = true
			start, end := language.Span(segment.EndMs - segment.StartMs)
			detected, err := policy.Detector.DetectLanguage(ctx, file, segment.StartMs+start, min(segment.EndMs, segment.StartMs+end))
			if err != nil {
				if ctx.Err() != nil {
					return false, err
				}
				segment.LanguageReason = SplitLanguageFailed
				segment.LanguageNote = "Language could not be checked"
				continue
			}
			segment.Language = language.Normalize(detected)
			if segment.Language == language.Undetermined {
				segment.LanguageReason = SplitLanguageInconclusive
			}
		}
		p.Language.Next = endIndex
	}
	if p.Language.Next < len(p.Segments) {
		return false, nil
	}
	want := p.Language.Want
	kept := make([]split.Segment, 0, len(p.Segments))
	for _, segment := range p.Segments {
		if language.Rejects(segment.Language, want) {
			p.LanguageExclusions = append(p.LanguageExclusions, SplitLanguageExclusion{
				StartMs:          segment.StartMs,
				EndMs:            segment.EndMs,
				Name:             segment.Name,
				DetectedLanguage: segment.Language,
				ExpectedLanguage: want,
				Reason:           SplitExclusionLanguageMismatch,
			})
			continue
		}
		kept = append(kept, segment)
	}
	for i := range kept {
		kept[i].Index = i
	}
	p.Segments = kept
	p.Language = nil
	return true, nil
}

// Reground writes a grounding pass back onto an existing proposal WITHOUT re-detecting (§10 V54).
//
// ⚠ Re-detection is the operator's call, never a rung's: `Propose` replaces the whole cut list, and
// a scheduled job redrawing cuts a human may be looking at is the hazard the split stage's
// pending-proposal check exists to prevent. This writes only what the grounder learned.
//
// The store update refuses to insert, so a proposal confirmed or rejected while this pass was
// grounding surfaces as ErrNotFound rather than being resurrected — the caller treats that as
// "resolved under us", which it is.
func (sp *Splitter) Reground(ctx context.Context, proposalID string, grounded []split.Segment) (SplitProposal, error) {
	current, err := sp.store.GetSplitProposal(ctx, proposalID)
	if err != nil {
		return SplitProposal{}, err
	}
	current.Segments = mergeGrounding(current.Segments, grounded)
	if current.Structure != nil {
		structure, structureErr := reassessProposalStructure(current, sp.now().UTC())
		if structureErr != nil {
			return SplitProposal{}, fmt.Errorf("reground source structure: %w", structureErr)
		}
		current.Structure = &structure
	}
	if err := sp.store.UpdateSplitProposal(ctx, current); err != nil {
		return SplitProposal{}, err
	}
	return current, nil
}

// mergeGrounding copies the grounding and review-decision fields from `from` onto the segments of
// `onto` that still describe the SAME span.
//
// ⚠ Matched on the span, not the index. Today nothing can reorder a proposal between the read and
// the write (there is no PATCH route), so this is belt-and-braces — but it is the invariant that
// keeps the merge correct if one is ever added, and index-matching would silently stamp the wrong
// segment the day it is.
func mergeGrounding(onto, from []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	stamps := make(map[span]split.Segment, len(from))
	for _, s := range from {
		stamps[span{s.StartMs, s.EndMs}] = s
	}
	out := append([]split.Segment(nil), onto...)
	for i := range out {
		g, ok := stamps[span{out[i].StartMs, out[i].EndMs}]
		if !ok {
			continue
		}
		out[i].Looked = out[i].Looked || g.Looked
		if g.Category != "" {
			out[i].Category = g.Category
		}
		if len(g.Tags) > 0 {
			out[i].Tags = unionLeaves(out[i].Tags, g.Tags)
		}
		if g.Era > 0 {
			out[i].Era = g.Era
		}
		if g.RoleEvidence != nil {
			roleEvidence := split.CloneRoleEvidence(*g.RoleEvidence)
			out[i].RoleEvidence = &roleEvidence
		}
		if out[i].NameOrigin == SplitNameFallback && g.NameOrigin == SplitNameModelProposed {
			if name, evidence, ok := groundSegmentName(g.Name, g.NameEvidence); ok {
				out[i].Name = name
				out[i].NameOrigin = g.NameOrigin
				out[i].NameEvidence = evidence
			}
		}
		// Unlike learned tags, an empty reason is meaningful: a later pass may have supplied the
		// evidence that clears an earlier hold. Always copy it so stale explanations cannot survive.
		out[i].HoldReason = g.HoldReason
	}
	return out
}

// floor resolves the detection floor ONCE for a call.
//
// ⚠ Resolve it into a local and thread the VALUE, never the closure. `filler.min_duration`
// hot-applies, and a floor that changed between `triage` and `rescue` would produce a cut list
// that is internally inconsistent — some spans admitted under the old number, some under the new.
func (sp *Splitter) floor() segmentFloor {
	if sp.minClipDuration == nil {
		return newSegmentFloor(0)
	}
	return newSegmentFloor(sp.minClipDuration())
}

// boundaryScanChunkMs is an implementation budget rather than another operator setting. A
// ten-minute span keeps each decode bounded while making multi-hour recordings resumable.
const boundaryScanChunkMs int64 = 10 * 60 * 1000

// Propose detects cuts in one compilation clip and persists the resulting
// proposal (§10). The catalog is untouched.
// ⚠ Takes the clip HASH, not its path — `GetClip` below is keyed `WHERE hash = ?`, and a path
// handed to a hash-keyed call fails silently (the class that shipped the V41 tagger bug). The API
// passes `clipID`; so does the scheduled runner.
func (sp *Splitter) Propose(ctx context.Context, clipHash string) (*SplitProposal, error) {
	var p *SplitProposal
	pending, err := sp.store.ListSplitProposals(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pending {
		if pending[i].ClipHash == clipHash && !pending[i].Ready() {
			p = &pending[i]
			break
		}
	}
	for {
		var done bool
		p, done, err = sp.advanceProposal(ctx, clipHash, p)
		if err != nil {
			return nil, err
		}
		if done {
			for p.Ready() && !p.ArtworkPrepared {
				pValue, pending, artworkErr := sp.prepareArtwork(ctx, *p)
				if artworkErr != nil {
					return nil, artworkErr
				}
				p = &pValue
				if pending == 0 {
					break
				}
			}
			return p, nil
		}
	}
}

// advanceProposal performs one durable unit of work. The scheduled pipeline invokes one unit per
// pass; the explicit operator path loops under its longer request context.
func (sp *Splitter) advanceProposal(ctx context.Context, clipHash string, p *SplitProposal) (*SplitProposal, bool, error) {
	clip, found, err := sp.store.GetClip(ctx, clipHash)
	if err != nil {
		return p, false, err
	}
	if !found {
		return p, false, fmt.Errorf("clip %s not found", clipHash)
	}
	if clip.DurationMs <= 0 {
		return p, false, fmt.Errorf("clip %s has no probed duration — sync the catalog first", clipHash)
	}
	sourceWasUnbound := p != nil && p.Source == (split.SourceAsset{})
	var boundSource split.SourceAsset
	if p != nil {
		boundSource = p.Source
	}
	source, file, err := resolveSplitSource(ctx, sp.dropDir, clip, boundSource)
	if err != nil {
		return p, false, err
	}
	durationMs := source.DurationMs
	floor := sp.floor()

	if p == nil {
		p = &SplitProposal{ID: sp.newID(), ClipHash: clip.Hash, CreatedAt: sp.now().UTC(), Source: source, Detection: &split.DetectionProgress{}}
		chapters, chapterErr := sp.tools.Chapters(ctx, file)
		if chapterErr != nil && sp.log != nil {
			sp.log.Warn("chapter triage failed, falling back to coarse split", "file", file, "err", chapterErr)
		}
		if segs, dropped := segmentsFromChapters(chapters, floor); len(segs) > 0 {
			p.Detection.ScannedThroughMs = durationMs
			p.Detection.Chapters = true
			p.Detection.ChapterEdges = split.ChapterEdges(chapters)
			p.Detection.CoarseSegments = segs
			p.Detection.Discarded = append([]tools.Interval(nil), dropped.Spans...)
			p.Dropped = dropped
			if err := sp.saveProposal(ctx, *p); err != nil {
				return p, false, err
			}
			return p, false, nil
		}
	} else {
		if p.ClipHash != clipHash {
			return p, false, fmt.Errorf("split proposal %s belongs to %s, not %s", p.ID, p.ClipHash, clipHash)
		}
		if sourceWasUnbound {
			p.Source = source
			if err := sp.saveProposal(ctx, *p); err != nil {
				return p, false, err
			}
		}
		if p.Detection == nil && p.Language != nil {
			done, languageErr := sp.advanceSegmentLanguage(ctx, file, p)
			if languageErr != nil {
				return p, false, languageErr
			}
			if err := sp.saveProposal(ctx, *p); err != nil {
				return p, false, err
			}
			return p, done, nil
		}
		if p.Ready() {
			return p, true, nil
		}
	}

	if len(p.Detection.CoarseSegments) == 0 {
		start := max(p.Detection.ScannedThroughMs, 0)
		if start < durationMs {
			end := min(start+boundaryScanChunkMs, durationMs)
			black, silence, detectErr := sp.tools.Boundaries(ctx, file, start, end)
			if detectErr != nil {
				if ctx.Err() != nil {
					return p, false, detectErr
				}
				if sp.log != nil {
					sp.log.Warn("boundary detection failed, falling back to transcript rescue of the whole file",
						"file", file, "startMs", start, "endMs", end, "err", detectErr)
				}
				p.Detection.ScannedThroughMs = durationMs
				p.Detection.CoarseSegments, p.Dropped = segmentsFromBoundaries(durationMs, nil, floor)
				p.Detection.Discarded = append([]tools.Interval(nil), p.Dropped.Spans...)
			} else {
				p.Detection.Black = append(p.Detection.Black, black...)
				p.Detection.Silence = append(p.Detection.Silence, silence...)
				p.Detection.ScannedThroughMs = end
			}
		}
		if p.Detection.ScannedThroughMs >= durationMs && len(p.Detection.CoarseSegments) == 0 {
			gaps := sourcedGaps(p.Detection.Black, p.Detection.Silence)
			p.Detection.CoarseSegments, p.Dropped = segmentsFromBoundaries(durationMs, gaps, floor)
			p.Detection.Discarded = append([]tools.Interval(nil), p.Dropped.Spans...)
			if len(p.Detection.CoarseSegments) == 0 {
				return p, false, fmt.Errorf("no usable segments detected in %s (everything was under %dms — filler.min_duration)", clipHash, floor.ms())
			}
		}
		if err := sp.saveProposal(ctx, *p); err != nil {
			return p, false, err
		}
		return p, false, nil
	}

	// Coarse segments crossed the durable JSON checkpoint before this pass. Their private source
	// bitmasks are intentionally not part of split.Segment's API shape, so derive them again from
	// the detection facts that ARE persisted before the confidence ladder reads them. Without this
	// restore every resumed boundary scored 0 and even black+silence agreement could never clear
	// the default auto-split threshold.
	restoreCoarseBoundarySources(p.Detection, durationMs)
	segs := append([]split.Segment(nil), p.Detection.CoarseSegments...)

	// 2. Names for the unnamed (chapters bring their own).
	base := strings.TrimSuffix(clip.Name, filepath.Ext(clip.Name))
	for i := range segs {
		if segs[i].Name == "" {
			segs[i].Name = fallbackSegmentName(base, i+1)
			segs[i].NameOrigin = SplitNameFallback
		}
	}

	// 3. Rescue: over-long segments go to transcript + LLM — the only signal
	// that sees a boundary with no black frame and no silence. Failure modes all
	// land on Unsplittable, never on a guessed cut.
	segs = sp.rescue(ctx, file, base, segs, floor)

	// 4. Split does not classify (§10 V51g).
	//
	// ⚠ **Classifying here would be one LLM turn per segment inside a bounded pass** — 51 segments
	// × 7.4s ≈ 377s on a 16m47s reel, so the rung could never finish. It would also be worse
	// duplicate work: `split.Segment.Transcript` is EMPTY unless `rescue` ran (and rescue only
	// transcribes segments over ~120s), so `Classify` would see nothing but a generated name,
	// `"… part 7"`, while every spawned segment runs `transcribe` → `tag` anyway with a real
	// transcript.
	//
	// V69's bounded display-name proposal is the narrow descriptive exception: it reuses exact-span
	// vision/transcript evidence and carries no admission authority. Each segment is spawned as its
	// own clip and runs the whole ladder for itself — one clip at a time, budget-bounded,
	// resumable, and individually visible in Incoming. Classification happens where the scheduler
	// can hold it.
	//
	// ⚠ The cost: the split-review screen shows cuts without tags. That is the right trade — that
	// screen asks "are these the right cuts?", which an operator answers from the filmstrip and the
	// timings. "What is each one?" is a different question, asked later, per clip, once there is a
	// transcript to answer it from.

	// 5. Dedup against the catalog — a FLAG on the proposal, never a silent drop.
	sp.dedup(ctx, file, clipHash, segs)

	for i := range segs {
		segs[i].Index = i
	}
	// ⚠ ONE scoring pass, ONE writer, and it runs LAST — after rescue has redrawn spans and dedup
	// has flagged duplicates, so every fact the ladder reads is final. Scoring earlier would
	// stamp numbers the rest of Propose then invalidates (§10 V34).
	scoreBoundaries(segs)
	structure, err := split.AssessDetection(p.Source, *p.Detection, segs, sp.now().UTC())
	if err != nil {
		return p, false, fmt.Errorf("assess source structure: %w", err)
	}
	// ⚠ The proposal carries the compilation's HASH, not its path. Confirm looks the clip back up
	// with `GetClip`, which is hash-keyed — storing `clip.Path` here would make that lookup never
	// match, so no split could ever be committed. The file location Confirm needs is derived from
	// the hash, so there is one identity and nothing to disagree with it.
	p.Segments = segs
	p.Structure = &structure
	p.Detection = nil
	languagePending, err := sp.startSegmentLanguage(ctx, p)
	if err != nil {
		return p, false, err
	}
	if p.Dropped.Count > 0 && sp.log != nil {
		// INFO, not WARN: discarding sub-floor fragments is the design working, not a fault. It is
		// logged because it costs recording time the operator can otherwise only infer from
		// arithmetic (§10 V45).
		sp.log.Info("split: fragments under the clip floor discarded",
			"clip", clipHash, "dropped", p.Dropped.Count, "droppedMs", p.Dropped.Ms,
			"floorMs", floor.ms(), "segments", len(segs))
	}
	if err := sp.store.UpsertSplitProposal(ctx, *p); err != nil {
		return p, false, err
	}
	return p, !languagePending, nil
}

// restoreCoarseBoundarySources rebuilds the private scoring inputs after a checkpoint round trip.
// Black/silence intervals are the authoritative detector facts; chapter segments are the only
// coarse segments with names before the resume pass, and a nameless whole-reel fallback has only
// the two reel edges. Deriving instead of serialising the bitmask keeps implementation evidence
// out of the operator-facing split.Segment schema while making restarts lossless.
func restoreCoarseBoundarySources(progress *split.DetectionProgress, durationMs int64) {
	if progress == nil || len(progress.CoarseSegments) == 0 {
		return
	}

	cuts := boundaryCuts(sourcedGaps(progress.Black, progress.Silence), durationMs)
	if len(cuts) == 0 {
		chapterAuthored := progress.Chapters || len(progress.CoarseSegments) > 1
		for i := range progress.CoarseSegments {
			if strings.TrimSpace(progress.CoarseSegments[i].Name) != "" {
				chapterAuthored = true
				break
			}
		}
		for i := range progress.CoarseSegments {
			if chapterAuthored {
				progress.CoarseSegments[i].StartSrc = split.SrcChapter
				progress.CoarseSegments[i].EndSrc = split.SrcChapter
				continue
			}
			if progress.CoarseSegments[i].StartMs == 0 {
				progress.CoarseSegments[i].StartSrc = split.SrcReelEdge
			}
			if progress.CoarseSegments[i].EndMs == durationMs {
				progress.CoarseSegments[i].EndSrc = split.SrcReelEdge
			}
		}
		return
	}

	sources := map[int64]split.BoundarySource{0: split.SrcReelEdge, durationMs: split.SrcReelEdge}
	for _, cut := range cuts {
		sources[cut.Ms] |= cut.Src
	}
	for i := range progress.CoarseSegments {
		progress.CoarseSegments[i].StartSrc = sources[progress.CoarseSegments[i].StartMs]
		progress.CoarseSegments[i].EndSrc = sources[progress.CoarseSegments[i].EndMs]
	}
}

func sourcedGaps(blacks, silences []tools.Interval) []detectedGap {
	gaps := make([]detectedGap, 0, len(blacks)+len(silences))
	for _, b := range blacks {
		gaps = append(gaps, detectedGap{Interval: b, Src: split.SrcBlack})
	}
	for _, s := range silences {
		gaps = append(gaps, detectedGap{Interval: s, Src: split.SrcSilence})
	}
	return gaps
}

func (sp *Splitter) saveProposal(ctx context.Context, p SplitProposal) error {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return sp.store.UpsertSplitProposal(saveCtx, p)
}

func (sp *Splitter) resolveEmpty(ctx context.Context, proposalID string) error {
	p, err := sp.store.GetSplitProposal(ctx, proposalID)
	if err != nil {
		return err
	}
	if len(p.Segments) != 0 {
		return fmt.Errorf("%w: refusing to resolve non-empty proposal %s as discarded", ErrSplitValidation, proposalID)
	}
	clip, found, err := sp.store.GetClip(ctx, p.ClipHash)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("compilation %s no longer in the catalog", p.ClipHash)
	}
	now := sp.now().UTC()
	if err := sp.store.SetClipComposite(ctx, p.ClipHash, true, now); err != nil {
		return err
	}
	// Deterministically discarding every candidate is also a terminal resolution. The parent is
	// still the useful catalog record of the reel, so expose it as a non-airable composite instead
	// of leaving it hidden behind a hold for a proposal that no longer exists.
	if _, err := sp.store.ReleaseCompositeHolds(ctx, []string{clip.Path}, now); err != nil {
		return err
	}
	return sp.store.DeleteSplitProposal(ctx, proposalID)
}

// rescue replaces over-long segments with their transcript-derived sub-segments
// where the rescue succeeds, and marks them Unsplittable where it cannot run.
func (sp *Splitter) rescue(ctx context.Context, file, base string, segs []split.Segment, floor segmentFloor) []split.Segment {
	var out []split.Segment
	for _, seg := range segs {
		if !overlong(seg) {
			out = append(out, seg)
			continue
		}
		if sp.provider == nil {
			seg.Unsplittable = true
			out = append(out, seg)
			continue
		}
		transcript, err := sp.tools.Transcribe(ctx, file, seg.StartMs, seg.EndMs)
		if err != nil {
			if sp.log != nil {
				sp.log.Warn("transcribe failed; segment left unsplittable", "file", file, "startMs", seg.StartMs, "err", err)
			}
			seg.Unsplittable = true
			out = append(out, seg)
			continue
		}
		seg.Transcript = TranscriptText(transcript)
		spans, err := findAdBreaks(ctx, sp.provider, transcript, seg.EndMs-seg.StartMs, floor)
		if err != nil {
			if sp.log != nil {
				sp.log.Warn("boundary rescue found nothing usable; segment left unsplittable", "file", file, "startMs", seg.StartMs, "err", err)
			}
			seg.Unsplittable = true
			out = append(out, seg)
			continue
		}
		if len(spans) == 1 {
			// ⚠ The load-bearing case (measured, plan §6.4): a 121s infomercial for
			// ONE product must stay ONE segment. Without the single-advert prompt
			// rule the model invents cuts at round timestamps, manufacturing clips
			// that were never adverts.
			//
			// ⚠ It also CLEARS the over-long cap rather than adding points (§10 V34). The model
			// saying "this is one advert" is the fact that defeats "over-long means a missed
			// boundary" — and removing a cap is the only legal move a corroboration has in a
			// ceiling ladder. The segment's own two boundaries keep whatever found them.
			seg.RescueConfirmedWhole = true
			if seg.NameOrigin == SplitNameFallback {
				seg.Name, seg.NameOrigin, seg.NameEvidence = proposedSegmentName(
					spans[0].Product, seg.Transcript, base, len(out)+1,
				)
			}
			out = append(out, seg)
			continue
		}
		for _, s := range spans {
			transcriptText := TranscriptText(sliceTranscript(transcript, s.StartMs, s.EndMs))
			name, nameOrigin, nameEvidence := proposedSegmentName(s.Product, transcriptText, base, len(out)+1)
			sub := split.Segment{
				StartMs:      seg.StartMs + s.StartMs,
				EndMs:        seg.StartMs + s.EndMs,
				Name:         name,
				NameOrigin:   nameOrigin,
				NameEvidence: nameEvidence,
				Transcript:   transcriptText,
				// ⚠ Both edges are the TRANSCRIPT's, even where a sub-segment happens to abut the
				// parent's own boundary: the rescue redrew this span, so the parent's evidence no
				// longer describes it. Measured at ±2–3s, which is why its ceiling is the lowest
				// of the detected sources.
				StartSrc: split.SrcTranscript,
				EndSrc:   split.SrcTranscript,
			}
			out = append(out, sub)
		}
	}
	return out
}

// `classify` is DELETED (§10 V51g), not merely unwired — an orphaned method reads as a capability
// the next person can switch back on, and this one must not come back to this file. The tagging it
// did happens on each spawned segment's own `tag` rung, with a transcript, one clip at a time.
