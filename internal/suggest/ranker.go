package suggest

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// FeedbackSignal is the ranker's store-independent view of one effective
// explicit household signal. Passive playback never enters this interface.
type FeedbackSignal struct {
	Target provision.Key
	Action FeedbackAction
}

type FeedbackAction string

const (
	FeedbackKeep     FeedbackAction = "keep"
	FeedbackLess     FeedbackAction = "less"
	FeedbackNever    FeedbackAction = "never"
	FeedbackSurprise FeedbackAction = "surprise"
)

type rankedCandidate struct {
	candidate  catalog.Candidate
	key        provision.Key
	relevance  int
	preference int
	novelty    int
}

// DecisionTrace is bounded, immutable evidence from one original proposal run.
// Callers receive value copies and must not use it as current channel evidence.
type DecisionTrace struct {
	Version       int                 `json:"version"`
	Candidates    []DecisionCandidate `json:"candidates"`
	SurfacedTotal int                 `json:"surfacedTotal"`
	RecordedTotal int                 `json:"recordedTotal"`
	Truncated     bool                `json:"truncated"`
	Terminal      string              `json:"terminal,omitempty"`
}

type DecisionCandidate struct {
	Key         string    `json:"key"`
	Name        string    `json:"name,omitempty"`
	Source      string    `json:"source,omitempty"`
	Ownership   string    `json:"ownership"`
	Rank        RankTuple `json:"rank,omitempty"`
	Disposition string    `json:"disposition"`
	Reason      string    `json:"reason"`
}

// RankTuple is the exact lexicographic ordering tuple. It is not a scalar score.
type RankTuple struct {
	Relevance  int    `json:"relevance"`
	Preference int    `json:"preference"`
	Novelty    int    `json:"novelty"`
	TieKey     string `json:"tieKey"`
}

const DecisionTraceVersion = 1
const DecisionTraceMaxCandidates = 64

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
	ReasonValidationDropped      = "validation_dropped"
	ReasonAcquisitionCap         = "acquisition_cap"
	ReasonOverCeiling            = "over_ceiling"
	ReasonBudgetExhausted        = "budget_exhausted"
	ReasonNotSelected            = "not_selected"
	TerminalProviderFailure      = "provider_failure"
	TerminalGenerationFailure    = "generation_failure"
	TerminalMalformedExhausted   = "malformed_exhausted"
)

func (t DecisionTrace) Clone() DecisionTrace {
	t.Candidates = append([]DecisionCandidate(nil), t.Candidates...)
	return t
}

type RankedCandidates struct {
	Candidates []catalog.Candidate
	Trace      DecisionTrace
}

func RankGroundedCandidatesWithTrace(intent string, candidates []catalog.Candidate, signals []FeedbackSignal) RankedCandidates {
	// The legacy ordering function remains a compatibility adapter; this seam publishes
	// the same tuple used by the comparator, without recomputing it downstream.
	ranked := rankDetailed(intent, candidates, signals)
	result := make([]catalog.Candidate, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.candidate)
	}
	trace := DecisionTrace{Version: DecisionTraceVersion, SurfacedTotal: len(candidates), RecordedTotal: len(ranked), Candidates: make([]DecisionCandidate, 0, min(len(ranked), DecisionTraceMaxCandidates))}
	for _, item := range ranked {
		if len(trace.Candidates) >= DecisionTraceMaxCandidates {
			trace.Truncated = true
			continue
		}
		trace.Candidates = append(trace.Candidates, DecisionCandidate{Key: string(item.key), Name: item.candidate.Name, Source: string(item.candidate.Source), Ownership: ownership(item.candidate.InLibrary), Rank: RankTuple{Relevance: item.relevance, Preference: item.preference, Novelty: item.novelty, TieKey: string(item.key)}, Disposition: DispositionNotSelected, Reason: ReasonNotSelected})
	}
	if len(result) == 0 {
		trace.Terminal = ReasonRetrievalEmpty
	}
	return RankedCandidates{Candidates: result, Trace: trace}
}

func ownership(inLibrary bool) string {
	if inLibrary {
		return "library"
	}
	return "acquisition"
}

// RankGroundedCandidates is the one pure ordering seam for discovery. Relevance
// is the primary sort key, so a taste signal cannot promote an unrelated title
// above a relevant one. Feedback and novelty only order candidates within the
// same relevance band; identity is the final deterministic tie-break.
func RankGroundedCandidates(intent string, candidates []catalog.Candidate, signals []FeedbackSignal) []catalog.Candidate {
	return rankGroundedCandidates(intent, candidates, signals)
}

func rankGroundedCandidates(intent string, candidates []catalog.Candidate, signals []FeedbackSignal) []catalog.Candidate {
	ranked := rankDetailed(intent, candidates, signals)
	out := make([]catalog.Candidate, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.candidate)
	}
	return out
}

func rankDetailed(intent string, candidates []catalog.Candidate, signals []FeedbackSignal) []rankedCandidate {
	effective := make(map[provision.Key]FeedbackAction, len(signals))
	lessGenres := map[string]bool{}
	byKey := make(map[provision.Key]catalog.Candidate, len(candidates))
	for _, candidate := range candidates {
		if key, err := candidate.Key(); err == nil {
			byKey[key] = candidate
		}
	}
	for _, signal := range signals {
		effective[signal.Target] = signal.Action
		if signal.Action == FeedbackLess {
			for _, genre := range byKey[signal.Target].Genres {
				lessGenres[strings.ToLower(genre)] = true
			}
		}
	}
	query := wordSet(intent)
	ranked := make([]rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key, err := candidate.Key()
		if err != nil || effective[key] == FeedbackNever {
			continue
		}
		preference := 0
		switch effective[key] {
		case FeedbackKeep:
			preference += 3
		case FeedbackLess:
			preference -= 4
		case FeedbackSurprise:
			preference += 3
		}
		for _, genre := range candidate.Genres {
			if lessGenres[strings.ToLower(genre)] && effective[key] != FeedbackSurprise {
				preference--
				break
			}
		}
		novelty := 0
		if !candidate.InLibrary {
			novelty = 1
		}
		ranked = append(ranked, rankedCandidate{candidate: candidate, key: key,
			relevance:  overlap(query, wordSet(candidate.Name+" "+candidate.Overview+" "+strings.Join(candidate.Genres, " "))),
			preference: preference, novelty: novelty})
	}
	slices.SortFunc(ranked, func(a, b rankedCandidate) int {
		if order := cmp.Compare(b.relevance, a.relevance); order != 0 {
			return order
		}
		if order := cmp.Compare(b.preference, a.preference); order != 0 {
			return order
		}
		if order := cmp.Compare(b.novelty, a.novelty); order != 0 {
			return order
		}
		return cmp.Compare(string(a.key), string(b.key))
	})
	return ranked
}

func wordSet(text string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := make(map[string]bool, len(words))
	for _, word := range words {
		if strings.HasSuffix(word, "ies") && len(word) > 4 {
			word = strings.TrimSuffix(word, "ies") + "y"
		} else if strings.HasSuffix(word, "s") && len(word) > 3 {
			word = strings.TrimSuffix(word, "s")
		}
		out[word] = true
	}
	return out
}

func overlap(a, b map[string]bool) int {
	n := 0
	for word := range a {
		if b[word] {
			n++
		}
	}
	return n
}
