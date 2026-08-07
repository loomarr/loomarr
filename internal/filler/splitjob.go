package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// ErrSplitValidation marks a rejected confirm edit (out-of-clip bounds,
// overlaps, zero segments, slivers) so the API can render 422 rather than 500 —
// the operator's edit was wrong, not the server.
var ErrSplitValidation = errors.New("invalid split segments")

// The splitter (§10, V34): turns a compilation clip into a REVIEWED set of
// clips. Propose runs detection and persists a SplitProposal; Confirm writes
// the operator's edited cut list to the catalog and removes the compilation.
// NOTHING enters the catalog from Propose — review is not optional, because
// detection quality is a property of the source (measured 69–100%, plan §6.4).

// SplitStore is the slice of the store the splitter needs (mirrors sync.go's
// pattern — declared here so filler doesn't import store; app bridges them).
type SplitStore interface {
	GetClip(ctx context.Context, id string) (StoreClip, bool, error)
	ListClips(ctx context.Context) ([]StoreClip, error) // the dedup candidate set
	UpsertClip(ctx context.Context, c StoreClip) error
	// SetClipComposite marks the parent as a composite on confirm (§10 V45) — the parent is KEPT,
	// not deleted, so its segments can point back at it and a re-split stays possible.
	SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error
	// ListTaxa is the taxonomy path (§10 V45a): classify serves this vocabulary to the model and
	// grounds the answer against it. ⚠ The split job does NOT call SetClipTags — a segment clip has
	// no content hash at confirm time (it gets one from the intake pipeline when its cut file is
	// hashed), and SetClipTags is hash-keyed, so persisting here would orphan the tag rows. The
	// segment carries its grounded Tags on the proposal for the reviewer; the TAG JOB persists the
	// taxonomy tags once the segment file is intaken and hashed — the one hash-keyed tag writer.
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	UpsertSplitProposal(ctx context.Context, p SplitProposal) error
	GetSplitProposal(ctx context.Context, id string) (SplitProposal, error)
	DeleteSplitProposal(ctx context.Context, id string) error
}

// Splitter runs compilation splitting. provider may be nil: rescue and
// classification then simply don't run (over-long segments come out
// Unsplittable and tags empty) — an install without an LLM still gets the
// coarse split, which is the honest degradation.
type Splitter struct {
	store    SplitStore
	tools    MediaTools
	provider llm.Provider
	dropDir  string // FILLER_DIR — clip paths are relative to it (§10)
	now      func() time.Time
	newID    func() string
	log      *slog.Logger
}

// NewSplitter builds the splitter. dropDir is the filler drop-folder root.
func NewSplitter(store SplitStore, tools MediaTools, provider llm.Provider, dropDir string, newID func() string, now func() time.Time, log *slog.Logger) *Splitter {
	if now == nil {
		now = time.Now
	}
	return &Splitter{store: store, tools: tools, provider: provider, dropDir: dropDir, newID: newID, now: now, log: log}
}

// Propose detects cuts in one compilation clip and persists the resulting
// proposal (§10). The catalog is untouched.
// ⚠ Takes the clip HASH, not its path — `GetClip` below is keyed `WHERE hash = ?`. The
// parameter was named `clipPath` until V43, a leftover from the pre-V38c identity, and that
// naming is the exact class that shipped the V41 tagger bug (a path handed to a hash-keyed
// call, failing silently). The API passes `clipID`; so does the scheduled runner.
func (sp *Splitter) Propose(ctx context.Context, clipHash string) (*SplitProposal, error) {
	clip, found, err := sp.store.GetClip(ctx, clipHash)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("clip %s not found", clipHash)
	}
	if clip.DurationMs <= 0 {
		return nil, fmt.Errorf("clip %s has no probed duration — sync the catalog first", clipHash)
	}
	// ⚠ The LOCATION comes from the row, not from the argument. The argument is an identity
	// (a hash); joining it onto the drop dir would build a path that does not exist. They were
	// the same string before V38c, which is why one variable used to serve both.
	file := filepath.Join(sp.dropDir, clip.Path)

	// 1. Triage → coarse split. Chapters split for free; a chapter read failure
	// is not fatal (the common case is NO chapters anyway — 6 of 8 measured).
	segs := sp.triage(ctx, file, clip.DurationMs)
	if len(segs) == 0 {
		return nil, fmt.Errorf("no usable segments detected in %s (everything was under %dms)", clipHash, MinSegmentMs)
	}

	// 2. Names for the unnamed (chapters bring their own).
	base := strings.TrimSuffix(clip.Name, filepath.Ext(clip.Name))
	for i := range segs {
		if segs[i].Name == "" {
			segs[i].Name = fmt.Sprintf("%s part %d", base, i+1)
		}
	}

	// 3. Rescue: over-long segments go to transcript + LLM — the only signal
	// that sees a boundary with no black frame and no silence. Failure modes all
	// land on Unsplittable, never on a guessed cut.
	segs = sp.rescue(ctx, file, base, segs)

	// 4. Classify each segment through the SAME tagger the tag job uses (§10) —
	// including the era grounding rule: an era whose year is not in the text
	// arrives as SuggestedEra only.
	sp.classify(ctx, segs)

	// 5. Dedup against the catalog — a FLAG on the proposal, never a silent drop.
	sp.dedup(ctx, file, clipHash, segs)

	for i := range segs {
		segs[i].Index = i
	}
	// ⚠ `ClipPath` is a PATH, per its name and per what Confirm joins onto the drop dir.
	p := &SplitProposal{ID: sp.newID(), ClipPath: clip.Path, CreatedAt: sp.now().UTC(), Segments: segs}
	if err := sp.store.UpsertSplitProposal(ctx, *p); err != nil {
		return nil, err
	}
	return p, nil
}

// triage runs chapters-first, then the black/silence coarse split.
func (sp *Splitter) triage(ctx context.Context, file string, durationMs int64) []SplitSegment {
	chapters, err := sp.tools.Chapters(ctx, file)
	if err != nil && sp.log != nil {
		sp.log.Warn("chapter triage failed, falling back to coarse split", "file", file, "err", err)
	}
	if segs := segmentsFromChapters(chapters); len(segs) > 0 {
		return segs
	}
	blacks, silences, err := sp.tools.BlackSilence(ctx, file)
	if err != nil {
		if sp.log != nil {
			sp.log.Warn("black/silence detection failed, treating the file as one segment", "file", file, "err", err)
		}
		// A detector failure is not "no boundaries": the whole file is one segment,
		// which (being a compilation) is over-long and goes to rescue — or comes
		// back Unsplittable. Guessing cuts would be worse.
		return segmentsFromBoundaries(durationMs, nil)
	}
	return segmentsFromBoundaries(durationMs, append(blacks, silences...))
}

// rescue replaces over-long segments with their transcript-derived sub-segments
// where the rescue succeeds, and marks them Unsplittable where it cannot run.
func (sp *Splitter) rescue(ctx context.Context, file, base string, segs []SplitSegment) []SplitSegment {
	var out []SplitSegment
	for _, seg := range segs {
		if !seg.overlong() {
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
		spans, err := findAdBreaks(ctx, sp.provider, transcript, seg.EndMs-seg.StartMs)
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
			out = append(out, seg)
			continue
		}
		for _, s := range spans {
			sub := SplitSegment{
				StartMs:    seg.StartMs + s.StartMs,
				EndMs:      seg.StartMs + s.EndMs,
				Name:       subSegmentName(s.Product, base, len(out)+1),
				Transcript: TranscriptText(sliceTranscript(transcript, s.StartMs, s.EndMs)),
			}
			out = append(out, sub)
		}
	}
	return out
}

// classify tags every segment with the existing tagger. A classify failure
// leaves the segment untagged — the review shows that, and the tag job can
// retry after confirm.
//
// ⚠ Loads the taxonomy ONCE (§10 V45a) and serves/grounds every segment against it — the same forest
// the tag job uses, so a split segment and a directly-tagged clip are grounded identically. A ListTaxa
// failure leaves the forest nil, which grounds no tags (segments come out untagged) rather than
// failing the whole split — the honest degradation, matching the nil-provider case.
func (sp *Splitter) classify(ctx context.Context, segs []SplitSegment) {
	if sp.provider == nil {
		return
	}
	taxa, err := sp.store.ListTaxa(ctx)
	if err != nil && sp.log != nil {
		sp.log.Warn("split classify: could not load taxonomy; segments will be untagged", "err", err)
	}
	forest := taxonomy.New(taxa)
	for i := range segs {
		sug, err := Classify(ctx, sp.provider, forest, segs[i].Name, segs[i].Transcript)
		if err != nil {
			if sp.log != nil {
				sp.log.Warn("segment classify failed", "segment", segs[i].Name, "err", err)
			}
			continue
		}
		segs[i].Era = sug.Era
		segs[i].SuggestedEra = sug.SuggestedEra
		segs[i].Audience = sug.Audience
		segs[i].Tags = sug.Tags
		segs[i].Category = sug.Category
	}
}

// dedup flags segments whose dHash matches an existing catalog clip. Hashing
// failures (undecodable span, unreadable catalog file) mean NO flag — a false
// "already have it" hides a genuinely new advert, which is the worse direction.
func (sp *Splitter) dedup(ctx context.Context, file, clipPath string, segs []SplitSegment) {
	catalog, err := sp.store.ListClips(ctx)
	if err != nil || len(catalog) == 0 {
		return
	}
	type hashed struct {
		path   string
		hashes []uint64
	}
	var candidates []hashed
	for _, c := range catalog {
		if c.Path == clipPath {
			continue // the compilation itself — segments trivially live inside it
		}
		frames, err := sp.tools.GrayFrames(ctx, filepath.Join(sp.dropDir, c.Path), 0, c.DurationMs)
		if err != nil {
			continue
		}
		candidates = append(candidates, hashed{path: c.Path, hashes: dHashFrames(frames)})
	}
	if len(candidates) == 0 {
		return
	}
	for i := range segs {
		frames, err := sp.tools.GrayFrames(ctx, file, segs[i].StartMs, segs[i].EndMs)
		if err != nil {
			continue
		}
		h := dHashFrames(frames)
		for _, c := range candidates {
			if mean, ok := meanHamming(h, c.hashes); ok && mean <= DupHashThreshold {
				segs[i].DupOf = c.path
				break
			}
		}
	}
}

// Confirm writes the operator's reviewed cut list to the catalog (§10): each
// kept segment is cut with stream copy into the drop-folder and becomes a clip
// row; the compilation's file AND row are removed — its identity is a path that
// now means twenty clips, not one. The proposal is consumed.
//
// The segments arrive operator-edited and are re-validated: inside the clip,
// start<end, non-overlapping. Anything else is an error, not a best effort —
// this is the write path, and the review gate is the whole point of the phase.
func (sp *Splitter) Confirm(ctx context.Context, proposalID string, segments []SplitSegment) error {
	p, err := sp.store.GetSplitProposal(ctx, proposalID)
	if err != nil {
		return err
	}
	clip, found, err := sp.store.GetClip(ctx, p.ClipPath)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("compilation %s no longer in the catalog", p.ClipPath)
	}
	if len(segments) == 0 {
		return fmt.Errorf("%w: zero segments — reject the proposal instead of gutting the compilation", ErrSplitValidation)
	}
	if err := validateConfirmedSegments(segments, clip.DurationMs); err != nil {
		return err
	}

	src := filepath.Join(sp.dropDir, p.ClipPath)
	dir := filepath.Dir(p.ClipPath)
	ext := filepath.Ext(p.ClipPath)
	// Cut everything FIRST: a failure mid-confirm leaves the compilation intact
	// (the proposal is only consumed once every segment exists on disk and in
	// the catalog), so the operator can fix and retry rather than losing cuts.
	type written struct {
		seg  SplitSegment
		path string
	}
	var cuts []written
	for _, seg := range segments {
		rel, err := sp.uniqueClipPath(ctx, dir, sanitizeClipName(seg.Name), ext)
		if err != nil {
			return err
		}
		out := filepath.Join(sp.dropDir, rel)
		if err := sp.tools.Cut(ctx, src, seg.StartMs, seg.EndMs, out); err != nil {
			return err
		}
		cuts = append(cuts, written{seg: seg, path: rel})
	}
	now := sp.now().UTC()
	for _, c := range cuts {
		nc := StoreClip{UpdatedAt: now}
		nc.Path = c.path
		nc.Name = c.seg.Name
		nc.Kind = clip.Kind
		nc.DurationMs = c.seg.EndMs - c.seg.StartMs
		nc.Era = c.seg.Era
		nc.SuggestedEra = c.seg.SuggestedEra
		nc.Audience = c.seg.Audience
		nc.Category = c.seg.Category
		// ⚠ Lineage: each segment points back at the composite it was cut from (§10 V45). This is what
		// V45 keeps that V34 discarded — provenance ("which break did this advert air in?"), a
		// re-split when detection improves, and broadcast-context inheritance. `clip.Hash` is the
		// composite's identity (the parent is kept, below, not deleted).
		nc.ParentHash = clip.Hash
		// Persist the transcript the rescue step already produced (§10 V44). Pre-V44 this was
		// computed to find ad boundaries and then thrown away; it is the richest metadata signal a
		// split segment has — a segment with no source description still SAYS its brand — so it
		// carries onto the clip row instead of being re-derived by the transcribe job later.
		nc.Transcript = c.seg.Transcript
		// Provenance inherits from the compilation: same source, same declared
		// licence (the segments ARE the source's content), same resolution.
		nc.Source = clip.Source
		nc.License = clip.License
		nc.Quality = clip.Quality
		nc.Rating = clip.Rating
		// The tags came from AI classification — operator-REVIEWED, but the AI
		// flag records origin, not approval (a manual PATCH still clears it). Includes the taxonomy
		// tag set (§10 V45a): a segment grounded only on a non-product axis (`psa`, `christmas`) has
		// an empty Category shadow but real tags, and is still AI-tagged.
		nc.AITagged = c.seg.Era > 0 || c.seg.Audience != "" || c.seg.Category != "" || len(c.seg.Tags) > 0
		if err := sp.store.UpsertClip(ctx, nc); err != nil {
			return err
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
	if err := sp.store.SetClipComposite(ctx, clip.Hash, true, now); err != nil {
		return err
	}
	return sp.store.DeleteSplitProposal(ctx, proposalID)
}

// validateConfirmedSegments enforces the invariants the write path needs:
// inside the clip, start<end, ordered and non-overlapping.
func validateConfirmedSegments(segs []SplitSegment, durationMs int64) error {
	sorted := append([]SplitSegment(nil), segs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	for i, s := range sorted {
		if s.StartMs < 0 || s.EndMs > durationMs || s.EndMs-s.StartMs < MinSegmentMs {
			return fmt.Errorf("%w: segment %d [%d,%d) outside the clip or under %dms", ErrSplitValidation, i, s.StartMs, s.EndMs, MinSegmentMs)
		}
		if i > 0 && s.StartMs < sorted[i-1].EndMs {
			return fmt.Errorf("%w: segments %d and %d overlap — two clips cannot share seconds", ErrSplitValidation, i-1, i)
		}
	}
	return nil
}

// uniqueClipPath picks a collision-free relative path for a new segment clip.
func (sp *Splitter) uniqueClipPath(ctx context.Context, dir, name, ext string) (string, error) {
	for i := 1; ; i++ {
		candidate := name + ext
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d%s", name, i, ext)
		}
		rel := filepath.Join(dir, candidate)
		if dir == "." {
			rel = candidate
		}
		if _, err := os.Stat(filepath.Join(sp.dropDir, rel)); err == nil {
			continue
		}
		if _, found, err := sp.store.GetClip(ctx, rel); err != nil {
			return "", err
		} else if found {
			continue
		}
		return rel, nil
	}
}

// sanitizeClipName makes a proposed segment name safe as a filename while
// keeping it displayable ("McDonald's — 1987!" → "mcdonalds-1987").
// Apostrophes VANISH rather than becoming dashes — "mcdonald-s" is not a word.
func sanitizeClipName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if r == '\'' || r == '’' {
			continue
		}
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		switch {
		case ok:
			b.WriteRune(r)
			lastDash = false
		case !lastDash && b.Len() > 0:
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "segment"
	}
	return out
}

// subSegmentName prefers the LLM's product label, falling back to the
// compilation-part convention when the model said "unknown".
func subSegmentName(product, base string, n int) string {
	if product == "" || strings.EqualFold(product, "unknown") {
		return fmt.Sprintf("%s part %d", base, n)
	}
	return product
}

// sliceTranscript keeps the utterances overlapping [startMs,endMs) — the text a
// rescued sub-segment is classified from.
func sliceTranscript(transcript []TranscriptSegment, startMs, endMs int64) []TranscriptSegment {
	var out []TranscriptSegment
	for _, t := range transcript {
		if t.EndMs > startMs && t.StartMs < endMs {
			out = append(out, t)
		}
	}
	return out
}
