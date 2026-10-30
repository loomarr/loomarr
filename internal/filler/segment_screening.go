package filler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerairworthiness"
)

const (
	SegmentScreeningSchemaVersion   = 5
	SegmentScreeningContractVersion = "filler-rendered-child-screening-v5"
)

var segmentScreeningAxisOrder = []SegmentScreeningAxis{
	ScreenVisualSafety,
	ScreenSpokenSafety,
	ScreenWrittenSafety,
	ScreenPlayback,
}

// SegmentScreeningAxes returns the four screening axes in canonical order. The caller owns the
// returned slice.
func SegmentScreeningAxes() []SegmentScreeningAxis {
	return slices.Clone(segmentScreeningAxisOrder)
}

// ValidateSegmentScreeningAxis reports whether axis is one of the four screening axes.
func ValidateSegmentScreeningAxis(axis SegmentScreeningAxis) error {
	if !slices.Contains(segmentScreeningAxisOrder, axis) {
		return fmt.Errorf("segment screening axis is invalid")
	}
	return nil
}

type SegmentScreeningAxis string

const (
	ScreenVisualSafety  SegmentScreeningAxis = "visual_safety"
	ScreenSpokenSafety  SegmentScreeningAxis = "spoken_safety"
	ScreenWrittenSafety SegmentScreeningAxis = "written_safety"
	ScreenPlayback      SegmentScreeningAxis = "playback_integrity"
)

type SegmentScreeningOutcome string

const (
	ScreenPass   SegmentScreeningOutcome = "pass"
	ScreenReject SegmentScreeningOutcome = "reject"
	ScreenHold   SegmentScreeningOutcome = "hold"
)

// SegmentScreeningResult carries one closed decision and the immutable artifact that made it.
// ReasonCode is deliberately opaque: restricted words or visual descriptions do not enter the
// proposal document merely to explain why a segment was blocked.
type SegmentScreeningResult struct {
	Axis            SegmentScreeningAxis    `json:"axis"`
	Outcome         SegmentScreeningOutcome `json:"outcome"`
	AuthoritySHA256 string                  `json:"authoritySha256"`
	ReasonCode      string                  `json:"reasonCode"`
}

// SegmentScreeningEvidence proves the four independent pre-publication screens for one immutable
// rendered-child subject. No individual pass, aggregate confidence, or absent result can
// substitute for all four authority-bound outcomes.
type SegmentScreeningEvidence struct {
	SchemaVersion   int                          `json:"schemaVersion"`
	ContractVersion string                       `json:"contractVersion"`
	SubjectSHA256   string                       `json:"subjectSha256"`
	Results         []SegmentScreeningResult     `json:"results"`
	Airworthiness   fillerairworthiness.Decision `json:"airworthiness"`
	AssessedAt      time.Time                    `json:"assessedAt"`
	SHA256          string                       `json:"sha256"`
}

func NewSegmentScreeningEvidence(subject SegmentScreeningSubject, results []SegmentScreeningResult, airworthiness fillerairworthiness.Decision, assessedAt time.Time) (SegmentScreeningEvidence, error) {
	if err := ValidateSegmentScreeningSubject(subject); err != nil {
		return SegmentScreeningEvidence{}, err
	}
	evidence := SegmentScreeningEvidence{
		SchemaVersion: SegmentScreeningSchemaVersion, ContractVersion: SegmentScreeningContractVersion,
		SubjectSHA256: subject.SHA256, Results: slices.Clone(results), Airworthiness: cloneAirworthinessDecision(airworthiness), AssessedAt: assessedAt.UTC(),
	}
	slices.SortFunc(evidence.Results, func(a, b SegmentScreeningResult) int { return strings.Compare(string(a.Axis), string(b.Axis)) })
	evidence.SHA256 = SegmentScreeningEvidenceSHA256(evidence)
	if err := ValidateSegmentScreeningEvidence(evidence); err != nil {
		return SegmentScreeningEvidence{}, err
	}
	return evidence, nil
}

func cloneAirworthinessDecision(decision fillerairworthiness.Decision) fillerairworthiness.Decision {
	cloned := decision
	cloned.ReasonCodes = slices.Clone(decision.ReasonCodes)
	cloned.ObservedFlags = slices.Clone(decision.ObservedFlags)
	cloned.Triggers = slices.Clone(decision.Triggers)
	cloned.HeldAxes = slices.Clone(decision.HeldAxes)
	cloned.EvidenceSHA256s = slices.Clone(decision.EvidenceSHA256s)
	return cloned
}

func ValidateSegmentScreeningEvidence(evidence SegmentScreeningEvidence) error {
	if evidence.SchemaVersion != SegmentScreeningSchemaVersion || evidence.ContractVersion != SegmentScreeningContractVersion ||
		!isContentHash(evidence.SubjectSHA256) || evidence.AssessedAt.IsZero() ||
		fillerairworthiness.ValidateDecision(evidence.Airworthiness) != nil || evidence.Airworthiness.SubjectSHA256 != evidence.SubjectSHA256 {
		return fmt.Errorf("segment screening identity or interval is invalid")
	}
	want := map[SegmentScreeningAxis]struct{}{ScreenVisualSafety: {}, ScreenSpokenSafety: {}, ScreenWrittenSafety: {}, ScreenPlayback: {}}
	if len(evidence.Results) != len(want) || !slices.IsSortedFunc(evidence.Results, func(a, b SegmentScreeningResult) int { return strings.Compare(string(a.Axis), string(b.Axis)) }) {
		return fmt.Errorf("segment screening must contain four ordered axis results")
	}
	for _, result := range evidence.Results {
		if _, ok := want[result.Axis]; !ok || ValidateSegmentScreeningResult(result) != nil {
			return fmt.Errorf("segment screening contains an invalid axis result")
		}
		delete(want, result.Axis)
	}
	if len(want) != 0 || evidence.SHA256 == "" || evidence.SHA256 != SegmentScreeningEvidenceSHA256(evidence) {
		return fmt.Errorf("segment screening coverage or digest is invalid")
	}
	return nil
}

// ValidateSegmentScreeningResult checks one axis result's closed vocabulary and authority identity.
func ValidateSegmentScreeningResult(result SegmentScreeningResult) error {
	if result.Axis != ScreenVisualSafety && result.Axis != ScreenSpokenSafety && result.Axis != ScreenWrittenSafety && result.Axis != ScreenPlayback {
		return fmt.Errorf("segment screening axis is invalid")
	}
	if result.Outcome != ScreenPass && result.Outcome != ScreenReject && result.Outcome != ScreenHold {
		return fmt.Errorf("segment screening outcome is invalid")
	}
	if !isContentHash(result.AuthoritySHA256) || !ValidScreeningReasonCode(result.ReasonCode) {
		return fmt.Errorf("segment screening authority is invalid")
	}
	return nil
}

func (e SegmentScreeningEvidence) Passes() bool {
	if ValidateSegmentScreeningEvidence(e) != nil {
		return false
	}
	for _, result := range e.Results {
		if result.Outcome != ScreenPass {
			return false
		}
	}
	return e.Airworthiness.Verdict == fillerairworthiness.VerdictPass
}

func SegmentScreeningEvidenceSHA256(evidence SegmentScreeningEvidence) string {
	evidence.SHA256 = ""
	raw, err := json.Marshal(evidence)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// ScreeningBytesSHA256 is the lowercase hex SHA-256 every screening identity is built from.
func ScreeningBytesSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// ValidScreeningReasonCode reports whether value is a closed, opaque screening reason code.
func ValidScreeningReasonCode(value string) bool {
	if len(value) < 1 || len(value) > 96 || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if r < 'a' || r > 'z' {
			if r < '0' || r > '9' {
				if r != '_' && r != '-' && r != '.' {
					return false
				}
			}
		}
	}
	return true
}
