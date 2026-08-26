// Package filleradmission owns the deterministic semantic boundary between
// versioned filler evidence and a catalog-admission decision.
package filleradmission

const SchemaVersion = 1

type Claim string

const (
	ClaimMediaUsability Claim = "media_usability"
	ClaimRecordingDate  Claim = "recording_date"
	ClaimBrand          Claim = "brand"
	ClaimProduct        Claim = "product"
	ClaimContentRole    Claim = "content_role"
	ClaimSourceLicense  Claim = "source_license"
	ClaimSensitiveFlag  Claim = "sensitive_policy_flag"
)

type EvidenceKind string

const (
	KindDecoder          EvidenceKind = "decoder"
	KindSourcePolicy     EvidenceKind = "source_policy"
	KindRecordingSidecar EvidenceKind = "recording_sidecar"
	KindFilename         EvidenceKind = "filename"
	KindUploaderMetadata EvidenceKind = "uploader_metadata"
	KindTranscript       EvidenceKind = "transcript"
	KindOCR              EvidenceKind = "ocr"
	KindFrame            EvidenceKind = "frame"
	KindAudio            EvidenceKind = "audio"
	KindVideo            EvidenceKind = "video"
)

const (
	UsabilityUsable   = "usable"
	UsabilityUnusable = "unusable"

	EligibilityEligible   = "eligible"
	EligibilityIneligible = "ineligible"

	RoleCommercial       = "commercial"
	RoleBumper           = "bumper"
	RolePSA              = "psa"
	RoleStationID        = "station_id"
	RoleTrailer          = "trailer"
	RoleInterstitial     = "interstitial"
	RoleProgrammeExcerpt = "programme_excerpt"
	RoleCompilation      = "compilation"
)

type Evidence struct {
	ID           string       `json:"id"`
	Claim        Claim        `json:"claim"`
	Value        string       `json:"value"`
	Kind         EvidenceKind `json:"kind"`
	Source       string       `json:"source"`
	Derivative   string       `json:"derivative,omitempty"`
	Location     string       `json:"location,omitempty"`
	AtMS         int64        `json:"atMs,omitempty"`
	EvaluationID string       `json:"evaluationId,omitempty"`
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

// Attribution is copied from the provider-neutral inference record. The
// evaluator does not calculate usage, price, model identity, or confidence.
type Attribution struct {
	EvaluationID      string     `json:"evaluationId"`
	Role              string     `json:"role"`
	Rung              string     `json:"rung,omitempty"`
	RequestedProvider string     `json:"requestedProvider,omitempty"`
	RequestedModel    string     `json:"requestedModel,omitempty"`
	ResolvedProvider  string     `json:"resolvedProvider,omitempty"`
	ResolvedModel     string     `json:"resolvedModel,omitempty"`
	UpstreamProvider  string     `json:"upstreamProvider,omitempty"`
	Modalities        []string   `json:"modalities,omitempty"`
	Tokens            TokenUsage `json:"tokens"`
	ChargedAmount     string     `json:"chargedAmount,omitempty"`
	ChargedCurrency   string     `json:"chargedCurrency,omitempty"`
	LatencyMS         int64      `json:"latencyMs,omitempty"`
	Attempts          int        `json:"attempts,omitempty"`
	GenerationID      string     `json:"generationId,omitempty"`
}

type OperationalCode string

const (
	HoldProviderUnavailable OperationalCode = "provider_unavailable"
	HoldBudgetExhausted     OperationalCode = "budget_exhausted"
	HoldExtractionFailed    OperationalCode = "extraction_failed"
	HoldRouteUnavailable    OperationalCode = "route_unavailable"
	HoldSchemaInvalid       OperationalCode = "schema_invalid"
	HoldTaxonomyInvalid     OperationalCode = "taxonomy_invalid"
	HoldEvidenceInvalid     OperationalCode = "evidence_invalid"
)

type OperationalIssue struct {
	Code      OperationalCode `json:"code"`
	Detail    string          `json:"detail,omitempty"`
	Retryable bool            `json:"retryable,omitempty"`
}

type Document struct {
	SchemaVersion   int                `json:"schemaVersion"`
	EvidenceVersion string             `json:"evidenceVersion"`
	PolicyVersion   string             `json:"policyVersion"`
	TaxonomyVersion string             `json:"taxonomyVersion"`
	ClipHash        string             `json:"clipHash"`
	Evidence        []Evidence         `json:"evidence"`
	Operational     []OperationalIssue `json:"operational,omitempty"`
	Attribution     []Attribution      `json:"attribution,omitempty"`
	// ModelConfidence survives for diagnostics and calibration. Evaluate never
	// reads it when selecting a verdict, reason, reference, or question.
	ModelConfidence *float64 `json:"modelConfidence,omitempty"`
}

type Policy struct {
	Version             string
	TaxonomyVersion     string
	AllowedProducts     []string
	AllowedContentRoles []string
	KnownSensitiveFlags []string
	ProhibitedFlags     []string
}

type Verdict string

const (
	VerdictAdmit  Verdict = "admit"
	VerdictReject Verdict = "reject"
	VerdictReview Verdict = "review"
)

type ReasonCode string

const (
	ReasonEvidenceSatisfied             ReasonCode = "evidence_satisfied"
	ReasonMediaUnusable                 ReasonCode = "media_unusable"
	ReasonSourceIneligible              ReasonCode = "source_ineligible"
	ReasonSensitivePolicyProhibited     ReasonCode = "sensitive_policy_prohibited"
	ReasonContentRoleNotFiller          ReasonCode = "content_role_not_filler"
	ReasonConflictMediaUsability        ReasonCode = "conflict_media_usability"
	ReasonConflictRecordingDate         ReasonCode = "conflict_recording_date"
	ReasonConflictBrand                 ReasonCode = "conflict_brand"
	ReasonConflictProduct               ReasonCode = "conflict_product"
	ReasonConflictContentRole           ReasonCode = "conflict_content_role"
	ReasonConflictSourceLicense         ReasonCode = "conflict_source_license"
	ReasonMissingMediaUsability         ReasonCode = "missing_media_usability"
	ReasonMissingSourceLicense          ReasonCode = "missing_source_license"
	ReasonMissingContentRole            ReasonCode = "missing_content_role"
	ReasonInsufficientContentRole       ReasonCode = "insufficient_content_role_evidence"
	ReasonMissingCommercialIdentity     ReasonCode = "missing_commercial_identity"
	ReasonInsufficientProductEvidence   ReasonCode = "insufficient_product_evidence"
	ReasonInsufficientSensitiveEvidence ReasonCode = "insufficient_sensitive_policy_evidence"
	// ReasonTemporalAmbiguity is reserved for evidence that requires observing
	// ordering or change over time; only the direct-video bakeoff rung may use it.
	ReasonTemporalAmbiguity ReasonCode = "temporal_ambiguity"
)

type Conflict struct {
	Claim        Claim    `json:"claim"`
	Values       []string `json:"values"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Resolved     bool     `json:"resolved"`
	ResolvedBy   []string `json:"resolvedBy,omitempty"`
}

type Decision struct {
	Verdict        Verdict       `json:"verdict"`
	ReasonCodes    []ReasonCode  `json:"reasonCodes"`
	EvidenceRefs   []string      `json:"evidenceRefs"`
	Conflicts      []Conflict    `json:"conflicts,omitempty"`
	ReviewQuestion string        `json:"reviewQuestion,omitempty"`
	Attribution    []Attribution `json:"attribution,omitempty"`
}

type Hold struct {
	Code        OperationalCode `json:"code"`
	Detail      string          `json:"detail,omitempty"`
	Retryable   bool            `json:"retryable,omitempty"`
	Attribution []Attribution   `json:"attribution,omitempty"`
}

// Result carries exactly one of Decision or Hold. Operational failure is not a
// fourth semantic verdict and therefore cannot accidentally enter admission
// metrics or the human review queue.
type Result struct {
	Decision *Decision `json:"decision,omitempty"`
	Hold     *Hold     `json:"hold,omitempty"`
}
