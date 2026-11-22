package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// SubtitleCue is one state SubtitleDecoder hands back: a box drawn on the canvas from StartMs
// until EndMs, or a clear canvas when Clear is set.
type SubtitleCue struct {
	StartMs, EndMs int64
	Clear          bool
}

// SubtitleDecoder is the shared fixture-backed double for playout.SubtitleStillDecoder. Decode
// writes one small real PNG per shown cue into the track's directory, a white box on a
// transparent full canvas, and returns the cues as states, as the image worker hands back a
// track. It refuses what the worker refuses: a cancelled context, a codec playout cannot burn, a
// directory that does not exist, and every track when Err is set.
type SubtitleDecoder struct {
	// Width and Height are the canvas; zero is 64x36.
	Width, Height int
	Cues          []SubtitleCue
	Err           error

	mu    sync.Mutex
	calls []playout.SubtitleTrack
}

// Decode writes the cues' stills into track.Dir.
func (d *SubtitleDecoder) Decode(ctx context.Context, track playout.SubtitleTrack) (playout.SubtitleStills, error) {
	d.mu.Lock()
	d.calls = append(d.calls, track)
	d.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return playout.SubtitleStills{}, err
	}
	if d.Err != nil {
		return playout.SubtitleStills{}, d.Err
	}
	if !playout.IsBurnableSubtitle(track.Codec) {
		return playout.SubtitleStills{}, fmt.Errorf("subtitle decoder: codec %q is not an image track it decodes", track.Codec)
	}
	if fi, err := os.Stat(track.Dir); err != nil || !fi.IsDir() {
		return playout.SubtitleStills{}, fmt.Errorf("subtitle decoder: no directory %q for the stills", track.Dir)
	}
	w, h := d.Width, d.Height
	if w == 0 || h == 0 {
		w, h = 64, 36
	}
	stills := playout.SubtitleStills{Width: w, Height: h}
	for i, c := range d.Cues {
		state := playout.SubtitleState{StartMs: c.StartMs, EndMs: c.EndMs}
		if !c.Clear {
			state.PNG = filepath.Join(track.Dir, fmt.Sprintf("cue%03d.png", i))
			if err := writeBoxStill(state.PNG, w, h); err != nil {
				return playout.SubtitleStills{}, err
			}
		}
		stills.States = append(stills.States, state)
	}
	return stills, nil
}

// Calls returns every track Decode was asked for, refused or not.
func (d *SubtitleDecoder) Calls() []playout.SubtitleTrack {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]playout.SubtitleTrack(nil), d.calls...)
}

// writeBoxStill writes a w x h transparent canvas with an opaque white box across its lower third.
func writeBoxStill(path string, w, h int) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, image.Rect(w/4, h*2/3, w*3/4, h*5/6), image.NewUniform(color.White), image.Point{}, draw.Src)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// BurnInPreparer is the shared counting double for playout.BurnInPreparer. Prepare writes a
// prepared one-cue 1920x1080 track that playout.LoadBurnIn reads, without ffmpeg, building it
// beside dir and renaming it into place as the real preparer does, so dir is complete or absent.
// It refuses a cancelled context and, while SetFail is on, every track.
type BurnInPreparer struct {
	fail     atomic.Bool
	runs     atomic.Int32
	finished atomic.Int32
}

// burnInIndex is playout's index file name; Prepare checks LoadBurnIn reads what it writes, so a
// renamed index fails loudly.
const burnInIndex = "burnin.json"

// Prepare writes the prepared track into dir.
func (p *BurnInPreparer) Prepare(ctx context.Context, _ string, _ int, dir string) (*playout.BurnIn, error) {
	p.runs.Add(1)
	defer p.finished.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.fail.Load() {
		return nil, errors.New("burn-in preparer: no such track")
	}
	build := dir + ".building"
	if err := os.MkdirAll(build, 0o755); err != nil {
		return nil, err
	}
	b := playout.BurnIn{Width: 1920, Height: 1080, Cues: []playout.BurnCue{{At: time.Second, Still: "c00001.nut"}}}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	for name, body := range map[string][]byte{burnInIndex: raw, "c00001.nut": nil, "blank.nut": nil} {
		if err := os.WriteFile(filepath.Join(build, name), body, 0o644); err != nil {
			return nil, err
		}
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(build, dir); err != nil {
		return nil, err
	}
	loaded, ok := playout.LoadBurnIn(dir)
	if !ok {
		return nil, fmt.Errorf("burn-in preparer: playout.LoadBurnIn does not read the double's %s", burnInIndex)
	}
	return loaded, nil
}

// SetFail makes every later Prepare refuse (true) or succeed (false).
func (p *BurnInPreparer) SetFail(fail bool) { p.fail.Store(fail) }

// Runs is how many times Prepare was called.
func (p *BurnInPreparer) Runs() int { return int(p.runs.Load()) }

// WaitRuns waits until n calls to Prepare have returned.
func (p *BurnInPreparer) WaitRuns(t testing.TB, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for int(p.finished.Load()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("burn-in preparer: %d of %d preparations returned", p.finished.Load(), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
