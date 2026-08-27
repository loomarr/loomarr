// Package fillereval owns the hermetic certification contract for filler admission.
// It scores captured decisions; it never calls a provider or reads media itself.
package fillereval

import "time"

const SchemaVersion = 5

type CorpusKind string

const (
	CorpusDevelopmentSeed CorpusKind = "development_seed"
	CorpusCertification   CorpusKind = "certification"
)

type Split string

const (
	SplitDevelopment Split = "development"
	SplitHoldout     Split = "holdout"
)

type Truth string

const (
	TruthEligible  Truth = "eligible"
	TruthInvalid   Truth = "invalid"
	TruthAmbiguous Truth = "ambiguous"
)

type Verdict string

const (
	VerdictAdmit  Verdict = "admit"
	VerdictReject Verdict = "reject"
	VerdictReview Verdict = "review"
)

type RejectClass string

const (
	RejectDeterministic RejectClass = "deterministic"
	RejectSemantic      RejectClass = "semantic"
)

// Manifest is versioned independently from prompts, models, and admission policy.
// Corpus media is deliberately external; Case records its content hash and provenance.
type Manifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	Kind          CorpusKind  `json:"kind"`
	CorpusVersion string      `json:"corpusVersion"`
	LockedAt      time.Time   `json:"lockedAt,omitempty"`
	Cases         []Case      `json:"cases"`
	SliceGates    []SliceGate `json:"sliceGates"`
}

type SliceGate struct {
	Slice       string  `json:"slice"`
	MinCases    int     `json:"minCases"`
	MinAccuracy float64 `json:"minAccuracy"`
	// MinAccuracyLower is the required one-sided 95% Wilson lower bound.
	// Certification manifests must predeclare it; development seeds may omit it.
	MinAccuracyLower float64 `json:"minAccuracyLower,omitempty"`
}

type Case struct {
	ID             string              `json:"id"`
	Split          Split               `json:"split"`
	Cluster        string              `json:"cluster"`
	ContentSHA256  string              `json:"contentSha256,omitempty"`
	EvidenceSHA256 string              `json:"evidenceSha256,omitempty"`
	Source         string              `json:"source"`
	License        string              `json:"license"`
	Provenance     MediaProvenance     `json:"provenance,omitempty"`
	LabelReviews   []LabelReview       `json:"labelReviews,omitempty"`
	Adjudication   *LabelAdjudication  `json:"adjudication,omitempty"`
	Truth          Truth               `json:"truth"`
	RejectClass    RejectClass         `json:"rejectClass,omitempty"`
	ContentRole    string              `json:"contentRole"`
	Taxonomy       map[string][]string `json:"taxonomy,omitempty"`
	PolicyFlags    []string            `json:"policyFlags,omitempty"`
	Slices         []string            `json:"slices"`
	Evidence       []Evidence          `json:"evidence"`
	ReviewQuestion string              `json:"reviewQuestion,omitempty"`
}

// MediaProvenance locks the external media and the item-level rights decision.
// Media bytes remain outside git when redistribution is not allowed.
type MediaProvenance struct {
	Authority           string    `json:"authority"`
	Collection          string    `json:"collection,omitempty"`
	ItemID              string    `json:"itemId"`
	ItemRef             string    `json:"itemRef"`
	MetadataRetrievedAt time.Time `json:"metadataRetrievedAt"`
	MetadataSHA256      string    `json:"metadataSha256"`
	EvidenceRef         string    `json:"evidenceRef"`
	LicenseURL          string    `json:"licenseUrl,omitempty"`
	RightsStatement     string    `json:"rightsStatement"`
	RightsDecision      string    `json:"rightsDecision"`
	RightsReviewerID    string    `json:"rightsReviewerId"`
	RightsReviewedAt    time.Time `json:"rightsReviewedAt"`
	Redistributable     bool      `json:"redistributable"`
	Creator             string    `json:"creator,omitempty"`
	Campaign            string    `json:"campaign,omitempty"`
	SourceFamily        string    `json:"sourceFamily,omitempty"`
	RequiredCredit      string    `json:"requiredCredit,omitempty"`
	Restrictions        []string  `json:"restrictions,omitempty"`
	SourceFilename      string    `json:"sourceFilename"`
	SourceRef           string    `json:"sourceRef"`
	SourceBytes         int64     `json:"sourceBytes"`
	SegmentStartMS      int64     `json:"segmentStartMs,omitempty"`
	SegmentDurationMS   int64     `json:"segmentDurationMs"`
}

// LabelReview is an independently produced attestation over the canonical
// disposition, role, taxonomy, policy, evidence, and review-question labels.
type LabelReview struct {
	ReviewerID       string    `json:"reviewerId"`
	BatchID          string    `json:"batchId"`
	ReviewedAt       time.Time `json:"reviewedAt"`
	Independent      bool      `json:"independent"`
	SubmissionSHA256 string    `json:"submissionSha256"`
}

type LabelAdjudication struct {
	AdjudicatorID string    `json:"adjudicatorId"`
	AdjudicatedAt time.Time `json:"adjudicatedAt"`
	LabelSHA256   string    `json:"labelSha256"`
	Reason        string    `json:"reason"`
}

type Evidence struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Claim      string `json:"claim"`
	Value      string `json:"value"`
	Provenance string `json:"provenance"`
	AtMS       int64  `json:"atMs,omitempty"`
}

// Prediction is one captured terminal decision. Model confidence is retained as
// a diagnostic feature only and is never interpreted as admission authority here.
type Prediction struct {
	CaseID             string              `json:"caseId"`
	Verdict            Verdict             `json:"verdict"`
	RejectClass        RejectClass         `json:"rejectClass,omitempty"`
	ContentRole        string              `json:"contentRole,omitempty"`
	Taxonomy           map[string][]string `json:"taxonomy,omitempty"`
	PolicyFlags        []string            `json:"policyFlags,omitempty"`
	ReasonCodes        []string            `json:"reasonCodes,omitempty"`
	EvidenceRefs       []string            `json:"evidenceRefs,omitempty"`
	Conflicts          []Conflict          `json:"conflicts,omitempty"`
	ReviewQuestion     string              `json:"reviewQuestion,omitempty"`
	Probability        *float64            `json:"probability,omitempty"`
	OperationalFailure string              `json:"operationalFailure,omitempty"`
	Steps              []InferenceStep     `json:"steps,omitempty"`
}

// InferenceStep is the immutable accounting envelope for one attempted rung.
// A cascade must not collapse several charged calls into the terminal route.
type InferenceStep struct {
	EvaluationID       string     `json:"evaluationId"`
	Role               string     `json:"role"`
	Rung               string     `json:"rung"`
	RequestedProvider  string     `json:"requestedProvider"`
	RequestedModel     string     `json:"requestedModel"`
	ResolvedModel      string     `json:"resolvedModel"`
	ResolvedProvider   string     `json:"resolvedProvider"`
	UpstreamProvider   string     `json:"upstreamProvider,omitempty"`
	Modalities         []string   `json:"modalities"`
	Derivative         Derivative `json:"derivative"`
	Tokens             TokenUsage `json:"tokens"`
	ChargedAmount      string     `json:"chargedAmount,omitempty"`
	ChargedCurrency    string     `json:"chargedCurrency,omitempty"`
	ChargedNanoUSD     int64      `json:"chargedNanoUsd,omitempty"`
	ReservedNanoUSD    int64      `json:"reservedNanoUsd,omitempty"`
	EstimatedNanoUSD   int64      `json:"estimatedNanoUsd,omitempty"`
	Attempts           int        `json:"attempts"`
	GenerationID       string     `json:"generationId,omitempty"`
	LatencyMS          int64      `json:"latencyMs,omitempty"`
	Abstained          bool       `json:"abstained,omitempty"`
	AbstentionReason   string     `json:"abstentionReason,omitempty"`
	OperationalFailure string     `json:"operationalFailure,omitempty"`
}

type Derivative struct {
	Bytes      int64 `json:"bytes,omitempty"`
	DurationMS int64 `json:"durationMs,omitempty"`
	Pixels     int64 `json:"pixels,omitempty"`
}

type TokenUsage struct {
	Prompt     int64 `json:"prompt,omitempty"`
	Completion int64 `json:"completion,omitempty"`
	Reasoning  int64 `json:"reasoning,omitempty"`
	Cached     int64 `json:"cached,omitempty"`
	CacheWrite int64 `json:"cacheWrite,omitempty"`
	Image      int64 `json:"image,omitempty"`
	Audio      int64 `json:"audio,omitempty"`
	Video      int64 `json:"video,omitempty"`
}

type Conflict struct {
	Claim        string   `json:"claim"`
	Values       []string `json:"values"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type RunIdentity struct {
	Profile            string    `json:"profile"`
	EvaluationSplit    Split     `json:"evaluationSplit"`
	EvidenceVersion    string    `json:"evidenceVersion"`
	PromptVersion      string    `json:"promptVersion"`
	TaxonomyVersion    string    `json:"taxonomyVersion"`
	PolicyVersion      string    `json:"policyVersion"`
	RolePolicyVersion  string    `json:"rolePolicyVersion"`
	CapabilitySnapshot string    `json:"capabilitySnapshot"`
	PriceSnapshot      string    `json:"priceSnapshot"`
	GeneratedAt        time.Time `json:"generatedAt"`
	MaxRequests        int       `json:"maxRequests"`
	MaxSpendNanoUSD    int64     `json:"maxSpendNanoUsd"`
	MaxConcurrency     int       `json:"maxConcurrency"`
}

type Report struct {
	SchemaVersion  int          `json:"schemaVersion"`
	CorpusVersion  string       `json:"corpusVersion"`
	ManifestSHA256 string       `json:"manifestSha256"`
	Run            RunIdentity  `json:"run"`
	Certified      bool         `json:"certified"`
	Failures       []string     `json:"failures"`
	Metrics        Metrics      `json:"metrics"`
	Slices         []SliceScore `json:"slices"`
	Cases          []CaseResult `json:"cases"`
}

type Metrics struct {
	Cases                             int         `json:"cases"`
	AutoAdmit                         int         `json:"autoAdmit"`
	AutoAdmitCorrect                  int         `json:"autoAdmitCorrect"`
	AutoAdmitPrecision                float64     `json:"autoAdmitPrecision"`
	AutoAdmitPrecisionLower           float64     `json:"autoAdmitPrecisionLower"`
	ValidAutomation                   float64     `json:"validAutomation"`
	AutoReject                        int         `json:"autoReject"`
	AutoRejectCorrect                 int         `json:"autoRejectCorrect"`
	AutoRejectPrecision               float64     `json:"autoRejectPrecision"`
	AutoRejectPrecisionLower          float64     `json:"autoRejectPrecisionLower"`
	DeterministicRejectPrecision      float64     `json:"deterministicRejectPrecision"`
	DeterministicRejectPrecisionLower float64     `json:"deterministicRejectPrecisionLower"`
	SemanticRejectPrecision           float64     `json:"semanticRejectPrecision"`
	SemanticRejectPrecisionLower      float64     `json:"semanticRejectPrecisionLower"`
	InvalidAutomation                 float64     `json:"invalidAutomation"`
	InvalidAutomationLower            float64     `json:"invalidAutomationLower"`
	ReviewRate                        float64     `json:"reviewRate"`
	ReviewRateUpper                   float64     `json:"reviewRateUpper"`
	ReviewAnswerable                  float64     `json:"reviewAnswerable"`
	ReviewAnswerableLower             float64     `json:"reviewAnswerableLower"`
	ValidAutomationLower              float64     `json:"validAutomationLower"`
	AdmittedRoleAccuracy              float64     `json:"admittedRoleAccuracy"`
	AdmittedTaxonomyAccuracy          float64     `json:"admittedTaxonomyAccuracy"`
	BrierScore                        float64     `json:"brierScore,omitempty"`
	TotalChargedNanoUSD               int64       `json:"totalChargedNanoUsd"`
	TotalChargedCostUSD               float64     `json:"totalChargedCostUsd"`
	CostPerThousandCasesNanoUSD       int64       `json:"costPerThousandCasesNanoUsd"`
	CostPerCorrectAutomationNanoUSD   int64       `json:"costPerCorrectAutomationNanoUsd"`
	CostPerAdmitNanoUSD               int64       `json:"costPerAdmitNanoUsd"`
	P50LatencyMS                      int64       `json:"p50LatencyMs"`
	P95LatencyMS                      int64       `json:"p95LatencyMs"`
	Rungs                             []RungScore `json:"rungs"`
}

type SliceScore struct {
	Slice                 string  `json:"slice"`
	Cases                 int     `json:"cases"`
	Correct               int     `json:"correct"`
	Accuracy              float64 `json:"accuracy"`
	AccuracyLower         float64 `json:"accuracyLower"`
	ChargedNanoUSD        int64   `json:"chargedNanoUsd"`
	CostPerCorrectNanoUSD int64   `json:"costPerCorrectNanoUsd"`
}

type RungScore struct {
	Rung           string `json:"rung"`
	Cases          int    `json:"cases"`
	Correct        int    `json:"correct"`
	ChargedNanoUSD int64  `json:"chargedNanoUsd"`
}

type CaseResult struct {
	CaseID            string          `json:"caseId"`
	Slice             []string        `json:"slices"`
	Expected          Truth           `json:"expected"`
	Actual            Verdict         `json:"actual"`
	Correct           bool            `json:"correct"`
	Failure           string          `json:"failure,omitempty"`
	Role              string          `json:"role,omitempty"`
	Rung              string          `json:"rung,omitempty"`
	RequestedProvider string          `json:"requestedProvider,omitempty"`
	RequestedModel    string          `json:"requestedModel,omitempty"`
	ResolvedProvider  string          `json:"resolvedProvider,omitempty"`
	ResolvedModel     string          `json:"resolvedModel,omitempty"`
	UpstreamProvider  string          `json:"upstreamProvider,omitempty"`
	Modalities        []string        `json:"modalities,omitempty"`
	Derivative        Derivative      `json:"derivative,omitempty"`
	GenerationID      string          `json:"generationId,omitempty"`
	Attempts          int             `json:"attempts,omitempty"`
	ChargedNanoUSD    int64           `json:"chargedNanoUsd,omitempty"`
	LatencyMS         int64           `json:"latencyMs,omitempty"`
	Steps             []InferenceStep `json:"steps,omitempty"`
}
