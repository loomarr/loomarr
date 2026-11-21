//go:build eval

package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The whole-cut eval (#4409 PR C): does the install's vision model, asked the production split
// prompt over the production frames, tell one whole spot from a cut that is not one?
//
// Real single ads from a folder (LOOMARR_SPLIT_EVAL_ADS) become three kinds of cut:
//   - one:     an ad on its own, exactly as a clean cut would hold it;
//   - several: two ads joined by a hard cut, which black and silence cannot see (the split corpus);
//   - part:    the first or second half of an ad, as a cut at a dip inside one would hold it.
//
// Every cut sits inside a reel, between other ads, as the split stage meets it. Each is judged by
// the production verdict (`cutVerdict`, with edges taken as corroborated), so the gate is on what
// would air: a cut that is not one spot and would be made (LOOMARR_SPLIT_EVAL_MAX_FALSE_ONE,
// default 0.05). Dropping a real single ad only costs inventory; that rate is reported, not gated.
//
// It spends inference: run it by hand, never in CI. The report (LOOMARR_SPLIT_EVAL_OUT) holds
// counts only, never a file name, title or the model's prose: the repo is public.
//
//	LOOMARR_SPLIT_EVAL_ADS=/path/to/ads LLM_PROVIDER=custom LLM_URL=... LLM_MODEL=... \
//	  LLM_API_KEY_FILE=... go test -tags eval -run TestSplitWholeCutEval -v -timeout 2h ./internal/filler/

type wholeCutCase struct {
	kind           string // one | several | part
	file           string
	startMs, endMs int64
}

type wholeCutTally struct {
	Cases int `json:"cases"`
	// Answers is the vision look's whole-cut answer; Verdicts is why the cut would or would not be
	// made ("ok" when made).
	Answers  map[string]int `json:"answers"`
	Verdicts map[string]int `json:"verdicts"`
	// Roles is the role the look gave ("" when none).
	Roles map[string]int `json:"roles"`
	// SoundClean counts cuts whose sound alone says one whole spot.
	SoundClean int `json:"soundClean"`
	WouldCut   int `json:"wouldCut"`
	Failed     int `json:"failed"`
}

type wholeCutReport struct {
	Model       string                    `json:"model"`
	Prompt      string                    `json:"promptVersion"`
	Keyframes   int                       `json:"keyframes"`
	Ads         int                       `json:"ads"`
	Kinds       map[string]*wholeCutTally `json:"kinds"`
	NotOneCases int                       `json:"notOneCases"`
	// FalseOne is the vision answer alone; FalseCut is what would air.
	FalseOne      int     `json:"falseOne"`
	FalseCut      int     `json:"falseCut"`
	FalseCutRate  float64 `json:"falseCutRate"`
	CutRecall     float64 `json:"cutRecall"`
	MaxFalseCut   float64 `json:"maxFalseCut"`
	ElapsedSecond int     `json:"elapsedSeconds"`
}

func TestSplitWholeCutEval(t *testing.T) {
	adsDir := os.Getenv("LOOMARR_SPLIT_EVAL_ADS")
	if adsDir == "" {
		t.Skip("LOOMARR_SPLIT_EVAL_ADS is not set")
	}
	provider, model := wholeCutEvalProvider(t)
	maxFalseCut := envFloat(t, "LOOMARR_SPLIT_EVAL_MAX_FALSE_ONE", 0.05)
	ffmpeg, ffprobe, work, ads, cases := wholeCutEvalCases(t, adsDir)

	ff := tools.NewFFmpegTools(ffmpeg, ffprobe, "", "", work)
	stage := NewSplitStage(nil, nil).WithSegmentVision(&SegmentVision{
		Tools: ff, Loudness: ff, Provider: provider, Taxa: seedTaxa{}, Budget: func() int { return 1 },
	})
	pol := &AutoSplitPolicy{Enabled: func() bool { return true }}
	report := wholeCutReport{
		Model: model, Prompt: splitVisionPromptVersion, Keyframes: split.WholeCutKeyframes,
		Ads: len(ads), Kinds: map[string]*wholeCutTally{}, MaxFalseCut: maxFalseCut,
	}
	started := time.Now()
	for n, c := range cases {
		tally := report.Kinds[c.kind]
		if tally == nil {
			tally = &wholeCutTally{Answers: map[string]int{}, Verdicts: map[string]int{}, Roles: map[string]int{}}
			report.Kinds[c.kind] = tally
		}
		tally.Cases++
		segs := []split.Segment{{StartMs: c.startMs, EndMs: c.endMs, BoundaryConfidence: 100}}
		pass := stage.groundAt(context.Background(), clipstore.Clip{}, c.file, structureSource(c.endMs), segs)
		if pass.Looked != 1 || segs[0].Sound == nil {
			tally.Failed++
			t.Logf("case %d (%s): the look or the sound did not complete", n, c.kind)
			continue
		}
		s := segs[0]
		tally.Answers[string(s.Whole)]++
		if s.Sound.Clean() {
			tally.SoundClean++
		}
		verdict := cutVerdict(s, pol, 0, 120*time.Second)
		cut := verdict == AutoSplitOK
		if cut {
			tally.WouldCut++
			tally.Verdicts["ok"]++
		} else {
			tally.Verdicts[string(verdict)]++
		}
		if c.kind != "one" {
			report.NotOneCases++
			if s.Whole == split.WholeOne {
				report.FalseOne++
			}
			if cut {
				report.FalseCut++
			}
		}
		tally.Roles[string(s.LookedRole)]++
		t.Logf("case %d/%d %-7s -> %-7s role=%q sound=%v cut=%v", n+1, len(cases), c.kind, s.Whole, s.LookedRole, s.Sound.Clean(), cut)
	}
	report.ElapsedSecond = int(time.Since(started).Seconds())
	if report.NotOneCases > 0 {
		report.FalseCutRate = float64(report.FalseCut) / float64(report.NotOneCases)
	}
	if one := report.Kinds["one"]; one != nil && one.Cases > 0 {
		report.CutRecall = float64(one.WouldCut) / float64(one.Cases)
	}

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("whole-cut eval:\n%s", raw)
	if out := os.Getenv("LOOMARR_SPLIT_EVAL_OUT"); out != "" {
		if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for kind, tally := range report.Kinds {
		if tally.Failed > 0 {
			t.Errorf("%d %s looks did not complete; the eval measured fewer cuts than it built", tally.Failed, kind)
		}
	}
	if report.FalseCutRate > maxFalseCut {
		t.Errorf("%d of %d cuts that are not one spot would be made (%.1f%% > %.1f%%): those would air",
			report.FalseCut, report.NotOneCases, 100*report.FalseCutRate, 100*maxFalseCut)
	}
}

func wholeCutEvalProvider(t *testing.T) (llm.VisionProvider, string) {
	t.Helper()
	base, model := os.Getenv("LLM_URL"), os.Getenv("LLM_MODEL")
	if base == "" || model == "" {
		t.Skip("LLM_URL and LLM_MODEL are not set")
	}
	key := os.Getenv("LLM_API_KEY")
	if file := os.Getenv("LLM_API_KEY_FILE"); key == "" && file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read LLM_API_KEY_FILE: %v", err)
		}
		key = strings.TrimSpace(string(raw))
	}
	kind := os.Getenv("LLM_PROVIDER")
	if kind == "ollama" {
		return llm.NewOllama(base, model), model
	}
	return llm.NewOpenAIForProvider(kind, base, model, key), model
}

func listAds(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && !strings.HasSuffix(e.Name(), ".json") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// normalizeAd re-encodes an ad to one shape so two can be joined by stream copy, which is what a
// hard cut in a broadcast reel is. An ad without sound gets silence, as it would in a reel.
func normalizeAd(t *testing.T, ffmpeg, ffprobe, src, dst string) {
	t.Helper()
	args := []string{"-v", "error", "-y", "-i", src}
	hasAudio := probeHasAudio(t, ffprobe, src)
	if !hasAudio {
		args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
	}
	args = append(args,
		"-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30,format=yuv420p",
		"-map", "0:v:0")
	if hasAudio {
		args = append(args, "-map", "0:a:0")
	} else {
		args = append(args, "-map", "1:a:0", "-shortest")
	}
	args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
		"-c:a", "aac", "-ar", "48000", "-ac", "2", dst)
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("normalize an ad: %v\n%s", err, out)
	}
}

// wholeCutEvalCases normalizes the ads and builds the cases both evals measure.
func wholeCutEvalCases(t *testing.T, adsDir string) (ffmpeg, ffprobe, work string, ads []string, cases []wholeCutCase) {
	t.Helper()
	var err error
	if ffmpeg, err = exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if ffprobe, err = exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	limit := envInt(t, "LOOMARR_SPLIT_EVAL_LIMIT", 20)
	ads = listAds(t, adsDir)
	if len(ads) < 3 {
		t.Fatalf("need at least three ads in %s, found %d", adsDir, len(ads))
	}
	if len(ads) > 2*limit {
		ads = ads[:2*limit]
	}
	work = t.TempDir()
	norm := make([]string, len(ads))
	durs := make([]int64, len(ads))
	for i, ad := range ads {
		norm[i] = filepath.Join(work, fmt.Sprintf("n%03d.mp4", i))
		normalizeAd(t, ffmpeg, ffprobe, ad, norm[i])
		durs[i] = probeMs(t, ffprobe, norm[i])
	}
	return ffmpeg, ffprobe, work, ads, buildWholeCutCases(t, ffmpeg, work, norm, durs, limit)
}

// buildWholeCutCases puts every cut inside a reel, between other ads, as the split stage meets it:
// the frames and sound just past each edge belong to a neighbour, not to the end of a file.
func buildWholeCutCases(t *testing.T, ffmpeg, work string, norm []string, durs []int64, limit int) []wholeCutCase {
	t.Helper()
	n := len(norm)
	reel := func(name string, idx ...int) string {
		dst := filepath.Join(work, name)
		files := make([]string, len(idx))
		for k, i := range idx {
			files[k] = norm[i]
		}
		joinHard(t, ffmpeg, work, dst, files...)
		return dst
	}
	prev := func(i int) int { return (i + n - 1) % n }
	next := func(i, after int) int { return (i + after + 1) % n }

	var cases []wholeCutCase
	for i := 0; i < n && i < limit; i++ {
		file := reel(fmt.Sprintf("one%03d.mp4", i), prev(i), i, next(i, 0))
		start := durs[prev(i)]
		cases = append(cases, wholeCutCase{kind: "one", file: file, startMs: start, endMs: start + durs[i]})
	}
	for i := 0; i+1 < n && len(cases) < 2*limit; i += 2 {
		file := reel(fmt.Sprintf("several%03d.mp4", i), prev(i), i, i+1, next(i, 1))
		start := durs[prev(i)]
		cases = append(cases, wholeCutCase{kind: "several", file: file, startMs: start, endMs: start + durs[i] + durs[i+1]})
	}
	for i := 0; i < n && i < limit; i++ {
		file := filepath.Join(work, fmt.Sprintf("one%03d.mp4", i)) // the same reel as the whole cut
		start, half := durs[prev(i)], durs[i]/2
		c := wholeCutCase{kind: "part", file: file, startMs: start, endMs: start + half}
		if i%2 == 1 {
			c.startMs, c.endMs = start+half, start+durs[i]
		}
		cases = append(cases, c)
	}
	return cases
}

// joinHard joins ads by stream copy: a hard cut, as in a broadcast reel.
func joinHard(t *testing.T, ffmpeg, dir, dst string, files ...string) {
	t.Helper()
	var list strings.Builder
	for _, f := range files {
		fmt.Fprintf(&list, "file '%s'\n", f)
	}
	listFile := filepath.Join(dir, filepath.Base(dst)+".txt")
	if err := os.WriteFile(listFile, []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", listFile,
		"-c", "copy", dst).CombinedOutput(); err != nil {
		t.Fatalf("join ads: %v\n%s", err, out)
	}
}

func probeMs(t *testing.T, ffprobe, path string) int64 {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		t.Fatalf("probe duration: %v", err)
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("probe duration: %v", err)
	}
	return int64(sec * 1000)
}

func probeHasAudio(t *testing.T, ffprobe, path string) bool {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "a", "-show_entries",
		"stream=index", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("probe audio: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	if v := os.Getenv(name); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("%s=%q: want a positive integer", name, v)
		}
		return n
	}
	return def
}

func envFloat(t *testing.T, name string, def float64) float64 {
	t.Helper()
	if v := os.Getenv(name); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			t.Fatalf("%s=%q: want a rate between 0 and 1", name, v)
		}
		return f
	}
	return def
}
