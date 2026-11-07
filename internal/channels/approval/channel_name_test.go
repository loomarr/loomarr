package approval

import (
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/store"
)

// channelNameFromIntent prefers the LLM's proposed channelName (a real network-style name),
// falls back to a truncated intent description, then a generic label. Fix B from the live
// smoke: a raw prompt made a half-assed title.
func TestChannelNameFromIntent(t *testing.T) {
	t.Parallel()
	mk := func(p proposals.Proposal) store.Proposal {
		return store.Proposal{Body: p}
	}

	cases := []struct {
		name string
		prop proposals.Proposal
		want string
	}{
		{
			name: "prefers the LLM channelName",
			prop: proposals.Proposal{
				ChannelName: "Mudpuddle Classics",
				Intent:      proposals.Intent{Description: "A channel with old-school Mudpuddles episodes"},
			},
			want: "Mudpuddle Classics",
		},
		{
			name: "falls back to the intent when no channelName",
			prop: proposals.Proposal{Intent: proposals.Intent{Description: "90s Saturday morning cartoons"}},
			want: "90s Saturday morning cartoons",
		},
		{
			name: "generic label when nothing usable",
			prop: proposals.Proposal{},
			want: "Suggested channel",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := channelNameFromIntent(mk(c.prop)); got != c.want {
				t.Errorf("channelNameFromIntent = %q, want %q", got, c.want)
			}
		})
	}
}
