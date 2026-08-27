package fillereval

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// LabelSubmission is one blind reviewer batch entry. Review files contain
// labels, not decisions copied from the draft manifest.
type LabelSubmission struct {
	Alias      string    `json:"alias"`
	ReviewerID string    `json:"reviewerId"`
	BatchID    string    `json:"batchId"`
	ReviewedAt time.Time `json:"reviewedAt"`
	Labels     Labels    `json:"labels"`
}

type AdjudicationSubmission struct {
	CaseID        string    `json:"caseId"`
	AdjudicatorID string    `json:"adjudicatorId"`
	AdjudicatedAt time.Time `json:"adjudicatedAt"`
	Reason        string    `json:"reason"`
	Labels        Labels    `json:"labels"`
}

// LockReviewedManifest combines two independently authored review batches.
// Matching labels become final directly; disagreements require a third-party
// adjudication. No source media or evidence extraction happens here.
func LockReviewedManifest(draft Manifest, first, second BlindReviewSet, adjudications []AdjudicationSubmission, lockedAt time.Time) (Manifest, []string) {
	locked := draft
	locked.Cases = slices.Clone(draft.Cases)
	locked.LockedAt = lockedAt.UTC()
	failures := ValidateReviewDraft(draft)
	firstByID, firstFailures := unblindSubmissions("first review", draft, first)
	failures = append(failures, firstFailures...)
	secondByID, moreFailures := unblindSubmissions("second review", draft, second)
	failures = append(failures, moreFailures...)
	adjudicationByID, adjudicationFailures := indexAdjudications(adjudications)
	failures = append(failures, adjudicationFailures...)
	usedAdjudications := map[string]struct{}{}

	caseIDs := make(map[string]struct{}, len(locked.Cases))
	for i := range locked.Cases {
		c := &locked.Cases[i]
		caseIDs[c.ID] = struct{}{}
		a, aOK := firstByID[c.ID]
		b, bOK := secondByID[c.ID]
		if !aOK || !bOK {
			failures = append(failures, c.ID+": both independent review submissions are required")
			continue
		}
		if a.ReviewerID == b.ReviewerID || first.Map.BatchID == second.Map.BatchID {
			failures = append(failures, c.ID+": reviewers and blind-review batches must be distinct")
			continue
		}
		aHash := LabelsSHA256(a.Labels)
		bHash := LabelsSHA256(b.Labels)
		failures = append(failures, validateLabels(c.ID+": first review", a.Labels)...)
		failures = append(failures, validateLabels(c.ID+": second review", b.Labels)...)
		finalLabels := a.Labels
		var adjudication *LabelAdjudication
		if aHash != bHash {
			resolution, ok := adjudicationByID[c.ID]
			if !ok {
				failures = append(failures, c.ID+": divergent labels require adjudication")
				continue
			}
			if resolution.AdjudicatorID == a.ReviewerID || resolution.AdjudicatorID == b.ReviewerID {
				failures = append(failures, c.ID+": adjudicator must differ from both reviewers")
				continue
			}
			failures = append(failures, validateLabels(c.ID+": adjudication", resolution.Labels)...)
			finalLabels = resolution.Labels
			usedAdjudications[c.ID] = struct{}{}
			adjudication = &LabelAdjudication{
				AdjudicatorID: resolution.AdjudicatorID, AdjudicatedAt: resolution.AdjudicatedAt,
				LabelSHA256: LabelsSHA256(resolution.Labels), Reason: strings.TrimSpace(resolution.Reason),
			}
		}
		ApplyLabels(c, finalLabels)
		c.LabelReviews = []LabelReview{
			{ReviewerID: a.ReviewerID, BatchID: first.Map.BatchID, ReviewedAt: a.ReviewedAt, Independent: true, SubmissionSHA256: aHash},
			{ReviewerID: b.ReviewerID, BatchID: second.Map.BatchID, ReviewedAt: b.ReviewedAt, Independent: true, SubmissionSHA256: bHash},
		}
		c.Adjudication = adjudication
	}
	for id := range firstByID {
		if _, exists := caseIDs[id]; !exists {
			failures = append(failures, id+": first review has no draft case")
		}
	}
	for id := range secondByID {
		if _, exists := caseIDs[id]; !exists {
			failures = append(failures, id+": second review has no draft case")
		}
	}
	for id := range adjudicationByID {
		if _, used := usedAdjudications[id]; !used {
			failures = append(failures, id+": adjudication does not resolve a label disagreement")
		}
	}
	if len(failures) == 0 {
		failures = append(failures, ValidateManifest(locked)...)
		if locked.Kind == CorpusCertification {
			failures = append(failures, ValidateCertificationContract(locked)...)
		}
	}
	return locked, failures
}

func validateUnlabeledDraft(draft Manifest) []string {
	var failures []string
	if draft.SchemaVersion != SchemaVersion || draft.Kind != CorpusDevelopmentSeed && draft.Kind != CorpusCertification || strings.TrimSpace(draft.CorpusVersion) == "" || !draft.LockedAt.IsZero() {
		failures = append(failures, "draft requires the current development_seed or certification schema, corpus identity, and no prior lock time")
	}
	for _, c := range draft.Cases {
		if c.Truth != "" || c.RejectClass != "" || strings.TrimSpace(c.ContentRole) != "" || len(c.Taxonomy) != 0 || len(c.PolicyFlags) != 0 || len(c.Slices) != 0 || len(c.Evidence) != 0 || strings.TrimSpace(c.ReviewQuestion) != "" || len(c.LabelReviews) != 0 || c.Adjudication != nil {
			failures = append(failures, c.ID+": draft must not contain labels, reviews, or adjudication")
		}
	}
	return failures
}

func validateReviewDraftScale(draft Manifest) []string {
	if draft.Kind == CorpusCertification {
		return ValidateCertificationDraft(draft)
	}
	if draft.Kind != CorpusDevelopmentSeed {
		return nil
	}
	var failures []string
	development := 0
	for _, c := range draft.Cases {
		if c.Split != SplitDevelopment {
			failures = append(failures, c.ID+": development seed may contain only development cases")
			continue
		}
		development++
	}
	if development < CertificationMinDevelopment {
		failures = append(failures, fmt.Sprintf("development seed has %d cases; require at least %d", development, CertificationMinDevelopment))
	}
	return failures
}

// ValidateReviewDraft verifies the complete unlabeled input contract shared by
// blind packet generation and reviewer evidence packaging.
func ValidateReviewDraft(draft Manifest) []string {
	failures := validateUnlabeledDraft(draft)
	return append(failures, validateReviewDraftScale(draft)...)
}

func validateLabels(prefix string, labels Labels) []string {
	var failures []string
	if labels.Truth != TruthEligible && labels.Truth != TruthInvalid && labels.Truth != TruthAmbiguous {
		failures = append(failures, prefix+": invalid truth")
	}
	if labels.Truth == TruthInvalid && labels.RejectClass != RejectDeterministic && labels.RejectClass != RejectSemantic {
		failures = append(failures, prefix+": invalid truth requires a reject class")
	}
	if labels.Truth != TruthInvalid && labels.RejectClass != "" {
		failures = append(failures, prefix+": only invalid truth may carry a reject class")
	}
	if labels.Truth == TruthAmbiguous && strings.TrimSpace(labels.ReviewQuestion) == "" {
		failures = append(failures, prefix+": ambiguous truth requires a review question")
	}
	if labels.Truth != TruthAmbiguous && strings.TrimSpace(labels.ReviewQuestion) != "" {
		failures = append(failures, prefix+": only ambiguous truth may carry a review question")
	}
	if strings.TrimSpace(labels.ContentRole) == "" {
		failures = append(failures, prefix+": content role is required")
	}
	if len(labels.Slices) == 0 {
		failures = append(failures, prefix+": at least one slice is required")
	}
	if len(labels.Evidence) == 0 {
		failures = append(failures, prefix+": at least one evidence label is required")
	}
	evidenceIDs := map[string]struct{}{}
	for _, evidence := range labels.Evidence {
		if evidence.ID == "" || evidence.Kind == "" || evidence.Claim == "" || evidence.Provenance == "" {
			failures = append(failures, prefix+": evidence requires id, kind, claim, and provenance")
		}
		if _, exists := evidenceIDs[evidence.ID]; exists {
			failures = append(failures, prefix+": duplicate evidence id "+evidence.ID)
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}
	return failures
}

func indexAdjudications(submissions []AdjudicationSubmission) (map[string]AdjudicationSubmission, []string) {
	indexed := make(map[string]AdjudicationSubmission, len(submissions))
	var failures []string
	for i, submission := range submissions {
		if strings.TrimSpace(submission.CaseID) == "" || strings.TrimSpace(submission.AdjudicatorID) == "" || submission.AdjudicatedAt.IsZero() || strings.TrimSpace(submission.Reason) == "" {
			failures = append(failures, fmt.Sprintf("adjudication[%d]: case, adjudicator, time, and reason are required", i))
			continue
		}
		if _, exists := indexed[submission.CaseID]; exists {
			failures = append(failures, "duplicate adjudication for "+submission.CaseID)
			continue
		}
		indexed[submission.CaseID] = submission
	}
	return indexed, failures
}
