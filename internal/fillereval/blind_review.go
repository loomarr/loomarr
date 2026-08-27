package fillereval

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strings"
)

const BlindReviewSchemaVersion = 1

// BlindReviewPacket is reviewer-visible. It deliberately has no case ID,
// split, cluster, source title, source filename, creator, campaign, or labels.
type BlindReviewPacket struct {
	SchemaVersion int               `json:"schemaVersion"`
	BatchID       string            `json:"batchId"`
	DraftSHA256   string            `json:"draftSha256"`
	Cases         []BlindReviewCase `json:"cases"`
}

type BlindReviewCase struct {
	Alias             string `json:"alias"`
	ContentSHA256     string `json:"contentSha256"`
	EvidenceSHA256    string `json:"evidenceSha256"`
	SegmentStartMS    int64  `json:"segmentStartMs,omitempty"`
	SegmentDurationMS int64  `json:"segmentDurationMs"`
}

// BlindReviewMap is private coordinator material. It is required to unblind a
// completed batch and is never part of the reviewer packet.
type BlindReviewMap struct {
	SchemaVersion int                `json:"schemaVersion"`
	BatchID       string             `json:"batchId"`
	DraftSHA256   string             `json:"draftSha256"`
	Entries       []BlindReviewEntry `json:"entries"`
}

type BlindReviewEntry struct {
	Alias  string `json:"alias"`
	CaseID string `json:"caseId"`
}

type BlindReviewSet struct {
	Map         BlindReviewMap
	Submissions []LabelSubmission
}

// PrepareBlindReview gives each reviewer an independently randomized order and
// opaque aliases. Callers must keep the returned map private and write it with
// owner-only permissions.
func PrepareBlindReview(draft Manifest, batchID string, randomness io.Reader) (BlindReviewPacket, BlindReviewMap, []string) {
	batchID = strings.TrimSpace(batchID)
	failures := validateUnlabeledDraft(draft)
	failures = append(failures, ValidateCertificationDraft(draft)...)
	if batchID == "" {
		failures = append(failures, "blind review batch id is required")
	}
	if randomness == nil {
		randomness = rand.Reader
	}
	if len(failures) > 0 {
		return BlindReviewPacket{}, BlindReviewMap{}, failures
	}
	digest := ManifestSHA256(draft)
	packet := BlindReviewPacket{SchemaVersion: BlindReviewSchemaVersion, BatchID: batchID, DraftSHA256: digest}
	mapping := BlindReviewMap{SchemaVersion: BlindReviewSchemaVersion, BatchID: batchID, DraftSHA256: digest}
	aliases := map[string]struct{}{}
	for _, c := range draft.Cases {
		alias, err := randomAlias(randomness, aliases)
		if err != nil {
			return BlindReviewPacket{}, BlindReviewMap{}, []string{"blind review alias generation: " + err.Error()}
		}
		aliases[alias] = struct{}{}
		packet.Cases = append(packet.Cases, BlindReviewCase{Alias: alias, ContentSHA256: c.ContentSHA256, EvidenceSHA256: c.EvidenceSHA256, SegmentStartMS: c.Provenance.SegmentStartMS, SegmentDurationMS: c.Provenance.SegmentDurationMS})
		mapping.Entries = append(mapping.Entries, BlindReviewEntry{Alias: alias, CaseID: c.ID})
	}
	if err := shuffleReviewCases(packet.Cases, randomness); err != nil {
		return BlindReviewPacket{}, BlindReviewMap{}, []string{"blind review shuffle: " + err.Error()}
	}
	return packet, mapping, nil
}

func randomAlias(randomness io.Reader, seen map[string]struct{}) (string, error) {
	for attempts := 0; attempts < 4; attempts++ {
		value := make([]byte, 16)
		if _, err := io.ReadFull(randomness, value); err != nil {
			return "", err
		}
		alias := "review-" + hex.EncodeToString(value)
		if _, duplicate := seen[alias]; !duplicate {
			return alias, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique alias")
}

func shuffleReviewCases(cases []BlindReviewCase, randomness io.Reader) error {
	for i := len(cases) - 1; i > 0; i-- {
		j, err := rand.Int(randomness, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		cases[i], cases[j.Int64()] = cases[j.Int64()], cases[i]
	}
	return nil
}

func unblindSubmissions(name string, draft Manifest, review BlindReviewSet) (map[string]LabelSubmission, []string) {
	mapping := review.Map
	var failures []string
	if mapping.SchemaVersion != BlindReviewSchemaVersion || strings.TrimSpace(mapping.BatchID) == "" || mapping.DraftSHA256 != ManifestSHA256(draft) {
		failures = append(failures, name+": alias map must use the current schema and bind this exact draft")
	}
	caseIDs := make(map[string]struct{}, len(draft.Cases))
	for _, c := range draft.Cases {
		caseIDs[c.ID] = struct{}{}
	}
	byAlias := make(map[string]string, len(mapping.Entries))
	mappedCases := make(map[string]struct{}, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		if strings.TrimSpace(entry.Alias) == "" || strings.TrimSpace(entry.CaseID) == "" {
			failures = append(failures, name+": alias map entries require alias and case identity")
			continue
		}
		if _, duplicate := byAlias[entry.Alias]; duplicate {
			failures = append(failures, name+": duplicate alias "+entry.Alias)
		}
		if _, duplicate := mappedCases[entry.CaseID]; duplicate {
			failures = append(failures, name+": duplicate mapped case "+entry.CaseID)
		}
		if _, exists := caseIDs[entry.CaseID]; !exists {
			failures = append(failures, name+": mapped case is absent from draft "+entry.CaseID)
		}
		byAlias[entry.Alias] = entry.CaseID
		mappedCases[entry.CaseID] = struct{}{}
	}
	if len(mappedCases) != len(caseIDs) {
		failures = append(failures, fmt.Sprintf("%s: alias map covers %d/%d draft cases", name, len(mappedCases), len(caseIDs)))
	}
	indexed := make(map[string]LabelSubmission, len(review.Submissions))
	var reviewerID string
	for i, submission := range review.Submissions {
		if strings.TrimSpace(submission.Alias) == "" || strings.TrimSpace(submission.ReviewerID) == "" || submission.BatchID != mapping.BatchID || submission.ReviewedAt.IsZero() {
			failures = append(failures, fmt.Sprintf("%s[%d]: alias, reviewer, mapped batch, and review time are required", name, i))
			continue
		}
		caseID, ok := byAlias[submission.Alias]
		if !ok {
			failures = append(failures, name+": unknown alias "+submission.Alias)
			continue
		}
		if _, duplicate := indexed[caseID]; duplicate {
			failures = append(failures, name+": duplicate submission for mapped case "+caseID)
			continue
		}
		if reviewerID == "" {
			reviewerID = submission.ReviewerID
		} else if submission.ReviewerID != reviewerID {
			failures = append(failures, name+": one file must contain exactly one reviewer")
			continue
		}
		indexed[caseID] = submission
	}
	return indexed, failures
}
