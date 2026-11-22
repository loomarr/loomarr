package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/testkit"
)

type fakeModel struct{ selfHosted bool }

func (f fakeModel) Selection() llm.Selection {
	return llm.Selection{Provider: "openai", Model: "judge-1"}
}
func (f fakeModel) SelfHosted() bool { return f.selfHosted }

// On a hosted model each question is reserved, then settled at what it cost, so nothing stays
// held as "reserved"; a call that failed keeps its price counted; and the day's cap, shared with
// filler, holds the question past it.
func TestGrowthReviewSpendSettlesEachQuestionAndHoldsAtTheCap(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	ids := 0
	spend := growthSpend{ledger: st, set: visionSet(t, map[string]string{"filler.defects.judge_daily_cents": "2"}),
		model: fakeModel{}, newID: func() string { ids++; return fmt.Sprintf("ev-%d", ids) }}

	first, ok, err := spend.ReserveReview(ctx, "ch-1", "series:tvdb:90000801", at)
	if err != nil || !ok || first == "" {
		t.Fatalf("first question: id %q ok %v err %v, want it reserved", first, ok, err)
	}
	if err := spend.SettleReview(ctx, first, llm.Attribution{}, false, at); err != nil {
		t.Fatal(err)
	}
	second, ok, err := spend.ReserveReview(ctx, "ch-1", "series:tvdb:90000802", at)
	if err != nil || !ok {
		t.Fatalf("second question: ok %v err %v, want it reserved under the cap", ok, err)
	}
	if err := spend.SettleReview(ctx, second, llm.Attribution{}, true, at); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := spend.ReserveReview(ctx, "ch-1", "series:tvdb:90000803", at); err != nil || ok {
		t.Fatalf("third question: ok %v err %v, want it held at the two-cent cap", ok, err)
	}

	for _, id := range []string{first, second} {
		ev, err := st.GetInferenceEvaluation(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if ev.State == records.InferenceReserved || ev.ReservedNanoUSD != growthReviewNanoUSD {
			t.Errorf("%s: state %s, counted %d; want it settled at its price", id, ev.State, ev.ReservedNanoUSD)
		}
	}
}

// A self-hosted model costs nothing: nothing is reserved, and nothing waits on the cap.
func TestGrowthReviewSpendNeverHoldsASelfHostedModel(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	spend := growthSpend{ledger: st, set: visionSet(t, map[string]string{"filler.defects.judge_daily_cents": "0"}),
		model: fakeModel{selfHosted: true}, newID: func() string { return "never" }}
	id, ok, err := spend.ReserveReview(context.Background(), "ch-1", "series:tvdb:90000801", time.Now())
	if err != nil || !ok || id != "" {
		t.Fatalf("id %q ok %v err %v, want an unmetered question", id, ok, err)
	}
	if err := spend.SettleReview(context.Background(), id, llm.Attribution{}, false, time.Now()); err != nil {
		t.Fatal(err)
	}
}
