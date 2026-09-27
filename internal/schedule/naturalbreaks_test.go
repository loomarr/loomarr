package schedule

import (
	"slices"
	"testing"
)

const minute = int64(60_000)

func fades(atMin ...float64) []BreakCandidate {
	out := make([]BreakCandidate, 0, len(atMin))
	for _, m := range atMin {
		out = append(out, BreakCandidate{AtMs: int64(m * float64(minute)), Confidence: 1})
	}
	return out
}

func mins(ms []int64) []float64 {
	out := make([]float64, 0, len(ms))
	for _, v := range ms {
		out = append(out, float64(v)/float64(minute))
	}
	return out
}

func TestPlaceMidRollCuts(t *testing.T) {
	p := DefaultMidRollPolicy(4) // a break due every 15 min
	for _, tc := range []struct {
		name       string
		duration   int64
		since      int64
		candidates []BreakCandidate
		policy     *MidRollPolicy
		want       []float64
	}{
		{name: "no candidates means no mid-roll breaks", duration: 120 * minute, want: nil},
		{name: "breaks off (0 per hour) means none", duration: 120 * minute, candidates: fades(15, 30), policy: &MidRollPolicy{}, want: nil},
		{name: "a half-hour sitcom airs whole", duration: 22 * minute, candidates: fades(11), want: nil},
		{
			name: "an hour-long episode breaks at the fades nearest each quarter hour", duration: 44 * minute,
			candidates: fades(9, 14, 16.5, 29.5, 33), want: []float64{14, 29.5},
		},
		{
			name: "a film breaks through its whole runtime", duration: 118 * minute,
			candidates: fades(13, 31, 44, 61, 74, 88, 103), want: []float64{13, 31, 44, 61, 74, 88, 103},
		},
		{
			name: "a due break with no fade inside ±3 min is skipped, never forced", duration: 90 * minute,
			candidates: fades(15, 38, 45), want: []float64{15, 45},
		},
		{
			name: "cadence follows the actual cut, not the original grid", duration: 90 * minute,
			candidates: fades(17.5, 33, 35), want: []float64{17.5, 33},
		},
		{
			// 14 min since the last break puts the first due point at the 8 min floor, so the
			// cold-open fade at 6 min sits inside its ±3 window and only the part floor rejects it.
			name: "a fade in the cold open is rejected", duration: 60 * minute, since: 14 * minute,
			candidates: fades(6, 23), want: []float64{23},
		},
		{
			name: "a fade in the closing minutes is rejected", duration: 50 * minute,
			candidates: fades(15, 30, 44), want: []float64{15, 30},
		},
		{
			name: "runtime since the last break pulls the first break earlier", duration: 60 * minute, since: 5 * minute,
			candidates: fades(10, 25, 40), want: []float64{10, 25, 40},
		},
		{
			// Measured live: an hour drama's act-break fades, 11.7 min after the previous break.
			// A ±3 min window skipped every one of them (41 min without a break); ±5 takes two.
			name: "act breaks off the quarter-hour grid are within the shared window", duration: 41.2 * 60_000, since: 11.7 * 60_000,
			candidates: fades(11.23, 18.47, 28.1, 33.6, 40.56), want: []float64{11.23, 28.1},
		},
		{
			name: "a faint coincidence is not a scene fade", duration: 60 * minute,
			candidates: []BreakCandidate{{AtMs: 15 * minute, Confidence: 0.1}, {AtMs: 30 * minute, Confidence: 0.9}},
			want:       []float64{30},
		},
		{
			name: "equidistant fades: the more certain one wins", duration: 60 * minute,
			candidates: []BreakCandidate{{AtMs: 14 * minute, Confidence: 0.5}, {AtMs: 16 * minute, Confidence: 1}},
			want:       []float64{16},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := p
			if tc.policy != nil {
				policy = *tc.policy
			}
			got := mins(PlaceMidRollCuts(tc.duration, tc.since, tc.candidates, policy))
			if !slices.Equal(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
				t.Fatalf("cuts = %v min, want %v", got, tc.want)
			}
		})
	}
}

type fadeMap map[string][]BreakCandidate

func (f fadeMap) NaturalBreaks(id string) []BreakCandidate { return f[id] }

// A long programme airs as parts with a break between each, resuming at the exact cut; the
// between-programme cadence is unchanged and continues from the last part.
func TestInterleaveBreaksSplitsLongProgrammesAtFades(t *testing.T) {
	ch := Channel{BreaksPerHour: 4, BreakDurationMs: 30_000, NaturalBreaks: fadeMap{
		"film": fades(14.5, 31, 44), // 44 leaves a 16 min tail
		"ep":   fades(10),           // a half-hour episode is never split
	}}
	slots := []Slot{
		{Kind: SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "film", Title: "Film", DurationMs: 60 * minute},
		{Kind: SlotProgram, Key: "series:tvdb:2", LibraryItemID: "ep", Title: "Ep", DurationMs: 22 * minute},
		{Kind: SlotProgram, Key: "movie:tmdb:3", LibraryItemID: "unmeasured", Title: "Other", DurationMs: 90 * minute},
	}
	got := interleaveBreaks(ch, slots)
	type row struct {
		kind            SlotKind
		lib             string
		seg             int
		fromMin, durMin float64
		midRoll         bool
	}
	var rows []row
	for _, s := range got {
		rows = append(rows, row{s.Kind, s.LibraryItemID, s.Segment, float64(s.SourceOffsetMs) / float64(minute), float64(s.DurationMs) / float64(minute), s.MidRoll})
	}
	brk := func(mid bool) row { return row{kind: SlotFiller, durMin: 0.5, midRoll: mid} }
	want := []row{
		{SlotProgram, "film", 1, 0, 14.5, false}, brk(true),
		{SlotProgram, "film", 2, 14.5, 16.5, false}, brk(true),
		{SlotProgram, "film", 3, 31, 13, false}, brk(true),
		{SlotProgram, "film", 4, 44, 16, false}, brk(false), // tail 16 min ≥ 15: today's boundary break
		{SlotProgram, "ep", 0, 0, 22, false}, brk(false),
		{SlotProgram, "unmeasured", 0, 0, 90, false}, // last programme: no trailing break
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("slots:\n got %v\nwant %v", rows, want)
	}
	if n := (DesiredLineup{Slots: got}).ProgramCount(); n != 3 {
		t.Fatalf("ProgramCount = %d, want 3 (parts of one airing are one programme)", n)
	}
}

// Without a candidate source (Tunarr, the per-channel off switch) nothing changes.
func TestInterleaveBreaksWithoutNaturalBreaksIsUnchanged(t *testing.T) {
	slots := []Slot{
		{Kind: SlotProgram, LibraryItemID: "film", DurationMs: 60 * minute},
		{Kind: SlotProgram, LibraryItemID: "ep", DurationMs: 22 * minute},
	}
	got := interleaveBreaks(Channel{BreaksPerHour: 4}, slots)
	if len(got) != 3 || got[0].Segment != 0 || got[1].Kind != SlotFiller || got[1].MidRoll {
		t.Fatalf("slots = %+v", got)
	}
}

// A programme already on air (or about to be) keeps the split the accepted cycle gave it: a newly
// measured fade never re-cuts it, and switching mid-roll off never un-cuts it. New splits apply to
// later airings only.
func TestInterleaveBreaksKeepsPinnedAiringsAsAccepted(t *testing.T) {
	slots := []Slot{
		{Kind: SlotProgram, LibraryItemID: "on-air", DurationMs: 60 * minute},
		{Kind: SlotProgram, LibraryItemID: "on-air-split", DurationMs: 60 * minute},
		{Kind: SlotProgram, LibraryItemID: "later", DurationMs: 60 * minute},
	}
	ch := Channel{BreaksPerHour: 4, BreakDurationMs: 30_000,
		NaturalBreaks: fadeMap{"on-air": fades(15, 30), "on-air-split": fades(14, 31), "later": fades(15, 30)},
		PinnedCuts:    map[string][]int64{"on-air": nil, "on-air-split": {20 * minute}},
	}
	cutsOf := func(got []Slot) map[string][]float64 {
		out := map[string][]float64{}
		for _, s := range got {
			if s.Segment > 1 {
				out[s.LibraryItemID] = append(out[s.LibraryItemID], float64(s.SourceOffsetMs)/float64(minute))
			}
		}
		return out
	}
	got := cutsOf(interleaveBreaks(ch, slots))
	if len(got["on-air"]) != 0 || !slices.Equal(got["on-air-split"], []float64{20}) || !slices.Equal(got["later"], []float64{15, 30}) {
		t.Fatalf("cuts = %v, want on-air whole, on-air-split at its accepted 20 min, later newly split", got)
	}
	ch.NaturalBreaks = nil // mid-roll switched off while a split programme is on air
	got = cutsOf(interleaveBreaks(ch, slots))
	if !slices.Equal(got["on-air-split"], []float64{20}) || len(got["later"]) != 0 {
		t.Fatalf("cuts with mid-roll off = %v, want the on-air split kept and later airings whole", got)
	}
}
