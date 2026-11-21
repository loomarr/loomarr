package app

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/filler/capability"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/openroutermedia"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// growthReviewNanoUSD is what one review question reserves on a hosted model: a short text
// question and a few words back, priced generously so the day's cap holds.
const growthReviewNanoUSD = 10_000_000 // one cent

// growthSpend reserves growth's review questions in the filler inference ledger, so the one daily
// AI spending cap covers both (#3838 decision 19, #4140 decision 33). A self-hosted model costs
// nothing and is never held.
type growthSpend struct {
	ledger capability.InferenceLedger
	set    settings.Reader
	model  meteredModel
	newID  func() string
}

// meteredModel is what the spend needs to know of the model: who serves it, and whether that is
// an operator-run endpoint (*llm.Swappable).
type meteredModel interface {
	Selection() llm.Selection
	SelfHosted() bool
}

func (s growthSpend) ReserveReview(ctx context.Context, channelID string, key provision.Key, at time.Time) (string, bool, error) {
	if s.model.SelfHosted() {
		return "", true, nil
	}
	sel := s.model.Selection()
	daily := int64(max(settings.FillerDefectsJudgeDailyCents.Get(s.set), 0)) * 10_000_000
	ev, err := s.ledger.ReserveInferenceEvaluation(ctx, records.InferenceEvaluation{
		ID: s.newID(), ClipHash: "growth:" + channelID + ":" + string(key), Role: "growth_review", Rung: "review",
		RequestedProvider: sel.Provider, RequestedModel: sel.Model, Modalities: []string{"text"},
		ReservedNanoUSD: growthReviewNanoUSD,
		Versions: records.InferenceVersions{
			Evidence: "growth-line-v1", Extractor: "growth-line-v1", Prompt: "growth-review-v1", Schema: "growth-review-v1",
			Taxonomy: "growth-line-v1", AdmissionPolicy: "growth-review-v1", RolePolicy: "growth_review",
			CapabilitySnapshot: sel.Provider + "/" + sel.Model,
		},
		CreatedAt: at,
	}, records.InferenceBudget{PerClipNanoUSD: math.MaxInt64, PerDayNanoUSD: daily})
	if err != nil {
		return "", false, err
	}
	if ev.State == records.InferenceHeldBudget {
		return "", false, nil
	}
	return ev.ID, true, nil
}

// SettleReview records what a question cost. A reported USD charge is recorded as charged. A call
// that reported none counts at its reservation, failed or not, because a provider may bill a call
// that timed out and the day's cap must not understate spend.
func (s growthSpend) SettleReview(ctx context.Context, id string, cost llm.Attribution, failed bool, at time.Time) error {
	if id == "" {
		return nil // self-hosted: nothing was reserved
	}
	st := records.InferenceSettlement{
		ResolvedProvider: cost.ResolvedProvider, ResolvedModel: cost.ResolvedModel,
		Tokens:    records.InferenceTokens{Prompt: int64(cost.Tokens.Prompt), Completion: int64(cost.Tokens.Completion)},
		LatencyMS: cost.Latency.Milliseconds(), Attempts: max(cost.Attempts, 1), GenerationID: cost.GenerationID,
		State: records.InferenceCompleted, UpdatedAt: at,
	}
	if failed {
		st.State, st.FailureReason = records.InferenceFailed, "no usable verdict"
	}
	switch {
	case cost.Charge != nil && strings.EqualFold(cost.Charge.Currency, "USD"):
		nano, err := openroutermedia.USDToNanoCeil(cost.Charge.Amount)
		if err != nil {
			return err
		}
		st.ChargedAmount, st.ChargedCurrency, st.ChargedNanoUSD, st.EstimatedNanoUSD = cost.Charge.Amount, "USD", nano, nano
	case failed:
		st.RetainReservation = true
	default:
		st.ChargedNanoUSD, st.EstimatedNanoUSD = growthReviewNanoUSD, growthReviewNanoUSD
	}
	_, err := s.ledger.SettleInferenceEvaluation(ctx, id, st)
	if errors.Is(err, records.ErrInferenceBudgetExceeded) {
		return nil // recorded as held; the verdict was paid for and stands
	}
	return err
}

// growthReview wires the background review of what channels grew by. ok is false when the model
// is not the live, swappable selection (a test override), and the review is not registered. With no
// model selected, each run asks nothing, and the fill lets unclear fits join a short channel by
// itself (decision 8).
func growthReview(st records.Store, provider llm.Provider, reconciler edit.Reconciler, busy func() bool, set settings.Reader, log *slog.Logger) (*growth.Review, bool) {
	sw, ok := provider.(*llm.Swappable)
	if !ok {
		return nil, false
	}
	editor := &edit.Service{Store: st, Reconciler: reconciler, Log: log}
	spend := growthSpend{ledger: st, set: set, model: sw, newID: newID}
	return growth.NewReview(st, sw, editor, spend, busy, time.Now, log), true
}
