package fillerreview

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
)

func estimateTemporalStructureOpenRouterCharge(config TemporalStructureOpenRouterConfig) (int64, error) {
	endpoint, ok := temporalStructureOpenRouterEndpoint(config)
	if !ok {
		return 0, fmt.Errorf("OpenRouter structure route is absent from the snapshot")
	}
	if config.MaximumInputTokens <= 0 || endpoint.ContextLength <= 0 || config.MaximumInputTokens > endpoint.ContextLength-int64(temporalStructureOpenRouterMaxTokens) {
		return 0, fmt.Errorf("OpenRouter structure maximum input-token allowance is invalid for the route context")
	}
	if endpoint.MaxPromptTokens > 0 && config.MaximumInputTokens > endpoint.MaxPromptTokens {
		return 0, fmt.Errorf("OpenRouter structure maximum input-token allowance exceeds the route limit")
	}
	prompt, ok := new(big.Rat).SetString(strings.TrimSpace(endpoint.Pricing["prompt"]))
	if !ok || prompt.Sign() < 0 {
		return 0, fmt.Errorf("OpenRouter structure route has invalid prompt pricing")
	}
	completion, ok := new(big.Rat).SetString(strings.TrimSpace(endpoint.Pricing["completion"]))
	if !ok || completion.Sign() < 0 {
		return 0, fmt.Errorf("OpenRouter structure route has invalid completion pricing")
	}
	total := new(big.Rat).Mul(prompt, big.NewRat(config.MaximumInputTokens, 1))
	total.Add(total, new(big.Rat).Mul(completion, big.NewRat(temporalStructureOpenRouterMaxTokens, 1)))
	total.Mul(total, big.NewRat(1_000_000_000, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(total.Num(), total.Denom(), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("OpenRouter structure price bound exceeds nanodollar range")
	}
	return quotient.Int64(), nil
}

func temporalStructureOpenRouterEndpoint(config TemporalStructureOpenRouterConfig) (fillerbakeoff.OpenRouterEndpointSnapshot, bool) {
	model := openRouterTemporalModel(config.Snapshot, config.Model)
	for _, endpoint := range model.Endpoints {
		if endpoint.ProviderName == config.UpstreamProvider && endpoint.ProviderSlug == config.UpstreamProviderSlug {
			return endpoint, true
		}
	}
	return fillerbakeoff.OpenRouterEndpointSnapshot{}, false
}
