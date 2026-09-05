package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
)

type appliedAdmissionResolverFunc func(context.Context, string) (string, error)

func (f appliedAdmissionResolverFunc) ResolveAppliedAdmissionMedia(ctx context.Context, hash string) (string, error) {
	return f(ctx, hash)
}

type appliedAdmissionCommitter struct {
	actions []fillerdecision.Action
	err     error
}

func (c *appliedAdmissionCommitter) CommitAppliedFillerDecisionAction(_ context.Context, action fillerdecision.Action) error {
	if c.err != nil {
		return c.err
	}
	c.actions = append(c.actions, action)
	return nil
}

func TestAppliedAdmissionReplaysCurrentReleaseBeforePublication(t *testing.T) {
	module, record, action, committer, _, _ := appliedAdmissionFixture(t)
	if err := module.ActOnAppliedFillerDecision(t.Context(), record, action); err != nil {
		t.Fatal(err)
	}
	if len(committer.actions) != 1 || committer.actions[0] != action {
		t.Fatalf("committed actions = %+v, want the verified admit", committer.actions)
	}
}

func TestAppliedAdmissionFailsClosedBeforeTheCatalogTransaction(t *testing.T) {
	t.Run("release authority drift", func(t *testing.T) {
		module, record, action, committer, _, _ := appliedAdmissionFixture(t)
		record.ReleaseAuthoritySHA256 = strings.Repeat("f", 64)
		if err := module.ActOnAppliedFillerDecision(t.Context(), record, action); !errors.Is(err, fillerdecision.ErrAppliedUnavailable) {
			t.Fatalf("authority drift = %v, want unavailable", err)
		}
		if len(committer.actions) != 0 {
			t.Fatal("authority drift reached the catalog transaction")
		}
	})
	t.Run("current playback drift", func(t *testing.T) {
		module, record, action, committer, summary, _ := appliedAdmissionFixture(t)
		summary.inspect = func(context.Context, string, string, int64, string) (segmentScreeningArtifactObservation, bool, error) {
			return segmentScreeningArtifactObservation{State: "changed"}, false, nil
		}
		if err := module.ActOnAppliedFillerDecision(t.Context(), record, action); !errors.Is(err, fillerdecision.ErrAppliedUnavailable) {
			t.Fatalf("playback drift = %v, want unavailable", err)
		}
		if len(committer.actions) != 0 {
			t.Fatal("playback drift reached the catalog transaction")
		}
	})
	t.Run("current rights withdrawn", func(t *testing.T) {
		module, record, action, committer, _, certification := appliedAdmissionFixture(t)
		certification.rights = currentFillerRightsAuthorityFunc(func(_ context.Context, request FillerRightsUseRequest) (FillerRightsUseDecision, bool, error) {
			withdrawn := request.RequestedAt.Add(-time.Minute)
			decision, err := NewFillerRightsUseDecision(request, FillerRightsProhibited, FillerRightsWithdrawalActive, screeningDigest("7"), nil, &withdrawn)
			return decision, true, err
		})
		if err := module.ActOnAppliedFillerDecision(t.Context(), record, action); !errors.Is(err, fillerdecision.ErrAppliedUnavailable) {
			t.Fatalf("withdrawn rights = %v, want unavailable", err)
		}
		if len(committer.actions) != 0 {
			t.Fatal("withdrawn rights reached the catalog transaction")
		}
	})
}

func TestAppliedAdmissionRejectDoesNotRequirePublicationEvidence(t *testing.T) {
	module, record, action, committer, _, _ := appliedAdmissionFixture(t)
	module.resolver = appliedAdmissionResolverFunc(func(context.Context, string) (string, error) {
		return "", errors.New("playback deliberately unavailable")
	})
	action.Kind = fillerdecision.ActionReject
	if err := module.ActOnAppliedFillerDecision(t.Context(), record, action); err != nil {
		t.Fatal(err)
	}
	if len(committer.actions) != 1 || committer.actions[0].Kind != fillerdecision.ActionReject {
		t.Fatalf("committed actions = %+v, want reject", committer.actions)
	}
}

func appliedAdmissionFixture(t *testing.T) (
	*AppliedAdmission,
	fillerdecision.Record,
	fillerdecision.Action,
	*appliedAdmissionCommitter,
	*SegmentScreeningSummaryService,
	*SegmentScreeningCertification,
) {
	t.Helper()
	tags := screeningChildTagsFixture(t)
	subject, err := NewSegmentScreeningSubject(tags.MediaAssets.Playback.Asset.ClipHash, tags)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, certification, repository, _ := screeningCertificationFixture(t, subject, true)
	reference, err := NewSegmentScreeningReference(subject, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	tags.SegmentScreening = &reference
	mediaPath := filepath.Join(t.TempDir(), "child.mp4")
	if err := os.WriteFile(mediaPath, []byte("current screened child"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarTags(mediaPath, tags, false); err != nil {
		t.Fatal(err)
	}
	resolver := appliedAdmissionResolverFunc(func(_ context.Context, hash string) (string, error) {
		if hash != subject.CatalogHash {
			return "", errors.New("unexpected catalog hash")
		}
		return mediaPath, nil
	})
	committer := &appliedAdmissionCommitter{}
	module, err := NewAppliedAdmission(resolver, repository, certification, committer)
	if err != nil {
		t.Fatal(err)
	}
	summary := module.summary
	summary.inspect = func(_ context.Context, path, expectedSHA256 string, expectedBytes int64, expectedClipHash string) (segmentScreeningArtifactObservation, bool, error) {
		if path != mediaPath || expectedSHA256 != subject.PlaybackSHA256 || expectedBytes != subject.PlaybackBytes || expectedClipHash != subject.CatalogHash {
			return segmentScreeningArtifactObservation{}, false, errors.New("unexpected playback identity")
		}
		return segmentScreeningArtifactObservation{
			State: "observed", SHA256: expectedSHA256, Bytes: expectedBytes, ClipHash: expectedClipHash,
		}, true, nil
	}
	record := fillerdecision.Record{
		ID: "applied-decision", ClipHash: subject.CatalogHash,
		EvidenceHash: "admission-evidence", EvidenceVersion: "applied-evidence-v1",
		SchemaVersion: filleradmission.SchemaVersion, PolicyVersion: "policy-v1", TaxonomyVersion: "taxonomy-v1",
		ApplicationMode:         fillerdecision.ApplicationModeApplied,
		ScreeningEvidenceSHA256: aggregate.SHA256, ReleaseAuthoritySHA256: certification.AuthoritySHA256(),
		Result: filleradmission.Result{Decision: &filleradmission.Decision{
			Verdict: filleradmission.VerdictReview, ReasonCodes: []filleradmission.ReasonCode{filleradmission.ReasonMissingCommercialIdentity},
			ReviewQuestion: "What product is this clip advertising?",
		}},
		CreatedAt: time.Date(2026, time.September, 13, 1, 0, 0, 0, time.UTC),
	}
	action := fillerdecision.Action{
		ID: "applied-action", DecisionID: record.ID, Kind: fillerdecision.ActionAdmit,
		ActorID: "admin-1", Answer: "The closing card identifies soda.", CreatedAt: record.CreatedAt.Add(time.Minute),
	}
	return module, record, action, committer, summary, certification
}
