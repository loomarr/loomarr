package suggest

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// scopeNarrows decides whether a lowered rule scope is kept or nilled to "inherit the channel
// scope". A scope that narrows NOTHING must be nilled — a non-nil empty narrower reads as an
// active constraint that silently constrains nothing.
func TestScopeNarrows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope *schedule.ScopePolicy
		want  bool
	}{
		{"nil", nil, false},
		{"empty", &schedule.ScopePolicy{}, false},
		{"series", &schedule.ScopePolicy{Series: []provision.Key{"series:tvdb:1"}}, true},
		{"era", &schedule.ScopePolicy{Era: &schedule.Range{From: 1990, To: 1999}}, true},
		{"genres include", &schedule.ScopePolicy{Genres: schedule.GenreFilter{Include: []string{"Animation"}}}, true},
		{"genres exclude", &schedule.ScopePolicy{Genres: schedule.GenreFilter{Exclude: []string{"Horror"}}}, true},
		{"runtime cap", &schedule.ScopePolicy{RuntimeMax: 3600}, true},

		// ⚠ **The one that is deliberately false.** `Collections` round-trips through PATCH and
		// the policy blob, but NO filter reads it — the scheduler's scope pass checks Series,
		// Era, Genres and RuntimeMax and skips it entirely (pinned by
		// schedule.TestEnforce_CollectionsDoesNotBindYet).
		//
		// Counting it here would make a collections-only scope survive the nil-out in
		// groundRules as a "narrower" that narrows nothing — exactly the active-but-empty scope
		// this function exists to prevent. When collections starts binding, flip this to `true`
		// in the same change that adds the filter.
		{"collections alone — INERT, must not count", &schedule.ScopePolicy{Collections: []string{"star-trek"}}, false},

		// …but a collections scope that ALSO carries a real narrower still counts, on the
		// strength of the real one. The exclusion is about Collections voting alone, not about
		// poisoning a scope that happens to mention it.
		{"collections + era", &schedule.ScopePolicy{
			Collections: []string{"star-trek"},
			Era:         &schedule.Range{From: 1990, To: 1999},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeNarrows(tc.scope); got != tc.want {
				t.Errorf("scopeNarrows(%+v) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}
