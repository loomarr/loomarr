package filler

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// The split corpus (#3838 step 6): reels stitched from known single ads, so the true cut points
// are known without labelling. Every reel runs through the real detectors and the real segment
// builder, and the gate is that no cut which may air unattended holds two ads.

// corpusAd is one known single ad: its picture, its tone and its length.
type corpusAd struct {
	video string
	freq  int
	secs  float64
}

var corpusAds = []corpusAd{
	{"testsrc2", 330, 15},
	{"testsrc", 440, 30},
	{"smptebars", 550, 20},
	{"rgbtestsrc", 660, 15},
	{"yuvtestsrc", 770, 30},
}

// corpusGap is how two ads are joined on a reel.
type corpusGap struct {
	name string
	// piece returns the lavfi video and audio between two ads, or "" for a hard cut.
	piece func(prev corpusAd) (video, audio string, secs float64)
	// fade dims the last half second of each ad to black and silence, the way a broadcast spot ends.
	fade bool
	// dip puts a fifth of a second of black and silence 12 s into each 30 s ad, the way a spot can
	// cut to black between its own scenes: a cut there would air half an ad.
	dip bool
}

const corpusSize, corpusRate = "160x90", "30000/1001"

var corpusGaps = []corpusGap{
	{name: "hard cut", piece: func(corpusAd) (string, string, float64) { return "", "", 0 }},
	{name: "black+silent 0.5s", piece: func(corpusAd) (string, string, float64) {
		return "color=c=black", "anullsrc=channel_layout=stereo:sample_rate=48000", 0.5
	}},
	{name: "black+silent 4 frames", piece: func(corpusAd) (string, string, float64) {
		return "color=c=black", "anullsrc=channel_layout=stereo:sample_rate=48000", 0.134
	}},
	{name: "black, sound runs on", piece: func(p corpusAd) (string, string, float64) {
		return "color=c=black", fmt.Sprintf("sine=frequency=%d:sample_rate=48000", p.freq), 0.5
	}},
	{name: "silent, picture runs on", piece: func(p corpusAd) (string, string, float64) {
		return p.video, "anullsrc=channel_layout=stereo:sample_rate=48000", 0.5
	}},
	{name: "fade out, hard cut", fade: true, piece: func(corpusAd) (string, string, float64) { return "", "", 0 }},
	{name: "clean gaps, ads dip", dip: true, piece: func(corpusAd) (string, string, float64) {
		return "color=c=black", "anullsrc=channel_layout=stereo:sample_rate=48000", 0.5
	}},
}

type corpusSpan struct{ startMs, endMs int64 }

// buildCorpusReel stitches the ads with one kind of gap, and returns the reel and each ad's span.
func buildCorpusReel(t *testing.T, dir string, gap corpusGap) (string, []corpusSpan) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	var args, labels []string
	var spans []corpusSpan
	n := 0
	at := 0.0
	add := func(video, audio string, secs float64, fade bool) {
		args = append(args,
			"-f", "lavfi", "-i", withOptions(video, fmt.Sprintf("size=%s:rate=%s:duration=%.3f", corpusSize, corpusRate, secs)),
			"-f", "lavfi", "-i", withOptions(audio, fmt.Sprintf("duration=%.3f", secs)))
		v, a := fmt.Sprintf("[%d:v]", 2*n), fmt.Sprintf("[%d:a]", 2*n+1)
		vf, af := "format=yuv420p,setsar=1", "aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo"
		if fade {
			vf += fmt.Sprintf(",fade=t=out:st=%.3f:d=0.5", secs-0.5)
			af += fmt.Sprintf(",afade=t=out:st=%.3f:d=0.5", secs-0.5)
		}
		labels = append(labels, fmt.Sprintf("%s%s[v%d];%s%s[a%d]", v, vf, n, a, af, n))
		n++
		at += secs
	}
	for i, ad := range corpusAds {
		if i > 0 {
			if video, audio, secs := gap.piece(corpusAds[i-1]); secs > 0 {
				add(video, audio, secs, false)
			}
		}
		start := at
		tone := fmt.Sprintf("sine=frequency=%d:sample_rate=48000", ad.freq)
		if gap.dip && ad.secs == 30 {
			add(ad.video, tone, 12, false)
			add("color=c=black", "anullsrc=channel_layout=stereo:sample_rate=48000", 0.2, false)
			add(ad.video, tone, ad.secs-12.2, gap.fade)
		} else {
			add(ad.video, tone, ad.secs, gap.fade)
		}
		spans = append(spans, corpusSpan{int64(start * 1000), int64(at * 1000)})
	}
	var concat strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&concat, "[v%d][a%d]", i, i)
	}
	fmt.Fprintf(&concat, "concat=n=%d:v=1:a=1[v][a]", n)
	out := filepath.Join(dir, strings.NewReplacer(" ", "-", "+", "", ",", "", ".", "").Replace(gap.name)+".mp4")
	args = append([]string{"-nostdin", "-v", "error"}, args...)
	args = append(args, "-filter_complex", strings.Join(labels, ";")+";"+concat.String(),
		"-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k", "-ar", "48000", "-ac", "2", "-y", out)
	if b, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("build %s reel: %v: %s", gap.name, err, b)
	}
	return out, spans
}

// withOptions appends lavfi options to a source that may already carry some.
func withOptions(src, opts string) string {
	if strings.Contains(src, "=") {
		return src + ":" + opts
	}
	return src + "=" + opts
}

// adsIn counts the ads a span holds more than a second of.
func adsIn(s split.Segment, ads []corpusSpan) int {
	n := 0
	for _, ad := range ads {
		if min(s.EndMs, ad.endMs)-max(s.StartMs, ad.startMs) > 1000 {
			n++
		}
	}
	return n
}

// partOfAnAd reports whether a span inside one ad misses more than a second of it: half an ad.
func partOfAnAd(s split.Segment, ads []corpusSpan) bool {
	for _, ad := range ads {
		if min(s.EndMs, ad.endMs)-max(s.StartMs, ad.startMs) > 1000 {
			return (ad.endMs-ad.startMs)-(min(s.EndMs, ad.endMs)-max(s.StartMs, ad.startMs)) > 1000
		}
	}
	return false
}

// What the boundary evidence alone should do with each join. Black and silence cannot see a hard
// cut, and cannot tell a dip inside an ad from a short gap between two: those cuts are confident
// on boundaries and wrong, and only the judgement of the whole cut (#4409) may refuse them.
const (
	corpusRecovers  = "every ad is a confident cut of its own"
	corpusUncertain = "no cut is confident"
	corpusNeedsLook = "boundaries are confident and wrong; the look at the whole cut must refuse it"
)

var corpusWant = map[string]string{
	"hard cut":                corpusNeedsLook,
	"black+silent 0.5s":       corpusRecovers,
	"black+silent 4 frames":   corpusRecovers,
	"black, sound runs on":    corpusUncertain,
	"silent, picture runs on": corpusUncertain,
	"fade out, hard cut":      corpusRecovers,
	"clean gaps, ads dip":     corpusNeedsLook,
}

// The boundary half of the split corpus: what the detectors and the confidence bar make of each
// join. It pins recall on clean joins and that a join one detector alone saw is never confident,
// and it records the joins only the whole-cut judgement can catch.
func TestSplitCorpus_BoundaryEvidencePerJoin(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	dir := t.TempDir()
	mt := tools.NewFFmpegTools(ffmpeg, ffprobe, "", "", dir)
	pol := &AutoSplitPolicy{Enabled: func() bool { return true }, MinConfidence: func() int { return 85 }}
	const minClip, maxClip = 10 * time.Second, 120 * time.Second

	for _, gap := range corpusGaps {
		reel, ads := buildCorpusReel(t, dir, gap)
		durationMs := ads[len(ads)-1].endMs
		black, silence, err := mt.Boundaries(context.Background(), reel, 0, durationMs)
		if err != nil {
			t.Fatal(err)
		}
		segs, _ := segmentsFromBoundaries(durationMs, sourcedGaps(black, silence), newSegmentFloor(minClip))
		scoreBoundaries(segs)
		var airs, merged, whole, fragments int
		for _, s := range segs {
			// The look and the sound are not what this corpus measures: only where it was cut.
			s.LookedRole, s.Whole, s.Sound = split.RoleCommercial, split.WholeOne, cleanSound()
			if cutVerdict(s, pol, minClip, maxClip) != AutoSplitOK {
				continue
			}
			airs++
			switch {
			case adsIn(s, ads) > 1:
				merged++
			case partOfAnAd(s, ads):
				fragments++
			default:
				whole++
			}
		}
		t.Logf("%-24s %d ads → %d cuts, %d confident: %d single ads, %d merged, %d part of an ad",
			gap.name, len(ads), len(segs), airs, whole, merged, fragments)
		switch want := corpusWant[gap.name]; want {
		case corpusRecovers:
			if whole != len(ads) || merged+fragments > 0 {
				t.Errorf("%s: %d of %d ads recovered, %d merged, %d part of an ad; want %s",
					gap.name, whole, len(ads), merged, fragments, want)
			}
		case corpusUncertain:
			if airs > 0 {
				t.Errorf("%s: %d confident cuts; want %s", gap.name, airs, want)
			}
		case corpusNeedsLook:
			if merged+fragments == 0 {
				t.Errorf("%s: no wrong confident cut; the corpus no longer exercises the whole-cut judgement", gap.name)
			}
		default:
			t.Fatalf("%s: no expectation recorded", gap.name)
		}
	}
}
