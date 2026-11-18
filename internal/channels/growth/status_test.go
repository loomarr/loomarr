package growth_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// The status a person reads: the line in words, the shortfall, what joined, and what the model
// took off with its reason. A kept channel says so.
func TestTheStatusShowsTheLineWhatJoinedAndWhatCameOff(t *testing.T) {
	st, fill := fillHarnessWith(t, schedule.ChannelPolicy{}, true)
	libraryShow(t, st, 90000708, 1991, "TV-PG", 20, "Comedy", "Family") // a second clear fit
	ctx := context.Background()
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	j := &judge{answers: map[string]string{"Show 90000703": notFits, "Show 90000708": fits}}
	if _, err := review(st, j, &allow{room: 10}, nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	reader := growth.NewStatusReader(st, func() bool { return true })
	s, err := reader.Status(ctx, lineupOf(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if s.Still != "" || s.Words != "series from 1986 to 1995, rated up to TV-PG, comedy and family" {
		t.Errorf("still %q, words %q", s.Still, s.Words)
	}
	if len(s.Joined) != 1 || s.Joined[0].Key != "series:tvdb:90000708" || s.Joined[0].Batch != "growth-2026-11-16" {
		t.Errorf("joined %+v, want the accepted title", s.Joined)
	}
	if len(s.Off) != 1 || s.Off[0].Name != "Show 90000703" || s.Off[0].Reason != "a crime drama, not a sitcom" {
		t.Errorf("taken off %+v, want the rejected title with the model's reason", s.Off)
	}
	if s.Short.Episodes == 0 || s.Carried.Shows != 3 {
		t.Errorf("carried %+v short %+v; want three shows and still short of a week of episodes", s.Carried, s.Short)
	}

	st, _ = fillHarness(t, schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Growth: &schedule.GrowthPolicy{Keep: true}}})
	if s, err := growth.NewStatusReader(st, nil).Status(ctx, lineupOf(t, st)); err != nil || s.Still != growth.StillKept {
		t.Errorf("a kept channel: still %q (err %v), want %q", s.Still, err, growth.StillKept)
	}
}
