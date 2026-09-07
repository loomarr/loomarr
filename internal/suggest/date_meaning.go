package suggest

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
)

// DateMeaning is the untrusted, model-facing date interpretation. It deliberately
// contains only references into the submitted Intent; it does not infer dates from
// its text.
type DateMeaning struct {
	Kind    DateMeaningKind `json:"kind"`
	Anchors []DateAnchor    `json:"anchors,omitempty"`
	Axes    []DateAxis      `json:"axes,omitempty"`
}

type DateMeaningKind string

const (
	DateMeaningNone        DateMeaningKind = "none"
	DateMeaningConstraints DateMeaningKind = "constraints"
	DateMeaningAmbiguous   DateMeaningKind = "ambiguous"
)

// DateAnchor names a nonempty, half-open rune span in the submitted Intent.
// Index is required for an array field and forbidden for a scalar field.
type DateAnchor struct {
	Field DateAnchorField `json:"field"`
	Index *int            `json:"index,omitempty"`
	Start int             `json:"start"`
	End   int             `json:"end"`
}

type DateAnchorField string

const (
	DateAnchorDescription DateAnchorField = "description"
	DateAnchorEra         DateAnchorField = "era"
	DateAnchorRefineText  DateAnchorField = "refineText"
	DateAnchorMustInclude DateAnchorField = "mustInclude"
	DateAnchorMustExclude DateAnchorField = "mustExclude"
)

// DateAxis is one independent date requirement. Dates on different axes never
// intersect each other: a series' premiere and its airing window are distinct.
type DateAxis struct {
	Kind      DateAxisKind   `json:"kind"`
	Combine   DateCombine    `json:"combine"`
	Intervals []DateInterval `json:"intervals"`
}

type DateAxisKind string

const (
	DateAxisMovieRelease   DateAxisKind = "movie_release"
	DateAxisSeriesPremiere DateAxisKind = "series_premiere"
	DateAxisSeriesAiring   DateAxisKind = "series_airing"
)

type DateCombine string

const (
	DateCombineAny DateCombine = "any"
	DateCombineAll DateCombine = "all"
)

// DateInterval is an inclusive year range whose Anchor is an index in Anchors.
// Start and End are years, rather than text offsets.
type DateInterval struct {
	Anchor int `json:"anchor"`
	Start  int `json:"start"`
	End    int `json:"end"`
}

// DateMeaningError marks malformed model output. A conflict is deliberately a
// separate code: it is a well-formed constraints meaning that cannot be true.
type DateMeaningError struct {
	Code string
	Path string
}

func (e *DateMeaningError) Error() string {
	if e.Path == "" {
		return "date meaning: " + e.Code
	}
	return fmt.Sprintf("date meaning: %s at %s", e.Code, e.Path)
}

const DateMeaningConstraintsConflict = "constraints_conflict"

// ValidatedDateMeaning is an immutable canonical interpretation. DateMeaning
// returns a deep copy of its lossless, canonical source clauses. ExecutionWindows
// returns the derived date ranges for retrieval; unlike source clauses, windows
// deliberately have no anchor because a window can have several origins.
type ValidatedDateMeaning struct {
	meaning DateMeaning
	windows []DateExecutionAxis
}

// DateExecutionAxis is one axis lowered to canonical year windows. It is a
// derived API for execution, not model-facing JSON, so it has no anchors or
// combine mode.
type DateExecutionAxis struct {
	Kind    DateAxisKind
	Windows []DateYearRange
}

// DateYearRange is an inclusive execution range.
type DateYearRange struct {
	Start int
	End   int
}

// ValidateDateMeaning validates and canonicalizes raw model output for intent.
// A nil meaning represents missing model output and is malformed, not ambiguity.
func ValidateDateMeaning(intent Intent, raw *DateMeaning) (ValidatedDateMeaning, error) {
	if raw == nil {
		return ValidatedDateMeaning{}, dateMeaningErr("missing", "")
	}
	m := cloneDateMeaning(*raw)
	if err := validateDateMeaningShape(intent, m); err != nil {
		return ValidatedDateMeaning{}, err
	}
	canonicalizeAnchors(&m)
	return canonicalizeAxes(m)
}

// DateMeaning returns a detached canonical value suitable for comparison or
// rendering. Mutating it cannot alter the validated interpretation.
func (v ValidatedDateMeaning) DateMeaning() DateMeaning { return cloneDateMeaning(v.meaning) }

// ExecutionWindows returns detached, canonical retrieval ranges. Mutating the
// result cannot alter the validated interpretation.
func (v ValidatedDateMeaning) ExecutionWindows() []DateExecutionAxis {
	windows := append([]DateExecutionAxis(nil), v.windows...)
	for i := range windows {
		windows[i].Windows = append([]DateYearRange(nil), windows[i].Windows...)
	}
	return windows
}

// Equal compares canonical semantics and their source evidence.
func (v ValidatedDateMeaning) Equal(other ValidatedDateMeaning) bool {
	return v.meaning.Kind == other.meaning.Kind && slices.EqualFunc(v.meaning.Anchors, other.meaning.Anchors, func(a, b DateAnchor) bool {
		return a.Field == b.Field && sameIndex(a.Index, b.Index) && a.Start == b.Start && a.End == b.End
	}) && slices.EqualFunc(v.meaning.Axes, other.meaning.Axes, equalAxis) && slices.EqualFunc(v.windows, other.windows, equalExecutionAxis)
}

func validateDateMeaningShape(intent Intent, m DateMeaning) error {
	switch m.Kind {
	case DateMeaningNone:
		if len(m.Anchors) != 0 || len(m.Axes) != 0 {
			return dateMeaningErr("none_shape", "")
		}
		return nil
	case DateMeaningAmbiguous:
		if len(m.Anchors) == 0 || len(m.Axes) != 0 {
			return dateMeaningErr("ambiguous_shape", "")
		}
	case DateMeaningConstraints:
		if len(m.Axes) < 1 || len(m.Axes) > 3 {
			return dateMeaningErr("constraints_shape", "axes")
		}
	default:
		return dateMeaningErr("unknown_kind", "kind")
	}
	for i, anchor := range m.Anchors {
		if err := validateAnchor(intent, anchor); err != nil {
			return dateMeaningErr(err.Error(), fmt.Sprintf("anchors[%d]", i))
		}
	}
	if m.Kind != DateMeaningConstraints {
		return nil
	}
	seen := make(map[DateAxisKind]bool, len(m.Axes))
	for i, axis := range m.Axes {
		path := fmt.Sprintf("axes[%d]", i)
		if !validAxis(axis.Kind) {
			return dateMeaningErr("unknown_axis", path+".kind")
		}
		if seen[axis.Kind] {
			return dateMeaningErr("duplicate_axis", path+".kind")
		}
		seen[axis.Kind] = true
		if axis.Combine != DateCombineAny && axis.Combine != DateCombineAll {
			return dateMeaningErr("unknown_combine", path+".combine")
		}
		if len(axis.Intervals) < 1 || len(axis.Intervals) > 4 {
			return dateMeaningErr("interval_count", path+".intervals")
		}
		for j, interval := range axis.Intervals {
			if interval.Anchor < 0 || interval.Anchor >= len(m.Anchors) {
				return dateMeaningErr("invalid_anchor", fmt.Sprintf("%s.intervals[%d].anchor", path, j))
			}
			if interval.Start < 1900 || interval.End > 2099 || interval.Start > interval.End {
				return dateMeaningErr("invalid_range", fmt.Sprintf("%s.intervals[%d]", path, j))
			}
		}
	}
	return nil
}

func validateAnchor(intent Intent, a DateAnchor) error {
	var text string
	switch a.Field {
	case DateAnchorDescription:
		text = intent.Description
	case DateAnchorEra:
		text = intent.Era
	case DateAnchorRefineText:
		text = intent.RefineText
	case DateAnchorMustInclude, DateAnchorMustExclude:
		if a.Index == nil {
			return fmt.Errorf("missing_index")
		}
		values := intent.MustInclude
		if a.Field == DateAnchorMustExclude {
			values = intent.MustExclude
		}
		if *a.Index < 0 || *a.Index >= len(values) {
			return fmt.Errorf("invalid_index")
		}
		text = values[*a.Index]
	default:
		return fmt.Errorf("unknown_field")
	}
	if a.Field != DateAnchorMustInclude && a.Field != DateAnchorMustExclude && a.Index != nil {
		return fmt.Errorf("scalar_index")
	}
	runes := []rune(text)
	if a.Start < 0 || a.End <= a.Start || a.End > len(runes) {
		return fmt.Errorf("invalid_span")
	}
	return nil
}

func canonicalizeAnchors(m *DateMeaning) {
	old := append([]DateAnchor(nil), m.Anchors...)
	order := make([]int, len(old))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return compareAnchor(old[order[i]], old[order[j]]) < 0 })
	remap := make([]int, len(old))
	m.Anchors = m.Anchors[:0]
	for _, oldIndex := range order {
		a := old[oldIndex]
		if n := len(m.Anchors); n > 0 && compareAnchor(m.Anchors[n-1], a) == 0 {
			remap[oldIndex] = n - 1
			continue
		}
		remap[oldIndex] = len(m.Anchors)
		m.Anchors = append(m.Anchors, a)
	}
	for i := range m.Axes {
		for j := range m.Axes[i].Intervals {
			m.Axes[i].Intervals[j].Anchor = remap[m.Axes[i].Intervals[j].Anchor]
		}
	}
}

func canonicalizeAxes(m DateMeaning) (ValidatedDateMeaning, error) {
	if m.Kind != DateMeaningConstraints {
		return ValidatedDateMeaning{meaning: m}, nil
	}
	sort.Slice(m.Axes, func(i, j int) bool { return axisOrder(m.Axes[i].Kind) < axisOrder(m.Axes[j].Kind) })
	v := ValidatedDateMeaning{meaning: m, windows: make([]DateExecutionAxis, len(m.Axes))}
	for i := range v.meaning.Axes {
		axis := &v.meaning.Axes[i]
		axis.Intervals = canonicalSourceIntervals(axis.Combine, axis.Intervals)
		var windows []DateInterval
		if axis.Combine == DateCombineAny {
			windows = unionIntervals(axis.Intervals)
		} else {
			windows = intersectIntervals(axis.Intervals)
		}
		if len(windows) == 0 {
			return ValidatedDateMeaning{}, dateMeaningErr(DateMeaningConstraintsConflict, fmt.Sprintf("axes[%d]", i))
		}
		v.windows[i] = DateExecutionAxis{Kind: axis.Kind, Windows: dateYearRanges(windows)}
	}
	return v, nil
}

func canonicalSourceIntervals(combine DateCombine, in []DateInterval) []DateInterval {
	sorted := append([]DateInterval(nil), in...)
	sort.Slice(sorted, func(i, j int) bool {
		return cmp.Or(cmp.Compare(sorted[i].Anchor, sorted[j].Anchor), cmp.Compare(sorted[i].Start, sorted[j].Start), cmp.Compare(sorted[i].End, sorted[j].End)) < 0
	})
	out := make([]DateInterval, 0, len(sorted))
	for _, x := range sorted {
		if len(out) == 0 || x.Anchor != out[len(out)-1].Anchor {
			out = append(out, x)
			continue
		}
		last := &out[len(out)-1]
		if combine == DateCombineAny && x.Start <= last.End+1 {
			last.End = max(last.End, x.End)
			continue
		}
		if combine == DateCombineAll {
			last.Start = max(last.Start, x.Start)
			last.End = min(last.End, x.End)
			if last.Start > last.End {
				return sorted
			}
			continue
		}
		out = append(out, x)
	}
	return out
}

func unionIntervals(in []DateInterval) []DateInterval {
	sorted := append([]DateInterval(nil), in...)
	sort.Slice(sorted, func(i, j int) bool {
		return cmp.Or(sorted[i].Start-sorted[j].Start, sorted[i].End-sorted[j].End, sorted[i].Anchor-sorted[j].Anchor) < 0
	})
	out := make([]DateInterval, 0, len(sorted))
	for _, x := range sorted {
		if len(out) == 0 || x.Start > out[len(out)-1].End+1 {
			out = append(out, x)
			continue
		}
		last := &out[len(out)-1]
		if x.End > last.End {
			last.End = x.End
		}
		if x.Anchor < last.Anchor {
			last.Anchor = x.Anchor
		}
	}
	return out
}

func intersectIntervals(in []DateInterval) []DateInterval {
	start, end := in[0].Start, in[0].End
	anchor := in[0].Anchor
	for _, x := range in {
		start = max(start, x.Start)
		end = min(end, x.End)
		anchor = min(anchor, x.Anchor)
	}
	if start > end {
		return nil
	}
	return []DateInterval{{Anchor: anchor, Start: start, End: end}}
}

func dateYearRanges(intervals []DateInterval) []DateYearRange {
	ranges := make([]DateYearRange, len(intervals))
	for i, interval := range intervals {
		ranges[i] = DateYearRange{Start: interval.Start, End: interval.End}
	}
	return ranges
}

func cloneDateMeaning(m DateMeaning) DateMeaning {
	m.Anchors = append([]DateAnchor(nil), m.Anchors...)
	for i := range m.Anchors {
		if m.Anchors[i].Index != nil {
			n := *m.Anchors[i].Index
			m.Anchors[i].Index = &n
		}
	}
	m.Axes = append([]DateAxis(nil), m.Axes...)
	for i := range m.Axes {
		m.Axes[i].Intervals = append([]DateInterval(nil), m.Axes[i].Intervals...)
	}
	return m
}
func dateMeaningErr(code, path string) error { return &DateMeaningError{Code: code, Path: path} }
func validAxis(k DateAxisKind) bool {
	return k == DateAxisMovieRelease || k == DateAxisSeriesPremiere || k == DateAxisSeriesAiring
}
func axisOrder(k DateAxisKind) int {
	switch k {
	case DateAxisMovieRelease:
		return 0
	case DateAxisSeriesPremiere:
		return 1
	default:
		return 2
	}
}
func sameIndex(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func compareAnchor(a, b DateAnchor) int {
	return cmp.Or(cmp.Compare(string(a.Field), string(b.Field)), compareIndex(a.Index, b.Index), cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End))
}
func compareIndex(a, b *int) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	return cmp.Compare(*a, *b)
}
func equalAxis(a, b DateAxis) bool {
	return a.Kind == b.Kind && a.Combine == b.Combine && slices.Equal(a.Intervals, b.Intervals)
}
func equalExecutionAxis(a, b DateExecutionAxis) bool {
	return a.Kind == b.Kind && slices.Equal(a.Windows, b.Windows)
}
