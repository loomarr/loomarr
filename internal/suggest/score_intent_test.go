package suggest_test

import (
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
	"reflect"
	"testing"
)

func TestScoreIntentCounterexamples(t *testing.T) {
	t.Run("one qualifier is not complete adherence", func(t *testing.T) {
		scores := suggest.ScoreForTest(suggest.Intent{Description: "cozy British murder mysteries"}, []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "A Case", Overview: "A murder investigation", Year: 1994}}, nil)
		if scores.ThemeFit == nil || *scores.ThemeFit >= 1 {
			t.Fatalf("one matching qualifier received complete theme credit: %+v", scores)
		}
	})
	t.Run("off era diversity cannot improve alignment", func(t *testing.T) {
		intent := suggest.Intent{Description: "1990s movies", Era: "1990s"}
		matching := []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "First", Year: 1993}, {MediaType: provision.Movie, TMDBID: 2, Name: "Second", Year: 1995}}
		meaning := movieDateMeaning(t, intent)
		before := suggest.ScoreWithDateForTest(intent, matching, nil, meaning)
		after := suggest.ScoreWithDateForTest(intent, append(matching, suggest.ProposalItem{MediaType: provision.Movie, TMDBID: 3, Name: "Older", Year: 1982}), nil, meaning)
		if before.EraBalance == nil || after.EraBalance == nil || *before.EraBalance != 1 || *after.EraBalance >= *before.EraBalance {
			t.Fatalf("off-era title improved alignment: before=%+v after=%+v", before, after)
		}
	})
	t.Run("unknown years remain unassessed", func(t *testing.T) {
		intent := suggest.Intent{Era: "1990s"}
		scores := suggest.ScoreWithDateForTest(intent, []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "Unknown date"}}, nil, movieDateMeaning(t, intent))
		if scores.EraBalance != nil {
			t.Fatalf("unknown year earned era credit: %+v", scores)
		}
	})
	t.Run("no era is not an adherence score", func(t *testing.T) {
		scores := suggest.ScoreForTest(suggest.Intent{Description: "movies"}, []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 1, Name: "Any year", Year: 1994}}, nil)
		if scores.EraBalance != nil {
			t.Fatalf("unrequested era earned adherence credit: %+v", scores)
		}
	})
}

func movieDateMeaning(t *testing.T, intent suggest.Intent) suggest.ValidatedDateMeaning {
	t.Helper()
	meaning, err := suggest.ValidateDateMeaning(intent, &suggest.DateMeaning{
		Kind:    suggest.DateMeaningConstraints,
		Anchors: []suggest.DateAnchor{{Field: suggest.DateAnchorEra, Start: 0, End: 5}},
		Axes:    []suggest.DateAxis{{Kind: suggest.DateAxisMovieRelease, Combine: suggest.DateCombineAny, Intervals: []suggest.DateInterval{{Anchor: 0, Start: 1990, End: 1999}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return meaning
}

func TestThemeEvidenceDoesNotConfuseMetadataWithUnderstanding(t *testing.T) {
	cases := []struct {
		name   string
		intent string
		item   suggest.ProposalItem
		want   *float64
		status string
	}{
		{"partial nonsense is explained", "asdf banana xqz 999", suggest.ProposalItem{Name: "Banana Fish", Overview: "A crime drama"}, new(0.0), "partial"},
		{"title cannot borrow unrelated metadata", "British murder mysteries", suggest.ProposalItem{Name: "British Murder Mysteries", Overview: "A documentary about undersea exploration"}, new(0.0), "partial"},
		{"title alone is insufficient", "banana", suggest.ProposalItem{Name: "Banana Fish"}, nil, "unassessed"},
		{"substrings do not prove a theme", "war", suggest.ProposalItem{Name: "Reward", Overview: "A heartwarming drama"}, new(0.0), "partial"},
		{"known equivalents and country", "cozy British murder mysteries", suggest.ProposalItem{Overview: "A cosy murder whodunit", OriginCountries: []string{"GB"}}, new(1.0), "supported"},
		{"murder mystery is one compound", "UK murder mystery series", suggest.ProposalItem{Genres: []string{"Mystery"}, OriginCountries: []string{"GB"}}, new(1.0), "supported"},
		{"science fiction equivalent", "sci-fi", suggest.ProposalItem{Genres: []string{"Science Fiction"}}, new(1.0), "supported"},
		{"rationale cannot fill metadata gap", "space", suggest.ProposalItem{Name: "Unrelated", Rationale: "Space space space"}, nil, "unassessed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := suggest.ScoreForTest(suggest.Intent{Description: tc.intent}, []suggest.ProposalItem{tc.item}, nil)
			if !reflect.DeepEqual(s.ThemeFit, tc.want) || s.Theme.Status != tc.status {
				t.Fatalf("assessment = %+v, want %v/%s", s, tc.want, tc.status)
			}
		})
	}
}

func TestThemeEvidenceUsesDedicatedGroundingFields(t *testing.T) {
	tests := []struct {
		name   string
		intent string
		item   suggest.ProposalItem
	}{
		{
			name:   "network",
			intent: "HBO drama series",
			item:   suggest.ProposalItem{Genres: []string{"Drama"}, Networks: []string{"HBO"}},
		},
		{
			name:   "people",
			intent: "Tom Hanks movies written or directed by Nora Ephron",
			item: suggest.ProposalItem{
				Cast: []string{"Tom Hanks"}, Creators: []string{"Nora Ephron"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scores := suggest.ScoreForTest(suggest.Intent{Description: tc.intent}, []suggest.ProposalItem{tc.item}, nil)
			if scores.ThemeFit == nil || *scores.ThemeFit != 1 || scores.Theme.Status != "supported" {
				t.Fatalf("dedicated evidence was reported as a loose match: %+v", scores)
			}
		})
	}
}

func TestThemeEvidenceUsesGroundedSeriesNameForEpisodeSubject(t *testing.T) {
	scores := suggest.ScoreForTest(
		suggest.Intent{Description: "Classic Simpsons: the best episodes, not a full binge."},
		[]suggest.ProposalItem{{MediaType: provision.Series, Name: "The Simpsons", Overview: "A family in Springfield"}}, nil,
	)
	if scores.ThemeFit == nil || *scores.ThemeFit != 1 || scores.Theme.Status != "supported" {
		t.Fatalf("grounded episode subject was reported as a loose match: %+v", scores)
	}
	if len(scores.Theme.Qualifiers) != 1 || scores.Theme.Qualifiers[0].Term != "simpsons" {
		t.Fatalf("editorial controls leaked into theme qualifiers: %+v", scores.Theme.Qualifiers)
	}
}

func TestThemeEvidenceIgnoresDirectTitleInstructions(t *testing.T) {
	scores := suggest.ScoreForTest(
		suggest.Intent{Description: "family sitcoms; include Boy Meets World and exclude Married... with Children."},
		[]suggest.ProposalItem{{MediaType: provision.Series, Name: "Boy Meets World", Genres: []string{"Comedy", "Family"}}}, nil,
	)
	if scores.ThemeFit == nil || *scores.ThemeFit != 1 || scores.Theme.Status != "supported" {
		t.Fatalf("direct title instructions polluted theme evidence: %+v", scores)
	}
	if len(scores.Theme.Qualifiers) != 2 || scores.Theme.Qualifiers[0].Term != "family" || scores.Theme.Qualifiers[1].Term != "sitcom" {
		t.Fatalf("direct title instructions leaked into qualifiers: %+v", scores.Theme.Qualifiers)
	}
}

func TestScoreDateAnchorsAreNotThemeQualifiers(t *testing.T) {
	intent := suggest.Intent{Description: "é action 1990s", Era: "1990s", MustInclude: []string{"1990s"}}
	index := 0
	meaning, err := suggest.ValidateDateMeaning(intent, &suggest.DateMeaning{
		Kind: suggest.DateMeaningConstraints,
		Anchors: []suggest.DateAnchor{
			{Field: suggest.DateAnchorDescription, Start: 9, End: 14},
			{Field: suggest.DateAnchorEra, Start: 0, End: 5},
			{Field: suggest.DateAnchorMustInclude, Index: &index, Start: 0, End: 5},
		},
		Axes: []suggest.DateAxis{{Kind: suggest.DateAxisMovieRelease, Combine: suggest.DateCombineAny,
			Intervals: []suggest.DateInterval{{Anchor: 0, Start: 1990, End: 1999}, {Anchor: 1, Start: 1990, End: 1999}, {Anchor: 2, Start: 1990, End: 1999}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := suggest.ScoreWithDateForTest(intent, []suggest.ProposalItem{{MediaType: provision.Movie, Year: 1994, Genres: []string{"Action"}, Overview: "é"}}, nil, meaning)
	if s.ThemeFit == nil || *s.ThemeFit != 1 || len(s.Theme.Qualifiers) != 2 || s.EraBalance == nil || *s.EraBalance != 1 {
		t.Fatalf("date anchors counted as theme or failed date adherence: %+v", s)
	}
	if intent.MustInclude[0] != "1990s" {
		t.Fatal("scoring mutated submitted intent")
	}
}

func TestScoreOversizedDateAnchorKeepsAdjacentTheme(t *testing.T) {
	intent := suggest.Intent{Description: "1990s family sitcoms"}
	meaning, err := suggest.ValidateDateMeaning(intent, &suggest.DateMeaning{
		Kind: suggest.DateMeaningConstraints,
		Anchors: []suggest.DateAnchor{
			{Field: suggest.DateAnchorDescription, Start: 0, End: len("1990s")},
			{Field: suggest.DateAnchorDescription, Start: 0, End: len([]rune(intent.Description))},
		},
		Axes: []suggest.DateAxis{{Kind: suggest.DateAxisSeriesPremiere, Combine: suggest.DateCombineAny,
			Intervals: []suggest.DateInterval{{Anchor: 0, Start: 1990, End: 1999}, {Anchor: 1, Start: 1990, End: 1999}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scores := suggest.ScoreWithDateForTest(intent, []suggest.ProposalItem{{
		MediaType: provision.Series, Year: 1993, Genres: []string{"Comedy", "Family"},
	}}, nil, meaning)
	if scores.ThemeFit == nil || *scores.ThemeFit != 1 || scores.Theme.Status != "supported" {
		t.Fatalf("oversized date anchor hid adjacent theme evidence: %+v", scores)
	}
	if len(scores.Theme.Qualifiers) != 2 || scores.Theme.Qualifiers[0].Term != "family" || scores.Theme.Qualifiers[1].Term != "sitcom" {
		t.Fatalf("theme qualifiers = %+v, want family and sitcom", scores.Theme.Qualifiers)
	}
}

func TestScoreMissingMetadataCannotBorrowOtherTitlesEvidence(t *testing.T) {
	s := suggest.ScoreForTest(suggest.Intent{Description: "action"}, []suggest.ProposalItem{{Genres: []string{"Action"}}, {Name: "Action"}}, nil)
	if s.ThemeFit != nil || s.Theme.UnknownItems != 1 || s.Theme.AssessedItems != 1 || s.Theme.Qualifiers[0].SupportedItems != 1 {
		t.Fatalf("unknown metadata became a numeric claim: %+v", s)
	}
}
