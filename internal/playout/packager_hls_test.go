package playout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

type fakeHLSOrigin struct {
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

func TestPackagerHLSAssetPathServesOnlyItsOwnFiles(t *testing.T) {
	m := &PackagerHLS{channels: map[packagedKey]*packagedChannel{
		{channel: "ch", format: FormatBaseline}: {dir: "/scratch/ch-1"},
	}}
	// A channel's formats share one flat namespace, `<format>-<file>`; the files keep bare names.
	for rel, want := range map[string]string{
		"1080p-h264-sdr-init.mp4": "/scratch/ch-1/init.mp4", "1080p-h264-sdr-seg00000007.m4s": "/scratch/ch-1/seg00000007.m4s",
		"init.mp4": "", "seg00000007.m4s": "", "4k-hevc-sdr-init.mp4": "", "1080p-h264-sdr.m3u8": "",
		"../secret": "", "1080p-h264-sdr-seg/../../x.m4s": "", "1080p-h264-sdr-init.mp4.bak": "", "seg-1.ts": "",
	} {
		if got, ok := m.AssetPath("ch", PlanBaseline, rel); got != want || ok != (want != "") {
			t.Errorf("AssetPath(%q) = %q, %v, want %q", rel, got, ok, want)
		}
	}
	if _, ok := m.AssetPath("other", PlanBaseline, "1080p-h264-sdr-init.mp4"); ok {
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
		_, args, err := packagerItemArgs(host, out, PackagerItem{Input: "clip.mp4", Format: format, GainDB: gain}, nil, slot, itemFault{})
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

// A software host degrades a 4K HDR item instead of refusing it (#1517): with no ledger the
// packager's item starts on the unmeasured start rung, as the retired program handler's did, and
// SDR 1080p stays full.
func TestPackagerItemArgsStartOnTheSoftwareRung(t *testing.T) {
	host := HostFor(EncoderSoftware, true, GPUFilters{})
	out := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	slot := packager.Slot{Frames: 250, AudioFrames: 469}
	for name, want := range map[string]bool{"hevc-4k-hdr-dv": true, "h264-1080p-sdr-25": false} {
		ffmpeg, runs := failingEncoder(t, "")
		m, err := NewPackagerHLS(stuckSlateSource{}, ffmpeg, t.TempDir(), time.Second, nil)
		if err != nil {
			t.Fatal(err)
		}
		m.source = fixedItemSource{item: PackagerItem{Label: "prog", Remaining: time.Hour, Input: "title.mkv", Format: testSources()[name]}}
		item, err := m.schedule(packagedKey{channel: "ch", format: FormatBaseline}, host, out, nil, nil, slog.New(slog.DiscardHandler))(t.Context(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if rc, err := item.Open(t.Context(), slot); err == nil {
			_, _ = io.Copy(io.Discard, rc)
			_ = rc.Close()
		}
		m.Stop()
		args := strings.Fields(runs()[0])
		skip := slices.Index(args, "-skip_frame:v")
		keyframesOnly := skip >= 0 && args[skip+1] == "nokey" && skip < slices.Index(args, "-i")
		if keyframesOnly != want {
			t.Errorf("%s: keyframes-only start = %v, want %v: %q", name, keyframesOnly, want, args)
		}
	}
}

// failingEncoder is an ffmpeg that records its arguments, one run per line, then fails before
// producing output with the given stderr, the way a GPU fault ends an item's encoder.
func failingEncoder(t *testing.T, stderr string) (ffmpeg string, runs func() []string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "runs")
	ffmpeg = filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\necho '" + stderr + "' >&2\nexit 1\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return ffmpeg, func() []string {
		b, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
}

// openItem runs the channel's schedule for one item n times, as the packager does when an
// item fails: the first encoder fails, the schedule is asked again, the next attempt opens.
// It returns the fallbacks the packager reported to its metrics observer.
func openItem(t *testing.T, ffmpeg string, host HostProfile, it PackagerItem, n int) []string {
	t.Helper()
	m, err := NewPackagerHLS(stuckSlateSource{}, ffmpeg, t.TempDir(), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	obs := &countingObserver{}
	m.WithObserver(obs)
	m.source = fixedItemSource{item: it}
	out := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	sched := m.schedule(packagedKey{channel: "ch", format: FormatBaseline}, host, out, nil, nil, slog.New(slog.DiscardHandler))
	for range n {
		item, err := sched(t.Context(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		rc, err := item.Open(t.Context(), packager.Slot{Frames: 250, AudioFrames: 469})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	return obs.Fallbacks()
}

type fixedItemSource struct {
	stuckSlateSource
	item PackagerItem
}

func (s fixedItemSource) ItemAt(context.Context, string, time.Time) (PackagerItem, error) {
	return s.item, nil
}

// progItem is an hour of a library title with no watermark.
func progItem(format MediaFormat) PackagerItem {
	return PackagerItem{Label: "prog", Remaining: time.Hour, Input: "title.mkv", Format: format}
}

// The channel's bug rides a programme item into its encoder, sized for the packager's own encoder
// and output (#1512 phase 1d, which the retired programme route drew). An item without a resolver
// (filler, bumper, ID), one whose resolver declines (the channel turned it off, or this host's
// overlay failed its self-check) and a host with no GPU overlay graph all encode bug-free.
func TestPackagerDrawsTheBugOnProgrammesOnly(t *testing.T) {
	bug := &Watermark{Straight: "/wm/bug.png", Width: 120, Height: 60, Corner: CornerTopRight, MarginX: 96, MarginY: 54}
	var asked []string
	resolver := func(declines bool) WatermarkFor {
		return func(_ context.Context, enc Encoder, width, height int) *Watermark {
			asked = append(asked, fmt.Sprintf("%s %dx%d", enc, width, height))
			if declines {
				return nil
			}
			return bug
		}
	}
	nvenc, software := HostFor(EncoderNVENC, true, GPUFilters{}), HostFor(EncoderSoftware, true, GPUFilters{})
	for _, tc := range []struct {
		name string
		host HostProfile
		wm   WatermarkFor
		want bool
	}{
		{"programme", nvenc, resolver(false), true},
		{"filler, bumper or ID", nvenc, nil, false},
		{"resolver declines", nvenc, resolver(true), false},
		{"software host", software, resolver(false), false},
	} {
		asked = nil
		ffmpeg, runs := failingEncoder(t, "stopped")
		it := progItem(testSources()["h264-1080p-sdr-25"])
		it.Watermark = tc.wm
		openItem(t, ffmpeg, tc.host, it, 1)
		args := runs()[0]
		overlaid := strings.Contains(args, "movie=filename=/wm/bug.png")
		if overlaid != tc.want {
			t.Errorf("%s: bug drawn = %v, want %v: %s", tc.name, overlaid, tc.want, args)
		}
		if tc.wm != nil && !slices.Equal(asked, []string{fmt.Sprintf("%s 1920x1080", tc.host.Encoder)}) {
			t.Errorf("%s: the resolver was asked %q, want once for the packager's encoder and output", tc.name, asked)
		}
		if tc.host.Family == FamilyNVENC && !tc.want && strings.Contains(args, "yuv420p") {
			t.Errorf("%s: a bug-free item keeps the nv12 main: %s", tc.name, args)
		}
	}
}

// A tune-in starts its encoder at the source's indexed keyframe at or before the airing's offset
// (#1595). From the exact offset, ffmpeg's accurate seek decodes and discards everything back to
// that keyframe, up to a whole source GOP (~10 s in common encodes): measured, the first fragment
// took 345-776 ms on NVENC by how far back the keyframe was, and 339-347 ms from the keyframe. The
// tune-in then airs from up to one GOP earlier and its slot ends that much short of the airing's
// end. Every later lookup (a retry, a resume after a break or a slate) keeps the exact offset, so
// nothing airs twice; a keyframe further back than tuneInRewindMax, or no index, keeps it too.
func TestPackagerTuneInSeeksFromTheIndexedKeyframe(t *testing.T) {
	for _, tc := range []struct {
		name     string
		keyframe time.Duration
		indexed  bool
		want     string
	}{
		{"indexed keyframe 8.5 s back", 91500 * time.Millisecond, true, "91.500"},
		{"offset on a keyframe", 100 * time.Second, true, "100.000"},
		{"no index", 91500 * time.Millisecond, false, "100.000"},
		{"keyframe beyond the rewind bound", 100*time.Second - tuneInRewindMax - time.Millisecond, true, "100.000"},
	} {
		ffmpeg, runs := failingEncoder(t, "stopped")
		it := progItem(testSources()["h264-1080p-sdr-25"])
		it.Seek, it.Keyframe, it.KeyframeIndexed = 100*time.Second, tc.keyframe, tc.indexed
		openItem(t, ffmpeg, HostFor(EncoderNVENC, true, GPUFilters{}), it, 2)
		got := runs()
		if len(got) != 2 {
			t.Fatalf("%s: %d encoder runs, want 2", tc.name, len(got))
		}
		if !strings.Contains(got[0], "-ss "+tc.want+" ") {
			t.Errorf("%s: the tune-in seeks %q, want -ss %s", tc.name, got[0], tc.want)
		}
		if !strings.Contains(got[1], "-ss 100.000 ") {
			t.Errorf("%s: a later lookup seeks %q, want the airing's exact offset", tc.name, got[1])
		}
	}
}

// A source the GPU decoder faults on is retried with a CPU decode and the same hardware encoder
// (§9.1 V47, the retired chain's ladder): retrying the same -hwaccel path fails identically.
func TestPackagerRetriesAHardwareDecodeFaultWithACPUDecode(t *testing.T) {
	ffmpeg, runs := failingEncoder(t, "[AVHWFramesContext @ 0x1] Failed to sync surface 0xc: 23 (internal decoding error)")
	fallbacks := openItem(t, ffmpeg, HostFor(EncoderNVENC, true, GPUFilters{}), progItem(testSources()["h264-1080p-sdr-25"]), 2)
	r := runs()
	if len(r) != 2 || !strings.Contains(r[0], "-hwaccel") {
		t.Fatalf("want a GPU-decoded first attempt and a retry, got %q", r)
	}
	if strings.Contains(r[1], "-hwaccel") || !strings.Contains(r[1], "h264_nvenc") {
		t.Errorf("retry after a decode fault: want a CPU decode into the same encoder, got %q", r[1])
	}
	// The demotion is the live hardware→software fallback the metrics count (#1512 phase 4: the
	// retired prepared→live fallback was that counter's only other producer). The second attempt
	// fails too, but already decodes on the CPU, so it demotes nothing more.
	if !slices.Equal(fallbacks, []string{"hardware_to_software"}) {
		t.Errorf("fallbacks reported = %q, want one hardware_to_software", fallbacks)
	}
}

// A GPU tone-mapper that fails the item (no output, not a decode fault) is dropped for that source,
// and the retry takes the next tone-mapper for the curve (DemoteTonemap's order).
func TestPackagerRetriesAFailedGPUTonemapWithTheNextOne(t *testing.T) {
	ffmpeg, runs := failingEncoder(t, "[Parsed_tonemap_opencl_3 @ 0x1] Failed to enqueue kernel: -5.")
	fallbacks := openItem(t, ffmpeg, HostFor(EncoderNVENC, true, GPUFilters{TonemapOpenCL: true, Libplacebo: true}), progItem(testSources()["hevc-4k-hdr-dv"]), 2)
	r := runs()
	if len(r) != 2 || !strings.Contains(r[0], "tonemap_opencl") {
		t.Fatalf("want an OpenCL tone-mapped first attempt and a retry, got %q", r)
	}
	if strings.Contains(r[1], "tonemap_opencl") || !strings.Contains(r[1], "libplacebo") {
		t.Errorf("retry after a tone-map failure: want libplacebo, got %q", r[1])
	}
	// OpenCL → libplacebo stays on the GPU; libplacebo failing too leaves only the CPU tone-map.
	if !slices.Equal(fallbacks, []string{"hardware_to_software"}) {
		t.Errorf("fallbacks reported = %q, want one hardware_to_software (the second failure only)", fallbacks)
	}
}

// The packager admits through the one cost-aware policy (Admit, §9.1 V49): a full budget refuses a
// new channel with ErrAtCapacity, and an unmeasured (zero) budget never blocks playout.
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

func (stuckSlateSource) ItemAt(context.Context, string, time.Time) (PackagerItem, error) {
	return PackagerItem{Label: "card", Remaining: time.Minute}, nil
}
func (stuckSlateSource) Premium(context.Context, string) FormatClass { return "" }

func (stuckSlateSource) Output(context.Context, string, FormatClass, int) (HostProfile, OutputProfile) {
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

// slowItemSource airs one real item, whose encoder is still starting.
type slowItemSource struct{ stuckSlateSource }

func (slowItemSource) ItemAt(context.Context, string, time.Time) (PackagerItem, error) {
	return PackagerItem{Label: "prog", Remaining: time.Hour, Input: "movie.mkv", Format: MediaFormat{
		VideoCodec: "h264", Width: 1280, Height: 720, FrameRate: 25, PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}}, nil
}

// Off the tune path means off the tune-in's encoder too (#1512 G2): live, a cold tune's first
// fragment took ~0.25 s longer while the slate encoded beside it. The slate waits until the first
// item is on air, or until a slot needs it.
func TestSlateEncodeWaitsForTheFirstItem(t *testing.T) {
	dir := t.TempDir()
	mark := filepath.Join(dir, "slate-started")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *lavfi*) touch " + mark + ";; esac\nexec sleep 5\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(slowItemSource{}, ffmpeg, t.TempDir(), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	lease, err := m.acquirePlaylist("ch", PlanBaseline, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	time.Sleep(time.Second)
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("the slate encode started beside the tune-in item's encoder")
	}
}

// hdrItemSource airs one 4K HDR title on a CPU-only host that can tone-map.
type hdrItemSource struct{}

func (hdrItemSource) ItemAt(context.Context, string, time.Time) (PackagerItem, error) {
	return PackagerItem{Label: "hdr", Remaining: time.Hour, Input: "hdr.mkv", Format: MediaFormat{
		VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24, PixelFormat: "yuv420p10le",
		ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}}, nil
}

func (hdrItemSource) Premium(context.Context, string) FormatClass { return "" }

func (hdrItemSource) Output(context.Context, string, FormatClass, int) (HostProfile, OutputProfile) {
	return HostFor(EncoderSoftware, true, GPUFilters{}), OutputProfile{Width: 1280, Height: 720, FPS: 25, GOPSeconds: 2}
}

// The channel packager is admitted by the ResourceBudget (#1520): one lease per running (channel,
// format), its item encoding at the software rung the ledger picked, a second channel that does not fit
// even keyframes-only refused, and the lease returned when the packager stops.
func TestPackagerHLSIsAdmittedByTheResourceBudget(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "item-args")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *hdr.mkv*) echo \"$*\" > " + argsFile + ";; esac\nexec sleep 5\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	budget := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(3.5) })
	m, err := NewPackagerHLS(hdrItemSource{}, ffmpeg, t.TempDir(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.WithBudget(budget)
	t.Cleanup(m.Stop)

	lease, err := m.acquirePlaylist("ch1", PlanBaseline, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	var args []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if args, err = os.ReadFile(argsFile); err == nil && len(args) > 0 {
			break
		}
	}
	// 3.6 cores at full does not fit 3.5; rung 1 (3.42) does.
	if got := string(args); !strings.Contains(got, "-skip_loop_filter:v all") || strings.Contains(got, "-skip_frame:v") {
		t.Fatalf("item did not encode at the ledger's rung 1:\n%s", got)
	}
	if use := budget.Snapshot().InUse; use.Transcodes != 1 || use.CPUCores < 3.4 {
		t.Fatalf("ledger = %+v, want one HDR transcode at rung 1", use)
	}
	// A second channel: even SDR keyframes-only (0.297) does not fit beside 3.42 on 3.5.
	if _, err := m.acquirePlaylist("ch2", PlanBaseline, false); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("second channel: err = %v, want ErrAtCapacity", err)
	}
	m.StopChannel("ch1")
	for deadline := time.Now().Add(5 * time.Second); budget.Snapshot().InUse.Sessions != 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the stopped packager kept its lease: %+v", budget.Snapshot().InUse)
		}
	}
}

// countingHDRSource is hdrItemSource counting its lookups.
type countingHDRSource struct {
	hdrItemSource
	calls atomic.Int32
}

func (s *countingHDRSource) ItemAt(ctx context.Context, ch string, at time.Time) (PackagerItem, error) {
	s.calls.Add(1)
	return s.hdrItemSource.ItemAt(ctx, ch, at)
}

// Admission is priced for the first item, resolved before any encoder starts (#1520 follow-up). On
// 0.8 cores with 0.432 held, SDR keyframes-only (0.297) would fit but the 4K HDR item airing now
// (0.432) does not: the channel is refused and no ffmpeg ever runs. With room, the lease holds the
// HDR price from the start, and the schedule reuses the resolved item instead of looking it up again.
func TestPackagerHLSAdmitsForTheFirstItemBeforeAnyEncoder(t *testing.T) {
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\ntouch "+ran+"\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	allowance := 0.8
	budget := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(allowance) })
	held, err := budget.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K})
	if err != nil {
		t.Fatal(err)
	}
	source := &countingHDRSource{}
	m, err := NewPackagerHLS(source, ffmpeg, t.TempDir(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.WithBudget(budget)
	t.Cleanup(m.Stop)

	if _, err := m.acquirePlaylist("ch", PlanBaseline, false); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("4K HDR first item on a nearly full host: err = %v, want ErrAtCapacity", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("an encoder started for a refused channel")
	}

	held.Release()
	allowance = 3.5
	lease, err := m.acquirePlaylist("ch", PlanBaseline, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	if use := budget.Snapshot().InUse; use.CPUCores < 3.42-1e-9 || use.CPUCores > 3.42+1e-9 {
		t.Fatalf("ledger at admission = %+v, want the HDR item's rung-1 price (3.42) before it encodes", use)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(ran); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the admitted channel never started its item encoder")
		}
	}
	if n := source.calls.Load(); n != 2 { // one per start; the schedule reused the second
		t.Errorf("item lookups = %d, want 2 (the schedule must reuse the admission's lookup)", n)
	}
}

// The ledger learns a class's real CPU cost from live encodes (#1520). The retired chain reported it
// from each finished programme; the packager reports each item encoder's CPU over the media it
// DELIVERED (packager.Item.Delivered), never over its slot, so an encoder closed early cannot
// over-count; an item whose channel stopped reports nothing. Each item follows airItem's order:
// Delivered, then the item context is cancelled, then the reader is closed.
func TestPackagerItemEncoderTeachesTheLedgerItsCPUCost(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	// Enough CPU for the kernel's tick-granular accounting to see, then output, then wait to be killed.
	script := "#!/bin/sh\ni=0\nwhile [ $i -lt 300000 ]; do i=$((i+1)); done\nprintf fragment\nexec sleep 30\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(slowItemSource{}, ffmpeg, t.TempDir(), time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	budget := NewResourceBudget(nvencFacts)
	lease, err := budget.Admit(t.Context(), AdmitRequest{Class: ClassSDR})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	cost := func() float64 { return budget.Snapshot().Classes[ClassSDR].CPUCores }
	base := cost()
	_, out := slowItemSource{}.Output(t.Context(), "ch", FormatBaseline, 0)
	sched := m.schedule(packagedKey{channel: "ch", format: FormatBaseline},
		HostFor(EncoderSoftware, true, GPUFilters{}), out, lease, nil, slog.New(slog.DiscardHandler))
	// encodeItem airs one item into a 60 s slot as airItem does; delivered < 0 is a channel that
	// stopped mid-item (airItem reports nothing).
	encodeItem := func(delivered int64) {
		ictx, cancel := context.WithCancel(t.Context())
		item, err := sched(ictx, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		rc, err := item.Open(ictx, packager.Slot{Frames: int64(60 * out.FPS)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(rc, make([]byte, len("fragment"))); err != nil {
			t.Fatal(err)
		}
		if delivered >= 0 && item.Delivered != nil {
			item.Delivered(delivered)
		}
		cancel()
		_ = rc.Close()
	}

	encodeItem(-1)
	if got := cost(); got != base {
		t.Fatalf("an encoder whose channel stopped taught the ledger: %v → %v", base, got)
	}
	// 10 s delivered of a 60 s slot is under the ledger's 20 s sample floor; counted over its slot
	// it would have taught the ledger.
	encodeItem(int64(10 * out.FPS))
	if got := cost(); got != base {
		t.Fatalf("an encoder closed after 10 s of a 60 s slot taught the ledger (%v → %v): its media was counted by slot", base, got)
	}
	encodeItem(int64(60 * out.FPS))
	if got := cost(); got >= base {
		t.Fatalf("SDR CPU cost = %v after a delivered item that used far less than the probe's %v: the ledger did not learn", got, base)
	}
}

// fakeVariantOrigin is a packager-shaped hlsOrigin: its variant playlists are rendered per request.
type fakeVariantOrigin struct {
	fakeHLSOrigin
	asked []string
}

func (f *fakeVariantOrigin) MediaPlaylist(_ context.Context, _ string, _ EncodePlan, rel string) ([]byte, bool, error) {
	f.asked = append(f.asked, rel)
	return []byte("#EXTM3U\n" + rel + "\n"), rel == "1080p-h264-sdr.m3u8", nil
}

// The packager's Tune answer is a master playlist (#1512 phase 2b); the player then fetches the
// variant it names as an asset. Origin renders that from the packager, marked as a playlist so the
// transport authenticates its URIs, and never looks for it on disk. Other assets are still files.
func TestOriginServesThePackagerVariantPlaylist(t *testing.T) {
	pk := &fakeVariantOrigin{fakeHLSOrigin: fakeHLSOrigin{assets: map[string]string{}}}
	o := newOrigin(nil, pk)

	asset, ok, err := o.OpenAsset(context.Background(), "ch", PlanBaseline, "1080p-h264-sdr.m3u8")
	if err != nil || !ok || !asset.Playlist {
		t.Fatalf("variant: ok %v playlist %v err %v", ok, asset.Playlist, err)
	}
	if b, _ := io.ReadAll(asset.Content); string(b) != "#EXTM3U\n1080p-h264-sdr.m3u8\n" {
		t.Fatalf("variant body %q", b)
	}
	if _, ok, _ := o.OpenAsset(context.Background(), "ch", PlanBaseline, "4k-hevc-hdr.m3u8"); ok {
		t.Fatal("a variant the packager does not serve resolved")
	}
	if _, ok, _ := o.OpenAsset(context.Background(), "ch", PlanBaseline, "1080p-h264-sdr-seg00000001.m4s"); ok {
		t.Fatal("a segment went to MediaPlaylist, or resolved with no file")
	}
	if len(pk.asked) != 2 {
		t.Fatalf("MediaPlaylist asked for %v, want only the two .m3u8", pk.asked)
	}
}

// premiumSource is a GPU channel whose lineup warrants premium (4K HDR10 unless set), airing a 4K
// HDR programme. The encoder is whatever ffmpeg the test gives the packager.
type premiumSource struct {
	hdrItemSource
	premium FormatClass
	lookups atomic.Int32 // Premium calls
}

func (s *premiumSource) Premium(context.Context, string) FormatClass {
	s.lookups.Add(1)
	return s.premium
}

func (s *premiumSource) Output(_ context.Context, _ string, class FormatClass, _ int) (HostProfile, OutputProfile) {
	base := OutputProfile{Width: 1920, Height: 1080, FPS: 25, GOPSeconds: 2, TargetKbps: 8000, MaxKbps: 12000, AudioKbps: 160}
	out := base
	if class != FormatBaseline {
		out, _ = PremiumOutput(class, base)
	}
	return HostFor(EncoderNVENC, false, GPUFilters{Libplacebo: true}), out
}

// premiumFacts is a measured NVENC host: SDR 1080p at 6x, 4K HDR tone-map at 2.4x, and the premium
// class at 2.4x (a 0.5 GPU share); premium is left unmeasured when measured is false.
func premiumFacts(measured bool) BudgetFacts {
	f := BudgetFacts{Hardware: true, SessionLimit: 12, CPUAllowance: 8, Rungs: []int{1080}, Costs: map[CostKey]ClassCost{
		{Class: ClassSDR, Height: 1080}:                {Speed: 6, CPUCores: 0.2},
		HDRKey(ClassHDR4K, 1080, DefaultToneCurve):     {Speed: 2.4, CPUCores: 0.4},
		{Class: ClassPremium4K, Height: premiumHeight}: {Speed: 2.4, CPUCores: 0.5},
	}}
	if !measured {
		delete(f.Costs, CostKey{Class: ClassPremium4K, Height: premiumHeight})
	}
	return f
}

func sleepingFFmpeg(t *testing.T) string {
	t.Helper()
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return ffmpeg
}

// A premium variant waits for a client opt-in (#1512 G10): the master names it beside the baseline,
// from its output alone (HEVC Main 10 CODECS, 3840x2160, VIDEO-RANGE=PQ), so a baseline viewer
// never starts the premium encode.
func TestPackagerHLSMasterListsThePremiumWithoutStartingIt(t *testing.T) {
	source := &premiumSource{premium: Format4KHDR}
	m, err := NewPackagerHLS(source, sleepingFFmpeg(t), t.TempDir(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	budget := NewResourceBudget(func() BudgetFacts { return premiumFacts(true) })
	m.WithBudget(budget)
	t.Cleanup(m.Stop)

	lease, err := m.acquirePlaylist("ch", PlanBaseline, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	body, err := lease.snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	master := string(body)
	for _, want := range []string{
		",RESOLUTION=1920x1080,FRAME-RATE=25.000,VIDEO-RANGE=SDR\n1080p-h264-sdr.m3u8\n",
		`#EXT-X-STREAM-INF:BANDWIDTH=24160000,AVERAGE-BANDWIDTH=16160000,CODECS="hvc1.2.4.L150.90,mp4a.40.2",RESOLUTION=3840x2160,FRAME-RATE=25.000,VIDEO-RANGE=PQ` + "\n4k-hevc-hdr.m3u8\n",
	} {
		if !strings.Contains(master, want) {
			t.Errorf("master lacks %q:\n%s", want, master)
		}
	}
	m.mu.Lock()
	keys := len(m.channels)
	_, premiumRunning := m.channels[packagedKey{channel: "ch", format: Format4KHDR}]
	m.mu.Unlock()
	if keys != 1 || premiumRunning {
		t.Fatalf("a baseline tune started %d packagers (premium running: %v); want the baseline only", keys, premiumRunning)
	}
	if use := budget.Snapshot().InUse; use.Transcodes != 1 {
		t.Fatalf("ledger after a baseline tune = %+v, want one transcode", use)
	}
}

// Playing the premium variant starts the channel's second packager, admitted at the premium class's
// own measured price (NVENC 2.4x = a 0.5 GPU share, 0.5 cores), never the item's; a premium the
// channel does not air is not served and starts nothing.
func TestPackagerHLSPremiumPlaylistStartsThePremiumPackager(t *testing.T) {
	source := &premiumSource{premium: Format4KHDR}
	m, err := NewPackagerHLS(source, sleepingFFmpeg(t), t.TempDir(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	budget := NewResourceBudget(func() BudgetFacts { return premiumFacts(true) })
	m.WithBudget(budget)
	t.Cleanup(m.Stop)

	if _, ok, err := m.MediaPlaylist(t.Context(), "ch", PlanBaseline, "4k-hevc-sdr.m3u8"); ok || err != nil {
		t.Fatalf("4k-hevc-sdr on a 4K HDR channel = %v, %v; want not found", ok, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if _, _, err := m.MediaPlaylist(ctx, "ch", PlanBaseline, "4k-hevc-hdr.m3u8"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("premium playlist err = %v, want the wait for its first segment", err)
	}
	m.mu.Lock()
	c := m.channels[packagedKey{channel: "ch", format: Format4KHDR}]
	keys := len(m.channels)
	m.mu.Unlock()
	if c == nil || keys != 1 {
		t.Fatalf("premium play started %d packagers (premium: %v); want the premium only", keys, c != nil)
	}
	if !c.out.HDR || c.out.Height != premiumHeight {
		t.Fatalf("premium packager encodes %+v, want 4K HDR10", c.out)
	}
	use := budget.Snapshot().InUse
	if use.Transcodes != 1 || math.Abs(use.GPUShare-0.5) > 1e-9 || math.Abs(use.CPUCores-0.5) > 1e-9 {
		t.Fatalf("ledger = %+v, want one premium lease at 0.5 GPU and 0.5 cores", use)
	}
}

// Premium is admitted only on its own measurement (#1520): a measured host that has not measured
// premium drops it from the master and refuses a premium play (503) before any encoder runs; so does
// a host with no room left for it.
func TestPackagerHLSDropsPremiumThatDoesNotFit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		measured bool
		held     int // SDR leases already running
	}{
		{"premium unmeasured", false, 0},
		{"no room", true, 4}, // 0.8 of the GPU held; premium needs 0.5
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ran := filepath.Join(dir, "ran")
			ffmpeg := filepath.Join(dir, "ffmpeg")
			if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\ntouch "+ran+"\nexec sleep 5\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			budget := NewResourceBudget(func() BudgetFacts { return premiumFacts(tc.measured) })
			for range tc.held {
				if _, err := budget.Admit(t.Context(), AdmitRequest{Class: ClassSDR}); err != nil {
					t.Fatal(err)
				}
			}
			m, err := NewPackagerHLS(&premiumSource{premium: Format4KHDR}, ffmpeg, t.TempDir(), time.Hour, nil)
			if err != nil {
				t.Fatal(err)
			}
			m.WithBudget(budget)
			t.Cleanup(m.Stop)

			if _, _, err := m.MediaPlaylist(t.Context(), "ch", PlanBaseline, "4k-hevc-hdr.m3u8"); !errors.Is(err, ErrAtCapacity) {
				t.Fatalf("premium play err = %v, want ErrAtCapacity", err)
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := os.Stat(ran); err == nil {
				t.Fatal("an encoder started for a refused premium")
			}
			if v := m.premiumVariant(t.Context(), "ch"); v != nil {
				t.Fatalf("master offers %+v, want the premium dropped", *v)
			}
		})
	}
}

func TestPackagerHLSMediaPlaylistServesOnlyTheChannelsFormats(t *testing.T) {
	m := &PackagerHLS{channels: map[packagedKey]*packagedChannel{}, source: &premiumSource{}}
	for _, rel := range []string{"4k-hevc-sdr.m3u8", "4k-hevc-hdr.m3u8", "../1080p-h264-sdr.m3u8", "live.m3u8", "1080p-h264-sdr"} {
		if _, ok, err := m.MediaPlaylist(context.Background(), "ch", PlanBaseline, rel); ok || err != nil {
			t.Errorf("MediaPlaylist(%q) = %v, %v; want not found, without starting a packager", rel, ok, err)
		}
	}
	if len(m.channels) != 0 {
		t.Fatal("a rejected variant started a packager")
	}
}

func TestMasterPlaylistNamesEachVariant(t *testing.T) {
	got := string(masterPlaylist([]hlsVariant{
		{uri: "1080p-h264-sdr.m3u8", bandwidth: 12_160_000, average: 8_160_000, codecs: "avc1.640028,mp4a.40.2", width: 1920, height: 1080, rate: 25},
		{uri: "4k-hevc-hdr.m3u8", bandwidth: 40_000_000, average: 30_000_000, width: 3840, height: 2160, rate: 25},
	}))
	want := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=12160000,AVERAGE-BANDWIDTH=8160000,CODECS=\"avc1.640028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=25.000\n1080p-h264-sdr.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=40000000,AVERAGE-BANDWIDTH=30000000,RESOLUTION=3840x2160,FRAME-RATE=25.000\n4k-hevc-hdr.m3u8\n"
	if got != want {
		t.Fatalf("master:\n%s\nwant:\n%s", got, want)
	}
}

// boxCut passes whole top-level boxes and, stopped, ends the stream cleanly at the next box
// boundary: the packager never reads a partial fragment of an encoder it is replacing.
func TestBoxCutEndsAtABoxBoundary(t *testing.T) {
	box := func(typ string, body int) []byte {
		b := make([]byte, 8+body)
		b[3] = byte(8 + body)
		copy(b[4:], typ)
		return b
	}
	stream := slices.Concat(box("moof", 8), box("mdat", 40), box("moof", 8), box("mdat", 40))
	c := &boxCut{r: bytes.NewReader(stream)}
	first := make([]byte, 3) // smaller than a header: reads must still split correctly
	got := []byte{}
	for len(got) < 16+48 {
		n, err := c.Read(first)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, first[:n]...)
	}
	c.stop.Store(true)
	rest, err := io.ReadAll(c)
	if err != nil || len(rest) != 0 || !bytes.Equal(got, stream[:64]) {
		t.Fatalf("after a stop at a boundary: %d more bytes, err %v; want a clean EOF after the first fragment", len(rest), err)
	}

	c = &boxCut{r: bytes.NewReader(stream)}
	head := make([]byte, 20)
	if _, err := io.ReadFull(c, head); err != nil { // inside the first mdat
		t.Fatal(err)
	}
	c.stop.Store(true)
	rest, err = io.ReadAll(c)
	if err != nil || 20+len(rest) != 64 {
		t.Fatalf("a stop mid-box read %d bytes in all (err %v); want the box finished, then EOF at 64", 20+len(rest), err)
	}

	c = &boxCut{r: bytes.NewReader(stream[:30])}
	if _, err := io.ReadAll(c); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a stream cut mid-box: err = %v, want ErrUnexpectedEOF", err)
	}
}

// On a software host the item encoder's -progress drives the ladder (#1517): a slow encoder is
// stepped down, its stream ends at a box boundary, the lease is re-priced, and the airing resumes on
// the new rung with a hold instead of a slate. A GPU host's encoder is not watched.
func TestPackagerLadderStepsASlowItemDownAndResumesIt(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	// A 0.5x encoder: 50 ms of media per 100 ms, and 8-byte boxes on stdout.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argsFile + "\nt=0\nwhile :; do\n  printf '\\000\\000\\000\\010free'\n" +
		"  t=$((t+50000))\n  printf 'out_time_ms=%d\\nprogress=continue\\n' $t >&3 2>/dev/null\n  sleep 0.1\ndone\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(hdrItemSource{}, ffmpeg, t.TempDir(), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	m.ladderCfg = RungMonitorConfig{Settle: 100 * time.Millisecond, Window: 300 * time.Millisecond, DownFor: 400 * time.Millisecond}
	budget := NewResourceBudget(func() BudgetFacts { return softwareHDRFacts(8) })
	lease, err := budget.Admit(context.Background(), AdmitRequest{Class: ClassHDR4K})
	if err != nil || lease.SoftwareRung() != RungFull {
		t.Fatalf("admission: %v, rung %s; want full on 8 cores", err, lease.SoftwareRung())
	}
	host, out := hdrItemSource{}.Output(context.Background(), "ch", "", 0)
	sched := m.schedule(packagedKey{channel: "ch"}, host, out, lease, nil, slog.New(slog.DiscardHandler))
	at := time.Now()
	slot := packager.Slot{Frames: 100_000, AudioFrames: 100_000}

	item, err := sched(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := item.Open(t.Context(), slot)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(rc); done <- b }()
	select {
	case b := <-done:
		if len(b) == 0 || len(b)%8 != 0 {
			t.Fatalf("the stepped encoder's stream ended with %d bytes; want whole boxes", len(b))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a 0.5x encoder was never stepped down")
	}
	_ = rc.Close()
	if lease.SoftwareRung() != RungLight || budget.Snapshot().InUse.CPUCores > 3.6*heavyRungCosts[RungLight]+1e-9 {
		t.Fatalf("lease after the step: rung %s, %.3f cores; want rung 1 re-priced", lease.SoftwareRung(), budget.Snapshot().InUse.CPUCores)
	}

	resumed, err := sched(t.Context(), at.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Wait != ladderResumeWait {
		t.Errorf("resumed airing Wait = %s, want %s so it holds instead of slating", resumed.Wait, ladderResumeWait)
	}
	rc, err = resumed.Open(t.Context(), slot)
	if err != nil {
		t.Fatal(err)
	}
	var args []byte
	for deadline := time.Now().Add(5 * time.Second); strings.Count(string(args), "\n") < 2 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		args, _ = os.ReadFile(argsFile)
	}
	_ = rc.Close()
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], "-skip_loop_filter") || !strings.Contains(lines[1], "-skip_loop_filter:v all") ||
		!strings.Contains(lines[0], "-progress pipe:3") {
		t.Fatalf("encoder commands: want a watched rung-0 encode, then rung 1:\n%s", args)
	}
	if again, _ := sched(t.Context(), at.Add(3*time.Second)); again.Wait != 0 {
		t.Error("a resume without a new step still held")
	}

	gpu := m.schedule(packagedKey{channel: "gpu"}, HostFor(EncoderNVENC, true, GPUFilters{}), out, nil, nil, slog.New(slog.DiscardHandler))
	if item, err := gpu(t.Context(), at); err == nil {
		if rc, err := item.Open(t.Context(), slot); err == nil {
			for deadline := time.Now().Add(5 * time.Second); strings.Count(string(args), "\n") < 3 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				args, _ = os.ReadFile(argsFile)
			}
			_ = rc.Close()
		}
	}
	args, _ = os.ReadFile(argsFile)
	lines = strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(lines) != 3 || strings.Contains(lines[2], "-progress") {
		t.Fatalf("a GPU host's item encoder: want one unwatched command, got:\n%s", args)
	}
}
