package playout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

type fakeHLSOrigin struct {
	name     string
	acquired []string
	stopped  int
	assets   map[string]string
}

var errFakeAcquire = errors.New("fake acquire")

func (f *fakeHLSOrigin) acquirePlaylist(ch string, _ EncodePlan, _ bool) (hlsPlaylistLease, error) {
	f.acquired = append(f.acquired, ch)
	return hlsPlaylistLease{}, errFakeAcquire
}
func (f *fakeHLSOrigin) AssetPath(_ string, _ EncodePlan, rel string) (string, bool) {
	p, ok := f.assets[rel]
	return p, ok
}
func (f *fakeHLSOrigin) StopChannel(string) { f.stopped++ }
func (f *fakeHLSOrigin) StopAll()           { f.stopped++ }

func TestSwitchedHLSChoosesPerTuneAndServesBothOrigins(t *testing.T) {
	remux := &fakeHLSOrigin{name: "remux", assets: map[string]string{"seg-1.ts": "/remux/seg-1.ts"}}
	pk := &fakeHLSOrigin{name: "packager", assets: map[string]string{"init.mp4": "/pk/init.mp4"}}
	on := false
	s := switchedHLS{remux: remux, packaged: pk, usePackager: func() bool { return on }}

	_, _ = s.acquirePlaylist("a", PlanBaseline, false)
	on = true
	_, _ = s.acquirePlaylist("b", PlanBaseline, false)
	if len(remux.acquired) != 1 || remux.acquired[0] != "a" || len(pk.acquired) != 1 || pk.acquired[0] != "b" {
		t.Fatalf("remux tuned %v, packager tuned %v", remux.acquired, pk.acquired)
	}
	// A channel keeps serving from the origin it started on after the setting flips.
	on = false
	if p, ok := s.AssetPath("b", PlanBaseline, "init.mp4"); !ok || p != "/pk/init.mp4" {
		t.Fatalf("packager asset after flip: %q %v", p, ok)
	}
	if p, ok := s.AssetPath("a", PlanBaseline, "seg-1.ts"); !ok || p != "/remux/seg-1.ts" {
		t.Fatalf("remux asset: %q %v", p, ok)
	}
	s.StopAll()
	s.StopChannel("a")
	if remux.stopped != 2 || pk.stopped != 2 {
		t.Fatalf("stops: remux %d packager %d", remux.stopped, pk.stopped)
	}
}

func TestPackagerHLSAssetPathServesOnlyItsOwnFiles(t *testing.T) {
	m := &PackagerHLS{channels: map[remuxKey]*packagedChannel{
		{channel: "ch", plan: PlanBaseline}: {dir: "/scratch/ch-1"},
	}}
	for rel, want := range map[string]bool{
		"init.mp4": true, "seg00000007.m4s": true,
		"../secret": false, "seg/../../x.m4s": false, "live.m3u8": false, "seg-1.ts": false,
	} {
		if _, ok := m.AssetPath("ch", PlanBaseline, rel); ok != want {
			t.Errorf("AssetPath(%q) = %v, want %v", rel, ok, want)
		}
	}
	if _, ok := m.AssetPath("other", PlanBaseline, "init.mp4"); ok {
		t.Error("an asset of a channel with no packager resolved")
	}
}

// playout.hls_dir is disk-backed beside the database (#1512), so a crash (which skips Stop) would
// leave its segments there for good. A new process sweeps the roots no live process owns, by the
// owner's lock, not by age: live, a new build swept the still-serving process's root because it
// had been idle for ten minutes, and that process's next tune failed.
func TestNewScratchRootSweepsOnlyUnownedRoots(t *testing.T) {
	base := t.TempDir()
	mk := func(rel string, age time.Duration, lock bool) string {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if lock {
			if err := os.WriteFile(filepath.Join(p, scratchLock), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		mtime := time.Now().Add(-age)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	crashed := mk("loomarr-packager-1", 0, true) // fresh, but its owner is gone
	crashedRemux := mk("loomarr-hls-2", 0, true)
	idle := mk("loomarr-packager-3", time.Hour, true) // a live process with no viewers for an hour
	release, err := holdScratch(idle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	legacy := mk("loomarr-hls-4", time.Hour, false) // no lock file: spared until long quiet
	legacyDead := mk("loomarr-hls-5", 25*time.Hour, false)
	other := mk("operator-files", 25*time.Hour, false)

	m, err := NewPackagerHLS(nil, "ffmpeg", base, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	for p, want := range map[string]bool{
		crashed: false, crashedRemux: false, legacyDead: false, idle: true, legacy: true, other: true, m.root: true,
	} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(p), err == nil, want)
		}
	}
}

// Filler on the packager path gets the static gain the old chain applied (#1512 G6), in the
// builder's audio stage; a library title (GainDB 0) gets no audio filter at all.
func TestPackagerItemArgsApplyTheFillerGain(t *testing.T) {
	host := HostFor(EncoderSoftware, true, GPUFilters{})
	out := OutputProfile{Width: 1280, Height: 720, FPS: 25, Quality: 23, TargetKbps: 3000, MaxKbps: 4500, GOPSeconds: 1, AudioKbps: 128}
	format := MediaFormat{VideoCodec: "h264", Width: 1280, Height: 720, FrameRate: 25, PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	slot := packager.Slot{Frames: 250, AudioFrames: 469}
	for gain, want := range map[float64]string{-2.5: "volume=-2.5dB", 0: ""} {
		_, args, err := packagerItemArgs(host, out, PackagerItem{Input: "clip.mp4", Format: format, GainDB: gain}, slot)
		if err != nil {
			t.Fatal(err)
		}
		af := slices.Index(args, "-af")
		switch {
		case want == "" && af >= 0:
			t.Errorf("gain 0: unexpected -af %q", args[af+1])
		case want != "" && (af < 0 || args[af+1] != want):
			t.Errorf("gain %v: args %q lack -af %s", gain, args, want)
		case want != "" && af > slices.Index(args, "-c:a"):
			t.Errorf("gain %v: -af after -c:a in %q", gain, args)
		}
	}
}

func TestFillerGainIsFillerOnly(t *testing.T) {
	lufs := -20.0
	for name, tc := range map[string]struct {
		a      Airing
		target string
		gain   float64
		noted  bool
	}{
		"filler measured":     {Airing{Source: "/f/clip.mp4", MeasuredLUFS: &lufs}, "-23", -3, false},
		"library title":       {Airing{LibraryItemID: "x", MeasuredLUFS: &lufs}, "-23", 0, false},
		"filler unmeasured":   {Airing{Source: "/f/clip.mp4"}, "-23", 0, true},
		"target not a number": {Airing{Source: "/f/clip.mp4", MeasuredLUFS: &lufs}, "loud", 0, true},
		"no target":           {Airing{Source: "/f/clip.mp4", MeasuredLUFS: &lufs}, "", 0, false},
	} {
		gain, note := FillerGain(tc.a, tc.target)
		if gain != tc.gain || (note != "") != tc.noted {
			t.Errorf("%s: FillerGain = %v, %q", name, gain, note)
		}
	}
}

// stuckSlateSource airs nothing (a card slot), so the packager wants slate from the first instant.
type stuckSlateSource struct{}

func (stuckSlateSource) ItemAt(context.Context, string, EncodePlan, time.Time) (PackagerItem, error) {
	return PackagerItem{Label: "card", Remaining: time.Minute}, nil
}
func (stuckSlateSource) Output(context.Context, string, EncodePlan) (HostProfile, OutputProfile) {
	return HostProfile{}, OutputProfile{Width: 1280, Height: 720, FPS: 25, GOPSeconds: 2}
}

// The slate is not on the tune path (#1512 G2): live, a cold process spent 0.8 s encoding it before
// the channel's first item even resolved, though the first manifest waits for a real item anyway.
// A channel starts at once while the slate encodes in the background, and Stop ends that encode.
func TestChannelStartDoesNotWaitForTheSlate(t *testing.T) {
	stuck := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(stuck, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(stuckSlateSource{}, stuck, t.TempDir(), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	started := make(chan error, 1)
	go func() {
		lease, err := m.acquirePlaylist("ch", PlanBaseline, false)
		if err == nil {
			lease.release()
		}
		started <- err
	}()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the channel start waited on the slate encode")
	}
}
