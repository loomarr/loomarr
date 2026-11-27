package approval_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/proposals"
)

// A rule that never fires was storable until the policy check learned to refuse it, so channels and
// pending proposals saved before then still carry them. Approving a proposal must not be blocked by
// them, whichever side they are on: an LLM rule in the proposal is dropped (it never fired, so the
// channel airs the same), and a rule the channel already holds is kept as stored.
func TestBindToleratesRulesThatNeverFire(t *testing.T) {
	t.Parallel()
	never := func(id, label string, source schedule.RuleSource, when schedule.WhenPredicate) schedule.SchedulingRule {
		return schedule.SchedulingRule{ID: id, Label: label, Source: source, When: when}
	}
	weekendWeekday := schedule.WhenPredicate{Weekend: true, Weekday: true}
	singleHour := schedule.WhenPredicate{HourFrom: 5, HourTo: 5} // "a single hour", legal before it was refused
	sound := schedule.WhenPredicate{HourFrom: 6, HourTo: 10}
	ids := func(rules []schedule.SchedulingRule) []string {
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			out = append(out, r.ID)
		}
		return out
	}

	for name, tc := range map[string]struct {
		stored   []schedule.SchedulingRule // the channel's rules before the approval
		proposed []schedule.SchedulingRule // the proposal's rules
		auto     bool                      // approved by the unattended re-curation
		want     []string                  // the channel's rule ids after
		wantErr  bool
	}{
		"a stored proposal's dead LLM rule is dropped": {
			proposed: []schedule.SchedulingRule{never("r1", "Cartoons", schedule.RuleSourceLLM, weekendWeekday), {ID: "r2", Source: schedule.RuleSourceLLM, When: sound}},
			want:     []string{"r2"},
		},
		"every proposed rule dead leaves no rules": {
			proposed: []schedule.SchedulingRule{never("r1", "A", schedule.RuleSourceLLM, weekendWeekday), never("r2", "B", schedule.RuleSourceLLM, singleHour)},
			want:     []string{},
		},
		"the channel's own dead operator rule is kept, not refused": {
			stored:   []schedule.SchedulingRule{never("o1", "Mine", schedule.RuleSourceOperator, singleHour)},
			proposed: []schedule.SchedulingRule{{ID: "r1", Source: schedule.RuleSourceLLM, When: sound}},
			want:     []string{"o1", "r1"},
		},
		"the same through the unattended re-curation": {
			stored:   []schedule.SchedulingRule{never("o1", "Mine", schedule.RuleSourceOperator, weekendWeekday)},
			proposed: []schedule.SchedulingRule{{ID: "r1", Source: schedule.RuleSourceLLM, When: sound}},
			auto:     true,
			want:     []string{"o1", "r1"},
		},
		"a dead stored LLM rule is replaced by the proposal's rules": {
			stored:   []schedule.SchedulingRule{never("old", "Old", schedule.RuleSourceLLM, weekendWeekday)},
			proposed: []schedule.SchedulingRule{{ID: "r1", Source: schedule.RuleSourceLLM, When: sound}},
			want:     []string{"r1"},
		},
		"a dead rule from nowhere in particular (no source) is not LLM's to drop": {
			proposed: []schedule.SchedulingRule{never("x1", "Unknown", "", weekendWeekday)},
			wantErr:  true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			existing := existingChannel()
			existing.Policy.Rules = tc.stored
			approvedBy := "admin"
			if tc.auto {
				approvedBy = "auto-curate"
			}
			body := proposals.Proposal{
				ChannelName: "Saturday Cartoons",
				Intent:      proposals.Intent{Description: "kids cartoons"},
				Lineup:      []proposals.ProposalItem{item(1, "One")},
				Policy:      schedule.ChannelPolicy{Rules: tc.proposed},
			}
			in := approval.BindInput{Proposal: proposalFrom(approvedBy, body), Existing: existing, Number: 7, Now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
			got, err := approval.Bind(in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Bind = nil, want the unattributed dead rule refused; rules = %v", ids(got.Channel.Policy.Rules))
				}
				return
			}
			if err != nil {
				t.Fatalf("Bind = %v, want the approval to go through", err)
			}
			if gotIDs := ids(got.Channel.Policy.Rules); len(gotIDs) != len(tc.want) || (len(gotIDs) > 0 && !equal(gotIDs, tc.want)) {
				t.Fatalf("rules after = %v, want %v", gotIDs, tc.want)
			}
		})
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
