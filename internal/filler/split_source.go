package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler/split"
)

func resolveSplitSource(ctx context.Context, clipDir string, clip StoreClip, bound split.SourceAsset) (split.SourceAsset, string, error) {
	if bound == (split.SourceAsset{}) {
		playable := filepath.Join(clipDir, filepath.FromSlash(clip.Path))
		if tags, ok := ReadSidecarTags(playable); ok && tags.MediaAssets != nil && tags.MediaAssets.Evidence != nil {
			evidence := tags.MediaAssets.Evidence
			bound = split.SourceAsset{
				Role: split.SourceEvidence, SHA256: evidence.Asset.SHA256, Bytes: evidence.Asset.Bytes,
				ClipHash: evidence.Asset.ClipHash, Path: evidence.Asset.Path, DurationMs: evidence.DurationMs,
			}
		} else {
			digest, size, err := FileSHA256(playable)
			if err != nil {
				return split.SourceAsset{}, "", fmt.Errorf("resolve legacy split source: %w", err)
			}
			bound = split.SourceAsset{
				Role: split.SourceLegacyPlayback, SHA256: digest, Bytes: size,
				ClipHash: clip.Hash, Path: clip.Path, DurationMs: clip.DurationMs,
			}
		}
	}
	if err := bound.Validate(); err != nil {
		return split.SourceAsset{}, "", err
	}
	full := filepath.Join(clipDir, filepath.FromSlash(bound.Path))
	info, err := os.Lstat(full)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return split.SourceAsset{}, "", errors.New("split source is not a regular retained artifact")
	}
	digest, size, err := FileSHA256(full)
	if err != nil || digest != bound.SHA256 || size != bound.Bytes {
		return split.SourceAsset{}, "", errors.New("split source bytes do not match the proposal")
	}
	sparse, err := ClipID(full)
	if err != nil || sparse != bound.ClipHash {
		return split.SourceAsset{}, "", errors.New("split source sparse identity does not match the proposal")
	}
	if err := ctx.Err(); err != nil {
		return split.SourceAsset{}, "", err
	}
	return bound, full, nil
}
