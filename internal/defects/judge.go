package defects

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// JudgeVersion names the judge's prompt and reply contract. A new version rules on every
// borderline clip again.
const JudgeVersion = "judge-v1"

// JudgeStills is how many stills the judge sees, spread across the clip. The readings are clip
// means rather than moments, so no frame is more "flagged" than another.
const JudgeStills = 6

// Ruling is the judge's answer for one borderline clip.
type Ruling string

const (
	RulingPass    Ruling = "pass"
	RulingFail    Ruling = "fail"
	RulingAbstain Ruling = "abstain"
	// RulingNoJudge records that no vision model was configured when the clip needed one.
	RulingNoJudge Ruling = "no_judge"
)

// Judgement is a ruling on one assessment, kept beside the record it ruled on.
type Judgement struct {
	Version string `json:"version"`
	Ruling  Ruling `json:"ruling"`
	Reason  string `json:"reason,omitempty"`
	// Model is the judge that ruled; an abstention is asked again only of a different model.
	Model string `json:"model,omitempty"`
}

// Verdict is a clip's final standing under the floor: the detectors' outcome, with a borderline
// outcome settled by the judge.
type Verdict string

const (
	VerdictPass      Verdict = "pass"
	VerdictFail      Verdict = "fail"
	VerdictUndecided Verdict = "undecided" // too little picture to judge
	// VerdictPending is a borderline clip still waiting for a ruling: the judge was out of reach
	// or out of budget, and the next pass asks again.
	VerdictPending Verdict = "pending"
)

// Decide combines an assessment with its judgement. With no judge, or when the judge abstains, a
// borderline clip fails, with a reason that says so, and is ruled on again once a judge can rule.
func Decide(a Assessment, j *Judgement) (Verdict, string) {
	switch a.Outcome {
	case Pass:
		return VerdictPass, "detectors"
	case Fail:
		return VerdictFail, "detectors"
	case Undecided:
		return VerdictUndecided, "detectors"
	}
	if j == nil {
		return VerdictPending, "waiting for the judge"
	}
	switch j.Ruling {
	case RulingPass:
		return VerdictPass, "judge: " + j.Reason
	case RulingFail:
		return VerdictFail, "judge: " + j.Reason
	case RulingAbstain:
		return VerdictFail, "judge abstained: " + j.Reason
	default:
		return VerdictFail, "no vision model configured to rule on a borderline clip"
	}
}

// SameQuestion reports whether a ruling on one assessment still answers another: the same band
// and the same borderline checks. A floor recalibration that moves no clip in or out of the band
// costs no new look.
func SameQuestion(a, b Assessment) bool {
	return a.Band == b.Band && slices.Equal(borderlineChecks(a), borderlineChecks(b))
}

func borderlineChecks(a Assessment) []Check {
	var out []Check
	for _, f := range a.Findings {
		if f.Outcome == Borderline {
			out = append(out, f.Check)
		}
	}
	return out
}

// JudgePrompt asks a vision model to rule on a borderline clip from its stills, its readings and
// its era. It names only the borderline checks, so the model looks for what the detectors could
// not settle, and it says what a fair bar is for the era so an old ad is not held to HD.
func JudgePrompt(a Assessment, era int) string {
	var b strings.Builder
	b.WriteString("You are checking whether a television advertisement is clean enough to air. ")
	fmt.Fprintf(&b, "The stills are %d frames spread across the ad. ", JudgeStills)
	switch a.Band {
	case BandTape:
		b.WriteString("It dates from before 1995 and was recorded on tape: judge it against a good VHS or broadcast tape copy, so mild softness and faint noise are normal. ")
	case BandSD:
		if era == 0 {
			b.WriteString("Its year is unknown: judge it against a good standard-definition copy. ")
		} else {
			b.WriteString("It is a standard-definition ad: judge it against a good standard-definition copy. ")
		}
	case BandHD:
		b.WriteString("It is from the HD era: judge it against a clean high-definition copy. ")
	}
	b.WriteString("Automatic detectors found these measurements near the limit:\n")
	for _, f := range a.Findings {
		if f.Outcome == Borderline {
			fmt.Fprintf(&b, "- %s: %s\n", f.Check, checkMeaning[f.Check])
		}
	}
	b.WriteString("Fail the ad only for damage a viewer would notice: compression blocks, smearing or heavy blur, " +
		"tape dropouts or noise bars, combing on motion, a small picture inside black borders, or an obviously enlarged low-resolution picture. " +
		"Do not judge the ad's content, age or style. " +
		`Reply with JSON only: {"ruling":"pass"|"fail"|"abstain","reason":"one short sentence"}. ` +
		"Abstain when the stills do not show enough to decide.")
	return b.String()
}

var checkMeaning = map[Check]string{
	CheckBlocks:    "compression blocks may be visible",
	CheckBlur:      "the picture may be soft or smeared",
	CheckDropouts:  "there may be tape dropouts or noise lines",
	CheckInterlace: "moving areas may show combing between fields",
	CheckWindowbox: "the picture may sit inside black borders on every side",
	CheckUpscale:   "the picture may be a low-resolution source enlarged",
}

// maxReasonRunes bounds the model's reason as stored.
const maxReasonRunes = 200

// ParseJudgement reads the model's reply, already stripped to its JSON object. A reply with no
// readable ruling is an error, not an abstention: an abstention is the model declining, and is
// asked again only of another model, while a garbled reply is worth asking again.
func ParseJudgement(reply, model string) (Judgement, error) {
	var out struct {
		Ruling string `json:"ruling"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		return Judgement{}, fmt.Errorf("unreadable reply: %w", err)
	}
	r := Ruling(strings.ToLower(strings.TrimSpace(out.Ruling)))
	if r != RulingPass && r != RulingFail && r != RulingAbstain {
		return Judgement{}, fmt.Errorf("the reply named no ruling (%q)", cleanReason(out.Ruling))
	}
	return Judgement{Version: JudgeVersion, Ruling: r, Reason: cleanReason(out.Reason), Model: model}, nil
}

// cleanReason makes model text safe to store: valid UTF-8, no control characters (Postgres TEXT
// refuses NUL), cut by runes rather than bytes.
func cleanReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	s = strings.Join(strings.Fields(s), " ")
	if runes := []rune(s); len(runes) > maxReasonRunes {
		s = string(runes[:maxReasonRunes])
	}
	return s
}
