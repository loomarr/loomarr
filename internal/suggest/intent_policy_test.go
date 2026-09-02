package suggest

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
)

func TestDeriveIntentPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		intent Intent
		want   deterministicIntentPolicy
	}{
		{
			name:   "unqualified movie intent",
			intent: Intent{Description: "1980s action heroes"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
			},
		},
		{
			name:   "plain episodic intent",
			intent: Intent{Description: "Simpsons episodes"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
			},
		},
		{
			name:   "sequential intent",
			intent: Intent{Description: "Simpsons from the beginning"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				sequential:       true,
			},
		},
		{
			name:   "curated intent",
			intent: Intent{Description: "classic Simpsons"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
				curated:          true,
			},
		},
		{
			name:   "named holiday intent",
			intent: Intent{Description: "Christmas Simpsons episodes"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
				seasonal:         schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
			},
		},
		{
			name:   "child safety intent",
			intent: Intent{Description: "Saturday morning cartoons for kids"},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				safetyCeiling:    schedule.Rating("TV-Y7"),
				kidsSignal:       true,
			},
		},
		{
			name:   "explicit rating and era intent",
			intent: Intent{Description: "90s action rated PG-13", Era: "1990s"},
			want: deterministicIntentPolicy{
				episodeSelection:        schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				explicitAudienceCeiling: schedule.Rating("PG-13"),
				kidsSignal:              true,
			},
		},
		{
			name:   "negative holiday constraint",
			intent: Intent{Description: "classic Simpsons", MustExclude: []string{"holiday episodes"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
				curated:          true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveIntentPolicy(test.intent); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("deriveIntentPolicy() = %#v, want %#v", got, test.want)
			}
		})
	}
}
