package app

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/llm"
)

// defectJudgeRole names the judge's rows in the filler inference ledger (#551).
const defectJudgeRole = "defect_judge"

const nanoUSDPerCent = 10_000_000

// defectJudgeRoute is the vision model the defect judge would use now (#2034): the same one the
// vision rung uses, re-read from settings on every call, or ErrNoDefectJudge when vision is off
// or has no model it can reach.
func defectJudgeRoute(set resolved) filler.DefectJudgeRoute {
	return func() (string, string, error) {
		if !set.boolv("filler.vision.enabled") {
			return "", "", filler.ErrNoDefectJudge
		}
		v := visionEndpoint(set)
		if v.url == "" || strings.TrimSpace(v.model) == "" || (v.provider != "openai" && v.provider != "ollama") {
			return "", "", filler.ErrNoDefectJudge
		}
		identity := v.identity
		if identity == "" {
			identity = v.provider
		}
		return identity, v.model, nil
	}
}

// fillerDefectJudgeLedger meters the judge's looks in the filler inference ledger: each look is
// reserved at the per-look ceiling under the daily ceiling, then settled at what it cost.
type fillerDefectJudgeLedger struct {
	st  fillerstore.Store
	set resolved
}

func (l fillerDefectJudgeLedger) lookNanoUSD(provider string) int64 {
	if provider == "ollama" {
		return 0 // self-hosted: nothing to reserve, and a zero reservation passes every ceiling
	}
	return int64(max(l.set.intv("filler.defects.judge_look_cents"), 0)) * nanoUSDPerCent
}

func (l fillerDefectJudgeLedger) ReserveDefectJudge(ctx context.Context, clipHash, provider, model string, at time.Time) (string, bool, error) {
	look := l.lookNanoUSD(provider)
	if look == 0 && provider != "ollama" {
		return "", true, nil // a hosted look with no allowance waits
	}
	daily := int64(max(l.set.intv("filler.defects.judge_daily_cents"), 0)) * nanoUSDPerCent
	e, err := l.st.ReserveInferenceEvaluation(ctx, fillerstore.InferenceEvaluation{
		ID: newID(), ClipHash: clipHash, Role: defectJudgeRole, Rung: "defects",
		RequestedProvider: provider, RequestedModel: model, Modalities: []string{"image", "text"},
		ReservedNanoUSD: look,
		Versions: fillerstore.InferenceVersions{
			Evidence: defects.MeasurementVersion, Extractor: defects.MeasurementVersion,
			Prompt: defects.JudgeVersion, Schema: defects.JudgeVersion, Taxonomy: defects.FloorVersion,
			AdmissionPolicy: defects.FloorVersion, RolePolicy: defectJudgeRole, CapabilitySnapshot: provider + "/" + model,
		},
		CreatedAt: at,
	}, fillerstore.InferenceBudget{
		// The per-look ceiling is the reservation itself; a clip is looked at again only when the
		// judge contract or the model changes, so no lifetime per-clip ceiling is kept.
		PerClipNanoUSD: math.MaxInt64, PerDayNanoUSD: daily,
	})
	if err != nil {
		return "", false, err
	}
	return e.ID, e.State == fillerstore.InferenceHeldBudget, nil
}

// SettleDefectJudge records what the look cost. A hosted call that reported no charge is counted
// at its reservation, so the daily ceiling never understates spend.
func (l fillerDefectJudgeLedger) SettleDefectJudge(ctx context.Context, id string, a llm.Attribution, failed bool, at time.Time) error {
	s := fillerstore.InferenceSettlement{
		ResolvedProvider: a.ResolvedProvider, ResolvedModel: a.ResolvedModel,
		Tokens: fillerstore.InferenceTokens{Prompt: int64(a.Tokens.Prompt), Completion: int64(a.Tokens.Completion),
			Image: int64(a.Tokens.Image)},
		LatencyMS: a.Latency.Milliseconds(), Attempts: max(a.Attempts, 1), GenerationID: a.GenerationID,
		State: fillerstore.InferenceCompleted, UpdatedAt: at,
	}
	switch {
	case failed:
		s.State, s.FailureReason = fillerstore.InferenceFailed, "provider call failed"
	case a.Charge != nil && strings.EqualFold(a.Charge.Currency, "USD"):
		nano, err := fillereval.USDToNanoCeil(a.Charge.Amount)
		if err != nil {
			return err
		}
		s.ChargedAmount, s.ChargedCurrency, s.ChargedNanoUSD, s.EstimatedNanoUSD = a.Charge.Amount, "USD", nano, nano
	case !a.SelfHosted:
		look := l.lookNanoUSD(a.RequestedProvider)
		s.ChargedNanoUSD, s.EstimatedNanoUSD = look, look
	}
	_, err := l.st.SettleInferenceEvaluation(ctx, id, s)
	if errors.Is(err, fillerstore.ErrInferenceBudgetExceeded) {
		return nil // recorded as held; the look itself was paid for and its ruling stands
	}
	return err
}
