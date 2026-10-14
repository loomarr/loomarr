package suggest

import (
	"context"
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// A name the requester fixed (the manual builder's, handed to AI by "Try AI suggestions", #1914)
// survives the model naming the channel something else; with none fixed, the model's name stands.
func TestBuildProposalKeepsFixedChannelName(t *testing.T) {
	surfaced := map[provision.Key]catalog.Candidate{
		"movie:tmdb:90910001": {MediaType: provision.Movie, TMDBID: 90910001, Name: "The Lantern Keeper", InLibrary: true},
	}
	for _, tc := range []struct {
		name, fixed, model, want string
	}{
		{"fixed name kept", "Cozy Mystery Nights", "Rainy Village Mysteries", "Cozy Mystery Nights"},
		{"fixed name trimmed", "  Cozy Mystery Nights ", "Rainy Village Mysteries", "Cozy Mystery Nights"},
		{"fixed name kept when the model names nothing", "Cozy Mystery Nights", "", "Cozy Mystery Nights"},
		{"no fixed name uses the model's", "", "Rainy Village Mysteries", "Rainy Village Mysteries"},
		{"blank fixed name uses the model's", "   ", "Rainy Village Mysteries", "Rainy Village Mysteries"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent := Intent{Description: "cozy mysteries", ChannelName: tc.fixed}
			meaning, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
			if err != nil {
				t.Fatal(err)
			}
			out, err := parsePicks(fmt.Sprintf(`{"channelName":%q,"picks":[{"key":"movie:tmdb:90910001","mediaType":"movie","name":"The Lantern Keeper"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`, tc.model))
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := (&Suggester{maxAcq: 3}).buildProposal(context.Background(), intent, out, surfaced, &DecisionTrace{}, meaning)
			if err != nil {
				t.Fatal(err)
			}
			if proposal.ChannelName != tc.want {
				t.Fatalf("ChannelName = %q, want %q", proposal.ChannelName, tc.want)
			}
		})
	}
}

// A fixed name is part of the cache identity: a cached proposal named otherwise must not answer
// for it, while an unnamed intent hashes exactly as before the field existed.
func TestIntentHashSeparatesFixedChannelName(t *testing.T) {
	base := Intent{Description: "cozy mysteries"}
	named := base
	named.ChannelName = "Cozy Mystery Nights"
	if IntentHash(base) == IntentHash(named) {
		t.Fatal("a fixed channel name must change the intent hash")
	}
	blank := base
	blank.ChannelName = "  "
	if IntentHash(base) != IntentHash(blank) {
		t.Fatal("a blank channel name must hash like no name")
	}
}
