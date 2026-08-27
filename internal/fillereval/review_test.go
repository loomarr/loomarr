package fillereval

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestPrepareBlindReviewHidesCaseIdentityAndUsesIndependentAliases(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
	clearDraftLabels(&manifest)
	firstPacket, firstMap, failures := PrepareBlindReview(manifest, "blind-a", rand.New(rand.NewSource(1))) //nolint:gosec // deterministic test entropy
	if len(failures) > 0 {
		t.Fatalf("first packet: %v", failures)
	}
	secondPacket, _, failures := PrepareBlindReview(manifest, "blind-b", rand.New(rand.NewSource(2))) //nolint:gosec // deterministic test entropy
	if len(failures) > 0 {
		t.Fatalf("second packet: %v", failures)
	}
	raw, err := json.Marshal(firstPacket)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{manifest.Cases[0].ID, "caseId", "sourceFilename", "contentRole", "truth", "cluster", "campaign", "creator"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("review packet leaks %q", forbidden)
		}
	}
	if firstPacket.Cases[0].Alias == secondPacket.Cases[0].Alias || firstMap.DraftSHA256 != ManifestSHA256(manifest) {
		t.Fatal("review batches did not receive independent aliases bound to the draft")
	}
}

func TestLockReviewedManifestRejectsAliasMapFromAnotherDraft(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	clearDraftLabels(&manifest)
	first.Map.DraftSHA256 = strings.Repeat("f", 64)
	if _, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt); !containsFailure(failures, "bind this exact draft") {
		t.Fatalf("foreign alias map accepted: %v", failures)
	}
}

func TestLockReviewedManifestPreservesIndependentSubmissions(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
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
	manifest, _ := passingCorpus(CertificationMinHoldout)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	adjudicatedAt := manifest.LockedAt
	clearDraftLabels(&manifest)
	second.Submissions[0].Labels.ContentRole = "bumper"
	if _, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt); !containsFailure(failures, "divergent labels require adjudication") {
		t.Fatalf("missing adjudication failures = %v", failures)
	}
	adjudication := AdjudicationSubmission{
		CaseID: second.Map.Entries[0].CaseID, AdjudicatorID: "reviewer-c", AdjudicatedAt: adjudicatedAt,
		Reason: "frame and transcript agree with the commercial label", Labels: first.Submissions[0].Labels,
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
	manifest, _ := passingCorpus(CertificationMinHoldout)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	clearDraftLabels(&manifest)
	manifest.Cases[0].Truth = TruthEligible
	if _, failures := LockReviewedManifest(manifest, first, second, nil, lockedAt); !containsFailure(failures, "draft must not contain labels") {
		t.Fatalf("label-bearing draft failures = %v", failures)
	}
}

func TestLockReviewedManifestValidatesBothBlindSubmissions(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
	first, second := submissionsFor(manifest)
	lockedAt := manifest.LockedAt.Add(time.Hour)
	adjudicatedAt := manifest.LockedAt
	clearDraftLabels(&manifest)
	second.Submissions[0].Labels = Labels{Truth: TruthInvalid}
	adjudication := AdjudicationSubmission{CaseID: second.Map.Entries[0].CaseID, AdjudicatorID: "reviewer-c", AdjudicatedAt: adjudicatedAt, Reason: "first review is supported", Labels: first.Submissions[0].Labels}
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

func submissionsFor(manifest Manifest) (BlindReviewSet, BlindReviewSet) {
	labels := make(map[string]Labels, len(manifest.Cases))
	for _, c := range manifest.Cases {
		labels[c.ID] = LabelsFromCase(c)
	}
	draft := manifest
	clearDraftLabels(&draft)
	_, firstMap, firstFailures := PrepareBlindReview(draft, "blind-a", rand.New(rand.NewSource(1)))   //nolint:gosec // deterministic test entropy
	_, secondMap, secondFailures := PrepareBlindReview(draft, "blind-b", rand.New(rand.NewSource(2))) //nolint:gosec // deterministic test entropy
	if len(firstFailures) > 0 || len(secondFailures) > 0 {
		panic("test blind review preparation failed")
	}
	first := BlindReviewSet{Map: firstMap}
	second := BlindReviewSet{Map: secondMap}
	for _, entry := range firstMap.Entries {
		first.Submissions = append(first.Submissions, LabelSubmission{Alias: entry.Alias, ReviewerID: "reviewer-a", BatchID: "blind-a", ReviewedAt: manifest.LockedAt, Labels: labels[entry.CaseID]})
	}
	for _, entry := range secondMap.Entries {
		second.Submissions = append(second.Submissions, LabelSubmission{Alias: entry.Alias, ReviewerID: "reviewer-b", BatchID: "blind-b", ReviewedAt: manifest.LockedAt.Add(time.Minute), Labels: labels[entry.CaseID]})
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
