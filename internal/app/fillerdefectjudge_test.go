package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/llm"
)

// Each hosted look is reserved at the per-look ceiling under the daily one and settled at its cost;
// an unreported hosted charge counts at the reservation; a self-hosted Ollama never waits.
func TestFillerDefectJudgeLedger_HoldsHostedLooksAtTheDailyCeiling(t *testing.T) {
	ctx := t.Context()
	st, err := fillerstore.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ledger := fillerDefectJudgeLedger{st: st, set: visionSet(t, map[string]string{
		"filler.defects.judge_look_cents": "2", "filler.defects.judge_daily_cents": "5",
	})}
	at := time.Unix(1_800_000_000, 0).UTC()
	look := func(provider string, a llm.Attribution) bool {
		t.Helper()
		id, held, err := ledger.ReserveDefectJudge(ctx, "clip-1", provider, "m1", at)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			if err := ledger.SettleDefectJudge(ctx, id, a, false, at); err != nil {
				t.Fatal(err)
			}
		}
		return held
	}
	charged := llm.Attribution{RequestedProvider: "openrouter", Charge: &llm.Money{Amount: "0.015", Currency: "USD"}}
	unreported := llm.Attribution{RequestedProvider: "openrouter"}
	// 1.5c + 2c (unreported, counted at the reservation) = 3.5c; the next 2c look would pass 5c.
	if look("openrouter", charged) || look("openrouter", unreported) {
		t.Fatal("a look inside the daily ceiling was held")
	}
	if !look("openrouter", charged) {
		t.Fatal("a look past the daily ceiling was not held")
	}
	if look("ollama", llm.Attribution{SelfHosted: true}) {
		t.Fatal("a self-hosted look waited on the hosted ceiling")
	}

	off := fillerDefectJudgeLedger{st: st, set: visionSet(t, map[string]string{"filler.defects.judge_look_cents": "0"})}
	if _, held, err := off.ReserveDefectJudge(ctx, "clip-2", "openrouter", "m1", at); err != nil || !held {
		t.Fatalf("a zero per-look allowance = held %v err %v, want hosted looks held", held, err)
	}
}

func TestDefectJudgeRoute_IsTheVisionModelWhenVisionIsOn(t *testing.T) {
	if _, _, err := defectJudgeRoute(visionSet(t, map[string]string{
		"llm.provider": "ollama", "llm.url": "http://localhost:11434", "llm.model": "gemma",
	}))(); !errors.Is(err, filler.ErrNoDefectJudge) {
		t.Fatalf("vision off = %v, want no judge", err)
	}
	provider, model, err := defectJudgeRoute(visionSet(t, map[string]string{
		"filler.vision.enabled": "true", "llm.provider": "ollama", "llm.url": "http://localhost:11434",
		"llm.model": "qwen", "filler.vision.model": "gemma",
	}))()
	if err != nil || provider != "ollama" || model != "gemma" {
		t.Fatalf("vision on = %s/%s %v, want ollama/gemma", provider, model, err)
	}
}
