package api

import (
	"encoding/json"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// channelNameFromIntent prefers the LLM's proposed channelName (a real network-style name),
// falls back to a truncated intent description, then a generic label. Fix B from the live
// smoke: a raw prompt made a half-assed title.
func TestChannelNameFromIntent(t *testing.T) {
	mk := func(p suggest.Proposal) store.Proposal {
		b, _ := json.Marshal(p)
		return store.Proposal{ProposalJSON: string(b)}
	}

	cases := []struct {
		name string
		prop suggest.Proposal
		want string
	}{
		{
			name: "prefers the LLM channelName",
			prop: suggest.Proposal{
				ChannelName: "Springfield Classics",
				Intent:      suggest.Intent{Description: "A channel with old-school Simpson's episodes"},
			},
			want: "Springfield Classics",
		},
		{
			name: "falls back to the intent when no channelName",
			prop: suggest.Proposal{Intent: suggest.Intent{Description: "90s Saturday morning cartoons"}},
			want: "90s Saturday morning cartoons",
		},
		{
			name: "generic label when nothing usable",
			prop: suggest.Proposal{},
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
