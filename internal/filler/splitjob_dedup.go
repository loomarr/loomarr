package filler

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// dedup flags segments whose dHash matches an existing catalog clip. Hashing
// failures (undecodable span, unreadable catalog file) mean NO DupOf flag — a false
// "already have it" hides a genuinely new advert, which is the worse direction — and the segment
// is marked DedupUnchecked instead, so a comparison made against only part of the catalog never
// reads as complete.
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
		sp.log.Warn("split dedup: catalog clips could not be fingerprinted, so segments without a copy were left unchecked",
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
	if unreadable > 0 {
		// A copy found among the readable clips stands. Every other segment was compared with only
		// part of the catalog, so its empty verdict would read as "checked and new" while the clips
		// it was not compared with could still hold its copy. It is marked, held back from
		// unattended confirmation, and re-checked by RecheckDedup once those clips can be read.
		markDedupUnchecked(segs)
	}
}

// countDedupUnchecked counts the segments whose catalog comparison has not been made.
func countDedupUnchecked(segs []split.Segment) int {
	n := 0
	for i := range segs {
		if segs[i].DedupUnchecked && segs[i].DupOf == "" {
			n++
		}
	}
	return n
}

// RecheckDedup re-runs the catalog comparison for the proposal's unchecked segments, so a segment
// held back because the dedup tooling was down is released once it is back. A segment that is now
// checked loses its mark (a copy gets its DupOf); one that still cannot be checked keeps it. The
// proposal is saved only when something changed. It returns how many segments are still unchecked.
//
// A lookup failure is logged and leaves the segments unchecked, never checked: a failed re-check
// must not read as a clean result.
func (sp *Splitter) RecheckDedup(ctx context.Context, p *SplitProposal) (int, error) {
	var idx []int
	for i := range p.Segments {
		if p.Segments[i].DedupUnchecked && p.Segments[i].DupOf == "" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return 0, nil
	}
	clip, found, err := sp.store.GetClip(ctx, p.ClipHash)
	if err == nil && !found {
		err = errors.New("clip not found")
	}
	var file string
	if err == nil {
		_, file, err = resolveSplitSource(ctx, sp.dropDir, clip, p.Source)
	}
	if err != nil {
		if sp.log != nil {
			sp.log.Warn("split dedup re-check: the source could not be resolved; segments stay unchecked",
				"clip", p.ClipHash, "unchecked", len(idx), "err", err)
		}
		return len(idx), nil
	}
	sub := make([]split.Segment, len(idx))
	for j, i := range idx {
		sub[j] = p.Segments[i]
		sub[j].DedupUnchecked = false
	}
	sp.dedup(ctx, file, p.ClipHash, sub)
	remaining, changed := 0, false
	for j, i := range idx {
		if sub[j].DedupUnchecked {
			remaining++
			continue
		}
		p.Segments[i] = sub[j]
		changed = true
	}
	if changed {
		if err := sp.saveProposal(ctx, *p); err != nil {
			return remaining, err
		}
	}
	return remaining, nil
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
