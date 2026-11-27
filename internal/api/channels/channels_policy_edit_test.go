package channels

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// A channel saved before a rule that never fires was refused still holds one. Editing anything else
// on the channel must not 422 over it, and an edit that does touch it must name it.
func TestPolicyPatchOverALegacyDeadRule(t *testing.T) {
	dead := schedule.SchedulingRule{ID: "r1", Label: "Weekend cartoons", Source: schedule.RuleSourceOperator,
		When: schedule.WhenPredicate{Weekend: true, Weekday: true}}
	singleHour := schedule.SchedulingRule{ID: "r2", Label: "One hour", Source: schedule.RuleSourceOperator,
		When: schedule.WhenPredicate{HourFrom: 5, HourTo: 5}}
	sound := schedule.SchedulingRule{ID: "r3", Label: "Mornings", When: schedule.WhenPredicate{HourFrom: 6, HourTo: 10}}
	stored := func() *channelstore.Channel {
		ch := &channelstore.Channel{}
		ch.Policy.Rules = []schedule.SchedulingRule{dead, singleHour, sound}
		return ch
	}
	patch := func(p schedule.ChannelPolicy) *updateChannelInput {
		in := &updateChannelInput{}
		in.Body.Policy = &p
		return in
	}
	problem := func(t *testing.T, err error) huma.StatusError {
		t.Helper()
		var se huma.StatusError
		if !errors.As(err, &se) || se.GetStatus() != http.StatusUnprocessableEntity {
			t.Fatalf("error = %v, want a 422 problem", err)
		}
		return se
	}
	vocab := holidayvocab.Default()

	t.Run("an edit that leaves the rules alone is accepted", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		next.Audience.Ceiling = "TV-PG"
		if err := applyChannelPatch(ch, patch(next), vocab); err != nil {
			t.Fatalf("applyChannelPatch = %v, want the unrelated edit accepted", err)
		}
		if len(ch.Policy.Rules) != 3 || ch.Policy.Audience.Ceiling != "TV-PG" {
			t.Fatalf("policy after = %+v, want the audience set and all three rules kept", ch.Policy)
		}
	})

	t.Run("an edit that adds a sound rule beside them is accepted", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		next.Rules = append(append([]schedule.SchedulingRule(nil), next.Rules...),
			schedule.SchedulingRule{ID: "r4", Label: "Primetime", When: schedule.WhenPredicate{HourFrom: 20, HourTo: 23}})
		if err := applyChannelPatch(ch, patch(next), vocab); err != nil {
			t.Fatalf("applyChannelPatch = %v", err)
		}
	})

	t.Run("an edit that removes the dead rules is accepted", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		next.Rules = []schedule.SchedulingRule{sound}
		if err := applyChannelPatch(ch, patch(next), vocab); err != nil {
			t.Fatalf("applyChannelPatch = %v", err)
		}
	})

	t.Run("an edit that touches a dead rule without fixing it names that rule", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		touched := dead
		touched.Label = "Weekend cartoons, renamed"
		next.Rules = []schedule.SchedulingRule{touched, singleHour, sound}
		se := problem(t, applyChannelPatch(ch, patch(next), vocab))
		for _, want := range []string{"Rule 1", "Weekend cartoons, renamed", "never"} {
			if !strings.Contains(se.Error(), want) {
				t.Errorf("detail = %q, want it to mention %q", se.Error(), want)
			}
		}
		if strings.Contains(se.Error(), "audience and filler") {
			t.Errorf("detail = %q still points at the audience and filler options", se.Error())
		}
	})

	t.Run("a new dead rule is refused and named", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		next.Rules = append(append([]schedule.SchedulingRule(nil), next.Rules...),
			schedule.SchedulingRule{ID: "r9", Label: "Sunday on Monday", When: schedule.WhenPredicate{Weekend: true, Days: []time.Weekday{time.Monday}}})
		se := problem(t, applyChannelPatch(ch, patch(next), vocab))
		if !strings.Contains(se.Error(), "Rule 4") || !strings.Contains(se.Error(), "Sunday on Monday") {
			t.Errorf("detail = %q, want rule 4 named", se.Error())
		}
	})

	t.Run("a bad value elsewhere still reads as an audience or filler problem", func(t *testing.T) {
		ch := stored()
		next := ch.Policy
		next.Audience.Ceiling = "not-a-rating"
		se := problem(t, applyChannelPatch(ch, patch(next), vocab))
		if !strings.Contains(se.Error(), "audience and filler") {
			t.Errorf("detail = %q, want the generic policy wording for a non-rule failure", se.Error())
		}
	})

	t.Run("a channel with nothing stored gets a fresh write's checks", func(t *testing.T) {
		ch := &channelstore.Channel{}
		next := schedule.ChannelPolicy{Rules: []schedule.SchedulingRule{dead}}
		_ = problem(t, applyChannelPatch(ch, patch(next), vocab))
	})
}
