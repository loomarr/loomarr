// Package split holds the filler source-structure planner and the split model it plans over: the
// exact source asset, the proposed segments, the detector checkpoint, and the deterministic
// reduction of independent observations into one complete-timeline assessment. It sits above
// filler/structure and below the filler root, which owns the Splitter and the pipeline stages.
package split

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	SchemaVersion   = 1
	ContractVersion = "filler-source-structure-v1"
)

// Kind answers what the complete source contains. Duration is deliberately absent:
// it can quarantine a source for assessment, but cannot establish any of these semantic claims.
type Kind string

const (
	KindSingleUnit       Kind = "single_unit"
	KindCompilationBreak Kind = "compilation_break"
	KindProgrammeSpots   Kind = "programme_with_spots"
	KindAmbiguous        Kind = "ambiguous"
	KindUnusable         Kind = "unusable"
)

type ObservationKind string

const (
	ObservationChapterEdge      ObservationKind = "chapter_edge"
	ObservationBlackInterval    ObservationKind = "black_interval"
	ObservationSilenceInterval  ObservationKind = "silence_interval"
	ObservationTranscriptChange ObservationKind = "transcript_topic_change"
	ObservationOCRLogoChange    ObservationKind = "ocr_logo_change"
	ObservationAudioContinuity  ObservationKind = "audio_continuity"
	ObservationVisualContinuity ObservationKind = "visual_continuity"
	ObservationSceneChange      ObservationKind = "scene_change"
	ObservationStandardDuration ObservationKind = "standard_duration"
	ObservationSegmentRole      ObservationKind = "segment_role"
	// ObservationCompleteTimelineDecision projects a replayed multi-family artifact; it is not
	// another raw signal or assessor vote. Certified publication still requires that artifact.
	ObservationCompleteTimelineDecision ObservationKind = "complete_timeline_decision"
)

type ObservationEffect string

const (
	ObservationProposesBoundary    ObservationEffect = "proposes_boundary"
	ObservationSupportsBoundary    ObservationEffect = "supports_boundary"
	ObservationContradictsBoundary ObservationEffect = "contradicts_boundary"
	ObservationContextOnly         ObservationEffect = "context_only"
)

// Observation is one independently retained source-relative fact. StartMs and EndMs
// form an inclusive uncertainty window; a point observation has equal bounds.
type Observation struct {
	ID             string            `json:"id"`
	Kind           ObservationKind   `json:"kind"`
	Effect         ObservationEffect `json:"effect"`
	StartMs        int64             `json:"startMs"`
	EndMs          int64             `json:"endMs"`
	Producer       string            `json:"producer"`
	EvidenceSHA256 string            `json:"evidenceSha256"`
	RoleEvidence   *RoleEvidence     `json:"roleEvidence,omitempty"`
}

type BoundaryStatus string

const (
	BoundaryResolved   BoundaryStatus = "resolved"
	BoundaryUnresolved BoundaryStatus = "unresolved"
	BoundaryConflicted BoundaryStatus = "conflicted"
)

type Boundary struct {
	AtMs           int64          `json:"atMs"`
	WindowStartMs  int64          `json:"windowStartMs"`
	WindowEndMs    int64          `json:"windowEndMs"`
	Status         BoundaryStatus `json:"status"`
	ObservationIDs []string       `json:"observationIds"`
	ConflictIDs    []string       `json:"conflictIds,omitempty"`
}

type Role string

const (
	RoleCommercial        Role = "commercial"
	RolePromo             Role = "promo"
	RoleBumper            Role = "bumper"
	RoleStationID         Role = "station_id"
	RolePSA               Role = "psa"
	RoleTrailer           Role = "trailer"
	RoleInterstitial      Role = "interstitial"
	RoleProgrammeFragment Role = "programme_fragment"
	RoleNonFiller         Role = "non_filler"
	RoleAmbiguous         Role = "ambiguous"
	RoleUnusable          Role = "unusable"
)

type RoleClaim struct {
	StartMs     int64    `json:"startMs"`
	EndMs       int64    `json:"endMs"`
	Role        Role     `json:"role"`
	EvidenceIDs []string `json:"evidenceIds"`
	Reason      string   `json:"reason"`
}

type Disposition string

const (
	DispositionKeep       Disposition = "keep"
	DispositionDiscard    Disposition = "discard"
	DispositionUnresolved Disposition = "unresolved"
)

type DiscardReason string

const (
	DiscardBelowClipFloor    DiscardReason = "below_clip_floor"
	DiscardDuplicate         DiscardReason = "duplicate"
	DiscardProgrammeMaterial DiscardReason = "programme_material"
	DiscardNonFiller         DiscardReason = "non_filler"
	DiscardUnusableFragment  DiscardReason = "unusable_fragment"
)

type DiscardClaim struct {
	StartMs     int64         `json:"startMs"`
	EndMs       int64         `json:"endMs"`
	Reason      DiscardReason `json:"reason"`
	EvidenceIDs []string      `json:"evidenceIds"`
}

type PlanSegment struct {
	Index         int            `json:"index"`
	StartMs       int64          `json:"startMs"`
	EndMs         int64          `json:"endMs"`
	Disposition   Disposition    `json:"disposition"`
	Role          Role           `json:"role,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	DiscardReason DiscardReason  `json:"discardReason,omitempty"`
	EvidenceIDs   []string       `json:"evidenceIds,omitempty"`
	StartStatus   BoundaryStatus `json:"startStatus"`
	EndStatus     BoundaryStatus `json:"endStatus"`
}

// Assessment is the canonical complete-timeline result. SHA256 addresses the
// normalized document with SHA256 itself empty, so persisted copies can be independently verified.
type Assessment struct {
	SchemaVersion   int           `json:"schemaVersion"`
	ContractVersion string        `json:"contractVersion"`
	Source          SourceAsset   `json:"source"`
	DurationMs      int64         `json:"durationMs"`
	Kind            Kind          `json:"kind"`
	Observations    []Observation `json:"observations"`
	Boundaries      []Boundary    `json:"boundaries,omitempty"`
	Plan            []PlanSegment `json:"plan"`
	AssessedAt      time.Time     `json:"assessedAt"`
	UnusableReason  string        `json:"unusableReason,omitempty"`
	SHA256          string        `json:"sha256"`
}

type Input struct {
	Source         SourceAsset
	Observations   []Observation
	RoleClaims     []RoleClaim
	DiscardClaims  []DiscardClaim
	AssessedAt     time.Time
	UnusableReason string
}

// Assess is the module interface: exact source plus independent observations in,
// one deterministic complete-coverage assessment out.
func Assess(input Input) (Assessment, error) {
	return assess(input, nil)
}

func assess(input Input, authority *structureDecisionProjectionAuthority) (Assessment, error) {
	if err := input.Source.validate(); err != nil {
		return Assessment{}, fmt.Errorf("source structure: %w", err)
	}
	if input.AssessedAt.IsZero() {
		return Assessment{}, errors.New("source structure: assessment time is required")
	}
	observations, err := normalizeStructureObservations(input.Observations, input.Source.DurationMs)
	if err != nil {
		return Assessment{}, err
	}
	boundaries := fuseStructureBoundaries(observations, input.Source.DurationMs)
	plan, err := buildStructurePlan(input.Source, boundaries, input.RoleClaims, input.DiscardClaims, observations, authority)
	if err != nil {
		return Assessment{}, err
	}
	assessment := Assessment{
		SchemaVersion: SchemaVersion, ContractVersion: ContractVersion,
		Source: input.Source, DurationMs: input.Source.DurationMs, Observations: observations,
		Boundaries: boundaries, Plan: plan, AssessedAt: input.AssessedAt.UTC(),
		UnusableReason: strings.TrimSpace(input.UnusableReason),
	}
	assessment.Kind = reduceStructureKind(assessment)
	assessment.SHA256 = AssessmentSHA256(assessment)
	if err := validateAssessment(assessment, authority); err != nil {
		return Assessment{}, err
	}
	return assessment, nil
}

// ValidateAssessment reports whether assessment is exactly what Assess would reduce from its own
// source, observations and claims, with a matching digest.
func ValidateAssessment(assessment Assessment) error {
	return validateAssessment(assessment, nil)
}

func validateAssessment(assessment Assessment, authority *structureDecisionProjectionAuthority) error {
	if assessment.SchemaVersion != SchemaVersion || assessment.ContractVersion != ContractVersion {
		return errors.New("source structure: unsupported assessment contract")
	}
	if err := assessment.Source.validate(); err != nil || assessment.DurationMs != assessment.Source.DurationMs || assessment.AssessedAt.IsZero() {
		return errors.New("source structure: invalid source binding or assessment time")
	}
	if !validSourceStructureKind(assessment.Kind) {
		return errors.New("source structure: invalid source verdict")
	}
	if err := validateStructureCoverage(assessment.Plan, assessment.DurationMs); err != nil {
		return err
	}
	if assessment.Kind == KindUnusable && strings.TrimSpace(assessment.UnusableReason) == "" {
		return errors.New("source structure: unusable verdict requires a reason")
	}
	normalized, err := normalizeStructureObservations(assessment.Observations, assessment.DurationMs)
	if err != nil || !reflect.DeepEqual(normalized, assessment.Observations) {
		return errors.New("source structure: observations are invalid or non-canonical")
	}
	if err := validateStructureRoleObservationSources(normalized, assessment.Source); err != nil {
		return err
	}
	boundaries := fuseStructureBoundaries(normalized, assessment.DurationMs)
	if !reflect.DeepEqual(boundaries, assessment.Boundaries) {
		return errors.New("source structure: boundaries do not reduce from observations")
	}
	claims := make([]RoleClaim, 0, len(assessment.Plan))
	discards := make([]DiscardClaim, 0, len(assessment.Plan))
	for _, segment := range assessment.Plan {
		if segment.Role != "" {
			claims = append(claims, RoleClaim{
				StartMs: segment.StartMs, EndMs: segment.EndMs, Role: segment.Role,
				EvidenceIDs: segment.EvidenceIDs, Reason: segment.Reason,
			})
		}
		if segment.Disposition == DispositionDiscard && segment.Role == "" {
			discards = append(discards, DiscardClaim{
				StartMs: segment.StartMs, EndMs: segment.EndMs, Reason: segment.DiscardReason,
				EvidenceIDs: segment.EvidenceIDs,
			})
		}
	}
	plan, err := buildStructurePlan(assessment.Source, boundaries, claims, discards, normalized, authority)
	if err != nil || !reflect.DeepEqual(plan, assessment.Plan) {
		return errors.New("source structure: plan does not reduce from boundaries and role evidence")
	}
	if assessment.Kind != reduceStructureKind(assessment) {
		return errors.New("source structure: source verdict does not reduce from the plan")
	}
	if assessment.SHA256 == "" || assessment.SHA256 != AssessmentSHA256(assessment) {
		return errors.New("source structure: assessment digest does not match")
	}
	return nil
}

func AssessmentSHA256(assessment Assessment) string {
	assessment.SHA256 = ""
	raw, err := json.Marshal(assessment)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
