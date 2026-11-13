package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
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

// dedup flags segments whose dHash matches an existing catalog clip. Hashing
// failures (undecodable span, unreadable catalog file) mean NO flag — a false
// "already have it" hides a genuinely new advert, which is the worse direction.
//
// ⚠ **The self-exclusion below compares IDENTITIES, and comparing the wrong one broke it.** This
// parameter was named `clipPath` and tested against `c.Path`, while the one call site passes the
// compilation's HASH — so the guard never fired and every segment was compared against the very
// file it was cut from. Any segment resembling its parent came back flagged `DupOf` the
// compilation: noise in the review, and enough to make `AutoConfirmable` reject a perfectly good
// reel. It read as correct because the splitter's test
// store keyed clips by path, so the fixture made `hash` and `path` the same string (§10 V51a).
func (sp *Splitter) dedup(ctx context.Context, file, clipHash string, segs []split.Segment) {
	catalog, err := sp.store.ListClips(ctx)
	if err != nil {
		markDedupUnchecked(segs)
		if sp.log != nil {
			sp.log.Warn("split dedup: the catalog could not be listed; every segment was left unchecked against it",
				"clip", clipHash, "segments", len(segs), "err", err)
		}
		return
	}
	if len(catalog) == 0 {
		return // nothing to be a copy of: that is a finding, not a failure
	}
	type candidate struct {
		path string
		fp   creative.Fingerprint
	}
	var candidates []candidate
	cached, cacheErr := sp.store.ListClipFingerprints(ctx, creative.Algorithm)
	if cacheErr != nil && sp.log != nil {
		sp.log.Warn("catalog fingerprint cache read failed; missing entries will be recomputed", "err", cacheErr)
	}
	others, unreadable := 0, 0
	for _, c := range catalog {
		if c.Hash == clipHash {
			continue // the compilation itself — segments trivially live inside it
		}
		others++
		fp, ok := sp.catalogFingerprint(ctx, c, cached[c.Hash])
		if !ok {
			unreadable++
			continue
		}
		candidates = append(candidates, candidate{path: c.Path, fp: fp})
	}
	if unreadable > 0 && sp.log != nil {
		sp.log.Warn("split dedup: catalog clips could not be fingerprinted and were not compared against",
			"clip", clipHash, "unreadable", unreadable, "catalog", others)
	}
	if len(candidates) == 0 {
		if others > 0 {
			// There were clips to compare against and none could be read: the segments were checked
			// against nothing, which is not the same as being found new.
			markDedupUnchecked(segs)
		}
		return
	}
	// Only a copy is a duplicate. A segment that is a variant of a catalog clip, such as a
	// cut-down, is a separate ad and is kept (#2033).
	unchecked := 0
	var firstErr error
	for i := range segs {
		fp, err := sp.tools.Fingerprint(ctx, file, segs[i].StartMs, segs[i].EndMs)
		if err != nil {
			// The segment keeps no DupOf verdict. It is marked, so the review does not read the
			// empty verdict as "checked and new", and the pass says so once.
			segs[i].DedupUnchecked = true
			unchecked++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, c := range candidates {
			if creative.Compare(fp, c.fp) == creative.Same {
				segs[i].DupOf = c.path
				break
			}
		}
	}
	if unchecked > 0 && sp.log != nil {
		sp.log.Warn("split dedup: segments could not be fingerprinted and were left unchecked against the catalog",
			"clip", clipHash, "unchecked", unchecked, "segments", len(segs), "err", firstErr)
	}
}

// markDedupUnchecked records, on every segment that carries no finding, that the comparison with the
// catalog could not be made. A segment already flagged a copy has its finding and is left alone.
func markDedupUnchecked(segs []split.Segment) {
	for i := range segs {
		if segs[i].DupOf == "" {
			segs[i].DedupUnchecked = true
		}
	}
}

// catalogFingerprint makes whole-catalog dedup incremental. Content hash plus algorithm version
// is the cache key, so a hit is exact; failures always degrade to recomputation or no verdict.
func (sp *Splitter) catalogFingerprint(ctx context.Context, c clipstore.Clip, cached creative.Fingerprint) (creative.Fingerprint, bool) {
	if len(cached.Video) > 0 {
		return cached, true
	}
	fp, err := sp.tools.Fingerprint(ctx, filepath.Join(sp.dropDir, c.Path), 0, c.DurationMs)
	if err != nil || len(fp.Video) == 0 {
		return creative.Fingerprint{}, false
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := sp.store.UpsertClipFingerprint(saveCtx, c.Hash, creative.Algorithm, fp); err != nil && sp.log != nil {
		sp.log.Warn("catalog fingerprint cache write failed", "clip", c.Hash, "err", err)
	}
	return fp, true
}

// Confirm writes the operator's reviewed cut list to the catalog (§10): each kept segment is cut
// with stream copy into the clip folder and becomes a child row. The compilation remains as a
// non-airable composite for lineage and future re-splitting; the proposal is consumed. On a
// re-split, final confirmation tombstones the superseded child generation without deleting bytes.
//
// The segments arrive operator-edited and are re-validated: inside the clip,
// start<end, non-overlapping. Anything else is an error, not a best effort —
// this is the write path, and the review gate is the whole point of the phase.
//
// It returns the HASHES of the clips it created, in cut order (§10 V51b). They are what the
// ingest pipeline enrols, so each segment runs the full ladder for itself; a caller that does not
// need them can ignore the slice.
// Confirm cuts `segments` out of the compilation and files them, deleting the proposal.
//
// The MANUAL path: an operator has finished with this reel and their list is the whole answer.
func (sp *Splitter) Confirm(ctx context.Context, proposalID string, segments []split.Segment) ([]string, error) {
	return sp.confirm(ctx, proposalID, segments, nil, storagegovernor.ConfirmedManual)
}

// ConfirmSome cuts `segments` and leaves `hold` behind in a shrunken proposal (§10 V54).
//
// The AUTOMATIC path: the gate confirmed the cuts it was confident about and kept the rest for a
// human. ⚠ `hold` is what SURVIVES, stated by the caller rather than diffed from the store — see
// the reasoning at the write below.
func (sp *Splitter) ConfirmSome(ctx context.Context, proposalID string, segments, hold []split.Segment) ([]string, error) {
	return sp.confirm(ctx, proposalID, segments, hold, storagegovernor.Automatic)
}

// splitProposalClaimLease is deliberately longer than the bounded local split operation. A crash
// is recoverable after the deadline; a live owner renews immediately before making sidecars or
// media visible, so an expired predecessor is fenced before it can publish.
const splitProposalClaimLease = 30 * time.Minute

// confirm runs the three steps of a confirmation in order: claim the proposal so one confirmer owns
// it, cut the reviewed segments into hidden staging, and publish them (record the children and
// consume or shrink the proposal). Staging and the claim outlive the cut step on purpose: the cut
// bytes are only moved into place by the publish step, and the claim fences a predecessor until
// then.
func (sp *Splitter) confirm(ctx context.Context, proposalID string, segments, hold []split.Segment, storageMode storagegovernor.Mode) ([]string, error) {
	claim, err := sp.claimSplitProposal(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	defer claim.release()
	plan, err := sp.planSplitConfirmation(ctx, proposalID, claim, segments, hold)
	if err != nil {
		return nil, err
	}
	staging := &splitStaging{}
	defer staging.close()
	cuts, err := sp.cutConfirmedSegments(ctx, claim, plan, storageMode, staging)
	if err != nil {
		return nil, err
	}
	return sp.publishConfirmedSegments(ctx, proposalID, claim, plan, cuts)
}

// splitClaim is a confirmer's lease on one proposal: the token that fences it, the proposal as it
// was claimed, and the release that hands the lease back.
type splitClaim struct {
	token    string
	proposal SplitProposal
	release  func()
}

func (sp *Splitter) claimSplitProposal(ctx context.Context, proposalID string) (*splitClaim, error) {
	if sp.newID == nil {
		return nil, errors.New("split confirm: claim token generator is required")
	}
	claimToken := sp.newID()
	if claimToken == "" {
		return nil, errors.New("split confirm: claim token is empty")
	}
	claimedAt := sp.now().UTC()
	p, err := sp.store.AcquireSplitProposalClaim(ctx, proposalID, claimToken, claimedAt, claimedAt.Add(splitProposalClaimLease))
	if err != nil {
		return nil, err
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = sp.store.ReleaseSplitProposalClaim(releaseCtx, proposalID, claimToken)
	}
	return &splitClaim{token: claimToken, proposal: p, release: release}, nil
}

// splitConfirmPlan is a confirmation after validation: the compilation, the source bytes it is cut
// from, and the reviewed segments with the server-owned evidence rebound onto them.
type splitConfirmPlan struct {
	clip     clipstore.Clip
	source   split.SourceAsset
	src      string
	segments []split.Segment
	hold     []split.Segment
}

func (sp *Splitter) planSplitConfirmation(ctx context.Context, proposalID string, claim *splitClaim, segments, hold []split.Segment) (*splitConfirmPlan, error) {
	p := claim.proposal
	if !p.Ready() {
		return nil, fmt.Errorf("%w: proposal %s is still detecting boundaries", ErrSplitValidation, proposalID)
	}
	// Language is detector evidence, not an operator-editable field. The confirm body carries the
	// whole segment shape for one generated contract, but only an exact persisted interval may
	// reuse its answer. A hand-edited or merged interval is a different audio span and deliberately
	// reaches the ordinary post-confirm language rung with no answer.
	segments = bindSplitLanguageEvidence(segments, p.Segments)
	hold = bindSplitLanguageEvidence(hold, p.Segments)
	// Naming provenance is server-authored too. An unchanged exact span recovers the persisted
	// source/model evidence; any renamed, resized, or merged clip keeps the operator's display name
	// but cannot claim the machine's evidence for different content.
	segments = bindSplitNameEvidence(segments, p.Segments)
	hold = bindSplitNameEvidence(hold, p.Segments)
	// Artwork is also server-authored evidence. Preserve it only when an automatically held span
	// still names the exact interval that produced the frame; an edited interval must return to the
	// bounded artwork preparation pass rather than displaying a still from different content.
	hold = bindSplitArtworkEvidence(hold, p.Segments)
	clip, found, err := sp.store.GetClip(ctx, p.ClipHash)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("compilation %s no longer in the catalog", p.ClipHash)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w: zero segments — reject the proposal instead of gutting the compilation", ErrSplitValidation)
	}
	// V66 freezes the exact evidence location and full digest in the proposal. Pre-V66 proposals
	// resolve once through the catalog row and are upgraded to the same bound source shape.
	source, src, err := resolveSplitSource(ctx, sp.dropDir, clip, p.Source)
	if err != nil {
		return nil, fmt.Errorf("split confirm: %w", err)
	}
	if err := validateConfirmedSegments(segments, source.DurationMs, sp.floor()); err != nil {
		return nil, err
	}
	return &splitConfirmPlan{clip: clip, source: source, src: src, segments: segments, hold: hold}, nil
}

// splitStaging collects what the cut step must keep alive until the publish step is done (the
// storage lease, the staging directory, the write monitor) and releases it in reverse order.
type splitStaging struct{ cleanups []func() }

func (s *splitStaging) after(cleanup func()) { s.cleanups = append(s.cleanups, cleanup) }

func (s *splitStaging) close() {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
}

// splitCuts is what the cut step hands the publish step: the prepared cuts and the snapshot of the
// compilation they were cut from.
type splitCuts struct {
	publication splitPublication
	snapshot    string
}

// cutConfirmedSegments reserves storage, snapshots the compilation and cuts every reviewed segment
// into the hidden staging directory. It writes nothing the catalog can see.
func (sp *Splitter) cutConfirmedSegments(ctx context.Context, claim *splitClaim, plan *splitConfirmPlan, storageMode storagegovernor.Mode, staging *splitStaging) (*splitCuts, error) {
	p, clip, source, src, segments := claim.proposal, plan.clip, plan.source, plan.src, plan.segments
	claimToken := claim.token
	var storageLease *storagegovernor.Lease
	if sp.storage != nil {
		budget, ok := storagegovernor.EstimateMedia(storagegovernor.MediaEstimate{
			DeclaredBytes: source.Bytes, DurationMS: source.DurationMs,
		})
		if !ok {
			return nil, storagegovernor.Pause("estimate split storage", storagegovernor.Decision{
				Snapshot: storagegovernor.Snapshot{Domain: storagegovernor.DomainFiller, Reason: storagegovernor.ReasonEstimateUnknown},
			})
		}
		var decision storagegovernor.Decision
		storageLease, decision = sp.storage.Reserve(ctx, storagegovernor.Request{
			Path: src, Domain: storagegovernor.DomainFiller,
			EstimatedBytes: budget.ReservationBytes, Mode: storageMode,
		})
		if storageLease == nil {
			return nil, storagegovernor.Pause("reserve split storage", decision)
		}
		staging.after(storageLease.Release)
		if decision = storageLease.Revalidate(ctx, 0); !decision.Allowed {
			return nil, storagegovernor.Pause("revalidate split storage", decision)
		}
	}
	ext := filepath.Ext(source.Path)
	parentPlayable := filepath.Join(sp.dropDir, filepath.FromSlash(clip.Path))
	// A damaged parent sidecar holds the confirm: absence is ordinary (zero tags), but cutting from
	// unreadable provenance would publish children with no source or acquisition identity.
	parentTags, parentSidecar := asset.ReadSidecarTagsState(parentPlayable)
	if parentSidecar == asset.SidecarInvalid {
		return nil, fmt.Errorf("split confirm: parent sidecar for %s is unreadable; held until it is repaired", clip.Path)
	}

	// Segments are cut inside a hidden directory on the filler filesystem. Same-filesystem staging
	// makes final publication atomic, while ScanDir's dot-directory rule keeps partial bytes out of
	// the catalog. The final-bound lineage sidecar is linked first; only then does the media name
	// appear, so scanning cannot observe an unbound child.
	stageRoot := filepath.Join(sp.dropDir, ".split")
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		return nil, fmt.Errorf("split confirm: create staging root: %w", err)
	}
	tmpDir, err := os.MkdirTemp(stageRoot, "confirm-")
	if err != nil {
		return nil, fmt.Errorf("split confirm: temp dir: %w", err)
	}
	staging.after(func() { _ = os.RemoveAll(tmpDir) })
	splitCtx := ctx
	finishStorage := func() error { return nil }
	if storageLease != nil {
		splitCtx, finishStorage = storagegovernor.MonitorPath(ctx, storageLease, tmpDir, 0)
	}
	staging.after(func() { _ = finishStorage() })
	sourceSnapshot, err := snapshotSplitComposite(splitCtx, src, tmpDir, ext, source.ClipHash)
	if err != nil {
		if storageErr := finishStorage(); storageErr != nil {
			return nil, storageErr
		}
		return nil, err
	}

	// Cut everything FIRST: a failure mid-confirm leaves the compilation intact
	// (the proposal is only consumed once every segment exists on disk and in
	// the catalog), so the operator can fix and retry rather than losing cuts.
	publication := splitPublication{token: claimToken}
	for i, seg := range segments {
		childKind := clip.Role
		structureAuthority, hasStructureAuthority := structureDecisionAuthorityForInterval(p, seg)
		if hasStructureAuthority {
			var mapped bool
			childKind, mapped = catalogRoleForStructureRole(structureAuthority.role)
			if !mapped {
				return nil, fmt.Errorf("split confirm: certified segment %d has no catalog kind", i)
			}
		}
		tmp := filepath.Join(tmpDir, fmt.Sprintf("seg-%03d%s", i, ext))
		if err := sp.tools.Cut(splitCtx, sourceSnapshot, seg.StartMs, seg.EndMs, tmp); err != nil {
			if storageErr := finishStorage(); storageErr != nil {
				return nil, storageErr
			}
			return nil, err
		}
		id, err := asset.ClipID(tmp)
		if err != nil {
			return nil, fmt.Errorf("split confirm: hash segment %d: %w", i, err)
		}
		dst, err := cliplibrary.ClipPath(sp.dropDir, id, ext)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("split confirm: segment %d: %w", i, err)
		}
		childTags := asset.SidecarTags{
			SourceID:              parentTags.SourceID,
			AcquisitionID:         parentTags.AcquisitionID,
			OriginalName:          seg.Name + ext,
			Kind:                  string(childKind),
			SplitPublicationToken: claimToken,
			ConditioningLineage: &asset.ConditioningLineage{
				ChildHash:               id,
				ParentHash:              clip.Hash,
				ParentAssetRole:         string(source.Role),
				ParentAssetSHA256:       source.SHA256,
				StructureDecisionSHA256: structureAuthority.sha256,
				StructureRole:           structureAuthority.role,
				IntendedStartMs:         seg.StartMs,
				IntendedEndMs:           seg.EndMs,
			},
		}
		// The segment's name and where it came from travel beside the bytes, so a catalog rebuild
		// still knows a chapter title or an operator's edit from the filename fallback (#1988).
		// originalName keeps the name too: era grounding reads it as the clip's text.
		if nameSource := segmentNameSource(seg.NameOrigin); nameSource.Known() {
			childTags.NameProvenance = string(nameSource)
			if nameSource.ShownToViewers() {
				childTags.DisplayName = seg.Name
			}
		}
		if err := asset.WriteSidecarTags(tmp, childTags, false); err != nil {
			return nil, fmt.Errorf("split confirm: stage segment provenance: %w", err)
		}
		// ⚠ An existing destination is a byte-identical cut, not a collision: the path IS the
		// hash. The old code guarded this with a `-2`, `-3` suffix loop over operator-shaped
		// names, which content addressing makes both unnecessary and wrong. Reuse is safe only
		// when the existing sidecar names this exact reviewed child/parent/interval; the catalog
		// has one lineage slot and must not silently replace it for identical bytes from elsewhere.
		if _, statErr := os.Stat(dst); errors.Is(statErr, os.ErrNotExist) {
		} else if statErr != nil {
			return nil, fmt.Errorf("split confirm: inspect segment %d: %w", i, statErr)
		} else {
			equal, compareErr := asset.ExactFileBytesEqual(splitCtx, tmp, dst, tools.ConditioningMaxSnapshotBytes)
			if compareErr != nil || !equal {
				return nil, fmt.Errorf("split confirm: existing segment %d bytes do not match identity", i)
			}
			existingTags, ok := asset.ReadSidecarTags(dst)
			if !ok || !reflect.DeepEqual(existingTags.ConditioningLineage, childTags.ConditioningLineage) {
				return nil, fmt.Errorf("split confirm: existing segment %d is not bound to this reviewed cut", i)
			}
			if err := asset.WriteSidecarTags(dst, childTags, false); err != nil {
				return nil, fmt.Errorf("split confirm: fence existing segment %d: %w", i, err)
			}
			publication.cuts = append(publication.cuts, preparedSplitCut{segment: seg, kind: childKind, hash: id, path: cliplibrary.ClipRelPath(id, ext), staged: tmp, final: dst, existing: true})
			continue
		}
		publication.cuts = append(publication.cuts, preparedSplitCut{segment: seg, kind: childKind, hash: id, path: cliplibrary.ClipRelPath(id, ext), staged: tmp, final: dst})
	}
	if err := finishStorage(); err != nil {
		return nil, err
	}
	return &splitCuts{publication: publication, snapshot: sourceSnapshot}, nil
}

// publishConfirmedSegments moves the cut segments into place and records them: it fences the
// source, publishes the staged files, files each child in the catalog (held until the generation
// commits), and then either consumes the proposal or shrinks it to the segments still held for a
// person. It returns the hashes of the clips it created.
func (sp *Splitter) publishConfirmedSegments(ctx context.Context, proposalID string, claim *splitClaim, plan *splitConfirmPlan, cuts *splitCuts) ([]string, error) {
	p, clip, src, hold, claimToken := claim.proposal, plan.clip, plan.src, plan.hold, claim.token
	publication := cuts.publication
	if err := validateSplitCompositeOwnership(ctx, src, cuts.snapshot); err != nil {
		return nil, fmt.Errorf("split confirm: composite source changed while cuts were prepared: %w", err)
	}
	if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
		return nil, err
	}
	if err := publication.prepare(ctx); err != nil {
		publication.rollback()
		return nil, err
	}
	defer publication.rollback()
	now := sp.now().UTC()
	// ⚠ The hashes are RETURNED, not merely written. The ingest pipeline enrols each one at
	// `probe` so a fresh segment runs the whole ladder for itself (§10 V51b) — before this, a cut
	// advert had to wait for whichever of six catalog sweeps happened to reach it next.
	spawned := make([]string, 0, len(publication.cuts))
	for _, c := range publication.cuts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		spawned = append(spawned, c.hash)
		nc := clipstore.Clip{UpdatedAt: now}
		// ⚠ **The identity must be set.** `UpsertClip` is `ON CONFLICT(hash) DO UPDATE`, so a
		// segment inserted with `hash=''` overwrites the last one — a 41-segment reel would become
		// ONE catalog row. Invisible to the splitter's own tests because their store keys its map
		// on `Path`; the store conformance suite pins this class.
		nc.Hash = c.hash
		nc.Path = c.path
		nc.Name, nc.NameSource = c.segment.Name, segmentNameSource(c.segment.NameOrigin)
		nc.Role = c.kind
		nc.DurationMs = c.segment.EndMs - c.segment.StartMs
		nc.Era = c.segment.Era
		nc.Audience = c.segment.Audience
		// ⚠ Lineage: each segment points back at the composite it was cut from (§10 V45). This is what
		// V45 keeps that V34 discarded — provenance ("which break did this advert air in?"), a
		// re-split when detection improves, and broadcast-context inheritance. `clip.Hash` is the
		// composite's identity (the parent is kept, below, not deleted).
		nc.ParentHash = clip.Hash
		// A reviewed child is held and tombstoned before its media name is published. The pipeline
		// may process it, while ReplaceSplitChildren remains the atomic generation selector.
		nc.Held = true
		nc.RemovedAt = now
		// Persist the transcript the rescue step already produced (§10 V44). Pre-V44 this was
		// computed to find ad boundaries and then thrown away; it is the richest metadata signal a
		// split segment has — a segment with no source description still SAYS its brand — so it
		// carries onto the clip row instead of being re-derived by the transcribe job later.
		nc.Transcript = c.segment.Transcript
		// Reuse a completed per-segment language answer. The child starts again at probe, and the
		// ordinary language rung treats a non-empty code (including `none`) as already heard.
		// Checked-but-unknown stays empty so that defense rung may try again on the rendered child.
		if c.segment.LanguageChecked {
			nc.Language = c.segment.Language
		}
		// Provenance inherits from the compilation: same source, same declared
		// licence (the segments ARE the source's content), same resolution.
		nc.Source = clip.Source
		nc.License = clip.License
		nc.Quality = clip.Quality
		if err := sp.store.UpsertClip(ctx, nc); err != nil {
			return nil, err
		}
		// A segment's guessed era is carried onto the child as an uncited Context suggestion, not
		// a clip fact; the store skips it when the child already has a verified era.
		if c.segment.Era == 0 {
			if err := sp.store.RecordFillerEraObservation(ctx, c.hash, c.segment.SuggestedEra,
				research.ObservedBySplit, now); err != nil {
				return nil, err
			}
		}
		// Persist grounded proposal tags now that the cut has a stable content hash. Union with an
		// identical existing clip so deduplication never lets AI erase operator-authored knowledge.
		segmentTags := append([]string(nil), c.segment.Tags...)
		if len(segmentTags) == 0 && c.segment.Category != "" {
			// Upgrade compatibility for proposals stored before the wire carried the tag set.
			segmentTags = append(segmentTags, c.segment.Category)
		}
		if len(segmentTags) > 0 {
			existing, err := sp.store.GetClipTags(ctx, c.hash, true)
			if err != nil {
				return nil, err
			}
			if err := sp.store.SetClipTags(ctx, c.hash, unionLeaves(existing, segmentTags)); err != nil {
				return nil, err
			}
		}
		if err := sp.store.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
			ClipHash: c.hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now,
			PreparationAttempt: 1, PreparationStartedAt: now,
			PreparationStartReason: pipelinestate.PreparationStartedByEnrollment, PreparationProgress: 0,
			StageQueuedAt: now,
		}); err != nil {
			return nil, err
		}
	}
	// ⚠ V45 KEEPS the parent, marking it a composite — it does NOT delete the row or the file (the
	// reversal of V34). The composite is not airable (pod assembly excludes `is_composite`), so it
	// harms nothing by staying, and keeping it is what makes the segments' `parent_hash` resolve, a
	// re-split possible, and the recorded break's provenance answerable. Keyed by the composite's
	// HASH (its identity), not its path.
	//
	// The file stays on disk too: a re-scan finds it, but UpsertClip omits `is_composite` from its
	// DO UPDATE list, so the re-synced row keeps its composite mark rather than reverting to airable.
	// ⚠ **`hold` decides whether the proposal survives, and the CALLER supplies it — it is never
	// inferred from `segments`** (§10 V54).
	//
	// The two callers mean different things by the same cut list. An operator clicking Confirm is
	// saying "this reel is finished, cut exactly this", and their list is EDITED — spans retyped,
	// segments merged — so nothing in it need match what was stored. The gate is saying "cut these,
	// keep the rest for a human". Diffing the stored segments against the confirmed ones would read
	// the operator's edits as leftovers and resurrect a reel they had just finished.
	currentGeneration := appendUniqueStrings(p.Spawned, spawned...)
	if len(hold) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The review disposition held the parent out of every catalog read. Once the proposal is
		// fully consumed, file that parent so it can appear as the lineage container around the
		// generated clips. It remains non-airable because SetClipComposite above is the catalog's
		// independent airability chokepoint. Keep partial proposals held: their replacement
		// generation is not committed yet and Incoming still owns the decision.
		if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
			return nil, err
		}
		if err := publication.publish(ctx); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := sp.store.CompleteSplitConfirmation(ctx, SplitCompletion{
			ProposalID:     proposalID,
			ClaimToken:     claimToken,
			ParentHash:     clip.Hash,
			ChildHashes:    currentGeneration,
			ActivateHashes: spawned,
			At:             now,
		}); err != nil {
			return nil, err
		}
		publication.retain()
		return spawned, nil
	}
	if err := sp.store.SetClipComposite(ctx, clip.Hash, true, now); err != nil {
		return nil, err
	}
	// Renumber so the review's "#N" runs 1..n rather than showing gaps where the confirmed cuts
	// were. Names are untouched: a reel confirmed over two sittings must not rename its
	// remaining clips underneath the operator.
	remaining := append([]split.Segment(nil), hold...)
	for i := range remaining {
		remaining[i].Index = i
	}
	p.Segments = remaining
	p.ArtworkPrepared = splitArtworkPrepared(remaining)
	p.Spawned = currentGeneration
	if err := sp.store.RenewSplitProposalClaim(ctx, proposalID, claimToken, sp.now().UTC().Add(splitProposalClaimLease)); err != nil {
		return nil, err
	}
	if err := publication.publish(ctx); err != nil {
		return nil, err
	}
	if err := sp.store.CompletePartialSplitConfirmation(ctx, SplitPartialCompletion{
		Proposal: p, ClaimToken: claimToken, ActivateHashes: spawned, At: now,
	}); err != nil {
		return nil, err
	}
	publication.retain()
	return spawned, nil
}

func bindSplitLanguageEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		out[i].Language = ""
		out[i].LanguageChecked = false
		out[i].LanguageReason = ""
		out[i].LanguageNote = ""
		original, ok := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if !ok {
			continue
		}
		out[i].Language = original.Language
		out[i].LanguageChecked = original.LanguageChecked
		out[i].LanguageReason = original.LanguageReason
		out[i].LanguageNote = original.LanguageNote
	}
	return out
}

func bindSplitNameEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		original, exactSpan := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if exactSpan && out[i].Name == original.Name {
			out[i].NameOrigin = original.NameOrigin
			out[i].NameEvidence = original.NameEvidence
			continue
		}
		out[i].NameOrigin = SplitNameOperatorEdited
		out[i].NameEvidence = ""
	}
	return out
}

func bindSplitArtworkEvidence(submitted, persisted []split.Segment) []split.Segment {
	type span struct{ start, end int64 }
	bySpan := make(map[span]split.Segment, len(persisted))
	for _, segment := range persisted {
		bySpan[span{segment.StartMs, segment.EndMs}] = segment
	}
	out := append([]split.Segment(nil), submitted...)
	for i := range out {
		out[i].ArtworkImageHash = ""
		out[i].ArtworkChecked = false
		original, ok := bySpan[span{out[i].StartMs, out[i].EndMs}]
		if !ok {
			continue
		}
		out[i].ArtworkImageHash = original.ArtworkImageHash
		out[i].ArtworkChecked = original.ArtworkChecked
	}
	return out
}

func splitArtworkPrepared(segments []split.Segment) bool {
	for _, segment := range segments {
		if !segment.ArtworkChecked {
			return false
		}
	}
	return true
}

func appendUniqueStrings(existing []string, values ...string) []string {
	out := append([]string(nil), existing...)
	seen := make(map[string]struct{}, len(out)+len(values))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// validateConfirmedSegments enforces the invariants the write path needs:
// inside the clip, start<end, ordered and non-overlapping.
//
// ⚠ The floor here is the COMPOSED one, so the manual path refuses what the scan boundary would
// refuse. A bare `MinSegmentMs` (3s) would let a hand-drawn 8s cut be written, spawned, and then
// rejected `too_short` by the probe rung — a silent downstream loss the operator would never
// connect to their edit. Refusing it upfront is arguable; losing it quietly is not.
func validateConfirmedSegments(segs []split.Segment, durationMs int64, floor segmentFloor) error {
	sorted := append([]split.Segment(nil), segs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	for i, s := range sorted {
		if s.StartMs < 0 || s.EndMs > durationMs || !floor.admits(s.StartMs, s.EndMs) {
			return fmt.Errorf("%w: segment %d [%d,%d) outside the clip or under %dms (filler.min_duration)", ErrSplitValidation, i, s.StartMs, s.EndMs, floor.ms())
		}
		if i > 0 && s.StartMs < sorted[i-1].EndMs {
			return fmt.Errorf("%w: segments %d and %d overlap — two clips cannot share seconds", ErrSplitValidation, i-1, i)
		}
	}
	return nil
}

// ⚠ `uniqueClipPath` and `sanitizeClipName` were DELETED in V51a, and their absence is the point.
// They existed to turn a segment's display name into a collision-free filename
// ("McDonald's — 1987!" → `mcdonalds-1987.mp4`, then `-2`, `-3` on collision). Under content
// addressing a segment's location IS its hash, so there is no name to sanitise and no collision
// to break: two cuts that produce the same bytes are the same clip and belong in one row. The
// segment's name still travels — on the catalog row, where it is read by humans rather than by
// the filesystem.

// sliceTranscript keeps the utterances overlapping [startMs,endMs) — the text a
// rescued sub-segment is classified from.
func sliceTranscript(transcript []tools.TranscriptSegment, startMs, endMs int64) []tools.TranscriptSegment {
	var out []tools.TranscriptSegment
	for _, t := range transcript {
		if t.EndMs > startMs && t.StartMs < endMs {
			out = append(out, t)
		}
	}
	return out
}
