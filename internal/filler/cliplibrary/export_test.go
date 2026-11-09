package cliplibrary

import (
	"context"

	"github.com/loomarr/loomarr/internal/media/tools"
)

// The helpers below are test entry points into production code. None of them is reached from a
// binary, so they live here rather than in the package, where deadcode would report them.

// ScanDir is scanDir with no watch folder excluded. The duration floor is optional because most
// tests only assert that a file is catalogued.
func ScanDir(ctx context.Context, dir string, probe tools.Prober, minDurationMs ...int64) (clips []RawClip, skipped int, err error) {
	if dir == "" {
		return nil, 0, nil
	}
	if probe == nil {
		probe = FFprobe
	}
	var minMs int64
	if len(minDurationMs) > 0 {
		minMs = minDurationMs[0]
	}
	clips, skipped, _, err = scanDir(ctx, dir, "", probe, minMs)
	return clips, skipped, err
}

// GenerateArtwork is generateArtwork with no storage governor, returning the failure count.
func GenerateArtwork(ctx context.Context, dir string, clips []RawClip, render ArtworkRenderer) (failed int) {
	return generateArtwork(ctx, dir, clips, render, nil).Failed
}

// TakeIn is TakeInFrom with no source, binding or storage governor.
func TakeIn(watchDir, clipDir string, fetched bool, log func(string, ...any)) (IntakeResult, error) {
	return TakeInFrom(context.Background(), watchDir, clipDir, fetched, "", log, nil, nil, nil)
}

func movePath(src, dst string) error {
	return movePathWithStorage(context.Background(), src, dst, nil)
}

func moveIfPresent(src, dst string) error {
	return moveIfPresentWithStorage(context.Background(), src, dst, nil)
}
