// Package fillereval owns the hermetic certification contract for filler admission.
// It scores captured decisions; it never calls a provider or reads media itself.
package fillereval

import "time"

const SchemaVersion = 2

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
	CorpusVersion string      `json:"corpusVersion"`
	Cases         []Case      `json:"cases"`
	SliceGates    []SliceGate `json:"sliceGates"`
}

type SliceGate struct {
	Slice       string  `json:"slice"`
	MinCases    int     `json:"minCases"`
	MinAccuracy float64 `json:"minAccuracy"`
}

type Case struct {
	ID             string              `json:"id"`
	Split          Split               `json:"split"`
	Cluster        string              `json:"cluster"`
	ContentSHA256  string              `json:"contentSha256,omitempty"`
	Source         string              `json:"source"`
	License        string              `json:"license"`
	Truth          Truth               `json:"truth"`
	RejectClass    RejectClass         `json:"rejectClass,omitempty"`
	ContentRole    string              `json:"contentRole"`
	Taxonomy       map[string][]string `json:"taxonomy,omitempty"`
	PolicyFlags    []string            `json:"policyFlags,omitempty"`
	Slices         []string            `json:"slices"`
	Evidence       []Evidence          `json:"evidence"`
	ReviewQuestion string              `json:"reviewQuestion,omitempty"`
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
	Role               string              `json:"role"`
	Rung               string              `json:"rung"`
	RequestedProvider  string              `json:"requestedProvider"`
	RequestedModel     string              `json:"requestedModel"`
	ResolvedModel      string              `json:"resolvedModel"`
	ResolvedProvider   string              `json:"resolvedProvider"`
	Modalities         []string            `json:"modalities"`
	Derivative         Derivative          `json:"derivative"`
	Tokens             TokenUsage          `json:"tokens"`
	ChargedAmount      string              `json:"chargedAmount,omitempty"`
	ChargedCurrency    string              `json:"chargedCurrency,omitempty"`
	ChargedNanoUSD     int64               `json:"chargedNanoUsd,omitempty"`
	EstimatedNanoUSD   int64               `json:"estimatedNanoUsd,omitempty"`
	Attempts           int                 `json:"attempts"`
	GenerationID       string              `json:"generationId,omitempty"`
	LatencyMS          int64               `json:"latencyMs,omitempty"`
	OperationalFailure string              `json:"operationalFailure,omitempty"`
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
	EvidenceVersion    string    `json:"evidenceVersion"`
	PromptVersion      string    `json:"promptVersion"`
	TaxonomyVersion    string    `json:"taxonomyVersion"`
	PolicyVersion      string    `json:"policyVersion"`
	RolePolicyVersion  string    `json:"rolePolicyVersion"`
	CapabilitySnapshot string    `json:"capabilitySnapshot"`
	PriceSnapshot      string    `json:"priceSnapshot"`
	GeneratedAt        time.Time `json:"generatedAt"`
}

type Report struct {
	SchemaVersion int          `json:"schemaVersion"`
	CorpusVersion string       `json:"corpusVersion"`
	Run           RunIdentity  `json:"run"`
	Certified     bool         `json:"certified"`
	Failures      []string     `json:"failures"`
	Metrics       Metrics      `json:"metrics"`
	Slices        []SliceScore `json:"slices"`
	Cases         []CaseResult `json:"cases"`
}

type Metrics struct {
	Cases                           int         `json:"cases"`
	AutoAdmit                       int         `json:"autoAdmit"`
	AutoAdmitCorrect                int         `json:"autoAdmitCorrect"`
	AutoAdmitPrecision              float64     `json:"autoAdmitPrecision"`
	AutoAdmitPrecisionLower         float64     `json:"autoAdmitPrecisionLower"`
	ValidAutomation                 float64     `json:"validAutomation"`
	AutoReject                      int         `json:"autoReject"`
	AutoRejectCorrect               int         `json:"autoRejectCorrect"`
	AutoRejectPrecision             float64     `json:"autoRejectPrecision"`
	DeterministicRejectPrecision    float64     `json:"deterministicRejectPrecision"`
	SemanticRejectPrecision         float64     `json:"semanticRejectPrecision"`
	InvalidAutomation               float64     `json:"invalidAutomation"`
	ReviewRate                      float64     `json:"reviewRate"`
	ReviewAnswerable                float64     `json:"reviewAnswerable"`
	AdmittedRoleAccuracy            float64     `json:"admittedRoleAccuracy"`
	AdmittedTaxonomyAccuracy        float64     `json:"admittedTaxonomyAccuracy"`
	BrierScore                      float64     `json:"brierScore,omitempty"`
	TotalChargedNanoUSD             int64       `json:"totalChargedNanoUsd"`
	TotalChargedCostUSD             float64     `json:"totalChargedCostUsd"`
	CostPerThousandCasesNanoUSD     int64       `json:"costPerThousandCasesNanoUsd"`
	CostPerCorrectAutomationNanoUSD int64       `json:"costPerCorrectAutomationNanoUsd"`
	CostPerAdmitNanoUSD             int64       `json:"costPerAdmitNanoUsd"`
	P50LatencyMS                    int64       `json:"p50LatencyMs"`
	P95LatencyMS                    int64       `json:"p95LatencyMs"`
	Rungs                           []RungScore `json:"rungs"`
}

type SliceScore struct {
	Slice                 string  `json:"slice"`
	Cases                 int     `json:"cases"`
	Correct               int     `json:"correct"`
	Accuracy              float64 `json:"accuracy"`
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
	CaseID   string   `json:"caseId"`
	Slice    []string `json:"slices"`
	Expected Truth    `json:"expected"`
	Actual   Verdict  `json:"actual"`
	Correct  bool     `json:"correct"`
	Failure  string   `json:"failure,omitempty"`
}
