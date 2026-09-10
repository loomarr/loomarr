package suggest

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateDateMeaningRejectsMalformedValues(t *testing.T) {
	index := 0
	intent := Intent{Description: "café classics", Era: "the nineties", MustInclude: []string{"épisodes", "Star Trek"}}
	validAnchor := DateAnchor{Field: DateAnchorDescription, Start: 0, End: 4}
	cases := []struct {
		name    string
		meaning *DateMeaning
		code    string
	}{
		{"missing", nil, "missing"},
		{"unknown kind", &DateMeaning{Kind: "maybe"}, "unknown_kind"},
		{"none carries data", &DateMeaning{Kind: DateMeaningNone, Anchors: []DateAnchor{validAnchor}}, "none_shape"},
		{"ambiguous needs anchor", &DateMeaning{Kind: DateMeaningAmbiguous}, "ambiguous_shape"},
		{"scalar index", &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: DateAnchorDescription, Index: &index, End: 1}}}, "scalar_index"},
		{"bad rune span", &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: DateAnchorDescription, Start: 3, End: 20}}}, "invalid_span"},
		{"bad array index", &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: DateAnchorMustInclude, Index: ptr(2), End: 1}}}, "invalid_index"},
		{"unknown axis", &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{validAnchor}, Axes: []DateAxis{axis("title_date", DateCombineAny, 0, 1990, 1991)}}, "unknown_axis"},
		{"unknown field", &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: "title", End: 1}}}, "unknown_field"},
		{"duplicate axis", &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{validAnchor}, Axes: []DateAxis{axis(DateAxisMovieRelease, DateCombineAny, 0, 1990, 1991), axis(DateAxisMovieRelease, DateCombineAny, 0, 1992, 1993)}}, "duplicate_axis"},
		{"unknown combine", &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{validAnchor}, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: "both", Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1991}}}}}, "unknown_combine"},
		{"invalid reference", &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{validAnchor}, Axes: []DateAxis{axis(DateAxisMovieRelease, DateCombineAny, 1, 1990, 1991)}}, "invalid_anchor"},
		{"invalid range", &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{validAnchor}, Axes: []DateAxis{axis(DateAxisMovieRelease, DateCombineAny, 0, 1899, 1991)}}, "invalid_range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateDateMeaning(intent, tc.meaning)
			var meaningErr *DateMeaningError
			if !errors.As(err, &meaningErr) || meaningErr.Code != tc.code {
				t.Fatalf("error = %#v, want code %q", err, tc.code)
			}
		})
	}
}

func TestValidateIntentDateMeaningRequiresExplicitDateAcknowledgement(t *testing.T) {
	for name, intent := range map[string]Intent{
		"episode decade": {Description: "Play sitcom episodes from the 1990s in episode order."},
		"year range":     {Description: "Show films from 1990 through 1999."},
		"era field":      {Description: "Build an action channel.", Era: "1990s"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := validateIntentDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
			var meaningErr *DateMeaningError
			if !errors.As(err, &meaningErr) || meaningErr.Code != "unacknowledged_date" {
				t.Fatalf("error = %#v, want unacknowledged_date", err)
			}
		})
	}
}

func TestValidateIntentDateMeaningDoesNotTreatBareTitleYearAsFilter(t *testing.T) {
	for _, description := range []string{
		"Build a channel around 2001: A Space Odyssey.",
		"Build a channel around That '70s Show.",
	} {
		if _, err := validateIntentDateMeaning(Intent{Description: description}, &DateMeaning{Kind: DateMeaningNone}); err != nil {
			t.Fatalf("description %q: %v", description, err)
		}
	}
}

func TestValidateDateMeaningIndexesArrayAnchorsByRunes(t *testing.T) {
	meaning := &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: DateAnchorMustInclude, Index: ptr(0), Start: 0, End: 3}}}
	got, err := ValidateDateMeaning(Intent{MustInclude: []string{"été films"}}, meaning)
	if err != nil {
		t.Fatal(err)
	}
	if got.DateMeaning().Anchors[0].End != 3 {
		t.Fatalf("anchor = %#v", got.DateMeaning().Anchors[0])
	}
}

func TestValidateDateMeaningCanonicalizesAndPreservesEvidence(t *testing.T) {
	intent := Intent{Description: "1990s and 2000s films", Era: "1990s"}
	// The source fields sort independently of their submitted order, and the
	// duplicated era anchor becomes one anchor with all interval references remapped.
	raw := &DateMeaning{Kind: DateMeaningConstraints,
		Anchors: []DateAnchor{{Field: DateAnchorEra, Start: 0, End: 4}, {Field: DateAnchorDescription, Start: 0, End: 5}, {Field: DateAnchorEra, Start: 0, End: 4}},
		Axes: []DateAxis{
			axis(DateAxisSeriesAiring, DateCombineAny, 0, 2000, 2002),
			{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 2, Start: 1992, End: 1994}, {Anchor: 1, Start: 1990, End: 1991}, {Anchor: 0, Start: 1995, End: 1995}}},
		},
	}
	got, err := ValidateDateMeaning(intent, raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical := got.DateMeaning()
	if len(canonical.Anchors) != 2 || canonical.Anchors[0].Field != DateAnchorDescription {
		t.Fatalf("anchors = %#v", canonical.Anchors)
	}
	if canonical.Axes[0].Kind != DateAxisMovieRelease {
		t.Fatalf("axis order = %#v", canonical.Axes)
	}
	if clauses := canonical.Axes[0].Intervals; len(clauses) != 2 || clauses[0].Anchor != 0 || clauses[0].Start != 1990 || clauses[0].End != 1991 || clauses[1].Anchor != 1 || clauses[1].Start != 1992 || clauses[1].End != 1995 {
		t.Fatalf("source clauses = %#v", clauses)
	}
	if windows := got.ExecutionWindows()[0].Windows; len(windows) != 1 || windows[0].Start != 1990 || windows[0].End != 1995 {
		t.Fatalf("execution union = %#v", windows)
	}

	disjoint := &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{{Field: DateAnchorDescription, Start: 0, End: 5}}, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1991}, {Anchor: 0, Start: 2000, End: 2001}}}}}
	disjointCanonical, err := ValidateDateMeaning(intent, disjoint)
	if err != nil {
		t.Fatal(err)
	}
	if windows := disjointCanonical.DateMeaning().Axes[0].Intervals; len(windows) != 2 {
		t.Fatalf("disjoint union was collapsed into a hull: %#v", windows)
	}

	reordered := &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{{Field: DateAnchorDescription, Start: 0, End: 5}, {Field: DateAnchorEra, Start: 0, End: 4}}, Axes: []DateAxis{
		{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 1, Start: 1995, End: 1995}, {Anchor: 0, Start: 1990, End: 1991}, {Anchor: 0, Start: 1990, End: 1991}, {Anchor: 1, Start: 1992, End: 1994}}},
		axis(DateAxisSeriesAiring, DateCombineAny, 1, 2000, 2002),
	}}
	equivalent, err := ValidateDateMeaning(intent, reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(equivalent) {
		t.Fatal("reordered equivalent meanings differ")
	}

	differentSource := *reordered
	differentSource.Anchors = append([]DateAnchor(nil), reordered.Anchors...)
	differentSource.Anchors[0].Start = 1
	different, err := ValidateDateMeaning(intent, &differentSource)
	if err != nil {
		t.Fatal(err)
	}
	if got.Equal(different) {
		t.Fatal("different source spans compare equal")
	}

	canonical.Anchors[0].Start = 99
	canonical.Axes[0].Intervals[0].Start = 1
	if after := got.DateMeaning(); after.Anchors[0].Start == 99 || after.Axes[0].Intervals[0].Start == 1 {
		t.Fatal("validated meaning leaked mutable storage")
	}
}

func TestValidateDateMeaningCanonicalBoundaryAndEquality(t *testing.T) {
	intent := Intent{Description: "films from 1990 through 1999"}
	anchor := DateAnchor{Field: DateAnchorDescription, Start: 0, End: 5}
	joined := &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{anchor}, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1999}}}}}
	split := &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{anchor}, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1994}, {Anchor: 0, Start: 1995, End: 1999}, {Anchor: 0, Start: 1992, End: 1993}}}}}
	whole, err := ValidateDateMeaning(intent, joined)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := ValidateDateMeaning(intent, split)
	if err != nil {
		t.Fatal(err)
	}
	if !whole.Equal(parts) {
		t.Fatal("equivalent adjacent and overlapping source ranges differ")
	}

	zero := ValidatedDateMeaning{}
	none, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	if zero.Equal(none) {
		t.Fatal("zero validated value equals valid none")
	}

	differentOrigin := *joined
	differentOrigin.Anchors = []DateAnchor{{Field: DateAnchorDescription, Start: 1, End: 5}}
	different, err := ValidateDateMeaning(intent, &differentOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if whole.Equal(different) {
		t.Fatal("different origin spans compare equal")
	}

	public := whole.DateMeaning()
	roundTrip, err := ValidateDateMeaning(intent, &public)
	if err != nil {
		t.Fatal(err)
	}
	if !whole.Equal(roundTrip) {
		t.Fatal("public canonical representation does not round trip")
	}
	public.Axes[0].Intervals[0].Start = 1
	windows := whole.ExecutionWindows()
	windows[0].Windows[0].Start = 1
	if after := whole.DateMeaning(); after.Axes[0].Intervals[0].Start != 1990 {
		t.Fatal("public clauses mutate validated state")
	}
	if after := whole.ExecutionWindows(); after[0].Windows[0].Start != 1990 {
		t.Fatal("execution windows mutate validated state")
	}
}

func TestDateMeaningNoneMarshalsAsStrictEmptyArrays(t *testing.T) {
	validated, err := ValidateDateMeaning(Intent{}, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(validated.DateMeaning())
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"kind":"none","anchors":[],"axes":[]}` {
		t.Fatalf("JSON = %s", encoded)
	}
	decoded, err := decodeDateMeaning(encoded)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := ValidateDateMeaning(Intent{}, &decoded)
	if err != nil || !validated.Equal(roundTrip) {
		t.Fatalf("round trip = %#v, %v", roundTrip, err)
	}
}

func TestValidateDateMeaningPreservesMultiAnchorSourceClauses(t *testing.T) {
	intent := Intent{Description: "1990 films", Era: "1991 films"}
	raw := &DateMeaning{Kind: DateMeaningConstraints, Anchors: []DateAnchor{{Field: DateAnchorDescription, Start: 0, End: 4}, {Field: DateAnchorEra, Start: 0, End: 4}}, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: DateCombineAny, Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1994}, {Anchor: 1, Start: 1995, End: 1999}}}}}
	got, err := ValidateDateMeaning(intent, raw)
	if err != nil {
		t.Fatal(err)
	}
	clauses := got.DateMeaning().Axes[0].Intervals
	if len(clauses) != 2 || clauses[0].Anchor == clauses[1].Anchor {
		t.Fatalf("multi-anchor source evidence lost: %#v", clauses)
	}
	roundTripRaw := got.DateMeaning()
	roundTrip, err := ValidateDateMeaning(intent, &roundTripRaw)
	if err != nil || !got.Equal(roundTrip) {
		t.Fatalf("multi-anchor public representation did not round trip: %v", err)
	}
	if windows := got.ExecutionWindows()[0].Windows; len(windows) != 1 || windows[0] != (DateYearRange{Start: 1990, End: 1999}) {
		t.Fatalf("execution windows = %#v", windows)
	}
}

func TestValidateDateMeaningAllConflictAndCrossAxisIndependence(t *testing.T) {
	intent := Intent{Description: "a long running show", Era: "the nineties"}
	anchors := []DateAnchor{{Field: DateAnchorDescription, Start: 0, End: 1}, {Field: DateAnchorEra, Start: 0, End: 3}}
	conflict := &DateMeaning{Kind: DateMeaningConstraints, Anchors: anchors, Axes: []DateAxis{{Kind: DateAxisMovieRelease, Combine: DateCombineAll, Intervals: []DateInterval{{Anchor: 0, Start: 1990, End: 1992}, {Anchor: 1, Start: 1995, End: 1997}}}}}
	_, err := ValidateDateMeaning(intent, conflict)
	var meaningErr *DateMeaningError
	if !errors.As(err, &meaningErr) || meaningErr.Code != DateMeaningConstraintsConflict {
		t.Fatalf("conflict error = %#v", err)
	}

	independent := &DateMeaning{Kind: DateMeaningConstraints, Anchors: anchors, Axes: []DateAxis{
		axis(DateAxisSeriesPremiere, DateCombineAll, 0, 1990, 1992),
		axis(DateAxisSeriesAiring, DateCombineAll, 1, 2000, 2002),
	}}
	got, err := ValidateDateMeaning(intent, independent)
	if err != nil {
		t.Fatalf("cross-axis ranges must remain independent: %v", err)
	}
	if len(got.DateMeaning().Axes) != 2 {
		t.Fatal("cross-axis requirements collapsed")
	}
}

func TestValidateDateMeaningAmbiguityIsValid(t *testing.T) {
	got, err := ValidateDateMeaning(Intent{Description: "classic movies"}, &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{{Field: DateAnchorDescription, Start: 0, End: 7}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.DateMeaning().Kind != DateMeaningAmbiguous {
		t.Fatalf("kind = %q", got.DateMeaning().Kind)
	}
}

func axis(kind DateAxisKind, combine DateCombine, anchor, start, end int) DateAxis {
	return DateAxis{Kind: kind, Combine: combine, Intervals: []DateInterval{{Anchor: anchor, Start: start, End: end}}}
}
func ptr(v int) *int { return &v }
