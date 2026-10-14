package mediameasure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// chapterCheck is one measured chapter mark: YAVG (8-bit, limited range) of the first and last
// frame of the second around it, and the audio's mean level. Values are from live remuxes:
// an hour drama's act breaks and a 1940s crime film's scene-selection chapters.
type chapterCheck struct {
	firstY, lastY, meanDB float64
}

func chapterRunner(marks map[string]chapterCheck, order []string) Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name == "ffprobe" {
			return []byte(strings.Join(order, "\n") + "\n"), nil, nil
		}
		// The check window starts half a second before the mark.
		ss := args[slices.Index(args, "-ss")+1]
		for mark, c := range marks {
			var at float64
			_, _ = fmt.Sscan(mark, &at)
			if fmt.Sprintf("%.3f", at-0.5) == ss {
				stdout := fmt.Sprintf("frame:0\nlavfi.signalstats.YAVG=%g\nframe:23\nlavfi.signalstats.YAVG=%g\n", c.firstY, c.lastY)
				stderr := fmt.Sprintf("[Parsed_volumedetect_0 @ 0x1] mean_volume: %g dB\n", c.meanDB)
				return []byte(stdout), []byte(stderr), nil
			}
		}
		return nil, nil, fmt.Errorf("unexpected run %s %v", name, args)
	}
}

// A chapter mark becomes a break candidate only when the picture is black on both sides of it and
// the sound is not loud programme audio. Scene-selection chapters sit mid-picture.
func TestChapterBreaksKeepOnlyMarksThatSitInAFade(t *testing.T) {
	marks := map[string]chapterCheck{
		"961.794":  {16, 17.33, -33.8},  // act-break fade (its clock sting keeps it above -35 dB)
		"2017.349": {16, 19.82, -28.0},  // act-break fade, loudest measured
		"904.862":  {88.5, 92.5, -22.4}, // scene chapter in a bright scene
		"1653.819": {31.4, 32.5, -36.7}, // scene chapter: quiet, but the picture is not black
		"3512.008": {53.7, 52.8, -48.2}, // scene chapter: silent, but not black
	}
	order := []string{"0.000", "904.862", "961.794", "1653.819", "2017.349", "3512.008"}
	tools := Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe", Run: chapterRunner(marks, order)}
	got, err := tools.ChapterBreaks(context.Background(), "in.mkv", 6_000_000, 8<<30, nil, DefaultSampling())
	if err != nil {
		t.Fatal(err)
	}
	var at []int64
	for _, b := range got {
		at = append(at, b.AtMs)
		if b.Source != "chapter" || b.OverlapMs <= 0 || b.Confidence <= 0 {
			t.Fatalf("verified chapter %+v must carry its measured fade", b)
		}
	}
	if !slices.Equal(at, []int64{961_794, 2_017_349}) {
		t.Fatalf("kept chapter marks %v, want only the two fades", at)
	}
}

// A mark that sits in a loud scene is not a fade even when the picture happens to be dark.
func TestChapterFadeNeedsBlackOnBothSidesAndNoLoudAudio(t *testing.T) {
	for _, tc := range []struct {
		c    chapterCheck
		want bool
	}{
		{chapterCheck{16, 20.56, -37.5}, true},
		{chapterCheck{16, 30.8, -40}, false}, // fading in too fast: not black after the mark
		{chapterCheck{30.8, 16, -40}, false},
		{chapterCheck{16, 16, -22.4}, false}, // loud programme audio over a dark frame
	} {
		if got := chapterIsFade(tc.c.firstY, tc.c.lastY, tc.c.meanDB); got != tc.want {
			t.Errorf("chapterIsFade%+v = %v, want %v", tc.c, got, tc.want)
		}
	}
}

// fadeChapterFixture is 3 s of picture and tone, 2 s of black and silence, then 5 s of picture and
// tone, with chapter marks at 4 s (the middle of the fade) and 7 s (mid-picture).
func fadeChapterFixture(t *testing.T) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	meta := filepath.Join(dir, "meta.txt")
	body := ";FFMETADATA1\n"
	for _, c := range [][2]int{{0, 4000}, {4000, 7000}, {7000, 10000}} {
		body += "[CHAPTER]\nTIMEBASE=1/1000\nSTART=" + itoa(c[0]) + "\nEND=" + itoa(c[1]) + "\ntitle=c\n"
	}
	if err := os.WriteFile(meta, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fade-chapters.mkv")
	out, err := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=3",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:r=25:d=2",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=5",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=3",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=2",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=5",
		"-i", meta, "-map_metadata", "6",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0[v];[3:a][4:a][5:a]concat=n=3:v=0:a=1[a]",
		"-map", "[v]", "-map", "[a]", "-c:v", "mpeg4", "-g", "25", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	return path
}

// End to end with a real decode: the check reads the second around each mark and keeps only the
// mark inside the fade.
func TestChapterBreaks_RealDecodeKeepsTheMarkInsideAFade(t *testing.T) {
	got, err := DefaultTools("", "").ChapterBreaks(context.Background(), fadeChapterFixture(t), 10_000, 0, nil, DefaultSampling())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AtMs != 4000 || got[0].OverlapMs <= 0 {
		t.Fatalf("chapter breaks = %+v, want only the 4 s mark inside the fade", got)
	}
}
