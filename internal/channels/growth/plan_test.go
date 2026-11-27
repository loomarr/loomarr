package growth_test

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/library/provision"
)

func familyLine() growth.Line {
	seeds := []growth.Title{
		sitcom("s1", 1989, "TV-PG", "Comedy", "Family"), sitcom("s2", 1991, "TV-PG", "Comedy", "Family"),
		sitcom("s3", 1993, "TV-PG", "Comedy", "Family"),
	}
	return growth.DrawLine(seeds, 3)
}

func keys(picks []growth.Pick) []provision.Key {
	out := make([]provision.Key, 0, len(picks))
	for _, p := range picks {
		out = append(out, p.Key)
	}
	return out
}

// The fill stops at the target: a channel three shows short takes three, the closest to its seeds
// first, though the library holds more that fit.
func TestTheFillStopsAtTheTarget(t *testing.T) {
	cands := []growth.Candidate{
		{Title: sitcom("far", 1995, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("near", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("mid", 1993, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("also", 1994, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("half", 1991, "TV-PG", "Comedy"), Episodes: 50}, // unclear: one of two genres
	}
	got := keys(growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 4}, growth.Target{Episodes: 400, Shows: 7}, 1, nil, nil, true))
	want := []provision.Key{"series:tvdb:near", "series:tvdb:mid", "series:tvdb:also"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("picks %v, want %v", got, want)
	}
}

// With no model, an unclear fit joins only while the channel is still short once the clear fits are
// in; with a model it waits for the verdict however short the channel is. A title a person took
// back off never rejoins by itself.
func TestUnclearFitsFillOnlyAShortfallAndDeclinedNeverRejoin(t *testing.T) {
	cands := []growth.Candidate{
		{Title: sitcom("clear", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("declined", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("unclear", 1991, "TV-PG", "Comedy"), Episodes: 50},
	}
	declined := func(k provision.Key) bool { return k == "series:tvdb:declined" }
	short := growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 4}, growth.Target{Episodes: 400, Shows: 7}, 1, declined, nil, true)
	if got := keys(short); len(got) != 2 || got[0] != "series:tvdb:clear" || got[1] != "series:tvdb:unclear" || short[1].Fit != growth.Unclear {
		t.Fatalf("picks %+v, want the clear fit then the unclear one", short)
	}
	full := growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 6}, growth.Target{Episodes: 400, Shows: 7}, 1, declined, nil, true)
	if got := keys(full); len(got) != 1 || got[0] != "series:tvdb:clear" {
		t.Fatalf("one show short: picks %v, want only the clear fit", got)
	}
	reviewed := growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 4}, growth.Target{Episodes: 400, Shows: 7}, 1, declined, nil, false)
	if got := keys(reviewed); len(got) != 1 || got[0] != "series:tvdb:clear" {
		t.Fatalf("with a model: picks %v, want only the clear fit while the unclear one waits", got)
	}
}

// The model's verdict decides what a fit alone cannot: a clear fit it judged does not fit never
// joins, and an unclear fit it judged fits joins while the channel is short, though a model is
// configured.
func TestTheModelsVerdictDecidesWhatTheFitCannot(t *testing.T) {
	cands := []growth.Candidate{
		{Title: sitcom("rejected", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("clear", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50},
		{Title: sitcom("accepted", 1991, "TV-PG", "Comedy"), Episodes: 50},
		{Title: sitcom("waiting", 1991, "TV-PG", "Comedy"), Episodes: 50},
	}
	judged := func(k provision.Key) growth.Judgement {
		switch k {
		case "series:tvdb:rejected":
			return growth.JudgedNotFit
		case "series:tvdb:accepted":
			return growth.JudgedFits
		}
		return ""
	}
	got := keys(growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 4}, growth.Target{Episodes: 400, Shows: 7}, 1, nil, judged, false))
	want := []provision.Key{"series:tvdb:clear", "series:tvdb:accepted"}
	if !slices.Equal(got, want) {
		t.Fatalf("picks %v, want %v", got, want)
	}
}

// A channel at its target takes nothing, and a fixed line takes nothing whatever the library holds.
func TestAChannelAtItsTargetOrFixedTakesNothing(t *testing.T) {
	cands := []growth.Candidate{{Title: sitcom("clear", 1991, "TV-PG", "Comedy", "Family"), Episodes: 50}}
	if got := growth.Plan(familyLine(), cands, growth.Carried{Episodes: 900, Shows: 7}, growth.Target{Episodes: 400, Shows: 7}, 1, nil, nil, true); len(got) != 0 {
		t.Errorf("at target: %v", got)
	}
	fixed := growth.DrawLine([]growth.Title{sitcom("only", 1990, "TV-PG", "Comedy")}, 3)
	if got := growth.Plan(fixed, cands, growth.Carried{Shows: 1}, growth.Target{Shows: 7}, 0, nil, nil, true); len(got) != 0 {
		t.Errorf("fixed: %v", got)
	}
}
