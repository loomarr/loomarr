package split

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Locations of the retained evidence tree below a clip folder. They mirror the filler root's
// MediaAssetRootName and its evidence directory; the root's resolveSplitSource test builds a real
// evidence derivative under the root's names and requires it to validate here.
const (
	mediaAssetRootName   = ".loomarr-media"
	mediaEvidenceDirName = "evidence"
)

type SourceRole string

const (
	SourceEvidence       SourceRole = "evidence"
	SourceLegacyPlayback SourceRole = "legacy_playback"
)

// SourceAsset freezes the exact bytes and timeline on which a cut proposal is based. A
// playback recipe may change later without rewriting the meaning of an existing review.
type SourceAsset struct {
	Role       SourceRole `json:"role"`
	SHA256     string     `json:"sha256"`
	Bytes      int64      `json:"bytes"`
	ClipHash   string     `json:"clipHash"`
	Path       string     `json:"path"`
	DurationMs int64      `json:"durationMs"`
}

// Validate reports whether the source carries a complete, tree-correct retained identity.
func (s SourceAsset) Validate() error {
	return s.validate()
}

func (s SourceAsset) validate() error {
	if s.Role != SourceEvidence && s.Role != SourceLegacyPlayback {
		return fmt.Errorf("split source role %q is unsupported", s.Role)
	}
	if !IsContentHash(s.SHA256) || !IsContentHash(s.ClipHash) || s.Bytes <= 0 || s.DurationMs <= 0 || !IsRelativePath(filepath.FromSlash(s.Path)) {
		return errors.New("split source identity is incomplete")
	}
	clean := filepath.Clean(filepath.FromSlash(s.Path))
	if s.Role == SourceEvidence {
		if !PathContains(filepath.Join(mediaAssetRootName, mediaEvidenceDirName), clean) {
			return errors.New("split evidence source is outside the evidence tree")
		}
	} else if PathContains(mediaAssetRootName, clean) {
		return errors.New("legacy split source is unexpectedly hidden")
	}
	return nil
}
