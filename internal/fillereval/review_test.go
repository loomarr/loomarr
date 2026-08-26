package fillereval

import (
	"strings"
	"testing"
	"time"
)

func TestLockReviewedManifestPreservesIndependentSubmissions(t *testing.T) {
	manifest, _ := passingCorpus(500)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	clearDraftLabels(&manifest)
	locked, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt)
	if len(failures) > 0 {
		t.Fatalf("lock failed: %v", failures)
	}
	if len(locked.Cases[0].LabelReviews) != 2 || locked.Cases[0].LabelReviews[0].SubmissionSHA256 == "" {
		t.Fatalf("reviews = %+v", locked.Cases[0].LabelReviews)
	}
}

func TestLockReviewedManifestRequiresThirdPartyAdjudication(t *testing.T) {
	manifest, _ := passingCorpus(500)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	adjudicatedAt := manifest.LockedAt
	clearDraftLabels(&manifest)
	second[0].Labels.ContentRole = "bumper"
	if _, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt); !containsFailure(failures, "divergent labels require adjudication") {
		t.Fatalf("missing adjudication failures = %v", failures)
	}
	adjudication := AdjudicationSubmission{
		CaseID: first[0].CaseID, AdjudicatorID: "reviewer-c", AdjudicatedAt: adjudicatedAt,
		Reason: "frame and transcript agree with the commercial label", Labels: first[0].Labels,
	}
	locked, failures := LockReviewedManifest(manifest, first, second, []AdjudicationSubmission{adjudication}, lockedAt)
	if len(failures) > 0 || locked.Cases[0].Adjudication == nil {
		t.Fatalf("adjudicated lock = %+v, %v", locked.Cases[0].Adjudication, failures)
	}
	if locked.Cases[0].LabelReviews[0].SubmissionSHA256 == locked.Cases[0].LabelReviews[1].SubmissionSHA256 {
		t.Fatal("divergent blind submissions were overwritten")
	}
}

func TestLockReviewedManifestRejectsLabelBearingDraft(t *testing.T) {
	manifest, _ := passingCorpus(500)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	clearDraftLabels(&manifest)
	manifest.Cases[0].Truth = TruthEligible
	if _, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt); !containsFailure(failures, "draft must not contain labels") {
		t.Fatalf("label-bearing draft failures = %v", failures)
	}
}

func TestLockReviewedManifestValidatesBothBlindSubmissions(t *testing.T) {
	manifest, _ := passingCorpus(500)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	adjudicatedAt := manifest.LockedAt
	clearDraftLabels(&manifest)
	second[0].Labels = Labels{Truth: TruthInvalid}
	adjudication := AdjudicationSubmission{CaseID: first[0].CaseID, AdjudicatorID: "reviewer-c", AdjudicatedAt: adjudicatedAt, Reason: "first review is supported", Labels: first[0].Labels}
	if _, failures := LockReviewedManifest(manifest, first, second, []AdjudicationSubmission{adjudication}, lockedAt); !containsFailure(failures, "second review: invalid truth requires a reject class") || !containsFailure(failures, "second review: at least one slice is required") {
		t.Fatalf("invalid losing review failures = %v", failures)
	}
}

func clearDraftLabels(manifest *Manifest) {
	manifest.LockedAt = time.Time{}
	for i := range manifest.Cases {
		c := &manifest.Cases[i]
		c.Truth, c.RejectClass, c.ContentRole, c.ReviewQuestion = "", "", "", ""
		c.Taxonomy, c.PolicyFlags, c.Slices, c.Evidence = nil, nil, nil, nil
		c.LabelReviews, c.Adjudication = nil, nil
	}
}

func submissionsFor(manifest Manifest) ([]LabelSubmission, []LabelSubmission) {
	first := make([]LabelSubmission, 0, len(manifest.Cases))
	second := make([]LabelSubmission, 0, len(manifest.Cases))
	for _, c := range manifest.Cases {
		labels := LabelsFromCase(c)
		first = append(first, LabelSubmission{CaseID: c.ID, ReviewerID: "reviewer-a", BatchID: "blind-a", ReviewedAt: manifest.LockedAt, Labels: labels})
		second = append(second, LabelSubmission{CaseID: c.ID, ReviewerID: "reviewer-b", BatchID: "blind-b", ReviewedAt: manifest.LockedAt.Add(time.Minute), Labels: labels})
	}
	return first, second
}

func TestLabelsHashCoversSlices(t *testing.T) {
	labels := Labels{Truth: TruthEligible, ContentRole: "commercial", Slices: []string{"commercial"}}
	before := LabelsSHA256(labels)
	labels.Slices = []string{"programme-excerpt"}
	if before == LabelsSHA256(labels) || strings.TrimSpace(before) == "" {
		t.Fatal("slice change did not change label identity")
	}
}

func TestLabelsHashIgnoresSetAndEvidenceOrder(t *testing.T) {
	first := Labels{
		Truth: TruthEligible, ContentRole: "commercial",
		PolicyFlags: []string{"political", "sensitive"}, Slices: []string{"commercial", "modern"},
		Taxonomy: map[string][]string{"product": {"soda", "drinks"}},
		Evidence: []Evidence{
			{ID: "b", Kind: "frame", Claim: "product", Value: "soda", Provenance: "frame"},
			{ID: "a", Kind: "transcript", Claim: "product", Value: "soda", Provenance: "transcript"},
		},
	}
	second := Labels{
		Truth: TruthEligible, ContentRole: "commercial",
		PolicyFlags: []string{"sensitive", "political"}, Slices: []string{"modern", "commercial"},
		Taxonomy: map[string][]string{"product": {"drinks", "soda"}},
		Evidence: []Evidence{first.Evidence[1], first.Evidence[0]},
	}
	if LabelsSHA256(first) != LabelsSHA256(second) {
		t.Fatal("semantically identical labels produced different hashes")
	}
}
