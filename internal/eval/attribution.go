//go:build eval

package eval

import "github.com/loomarr/loomarr/internal/llm"

const (
	InferenceMaxIdentityRunes     = 256
	InferenceMaxChargeAmountRunes = 64
)

type InferenceChargeStatus string

const (
	InferenceChargeMissing  InferenceChargeStatus = "missing"
	InferenceChargeReported InferenceChargeStatus = "reported"
	InferenceChargeInvalid  InferenceChargeStatus = "invalid"
)

// InferenceCall is the bounded, scrubbed scorecard projection of one provider
// call. It intentionally excludes credentials, endpoints, prompts, payloads,
// modalities, and provider generation identifiers.
type InferenceCall struct {
	RequestedProvider string                `json:"requestedProvider"`
	RequestedModel    string                `json:"requestedModel"`
	ResolvedProvider  string                `json:"resolvedProvider"`
	ResolvedModel     string                `json:"resolvedModel"`
	Tokens            InferenceTokens       `json:"tokens"`
	Charge            InferenceCharge       `json:"charge"`
	ChargeStatus      InferenceChargeStatus `json:"chargeStatus"`
	Attempts          int                   `json:"attempts"`
	LatencyNanos      int64                 `json:"latencyNanos"`
}

type InferenceTokens struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Reasoning  int `json:"reasoning"`
	Cached     int `json:"cached"`
	CacheWrite int `json:"cacheWrite"`
	Image      int `json:"image"`
	Audio      int `json:"audio"`
	Video      int `json:"video"`
}

type InferenceCharge struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func scrubAttribution(attribution llm.Attribution) InferenceCall {
	call := InferenceCall{
		RequestedProvider: boundedInferenceText(attribution.RequestedProvider, InferenceMaxIdentityRunes),
		RequestedModel:    boundedInferenceText(attribution.RequestedModel, InferenceMaxIdentityRunes),
		ResolvedProvider:  boundedInferenceText(attribution.ResolvedProvider, InferenceMaxIdentityRunes),
		ResolvedModel:     boundedInferenceText(attribution.ResolvedModel, InferenceMaxIdentityRunes),
		Tokens: InferenceTokens{
			Prompt: attribution.Tokens.Prompt, Completion: attribution.Tokens.Completion,
			Reasoning: attribution.Tokens.Reasoning, Cached: attribution.Tokens.Cached,
			CacheWrite: attribution.Tokens.CacheWrite, Image: attribution.Tokens.Image,
			Audio: attribution.Tokens.Audio, Video: attribution.Tokens.Video,
		},
		ChargeStatus: InferenceChargeMissing,
		Attempts:     attribution.Attempts, LatencyNanos: int64(attribution.Latency),
	}
	if attribution.Charge != nil {
		call.ChargeStatus = InferenceChargeInvalid
		if validChargeAmount(attribution.Charge.Amount) && validChargeCurrency(attribution.Charge.Currency) {
			call.Charge = InferenceCharge{Amount: attribution.Charge.Amount, Currency: attribution.Charge.Currency}
			call.ChargeStatus = InferenceChargeReported
		}
	}
	return call
}

func validChargeAmount(value string) bool {
	if value == "" || len([]rune(value)) > InferenceMaxChargeAmountRunes {
		return false
	}
	dot := false
	digitsBefore, digitsAfter := 0, 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			if dot {
				digitsAfter++
			} else {
				digitsBefore++
			}
		case r == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digitsBefore > 0 && (!dot || digitsAfter > 0)
}

func validChargeCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func boundedInferenceText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
