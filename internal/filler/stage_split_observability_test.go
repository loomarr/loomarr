package filler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// capture returns a logger writing to a buffer, and the buffer.
func capture() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// ⚠ **THE case this exists for, and it is not an error on any path.** The model answers every
// call, its JSON parses, `Looked` increments for every segment — and nothing is learned, because
// the category it returned is not readable on the frame. Measured 2026-08-13 with `llava:7b`,
// which echoed the prompt's own placeholder list back as its answer: 37 calls, 37 successes, one
// usable field, and the reel refused as untagged with no clue why.
func TestGround_WarnsWhenItReadEveryCutAndLearnedNothing(t *testing.T) {
	log, buf := capture()
	// Valid JSON, plausible answer, and grounded on NOTHING: "toys" does not appear in the text
	// the model says it read, so groundVisionTags correctly drops it.
	model := &fixedVision{answer: `{"category":"toys","visibleText":"CHANNEL 5 NEWS AT TEN"}`}
	s := NewSplitStage(nil, nil).WithLogger(log).WithSegmentVision(&SegmentVision{
		Tools:    &spanTools{frames: [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}},
		Provider: model, Taxa: seedTaxa{}, ClipDir: "/clips",
		Budget: func() int { return 10 },
	})

	segs := twoSegments()
	pass := s.ground(context.Background(), StoreClip{Clip: Clip{Path: "reel.mp4"}}, segs)

	if pass.Looked != 2 {
		t.Fatalf("looked = %d, want 2 — the premise is that every call SUCCEEDED", pass.Looked)
	}
	out := buf.String()
	if !strings.Contains(out, "learned nothing") {
		t.Errorf("a pass that read every cut and grounded none of them logged:\n%s", out)
	}
	if !strings.Contains(out, "learned=0") {
		t.Errorf("the log does not carry the learned count that distinguishes this case:\n%s", out)
	}
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("reported below WARN, so it will not be noticed:\n%s", out)
	}
}

// The counterpart: a pass that DID learn something must not cry wolf, or the warning above stops
// meaning anything.
func TestGround_DoesNotWarnWhenItLearnedSomething(t *testing.T) {
	log, buf := capture()
	model := &fixedVision{answer: `{"category":"toys","visibleText":"TOYS R US MEGA SALE"}`}
	s := NewSplitStage(nil, nil).WithLogger(log).WithSegmentVision(&SegmentVision{
		Tools:    &spanTools{frames: [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}},
		Provider: model, Taxa: seedTaxa{}, ClipDir: "/clips",
		Budget: func() int { return 10 },
	})

	s.ground(context.Background(), StoreClip{Clip: Clip{Path: "reel.mp4"}}, twoSegments())

	if out := buf.String(); strings.Contains(out, "learned nothing") {
		t.Errorf("warned about a pass that grounded both segments:\n%s", out)
	}
}

// Each way a pass can decline to start must be DISTINGUISHABLE. These were four identical silent
// early returns.
func TestGround_SaysWhyItDidNotRun(t *testing.T) {
	frames := &spanTools{frames: [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}}
	for _, tc := range []struct {
		name, want string
		stage      func(*slog.Logger) *SplitStage
	}{
		{"vision not wired", "not wired", func(l *slog.Logger) *SplitStage {
			return NewSplitStage(nil, nil).WithLogger(l)
		}},
		{"zero budget", "budget is zero", func(l *slog.Logger) *SplitStage {
			return NewSplitStage(nil, nil).WithLogger(l).WithSegmentVision(&SegmentVision{
				Tools: frames, Provider: &fixedVision{answer: "{}"}, Taxa: seedTaxa{},
				Budget: func() int { return 0 },
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, buf := capture()
			tc.stage(log).ground(context.Background(), StoreClip{Clip: Clip{Path: "reel.mp4"}}, twoSegments())
			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("log does not say %q:\n%s", tc.want, buf.String())
			}
		})
	}
}

// A provider outage is reported with its error, not swallowed by the `break`.
func TestGround_ReportsAProviderRefusal(t *testing.T) {
	log, buf := capture()
	s := NewSplitStage(nil, nil).WithLogger(log).WithSegmentVision(&SegmentVision{
		Tools:    &spanTools{frames: [][]byte{[]byte("\xff\xd8jpeg\xff\xd9")}},
		Provider: &fixedVision{err: context.DeadlineExceeded}, Taxa: seedTaxa{},
		Budget: func() int { return 10 },
	})

	s.ground(context.Background(), StoreClip{Clip: Clip{Path: "reel.mp4"}}, twoSegments())

	out := buf.String()
	if !strings.Contains(out, "provider refused") {
		t.Errorf("a provider outage was not reported:\n%s", out)
	}
	if !strings.Contains(out, "deadline exceeded") {
		t.Errorf("the provider's error is missing, so the cause is still unknown:\n%s", out)
	}
}

// ⚠ The regression: `Verdict` returned `Hold[0].HoldReason`, so a single tagged-but-doubtful
// segment sorting first spoke for 36 untagged ones — and sent the operator to a threshold that
// was never consulted.
func TestVerdict_ReportsTheMajorityReasonNotTheFirst(t *testing.T) {
	hold := []SplitSegment{{HoldReason: string(RejectBoundaryUncertain)}}
	for range 36 {
		hold = append(hold, SplitSegment{HoldReason: string(RejectUntagged)})
	}
	part := SplitPartition{Hold: hold}

	if got := part.Verdict(); got != RejectUntagged {
		t.Errorf("Verdict = %q, want %q — 36 of 37 segments give that reason", got, RejectUntagged)
	}
	reason, shared, total := part.HoldSummary()
	if reason != RejectUntagged || shared != 36 || total != 37 {
		t.Errorf("HoldSummary = (%q, %d, %d), want (%q, 36, 37)", reason, shared, total, RejectUntagged)
	}
	// The note must show the SHARE, so "fix the tagging" is visibly not the whole story.
	note := holdNote(part)
	if !strings.Contains(note, "36 of 37 cuts") {
		t.Errorf("note = %q, want it to state the share", note)
	}
	if !strings.Contains(note, string(RejectUntagged)) {
		t.Errorf("note = %q, want the majority reason", note)
	}
}

// When every held segment agrees there is no share to state — "37 cuts: …" reads better than
// "37 of 37".
func TestHoldNote_OmitsTheShareWhenUnanimous(t *testing.T) {
	var hold []SplitSegment
	for range 3 {
		hold = append(hold, SplitSegment{HoldReason: string(RejectUntagged)})
	}
	if note := holdNote(SplitPartition{Hold: hold}); note != "3 cuts: "+string(RejectUntagged) {
		t.Errorf("note = %q", note)
	}
}
