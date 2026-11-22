package testkit

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
)

// The decoder hands back real canvas-sized PNGs and the cues' timing, and refuses what the image
// worker refuses.
func TestSubtitleDecoderWritesRealStillsAndRefuses(t *testing.T) {
	d := &SubtitleDecoder{Width: 96, Height: 54, Cues: []SubtitleCue{
		{StartMs: 1000, EndMs: 2000}, {StartMs: 2000, Clear: true},
	}}
	dir := t.TempDir()
	got, err := d.Decode(t.Context(), playout.SubtitleTrack{Input: "/media/film.mkv", Codec: "hdmv_pgs_subtitle", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 96 || got.Height != 54 || len(got.States) != 2 || got.States[1].PNG != "" {
		t.Fatalf("stills %+v", got)
	}
	s := got.States[0]
	if s.StartMs != 1000 || s.EndMs != 2000 || filepath.Dir(s.PNG) != dir {
		t.Fatalf("shown state %+v: want its window and a still in the track's directory", s)
	}
	f, err := os.Open(s.PNG)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if cfg, err := png.DecodeConfig(f); err != nil || cfg.Width != 96 || cfg.Height != 54 {
		t.Errorf("still %+v %v: want a 96x54 PNG", cfg, err)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	refused := map[string]struct {
		ctx   context.Context
		dec   *SubtitleDecoder
		track playout.SubtitleTrack
	}{
		"cancelled": {cancelled, &SubtitleDecoder{}, playout.SubtitleTrack{Codec: "hdmv_pgs_subtitle", Dir: dir}},
		"vobsub":    {t.Context(), &SubtitleDecoder{}, playout.SubtitleTrack{Codec: "dvd_subtitle", Dir: dir}},
		"no dir":    {t.Context(), &SubtitleDecoder{}, playout.SubtitleTrack{Codec: "hdmv_pgs_subtitle", Dir: filepath.Join(dir, "absent")}},
		"err":       {t.Context(), &SubtitleDecoder{Err: playout.ErrSubtitleDecoderUnavailable}, playout.SubtitleTrack{Codec: "hdmv_pgs_subtitle", Dir: dir}},
	}
	for name, c := range refused {
		if _, err := c.dec.Decode(c.ctx, c.track); err == nil {
			t.Errorf("%s: decoded", name)
		}
		if len(c.dec.Calls()) != 1 {
			t.Errorf("%s: %d calls recorded, want the refused one", name, len(c.dec.Calls()))
		}
	}
}

// The preparer's track is one playout loads, and it refuses a cancelled context and a failure.
func TestBurnInPreparerWritesALoadableTrackAndRefuses(t *testing.T) {
	var p BurnInPreparer
	dir := filepath.Join(t.TempDir(), "track")
	b, err := p.Prepare(t.Context(), "/media/film.mkv", 1, dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded, ok := playout.LoadBurnIn(dir); !ok || loaded.Width != b.Width || len(loaded.Cues) != 1 {
		t.Fatalf("loaded %+v %v", loaded, ok)
	}
	if _, err := os.Stat(dir + ".building"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("build directory left behind: %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Prepare(cancelled, "/media/film.mkv", 1, filepath.Join(t.TempDir(), "t")); err == nil {
		t.Error("prepared through a cancelled context")
	}
	p.SetFail(true)
	if _, err := p.Prepare(t.Context(), "/media/film.mkv", 1, filepath.Join(t.TempDir(), "t")); err == nil {
		t.Error("prepared while failing")
	}
	p.WaitRuns(t, 3)
	if p.Runs() != 3 {
		t.Errorf("runs %d, want 3", p.Runs())
	}
}
