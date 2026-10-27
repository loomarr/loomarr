package defects

import "slices"

// Label is how a clip looks against a good copy of its era: what the floor should have found. The
// committed corpus labels by how each clip was made; the household report by the maintainer's eye.
type Label string

const (
	LabelPass Label = "pass"
	LabelFail Label = "fail"
)

// LabelledResult is one labelled clip judged in one era.
type LabelledResult struct {
	Era     int
	Label   Label
	Outcome Outcome
	// Ruling is the judge's, for a borderline result scored with a judge; empty otherwise.
	Ruling Ruling
}

// BandScore counts one era band's results against their labels.
type BandScore struct {
	Band Band `json:"band"`
	// FalseFails are labelled pass but failed outright by the detectors: what precision costs.
	FalseFails int `json:"falseFails"`
	// Failed are labelled fail and failed outright; Judged are labelled fail and sent to the judge.
	Failed int `json:"failed"`
	Judged int `json:"judged"`
	// Missed are labelled fail and passed by the detectors.
	Missed int `json:"missed"`
	// BorderlinePass are labelled pass and sent to the judge, who should pass them.
	BorderlinePass int `json:"borderlinePass"`
	// JudgeRight and JudgeWrong score the judge's rulings in the band, when it ruled. An abstention
	// is wrong: it fails the clip whatever its label.
	JudgeRight int `json:"judgeRight"`
	JudgeWrong int `json:"judgeWrong"`
	Total      int `json:"total"`
}

// Precision is the share of the detectors' outright fails that were labelled fail.
func (s BandScore) Precision() float64 {
	if s.Failed+s.FalseFails == 0 {
		return 1
	}
	return float64(s.Failed) / float64(s.Failed+s.FalseFails)
}

// Caught is the share of clips labelled fail that the detectors failed or sent to the judge.
func (s BandScore) Caught() float64 {
	labelledFail := s.Failed + s.Judged + s.Missed
	if labelledFail == 0 {
		return 1
	}
	return float64(s.Failed+s.Judged) / float64(labelledFail)
}

// Score tallies results per era band, tape first. A result the detectors left Undecided is counted
// in Total only: it says nothing about the bars.
func Score(results []LabelledResult) []BandScore {
	byBand := map[Band]*BandScore{}
	for _, r := range results {
		band := BandFor(r.Era)
		s := byBand[band]
		if s == nil {
			s = &BandScore{Band: band}
			byBand[band] = s
		}
		s.Total++
		switch {
		case r.Outcome == Undecided:
		case r.Outcome == Fail && r.Label == LabelPass:
			s.FalseFails++
		case r.Outcome == Fail:
			s.Failed++
		case r.Outcome == Borderline && r.Label == LabelFail:
			s.Judged++
		case r.Outcome == Borderline:
			s.BorderlinePass++
		case r.Label == LabelFail:
			s.Missed++
		}
		if r.Outcome == Borderline && r.Ruling != "" {
			if (r.Ruling == RulingFail) == (r.Label == LabelFail) && r.Ruling != RulingAbstain {
				s.JudgeRight++
			} else {
				s.JudgeWrong++
			}
		}
	}
	var out []BandScore
	for _, band := range []Band{BandTape, BandSD, BandHD} {
		if s := byBand[band]; s != nil {
			out = append(out, *s)
		}
	}
	return slices.Clip(out)
}
