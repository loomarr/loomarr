package playoutbench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -t before the first -i is an INPUT option: it capped the video and left the infinite sine audio
// running, so a 6 s clip encoded until the ffmpeg was killed (25 min holding the shared gate lock).
func TestClipArgsBoundTheOutput(t *testing.T) {
	for _, c := range Corpus() {
		args := c.Args("/x/" + c.Name + ".mkv")
		lastInput, dur := -1, -1
		for i, a := range args {
			switch a {
			case "-i":
				lastInput = i
			case "-t":
				dur = i
			}
		}
		if dur < lastInput {
			t.Errorf("%s: -t at %d precedes the last -i at %d, so it bounds one input, not the output: %v", c.Name, dur, lastInput, args)
		}
	}
}

func tinyClip() Clip {
	return Clip{Name: "tiny", Seconds: 1,
		VideoSrc: video(160, 90, "25", ""), VideoEnc: x264(sdr709...), AudioSrc: tone("stereo", ""), AudioEnc: acodec("aac")}
}

func TestGenerateClipTerminatesAtItsDuration(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	path := filepath.Join(t.TempDir(), "tiny.mkv")
	start := time.Now()
	if err := generateClip(context.Background(), ffmpeg, tinyClip(), path, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("a 1 s 160x90 clip took %s", d)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() > 1<<20 {
		t.Errorf("clip missing or unbounded: %v %v", fi, err)
	}
}

func TestGenerateClipTimeoutFailsLoudly(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := generateClip(context.Background(), fake, tinyClip(), filepath.Join(t.TempDir(), "x.mkv"), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want a timed-out error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the runaway child was not killed promptly")
	}
}

func TestRecipeHashKeysTheCorpusCache(t *testing.T) {
	a := recipeHash(Corpus())
	if a != recipeHash(Corpus()) {
		t.Fatal("the recipe hash must be stable")
	}
	changed := Corpus()
	changed[0].Seconds++
	if a == recipeHash(changed) {
		t.Error("changing a clip must change the cache key")
	}
	if got := CorpusDir("/base"); got != filepath.Join("/base", CorpusName+"-"+a[:12]) {
		t.Errorf("CorpusDir = %s", got)
	}
}
