package suggest

import "testing"

func TestNetworkReferenceDoesNotTreatChannelCreationPhrasesAsNetworkNames(t *testing.T) {
	for _, text := range []string{
		"I'd like you to Make a channel built around The Matrix franchise.",
		"I'd like you to make a channel from the 1990s with action movies.",
		"Recreate a channel with British mysteries.",
	} {
		intent := Intent{Description: text}
		if networkStyleRequest(intent) || networkProgrammingEpochEnd(intent) != 0 {
			t.Errorf("channel-creation directive became a named network reference: %q", text)
		}
	}
}

func TestNetworkProgrammingEpochEndProjectsOnlyUnambiguousContext(t *testing.T) {
	for _, test := range []struct {
		description, refine string
		end                 int
	}{
		{"A channel like the History Channel from the 90s", "", 1999},
		{"Like the Discovery Channel during the 2000s", "", 2009},
		{"Recreate the Science network in the 80s", "", 1989},
		{"Like the History Channel from the 1990s", "like the Discovery Channel from the 2000s", 0},
		{"1990s documentary episodes", "", 0},
		{"Like the History Channel", "", 0},
	} {
		if got := networkProgrammingEpochEnd(Intent{Description: test.description, RefineText: test.refine}); got != test.end {
			t.Errorf("%q / %q: epoch end=%d want %d", test.description, test.refine, got, test.end)
		}
	}
}

func TestNetworkToolKeepsIndependentDateAndOrdinaryLookupInterfaces(t *testing.T) {
	for _, intent := range []Intent{
		{Description: "Like the History Channel from the 1990s", Era: "1990s"},
		{Description: "Like the History Channel from the 1990s. Only episodes aired during the 2000s."},
		{Description: "Like the History Channel from the 1990s. Episodes from the last five years."},
		{Description: "Like the History Channel from the 1990s. Only the first two seasons."},
	} {
		tool := catalogToolForIntent(intent)
		properties := tool.Parameters["properties"].(map[string]any)
		dateProperties := properties["dateMeaning"].(map[string]any)["properties"].(map[string]any)
		kinds := dateProperties["kind"].(map[string]any)["enum"].([]string)
		if len(kinds) != 3 {
			t.Fatalf("independent date request lost anchored interpretation: %+v", dateProperties)
		}
	}
	ordinary := catalogToolForIntent(Intent{Description: "Cozy British mysteries"})
	if _, present := ordinary.Parameters["allOf"]; !present {
		t.Fatal("ordinary discovery lost its existing mode contract")
	}
	named := catalogToolForIntent(Intent{Description: "TGIF programming block"})
	if _, present := named.Parameters["properties"].(map[string]any)["titles"]; !present {
		t.Fatal("named block lost collection membership lookup")
	}
}

func TestNetworkToolRetainsRequestedScalarBounds(t *testing.T) {
	intent := Intent{Description: "Like the History Channel from the 1990s. At least 50 votes and a rating above 8, with episodes under 60 minutes."}
	properties := catalogToolForIntent(intent).Parameters["properties"].(map[string]any)
	for _, field := range []string{"vote_count_min", "vote_average_min", "runtime_min", "runtime_max"} {
		if _, present := properties[field]; !present || !networkScalarRequested(intent, field) {
			t.Errorf("explicit scalar request lost %s", field)
		}
	}
}

func TestNetworkEpochAnchorsUseRuneCoordinatesAndKeepSeparateDates(t *testing.T) {
	intent := Intent{Description: "📺 Like the History Channel from the 1990s. Only 2000s episodes."}
	text := []rune(intent.Description)
	for start := 0; start+5 <= len(text); start++ {
		switch string(text[start : start+5]) {
		case "1990s":
			if !networkEpochAnchor(intent, DateAnchor{Field: DateAnchorDescription, Start: start, End: start + 5}) {
				t.Fatal("context anchor was not recognized with rune offsets")
			}
		case "2000s":
			if networkEpochAnchor(intent, DateAnchor{Field: DateAnchorDescription, Start: start, End: start + 5}) {
				t.Fatal("separate restriction became editorial context")
			}
		}
	}
}
