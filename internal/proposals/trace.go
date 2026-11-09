package proposals

import (
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/provision"
)

// DecisionTraceVersion is the one trace schema version.
const DecisionTraceVersion = 1

// DecisionTraceMaxCandidates bounds the candidates one trace carries.
const DecisionTraceMaxCandidates = 64

// RankNoveltyMax is the highest novelty a RankTuple can hold.
const RankNoveltyMax = 2

// DecisionTraceMaxTotal bounds all surfaced and recorded facts. Production can
// surface 576 candidates; the remainder is reserved for adjacent decisions.
const DecisionTraceMaxTotal = 1024

// A DecisionCandidate's Disposition, the Reason it has one, and a trace's Terminal value.
const (
	DispositionSelected          = "selected"
	DispositionAlternate         = "alternate"
	DispositionNotSelected       = "not_selected"
	DispositionRefused           = "refused"
	DispositionValidationDropped = "validation_dropped"
	DispositionTerminal          = "terminal"
	ReasonRetrievalEmpty         = "retrieval_empty"
	ReasonSelectionEmpty         = "selection_empty"
	ReasonNever                  = "never"
	ReasonMalformedID            = "malformed_id"
	ReasonNotSurfaced            = "not_surfaced"
	ReasonNoRelevanceEvidence    = "no_relevance_evidence"
	ReasonValidationDropped      = "validation_dropped"
	ReasonAcquisitionCap         = "acquisition_cap"
	ReasonOverCeiling            = "over_ceiling"
	ReasonBudgetExhausted        = "budget_exhausted"
	ReasonNotSelected            = "not_selected"
	TerminalProviderFailure      = "provider_failure"
	TerminalRetrievalFailure     = "retrieval_failure"
	TerminalReferenceUnreadable  = "reference_unreadable"
	TerminalGenerationFailure    = "generation_failure"
	TerminalMalformedExhausted   = "malformed_exhausted"
	TerminalNamedSetUnproven     = "named_set_unproven"
	TerminalConstraintsConflict  = "constraints_conflict"
	TerminalDateSemanticsUnclear = "date_semantics_unclear"
	TerminalInvalidToolCalls     = "invalid_tool_calls"
	TerminalProviderTimeout      = "provider_timeout"
)

const decisionTraceMaxString = 256

// ValidateDecisionTrace is the shared typed boundary for persisted and evaluated traces.
func ValidateDecisionTrace(trace DecisionTrace) error {
	if trace.Version == 0 && len(trace.Candidates) == 0 && trace.Terminal == "" && trace.SurfacedTotal == 0 && trace.RecordedTotal == 0 {
		return nil // absent trace on pre-v1 proposals
	}
	if trace.Version != DecisionTraceVersion || len(trace.Candidates) > DecisionTraceMaxCandidates || trace.SurfacedTotal < 0 || trace.RecordedTotal < 0 || trace.SurfacedTotal > DecisionTraceMaxTotal || trace.RecordedTotal > DecisionTraceMaxTotal || trace.RecordedTotal < len(trace.Candidates) {
		return fmt.Errorf("invalid version, bounds, or totals")
	}
	if (trace.SurfacedTotal > DecisionTraceMaxCandidates || trace.RecordedTotal > DecisionTraceMaxCandidates) && !trace.Truncated {
		return fmt.Errorf("unbounded surfaced or recorded total")
	}
	if trace.Terminal != "" && !knownTerminal(trace.Terminal) {
		return fmt.Errorf("unknown terminal %q", trace.Terminal)
	}
	for _, c := range trace.Candidates {
		for name, value := range map[string]string{"key": c.Key, "name": c.Name, "source": c.Source, "ownership": c.Ownership, "disposition": c.Disposition, "reason": c.Reason, "tieKey": c.Rank.TieKey} {
			if len(value) > decisionTraceMaxString || strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("invalid %s", name)
			}
		}
		if c.Disposition == DispositionValidationDropped && c.Reason == ReasonMalformedID && c.Key == "" && c.Name == "" && c.Source == "" && c.Ownership == "" && c.Rank == (RankTuple{}) && c.Constraints == (ConstraintMatches{}) {
			continue
		}
		if c.Disposition == DispositionValidationDropped && c.Reason == ReasonNotSurfaced && c.Key != "" && c.Name == "" && c.Source == "" && c.Ownership == "" && c.Rank == (RankTuple{}) && c.Constraints == (ConstraintMatches{}) {
			if _, _, _, ok := provision.ParseKey(provision.Key(c.Key)); !ok {
				return fmt.Errorf("invalid canonical key")
			}
			continue
		}
		if c.Ownership != "library" && c.Ownership != "acquisition" {
			return fmt.Errorf("invalid ownership")
		}
		if c.Key == "" || c.Rank.TieKey == "" || c.Rank.TieKey != c.Key {
			return fmt.Errorf("invalid canonical key or tie key")
		}
		if _, _, _, ok := provision.ParseKey(provision.Key(c.Key)); !ok {
			return fmt.Errorf("invalid canonical key")
		}
		if c.Rank.Relevance < 0 || c.Rank.Relevance > 1<<31-1 || c.Rank.Preference < -5 || c.Rank.Preference > 3 || c.Rank.Novelty < 0 || c.Rank.Novelty > RankNoveltyMax {
			return fmt.Errorf("rank tuple outside representation bounds")
		}
		if (c.Rank.Relevance > 0) != c.Constraints.Any() {
			return fmt.Errorf("constraint matches do not explain relevance")
		}
		if !validDispositionReason(c.Disposition, c.Reason) {
			return fmt.Errorf("invalid disposition/reason")
		}
	}
	return nil
}

func knownTerminal(value string) bool {
	switch value {
	case ReasonRetrievalEmpty, ReasonSelectionEmpty, ReasonBudgetExhausted, TerminalProviderFailure, TerminalRetrievalFailure, TerminalReferenceUnreadable, TerminalGenerationFailure, TerminalMalformedExhausted, TerminalNamedSetUnproven, TerminalConstraintsConflict, TerminalDateSemanticsUnclear, TerminalInvalidToolCalls, TerminalProviderTimeout:
		return true
	default:
		return false
	}
}

func validDispositionReason(disposition, reason string) bool {
	if disposition == DispositionTerminal {
		return knownTerminal(reason)
	}
	allowed := map[string]map[string]bool{
		DispositionSelected:          {"selected": true},
		DispositionAlternate:         {ReasonAcquisitionCap: true},
		DispositionNotSelected:       {ReasonNotSelected: true, ReasonNever: true, ReasonOverCeiling: true},
		DispositionRefused:           {ReasonOverCeiling: true},
		DispositionValidationDropped: {ReasonMalformedID: true, ReasonNotSurfaced: true, ReasonNoRelevanceEvidence: true, ReasonValidationDropped: true},
	}
	return allowed[disposition][reason]
}

// DecisionTrace is bounded, immutable evidence from one original proposal run.
// Callers receive value copies and must not use it as current channel evidence.
type DecisionTrace struct {
	Version                 int                 `json:"version"`
	Candidates              []DecisionCandidate `json:"candidates"`
	SurfacedTotal           int                 `json:"surfacedTotal"`
	RecordedTotal           int                 `json:"recordedTotal"`
	Truncated               bool                `json:"truncated"`
	WindowsCompleted        int                 `json:"windowsCompleted,omitempty"`
	SourceQueriesDispatched int                 `json:"sourceQueriesDispatched,omitempty"`
	Terminal                string              `json:"terminal,omitempty"`
}

// Clone returns a copy that shares no candidate storage with the receiver.
func (t DecisionTrace) Clone() DecisionTrace {
	t.Candidates = append([]DecisionCandidate(nil), t.Candidates...)
	return t
}

type DecisionCandidate struct {
	Key         string            `json:"key"`
	Name        string            `json:"name,omitempty"`
	Source      string            `json:"source,omitempty"`
	Ownership   string            `json:"ownership"`
	Rank        RankTuple         `json:"rank,omitempty"`
	Constraints ConstraintMatches `json:"constraints,omitempty"`
	Disposition string            `json:"disposition"`
	Reason      string            `json:"reason"`
}

// ConstraintMatches identifies which safe intent categories contributed at
// least one grounded term to Relevance. It never copies caller-authored terms.
type ConstraintMatches struct {
	Request     bool `json:"request,omitempty"`
	Tone        bool `json:"tone,omitempty"`
	Era         bool `json:"era,omitempty"`
	MustInclude bool `json:"mustInclude,omitempty"`
	MustExclude bool `json:"mustExclude,omitempty"`
	Refine      bool `json:"refine,omitempty"`
}

// Any reports whether at least one intent category contributed a grounded term.
func (m ConstraintMatches) Any() bool {
	return m.Request || m.Tone || m.Era || m.MustInclude || m.MustExclude || m.Refine
}

// RankTuple is the exact lexicographic ordering tuple. It is not a scalar score.
type RankTuple struct {
	Relevance  int    `json:"relevance"`
	Preference int    `json:"preference"`
	Novelty    int    `json:"novelty"`
	TieKey     string `json:"tieKey"`
}
