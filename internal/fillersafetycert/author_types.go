package fillersafetycert

import (
	"time"

	"github.com/loomarr/loomarr/internal/fillersafety"
)

const (
	AuthorityDraftSchemaVersion    = 1
	AuthorityDraftContractVersion  = "filler-spoken-cascade-authority-draft-v1"
	AuthorityReviewSchemaVersion   = 1
	AuthorityReviewContractVersion = "filler-spoken-cascade-authority-review-v1"
)

// AuthorityDraft is the private, path-bearing source and truth declaration
// reviewed before any certification run. Its exact bytes are the corpus
// manifest identity; none of its private identifiers or paths reach Authority.
type AuthorityDraft struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ContractVersion string               `json:"contractVersion"`
	ChallengeKind   string               `json:"challengeKind"`
	PolicySHA256    string               `json:"policySha256"`
	ProposerSHA256  string               `json:"proposerSha256"`
	ProposerFamily  string               `json:"proposerFamily"`
	Implementation  string               `json:"implementation"`
	AudioRoute      RouteAuthority       `json:"audioRoute"`
	VideoRoute      RouteAuthority       `json:"videoRoute"`
	Cases           []AuthorityDraftCase `json:"cases"`
}

type AuthorityDraftCase struct {
	CaseID                string                       `json:"caseId"`
	SourcePath            string                       `json:"sourcePath"`
	SourceAuthority       fillersafety.SourceAuthority `json:"sourceAuthority"`
	SourceFamily          string                       `json:"sourceFamily"`
	TruthProvenancePath   string                       `json:"truthProvenancePath"`
	TruthProvenanceSHA256 string                       `json:"truthProvenanceSha256"`
	RightsPath            string                       `json:"rightsPath"`
	RightsSHA256          string                       `json:"rightsSha256"`
	Label                 string                       `json:"label"`
	Locale                string                       `json:"locale"`
	Slices                []string                     `json:"slices"`
	PositiveIntervals     []PositiveInterval           `json:"positiveIntervals,omitempty"`
}

// AuthorityReview is one independently produced, evaluation-output-blind
// submission over the exact draft. Primary submissions cover every case; an
// adjudicator covers exactly the cases on which the primaries disagree.
type AuthorityReview struct {
	SchemaVersion   int                `json:"schemaVersion"`
	ContractVersion string             `json:"contractVersion"`
	DraftSHA256     string             `json:"draftSha256"`
	ReviewerID      string             `json:"reviewerId"`
	Role            string             `json:"role"`
	Method          string             `json:"method"`
	ModelFamily     string             `json:"modelFamily,omitempty"`
	SubmittedAt     time.Time          `json:"submittedAt"`
	Assessments     []ReviewAssessment `json:"assessments"`
}

type ReviewAssessment struct {
	CaseID            string             `json:"caseId"`
	Decision          string             `json:"decision"`
	PositiveIntervals []PositiveInterval `json:"positiveIntervals,omitempty"`
}

type AuthorityBuildConfig struct {
	DraftPath          string
	FirstReviewPath    string
	SecondReviewPath   string
	AdjudicatorPath    string
	SeedPath           string
	SourceRoot         string
	AuthoredAt         time.Time
	ExpectedCases      int
	MaximumSourceBytes int64
	OutputPath         string
}

type AuthorityBuildResult struct {
	Cases            int
	PositiveFamilies int
	CleanFamilies    int
	AuthoritySHA256  string
}
