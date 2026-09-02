package recommend

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
)

const (
	StopAccountingUnknown = "accounting_unknown"
	StopCallExceeded      = "call_budget_exceeded"
	StopTokenExceeded     = "token_budget_exceeded"
	StopSpendExceeded     = "spend_budget_exceeded"
	StopProviderFailure   = "provider_failure"
	StopConfiguration     = "configuration_invalid"
)

const recommendationSystemPrompt = `You recommend inert draft channel concepts from one synthetic Loomarr context.
Return only JSON matching {"concepts":[{"name":"...","intent":{"description":"...","era":"...","tone":"...","mustInclude":[],"mustExclude":[]},"evidenceIds":["..."]}]}.
Every factual or policy basis must cite an exact signal id from the supplied snapshot. Never invent evidence.
Do not create or identify a Channel, Proposal, job, approval, acquisition, status, or other effect. Avoid concepts already represented in existingConcepts.
Return {"concepts":[]} when the evidence is insufficient or contradictory.`

type RunConfig struct {
	Profile         string        `json:"profile"`
	Model           string        `json:"model"`
	ExpectedCases   int           `json:"expectedCases"`
	MaxCalls        int           `json:"maxCalls"`
	MaxTokens       int           `json:"maxTokens"`
	MaxSpendNanoUSD int64         `json:"maxSpendNanoUsd"`
	MaxOutputTokens int           `json:"maxOutputTokens"`
	PerCaseTimeout  time.Duration `json:"perCaseTimeoutNanos"`
}

type ResourceUsage struct {
	Calls              int   `json:"calls"`
	Tokens             int   `json:"tokens"`
	SpendNanoUSD       int64 `json:"spendNanoUsd"`
	AccountingComplete bool  `json:"accountingComplete"`
}

type CallRecord struct {
	RequestedProvider string `json:"requestedProvider"`
	RequestedModel    string `json:"requestedModel"`
	ResolvedProvider  string `json:"resolvedProvider,omitempty"`
	ResolvedModel     string `json:"resolvedModel,omitempty"`
	PromptTokens      int    `json:"promptTokens"`
	CompletionTokens  int    `json:"completionTokens"`
	ReasoningTokens   int    `json:"reasoningTokens,omitempty"`
	CachedTokens      int    `json:"cachedTokens,omitempty"`
	ChargeNanoUSD     int64  `json:"chargeNanoUsd"`
	Attempts          int    `json:"attempts"`
	LatencyNanos      int64  `json:"latencyNanos"`
}

type CaseRun struct {
	CaseResult
	Call          CallRecord `json:"call"`
	ProviderError string     `json:"providerError,omitempty"`
}

type AggregateQuality struct {
	Quality
	SchemaValidity float64 `json:"schemaValidity"`
}

type Scorecard struct {
	SchemaVersion     int              `json:"schemaVersion"`
	CorpusVersion     string           `json:"corpusVersion"`
	CorpusSHA256      string           `json:"corpusSha256"`
	PromptVersion     string           `json:"promptVersion"`
	OutputSchema      string           `json:"outputSchema"`
	ScorerVersion     string           `json:"scorerVersion"`
	Profile           string           `json:"profile"`
	Provider          string           `json:"provider"`
	Model             string           `json:"model"`
	Budget            RunConfig        `json:"budget"`
	Resources         ResourceUsage    `json:"resources"`
	Quality           AggregateQuality `json:"quality"`
	Thresholds        Thresholds       `json:"thresholds"`
	SelectionMetric   string           `json:"selectionMetric"`
	SelectionMargin   float64          `json:"selectionMargin"`
	HardFailureCounts map[string]int   `json:"hardFailureCounts"`
	Results           []CaseRun        `json:"results"`
	StopReason        string           `json:"stopReason,omitempty"`
	Certified         bool             `json:"certified"`
}

type Runner struct {
	provider llm.Provider
	config   RunConfig
}

func NewRunner(provider llm.Provider, config RunConfig) (*Runner, error) {
	if provider == nil {
		return nil, fmt.Errorf("recommendation runner requires a provider")
	}
	if strings.TrimSpace(config.Profile) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("recommendation runner requires profile and model identities")
	}
	if config.ExpectedCases <= 0 {
		return nil, fmt.Errorf("recommendation runner requires a positive expected case count")
	}
	if config.MaxCalls < config.ExpectedCases {
		return nil, fmt.Errorf("max calls %d cannot cover %d declared cases", config.MaxCalls, config.ExpectedCases)
	}
	if config.MaxTokens <= 0 || config.MaxSpendNanoUSD <= 0 || config.MaxOutputTokens <= 0 || config.PerCaseTimeout <= 0 {
		return nil, fmt.Errorf("recommendation runner requires positive token, spend, output, and timeout ceilings")
	}
	return &Runner{provider: provider, config: config}, nil
}

func (r *Runner) Run(ctx context.Context, corpus Corpus) Scorecard {
	card := Scorecard{
		SchemaVersion: 1, CorpusVersion: corpus.Version, CorpusSHA256: corpus.Fixture.SHA256,
		PromptVersion: corpus.PromptVersion, OutputSchema: corpus.SchemaVersionName,
		ScorerVersion: corpus.ScorerVersion, Profile: r.config.Profile,
		Provider: boundedIdentity(r.provider.Name()), Model: boundedIdentity(r.config.Model),
		Budget: r.config, Thresholds: corpus.Thresholds, SelectionMetric: corpus.SelectionMetric,
		SelectionMargin:   corpus.SelectionMargin,
		Resources:         ResourceUsage{AccountingComplete: true},
		HardFailureCounts: make(map[string]int),
	}
	if len(corpus.Cases) != r.config.ExpectedCases {
		card.StopReason = StopConfiguration
		return card
	}
	for _, certificationCase := range corpus.Cases {
		if card.Resources.Calls >= r.config.MaxCalls {
			card.StopReason = StopCallExceeded
			break
		}
		snapshot, err := json.Marshal(certificationCase.Snapshot)
		if err != nil {
			card.StopReason = StopConfiguration
			break
		}
		caseCtx, cancel := context.WithTimeout(ctx, r.config.PerCaseTimeout)
		response, chatErr := r.provider.Chat(caseCtx, []llm.Message{
			{Role: llm.System, Content: recommendationSystemPrompt},
			{Role: llm.User, Content: string(snapshot)},
		}, llm.ChatOptions{JSONMode: true, MaxTokens: r.config.MaxOutputTokens})
		cancel()

		run := CaseRun{CaseResult: CaseResult{CaseID: certificationCase.ID}}
		if chatErr != nil {
			run.ProviderError = boundedError(chatErr)
			card.Results = append(card.Results, run)
			card.Resources.AccountingComplete = false
			card.StopReason = StopProviderFailure
			break
		}
		call, attempts, tokens, charge, accountingErr := accountCall(r.provider.Name(), response.Attribution)
		run.Call = call
		card.Resources.Calls += attempts
		card.Resources.Tokens, err = checkedIntAdd(card.Resources.Tokens, tokens)
		if err != nil {
			accountingErr = err
		}
		if charge > math.MaxInt64-card.Resources.SpendNanoUSD {
			accountingErr = fmt.Errorf("provider spend overflow")
		} else {
			card.Resources.SpendNanoUSD += charge
		}
		if accountingErr != nil {
			run.ProviderError = boundedError(accountingErr)
			card.Results = append(card.Results, run)
			card.Resources.AccountingComplete = false
			card.StopReason = StopAccountingUnknown
			break
		}
		run.CaseResult = ScoreCase(certificationCase, []byte(response.Content))
		for _, failure := range run.HardFailures {
			card.HardFailureCounts[failure.Code]++
		}
		card.Results = append(card.Results, run)
		switch {
		case card.Resources.Calls > r.config.MaxCalls:
			card.StopReason = StopCallExceeded
		case card.Resources.Tokens > r.config.MaxTokens:
			card.StopReason = StopTokenExceeded
		case card.Resources.SpendNanoUSD > r.config.MaxSpendNanoUSD:
			card.StopReason = StopSpendExceeded
		}
		if card.StopReason != "" {
			break
		}
	}
	card.Quality = aggregateQuality(card.Results, len(corpus.Cases))
	card.Certified = card.StopReason == "" && len(card.Results) == len(corpus.Cases) &&
		len(card.HardFailureCounts) == 0 && meetsThresholds(card.Quality, corpus.Thresholds)
	return card
}

func accountCall(provider string, attribution llm.Attribution) (CallRecord, int, int, int64, error) {
	attempts := attribution.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	tokens, err := checkedIntAdd(attribution.Tokens.Prompt, attribution.Tokens.Completion)
	call := CallRecord{
		RequestedProvider: boundedIdentity(attribution.RequestedProvider), RequestedModel: boundedIdentity(attribution.RequestedModel),
		ResolvedProvider: boundedIdentity(attribution.ResolvedProvider), ResolvedModel: boundedIdentity(attribution.ResolvedModel),
		PromptTokens: attribution.Tokens.Prompt, CompletionTokens: attribution.Tokens.Completion,
		ReasoningTokens: attribution.Tokens.Reasoning, CachedTokens: attribution.Tokens.Cached,
		Attempts: attempts, LatencyNanos: int64(attribution.Latency),
	}
	if err != nil || attribution.Tokens.Prompt < 0 || attribution.Tokens.Completion < 0 {
		return call, attempts, 0, 0, fmt.Errorf("provider token accounting is invalid")
	}
	hosted := !strings.EqualFold(strings.TrimSpace(provider), "ollama")
	if hosted && tokens == 0 {
		return call, attempts, tokens, 0, fmt.Errorf("hosted provider omitted token accounting")
	}
	if attribution.Charge == nil {
		if hosted {
			return call, attempts, tokens, 0, fmt.Errorf("hosted provider omitted charge accounting")
		}
		return call, attempts, tokens, 0, nil
	}
	if attribution.Charge.Currency != "USD" {
		return call, attempts, tokens, 0, fmt.Errorf("provider charge is not USD")
	}
	charge, err := decimalUSDToNano(attribution.Charge.Amount)
	if err != nil {
		return call, attempts, tokens, 0, err
	}
	call.ChargeNanoUSD = charge
	return call, attempts, tokens, charge, nil
}

func decimalUSDToNano(value string) (int64, error) {
	whole, fraction, found := strings.Cut(value, ".")
	if !found {
		fraction = ""
	}
	if whole == "" || (found && fraction == "") || len(fraction) > 9 {
		return 0, fmt.Errorf("provider charge is not exact to nanodollars")
	}
	for _, part := range []string{whole, fraction} {
		for _, char := range part {
			if char < '0' || char > '9' {
				return 0, fmt.Errorf("provider charge is not a plain non-negative decimal")
			}
		}
	}
	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || wholeValue > math.MaxInt64/1_000_000_000 {
		return 0, fmt.Errorf("provider charge overflows nanodollars")
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	fractionValue := int64(0)
	if fraction != "" {
		fractionValue, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("provider charge is invalid")
		}
	}
	return wholeValue*1_000_000_000 + fractionValue, nil
}

func aggregateQuality(results []CaseRun, expected int) AggregateQuality {
	if expected == 0 {
		return AggregateQuality{}
	}
	var aggregate AggregateQuality
	validSchema := 0
	for _, result := range results {
		aggregate.Relevance += result.Quality.Relevance
		aggregate.Novelty += result.Quality.Novelty
		aggregate.Diversity += result.Quality.Diversity
		aggregate.CatalogFeasibility += result.Quality.CatalogFeasibility
		aggregate.PolicySafety += result.Quality.PolicySafety
		aggregate.Abstention += result.Quality.Abstention
		invalid := false
		for _, failure := range result.HardFailures {
			invalid = invalid || failure.Code == FailureInvalidSchema
		}
		if result.ProviderError == "" && !invalid {
			validSchema++
		}
	}
	denominator := float64(expected)
	aggregate.Relevance /= denominator
	aggregate.Novelty /= denominator
	aggregate.Diversity /= denominator
	aggregate.CatalogFeasibility /= denominator
	aggregate.PolicySafety /= denominator
	aggregate.Abstention /= denominator
	aggregate.SchemaValidity = float64(validSchema) / denominator
	return aggregate
}

func meetsThresholds(quality AggregateQuality, thresholds Thresholds) bool {
	return quality.Relevance >= thresholds.MinRelevance && quality.Novelty >= thresholds.MinNovelty &&
		quality.Diversity >= thresholds.MinDiversity && quality.CatalogFeasibility >= thresholds.MinCatalogFeasibility &&
		quality.PolicySafety >= thresholds.MinPolicySafety && quality.SchemaValidity >= thresholds.MinSchemaValidity &&
		quality.Abstention >= thresholds.MinAbstention
}

func checkedIntAdd(left, right int) (int, error) {
	if left < 0 || right < 0 || left > math.MaxInt-right {
		return 0, fmt.Errorf("provider token accounting overflow")
	}
	return left + right, nil
}

func boundedIdentity(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 256 {
		return string(runes)
	}
	return string(runes[:255]) + "…"
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	return boundedIdentity(err.Error())
}
