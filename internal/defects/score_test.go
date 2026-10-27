package defects

import (
	"strconv"
	"testing"
)

func TestScore_TalliesEachBandAgainstItsLabels(t *testing.T) {
	results := []LabelledResult{
		{Era: 1985, Label: LabelFail, Outcome: Fail},
		{Era: 1985, Label: LabelPass, Outcome: Fail},
		{Era: 1985, Label: LabelFail, Outcome: Pass},
		{Era: 1985, Label: LabelFail, Outcome: Borderline, Ruling: RulingFail},
		{Era: 1985, Label: LabelPass, Outcome: Borderline, Ruling: RulingAbstain},
		{Era: 1985, Label: LabelFail, Outcome: Undecided},
		{Era: 2015, Label: LabelPass, Outcome: Borderline, Ruling: RulingPass},
		{Era: 2015, Label: LabelPass, Outcome: Pass},
	}
	got := Score(results)
	want := []BandScore{
		{Band: BandTape, Failed: 1, FalseFails: 1, Missed: 1, Judged: 1, BorderlinePass: 1, JudgeRight: 1, JudgeWrong: 1, Total: 6},
		{Band: BandHD, BorderlinePass: 1, JudgeRight: 1, Total: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("Score = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("band %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if p, c := got[0].Precision(), got[0].Caught(); p != 0.5 || c != 2.0/3 {
		t.Errorf("tape precision %v caught %v, want 0.5 and 2/3", p, c)
	}
	if p, c := got[1].Precision(), got[1].Caught(); p != 1 || c != 1 {
		t.Errorf("a band with nothing failed and nothing labelled fail scores %v %v, want 1 1", p, c)
	}
}

func TestLoadCorpus_LabelsEveryCaseInEveryEra(t *testing.T) {
	c, err := LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != CorpusVersion || len(c.Cases) == 0 || len(c.Eras) != 3 {
		t.Fatalf("corpus = %s with %d cases over eras %v", c.Version, len(c.Cases), c.Eras)
	}
	bands := map[Band]bool{}
	for _, era := range c.Eras {
		bands[BandFor(era)] = true
		for _, cs := range c.Cases {
			if cs.Labels[strconv.Itoa(era)] == "" {
				t.Errorf("%s %g unlabelled for %d", cs.Damage, cs.Severity, era)
			}
		}
	}
	if len(bands) != 3 {
		t.Fatalf("the corpus eras %v do not cover every band", c.Eras)
	}
}
