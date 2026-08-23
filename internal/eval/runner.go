//go:build eval

package eval

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/suggest"
)

const (
	scorecardSchemaVersion = 3
	corpusVersion          = "2026-08-23.4"
)

// Generator is the one external seam the behavioral evaluator needs: production
// supplies the real grounded Suggester, while hermetic tests supply a scripted
// adapter. Everything after the Proposal remains deterministic Loomarr code.
type Generator interface {
	Suggest(context.Context, suggest.Intent) (suggest.Proposal, error)
}

// RunnerConfig identifies one reproducible evaluation profile. Credentials and
// provider payloads never enter it or the scorecard.
type RunnerConfig struct {
	Trials   int
	Profile  string
	Provider string
	Model    string
	Required bool
}

// Scorecard is the versioned machine-readable result of one Runner execution.
type Scorecard struct {
	SchemaVersion int           `json:"schemaVersion"`
	CorpusVersion string        `json:"corpusVersion"`
	GeneratedAt   time.Time     `json:"generatedAt"`
	Profile       string        `json:"profile"`
	Provider      string        `json:"provider"`
	Model         string        `json:"model"`
	Certified     bool          `json:"certified"`
	Results       []Result      `json:"results"`
	Cases         []CaseSummary `json:"cases"`
}

// CaseSummary makes stochastic stability visible rather than collapsing several
// trials into one last-write-wins result.
type CaseSummary struct {
	Case        string     `json:"case"`
	Trials      int        `json:"trials"`
	Passed      int        `json:"passed"`
	PassRate    float64    `json:"passRate"`
	Relevance   ScoreRange `json:"relevance"`
	Serendipity ScoreRange `json:"serendipity"`
}

type ScoreRange struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// Judge is the subjective scoring seam. Deterministic requirements never cross
// it; production supplies an LLM-backed adapter and hermetic tests a scripted one.
type Judge interface {
	Score(context.Context, Case, suggest.Proposal) judgeScores
}

// Observer records bounded structural evidence around one Generator trial.
type Observer interface {
	Begin()
	Snapshot(error) Observation
}

// Runner owns evaluation from grounded generation through deterministic gates.
// Later schedule and judge evidence deepen this same interface rather than
// creating parallel exploratory/certification paths.
type Runner struct {
	generator    Generator
	materializer ScheduleMaterializer
	judge        Judge
	observer     Observer
	config       RunnerConfig
}

func (r *Runner) WithMaterializer(materializer ScheduleMaterializer) *Runner {
	r.materializer = materializer
	return r
}

func (r *Runner) WithJudge(judge Judge) *Runner {
	r.judge = judge
	return r
}

func (r *Runner) WithObserver(observer Observer) *Runner {
	r.observer = observer
	return r
}

func NewRunner(generator Generator, config RunnerConfig) *Runner {
	if config.Trials <= 0 {
		config.Trials = 1
	}
	return &Runner{generator: generator, config: config}
}

// Run evaluates every case serially for the configured number of trials.
func (r *Runner) Run(ctx context.Context, cases []Case) Scorecard {
	card := Scorecard{
		SchemaVersion: scorecardSchemaVersion,
		CorpusVersion: corpusVersion,
		GeneratedAt:   time.Now().UTC(),
		Profile:       r.config.Profile,
		Provider:      r.config.Provider,
		Model:         r.config.Model,
		Certified:     len(cases) > 0,
	}
	for _, c := range cases {
		passed := 0
		var relevance, serendipity []float64
		for trial := 1; trial <= r.config.Trials; trial++ {
			if r.observer != nil {
				r.observer.Begin()
			}
			prop, err := r.generator.Suggest(ctx, mapIntent(c.Intent))
			result := Result{
				Case: c.Name, Trial: trial, Lineup: len(prop.Lineup), Acquisitions: len(prop.Acquisitions),
				Ceiling: string(prop.Policy.Audience.Ceiling), ThemeFit: prop.Scores.ThemeFit,
				JudgeScore: -1, RelevanceScore: -1, SerendipityScore: -1,
				Failures: deterministicChecks(c, prop, err),
			}
			if r.observer != nil {
				result.Observation = r.observer.Snapshot(err)
				if c.MaxToolCalls > 0 && result.ToolCalls > c.MaxToolCalls {
					result.Failures = append(result.Failures, fmt.Sprintf("tool calls %d > budget %d", result.ToolCalls, c.MaxToolCalls))
				}
				if c.MaxCandidatesSurfaced > 0 && result.CandidatesSurfaced > c.MaxCandidatesSurfaced {
					result.Failures = append(result.Failures, fmt.Sprintf("candidates surfaced %d > budget %d", result.CandidatesSurfaced, c.MaxCandidatesSurfaced))
				}
			}
			if err == nil && requiresSchedule(c) {
				if r.materializer == nil {
					result.Failures = append(result.Failures, "schedule materializer is not configured")
				} else {
					programs, scheduleErr := r.materializer.Materialize(ctx, c, prop)
					if scheduleErr != nil {
						result.Failures = append(result.Failures, "schedule materialization failed: "+scheduleErr.Error())
					} else {
						result.ScheduledPrograms = programs
						result.Failures = append(result.Failures, scheduledChecks(c, programs)...)
					}
				}
			}
			if result.Passed() && c.JudgeRubric != "" && r.judge != nil {
				scores := r.judge.Score(ctx, c, prop)
				result.JudgeScore = scores.Overall
				result.RelevanceScore = scores.Relevance
				result.SerendipityScore = scores.Serendipity
				result.JudgeNote = scores.Reason
				if c.MinJudgeScore > 0 && scores.Overall < c.MinJudgeScore && (r.config.Required || scores.Overall >= 0) {
					result.Failures = append(result.Failures,
						fmt.Sprintf("judge score %.2f < required %.2f: %s", scores.Overall, c.MinJudgeScore, scores.Reason))
				}
				if c.MinRelevanceScore > 0 && scores.Relevance < c.MinRelevanceScore && (r.config.Required || scores.Relevance >= 0) {
					result.Failures = append(result.Failures,
						fmt.Sprintf("relevance score %.2f < required %.2f: %s", scores.Relevance, c.MinRelevanceScore, scores.Reason))
				}
				if c.MinSerendipityScore > 0 && scores.Serendipity < c.MinSerendipityScore && (r.config.Required || scores.Serendipity >= 0) {
					result.Failures = append(result.Failures,
						fmt.Sprintf("serendipity score %.2f < required %.2f: %s", scores.Serendipity, c.MinSerendipityScore, scores.Reason))
				}
				if scores.Relevance >= 0 {
					relevance = append(relevance, scores.Relevance)
				}
				if scores.Serendipity >= 0 {
					serendipity = append(serendipity, scores.Serendipity)
				}
			}
			card.Results = append(card.Results, result)
			if result.Passed() {
				passed++
			}
			card.Certified = card.Certified && result.Passed()
		}
		card.Cases = append(card.Cases, CaseSummary{
			Case: c.Name, Trials: r.config.Trials, Passed: passed,
			PassRate:  float64(passed) / float64(r.config.Trials),
			Relevance: scoreRange(relevance), Serendipity: scoreRange(serendipity),
		})
	}
	return card
}

func requiresSchedule(c Case) bool {
	return len(c.RequireScheduledPrograms) > 0 || len(c.ForbidScheduledPrograms) > 0 || len(c.RequireScheduledSequence) > 0
}

func scoreRange(values []float64) ScoreRange {
	if len(values) == 0 {
		return ScoreRange{}
	}
	values = slices.Clone(values)
	slices.Sort(values)
	middle := len(values) / 2
	median := values[middle]
	if len(values)%2 == 0 {
		median = (values[middle-1] + values[middle]) / 2
	}
	return ScoreRange{Min: values[0], Median: median, Max: values[len(values)-1]}
}
