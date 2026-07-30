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

		// `Collections` now BINDS (programming-design §2.2): the scheduler's scope pass filters
		// on the membership stamped at reconcile, so a collections-only scope is a real narrower
		// and must count. This case was deliberately `false` while the field was inert, and was
		// flipped in the change that added the filter — as its own note instructed.
		{"collections alone — binds since §2.2", &schedule.ScopePolicy{Collections: []string{"star-trek"}}, true},

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
