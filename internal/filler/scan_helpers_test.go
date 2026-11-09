package filler

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

// ScanDir drives the real folder scanner over dir with the given prober and no watch folder, as
// the tests that exercise the sync and transcode lifecycle need. It returns the catalogued clips
// and how many files the scan boundary rejected.
func ScanDir(ctx context.Context, dir string, probe tools.Prober) ([]cliplibrary.RawClip, int, error) {
	layout, err := cliplibrary.NewLayout(dir, "")
	if err != nil {
		return nil, 0, err
	}
	rejected := 0
	source := cliplibrary.DirSource{
		Layout: layout, Probe: probe,
		// Artwork is not under test here; a renderer that writes nothing keeps ffmpeg out of it.
		Artwork: func(context.Context, string, string, string, float64) error { return nil },
		Log: func(_ string, args ...any) {
			for i := 0; i+1 < len(args); i += 2 {
				if args[i] == "rejected" {
					rejected, _ = args[i+1].(int)
				}
			}
		},
	}
	scan, err := source.ScanLocalClips(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("scan %s: %w", dir, err)
	}
	return scan.Clips, rejected, nil
}

type intakeCapacityMeter struct {
	measurement storagegovernor.Measurement
}

func (m intakeCapacityMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return m.measurement, nil
}

func (intakeCapacityMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	return 0, nil
}
