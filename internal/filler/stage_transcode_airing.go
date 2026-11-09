package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// The library re-encode (#2121, decisions 8, 9 and 11): a clip already on air is re-encoded from
// its retained source master onto the current profile WITHOUT leaving the air. Its file keeps
// airing while the replacement is encoded, verified and measured in the hidden staging folder;
// only a replacement that passes is published, and the switch is one transaction. A replacement
// that fails for any reason (an encode error, QC, a defect verdict) is discarded, and the clip
// airs on as it was.
//
// No content check runs again. A top-level clip's safety, rights and enrichment verdicts are of
// its source, which the re-encode does not change, and the re-key carries them to the new bytes.
// The defect floor does run again, because it judges the bytes that air.

// ReplacementCheck measures the staged bytes that would replace an airing clip's file
// (fillerdefectfloor's CheckReplacement). Bytes it admits switch with the record it returns.
type ReplacementCheck func(ctx context.Context, c StoreClip, staged string) (defects.Record, error)

// AiringReplacementStore commits an airing clip's switch to its re-encoded bytes.
type AiringReplacementStore interface {
	// ReplaceAiringClip re-keys the clip like ReplaceClipIdentity and records the replacement's
	// defect verdict under the new hash, in one transaction.
	ReplaceAiringClip(ctx context.Context, oldHash string, c StoreClip, defects defects.Record) error
}

// ErrReplacementRefused is a re-encode that ran but did not pass; the airing file stays.
var ErrReplacementRefused = errors.New("the re-encode did not pass, so the airing file stays")

// WithAiringReplacement enables ReplaceAiring.
func (s *TranscodeStage) WithAiringReplacement(store AiringReplacementStore, check ReplacementCheck) *TranscodeStage {
	if s != nil {
		s.airingStore, s.airingCheck = store, check
	}
	return s
}

// NeedsReencode reports whether c is an airing clip the library re-encode replaces: a top-level
// clip on air whose playback was encoded from a retained source master, under a profile other
// than the current one. A clip whose playback file is its own download has no original to encode
// from again, and a split child is cut from its parent, so neither is replaced. Nor is anything
// replaced on a host without a hardware encoder: the re-encode is what moves the library onto the
// GPU, and on the CPU it would spend the machine for no saving.
func (s *TranscodeStage) NeedsReencode(c StoreClip) bool {
	if s == nil || s.airingStore == nil || s.airingCheck == nil || s.evidenceTranscode == nil ||
		c.Held || !c.RemovedAt.IsZero() || c.IsComposite || c.ParentHash != "" || c.Path == "" {
		return false
	}
	profile := s.target().profile
	if profile.Family == "" {
		return false
	}
	tags, ok := asset.ReadSidecarTags(filepath.Join(s.clipDir, filepath.FromSlash(c.Path)))
	return ok && tags.MediaAssets != nil && tags.MediaAssets.Playback != nil && tags.Mezzanine != profile.ID()
}

// ReplaceAiring re-encodes an airing clip that NeedsReencode and switches it to the new bytes,
// returning the clip under its new identity. Any error leaves it airing as it was; a replacement
// that ran and did not pass is ErrReplacementRefused.
func (s *TranscodeStage) ReplaceAiring(ctx context.Context, c StoreClip) (StoreClip, error) {
	if !s.NeedsReencode(c) {
		return StoreClip{}, fmt.Errorf("re-encode %s: not an airing clip with an original to encode from", c.Path)
	}
	swap := &airingSwap{store: s.airingStore, check: s.airingCheck}
	if tags, ok := asset.ReadSidecarTags(filepath.Join(s.clipDir, filepath.FromSlash(c.Path))); ok {
		swap.oldEvidence = derivativePath(tags.MediaAssets.Evidence)
	}
	res, err := s.run(ctx, c, swap)
	if err != nil {
		return StoreClip{}, err
	}
	if !swap.committed {
		return StoreClip{}, fmt.Errorf("re-encode %s: the profile changed while it ran; nothing was replaced", c.Path)
	}
	return res.Clip, nil
}

// airingSwap is one ReplaceAiring run's state through the transcode.
type airingSwap struct {
	store AiringReplacementStore
	check ReplacementCheck
	// record is the defect verdict the staged bytes earned, set once admit passes them.
	record defects.Record
	// oldEvidence is the airing clip's evidence copy, when the defect floor has not released it.
	oldEvidence string
	committed   bool
}

// admit is the gate between the verified encode and its publication: the media-quality report
// and the defect floor must both pass the staged bytes.
func (w *airingSwap) admit(ctx context.Context, c StoreClip, staged, newHash string, quality MediaQuality) error {
	if newHash == c.Hash {
		return fmt.Errorf("%w: the re-encode reproduced the airing bytes", ErrReplacementRefused)
	}
	if verdict, _, detail := EvaluateMediaQuality(quality); verdict != pipelinestate.VerdictContinue {
		return fmt.Errorf("%w: %s", ErrReplacementRefused, detail)
	}
	record, err := w.check(ctx, c, staged)
	if err != nil {
		return fmt.Errorf("check the re-encode of %s: %w", c.Path, err)
	}
	if !record.Admits() {
		return fmt.Errorf("%w: defect floor: %s", ErrReplacementRefused, record.VerdictReason)
	}
	w.record = record
	return nil
}

// commit switches the clip to its published replacement. When the switch fails the replacement
// is taken down again: the clip still airs its old file, and the next scan must not find the new
// one and catalogue it as a second clip.
func (w *airingSwap) commit(ctx context.Context, oldHash string, c StoreClip, published string) error {
	if err := w.store.ReplaceAiringClip(ctx, oldHash, c, w.record); err != nil {
		_ = os.Remove(published)
		_ = os.Remove(tools.SidecarPathFor(published))
		return err
	}
	w.committed = true
	return nil
}

// discardUncommitted removes the replacement's evidence copy when the switch never committed.
func (w *airingSwap) discardUncommitted(clipDir, evidence string) {
	if !w.committed && evidence != "" && evidence != w.oldEvidence {
		_ = os.Remove(filepath.Join(clipDir, filepath.FromSlash(evidence)))
	}
}

// removeOldEvidence removes the airing clip's own evidence copy once it switched. Evidence is
// encoded from one clip's master, so no other clip names it; the master itself always stays.
func (w *airingSwap) removeOldEvidence(clipDir string, assets asset.Manifest) {
	if w.oldEvidence == "" || w.oldEvidence == derivativePath(assets.Evidence) || w.oldEvidence == assets.SourceMaster.Path {
		return
	}
	_ = os.Remove(filepath.Join(clipDir, filepath.FromSlash(w.oldEvidence)))
}
