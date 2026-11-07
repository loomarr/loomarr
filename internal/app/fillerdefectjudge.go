package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/openroutermedia"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// defectJudgeRole names the judge's rows in the filler inference ledger (#551).
const defectJudgeRole = "defect_judge"

const nanoUSDPerCent = 10_000_000

// defectJudgeEndpoint is the vision model the defect judge would ask now (#2034): the vision rung's,
// resolved on every call so a settings change applies to the next look. It names the model the
// endpoint actually serves, and decides "free" from the endpoint rather than the provider's name.
// Vision switched off, or configured with nothing it can reach, is no judge.
func (h *hotVisionProvider) defectJudgeEndpoint() (defectfloor.DefectJudgeEndpoint, error) {
	if !settings.FillerVisionEnabled.Get(h.set) {
		return defectfloor.DefectJudgeEndpoint{}, defectfloor.ErrNoDefectJudge
	}
	p, err := h.resolve()
	if err != nil {
		return defectfloor.DefectJudgeEndpoint{}, fmt.Errorf("%w: %w", defectfloor.ErrNoDefectJudge, err)
	}
	h.mu.Lock()
	w := h.last
	h.mu.Unlock()
	if strings.TrimSpace(w.model) == "" {
		return defectfloor.DefectJudgeEndpoint{}, fmt.Errorf("%w: no vision model named", defectfloor.ErrNoDefectJudge)
	}
	e := defectfloor.DefectJudgeEndpoint{Provider: cmp.Or(w.identity, w.provider), Model: w.model}
	if s, ok := p.(interface{ SelfHosted() bool }); ok {
		e.SelfHosted = s.SelfHosted()
	}
	return e, nil
}

// fillerDefectJudgeLedger meters the judge's looks in the filler inference ledger: each look is
// reserved at the per-look ceiling under the daily ceiling, then settled at what it cost.
type fillerDefectJudgeLedger struct {
	st  records.Store
	set resolved
}

// lookNanoUSD is what one look reserves: nothing on an operator-run endpoint, which also passes
// every ceiling, and the per-look ceiling on a hosted one.
func (l fillerDefectJudgeLedger) lookNanoUSD(e defectfloor.DefectJudgeEndpoint) int64 {
	if e.SelfHosted {
		return 0
	}
	return int64(max(settings.FillerDefectsJudgeLookCents.Get(l.set), 0)) * nanoUSDPerCent
}

func (l fillerDefectJudgeLedger) ReserveDefectJudge(ctx context.Context, clipHash string, e defectfloor.DefectJudgeEndpoint, at time.Time) (string, bool, error) {
	look := l.lookNanoUSD(e)
	if look == 0 && !e.SelfHosted {
		return "", true, nil // a hosted look with no allowance waits
	}
	daily := int64(max(settings.FillerDefectsJudgeDailyCents.Get(l.set), 0)) * nanoUSDPerCent
	ev, err := l.st.ReserveInferenceEvaluation(ctx, records.InferenceEvaluation{
		ID: newID(), ClipHash: clipHash, Role: defectJudgeRole, Rung: "defects",
		RequestedProvider: e.Provider, RequestedModel: e.Model, Modalities: []string{"image", "text"},
		ReservedNanoUSD: look,
		Versions: records.InferenceVersions{
			Evidence: defects.MeasurementVersion, Extractor: defects.MeasurementVersion,
			Prompt: defects.JudgeVersion, Schema: defects.JudgeVersion, Taxonomy: defects.FloorVersion,
			AdmissionPolicy: defects.FloorVersion, RolePolicy: defectJudgeRole, CapabilitySnapshot: e.Name(),
		},
		CreatedAt: at,
	}, records.InferenceBudget{
		// The per-look ceiling is the reservation itself; a clip is looked at again only when the
		// judge contract or the model changes, so no lifetime per-clip ceiling is kept.
		PerClipNanoUSD: math.MaxInt64, PerDayNanoUSD: daily,
	})
	if err != nil {
		return "", false, err
	}
	return ev.ID, ev.State == records.InferenceHeldBudget, nil
}

// SettleDefectJudge records what the look cost. A reported charge is recorded as charged, even on a
// failed call. A hosted call that reported none counts at its reservation, failed or not: a
// provider may bill a call that timed out, and the daily ceiling must not understate spend.
func (l fillerDefectJudgeLedger) SettleDefectJudge(ctx context.Context, id string, e defectfloor.DefectJudgeEndpoint, a llm.Attribution, failed bool, at time.Time) error {
	s := records.InferenceSettlement{
		ResolvedProvider: a.ResolvedProvider, ResolvedModel: a.ResolvedModel,
		Tokens: records.InferenceTokens{Prompt: int64(a.Tokens.Prompt), Completion: int64(a.Tokens.Completion),
			Image: int64(a.Tokens.Image)},
		LatencyMS: a.Latency.Milliseconds(), Attempts: max(a.Attempts, 1), GenerationID: a.GenerationID,
		State: records.InferenceCompleted, UpdatedAt: at,
	}
	if failed {
		s.State, s.FailureReason = records.InferenceFailed, "provider call failed"
	}
	switch {
	case a.Charge != nil && strings.EqualFold(a.Charge.Currency, "USD"):
		nano, err := openroutermedia.USDToNanoCeil(a.Charge.Amount)
		if err != nil {
			return err
		}
		s.ChargedAmount, s.ChargedCurrency, s.ChargedNanoUSD, s.EstimatedNanoUSD = a.Charge.Amount, "USD", nano, nano
	case e.SelfHosted:
	case failed:
		s.RetainReservation = true // the ledger keeps the reservation counted
	default:
		look := l.lookNanoUSD(e)
		s.ChargedNanoUSD, s.EstimatedNanoUSD = look, look
	}
	_, err := l.st.SettleInferenceEvaluation(ctx, id, s)
	if errors.Is(err, records.ErrInferenceBudgetExceeded) {
		return nil // recorded as held; the look itself was paid for and its ruling stands
	}
	return err
}
