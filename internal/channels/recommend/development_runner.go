package recommend

import (
	"context"
	"encoding/json"
	"math"

	"github.com/loomarr/loomarr/internal/llm"
)

type DevelopmentProtocol string

const (
	ProtocolJSONModeV1   DevelopmentProtocol = "json-mode-v1"
	ProtocolPromptOnlyV1 DevelopmentProtocol = "prompt-only-v1"
)

type DevelopmentCaseRun struct {
	CaseID            string                `json:"caseId"`
	Diagnostics       StructuralDiagnostics `json:"diagnostics"`
	Call              CallRecord            `json:"call"`
	ProviderErrorCode string                `json:"providerErrorCode,omitempty"`
}

// DevelopmentScorecard is a non-certifying, content-free protocol artifact.
// Unlike Scorecard, it deliberately has no quality score or ship decision.
type DevelopmentScorecard struct {
	SchemaVersion int                  `json:"schemaVersion"`
	CorpusVersion string               `json:"corpusVersion"`
	CorpusSHA256  string               `json:"corpusSha256"`
	PromptVersion string               `json:"promptVersion"`
	OutputSchema  string               `json:"outputSchema"`
	Protocol      DevelopmentProtocol  `json:"protocol"`
	Profile       string               `json:"profile"`
	Provider      string               `json:"provider"`
	Model         string               `json:"model"`
	Budget        RunConfig            `json:"budget"`
	Resources     ResourceUsage        `json:"resources"`
	Results       []DevelopmentCaseRun `json:"results"`
	StopReason    string               `json:"stopReason,omitempty"`
	Completed     bool                 `json:"completed"`
}

// RunDevelopment probes one transport variant against only the disjoint
// development split. Raw responses exist only long enough to classify their
// structure and are never copied into the returned artifact.
func (r *Runner) RunDevelopment(ctx context.Context, corpus Corpus, protocol DevelopmentProtocol) DevelopmentScorecard {
	card := DevelopmentScorecard{
		SchemaVersion: 1, CorpusVersion: corpus.Version, CorpusSHA256: corpus.Fixture.SHA256,
		PromptVersion: corpus.PromptVersion, OutputSchema: corpus.SchemaVersionName,
		Protocol: protocol, Profile: r.config.Profile, Provider: boundedIdentity(r.provider.Name()),
		Model: boundedIdentity(r.config.Model), Budget: r.config,
		Resources: ResourceUsage{AccountingComplete: true},
	}
	if corpus.Split != "development" || len(corpus.Cases) != r.config.ExpectedCases || !validDevelopmentProtocol(protocol) {
		card.StopReason = StopConfiguration
		return card
	}
	temperature := 0.0
	for _, developmentCase := range corpus.Cases {
		if card.Resources.Calls >= r.config.MaxCalls {
			card.StopReason = StopCallExceeded
			break
		}
		snapshot, err := json.Marshal(developmentCase.Snapshot)
		if err != nil {
			card.StopReason = StopConfiguration
			break
		}
		caseCtx, cancel := context.WithTimeout(ctx, r.config.PerCaseTimeout)
		response, chatErr := r.provider.Chat(caseCtx, []llm.Message{
			{Role: llm.System, Content: recommendationSystemPrompt},
			{Role: llm.User, Content: string(snapshot)},
		}, llm.ChatOptions{
			JSONMode: protocol == ProtocolJSONModeV1, Temperature: &temperature,
			MaxTokens: r.config.MaxOutputTokens,
		})
		cancel()

		run := DevelopmentCaseRun{CaseID: developmentCase.ID}
		if chatErr != nil {
			run.ProviderErrorCode = StopProviderFailure
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
			accountingErr = errSpendOverflow
		} else {
			card.Resources.SpendNanoUSD += charge
		}
		if accountingErr != nil {
			run.ProviderErrorCode = StopAccountingUnknown
			card.Results = append(card.Results, run)
			card.Resources.AccountingComplete = false
			card.StopReason = StopAccountingUnknown
			break
		}
		run.Diagnostics = DiagnoseOutput([]byte(response.Content))
		run.Diagnostics.OutputCeilingReached = response.Attribution.Tokens.Completion >= r.config.MaxOutputTokens
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
	card.Completed = card.StopReason == "" && len(card.Results) == len(corpus.Cases)
	return card
}

func validDevelopmentProtocol(protocol DevelopmentProtocol) bool {
	return protocol == ProtocolJSONModeV1 || protocol == ProtocolPromptOnlyV1
}

var errSpendOverflow = &developmentRunError{"provider spend overflow"}

type developmentRunError struct{ message string }

func (err *developmentRunError) Error() string { return err.message }
