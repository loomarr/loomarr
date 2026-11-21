package filler_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

type airWindowMem struct {
	clips   map[string]clipstore.Clip
	windows map[string][2]int64
}

func (m *airWindowMem) ClipsWithoutAirWindow(context.Context) ([]clipstore.Clip, error) {
	var out []clipstore.Clip
	for _, c := range m.clips {
		if _, done := m.windows[c.Hash]; !done {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *airWindowMem) SetClipAirWindow(_ context.Context, hash string, start, end int64, _ time.Time) error {
	m.windows[hash] = [2]int64{start, end}
	return nil
}

// Each pass measures the air window of clips without one (#3384), from the quality report in the
// sidecar. A clip with no report is set to air whole, so it is not read again every pass.
func TestPipeline_MeasuresAirWindowsFromTheSidecar(t *testing.T) {
	dir := t.TempDir()
	clip := func(hash string) clipstore.Clip {
		return clipstore.Clip{Clip: clips.Clip{Hash: hash, Path: "aa/bb/" + hash + ".mp4", DurationMs: 30_000}}
	}
	mem := &airWindowMem{clips: map[string]clipstore.Clip{
		"dead-edges": clip("dead-edges"), "no-report": clip("no-report"),
	}, windows: map[string][2]int64{}}
	media := filepath.Join(dir, "aa/bb/dead-edges.mp4")
	if err := os.MkdirAll(filepath.Dir(media), 0o755); err != nil {
		t.Fatal(err)
	}
	q := tools.MediaQuality{DurationMs: 30_000,
		Black:   []tools.Interval{{StartMs: 0, EndMs: 1_500}, {StartMs: 29_000, EndMs: 30_000}},
		Silence: []tools.Interval{{StartMs: 0, EndMs: 1_500}, {StartMs: 29_000, EndMs: 30_000}}}
	if err := asset.WriteSidecarTags(media, asset.SidecarTags{MediaQuality: &q}, false); err != nil {
		t.Fatal(err)
	}

	pipe := newPipe(newPipeMemStore(), nil, filler.DefaultBudget()).WithRewind(nil, dir).WithAirWindows(mem)
	if _, err := pipe.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mem.windows["dead-edges"]; got != [2]int64{1_300, 29_200} {
		t.Errorf("dead-edges window = %v, want [1300 29200]", got)
	}
	if got := mem.windows["no-report"]; got != [2]int64{0, 30_000} {
		t.Errorf("no-report window = %v, want it to air whole", got)
	}

	// A measured clip is not measured again.
	mem.windows["dead-edges"] = [2]int64{1, 2}
	if _, err := pipe.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mem.windows["dead-edges"]; got != [2]int64{1, 2} {
		t.Errorf("a measured window was measured again: %v", got)
	}
}
