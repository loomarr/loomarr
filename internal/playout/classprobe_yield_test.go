//go:build linux

package playout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fakeProbeFFmpeg writes an executable that stands in for ffmpeg: it records its pid and reports
// progress until killed, as a measurement run does.
func fakeProbeFFmpeg(t *testing.T, dir, pids string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho $$ >> '" + pids + "'\ni=0\nwhile :; do i=$((i+100000)); echo out_time_us=$i; echo progress=continue; sleep 0.05; done\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readPids(t *testing.T, path string) []int {
	t.Helper()
	raw, _ := os.ReadFile(path)
	var out []int
	for _, f := range strings.Fields(string(raw)) {
		if n, err := strconv.Atoi(f); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", within, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// niceOf reads a process's nice value from /proc/<pid>/stat (field 19).
func niceOf(t *testing.T, pid int) int {
	t.Helper()
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(raw[strings.LastIndexByte(string(raw), ')')+1:]))
	n, err := strconv.Atoi(fields[16])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The first-boot probe is low-priority background work (#1512): it runs niced, and a viewer who tunes
// in mid-measurement neither waits on it nor shares the encoder with it. The measurement is killed
// the moment the live transcode is admitted, nothing restarts while the viewer watches, and the probe
// finishes its table once playback is idle again.
func TestProbeClassCosts_YieldsImmediatelyToALiveTune(t *testing.T) {
	dir := t.TempDir()
	pids := filepath.Join(dir, "pids")
	clip := probeClips[0]
	if err := os.WriteFile(filepath.Join(dir, clip.fileName()), []byte("clip"), 0o600); err != nil {
		t.Fatal(err)
	}
	budget := NewResourceBudget(nvencFacts)
	out := Profile{Width: 1280, Height: 720, Framerate: 25, VideoBitrate: 3000, AudioBitrate: 128, Encoder: EncoderSoftware}
	cfg := ClassProbeConfig{
		FFmpeg: fakeProbeFFmpeg(t, dir, pids), ClipDir: dir, Encoder: EncoderSoftware,
		Outputs: []Profile{out}, Classes: []StreamClass{clip.class}, Foreground: budget,
	}
	done := make(chan ClassProbeResult, 1)
	go func() { done <- ProbeClassCosts(t.Context(), cfg) }()

	waitFor(t, 2*time.Second, "the probe's measurement to start", func() bool { return len(readPids(t, pids)) == 1 })
	probe := readPids(t, pids)[0]
	if n := niceOf(t, probe); n != probeNice {
		t.Errorf("probe encoder runs at nice %d, want %d (background priority)", n, probeNice)
	}

	start := time.Now()
	live, err := budget.Reserve(AdmitRequest{Class: ClassSDR})
	if err != nil {
		t.Fatalf("the tune was refused during the probe: %v", err)
	}
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Errorf("the tune waited %v on the probe", took)
	}
	waitFor(t, 500*time.Millisecond, "the probe's encoder to be killed", func() bool {
		return errors.Is(syscall.Kill(probe, 0), syscall.ESRCH)
	})
	time.Sleep(400 * time.Millisecond)
	if n := len(readPids(t, pids)); n != 1 {
		t.Fatalf("the probe started %d more encoders while a viewer was watching", n-1)
	}

	live.Release()
	select {
	case res := <-done:
		if _, ok := res.Costs[CostKey{Class: clip.class, Height: out.Height}]; !ok {
			t.Fatalf("the probe did not finish its measurement after playback went idle: %+v", res)
		}
	case <-time.After(probeWindow + 3*time.Second):
		t.Fatal("the probe never resumed after playback went idle")
	}
	if n := len(readPids(t, pids)); n != 2 {
		t.Errorf("measurement runs = %d, want 2 (the yielded one, then the retry)", n)
	}
}

// fakeSessionFFmpeg stands in for an encoder with a hard session cap: each run takes a free slot
// directory and encodes, or fails like a GeForce past its NVENC limit.
func fakeSessionFFmpeg(t *testing.T, dir, slots string, capacity int) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nfor i in $(seq 1 " + strconv.Itoa(capacity) + "); do if mkdir '" + slots + "'/slot_$i 2>/dev/null; then echo frame=1; exec sleep 30; fi; done\n" +
		"echo 'OpenEncodeSessionEx failed: out of memory (10)' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// The recorded session limit is the encoder's capacity, not what happened to be free (#1512): the
// probe settles until sessions released by the class measurements are gone, then counts the ones
// still in use (another app's encode) alongside the ones it opened. On the dev GeForce the unsettled
// ramp read 11 or 12 from run to run.
func TestProbeSessionLimit_SettlesAndCountsSessionsInUse(t *testing.T) {
	dir := t.TempDir()
	slots := filepath.Join(dir, "slots")
	if err := os.MkdirAll(filepath.Join(slots, "slot_1"), 0o700); err != nil { // another app's session
		t.Fatal(err)
	}
	// The driver still lists a just-finished class measurement on the first read.
	readings := []int{2, 1, 1}
	var reads atomic.Int32
	inUse := func(context.Context) (int, bool) {
		i := int(reads.Add(1)) - 1
		return readings[min(i, len(readings)-1)], true
	}
	limit, err := ProbeSessionLimit(t.Context(), SessionProbeConfig{
		FFmpeg: fakeSessionFFmpeg(t, dir, slots, 4), Encoder: EncoderNVENC, Bound: SessionProbeBound, InUse: inUse,
	})
	if err != nil {
		t.Fatal(err)
	}
	if limit != 4 {
		t.Fatalf("session limit = %d, want the encoder's capacity 4 (3 opened + 1 already in use)", limit)
	}
	if reads.Load() < 3 {
		t.Errorf("probed after %d in-use readings; it must wait for the count to settle", reads.Load())
	}
}

// fakeTonemapFFmpeg measures like a healthy encoder at full speed, writes the sample file it is
// asked for, and reports every sampled frame with the given luma, as signalstats would.
func fakeTonemapFFmpeg(t *testing.T, dir string, ymax int, yavg float64) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	frame := "echo lavfi.signalstats.YAVG=" + strconv.FormatFloat(yavg, 'f', 1, 64) + "; echo lavfi.signalstats.YMAX=" + strconv.Itoa(ymax)
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"*signalstats*) for i in 1 2 3; do echo frame:$i; " + frame + "; done ;;\n" +
		"*-progress*) i=0; while :; do i=$((i+200000)); echo out_time_us=$i; echo progress=continue; sleep 0.05; done ;;\n" +
		"*) eval out=\\${$#}; : > \"$out\" ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// #1516: tonemap_vaapi on an Arc exited cleanly at normal speed while every frame was black. The
// self-check reads the tone-mapped picture, so a black result fails it (a red Diagnostics error)
// however fast and clean the encode was.
func TestProbeClassCosts_TonemapSelfCheckRejectsABlackPicture(t *testing.T) {
	var clip probeClip
	for _, c := range probeClips {
		if c.class == ClassHDR4K {
			clip = c
		}
	}
	for _, tc := range []struct {
		name   string
		ymax   int
		yavg   float64
		wantOK bool
	}{
		{"black at full speed", 16, 16.0, false},
		{"a real picture", 235, 118.4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, clip.fileName()), []byte("clip"), 0o600); err != nil {
				t.Fatal(err)
			}
			res := ProbeClassCosts(t.Context(), ClassProbeConfig{
				FFmpeg: fakeTonemapFFmpeg(t, dir, tc.ymax, tc.yavg), ClipDir: dir, Encoder: EncoderSoftware, CPUTonemap: true,
				Outputs: []Profile{{Width: 1280, Height: 720, Framerate: 25, VideoBitrate: 3000, AudioBitrate: 128, Encoder: EncoderSoftware}},
				Classes: []StreamClass{ClassHDR4K},
			})
			check := res.Tonemap
			if !check.Ran || check.OK != tc.wantOK {
				t.Fatalf("tone-map self-check = %+v, want ran and OK=%v", check, tc.wantOK)
			}
			if !tc.wantOK && !strings.Contains(check.Detail, "black") {
				t.Errorf("a black picture's detail must say so: %q", check.Detail)
			}
		})
	}
}

func TestJudgeTonemapPicture(t *testing.T) {
	frame := func(ymax int, yavg float64) string {
		return "frame:0\nlavfi.signalstats.YAVG=" + strconv.FormatFloat(yavg, 'f', 1, 64) + "\nlavfi.signalstats.YMAX=" + strconv.Itoa(ymax) + "\n"
	}
	for _, tc := range []struct {
		name, metadata string
		ok             bool
	}{
		{"healthy", frame(235, 110) + frame(230, 112), true},
		{"all black (Arc tonemap_vaapi)", frame(16, 16) + frame(16, 16), false},
		{"one black frame among good ones", frame(235, 110) + frame(16, 16), false},
		{"crushed near-black", frame(90, 18), false},
		{"blown out", frame(235, 232), false},
		{"no frames", "", false},
	} {
		if _, err := judgeTonemapPicture(tc.metadata); (err == nil) != tc.ok {
			t.Errorf("%s: judge = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

// The probe measures HDR on the live curve's graph and keys the cell by that curve (#1516), so the
// ledger admits HDR against the cost of the curve the operator chose.
func TestProbeClassCosts_KeysHDRByTheLiveToneCurve(t *testing.T) {
	var clip probeClip
	for _, c := range probeClips {
		if c.class == ClassHDR4K {
			clip = c
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, clip.fileName()), []byte("clip"), 0o600); err != nil {
		t.Fatal(err)
	}
	const curve ToneCurve = "bt2390"
	res := ProbeClassCosts(t.Context(), ClassProbeConfig{
		FFmpeg: fakeTonemapFFmpeg(t, dir, 235, 118.4), ClipDir: dir, Encoder: EncoderSoftware, CPUTonemap: true,
		Outputs: []Profile{{Width: 1280, Height: 720, Framerate: 25, VideoBitrate: 3000, AudioBitrate: 128, Encoder: EncoderSoftware}},
		Classes: []StreamClass{ClassHDR4K}, Curve: curve,
	})
	if _, ok := res.Costs[HDRKey(ClassHDR4K, 720, curve)]; !ok {
		t.Fatalf("HDR cost not keyed by the live curve %q: %+v", curve, res.Costs)
	}
	if _, ok := res.Costs[HDRKey(ClassHDR4K, 720, DefaultToneCurve)]; ok {
		t.Errorf("a %s measurement was stored as Hable's", curve)
	}
}
