package openroutermedia

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

// ErrRouteMismatch reports that a charged response did not prove the exact
// requested model, upstream provider, selected endpoint, and one-attempt route.
var ErrRouteMismatch = errors.New("OpenRouter structured response route mismatch")

// ErrChargeExceedsReservation reports a known provider charge that exceeded
// the caller's accounting reservation. The response remains billed evidence,
// but its structured output is not usable.
var ErrChargeExceedsReservation = errors.New("OpenRouter structured response charge exceeds accounting reservation")

type structuredResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64       `json:"prompt_tokens"`
		CompletionTokens int64       `json:"completion_tokens"`
		Cost             json.Number `json:"cost"`
	} `json:"usage"`
	Metadata struct {
		Attempt   int `json:"attempt"`
		Endpoints struct {
			Available []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Selected bool   `json:"selected"`
			} `json:"available"`
		} `json:"endpoints"`
		Attempts []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Status   int    `json:"status"`
		} `json:"attempts"`
	} `json:"openrouter_metadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// StatusError retains a bounded non-success provider response.
type StatusError struct {
	StatusCode int
	Detail     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("OpenRouter structured call returned status %d: %s", e.StatusCode, e.Detail)
}

func settleResponse(result Result, raw []byte, config Config) (Result, error) {
	var wire structuredResponse
	if err := decodeProviderJSON(raw, &wire); err != nil {
		return result, err
	}
	result.GenerationID = wire.ID
	result.PromptTokens = wire.Usage.PromptTokens
	result.CompletionTokens = wire.Usage.CompletionTokens
	result.ChargedAmountUSD = wire.Usage.Cost.String()
	charged, err := fillereval.USDToNanoCeil(result.ChargedAmountUSD)
	if err != nil || charged < 0 {
		return result, fmt.Errorf("OpenRouter structured call returned missing or malformed cost")
	}
	result.ChargedNanoUSD, result.ChargeKnown = charged, true
	if charged > config.ReservationNanoUSD {
		result.OverReservationNanoUSD = charged - config.ReservationNanoUSD
	}
	if wire.Error != nil {
		providerErr := fmt.Errorf("OpenRouter structured call error: %s", strings.TrimSpace(wire.Error.Message))
		if result.OverReservationNanoUSD > 0 {
			return result, errors.Join(providerErr, ErrChargeExceedsReservation)
		}
		return result, providerErr
	}
	if wire.ID == "" || wire.Model != config.Model || len(wire.Choices) != 1 || wire.Metadata.Attempt != 1 || !validAttemptLedger(wire, config) || !selectedEndpoint(wire, config) {
		return result, fmt.Errorf("%w: does not bind the requested one-attempt route (generation=%t model=%q choices=%d attempt=%d attempts=%s selected=%s)", ErrRouteMismatch, wire.ID != "", wire.Model, len(wire.Choices), wire.Metadata.Attempt, attemptSummary(wire), endpointSummary(wire))
	}
	if result.OverReservationNanoUSD > 0 {
		return result, fmt.Errorf("%w: charged %d nano-USD against %d reserved", ErrChargeExceedsReservation, charged, config.ReservationNanoUSD)
	}
	result.StructuredOutput = wire.Choices[0].Message.Content
	result.ReasoningBytes = len(wire.Choices[0].Message.Reasoning)
	return result, nil
}

func decodeProviderJSON(raw []byte, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing provider JSON value")
	}
	return nil
}

func validAttemptLedger(wire structuredResponse, config Config) bool {
	if len(wire.Metadata.Attempts) == 0 {
		return true
	}
	if len(wire.Metadata.Attempts) != 1 {
		return false
	}
	attempt := wire.Metadata.Attempts[0]
	return attempt.Provider == config.UpstreamProvider && attempt.Model == config.ResolvedModel && attempt.Status >= 200 && attempt.Status < 300
}

func selectedEndpoint(wire structuredResponse, config Config) bool {
	selected := 0
	for _, endpoint := range wire.Metadata.Endpoints.Available {
		if !endpoint.Selected {
			continue
		}
		selected++
		if endpoint.Provider != config.UpstreamProvider || endpoint.Model != config.ResolvedModel {
			return false
		}
	}
	return selected == 1
}

func attemptSummary(wire structuredResponse) string {
	parts := make([]string, 0, len(wire.Metadata.Attempts))
	for _, attempt := range wire.Metadata.Attempts {
		parts = append(parts, fmt.Sprintf("%q/%q/%d", attempt.Provider, attempt.Model, attempt.Status))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func endpointSummary(wire structuredResponse) string {
	parts := make([]string, 0, len(wire.Metadata.Endpoints.Available))
	for _, endpoint := range wire.Metadata.Endpoints.Available {
		if endpoint.Selected {
			parts = append(parts, fmt.Sprintf("%q/%q", endpoint.Provider, endpoint.Model))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func boundedMessage(raw []byte) string {
	message := strings.TrimSpace(string(raw))
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
