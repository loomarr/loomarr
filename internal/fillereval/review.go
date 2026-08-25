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
	CaseID     string    `json:"caseId"`
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
func LockReviewedManifest(draft Manifest, first, second []LabelSubmission, adjudications []AdjudicationSubmission, lockedAt time.Time) (Manifest, []string) {
	locked := draft
	locked.Cases = slices.Clone(draft.Cases)
	locked.Kind = CorpusCertification
	locked.LockedAt = lockedAt.UTC()
	firstByID, failures := indexSubmissions("first review", first)
	secondByID, moreFailures := indexSubmissions("second review", second)
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
		if a.ReviewerID == b.ReviewerID || a.BatchID == b.BatchID {
			failures = append(failures, c.ID+": reviewers and blind-review batches must be distinct")
			continue
		}
		aHash := LabelsSHA256(a.Labels)
		bHash := LabelsSHA256(b.Labels)
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
			finalLabels = resolution.Labels
			usedAdjudications[c.ID] = struct{}{}
			adjudication = &LabelAdjudication{
				AdjudicatorID: resolution.AdjudicatorID, AdjudicatedAt: resolution.AdjudicatedAt,
				LabelSHA256: LabelsSHA256(resolution.Labels), Reason: strings.TrimSpace(resolution.Reason),
			}
		}
		ApplyLabels(c, finalLabels)
		c.LabelReviews = []LabelReview{
			{ReviewerID: a.ReviewerID, BatchID: a.BatchID, ReviewedAt: a.ReviewedAt, Independent: true, SubmissionSHA256: aHash},
			{ReviewerID: b.ReviewerID, BatchID: b.BatchID, ReviewedAt: b.ReviewedAt, Independent: true, SubmissionSHA256: bHash},
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
	}
	return locked, failures
}

func indexSubmissions(name string, submissions []LabelSubmission) (map[string]LabelSubmission, []string) {
	indexed := make(map[string]LabelSubmission, len(submissions))
	var failures []string
	var reviewerID, batchID string
	for i, submission := range submissions {
		if strings.TrimSpace(submission.CaseID) == "" || strings.TrimSpace(submission.ReviewerID) == "" || strings.TrimSpace(submission.BatchID) == "" || submission.ReviewedAt.IsZero() {
			failures = append(failures, fmt.Sprintf("%s[%d]: case, reviewer, batch, and review time are required", name, i))
			continue
		}
		if _, exists := indexed[submission.CaseID]; exists {
			failures = append(failures, name+": duplicate case "+submission.CaseID)
			continue
		}
		if reviewerID == "" {
			reviewerID, batchID = submission.ReviewerID, submission.BatchID
		} else if submission.ReviewerID != reviewerID || submission.BatchID != batchID {
			failures = append(failures, name+": one file must contain exactly one reviewer and blind-review batch")
			continue
		}
		indexed[submission.CaseID] = submission
	}
	return indexed, failures
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
