package defects

import (
	"strings"
	"testing"
)

func TestDecide_TheJudgeSettlesOnlyTheBorderlineBand(t *testing.T) {
	borderline := Assessment{Outcome: Borderline}
	for name, c := range map[string]struct {
		a    Assessment
		j    *Judgement
		want Verdict
		why  string
	}{
		"a clear pass needs no judge":     {Assessment{Outcome: Pass}, nil, VerdictPass, "detectors"},
		"a clear fail needs no judge":     {Assessment{Outcome: Fail}, &Judgement{Ruling: RulingPass}, VerdictFail, "detectors"},
		"too little picture":              {Assessment{Outcome: Undecided}, nil, VerdictUndecided, "detectors"},
		"waiting for a ruling":            {borderline, nil, VerdictPending, "waiting"},
		"the judge passes it":             {borderline, &Judgement{Ruling: RulingPass, Reason: "fine"}, VerdictPass, "judge: fine"},
		"the judge fails it":              {borderline, &Judgement{Ruling: RulingFail, Reason: "smeared"}, VerdictFail, "judge: smeared"},
		"an abstention fails, and says":   {borderline, &Judgement{Ruling: RulingAbstain}, VerdictFail, "judge abstained"},
		"no judge fails, and says":        {borderline, &Judgement{Ruling: RulingNoJudge}, VerdictFail, "no vision model"},
		"an unknown ruling is not a pass": {borderline, &Judgement{Ruling: "maybe"}, VerdictFail, "no vision model"},
	} {
		t.Run(name, func(t *testing.T) {
			got, why := Decide(c.a, c.j)
			if got != c.want || !strings.Contains(why, c.why) {
				t.Fatalf("Decide = %q %q, want %q containing %q", got, why, c.want, c.why)
			}
		})
	}
}

func TestParseJudgement_AnUnreadableReplyIsAnAbstention(t *testing.T) {
	for reply, want := range map[string]Ruling{
		`{"ruling":"pass","reason":"clean"}`:  RulingPass,
		`{"ruling":" FAIL ","reason":"bars"}`: RulingFail,
		`{"ruling":"abstain"}`:                RulingAbstain,
		`{"ruling":"probably fine"}`:          RulingAbstain,
		`not json`:                            RulingAbstain,
		`{"ruling":"no_judge"}`:               RulingAbstain, // the model cannot claim there is no judge
	} {
		j := ParseJudgement(reply, "m1")
		if j.Ruling != want || j.Version != JudgeVersion || j.Model != "m1" {
			t.Errorf("ParseJudgement(%s) = %+v, want %q", reply, j, want)
		}
	}
}

// Sound has no borderline band: the judge sees stills and cannot hear.
func TestAssess_ClippingIsDecidedByTheDetectorAlone(t *testing.T) {
	for flat, want := range map[float64]Outcome{0.5: Pass, 3: Pass, 5: Fail} {
		r := Readings{Version: MeasurementVersion, Width: 640, Height: 480, Frames: 40, HasAudio: true, FlatFactor: flat}
		if got := Assess(r, 2015).Outcome; got != want {
			t.Errorf("flat factor %v = %q, want %q", flat, got, want)
		}
	}
}

func TestSameQuestion_IsTheBandAndTheBorderlineChecks(t *testing.T) {
	soft := Readings{Version: MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 10.5}
	if !SameQuestion(Assess(soft, 1980), Assess(soft, 1990)) {
		t.Error("two tape-era years ask different questions")
	}
	if SameQuestion(Assess(soft, 1990), Assess(soft, 2000)) {
		t.Error("tape and SD bands ask the same question")
	}
}
