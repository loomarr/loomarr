package testkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// The decoder hands back real canvas-sized PNGs and the cues' timing, and refuses what the image
// worker refuses.
func TestSubtitleDecoderWritesRealStillsAndRefuses(t *testing.T) {
	d := &SubtitleDecoder{Width: 96, Height: 54, Cues: []SubtitleCue{
		{StartMs: 1000, EndMs: 2000}, {StartMs: 2000, Clear: true},
	}}
	dir := t.TempDir()
	sup := filepath.Join(t.TempDir(), "track.sup")
	if err := os.WriteFile(sup, []byte("PG"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("PG"))
	track := playout.SubtitleTrack{Sup: sup, SHA256: hex.EncodeToString(sum[:]), Codec: "hdmv_pgs_subtitle", Dir: dir}
	got, err := d.Decode(t.Context(), track)
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

	escaped := t.TempDir()
	out, err := (&SubtitleDecoder{StillDir: escaped, Cues: []SubtitleCue{{StartMs: 0, EndMs: 10}}}).Decode(t.Context(), track)
	if err != nil || filepath.Dir(out.States[0].PNG) != escaped {
		t.Errorf("StillDir: %+v %v, want the still written outside the track's directory", out, err)
	}

	with := func(edit func(*playout.SubtitleTrack)) playout.SubtitleTrack {
		c := track
		edit(&c)
		return c
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	refused := map[string]struct {
		ctx   context.Context
		dec   *SubtitleDecoder
		track playout.SubtitleTrack
	}{
		"cancelled": {cancelled, &SubtitleDecoder{}, track},
		"vobsub":    {t.Context(), &SubtitleDecoder{}, with(func(c *playout.SubtitleTrack) { c.Codec = "dvd_subtitle" })},
		"no sup":    {t.Context(), &SubtitleDecoder{}, with(func(c *playout.SubtitleTrack) { c.Sup = filepath.Join(dir, "absent.sup") })},
		"bad sum":   {t.Context(), &SubtitleDecoder{}, with(func(c *playout.SubtitleTrack) { c.SHA256 = "00" })},
		"no dir":    {t.Context(), &SubtitleDecoder{}, with(func(c *playout.SubtitleTrack) { c.Dir = filepath.Join(dir, "absent") })},
		"err":       {t.Context(), &SubtitleDecoder{Err: playout.ErrSubtitleDecoderUnavailable}, track},
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
	b, err := p.Prepare(t.Context(), "/media/film.mkv", 1, "hdmv_pgs_subtitle", dir)
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
	if _, err := p.Prepare(cancelled, "/media/film.mkv", 1, "hdmv_pgs_subtitle", filepath.Join(t.TempDir(), "t")); err == nil {
		t.Error("prepared through a cancelled context")
	}
	p.SetFail(true)
	if _, err := p.Prepare(t.Context(), "/media/film.mkv", 1, "hdmv_pgs_subtitle", filepath.Join(t.TempDir(), "t")); err == nil {
		t.Error("prepared while failing")
	}
	p.SetFail(false)
	p.SetHold(true)
	held, release := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := p.Prepare(held, "/media/film.mkv", 1, "hdmv_pgs_subtitle", filepath.Join(t.TempDir(), "t"))
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("a held preparation returned before its context was cancelled")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-done; err == nil {
		t.Error("a held preparation succeeded once cancelled")
	}
	p.WaitRuns(t, 4)
	if p.Runs() != 4 || p.Finished() != 4 {
		t.Errorf("runs %d finished %d, want 4", p.Runs(), p.Finished())
	}
}
