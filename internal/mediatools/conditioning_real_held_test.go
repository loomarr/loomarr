package mediatools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
)

// TestMeasureConditioningRealHeldSources is the #752 red loop. It uses one
// identity-bound Archive source from each Gate B failure class. It is opt-in
// because the development corpus is intentionally not a repository fixture.
func TestMeasureConditioningRealHeldSources(t *testing.T) {
	root := os.Getenv("FILLER_REFERENCE_MEDIA_ROOT")
	if root == "" {
		t.Skip("set FILLER_REFERENCE_MEDIA_ROOT to run the #752 real-source loop")
	}
	tools := mediatools.NewFFmpegTools("/opt/homebrew/bin/ffmpeg", "/opt/homebrew/bin/ffprobe", "", "", "")
	for _, test := range []struct {
		name string
		file string
	}{
		{name: "invalid detector interval", file: "e61b2c55d9af9fea.ogv"},
		{name: "decoded frame resource limit", file: "f7b45547f495867b.ogv"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, err := tools.MeasureConditioning(ctx, mediatools.ConditioningRequest{Path: filepath.Join(root, test.file)}); err != nil {
				t.Fatalf("MeasureConditioning(%s): %v", test.file, err)
			}
		})
	}
}
