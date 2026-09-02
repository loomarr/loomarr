package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
)

// Transcript rescue (§10 V34): an over-long segment means boundaries the A/V
// pass could not see, and some boundaries exist ONLY in language — measured: a
// 149s block defeated every detector while holding three complete adverts with
// no black frame and no silence between them (plan §6.4). The transcript is the
// only signal that sees that class, and the LLM turns it into cut points.
//
// ⚠ TWO measured failure modes shape this code, and both are pinned by tests:
//
//  1. **The LLM invents boundaries inside a single long advert.** A 121s
//     infomercial for ONE product came back split at suspiciously round
//     30/61/92s marks. The prompt's single-advert instruction is the fix — and
//     it is why a test must cover the single-advert case, or this ships a
//     splitter that manufactures clips.
//  2. **A poor transcript produces confident nonsense** (whisper tiny dropped
//     audio → the model returned phone-number digits as products). Model size
//     is a correctness property (§14); here the defence is validation: any
//     proposed span that clamps to nothing valid means NO CUTS, never a guess.

// rescuedSpan is one advert the LLM found inside an over-long segment, with its
// product label (which becomes the segment's proposed name).
type rescuedSpan struct {
	Interval
	Product string
}

// rescueSystemPrompt asks for advert boundaries in ONE transcript. The
// single-advert rule is the load-bearing line (above).
const rescueSystemPrompt = `You find the boundaries between TV adverts inside one transcript of a single continuous video segment.
The transcript lines are timestamped [mm:ss]. Adverts usually run back to back with no pause.
Return ONLY this JSON, no prose:
{"adverts":[{"start":"mm:ss","end":"mm:ss","product":"short product or brand name"}]}
Rules:
- If the WHOLE transcript is ONE advert, return exactly one entry spanning it. This is common — a long segment is often a single infomercial, not several adverts.
- Only place a boundary where one product's pitch ENDS and a different product's pitch BEGINS. Never split one product's pitch at a convenient-looking round timestamp.
- Name only products the transcript actually mentions. If you cannot tell what a span advertises, use "unknown".
- Cover the transcript from its first line to its last — no gaps, no overlaps.`

type rescueOutput struct {
	Adverts []struct {
		Start   string `json:"start"`
		End     string `json:"end"`
		Product string `json:"product"`
	} `json:"adverts"`
}

// findAdBreaks asks the LLM for the advert spans inside one over-long segment's
// transcript and VALIDATES them (above). segDurMs is the segment's length;
// transcript offsets are relative to its start. Returns the spans — exactly one
// spanning (nearly) the whole segment when the transcript is a single advert,
// which the caller treats as "keep whole", NOT as unsplittable.
func findAdBreaks(ctx context.Context, provider llm.Provider, transcript []TranscriptSegment, segDurMs int64, floor segmentFloor) ([]rescuedSpan, error) {
	text := TranscriptText(transcript)
	if text == "" {
		return nil, fmt.Errorf("empty transcript")
	}
	resp, err := provider.Chat(ctx, []llm.Message{
		{Role: llm.System, Content: rescueSystemPrompt},
		{Role: llm.User, Content: "Transcript:\n" + text + "\nFind the advert boundaries."},
	}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		return nil, fmt.Errorf("rescue llm: %w", err)
	}
	var out rescueOutput
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return nil, fmt.Errorf("rescue output not JSON: %w", err)
	}
	return validateRescueSpans(out, segDurMs, floor)
}

// validateRescueSpans clamps the model's answer to what the segment can
// contain: parseable mm:ss inside [0,segDur), start<end, ordered, no overlaps,
// no slivers. Anything else is dropped; NOTHING VALID AT ALL is an error, so
// the caller marks the segment Unsplittable instead of inventing a cut.
func validateRescueSpans(out rescueOutput, segDurMs int64, floor segmentFloor) ([]rescuedSpan, error) {
	var spans []rescuedSpan
	for _, a := range out.Adverts {
		start, err1 := parseMMS(a.Start)
		end, err2 := parseMMS(a.End)
		if err1 != nil || err2 != nil {
			continue
		}
		if start < 0 {
			start = 0
		}
		if end > segDurMs {
			end = segDurMs
		}
		if !floor.admits(start, end) {
			continue
		}
		product := strings.Join(strings.Fields(a.Product), " ")
		if product == "" {
			product = "unknown"
		}
		spans = append(spans, rescuedSpan{Interval: Interval{StartMs: start, EndMs: end}, Product: product})
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("no valid advert spans in model output")
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].StartMs < spans[j].StartMs })
	// Overlaps: keep the FIRST span, truncating later starts at the previous end —
	// the model's coverage rule makes slight overlaps common, and two clips
	// sharing seconds is worse than a slightly late cut.
	kept := spans[:1]
	for _, s := range spans[1:] {
		prev := kept[len(kept)-1]
		if s.StartMs < prev.EndMs {
			s.StartMs = prev.EndMs
		}
		// ⚠ The SECOND floor check, and it is not redundant with the one above: truncating an
		// overlap shortens a span that already passed, so a span admitted at its proposed length
		// can fall under the floor here.
		if !floor.admits(s.StartMs, s.EndMs) {
			continue
		}
		kept = append(kept, s)
	}
	return kept, nil
}

// parseMMS parses "mm:ss" (also tolerates "h:mm:ss" and bare seconds) to
// milliseconds — the format the prompt asks for, defensively.
func parseMMS(s string) (int64, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	var secs float64
	mult := 1.0
	for _, part := range slices.Backward(parts) {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return 0, fmt.Errorf("bad timestamp %q", s)
		}
		secs += v * mult
		mult *= 60
	}
	if len(parts) == 0 || len(parts) > 3 {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	return int64(secs * 1000), nil
}
