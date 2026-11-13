package filler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// --- probe -------------------------------------------------------------------

type probeMemStore struct {
	upserts    []clipstore.Clip
	composites []string
}

func (m *probeMemStore) UpsertClip(_ context.Context, c clipstore.Clip) error {
	m.upserts = append(m.upserts, c)
	return nil
}

func (m *probeMemStore) SetClipComposite(_ context.Context, hash string, composite bool, _ time.Time) error {
	if composite {
		m.composites = append(m.composites, hash)
	}
	return nil
}

func proberReturning(p tools.Probed, err error) tools.Prober {
	return func(context.Context, string) (tools.Probed, error) { return p, err }
}

func probeClip() clipstore.Clip {
	return clipstore.Clip{Clip: clips.Clip{Hash: "c1", Path: "a3/f9/c1.mp4", Name: "Toy ad"}}
}

// The hard gates, each with the MEASURED fact attached — "too short" is an assertion, "8.2s; the
// floor is 10.0s" is something an operator can argue with or act on.
func TestProbeStage_RefusesWhatIsNotAClip(t *testing.T) {
	floor := func() int64 { return 10_000 }
	for _, tc := range []struct {
		name   string
		probed tools.Probed
		reason pipelinestate.RejectReason
	}{
		{"no duration", tools.Probed{}, pipelinestate.ReasonUnprobeable},
		{"under the floor", tools.Probed{DurationMs: 8_200, Height: 480}, pipelinestate.ReasonTooShort},
		// ⚠ A video-only file plays as dead air in the middle of a break, which reads to a viewer
		// as the stream having dropped. That is why silence is a REJECT and not a warning.
		{"no audio stream", tools.Probed{DurationMs: 30_000, Height: 480, Silent: true}, pipelinestate.ReasonNoAudio},
		{"no video stream", tools.Probed{DurationMs: 30_000, NoVideo: true}, pipelinestate.ReasonNoVideo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &probeMemStore{}
			s := filler.NewProbeStage(proberReturning(tc.probed, nil), st, "/clips", floor, nil, nil)

			out, err := s.Run(context.Background(), probeClip())
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict != pipelinestate.VerdictReject || out.Reason != tc.reason {
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
	s := filler.NewProbeStage(proberReturning(tools.Probed{}, fmt.Errorf("moov atom not found")), nil, "/clips", nil, nil, nil)

	out, err := s.Run(context.Background(), probeClip())
	if err == nil {
		t.Fatal("a probe failure returned no error — the runner would treat it as a clean pass")
	}
	if out.Verdict == pipelinestate.VerdictReject {
		t.Error("a probe failure rejected the clip outright, skipping every retry")
	}
}

// A good probe writes duration + quality back, and only when something actually changed.
func TestProbeStage_PersistsWhatItMeasured(t *testing.T) {
	st := &probeMemStore{}
	s := filler.NewProbeStage(proberReturning(tools.Probed{DurationMs: 30_000, Height: 480}, nil), st, "/clips", nil, nil, nil)

	out, err := s.Run(context.Background(), probeClip())
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != pipelinestate.VerdictContinue {
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
		proberReturning(tools.Probed{DurationMs: int64((3 * time.Hour) / time.Millisecond), Height: 480}, nil),
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

// --- finish ------------------------------------------------------------------

// The finish rung is the ready commit's anchor: it applies to every clip and always continues,
// whatever the optional classifiers did or did not ground. Enrollment is the runtime authority, so
// a clip nothing has looked at is never rejected here.
func TestFinishStage_AlwaysAppliesAndContinues(t *testing.T) {
	s := filler.NewFinishStage()
	if s.ID() != pipelinestate.StageFinish || pipelinestate.StageOrder[len(pipelinestate.StageOrder)-1] != pipelinestate.StageFinish {
		t.Fatalf("finish is not the terminal rung: id=%s order=%v", s.ID(), pipelinestate.StageOrder)
	}
	for _, clip := range []clips.Clip{
		{Hash: "c1", Path: "a3/f9/c1.mp4", Role: clips.Commercial, Held: true},
		{Hash: "c2", Path: "b4/e8/c2.mp4", Role: clips.Unclassified, Held: true, Transcript: clips.TranscriptNone},
	} {
		if ok, why := s.Applies(context.Background(), clipstore.Clip{Clip: clip}); !ok {
			t.Fatalf("finish skipped %s (%s) — the ready commit would never validate", clip.Hash, why)
		}
		out, err := s.Run(context.Background(), clipstore.Clip{Clip: clip})
		if err != nil || out.Verdict != pipelinestate.VerdictContinue || out.Reason != "" {
			t.Fatalf("finish on %s = %+v, %v; want a plain continue", clip.Hash, out, err)
		}
	}
}
