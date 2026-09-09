package suggest_test

// Property tests for the deterministic scorer (score.go: score, themeFit,
// requested date adherence — §8 evidence assessment). These are seeded plain-Go loops
// (math/rand with a fixed source so any failure reproduces), NOT a framework.
// They assert the structural invariants of the scorer over many random intents +
// ProposalItem sets, complementing the fixture-driven TestScoring_Deterministic
// (suggester_test.go) which pins one concrete pipeline run.
//
// The unexported score functions are reached via the test-only bridge in
// export_score_test.go.

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
)

// randWords is a small vocabulary the generators draw from so terms sometimes hit
// and sometimes miss the haystack (exercising both themeFit branches).
var randWords = []string{
	"action", "sci-fi", "comedy", "noir", "high-energy", "1990s", "matrix",
	"speed", "rock", "thriller", "drama", "cozy", "space", "heist", "the",
	"of", "a", "channel", "movies", "", "xyzzy", "quux",
}

func pickWord(r *rand.Rand) string { return randWords[r.Intn(len(randWords))] }

// randIntent builds an arbitrary Intent from the seeded source.
func randIntent(r *rand.Rand) suggest.Intent {
	nMust := r.Intn(3)
	must := make([]string, nMust)
	for i := range must {
		must[i] = pickWord(r)
	}
	return suggest.Intent{
		Description: pickWord(r) + " " + pickWord(r) + " " + pickWord(r),
		Era:         pickWord(r),
		Tone:        pickWord(r),
		MustInclude: must,
	}
}

// randItem builds an arbitrary ProposalItem. Year is sometimes 0 (unknown) so the
// eraBalance "no year info" branch is hit; genres/overview/rationale/name draw from
// the shared vocabulary so themeFit sometimes matches.
func randItem(r *rand.Rand) suggest.ProposalItem {
	var year int
	switch r.Intn(3) {
	case 0:
		year = 0 // unknown year
	default:
		year = 1950 + r.Intn(80) // 1950..2029
	}
	nGenres := r.Intn(3)
	genres := make([]string, nGenres)
	for i := range genres {
		genres[i] = pickWord(r)
	}
	return suggest.ProposalItem{
		MediaType: provision.Movie,
		TMDBID:    r.Intn(100000),
		Name:      pickWord(r) + " " + pickWord(r),
		Year:      year,
		Genres:    genres,
		Overview:  pickWord(r) + " " + pickWord(r),
		Rationale: pickWord(r),
	}
}

// randItems builds n arbitrary items.
func randItems(r *rand.Rand, n int) []suggest.ProposalItem {
	if n == 0 {
		return nil
	}
	items := make([]suggest.ProposalItem, n)
	for i := range items {
		items[i] = randItem(r)
	}
	return items
}

const propIters = 2000

func in01(v float64) bool { return v >= 0 && v <= 1 }

// TestScore_AllSubScoresInUnitInterval: for arbitrary intents + lineup/acquisition
// sets, every assessed sub-score stays within [0,1]. A score outside the unit
// interval breaks ranking comparability (§8) and downstream weighting.
func TestScore_AllSubScoresInUnitInterval(t *testing.T) {
	r := rand.New(rand.NewSource(0xC0FFEE))
	for i := 0; i < propIters; i++ {
		intent := randIntent(r)
		nLine := r.Intn(8)
		nAcq := r.Intn(8)
		lineup := randItems(r, nLine)
		acq := randItems(r, nAcq)

		s := suggest.ScoreForTest(intent, lineup, acq)
		if s.ThemeFit != nil && !in01(*s.ThemeFit) {
			t.Fatalf("iter %d: ThemeFit out of [0,1]: %v (intent=%+v)", i, s.ThemeFit, intent)
		}
		if !in01(s.AvailabilityRatio) {
			t.Fatalf("iter %d: AvailabilityRatio out of [0,1]: %v", i, s.AvailabilityRatio)
		}
		if s.EraBalance != nil && !in01(*s.EraBalance) {
			t.Fatalf("iter %d: EraBalance out of [0,1]: %v", i, *s.EraBalance)
		}
	}
}

// TestScore_Deterministic: the same inputs always yield identical Scores. This is
// the core §8 guarantee ("same intent + same picks → same Scores"); here it is
// checked over many random inputs, not just one pinned lineup.
func TestScore_Deterministic(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for i := 0; i < propIters; i++ {
		intent := randIntent(r)
		lineup := randItems(r, r.Intn(8))
		acq := randItems(r, r.Intn(8))

		a := suggest.ScoreForTest(intent, lineup, acq)
		b := suggest.ScoreForTest(intent, lineup, acq)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("iter %d: scoring not deterministic: %+v vs %+v", i, a, b)
		}
		// Sub-functions are individually deterministic too.
		if !reflect.DeepEqual(suggest.ThemeFitForTest(intent, lineup, acq), suggest.ThemeFitForTest(intent, lineup, acq)) {
			t.Fatalf("iter %d: themeFit not deterministic", i)
		}

	}
}

// TestScore_AvailabilityRatioExact: AvailabilityRatio == len(lineup)/(len(lineup)+
// len(acquisitions)) exactly, for arbitrary non-empty splits. This is the
// library-presence diagnostic; the ratio must reflect the actual split.
func TestScore_AvailabilityRatioExact(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for i := 0; i < propIters; i++ {
		nLine := r.Intn(8)
		nAcq := r.Intn(8)
		if nLine+nAcq == 0 {
			continue // empty handled by its own test
		}
		intent := randIntent(r)
		lineup := randItems(r, nLine)
		acq := randItems(r, nAcq)

		s := suggest.ScoreForTest(intent, lineup, acq)
		want := float64(nLine) / float64(nLine+nAcq)
		if s.AvailabilityRatio != want {
			t.Fatalf("iter %d: AvailabilityRatio = %v, want %v (line=%d acq=%d)",
				i, s.AvailabilityRatio, want, nLine, nAcq)
		}
	}
}

// TestScore_EmptyInputZeroScores: no items at all → the zero Scores value (every
// numeric claim absent). Empty evidence remains explicitly unassessed, so a proposal
// carries no misleading non-zero signals.
func TestScore_EmptyInputZeroScores(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	for i := 0; i < 200; i++ {
		intent := randIntent(r) // intent varies; emptiness is about items
		got := suggest.ScoreForTest(intent, nil, nil)
		if got.ThemeFit != nil || got.EraBalance != nil || got.AvailabilityRatio != 0 {
			t.Fatalf("iter %d: empty input should give zero Scores, got %+v", i, got)
		}
	}
	// Explicit empty (non-nil) slices behave the same as nil.
	if got := suggest.ScoreForTest(suggest.Intent{Description: "action"}, []suggest.ProposalItem{}, []suggest.ProposalItem{}); got.ThemeFit != nil || got.EraBalance != nil || got.AvailabilityRatio != 0 {
		t.Fatalf("empty (non-nil) slices should give zero Scores, got %+v", got)
	}
}

func TestThemeFitDoesNotUseModelRationaleAsEvidence(t *testing.T) {
	got := suggest.ThemeFitForTest(
		suggest.Intent{Description: "friday family showcase"},
		[]suggest.ProposalItem{{
			Name: "Orbital Detectives", Genres: []string{"Science Fiction"},
			Rationale: "A perfect match for the Friday Family Showcase.",
		}},
		nil,
	)
	if got == nil || *got != 0 {
		t.Fatalf("model-authored rationale manufactured theme evidence: got %v", got)
	}
}

// An absent theme must never earn full credit, regardless of metadata richness.
func TestThemeFit_NoTermsIsUnassessed(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 200; i++ {
		if got := suggest.ThemeFitForTest(suggest.Intent{Description: "a channel of the"}, randItems(r, 1+r.Intn(5)), nil); got != nil {
			t.Fatalf("no requested theme earned credit: %v", *got)
		}
	}
}

// With no validated date constraints, date adherence is absent for any years.
func TestEra_NoRequestIsUnassessed(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 200; i++ {
		s := suggest.ScoreForTest(randIntent(r), randItems(r, r.Intn(8)), nil)
		if s.EraBalance != nil || s.Era.Status != "not_requested" {
			t.Fatalf("unrequested dates earned credit: %+v", s)
		}
	}
}

// Distinct qualifiers are averaged per item; repetition cannot inflate coverage.
func TestThemeFit_QualifierCoverageAndRepetition(t *testing.T) {
	r := rand.New(rand.NewSource(0xBEEF))
	for i := 0; i < propIters; i++ {
		n, hits := 1+r.Intn(8), 0
		items := make([]suggest.ProposalItem, n)
		for j := range items {
			items[j].Overview = "A catalog synopsis"
			if r.Intn(2) == 0 {
				items[j].Genres = append(items[j].Genres, "Action")
				hits++
			}
			if r.Intn(2) == 0 {
				items[j].Genres = append(items[j].Genres, "Comedy")
				hits++
			}
		}
		intent := suggest.Intent{Description: "action comedy action", Tone: "comedy"}
		s := suggest.ScoreForTest(intent, items, nil)
		want := float64(hits) / float64(2*n)
		if s.ThemeFit == nil || *s.ThemeFit != want || len(s.Theme.Qualifiers) != 2 {
			t.Fatalf("iter %d: coverage=%+v want=%v", i, s, want)
		}
	}
}

// FuzzScore_Invariants: native Go fuzzing over the scorer's structural invariants.
// The corpus bytes are decoded into a small random-ish intent + item set; every
// run must keep all scores in [0,1], keep AvailabilityRatio exact, and stay
// deterministic. Fuzzing complements the seeded loops by letting the engine search
// for adversarial byte sequences.
func FuzzScore_Invariants(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(0xC0FFEE))
	f.Add(int64(-5))
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		intent := randIntent(r)
		nLine := r.Intn(6)
		nAcq := r.Intn(6)
		lineup := randItems(r, nLine)
		acq := randItems(r, nAcq)

		s := suggest.ScoreForTest(intent, lineup, acq)
		values := []float64{s.AvailabilityRatio}
		if s.ThemeFit != nil {
			values = append(values, *s.ThemeFit)
		}
		if s.EraBalance != nil {
			values = append(values, *s.EraBalance)
		}
		for _, v := range values {
			if !in01(v) {
				t.Fatalf("score out of [0,1]: %+v (seed=%d)", s, seed)
			}
		}
		if nLine+nAcq == 0 {
			if s.ThemeFit != nil || s.EraBalance != nil || s.AvailabilityRatio != 0 {
				t.Fatalf("empty input should give zero Scores, got %+v (seed=%d)", s, seed)
			}
		} else {
			want := float64(nLine) / float64(nLine+nAcq)
			if s.AvailabilityRatio != want {
				t.Fatalf("AvailabilityRatio = %v, want %v (seed=%d)", s.AvailabilityRatio, want, seed)
			}
		}
		if again := suggest.ScoreForTest(intent, lineup, acq); !reflect.DeepEqual(again, s) {
			t.Fatalf("non-deterministic: %+v vs %+v (seed=%d)", s, again, seed)
		}
	})
}
