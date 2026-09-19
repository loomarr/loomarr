// Package fillerenrichment owns progressive descriptive understanding for filler clips.
//
// Its interface is deliberately independent from readiness and admission: callers provide current
// per-axis state and grounded results, and this package owns evidence precedence and idempotency.
package fillerenrichment

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type Axis string

const (
	AxisKind         Axis = "kind"
	AxisEra          Axis = "era"
	AxisAudience     Axis = "audience"
	AxisBrand        Axis = "brand"
	AxisGeography    Axis = "geography"
	AxisLanguage     Axis = "language"
	AxisProduct      Axis = "product"
	AxisFormat       Axis = "format"
	AxisSeasonal     Axis = "seasonal"
	AxisAudienceCue  Axis = "audience-cue"
	AxisPresentation Axis = "presentation"
)

var axes = []Axis{
	AxisKind, AxisEra, AxisAudience, AxisBrand, AxisGeography, AxisLanguage,
	AxisProduct, AxisFormat, AxisSeasonal, AxisAudienceCue, AxisPresentation,
}

func Axes() []Axis { return append([]Axis(nil), axes...) }

func (a Axis) Valid() bool {
	for _, candidate := range axes {
		if a == candidate {
			return true
		}
	}
	return false
}

type Status string

const (
	StatusMissing     Status = "missing"
	StatusComplete    Status = "complete"
	StatusUnsupported Status = "unsupported"
	StatusStale       Status = "stale"
)

func (s Status) Valid() bool {
	switch s {
	case StatusMissing, StatusComplete, StatusUnsupported, StatusStale:
		return true
	default:
		return false
	}
}

// EvidenceKind identifies where an enrichment answer came from. Rank is derived from this closed
// vocabulary rather than supplied by a provider, so a model cannot promote itself above item facts.
type EvidenceKind string

const (
	EvidenceInference     EvidenceKind = "inference"
	EvidenceSourceDefault EvidenceKind = "source_default"
	EvidenceTrustedMap    EvidenceKind = "trusted_mapping"
	EvidenceContent       EvidenceKind = "content_observation"
	EvidenceItem          EvidenceKind = "item_metadata"
	EvidenceOperator      EvidenceKind = "operator"
)

type EvidenceRank uint8

const (
	RankInference EvidenceRank = iota + 1
	RankSourceDefault
	RankTrustedMap
	RankContent
	RankItem
	RankOperator
)

func (k EvidenceKind) Rank() (EvidenceRank, bool) {
	switch k {
	case EvidenceInference:
		return RankInference, true
	case EvidenceSourceDefault:
		return RankSourceDefault, true
	case EvidenceTrustedMap:
		return RankTrustedMap, true
	case EvidenceContent:
		return RankContent, true
	case EvidenceItem:
		return RankItem, true
	case EvidenceOperator:
		return RankOperator, true
	default:
		return 0, false
	}
}

type Geography struct {
	Scope   string `json:"scope,omitempty"`
	Country string `json:"country,omitempty"`
	Market  string `json:"market,omitempty"`
	Network string `json:"network,omitempty"`
	Station string `json:"station,omitempty"`
	AirDate string `json:"airDate,omitempty"`
}

// Value is the closed value envelope shared by all enrichment axes. Validate enforces which member
// each axis may use, keeping the persisted JSON extensible without making callers interpret it.
type Value struct {
	Text      string    `json:"text,omitempty"`
	Year      int       `json:"year,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Geography Geography `json:"geography,omitempty"`
}

func (v Value) empty() bool {
	return v.Text == "" && v.Year == 0 && len(v.Tags) == 0 && v.Geography == (Geography{})
}

func (v Value) canonical() Value {
	v.Text = strings.TrimSpace(v.Text)
	v.Geography.Scope = strings.TrimSpace(v.Geography.Scope)
	v.Geography.Country = strings.ToUpper(strings.TrimSpace(v.Geography.Country))
	v.Geography.Market = strings.TrimSpace(v.Geography.Market)
	v.Geography.Network = strings.TrimSpace(v.Geography.Network)
	v.Geography.Station = strings.TrimSpace(v.Geography.Station)
	v.Geography.AirDate = strings.TrimSpace(v.Geography.AirDate)
	if len(v.Tags) > 0 {
		seen := make(map[string]bool, len(v.Tags))
		tags := make([]string, 0, len(v.Tags))
		for _, raw := range v.Tags {
			tag := strings.TrimSpace(raw)
			if tag != "" && !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
		sort.Strings(tags)
		v.Tags = tags
	}
	return v
}

type Evidence struct {
	Kind            EvidenceKind `json:"kind"`
	Reference       string       `json:"reference"`
	Confidence      int          `json:"confidence"`
	Producer        string       `json:"producer"`
	ProducerVersion string       `json:"producerVersion"`
	TaxonomyVersion string       `json:"taxonomyVersion,omitempty"`
	ObservedAt      time.Time    `json:"observedAt"`
}

type State struct {
	ClipHash string   `json:"clipHash"`
	Axis     Axis     `json:"axis"`
	Status   Status   `json:"status"`
	Value    Value    `json:"value"`
	Evidence Evidence `json:"evidence"`
}

// Pass is one bounded producer run for one exact clip. Stores commit its accepted axis updates and
// completion stamp atomically so a crash cannot make half a pass look complete.
type Pass struct {
	ClipHash        string
	Producer        string
	ProducerVersion string
	TaxonomyVersion string
	CompletedAt     time.Time
	States          []State
	Observation     *MediaObservation
}

// MediaObservation is the raw, bounded signal produced by an optional capability. It commits in
// the same transaction as the pass stamp and accepted axis facts so a crash cannot leave expensive
// media work persisted but absent from the progressive evidence model.
type MediaObservation struct {
	Transcript *string
	Vision     *VisionObservation
}

type VisionObservation struct {
	VisibleText  string
	SuggestedEra int
}

func (p Pass) Validate() error {
	if strings.TrimSpace(p.ClipHash) == "" || strings.TrimSpace(p.Producer) == "" ||
		strings.TrimSpace(p.ProducerVersion) == "" || p.CompletedAt.IsZero() {
		return fmt.Errorf("%w: pass identity and completion time are required", ErrInvalidState)
	}
	seen := make(map[Axis]bool, len(p.States))
	if p.Observation != nil && p.Observation.Transcript != nil && strings.TrimSpace(*p.Observation.Transcript) == "" {
		return fmt.Errorf("%w: a transcript observation must record speech or the wordless sentinel", ErrInvalidState)
	}
	for _, state := range p.States {
		if state.Status == StatusMissing {
			return fmt.Errorf("%w: a completed pass cannot persist a missing state", ErrInvalidState)
		}
		if state.ClipHash != p.ClipHash || state.Evidence.Producer != p.Producer ||
			state.Evidence.ProducerVersion != p.ProducerVersion {
			return fmt.Errorf("%w: pass state identity does not match the pass", ErrInvalidState)
		}
		if seen[state.Axis] {
			return fmt.Errorf("%w: pass repeats axis %q", ErrInvalidState, state.Axis)
		}
		seen[state.Axis] = true
		if state.Evidence.TaxonomyVersion != "" && state.Evidence.TaxonomyVersion != p.TaxonomyVersion {
			return fmt.Errorf("%w: state taxonomy version does not match the pass", ErrInvalidState)
		}
		if err := state.Validate(); err != nil {
			return err
		}
	}
	return nil
}

var ErrInvalidState = errors.New("invalid filler enrichment state")

func (s State) Validate() error {
	if strings.TrimSpace(s.ClipHash) == "" || !s.Axis.Valid() || !s.Status.Valid() {
		return fmt.Errorf("%w: clip, axis, and status are required", ErrInvalidState)
	}
	if s.Status == StatusMissing {
		if !s.Value.empty() || s.Evidence != (Evidence{}) {
			return fmt.Errorf("%w: missing state cannot carry value or evidence", ErrInvalidState)
		}
		return nil
	}
	if _, ok := s.Evidence.Kind.Rank(); !ok || strings.TrimSpace(s.Evidence.Reference) == "" ||
		strings.TrimSpace(s.Evidence.Producer) == "" || strings.TrimSpace(s.Evidence.ProducerVersion) == "" ||
		s.Evidence.Confidence < 0 || s.Evidence.Confidence > 100 || s.Evidence.ObservedAt.IsZero() {
		return fmt.Errorf("%w: non-missing state requires valid evidence", ErrInvalidState)
	}
	if err := validateValue(s.Axis, s.Value.canonical()); err != nil {
		return err
	}
	if s.Status == StatusUnsupported && !s.Value.empty() {
		return fmt.Errorf("%w: unsupported state cannot carry a value", ErrInvalidState)
	}
	return nil
}

func validateValue(axis Axis, value Value) error {
	switch axis {
	case AxisKind:
		switch value.Text {
		case "", "commercial", "bumper", "station_id", "psa", "trailer", "interstitial":
		default:
			return fmt.Errorf("%w: kind is outside the supported vocabulary", ErrInvalidState)
		}
	case AxisEra:
		if value.Year != 0 && (value.Year < 1930 || value.Year > 2100) {
			return fmt.Errorf("%w: era year is outside the supported range", ErrInvalidState)
		}
	case AxisGeography:
		if value.Geography.Country != "" && len(value.Geography.Country) != 2 {
			return fmt.Errorf("%w: geography country must be ISO alpha-2", ErrInvalidState)
		}
		if value.Geography.AirDate != "" {
			if _, err := time.Parse(time.DateOnly, value.Geography.AirDate); err != nil {
				return fmt.Errorf("%w: geography air date must use YYYY-MM-DD", ErrInvalidState)
			}
		}
	case AxisProduct, AxisFormat, AxisSeasonal, AxisAudienceCue, AxisPresentation:
		// An empty set is a valid completed observation: the pass checked and found no applicable tag.
	case AxisAudience, AxisBrand, AxisLanguage:
		// An empty string is likewise an honest completed observation.
	default:
		return fmt.Errorf("%w: unsupported axis %q", ErrInvalidState, axis)
	}
	return nil
}

// Apply is the module's write-policy interface. It accepts a candidate only when its evidence is
// stronger than the current answer, or equally ranked and more confident. Replaying the same result
// is an idempotent no-op. A stale state may be refreshed at the same rank even when confidence ties.
func Apply(current, candidate State) (State, bool, error) {
	candidate.Value = candidate.Value.canonical()
	if err := candidate.Validate(); err != nil {
		return State{}, false, err
	}
	if current.ClipHash == "" {
		return candidate, true, nil
	}
	current.Value = current.Value.canonical()
	if err := current.Validate(); err != nil {
		return State{}, false, err
	}
	if current.ClipHash != candidate.ClipHash || current.Axis != candidate.Axis {
		return State{}, false, fmt.Errorf("%w: current and candidate address different axes", ErrInvalidState)
	}
	if reflect.DeepEqual(current, candidate) {
		return current, false, nil
	}
	currentRank, _ := current.Evidence.Kind.Rank()
	candidateRank, _ := candidate.Evidence.Kind.Rank()
	refresh := candidateRank == currentRank && current.Evidence.Kind != EvidenceOperator &&
		((current.Evidence.Producer == candidate.Evidence.Producer &&
			(current.Evidence.ProducerVersion != candidate.Evidence.ProducerVersion ||
				current.Evidence.TaxonomyVersion != candidate.Evidence.TaxonomyVersion)) ||
			(current.Evidence.Kind == EvidenceInference && candidate.Evidence.Kind == EvidenceInference &&
				strings.HasPrefix(current.Evidence.Producer, "text-model:") &&
				strings.HasPrefix(candidate.Evidence.Producer, "text-model:") &&
				current.Evidence.Producer != candidate.Evidence.Producer)) &&
		(!candidate.Value.empty() || current.Value.empty())
	operatorCorrection := current.Evidence.Kind == EvidenceOperator && candidate.Evidence.Kind == EvidenceOperator &&
		candidate.Evidence.ObservedAt.After(current.Evidence.ObservedAt)
	if current.Status == StatusMissing || candidateRank > currentRank ||
		(current.Evidence.Kind != EvidenceOperator && current.Value.empty() && !candidate.Value.empty()) ||
		(candidateRank == currentRank && candidate.Evidence.Confidence > current.Evidence.Confidence) ||
		(candidateRank == currentRank && current.Status == StatusStale && (!candidate.Value.empty() || current.Value.empty())) ||
		refresh || operatorCorrection {
		return candidate, true, nil
	}
	return current, false, nil
}

func EncodeValue(value Value) (string, error) {
	raw, err := json.Marshal(value.canonical())
	return string(raw), err
}

func DecodeValue(raw string) (Value, error) {
	var value Value
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return Value{}, err
	}
	return value.canonical(), nil
}
