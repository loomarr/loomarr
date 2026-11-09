package suggest

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/rating"
	"github.com/loomarr/loomarr/internal/proposals"
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
			intent: Intent{Intent: proposals.Intent{Description: "1980s action heroes"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
			},
		},
		{
			name:   "plain episodic intent",
			intent: Intent{Intent: proposals.Intent{Description: "Mudpuddles episodes"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
			},
		},
		{
			name:   "sequential intent",
			intent: Intent{Intent: proposals.Intent{Description: "Mudpuddles from the beginning"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				sequential:       true,
			},
		},
		{
			name:   "curated intent",
			intent: Intent{Intent: proposals.Intent{Description: "classic Mudpuddles"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
				curated:          true,
			},
		},
		{
			name:   "curated intent with negated binge",
			intent: Intent{Intent: proposals.Intent{Description: "Classic Mudpuddles: the best episodes, not a full binge."}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
				curated:          true,
			},
		},
		{
			name:   "named holiday intent",
			intent: Intent{Intent: proposals.Intent{Description: "Christmas Mudpuddles episodes"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
				seasonal:         schedule.SeasonalPolicy{Mode: schedule.SeasonalExclusive, Holidays: []string{"christmas"}},
			},
		},
		{
			name:   "child safety intent",
			intent: Intent{Intent: proposals.Intent{Description: "Saturday morning cartoons for kids"}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				safetyCeiling:    rating.Rating("TV-Y7"),
				kidsSignal:       true,
			},
		},
		{
			name:   "explicit rating and era intent",
			intent: Intent{Intent: proposals.Intent{Description: "90s action rated PG-13", Era: "1990s"}},
			want: deterministicIntentPolicy{
				episodeSelection:        schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
				explicitAudienceCeiling: rating.Rating("PG-13"),
				kidsSignal:              true,
			},
		},
		{
			name:   "negative holiday constraint",
			intent: Intent{Intent: proposals.Intent{Description: "classic Mudpuddles", MustExclude: []string{"holiday episodes"}}},
			want: deterministicIntentPolicy{
				episodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
				curated:          true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveIntentPolicy(test.intent, shipped); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("deriveIntentPolicy() = %#v, want %#v", got, test.want)
			}
		})
	}
}

// A format word is not an audience: "90s cartoons" is nostalgia for adults and
// must not keep a model-proposed kids ceiling (#2739), while a request that
// names a young audience still does.
func TestIntentSignalsKidsNeedsAnAudienceNotAFormat(t *testing.T) {
	t.Parallel()
	for description, want := range map[string]bool{
		"90s cartoons":                          false,
		"classic cartoons and adult animation":  false,
		"cartoons for kids":                     true,
		"cartoons for my 5-year-old":            true,
		"cartoons for my 7 year old":            true,
		"cartoons my toddler will love":         true,
		"Saturday morning cartoons":             true,
		"cartoons for the little ones":          true,
		"80s action heroes with a 15-year-old":  false,
		"animated classics for my 12 year olds": true,
	} {
		got := intentSignalsKids(Intent{Intent: proposals.Intent{Description: description}})
		if got != want {
			t.Errorf("intentSignalsKids(%q) = %v, want %v", description, got, want)
		}
	}
}
