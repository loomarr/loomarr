package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

// The Duration newtype must (un)marshal as a Go-duration STRING ("168h"), not a
// nanosecond integer — that's what keeps policy_json human/LLM-authorable and
// matches the programming-design §2 schema.
func TestDuration_JSONIsAString(t *testing.T) {
	blob, err := json.Marshal(Duration(168 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) != `"168h0m0s"` {
		t.Errorf("marshal = %s, want a duration string", blob)
	}
	var d Duration
	if err := json.Unmarshal([]byte(`"168h"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Std() != 168*time.Hour {
		t.Errorf("unmarshal(\"168h\") = %v", d.Std())
	}
	// A garbage string is a decode error (not silently zero).
	if err := json.Unmarshal([]byte(`"not-a-duration"`), &d); err == nil {
		t.Error("expected an error unmarshaling a non-duration string")
	}
}

// Resolved fills every default from the built-in constants when the policy is empty.
func TestResolved_EmptyPolicyUsesBuiltins(t *testing.T) {
	r := ChannelPolicy{}.Resolved(Sequential, false)
	if r.Sep.EpisodeNoRepeat != defaultEpisodeNoRepeat {
		t.Errorf("episodeNoRepeat = %v, want default %v", r.Sep.EpisodeNoRepeat, defaultEpisodeNoRepeat)
	}
	if r.Sep.BlockMax != defaultBlockMax {
		t.Errorf("blockMax = %d, want %d", r.Sep.BlockMax, defaultBlockMax)
	}
	if r.Seasonal.Mode != SeasonalAuto {
		t.Errorf("seasonal mode = %q, want auto", r.Seasonal.Mode)
	}
}

// Omitted ordering INHERITS the channel's Strategy (the locked non-breaking
// default) — a policy-less shuffle channel stays shuffle, not syndication.
func TestResolved_OrderingInheritsStrategy(t *testing.T) {
	if got := (ChannelPolicy{}).Resolved(Shuffle, false).Ordering; got != OrderShuffle {
		t.Errorf("empty policy on a Shuffle channel = %q, want shuffle", got)
	}
	if got := (ChannelPolicy{}).Resolved(Sequential, false).Ordering; got != OrderSequential {
		t.Errorf("empty policy on a Sequential channel = %q, want sequential", got)
	}
	// An explicit policy ordering wins over the strategy.
	p := ChannelPolicy{ProposalPolicy: ProposalPolicy{Ordering: OrderSyndication}}
	if got := p.Resolved(Sequential, false).Ordering; got != OrderSyndication {
		t.Errorf("explicit syndication = %q, want syndication", got)
	}
}

// A single-series channel auto-relaxes series-level separation (separation there
// is EPISODE spacing, not series spacing) — rule-based, not LLM judgment.
func TestResolved_SingleSeriesRelaxesSeriesSeparation(t *testing.T) {
	p := ChannelPolicy{ProposalPolicy: ProposalPolicy{Separation: SeparationPolicy{SeriesMinGap: Duration(2 * time.Hour), BlockMax: 2}}}
	r := p.Resolved(Sequential, true)
	if r.Sep.SeriesMinGap != 0 || r.Sep.BlockMax != 0 {
		t.Errorf("single-series should relax series gap/block: got gap=%v block=%d", r.Sep.SeriesMinGap, r.Sep.BlockMax)
	}
	// Episode no-repeat is NOT relaxed — that's the meaningful spacing on a single-series channel.
	if r.Sep.EpisodeNoRepeat != defaultEpisodeNoRepeat {
		t.Errorf("episodeNoRepeat should survive single-series relax: %v", r.Sep.EpisodeNoRepeat)
	}
}

// Unrated policy fails CLOSED under a kids ceiling and open otherwise (§4).
func TestResolved_UnratedDefaultsByCeiling(t *testing.T) {
	cases := []struct {
		ceiling Rating
		want    UnratedPolicy
	}{
		{"TV-Y", UnratedExclude},
		{"TV-Y7", UnratedExclude},
		{"TV-PG", UnratedExclude}, // TV-PG is still a kids ceiling
		{"TV-14", UnratedAllow},
		{"TV-MA", UnratedAllow},
		{"", UnratedAllow}, // no ceiling → adult/general
	}
	for _, c := range cases {
		got := (ChannelPolicy{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: c.ceiling}}}).Resolved(Sequential, false).Unrated
		if got != c.want {
			t.Errorf("ceiling %q → unrated %q, want %q", c.ceiling, got, c.want)
		}
	}
	// An explicit unrated policy overrides the ceiling default.
	p := ChannelPolicy{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: "TV-Y7", Unrated: UnratedAllow}}}
	if got := p.Resolved(Sequential, false).Unrated; got != UnratedAllow {
		t.Errorf("explicit allow under a kids ceiling = %q, want allow", got)
	}
}

// The rating ladder orders TV and film systems together; unmappable → unrated.
func TestRatingLadder(t *testing.T) {
	if !Rating("TV-Y7").atOrBelow("TV-14") {
		t.Error("TV-Y7 should be at or below TV-14")
	}
	if Rating("TV-MA").atOrBelow("TV-Y7") {
		t.Error("TV-MA must NOT be at or below TV-Y7")
	}
	if !Rating("PG").atOrBelow("PG-13") {
		t.Error("PG (film) should be at or below PG-13 (film) on the shared ladder")
	}
	// Cross-system: G (film) and TV-G share a rank.
	if !Rating("G").atOrBelow("TV-G") || !Rating("TV-G").atOrBelow("G") {
		t.Error("G and TV-G should share a ladder rank")
	}
	// An unmapped rating is never "at or below" anything — it's unrated, decided by policy.
	if Rating("XYZ").atOrBelow("TV-MA") {
		t.Error("an unmapped rating must not compare as at-or-below")
	}
}

// normalizeRating folds the common media-server string variants onto the ladder.
func TestNormalizeRating(t *testing.T) {
	cases := map[string]Rating{
		"TV-Y7":   "TV-Y7",
		"tv-y7":   "TV-Y7",
		"TV_Y7":   "TV-Y7",
		"Rated R": "R",
		" pg-13 ": "PG-13",
		"NR":      "", // not rated → unrated
		"Unknown": "",
	}
	for raw, want := range cases {
		if got := normalizeRating(raw); got != want {
			t.Errorf("normalizeRating(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Validate rejects unknown enum values and off-ladder ceilings (grounding: the
// suggester must never emit a ceiling outside the closed ladder).
func TestValidate(t *testing.T) {
	if err := (ChannelPolicy{}).Validate(); err != nil {
		t.Errorf("empty policy should validate: %v", err)
	}
	valid := ChannelPolicy{
		ProposalPolicy: ProposalPolicy{
			Ordering: OrderSyndication,
			Audience: AudiencePolicy{Ceiling: "TV-Y7", Unrated: UnratedExclude},
			Seasonal: SeasonalPolicy{Mode: SeasonalAuto, OffSeason: OffSeasonLoop},
		},
		// A fully-specified filler selection with every closed-set value valid.
		OperatorPolicy: OperatorPolicy{Filler: &FillerSelection{
			Era: &Range{From: 1990, To: 1999}, Audience: "kids",
			Categories: []string{"toys", "cereal"}, Kinds: []string{"commercial", "bumper"},
			Pinned: []string{"prog-1"}, Excluded: []string{"prog-2"},
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid policy rejected: %v", err)
	}
	bad := []ChannelPolicy{
		{ProposalPolicy: ProposalPolicy{Ordering: "backwards"}},
		{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: "TV-BOGUS"}}},
		{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Unrated: "maybe"}}},
		{ProposalPolicy: ProposalPolicy{Seasonal: SeasonalPolicy{Mode: "sometimes"}}},
		{OperatorPolicy: OperatorPolicy{Filler: &FillerSelection{Audience: "nobody"}}},                // unknown audience
		{OperatorPolicy: OperatorPolicy{Filler: &FillerSelection{Kinds: []string{"advert"}}}},         // unknown kind
		{OperatorPolicy: OperatorPolicy{Filler: &FillerSelection{Categories: []string{"widgets"}}}},   // unknown category
		{OperatorPolicy: OperatorPolicy{Filler: &FillerSelection{Era: &Range{From: 1999, To: 1990}}}}, // inverted era range
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("bad policy %d should have been rejected", i)
		}
	}
}
