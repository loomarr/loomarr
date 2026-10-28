package filler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/llm"
)

// scriptedJudge answers Model with model (or err) and each clip from rulings; asked records who
// it was asked about.
type scriptedJudge struct {
	model     string
	err       error
	rulings   map[string]func() (defects.Judgement, error)
	asked     []string
	durations []int64
}

func (j *scriptedJudge) Model(context.Context) (string, error) { return j.model, j.err }
func (j *scriptedJudge) Rule(_ context.Context, req DefectJudgeRequest) (defects.Judgement, error) {
	j.asked = append(j.asked, req.ClipHash)
	j.durations = append(j.durations, req.DurationMs)
	if r, ok := j.rulings[req.ClipHash]; ok {
		return r()
	}
	return defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingPass, Reason: "clean enough", Model: j.model}, nil
}

func ruling(r defects.Ruling, model string) func() (defects.Judgement, error) {
	return func() (defects.Judgement, error) {
		return defects.Judgement{Version: defects.JudgeVersion, Ruling: r, Reason: "looked", Model: model}, nil
	}
}

// borderlineCatalog is n tape-era clips already measured as borderline soft, oldest first.
func borderlineCatalog(t *testing.T, n int) ([]DefectClip, *defectMemStore) {
	t.Helper()
	dir := t.TempDir()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	soft := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 10.5}
	st := &defectMemStore{records: map[string]DefectRecord{}}
	var clips []DefectClip
	for i := range n {
		hash := writeContentAddressedClip(t, dir, []byte{byte('a' + i)}, ".mp4")
		c := DefectClip{CreatedAt: t0.Add(time.Duration(i) * time.Minute)}
		c.Hash, c.Path, c.Era, c.Kind, c.Placement, c.DurationMs = hash, ClipRelPath(hash, ".mp4"), 1988, Commercial, PlacementBreakBody, 30_000
		clips = append(clips, c)
		st.records[hash] = DefectRecord{ClipHash: hash, Readings: soft, Era: 1988, Assessment: defects.Assess(soft, 1988)}
	}
	st.clips = clips
	return clips, st
}

func runFloor(t *testing.T, st *defectMemStore, judge DefectJudge) DefectFloorResult {
	t.Helper()
	return runFloorProbing(t, st, judge, nil)
}

func runFloorProbing(t *testing.T, st *defectMemStore, judge DefectJudge, probe Prober) DefectFloorResult {
	t.Helper()
	measure := func(context.Context, string, int, int, bool) (defects.Readings, error) {
		t.Fatal("a clip with a current measurement was decoded")
		return defects.Readings{}, nil
	}
	res, err := NewDefectFloor(st, probe, measure, judge, t.TempDir(), time.Now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// With no judge a borderline clip fails for that reason, and is ruled on as soon as one exists.
func TestDefectFloor_NoJudgeFailsUntilAJudgeRules(t *testing.T) {
	clips, st := borderlineCatalog(t, 1)
	hash := clips[0].Hash
	runFloor(t, st, &scriptedJudge{err: ErrNoDefectJudge})
	if got := st.records[hash]; got.Verdict != defects.VerdictFail || !strings.Contains(got.VerdictReason, "no vision model") {
		t.Fatalf("with no judge = %q %q, want a fail that says no vision model was configured", got.Verdict, got.VerdictReason)
	}
	judge := &scriptedJudge{model: "m1"}
	if res := runFloor(t, st, judge); res.Ruled != 1 || len(judge.asked) != 1 {
		t.Fatalf("pass with a judge = %+v asked %v, want the clip ruled on", res, judge.asked)
	}
	if got := st.records[hash]; got.Verdict != defects.VerdictPass || !strings.HasPrefix(got.VerdictReason, "judge: ") {
		t.Fatalf("after the judge = %q %q, want the judge's pass", got.Verdict, got.VerdictReason)
	}
	// A ruling stands: the next pass asks nothing.
	judge.asked = nil
	if runFloor(t, st, judge); len(judge.asked) != 0 {
		t.Fatalf("asked again about %v", judge.asked)
	}
}

// An abstention fails the clip, and is put again only to a different model.
func TestDefectFloor_AnAbstentionIsAskedAgainOnlyOfAnotherModel(t *testing.T) {
	clips, st := borderlineCatalog(t, 1)
	hash := clips[0].Hash
	judge := &scriptedJudge{model: "m1", rulings: map[string]func() (defects.Judgement, error){hash: ruling(defects.RulingAbstain, "m1")}}
	runFloor(t, st, judge)
	if got := st.records[hash]; got.Verdict != defects.VerdictFail || !strings.HasPrefix(got.VerdictReason, "judge abstained") {
		t.Fatalf("after an abstention = %q %q, want a fail that says the judge abstained", got.Verdict, got.VerdictReason)
	}
	judge.asked = nil
	if runFloor(t, st, judge); len(judge.asked) != 0 {
		t.Fatal("the same model was asked again about a clip it abstained on")
	}
	judge.model, judge.rulings = "m2", nil
	if runFloor(t, st, judge); len(judge.asked) != 1 || st.records[hash].Verdict != defects.VerdictPass {
		t.Fatalf("a new model asked %v, verdict %q; want it asked and its pass kept", judge.asked, st.records[hash].Verdict)
	}
}

// An outage or a spent budget leaves every waiting clip pending and stops the asking for the pass:
// six timed-out calls an hour would buy nothing.
func TestDefectFloor_AnOutageOrSpentBudgetStopsThePass(t *testing.T) {
	for name, fail := range map[string]error{
		"outage":       errors.New("connection refused"),
		"spent budget": ErrDefectJudgeHeld,
	} {
		t.Run(name, func(t *testing.T) {
			clips, st := borderlineCatalog(t, 3)
			judge := &scriptedJudge{model: "m1", rulings: map[string]func() (defects.Judgement, error){
				clips[0].Hash: func() (defects.Judgement, error) { return defects.Judgement{}, fail },
			}}
			if res := runFloor(t, st, judge); res.Held != 3 || res.Ruled != 0 || len(judge.asked) != 1 {
				t.Fatalf("pass = %+v asked %v; want one ask, then all three held", res, judge.asked)
			}
			for _, c := range clips {
				if got := st.records[c.Hash]; got.Verdict != defects.VerdictPending {
					t.Fatalf("%s = %q, want pending", c.Hash, got.Verdict)
				}
			}
			judge.rulings, judge.asked = nil, nil
			if res := runFloor(t, st, judge); res.Ruled != 3 {
				t.Fatalf("next pass = %+v, want every pending clip ruled on", res)
			}
		})
	}
}

// A clip the judge keeps failing on is counted, the next clip is still asked, and next pass it
// waits behind clips that have not failed, so it cannot take every turn.
func TestDefectFloor_AClipTheJudgeFailsOnFallsBehind(t *testing.T) {
	clips, st := borderlineCatalog(t, defectJudgeCalls+1)
	poison := func() (defects.Judgement, error) {
		return defects.Judgement{}, fmt.Errorf("%w: unreadable reply", ErrDefectJudgeClip)
	}
	judge := &scriptedJudge{model: "m1", rulings: map[string]func() (defects.Judgement, error){}}
	for _, c := range clips[:defectJudgeCalls] {
		judge.rulings[c.Hash] = poison
	}
	if res := runFloor(t, st, judge); len(judge.asked) != defectJudgeCalls || res.Held != defectJudgeCalls+1 {
		t.Fatalf("pass = %+v asked %d; want every turn taken by the failing clips and all held", res, len(judge.asked))
	}
	if got := st.records[clips[0].Hash]; got.JudgeFailures != 1 || got.Verdict != defects.VerdictPending {
		t.Fatalf("failing clip = %+v, want one failure counted and still pending", got)
	}
	last := clips[defectJudgeCalls].Hash
	judge.asked = nil
	runFloor(t, st, judge)
	if len(judge.asked) == 0 || judge.asked[0] != last || st.records[last].Verdict != defects.VerdictPass {
		t.Fatalf("next pass asked %v first, verdict %q; want the clip that never failed asked first and passed",
			judge.asked, st.records[last].Verdict)
	}
}

// A clip the catalog has no length for is probed; one the probe cannot read is a clip failure,
// asked of nobody.
func TestDefectFloor_ProbesALengthTheCatalogLacks(t *testing.T) {
	clips, st := borderlineCatalog(t, 2)
	st.clips[0].DurationMs, st.clips[1].DurationMs = 0, 0
	judge := &scriptedJudge{model: "m1"}
	probe := func(_ context.Context, file string) (Probed, error) {
		if strings.Contains(file, clips[1].Hash) {
			return Probed{}, errors.New("moov atom not found")
		}
		return Probed{DurationMs: 12_000}, nil
	}
	res := runFloorProbing(t, st, judge, probe)
	if len(judge.asked) != 1 || judge.durations[0] != 12_000 || res.Ruled != 1 {
		t.Fatalf("pass = %+v asked %v at %v; want only the probed clip asked, at its probed length", res, judge.asked, judge.durations)
	}
	if got := st.records[clips[1].Hash]; got.JudgeFailures != 1 || got.Verdict != defects.VerdictPending {
		t.Fatalf("unreadable clip = %+v, want a clip failure, still pending", got)
	}
}

// One pass asks the judge about at most defectJudgeCalls clips.
func TestDefectFloor_BoundsTheJudgeCallsInOnePass(t *testing.T) {
	_, st := borderlineCatalog(t, defectJudgeCalls+2)
	judge := &scriptedJudge{model: "m1"}
	res := runFloor(t, st, judge)
	if len(judge.asked) != defectJudgeCalls || res.Held != 2 {
		t.Fatalf("pass = %+v asked %d, want %d asked and 2 held", res, len(judge.asked), defectJudgeCalls)
	}
}

// A clip that leaves the borderline band keeps no ruling; one that stays keeps its ruling across a
// re-assessment that asks the same question.
func TestDefectFloor_ARulingFollowsTheQuestionItAnswered(t *testing.T) {
	clips, st := borderlineCatalog(t, 2)
	judge := &scriptedJudge{model: "m1"}
	runFloor(t, st, judge)
	// The first is re-dated within the tape band: the same question. The second to the HD era,
	// where the same softness is a clear fail.
	st.clips[0].Era, st.clips[1].Era = 1990, 2015
	judge.asked = nil
	runFloor(t, st, judge)
	if len(judge.asked) != 0 {
		t.Fatalf("asked %v again after re-dating", judge.asked)
	}
	if got := st.records[clips[0].Hash]; got.Judgement == nil || got.Verdict != defects.VerdictPass {
		t.Fatalf("re-dated within the band = %+v, want the ruling kept", got)
	}
	if got := st.records[clips[1].Hash]; got.Judgement != nil || got.Verdict != defects.VerdictFail || got.VerdictReason != "detectors" {
		t.Fatalf("re-dated to HD = %+v, want the detectors' fail and no ruling", got)
	}
}

type judgeLedger struct {
	held        bool
	events      []string
	failed      bool
	settleAlive bool // the settle's context was still live
}

func (l *judgeLedger) ReserveDefectJudge(_ context.Context, _ string, e DefectJudgeEndpoint, _ time.Time) (string, bool, error) {
	l.events = append(l.events, "reserve "+e.Name())
	return "look-1", l.held, nil
}

func (l *judgeLedger) SettleDefectJudge(ctx context.Context, id string, _ DefectJudgeEndpoint, _ llm.Attribution, failed bool, _ time.Time) error {
	l.events = append(l.events, "settle "+id)
	l.failed, l.settleAlive = failed, ctx.Err() == nil
	return nil
}

type judgeFrames struct {
	MediaTools // only VisionKeyframesIn is called
	frames     [][]byte
	file       string
	span       [2]int64
	n          int
}

func (f *judgeFrames) VisionKeyframesIn(_ context.Context, file string, startMs, endMs int64, n int) ([][]byte, error) {
	f.file, f.span, f.n = file, [2]int64{startMs, endMs}, n
	return f.frames, nil
}

type judgeProvider struct {
	reply  string
	err    error
	prompt string
	images int
}

func (p *judgeProvider) AskAboutImages(_ context.Context, prompt string, jpegs [][]byte) (llm.Response, error) {
	p.prompt, p.images = prompt, len(jpegs)
	return llm.Response{Content: p.reply, Attribution: llm.Attribution{RequestedModel: "m1", Latency: time.Second}}, p.err
}

// The vision judge takes its stills across the whole clip, reserves before it asks, settles after,
// and reads the model's ruling.
func TestVisionDefectJudge_ReservesAsksAndSettles(t *testing.T) {
	soft := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 10.5}
	req := DefectJudgeRequest{ClipHash: "c1", File: filepath.Join("clips", "c1.mp4"), DurationMs: 30_000, Era: 1988,
		Assessment: defects.Assess(soft, 1988)}
	route := func() (DefectJudgeEndpoint, error) {
		return DefectJudgeEndpoint{Provider: "openrouter", Model: "m1"}, nil
	}
	frames := &judgeFrames{frames: [][]byte{{1}, {2}, {3}, {4}, {5}, {6}}}
	provider := &judgeProvider{reply: "```json\n{\"ruling\":\"fail\",\"reason\":\"heavy smear\"}\n```"}
	ledger := &judgeLedger{}
	judge := NewVisionDefectJudge(provider, frames, ledger, route, time.Now, nil)

	j, err := judge.Rule(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if j.Ruling != defects.RulingFail || j.Reason != "heavy smear" || j.Model != "openrouter/m1" || j.Version != defects.JudgeVersion {
		t.Fatalf("judgement = %+v, want the model's fail", j)
	}
	if frames.file != req.File || frames.span != [2]int64{0, 30_000} || frames.n != defects.JudgeStills || provider.images != 6 {
		t.Fatalf("stills from %s %v n=%d sent %d, want %d across the whole clip", frames.file, frames.span, frames.n, provider.images, defects.JudgeStills)
	}
	if !strings.Contains(provider.prompt, string(defects.CheckBlur)) || !strings.Contains(provider.prompt, "tape") {
		t.Fatalf("prompt names neither the borderline check nor the era:\n%s", provider.prompt)
	}
	if strings.Join(ledger.events, ",") != "reserve openrouter/m1,settle look-1" || ledger.failed {
		t.Fatalf("ledger = %v failed=%v, want a reservation settled as a completed look", ledger.events, ledger.failed)
	}

	// A spent budget asks nothing.
	held := &judgeLedger{held: true}
	provider.prompt = ""
	if _, err := NewVisionDefectJudge(provider, frames, held, route, time.Now, nil).Rule(context.Background(), req); !errors.Is(err, ErrDefectJudgeHeld) || provider.prompt != "" {
		t.Fatalf("held = %v, asked %v; want ErrDefectJudgeHeld and no call", err, provider.prompt != "")
	}

	// A garbled reply is the clip's failure, not an abstention: it is worth asking again.
	garbled := &judgeLedger{}
	provider.reply = "I think it looks fine overall"
	if _, err := NewVisionDefectJudge(provider, frames, garbled, route, time.Now, nil).Rule(context.Background(), req); !errors.Is(err, ErrDefectJudgeClip) {
		t.Fatalf("garbled reply = %v, want a clip failure", err)
	}

	// A failed call is settled as failed and reported, even when the pass is being cancelled.
	failing := &judgeLedger{}
	provider.err = errors.New("502")
	ctx, cancel := context.WithCancel(context.Background())
	failingProvider := &cancellingProvider{judgeProvider: provider, cancel: cancel}
	if _, err := NewVisionDefectJudge(failingProvider, frames, failing, route, time.Now, nil).Rule(ctx, req); err == nil ||
		!failing.failed || !failing.settleAlive {
		t.Fatalf("failed call = %v settled-failed %v settle-alive %v, want the error and a failed settlement made outside the cancelled pass",
			err, failing.failed, failing.settleAlive)
	}

	// No model configured: nothing decoded, nothing reserved.
	none := &judgeLedger{}
	frames.n = 0
	noJudge := func() (DefectJudgeEndpoint, error) { return DefectJudgeEndpoint{}, ErrNoDefectJudge }
	if _, err := NewVisionDefectJudge(provider, frames, none, noJudge, time.Now, nil).
		Rule(context.Background(), req); !errors.Is(err, ErrNoDefectJudge) || frames.n != 0 || len(none.events) != 0 {
		t.Fatalf("no judge = %v, decoded %v, ledger %v", err, frames.n != 0, none.events)
	}
}

// cancellingProvider cancels the pass while the call is in flight, as a shutdown would.
type cancellingProvider struct {
	*judgeProvider
	cancel context.CancelFunc
}

func (p *cancellingProvider) AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (llm.Response, error) {
	p.cancel()
	return p.judgeProvider.AskAboutImages(ctx, prompt, jpegs)
}
