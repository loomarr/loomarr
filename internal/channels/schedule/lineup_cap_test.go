package schedule_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

func numbered(from, n int) []schedule.LineupEntry {
	out := make([]schedule.LineupEntry, n)
	for i := range out {
		out[i] = entry(fmt.Sprintf("movie:tmdb:%d", 90000000+from+i), fmt.Sprintf("Title %d", from+i))
	}
	return out
}

// The lineup editor saves a whole list, capped at MaxLineupEntries. The additive path (unattended
// auto-curate) unioned with no cap, after which the editor could no longer save the channel at all.
// It now stops at the ceiling and says which picks it turned away.
func TestApplyLineup_AdditiveStopsAtTheCeilingAndReportsWhatItRefused(t *testing.T) {
	ceiling := schedule.MaxLineupEntries
	for name, tc := range map[string]struct {
		existing, fresh []schedule.LineupEntry
		drop            func(schedule.LineupEntry) bool
		wantLen         int
		wantRefused     []int // numbered titles turned away, in order
	}{
		"room for every new pick":                 {numbered(0, 10), numbered(10, 10), nil, 20, nil},
		"exactly fills the ceiling":               {numbered(0, ceiling-5), numbered(ceiling-5, 5), nil, ceiling, nil},
		"one over the ceiling":                    {numbered(0, ceiling-5), numbered(ceiling-5, 6), nil, ceiling, []int{ceiling}},
		"five over, in the order they were given": {numbered(0, ceiling-2), numbered(ceiling-2, 7), nil, ceiling, []int{ceiling, ceiling + 1, ceiling + 2, ceiling + 3, ceiling + 4}},
		"already at the ceiling":                  {numbered(0, ceiling), numbered(ceiling, 3), nil, ceiling, []int{ceiling, ceiling + 1, ceiling + 2}},
		"an existing lineup already over it keeps every title": {
			numbered(0, ceiling+30), numbered(ceiling+30, 2), nil, ceiling + 30, []int{ceiling + 30, ceiling + 31}},
		"a re-pick is never turned away, even at the ceiling": {
			numbered(0, ceiling), numbered(ceiling-3, 3), nil, ceiling, nil},
		"a re-pick beside new picks at the ceiling": {
			numbered(0, ceiling), append(numbered(ceiling-1, 1), numbered(ceiling, 1)...), nil, ceiling, []int{ceiling}},
		"a dropped title makes room for a new one": {
			numbered(0, ceiling), numbered(ceiling, 4), func(e schedule.LineupEntry) bool { return e.Title == "Title 3" || e.Title == "Title 9" }, ceiling - 2 + 2, []int{ceiling + 2, ceiling + 3}},
		"a dropped title is room even for the first of many": {
			numbered(0, ceiling), numbered(ceiling, 1), func(e schedule.LineupEntry) bool { return e.Title == "Title 0" }, ceiling, nil},
		"a title repeated within the incoming list is one pick": {
			numbered(0, ceiling-1), append(numbered(ceiling-1, 1), numbered(ceiling-1, 1)...), nil, ceiling, nil},
	} {
		t.Run(name, func(t *testing.T) {
			var refused []schedule.LineupEntry
			got := schedule.ApplyLineup(tc.existing, tc.fresh, schedule.LineupAdditive,
				schedule.ApplyOpts{Drop: tc.drop, Full: func(e schedule.LineupEntry) { refused = append(refused, e) }})
			if len(got) != tc.wantLen {
				t.Fatalf("lineup has %d entries, want %d", len(got), tc.wantLen)
			}
			if over := len(got) - len(tc.existing); len(tc.existing) <= ceiling && len(got) > ceiling {
				t.Fatalf("a lineup that fit grew to %d, past the ceiling of %d (%d added)", len(got), ceiling, over)
			}
			var gotRefused []int
			for _, e := range refused {
				var n int
				_, _ = fmt.Sscanf(e.Title, "Title %d", &n)
				gotRefused = append(gotRefused, n)
			}
			if !reflect.DeepEqual(gotRefused, tc.wantRefused) {
				t.Fatalf("refused %v, want %v", gotRefused, tc.wantRefused)
			}
			// What was refused is not in the lineup, and nothing is in it twice.
			in := map[provision.Key]int{}
			for _, e := range got {
				in[e.Key]++
			}
			for k, n := range in {
				if n != 1 {
					t.Fatalf("%s appears %d times", k, n)
				}
			}
			for _, e := range refused {
				if _, ok := in[e.Key]; ok {
					t.Fatalf("%s was reported refused and is in the lineup", e.Key)
				}
			}
			// The existing titles that were not dropped keep their order at the front.
			if len(tc.existing) > 0 && tc.drop == nil && got[0].Key != tc.existing[0].Key {
				t.Fatalf("existing order not kept: first = %s", got[0].Key)
			}
		})
	}
	t.Run("no Full callback still caps", func(t *testing.T) {
		got := schedule.ApplyLineup(numbered(0, ceiling-1), numbered(ceiling-1, 5), schedule.LineupAdditive, schedule.ApplyOpts{})
		if len(got) != ceiling {
			t.Fatalf("lineup has %d entries without a Full callback, want the ceiling %d", len(got), ceiling)
		}
	})
}

// A whole-list replace is a person's decision, so the primitive does not truncate it; its callers
// hold the same ceiling (the editor's schema, Bind), and the constant is one value.
func TestApplyLineup_ReplaceIsNotTruncated(t *testing.T) {
	over := numbered(0, schedule.MaxLineupEntries+5)
	if got := schedule.ApplyLineup(nil, over, schedule.LineupReplace, schedule.ApplyOpts{}); len(got) != len(over) {
		t.Fatalf("replace returned %d of %d entries: the primitive must not drop a person's choice", len(got), len(over))
	}
	if schedule.MaxLineupEntries != 1000 {
		t.Fatalf("MaxLineupEntries = %d: the editor's schema tags say 1000, change them together", schedule.MaxLineupEntries)
	}
}
