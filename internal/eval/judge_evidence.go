//go:build eval

package eval

import (
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

// The Judge seam stays bounded even when a caller supplies unusually large
// grounded batches or open policy collections.
const (
	JudgeMaxTitlesPerOwnership = 16
	JudgeMaxGenresPerItem      = 8
	JudgeMaxPolicyValues       = 16
	JudgeMaxScheduledPrograms  = 24
	JudgeMaxEpisodeTags        = 16
	JudgeMaxTextRunes          = 512
	JudgeMaxTraceCandidates    = 64
)

// JudgeEvidence is the typed audit record passed across the subjective Judge seam.
// NewJudgeEvidence is its construction boundary; callers do not pass provider or
// transport configuration into model judgment.
type JudgeEvidence struct {
	Request           string                  `json:"request"`
	Rubric            string                  `json:"rubric"`
	Lineup            []JudgeTitleEvidence    `json:"lineup"`
	Acquisitions      []JudgeTitleEvidence    `json:"acquisitions"`
	Policy            schedule.ProposalPolicy `json:"policy"`
	Observation       Observation             `json:"observation"`
	ScheduledPrograms []MaterializedProgram   `json:"scheduledPrograms"`
	DecisionTrace     suggest.DecisionTrace   `json:"decisionTrace"`
}

// JudgeTitleEvidence is the grounded, auditable subset of a ProposalItem.
type JudgeTitleEvidence struct {
	Key        string         `json:"key"`
	Name       string         `json:"name"`
	Year       int            `json:"year,omitempty"`
	Ownership  JudgeOwnership `json:"ownership"`
	Source     string         `json:"source,omitempty"`
	Rationale  string         `json:"rationale,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	Genres     []string       `json:"genres,omitempty"`
	Rating     string         `json:"rating,omitempty"`
}

// JudgeOwnership is the closed proposal disposition vocabulary exposed to judges.
type JudgeOwnership string

const (
	JudgeOwnershipLibrary     JudgeOwnership = "library"
	JudgeOwnershipAcquisition JudgeOwnership = "acquisition"
)

// NewJudgeEvidence projects the facts already owned by Runner into the Judge
// contract. Bounding is enforced here rather than delegated to model adapters.
func NewJudgeEvidence(c Case, proposal suggest.Proposal, observation Observation, scheduledPrograms []MaterializedProgram) (JudgeEvidence, error) {
	trace, err := boundedDecisionTrace(proposal.Trace)
	if err != nil {
		return JudgeEvidence{}, err
	}
	lineup, err := judgeTitles(proposal.Lineup, JudgeOwnershipLibrary)
	if err != nil {
		return JudgeEvidence{}, err
	}
	acquisitions, err := judgeTitles(proposal.Acquisitions, JudgeOwnershipAcquisition)
	if err != nil {
		return JudgeEvidence{}, err
	}
	return boundJudgeEvidence(JudgeEvidence{
		Request:           c.Intent.Description,
		Rubric:            c.JudgeRubric,
		Lineup:            lineup,
		Acquisitions:      acquisitions,
		Policy:            proposal.Policy.ProposalPolicy,
		Observation:       observation,
		ScheduledPrograms: scheduledPrograms,
		DecisionTrace:     trace,
	}), nil
}

func boundedDecisionTrace(trace suggest.DecisionTrace) (suggest.DecisionTrace, error) {
	if trace.Version == 0 {
		return suggest.DecisionTrace{}, nil
	}
	trace.Candidates = append([]suggest.DecisionCandidate(nil), trace.Candidates...)
	if len(trace.Candidates) > JudgeMaxTraceCandidates {
		trace.Candidates = trace.Candidates[:JudgeMaxTraceCandidates]
		trace.Truncated = true
	}
	for i := range trace.Candidates {
		trace.Candidates[i].Key = boundedJudgeText(trace.Candidates[i].Key)
		trace.Candidates[i].Name = boundedJudgeText(trace.Candidates[i].Name)
		trace.Candidates[i].Source = boundedJudgeText(trace.Candidates[i].Source)
		trace.Candidates[i].Ownership = boundedJudgeText(trace.Candidates[i].Ownership)
		trace.Candidates[i].Disposition = boundedJudgeText(trace.Candidates[i].Disposition)
		trace.Candidates[i].Reason = boundedJudgeText(trace.Candidates[i].Reason)
		trace.Candidates[i].Rank.TieKey = boundedJudgeText(trace.Candidates[i].Rank.TieKey)
	}
	trace.Terminal = boundedJudgeText(trace.Terminal)
	if err := suggest.ValidateDecisionTrace(trace); err != nil {
		return suggest.DecisionTrace{}, fmt.Errorf("decision trace mismatch: %w", err)
	}
	return trace, nil
}

func judgeTitles(items []suggest.ProposalItem, ownership JudgeOwnership) ([]JudgeTitleEvidence, error) {
	out := make([]JudgeTitleEvidence, 0, len(items))
	for _, item := range items {
		key, err := item.Key()
		if err != nil {
			return nil, fmt.Errorf("judge evidence item %q has no canonical key: %w", item.Name, err)
		}
		evidence := JudgeTitleEvidence{
			Key: string(key), Name: item.Name, Year: item.Year, Ownership: ownership,
			Source: item.Source, Rationale: item.Rationale,
			Genres: item.Genres, Rating: item.OfficialRating,
		}
		if item.Confidence != 0 {
			confidence := item.Confidence
			evidence.Confidence = &confidence
		}
		out = append(out, evidence)
	}
	return out, nil
}

func boundJudgeEvidence(evidence JudgeEvidence) JudgeEvidence {
	evidence.Request = boundedJudgeText(evidence.Request)
	evidence.Rubric = boundedJudgeText(evidence.Rubric)
	evidence.Lineup = boundJudgeTitles(evidence.Lineup, JudgeOwnershipLibrary)
	evidence.Acquisitions = boundJudgeTitles(evidence.Acquisitions, JudgeOwnershipAcquisition)
	evidence.Policy = boundJudgePolicy(evidence.Policy)
	evidence.Observation.GroundingStage = boundedJudgeText(evidence.Observation.GroundingStage)
	evidence.ScheduledPrograms = boundScheduledPrograms(evidence.ScheduledPrograms)
	if trace, err := boundedDecisionTrace(evidence.DecisionTrace); err == nil {
		evidence.DecisionTrace = trace
	}
	return evidence
}

func boundScheduledPrograms(programs []MaterializedProgram) []MaterializedProgram {
	programs = boundedJudgeSlice(programs, JudgeMaxScheduledPrograms)
	for i := range programs {
		programs[i].Identity = boundedJudgeText(programs[i].Identity)
		programs[i].Title = boundedJudgeText(programs[i].Title)
		programs[i].Rating = boundedJudgeText(programs[i].Rating)
		programs[i].Overview = boundedJudgeText(programs[i].Overview)
		programs[i].Tags = boundJudgeStrings(programs[i].Tags, JudgeMaxEpisodeTags)
	}
	return programs
}

func boundJudgeTitles(items []JudgeTitleEvidence, ownership JudgeOwnership) []JudgeTitleEvidence {
	items = boundedJudgeSlice(items, JudgeMaxTitlesPerOwnership)
	for i := range items {
		items[i].Key = boundedJudgeText(items[i].Key)
		items[i].Name = boundedJudgeText(items[i].Name)
		items[i].Ownership = ownership
		items[i].Source = boundedJudgeText(items[i].Source)
		items[i].Rationale = boundedJudgeText(items[i].Rationale)
		items[i].Genres = boundJudgeStrings(items[i].Genres, JudgeMaxGenresPerItem)
		items[i].Rating = boundedJudgeText(items[i].Rating)
		if items[i].Confidence != nil {
			confidence := *items[i].Confidence
			items[i].Confidence = &confidence
		}
	}
	return items
}

func boundJudgePolicy(policy schedule.ProposalPolicy) schedule.ProposalPolicy {
	policy.Scope = boundJudgeScope(policy.Scope)
	policy.Audience.Ceiling = schedule.Rating(boundedJudgeText(string(policy.Audience.Ceiling)))
	policy.Audience.Unrated = schedule.UnratedPolicy(boundedJudgeText(string(policy.Audience.Unrated)))
	policy.Ordering = schedule.OrderingMode(boundedJudgeText(string(policy.Ordering)))
	policy.Seasonal.Mode = schedule.SeasonalMode(boundedJudgeText(string(policy.Seasonal.Mode)))
	policy.Seasonal.Holidays = boundJudgeStrings(policy.Seasonal.Holidays, JudgeMaxPolicyValues)
	policy.Seasonal.OffSeason = schedule.OffSeason(boundedJudgeText(string(policy.Seasonal.OffSeason)))
	policy.Rules = boundedJudgeSlice(policy.Rules, JudgeMaxPolicyValues)
	for i := range policy.Rules {
		rule := &policy.Rules[i]
		rule.ID = boundedJudgeText(rule.ID)
		rule.Source = schedule.RuleSource(boundedJudgeText(string(rule.Source)))
		rule.Label = boundedJudgeText(rule.Label)
		rule.When.Days = boundedJudgeSlice(rule.When.Days, JudgeMaxPolicyValues)
		rule.When.Holiday = boundedJudgeText(rule.When.Holiday)
		rule.How.Ordering = schedule.OrderingMode(boundedJudgeText(string(rule.How.Ordering)))
		if rule.What != nil {
			what := boundJudgeScope(*rule.What)
			rule.What = &what
		}
	}
	return policy
}

func boundJudgeScope(scope schedule.ScopePolicy) schedule.ScopePolicy {
	scope.Series = boundedJudgeSlice(scope.Series, JudgeMaxPolicyValues)
	for i := range scope.Series {
		scope.Series[i] = provision.Key(boundedJudgeText(string(scope.Series[i])))
	}
	scope.Collections = boundJudgeStrings(scope.Collections, JudgeMaxPolicyValues)
	scope.Genres.Include = boundJudgeStrings(scope.Genres.Include, JudgeMaxPolicyValues)
	scope.Genres.Exclude = boundJudgeStrings(scope.Genres.Exclude, JudgeMaxPolicyValues)
	return scope
}

func boundJudgeStrings(values []string, limit int) []string {
	values = boundedJudgeSlice(values, limit)
	for i := range values {
		values[i] = boundedJudgeText(values[i])
	}
	return values
}

func boundedJudgeSlice[T any](values []T, limit int) []T {
	if limit < len(values) {
		values = values[:limit]
	}
	return slices.Clone(values)
}

func boundedJudgeText(value string) string {
	runes := []rune(value)
	if len(runes) <= JudgeMaxTextRunes {
		return value
	}
	return string(runes[:JudgeMaxTextRunes-1]) + "…"
}
