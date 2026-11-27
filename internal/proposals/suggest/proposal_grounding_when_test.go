package suggest

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
)

// The model writes WHEN tokens. A composite it cannot honour (two day kinds, two hour windows)
// drops that rule and only that rule: the rest of the policy, the audience ceiling above all,
// survives. Before, such a token lowered to a rule that never fired, and once the policy check
// refused those it would have dropped the whole policy to its defaults.
func TestGroundRulesDropsAContradictoryCompositeAndKeepsThePolicy(t *testing.T) {
	kidsIntent := Intent{Intent: proposals.Intent{Description: "cartoons for kids"}}
	for name, tc := range map[string]struct {
		rules []pickRule
		keep  []string // the labels that survive, in order
	}{
		"two day kinds":                  {[]pickRule{{When: "weekend-weekday"}}, nil},
		"two day kinds, reversed":        {[]pickRule{{When: "weekday-weekend"}}, nil},
		"two hour windows":               {[]pickRule{{When: "mornings-primetime"}}, nil},
		"two hour windows, reversed":     {[]pickRule{{When: "primetime-mornings"}}, nil},
		"wrapping and plain windows":     {[]pickRule{{When: "late-night-daytime"}}, nil},
		"a contradiction beside a sound": {[]pickRule{{When: "weekend-weekday"}, {When: "weekend-mornings"}}, []string{"Weekend-mornings"}},
		"a sound one before a bad one":   {[]pickRule{{When: "weekday-primetime"}, {When: "mornings-overnight"}}, []string{"Weekday-primetime"}},
		"three parts is not a composite": {[]pickRule{{When: "weekend-mornings-weekday"}}, nil},
		"every rule sound":               {[]pickRule{{When: "weekend"}, {When: "primetime"}}, []string{"Weekend", "Primetime"}},
		// A part with a hyphen of its own is a sound rule whichever side it is on: the model is not
		// made to lose a rule it got right.
		"a hyphenated part first":   {[]pickRule{{When: "late-night-weekend"}, {When: "prime-time-weekday"}}, []string{"Late-night-weekend", "Prime-time-weekday"}},
		"a hyphenated part last":    {[]pickRule{{When: "weekend-late-night"}}, []string{"Weekend-late-night"}},
		"hyphenated parts, a clash": {[]pickRule{{When: "early-morning-late-night"}, {When: "early-morning-weekday"}}, []string{"Early-morning-weekday"}},
	} {
		t.Run(name, func(t *testing.T) {
			raw := &pickPolicy{Rules: tc.rules}
			raw.Audience.Ceiling = "TV-Y7"
			p := groundPolicy(raw, nil, nil, kidsIntent, shipped)

			if p.Audience.Ceiling == "" {
				t.Fatalf("the audience ceiling was lost with the contradictory rule: policy = %+v", p)
			}
			var got []string
			for _, r := range p.Rules {
				got = append(got, r.Label)
			}
			if strings.Join(got, ",") != strings.Join(tc.keep, ",") {
				t.Fatalf("rules = %v, want %v", got, tc.keep)
			}
			if err := p.Validate(); err != nil {
				t.Fatalf("the grounded policy fails Validate: %v", err)
			}
		})
	}
}
