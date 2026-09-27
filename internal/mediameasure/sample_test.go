package mediameasure

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSampleLoudness_EstimatesAToneFromShortWindows(t *testing.T) {
	path := fadeFixture(t)
	info, _ := os.Stat(path)
	s := DefaultSampling()
	s.LoudnessWindows, s.LoudnessWindow = 4, 1500*time.Millisecond
	lufs, peak, err := DefaultTools("", "").SampleLoudness(context.Background(), path, 7000, info.Size(), s)
	if err != nil {
		t.Fatal(err)
	}
	if lufs == nil || *lufs > -10 || *lufs < -60 || peak == nil {
		t.Fatalf("lufs %v peak %v, want a finite estimate for a tone", lufs, peak)
	}
}

func TestSampleWindow_ShrinksToTheByteBudget(t *testing.T) {
	// A 100 Mbps remux: 12.5 MB/s. A 256 MiB budget over 12 windows is ~1.7 s each, never 20 s.
	got := sampleWindow(20*time.Second, 12, 256<<20, 12_500_000)
	if got < time.Second || got > 2*time.Second {
		t.Fatalf("window = %s, want ~1.7 s so the whole sample stays inside the byte budget", got)
	}
	if got := sampleWindow(20*time.Second, 12, 256<<20, 500_000); got != 20*time.Second {
		t.Fatalf("a light file window = %s, want the full 20 s", got)
	}
}

// The fixture's chapter marks (4 s and 7 s) sit in a bright test pattern over a steady tone. A real
// ffmpeg check of each mark must find no fade, so neither becomes a break (a chapter mark alone is
// not evidence of one).
func TestChapters_MarksInsideAScreenfulOfPictureAreNotBreaks(t *testing.T) {
	tools := DefaultTools("", "")
	got, err := tools.ChapterBreaks(context.Background(), chapterFixture(t), 10_000, 0, nil, DefaultSampling())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("chapter breaks = %+v, want none: both marks are mid-picture", got)
	}
}

func TestTargetedBreaks_FindsTheFadeAtTheDuePointOnly(t *testing.T) {
	path := fadeFixture(t)
	info, _ := os.Stat(path)
	s := DefaultSampling()
	s.BreakEvery, s.BreakHalfWindow = 3500*time.Millisecond, 1500*time.Millisecond
	got, err := DefaultTools("", "").TargetedBreaks(context.Background(), path, 7000, info.Size(), nil, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AtMs < 3300 || got[0].AtMs > 3700 || got[0].Source != "fade" {
		t.Fatalf("breaks = %+v, want the fade at ~3.5 s", got)
	}
}

func TestTargetedBreaks_SkipsWhenTheBudgetCannotCoverAWindow(t *testing.T) {
	path := fadeFixture(t)
	s := DefaultSampling()
	s.BreakEvery, s.BreakHalfWindow, s.BudgetBytes = 3500*time.Millisecond, 1500*time.Millisecond, 1
	got, err := DefaultTools("", "").TargetedBreaks(context.Background(), path, 7000, 1<<40, nil, s)
	if err != nil || len(got) != 0 {
		t.Fatalf("breaks = %+v err %v, want none when a window would exceed the byte budget", got, err)
	}
}
