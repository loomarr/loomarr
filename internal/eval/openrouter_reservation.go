//go:build eval

package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/suggest"
)

const (
	openRouterSnapshotPathEnv  = "LOOMARR_EVAL_OPENROUTER_SNAPSHOT"
	openRouterSnapshotSHAEnv   = "LOOMARR_EVAL_OPENROUTER_SNAPSHOT_SHA256"
	generatorMaxInputTokensEnv = "LOOMARR_EVAL_GENERATOR_MAX_INPUT_TOKENS"
	judgeMaxInputTokensEnv     = "LOOMARR_EVAL_JUDGE_MAX_INPUT_TOKENS"
)

func openRouterReservationsFromEnv(getenv func(string) string, generator, judge CertificationProviderConfig, at time.Time) (*OpenRouterReservationConfig, *OpenRouterReservationConfig, error) {
	generatorHosted := normalizedProviderIdentity(generator.Provider) == "openrouter"
	judgeHosted := normalizedProviderIdentity(judge.Provider) == "openrouter"
	if !generatorHosted && !judgeHosted {
		return nil, nil, nil
	}
	path := strings.TrimSpace(getenv(openRouterSnapshotPathEnv))
	digest := strings.TrimSpace(getenv(openRouterSnapshotSHAEnv))
	if path == "" || digest == "" {
		return nil, nil, fmt.Errorf("%s and %s are required for snapshot-derived OpenRouter reservations", openRouterSnapshotPathEnv, openRouterSnapshotSHAEnv)
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read OpenRouter reservation snapshot: %w", err)
	}
	var snapshot fillerbakeoff.OpenRouterSnapshot
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, nil, fmt.Errorf("decode OpenRouter reservation snapshot: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, nil, fmt.Errorf("OpenRouter reservation snapshot has trailing data")
	}
	loadRole := func(enabled bool, role, rawInput string, provider CertificationProviderConfig, completion int, parameters []string) (*OpenRouterReservationConfig, error) {
		if !enabled {
			return nil, nil
		}
		input, err := strconv.Atoi(strings.TrimSpace(rawInput))
		if err != nil || input <= 0 {
			return nil, fmt.Errorf("%s maximum input tokens must be a positive integer", role)
		}
		return &OpenRouterReservationConfig{
			Snapshot: snapshot, SnapshotSHA256: digest, At: at,
			Model: provider.Model, UpstreamProvider: provider.UpstreamProvider,
			MaxInputTokens: input, MaxCompletionTokens: completion,
			RequiredParameters: parameters,
		}, nil
	}
	generatorReservation, err := loadRole(
		generatorHosted, "generator", getenv(generatorMaxInputTokensEnv), generator,
		suggest.ProductionBounds().MaxCompletionTokens,
		[]string{"response_format", "structured_outputs", "tools"},
	)
	if err != nil {
		return nil, nil, err
	}
	judgeReservation, err := loadRole(
		judgeHosted, "judge", getenv(judgeMaxInputTokensEnv), judge, judgeMaxTokens,
		[]string{"response_format", "structured_outputs"},
	)
	if err != nil {
		return nil, nil, err
	}
	return generatorReservation, judgeReservation, nil
}
