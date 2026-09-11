// Package fillercandidatepool owns the immutable replacement-candidate-pool
// contract. It contains no authority-building policy; composition lives in the
// build subpackage and consumers may only search validated eligible records.
package fillercandidatepool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
)

const (
	SchemaVersion   = 1
	ContractVersion = "filler-replacement-candidate-pool-v1"

	KindStandaloneAnchor = "standalone_anchor"
	KindProgrammeParent  = "programme_parent"

	DispositionEligible = "eligible"
	DispositionHeld     = "held"

	TransportHTTPS = "https"
	TransportLocal = "local"
)

const (
	HoldRights                   = "rights_held"
	HoldQuarantineMissing        = "quarantine_missing"
	HoldQuarantine               = "quarantine_held"
	HoldSourceBytes              = "source_bytes_invalid"
	HoldTechnical                = "technical_quality_held"
	HoldSoundtrackSilent         = "soundtrack_intentionally_silent"
	HoldSoundtrackUnknown        = "soundtrack_unknown"
	HoldSoundtrackDecodedAudio   = "soundtrack_decoded_audio_missing"
	HoldSuitability              = "suitability_held"
	HoldSemanticUnit             = "semantic_unit_held"
	HoldSemanticRole             = "semantic_role_held"
	HoldTransition               = "transition_evidence_missing"
	HoldFamily                   = "duplicate_family_held"
	HoldPriorSource              = "prior_source_exposure"
	HoldPriorFamily              = "prior_family_exposure"
	HoldPriorProgrammeProvenance = "prior_programme_provenance"
)

var knownHoldReasons = []string{
	HoldFamily, HoldPriorFamily, HoldPriorProgrammeProvenance, HoldPriorSource,
	HoldQuarantine, HoldQuarantineMissing, HoldRights, HoldSemanticRole,
	HoldSemanticUnit, HoldSoundtrackDecodedAudio, HoldSoundtrackSilent,
	HoldSoundtrackUnknown, HoldSourceBytes, HoldSuitability, HoldTechnical,
	HoldTransition,
}

// Pool is the sole candidate authority accepted by replacement planning.
type Pool struct {
	SchemaVersion              int         `json:"schemaVersion"`
	ContractVersion            string      `json:"contractVersion"`
	GeneratedAt                time.Time   `json:"generatedAt"`
	Inputs                     []Input     `json:"inputs"`
	PriorExposure              Exposure    `json:"priorExposure"`
	Candidates                 []Candidate `json:"candidates"`
	TrainingAllowed            bool        `json:"trainingAllowed"`
	CatalogIngestionAllowed    bool        `json:"catalogIngestionAllowed"`
	SchedulingAllowed          bool        `json:"schedulingAllowed"`
	CertificationResultAllowed bool        `json:"certificationResultAllowed"`
	ProductionAdmissionAllowed bool        `json:"productionAdmissionAllowed"`
}

type Input struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Exposure struct {
	SourceSHA256        []string              `json:"sourceSha256"`
	FamilyIDs           []string              `json:"familyIds"`
	ProgrammeProvenance []ProgrammeProvenance `json:"programmeProvenance"`
}

type ProgrammeProvenance struct {
	Authority string `json:"authority"`
	Reference string `json:"reference"`
}

type Candidate struct {
	CaseID      string      `json:"caseId"`
	Kind        string      `json:"kind"`
	Disposition string      `json:"disposition"`
	HoldReasons []string    `json:"holdReasons"`
	Source      Source      `json:"source"`
	FamilyID    string      `json:"familyId"`
	Role        string      `json:"role,omitempty"`
	Transition  *Transition `json:"transition,omitempty"`
}

type Source struct {
	ID                    string    `json:"id"`
	Path                  string    `json:"path"`
	SHA256                string    `json:"sha256"`
	Bytes                 int64     `json:"bytes"`
	DurationMS            int64     `json:"durationMs"`
	Transport             string    `json:"transport"`
	Authority             string    `json:"authority"`
	ItemID                string    `json:"itemId"`
	ItemURL               string    `json:"itemUrl"`
	MediaURL              string    `json:"mediaUrl,omitempty"`
	MetadataSHA256        string    `json:"metadataSha256"`
	MetadataRetrievedAt   time.Time `json:"metadataRetrievedAt"`
	SoundtrackStatus      string    `json:"soundtrackStatus"`
	SoundtrackEvidence    string    `json:"soundtrackEvidence"`
	SoundtrackEvidenceSHA string    `json:"soundtrackEvidenceSha256"`
}

type Transition struct {
	EvidenceAlias string `json:"evidenceAlias"`
	Head          Edge   `json:"head"`
	Tail          Edge   `json:"tail"`
}

type Edge struct {
	StartMS       int64      `json:"startMs"`
	EndMS         int64      `json:"endMs"`
	Black         []Interval `json:"black,omitempty"`
	Silence       []Interval `json:"silence,omitempty"`
	RMSMilliDBFS  int64      `json:"rmsMilliDbfs"`
	PeakMilliDBFS int64      `json:"peakMilliDbfs"`
}

type Interval struct {
	StartMS int64 `json:"startMs"`
	EndMS   int64 `json:"endMs"`
}

// Decode accepts one strict current-schema JSON value.
func Decode(raw []byte) (Pool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var pool Pool
	if err := decoder.Decode(&pool); err != nil {
		return Pool{}, fmt.Errorf("decode replacement candidate pool: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Pool{}, errors.New("decode replacement candidate pool: trailing JSON value")
	}
	if err := Validate(pool); err != nil {
		return Pool{}, fmt.Errorf("validate replacement candidate pool: %w", err)
	}
	return pool, nil
}

// Validate checks the canonical self-contained contract. Authority builders
// remain responsible for reproducing the external evidence represented here.
func Validate(pool Pool) error {
	if pool.SchemaVersion != SchemaVersion || pool.ContractVersion != ContractVersion ||
		pool.GeneratedAt.IsZero() || pool.GeneratedAt.Location() != time.UTC {
		return errors.New("pool identity or generation time is invalid")
	}
	if pool.TrainingAllowed || pool.CatalogIngestionAllowed || pool.SchedulingAllowed ||
		pool.CertificationResultAllowed || pool.ProductionAdmissionAllowed {
		return errors.New("pool grants forbidden downstream authority")
	}
	if err := validateInputs(pool.Inputs); err != nil {
		return err
	}
	if err := ValidateExposure(pool.PriorExposure); err != nil {
		return fmt.Errorf("prior exposure: %w", err)
	}
	if len(pool.Candidates) == 0 || !slices.IsSortedFunc(pool.Candidates, func(a, b Candidate) int { return strings.Compare(a.CaseID, b.CaseID) }) {
		return errors.New("candidate set is empty or unordered")
	}
	seen := make(map[string]struct{}, len(pool.Candidates))
	seenSourceIDs := make(map[string]struct{}, len(pool.Candidates))
	eligibleSources := make(map[string]struct{}, len(pool.Candidates))
	eligibleFamilies := make(map[string]struct{}, len(pool.Candidates))
	eligibleProgrammeProvenance := make(map[string]struct{}, len(pool.Candidates))
	priorSources := stringSet(pool.PriorExposure.SourceSHA256)
	priorFamilies := stringSet(pool.PriorExposure.FamilyIDs)
	priorProgrammeProvenance := make(map[string]struct{}, len(pool.PriorExposure.ProgrammeProvenance))
	for _, provenance := range pool.PriorExposure.ProgrammeProvenance {
		priorProgrammeProvenance[provenance.Authority+"\x00"+provenance.Reference] = struct{}{}
	}
	for _, candidate := range pool.Candidates {
		if _, duplicate := seen[candidate.CaseID]; duplicate {
			return fmt.Errorf("candidate set repeats %q", candidate.CaseID)
		}
		seen[candidate.CaseID] = struct{}{}
		if err := validateCandidate(candidate); err != nil {
			return fmt.Errorf("candidate %q: %w", candidate.CaseID, err)
		}
		if candidate.Source.ID != "" {
			if _, duplicate := seenSourceIDs[candidate.Source.ID]; duplicate {
				return fmt.Errorf("candidate set repeats source id %q", candidate.Source.ID)
			}
			seenSourceIDs[candidate.Source.ID] = struct{}{}
		}
		if candidate.Disposition != DispositionEligible {
			continue
		}
		if _, exposed := priorSources[candidate.Source.SHA256]; exposed {
			return errors.New("eligible candidate repeats prior source bytes")
		}
		if _, exposed := priorFamilies[candidate.FamilyID]; exposed {
			return errors.New("eligible candidate repeats a prior family")
		}
		if _, duplicate := eligibleSources[candidate.Source.SHA256]; duplicate {
			return errors.New("eligible candidates repeat source bytes")
		}
		if _, duplicate := eligibleFamilies[candidate.FamilyID]; duplicate {
			return errors.New("eligible candidates repeat a duplicate family")
		}
		eligibleSources[candidate.Source.SHA256], eligibleFamilies[candidate.FamilyID] = struct{}{}, struct{}{}
		if candidate.Kind == KindProgrammeParent {
			key := candidate.Source.Authority + "\x00" + candidate.Source.ItemURL
			if _, exposed := priorProgrammeProvenance[key]; exposed {
				return errors.New("eligible programme parent repeats prior provenance")
			}
			if _, duplicate := eligibleProgrammeProvenance[key]; duplicate {
				return errors.New("eligible programme parents repeat provenance")
			}
			eligibleProgrammeProvenance[key] = struct{}{}
		}
	}
	return nil
}

func validateInputs(inputs []Input) error {
	if len(inputs) == 0 || !slices.IsSortedFunc(inputs, func(a, b Input) int { return strings.Compare(a.Name, b.Name) }) {
		return errors.New("input authority is empty or unordered")
	}
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.Name == "" || input.Name != strings.TrimSpace(input.Name) || !SHA256(input.SHA256) {
			return errors.New("input authority is invalid")
		}
		if _, duplicate := seen[input.Name]; duplicate {
			return fmt.Errorf("input authority repeats %q", input.Name)
		}
		seen[input.Name] = struct{}{}
	}
	return nil
}

// ValidateExposure checks the canonical cumulative burned-challenge set.
func ValidateExposure(exposure Exposure) error {
	if exposure.SourceSHA256 == nil || exposure.FamilyIDs == nil || exposure.ProgrammeProvenance == nil ||
		!slices.IsSorted(exposure.SourceSHA256) || !slices.IsSorted(exposure.FamilyIDs) ||
		!slices.IsSortedFunc(exposure.ProgrammeProvenance, compareProvenance) {
		return errors.New("set is absent or unordered")
	}
	if hasAdjacentDuplicate(exposure.SourceSHA256) || hasAdjacentDuplicate(exposure.FamilyIDs) {
		return errors.New("set contains duplicates")
	}
	for _, digest := range exposure.SourceSHA256 {
		if !SHA256(digest) {
			return errors.New("source digest is invalid")
		}
	}
	for _, family := range exposure.FamilyIDs {
		if family == "" || family != strings.TrimSpace(family) {
			return errors.New("family identity is invalid")
		}
	}
	previous := ""
	for _, provenance := range exposure.ProgrammeProvenance {
		key := provenance.Authority + "\x00" + provenance.Reference
		if provenance.Authority == "" || provenance.Reference == "" ||
			provenance.Authority != strings.TrimSpace(provenance.Authority) ||
			provenance.Reference != strings.TrimSpace(provenance.Reference) || key == previous {
			return errors.New("programme provenance is invalid or duplicated")
		}
		previous = key
	}
	return nil
}

func validateCandidate(candidate Candidate) error {
	if candidate.CaseID == "" || candidate.CaseID != strings.TrimSpace(candidate.CaseID) ||
		(candidate.Kind != KindStandaloneAnchor && candidate.Kind != KindProgrammeParent) {
		return errors.New("identity or kind is invalid")
	}
	if candidate.Disposition != DispositionEligible && candidate.Disposition != DispositionHeld {
		return errors.New("disposition is invalid")
	}
	if candidate.HoldReasons == nil || !slices.IsSorted(candidate.HoldReasons) || hasAdjacentDuplicate(candidate.HoldReasons) {
		return errors.New("hold reasons are absent, unordered, or duplicated")
	}
	for _, reason := range candidate.HoldReasons {
		if !slices.Contains(knownHoldReasons, reason) {
			return fmt.Errorf("hold reason %q is unknown", reason)
		}
	}
	if (candidate.Disposition == DispositionEligible) != (len(candidate.HoldReasons) == 0) {
		return errors.New("disposition does not match hold reasons")
	}
	if candidate.FamilyID == "" || candidate.FamilyID != strings.TrimSpace(candidate.FamilyID) {
		return errors.New("family identity is invalid")
	}
	if err := validateSource(candidate.CaseID, candidate.Source, candidate.Disposition == DispositionEligible); err != nil {
		return err
	}
	if candidate.Kind == KindProgrammeParent {
		if candidate.Role != "" || candidate.Transition != nil {
			return errors.New("programme parent asserts standalone authority")
		}
		return nil
	}
	if candidate.Disposition == DispositionEligible {
		if !knownRole(candidate.Role) || candidate.Transition == nil {
			return errors.New("eligible standalone lacks role or transition authority")
		}
	}
	if candidate.Role != "" && !knownRole(candidate.Role) {
		return errors.New("standalone role is invalid")
	}
	if candidate.Transition != nil {
		if err := validateTransition(*candidate.Transition, candidate.Source.DurationMS); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(caseID string, source Source, eligible bool) error {
	if (source.Transport != TransportHTTPS && source.Transport != TransportLocal) ||
		source.Authority == "" || source.ItemID == "" || caseID != source.Authority+"/"+source.ItemID ||
		!canonicalHTTPS(source.ItemURL) || !SHA256(source.MetadataSHA256) ||
		source.MetadataRetrievedAt.IsZero() || source.MetadataRetrievedAt.Location() != time.UTC ||
		!knownSoundtrack(source.SoundtrackStatus) || source.SoundtrackEvidence == "" || !SHA256(source.SoundtrackEvidenceSHA) {
		return errors.New("source authority is incomplete or invalid")
	}
	if eligible && (source.ID == "" || source.ID != strings.TrimSpace(source.ID) || source.Path == "" ||
		path.Clean(source.Path) != source.Path || path.IsAbs(source.Path) || source.Path == ".." || strings.HasPrefix(source.Path, "../") ||
		!SHA256(source.SHA256) || source.Bytes <= 0 || source.DurationMS <= 0 || source.SoundtrackStatus != "present_expected") {
		return errors.New("eligible source bytes or soundtrack authority is incomplete")
	}
	if !eligible && (source.ID != "" && source.ID != strings.TrimSpace(source.ID) || source.Path != "" && (path.Clean(source.Path) != source.Path || path.IsAbs(source.Path) || source.Path == ".." || strings.HasPrefix(source.Path, "../")) || source.SHA256 != "" && !SHA256(source.SHA256) || source.Bytes < 0 || source.DurationMS < 0) {
		return errors.New("held source contains malformed optional byte authority")
	}
	if source.Transport == TransportHTTPS && source.MediaURL != "" && !canonicalHTTPS(source.MediaURL) {
		return errors.New("remote source media URL is invalid")
	}
	if eligible && source.Transport == TransportHTTPS && source.MediaURL == "" {
		return errors.New("eligible remote source media URL is missing")
	}
	if source.Transport == TransportLocal && source.MediaURL != "" {
		return errors.New("local source carries a remote media URL")
	}
	return nil
}

func knownSoundtrack(value string) bool {
	return value == "present_expected" || value == "intentionally_silent" || value == "unknown"
}

func validateTransition(transition Transition, durationMS int64) error {
	if transition.EvidenceAlias == "" || transition.EvidenceAlias != strings.TrimSpace(transition.EvidenceAlias) {
		return errors.New("transition alias is invalid")
	}
	for _, edge := range []Edge{transition.Head, transition.Tail} {
		if edge.StartMS < 0 || edge.EndMS <= edge.StartMS || edge.EndMS > durationMS ||
			!validIntervals(edge.Black, edge.StartMS, edge.EndMS) || !validIntervals(edge.Silence, edge.StartMS, edge.EndMS) {
			return errors.New("transition edge is invalid")
		}
	}
	return nil
}

func validIntervals(values []Interval, minimum, maximum int64) bool {
	previous := minimum
	for _, value := range values {
		if value.StartMS < minimum || value.EndMS <= value.StartMS || value.EndMS > maximum || value.StartMS < previous {
			return false
		}
		previous = value.EndMS
	}
	return true
}

func knownRole(role string) bool {
	switch role {
	case "commercial", "promo", "bumper", "psa", "station_id", "trailer", "interstitial":
		return true
	default:
		return false
	}
}

func canonicalHTTPS(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && parsed.String() == raw
}

func compareProvenance(a, b ProgrammeProvenance) int {
	return strings.Compare(a.Authority+"\x00"+a.Reference, b.Authority+"\x00"+b.Reference)
}

func hasAdjacentDuplicate(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func SHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
