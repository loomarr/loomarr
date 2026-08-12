package filler

import (
	"sort"
	"strings"
	"time"
)

// Compilation splitting (§10, V34) — domain types and the PURE segment logic.
// The exec boundary (ffmpeg/ffprobe/whisper-cli) lives in mediatools.go behind
// MediaTools, and the LLM boundary rescue in splitrescue.go; everything here is
// deterministic and unit-tested without touching a binary.
//
// The pipeline (plan §6.4, designed from measurement on six real compilations):
// triage (chapters) → coarse split (blackdetect + silencedetect) → transcript
// rescue for over-long segments → dHash dedup → REVIEW (not optional —
// detection quality is a property of the source, measured 69–100%, so nothing
// enters the catalog unconfirmed).
//
// ⚠ "classify each segment" used to sit before dedup and is GONE (§10 V51g). It
// was one LLM turn per segment — 51 × 7.4s ≈ 377s on a 16m47s reel, against a
// 120s pass — so the rung could never finish and restarted every two minutes.
// **Split cuts; it does not describe.** Each segment is spawned as its own clip
// and reaches `tag` on its own ladder, after `transcribe`, with a real
// transcript instead of the string "… part 7".

const (
	// MinSegmentMs drops slivers: black/silence detection on real compilations
	// produces sub-second "segments" between an advert's own fade-out and fade-in.
	MinSegmentMs int64 = 3000
	// OverlongSegmentMs is the "far longer than a plausible advert" threshold that
	// sends a segment to transcript rescue. Breaks are 30–90s; past two minutes
	// either the A/V pass missed boundaries (the measured 149s three-advert block)
	// or it genuinely is one long advert (the measured 121s infomercial) — only the
	// transcript can say which.
	OverlongSegmentMs int64 = 120_000
)

// SplitSegment is one proposed clip inside a compilation (§10 V34). The detector
// authors it; the reviewer edits it; only confirm writes it to the catalog.
type SplitSegment struct {
	Index   int   `json:"index"`
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
	// Name is the proposed clip name (from the LLM's product label, or
	// "<compilation> part N"). It becomes the clip's filename on confirm.
	Name string `json:"name"`
	// Era/Audience/Tags come from the SAME Classify the tag job uses, over the
	// segment's transcript. Era is grounded (year in the text) or zero — an
	// ungrounded guess is carried ONLY as SuggestedEra (§10 era rule).
	Era          int      `json:"era,omitempty"`
	SuggestedEra int      `json:"suggestedEra,omitempty"`
	Audience     Audience `json:"audience,omitempty"`
	// Tags is the grounded taxonomy leaf set (§10 V45a); Category is its DERIVED product-leaf shadow,
	// carried so the review and the confirmed clip both have the cheap read-path value. Both come from
	// the same Classify the tag job uses.
	Tags     []string `json:"tags,omitempty"`
	Category string   `json:"category,omitempty"`
	// DupOf is the path of an existing catalog clip this segment duplicates
	// (dHash, measured 25× separation). ⚠ A FLAG, never a silent drop — the
	// reviewer sees "already in the catalog" and decides.
	DupOf string `json:"dupOf,omitempty"`
	// Unsplittable means the segment is over-long AND the rescue could not run or
	// found nothing (no whisper, whisper/LLM failure). The review must say so —
	// the alternative is guessing, which is exactly what the era rule forbids in
	// tag form.
	Unsplittable bool `json:"unsplittable,omitempty"`
	// Transcript is the segment's whisper text, when the rescue ran. Carried on
	// the proposal so the reviewer can SEE why a boundary was proposed; not
	// persisted to the catalog on confirm.
	Transcript string `json:"transcript,omitempty"`
}

// SplitProposal is the persisted, operator-reviewable result of detecting cuts
// in a compilation clip (§10 V34). It is NOT a clip: nothing here is visible to
// pod matching, and the only way segments become clips is Confirm.
type SplitProposal struct {
	ID string `json:"id"`
	// ClipHash is the compilation's IDENTITY — its content hash (§10 V38c), the value
	// `GetClip` is keyed on.
	//
	// ⚠ **This field was `ClipPath` and held `clip.Path`, and that mismatch broke Confirm
	// outright.** `Propose` wrote the shard path (`a3/f9/<hash>.mp4`); `Confirm` handed the same
	// string to a HASH-keyed `GetClip`, which never matched — so every confirm returned
	// "compilation … no longer in the catalog" for a clip sitting in the catalog. An operator
	// could open a 41-segment reel, edit it, and never commit it. It survived because the
	// splitter's test store keys its map on `Path`, so the fixture answered a question
	// production's store does not (the same collapsed-key class `conformance_filler.go` records).
	//
	// ⚠ The FILE PATH is DERIVED from this, never stored beside it: `ClipPath(dropDir, hash, ext)`
	// is the containment boundary, and one identity with a derived location is what stops the two
	// disagreeing again.
	ClipHash  string         `json:"clipHash"`
	CreatedAt time.Time      `json:"createdAt"`
	Segments  []SplitSegment `json:"segments"`
}

// segmentsFromBoundaries builds segments by cutting the timeline at every
// detected gap (a black or silence interval), dropping slivers under
// MinSegmentMs. `gaps` may arrive unsorted and overlapping (blackdetect and
// silencedetect fire on the same boundary); they are merged first so a fade that
// is BOTH black and silent produces one cut, not two slivers.
//
// Boundaries are taken at the gap's MIDPOINT: blackdetect reports the whole
// fade interval, and cutting at its start would shave the advert's tail frame.
func segmentsFromBoundaries(durationMs int64, gaps []Interval) []SplitSegment {
	cuts := boundaryCuts(gaps, durationMs)
	var out []SplitSegment
	start := int64(0)
	for _, cut := range cuts {
		if seg, ok := makeSegment(len(out), start, cut); ok {
			out = append(out, seg)
		}
		start = cut
	}
	if seg, ok := makeSegment(len(out), start, durationMs); ok {
		out = append(out, seg)
	}
	return out
}

// boundaryCuts merges overlapping/adjacent gaps and returns their midpoints as
// ordered cut positions, clamped inside (0, durationMs).
func boundaryCuts(gaps []Interval, durationMs int64) []int64 {
	if len(gaps) == 0 {
		return nil
	}
	sorted := append([]Interval(nil), gaps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	var cuts []int64
	cur := sorted[0]
	for _, g := range sorted[1:] {
		if g.StartMs <= cur.EndMs {
			cur.EndMs = max(cur.EndMs, g.EndMs)
			continue
		}
		cuts = append(cuts, midpoint(cur, durationMs))
		cur = g
	}
	cuts = append(cuts, midpoint(cur, durationMs))
	// Midpoints of adjacent gaps can collide after clamping; dedupe + order.
	sort.Slice(cuts, func(i, j int) bool { return cuts[i] < cuts[j] })
	out := cuts[:0]
	for i, c := range cuts {
		if i == 0 || c != cuts[i-1] {
			out = append(out, c)
		}
	}
	return out
}

func midpoint(g Interval, durationMs int64) int64 {
	m := (g.StartMs + g.EndMs) / 2
	if m < 1 {
		m = 1
	}
	if m > durationMs-1 {
		m = durationMs - 1
	}
	return m
}

func makeSegment(index int, start, end int64) (SplitSegment, bool) {
	if end-start < MinSegmentMs {
		return SplitSegment{}, false
	}
	return SplitSegment{Index: index, StartMs: start, EndMs: end}, true
}

// segmentsFromChapters turns embedded chapters into segments (the triage step —
// a pre-chaptered source splits for free). Chapters that abut produce no gaps;
// slivers are dropped as usual. Chapters carry their own titles, which beat any
// generated name.
func segmentsFromChapters(chapters []Chapter) []SplitSegment {
	var out []SplitSegment
	for _, ch := range chapters {
		seg, ok := makeSegment(len(out), ch.StartMs, ch.EndMs)
		if !ok {
			continue
		}
		seg.Name = strings.TrimSpace(ch.Title)
		out = append(out, seg)
	}
	return out
}

// overlong reports whether a segment is far longer than a plausible advert and
// therefore needs the transcript rescue (§10 — boundaries that exist only in
// language).
func (s SplitSegment) overlong() bool { return s.EndMs-s.StartMs > OverlongSegmentMs }

// TranscriptText renders a transcript as timestamped lines for the LLM prompt
// ("[01:23] …"). mm:ss is what the model reads most reliably, and it is what the
// rescue asks it to answer in.
func TranscriptText(segs []TranscriptSegment) string {
	var b strings.Builder
	for _, s := range segs {
		text := strings.Join(strings.Fields(s.Text), " ")
		if text == "" {
			continue
		}
		b.WriteString("[" + formatMS(s.StartMs) + "] " + text + "\n")
	}
	return b.String()
}

// formatMS renders milliseconds as mm:ss for prompts and display.
func formatMS(ms int64) string {
	s := ms / 1000
	return pad2(s/60) + ":" + pad2(s%60)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
