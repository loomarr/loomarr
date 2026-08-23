package filler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// --- probe -------------------------------------------------------------------

type probeMemStore struct {
	upserts    []filler.StoreClip
	composites []string
}

func (m *probeMemStore) UpsertClip(_ context.Context, c filler.StoreClip) error {
	m.upserts = append(m.upserts, c)
	return nil
}

func (m *probeMemStore) SetClipComposite(_ context.Context, hash string, composite bool, _ time.Time) error {
	if composite {
		m.composites = append(m.composites, hash)
	}
	return nil
}

func proberReturning(p filler.Probed, err error) filler.Prober {
	return func(context.Context, string) (filler.Probed, error) { return p, err }
}

func probeClip() filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{Hash: "c1", Path: "a3/f9/c1.mp4", Name: "Toy ad"}}
}

// The hard gates, each with the MEASURED fact attached — "too short" is an assertion, "8.2s; the
// floor is 10.0s" is something an operator can argue with or act on.
func TestProbeStage_RefusesWhatIsNotAClip(t *testing.T) {
	floor := func() int64 { return 10_000 }
	for _, tc := range []struct {
		name   string
		probed filler.Probed
		reason filler.RejectReason
	}{
		{"no duration", filler.Probed{}, filler.ReasonUnprobeable},
		{"under the floor", filler.Probed{DurationMs: 8_200, Height: 480}, filler.ReasonTooShort},
		// ⚠ A video-only file plays as dead air in the middle of a break, which reads to a viewer
		// as the stream having dropped. That is why silence is a REJECT and not a warning.
		{"no audio stream", filler.Probed{DurationMs: 30_000, Height: 480, Silent: true}, filler.ReasonNoAudio},
		{"no video stream", filler.Probed{DurationMs: 30_000, NoVideo: true}, filler.ReasonNoVideo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &probeMemStore{}
			s := filler.NewProbeStage(proberReturning(tc.probed, nil), st, "/clips", floor, nil, nil)

			out, err := s.Run(context.Background(), probeClip())
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict != filler.VerdictReject || out.Reason != tc.reason {
				t.Fatalf("verdict = %v / %q, want reject / %q", out.Verdict, out.Reason, tc.reason)
			}
			if out.Detail == "" {
				t.Error("a reject with no measured detail is an assertion, not a finding")
			}
			if len(st.upserts) != 0 {
				t.Error("a refused clip was written back to the catalog")
			}
		})
	}
}

// A probe FAILURE is an error, not a reject: the runner retries it with backoff and only gives up
// after MaxAttempts. ffprobe failing on a file still being copied is the ordinary case, and
// tombstoning on the first attempt would refuse clips that are merely mid-download.
func TestProbeStage_AProbeFailureIsRetryableNotFatal(t *testing.T) {
	s := filler.NewProbeStage(proberReturning(filler.Probed{}, fmt.Errorf("moov atom not found")), nil, "/clips", nil, nil, nil)

	out, err := s.Run(context.Background(), probeClip())
	if err == nil {
		t.Fatal("a probe failure returned no error — the runner would treat it as a clean pass")
	}
	if out.Verdict == filler.VerdictReject {
		t.Error("a probe failure rejected the clip outright, skipping every retry")
	}
}

// A good probe writes duration + quality back, and only when something actually changed.
func TestProbeStage_PersistsWhatItMeasured(t *testing.T) {
	st := &probeMemStore{}
	s := filler.NewProbeStage(proberReturning(filler.Probed{DurationMs: 30_000, Height: 480}, nil), st, "/clips", nil, nil, nil)

	out, err := s.Run(context.Background(), probeClip())
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictContinue {
		t.Fatalf("verdict = %v, want continue", out.Verdict)
	}
	if len(st.upserts) != 1 || st.upserts[0].DurationMs != 30_000 || st.upserts[0].Quality != "480p" {
		t.Fatalf("probe did not persist its measurement: %+v", st.upserts)
	}
	// Unchanged on the second pass ⇒ no write. A re-probe that rewrites every row would bump
	// `updated_at` across the catalog on every pipeline pass for no new information.
	if _, err := s.Run(context.Background(), st.upserts[0]); err != nil {
		t.Fatal(err)
	}
	if len(st.upserts) != 1 {
		t.Errorf("an unchanged measurement was written again (%d writes)", len(st.upserts))
	}
}

// Probe must not decode an hour-long file merely to decide that it is not an advert. Duration
// quarantines it immediately; the split stage is the single owner of boundary detection.
func TestProbeStage_OverlongClipBecomesCompositeFromDuration(t *testing.T) {
	st := &probeMemStore{}
	maxAdvert := func() time.Duration { return 2 * time.Minute }
	s := filler.NewProbeStage(
		proberReturning(filler.Probed{DurationMs: int64((3 * time.Hour) / time.Millisecond), Height: 480}, nil),
		st, "/clips", nil, maxAdvert, nil,
	)

	out, err := s.Run(context.Background(), probeClip())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Clip.IsComposite || len(st.composites) != 1 || st.composites[0] != "c1" {
		t.Fatalf("overlong clip was not quarantined as a composite: out=%+v marks=%v", out, st.composites)
	}
}

// --- score -------------------------------------------------------------------

type scoreMemStore struct {
	confidence map[string]int
	filed      []string
}

func newScoreMemStore() *scoreMemStore {
	return &scoreMemStore{confidence: map[string]int{}}
}

func (m *scoreMemStore) SetClipConfidence(_ context.Context, path string, c int, _ time.Time) error {
	m.confidence[path] = c
	return nil
}

func (m *scoreMemStore) SetClipsHeld(_ context.Context, paths []string, held, _ bool, _ time.Time) (int, error) {
	if !held {
		m.filed = append(m.filed, paths...)
	}
	return len(paths), nil
}

func alwaysFileAt(min int) *filler.AutoFilePolicy {
	return &filler.AutoFilePolicy{
		Enabled:       func() bool { return true },
		MinConfidence: func() int { return min },
	}
}

func heldClipWith(mut func(*filler.Clip)) filler.StoreClip {
	c := filler.Clip{Hash: "c1", Path: "a3/f9/c1.mp4", Name: "Toy ad", Kind: filler.Commercial, Held: true}
	mut(&c)
	return filler.StoreClip{Clip: c}
}

// ⚠⚠ **THE guard in this phase.** `filler.reject.unidentified` defaults ON, so a clip that nothing
// has ever looked at must NOT be read as "we could not identify it". Without the `AITagged` check,
// the first pipeline pass on an install with no LLM — or any catalog imported before tagging
// existed — would tombstone every clip in it for a conclusion no tier ever reached.
//
// You cannot conclude that a signal is absent from a tier that never ran.
func TestScoreStage_NeverRejectsAClipNothingHasLookedAt(t *testing.T) {
	st := newScoreMemStore()
	rejectOn := func() bool { return true }
	s := filler.NewScoreStage(st, alwaysFileAt(85), rejectOn, nil)

	// Untagged AND untouched: AITagged false means the tagger never reached it.
	out, err := s.Run(context.Background(), heldClipWith(func(c *filler.Clip) { c.AITagged = false }))
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict == filler.VerdictReject {
		t.Fatal("a clip the tagger has never seen was REJECTED as unidentified — on a fresh install " +
			"or one with no LLM this tombstones the entire catalog")
	}
	if out.Verdict != filler.VerdictReview {
		t.Errorf("verdict = %v, want review", out.Verdict)
	}
}

// Once a tier HAS looked and found nothing, the reject is legitimate — and configurable.
func TestScoreStage_UnidentifiedIsConfigurable(t *testing.T) {
	looked := func(c *filler.Clip) { c.AITagged = true } // the tagger ran and grounded nothing

	for _, tc := range []struct {
		name   string
		reject bool
		want   filler.Verdict
	}{
		{"reject on", true, filler.VerdictReject},
		{"reject off", false, filler.VerdictReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newScoreMemStore()
			s := filler.NewScoreStage(st, alwaysFileAt(85), func() bool { return tc.reject }, nil)

			out, err := s.Run(context.Background(), heldClipWith(looked))
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict != tc.want {
				t.Errorf("verdict = %v, want %v", out.Verdict, tc.want)
			}
			if tc.reject && out.Reason != filler.ReasonUnidentified {
				t.Errorf("reason = %q, want unidentified", out.Reason)
			}
		})
	}
}

// ANY grounded signal is enough to be "identified" — including one that is not a match tag.
//
// ⚠ A wordless station ident grounded only by its on-screen text is exactly the clip §10 calls
// some of the best filler there is, so the test for "did we learn anything" has to be wider than
// "does it have era + audience + category".
func TestScoreStage_AnyGroundedSignalCountsAsIdentified(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*filler.Clip)
	}{
		{"era", func(c *filler.Clip) { c.Era = 1987 }},
		{"audience", func(c *filler.Clip) { c.Audience = filler.Kids }},
		{"a taxonomy tag", func(c *filler.Clip) { c.Tags = []string{"toys"} }},
		{"an advertiser", func(c *filler.Clip) { c.Brand = "Kellogg's" }},
		{"on-screen text", func(c *filler.Clip) { c.VisibleText = "COCA-COLA" }},
		{"speech", func(c *filler.Clip) { c.Transcript = "the real thing" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newScoreMemStore()
			s := filler.NewScoreStage(st, alwaysFileAt(85), func() bool { return true }, nil)

			out, err := s.Run(context.Background(), heldClipWith(func(c *filler.Clip) {
				c.AITagged = true
				tc.mut(c)
			}))
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict == filler.VerdictReject {
				t.Errorf("a clip grounded by %s was rejected as unidentified", tc.name)
			}
		})
	}
}

// ⚠ A transcript of "[no speech]" is the CHECKED-AND-WORDLESS sentinel, not content. Counting it
// as grounding would make every silent clip look identified and defeat the check.
func TestScoreStage_TheNoSpeechSentinelIsNotGrounding(t *testing.T) {
	st := newScoreMemStore()
	s := filler.NewScoreStage(st, alwaysFileAt(85), func() bool { return true }, nil)

	out, err := s.Run(context.Background(), heldClipWith(func(c *filler.Clip) {
		c.AITagged = true
		c.Transcript = filler.TranscriptNone
	}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != filler.VerdictReject {
		t.Errorf("verdict = %v — the wordless sentinel was counted as if it were speech", out.Verdict)
	}
}

// The score is persisted for every clip, and a fully-grounded held clip files itself.
func TestScoreStage_ScoresAndFiles(t *testing.T) {
	st := newScoreMemStore()
	s := filler.NewScoreStage(st, alwaysFileAt(85), func() bool { return false }, nil)

	clip := heldClipWith(func(c *filler.Clip) {
		c.AITagged, c.Era, c.Audience, c.Category = true, 1987, filler.Kids, "toys"
	})
	if _, err := s.Run(context.Background(), clip); err != nil {
		t.Fatal(err)
	}
	if st.confidence[clip.Path] != 100 {
		t.Errorf("confidence = %d, want 100 (everything grounded)", st.confidence[clip.Path])
	}
	if len(st.filed) != 1 || st.filed[0] != clip.Path {
		t.Errorf("filed = %v, want the clip filed unattended", st.filed)
	}
}

// ⚠ THE safety property, restated at the pipeline's own boundary: an era the model could not
// ground caps the score strictly below every settable threshold, so it can never file itself —
// whatever the operator sets.
func TestScoreStage_AnUngroundedEraCanNeverFileItself(t *testing.T) {
	for _, threshold := range []int{50, 85, 95} {
		st := newScoreMemStore()
		s := filler.NewScoreStage(st, alwaysFileAt(threshold), func() bool { return false }, nil)

		clip := heldClipWith(func(c *filler.Clip) {
			c.AITagged, c.Audience, c.Category = true, filler.Kids, "toys"
			c.SuggestedEra = 1985 // proposed, never grounded
		})
		out, err := s.Run(context.Background(), clip)
		if err != nil {
			t.Fatal(err)
		}
		if len(st.filed) != 0 {
			t.Errorf("threshold %d: an UNGROUNDED era filed itself — a fabricated tag reached a "+
				"live channel with nobody looking", threshold)
		}
		if out.Verdict != filler.VerdictReview {
			t.Errorf("threshold %d: verdict = %v, want review", threshold, out.Verdict)
		}
	}
}
