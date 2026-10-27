package defects

import (
	"strings"
	"testing"
	"unicode/utf8"
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

// A garbled reply is an error, worth asking again; only the model declining is an abstention.
func TestParseJudgement_OnlyADeclineIsAnAbstention(t *testing.T) {
	for reply, want := range map[string]Ruling{
		`{"ruling":"pass","reason":"clean"}`:  RulingPass,
		`{"ruling":" FAIL ","reason":"bars"}`: RulingFail,
		`{"ruling":"abstain"}`:                RulingAbstain,
		`{"ruling":"probably fine"}`:          "",
		`not json`:                            "",
		``:                                    "",
		`{"ruling":"no_judge"}`:               "", // the model cannot claim there is no judge
	} {
		j, err := ParseJudgement(reply, "m1")
		if want == "" {
			if err == nil {
				t.Errorf("ParseJudgement(%s) = %+v, want an error", reply, j)
			}
			continue
		}
		if err != nil || j.Ruling != want || j.Version != JudgeVersion || j.Model != "m1" {
			t.Errorf("ParseJudgement(%s) = %+v %v, want %q", reply, j, err, want)
		}
	}
}

// The reason is stored in a TEXT column Postgres guards: no NUL, no broken UTF-8, cut by runes.
func TestParseJudgement_TheReasonIsSafeToStore(t *testing.T) {
	long := strings.Repeat("é", maxReasonRunes+50)
	j, err := ParseJudgement(`{"ruling":"fail","reason":"bad\u0000 frames\n`+long+`"}`, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(j.Reason) || strings.ContainsAny(j.Reason, "\x00\n") || utf8.RuneCountInString(j.Reason) != maxReasonRunes {
		t.Fatalf("reason = %q (%d runes), want valid UTF-8 with no control characters, cut to %d runes",
			j.Reason, utf8.RuneCountInString(j.Reason), maxReasonRunes)
	}
	if !strings.HasPrefix(j.Reason, "bad frames é") {
		t.Fatalf("reason = %q, want the text kept with control characters as spaces", j.Reason[:20])
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
