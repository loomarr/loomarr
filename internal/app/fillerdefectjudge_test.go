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

// Each hosted look is reserved at the per-look ceiling under the daily one and settled at its cost.
// A hosted look that reported no charge, failed or not, counts at its reservation; an operator-run
// endpoint never waits, whatever its provider is called.
func TestFillerDefectJudgeLedger_HoldsHostedLooksAtTheDailyCeiling(t *testing.T) {
	ctx := t.Context()
	st, err := fillerstore.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ledger := fillerDefectJudgeLedger{st: st, set: visionSet(t, map[string]string{
		"filler.defects.judge_look_cents": "2", "filler.defects.judge_daily_cents": "7",
	})}
	hosted := filler.DefectJudgeEndpoint{Provider: "openrouter", Model: "m1"}
	ownServer := filler.DefectJudgeEndpoint{Provider: "custom", Model: "gemma", SelfHosted: true}
	at := time.Unix(1_800_000_000, 0).UTC()
	look := func(e filler.DefectJudgeEndpoint, a llm.Attribution, failed bool) bool {
		t.Helper()
		id, held, err := ledger.ReserveDefectJudge(ctx, "clip-1", e, at)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			if err := ledger.SettleDefectJudge(ctx, id, e, a, failed, at); err != nil {
				t.Fatal(err)
			}
		}
		return held
	}
	charged := llm.Attribution{Charge: &llm.Money{Amount: "0.015", Currency: "USD"}}
	// 1.5c charged + 2c unreported + 2c for a failed call the provider may still bill = 5.5c; the
	// next 2c look would pass 7c.
	if look(hosted, charged, false) || look(hosted, llm.Attribution{}, false) || look(hosted, llm.Attribution{}, true) {
		t.Fatal("a look inside the daily ceiling was held")
	}
	if !look(hosted, charged, false) {
		t.Fatal("a look past the daily ceiling was not held: a failed or unreported look was counted as free")
	}
	if look(ownServer, llm.Attribution{}, false) {
		t.Fatal("an operator-run endpoint named custom waited on the hosted ceiling")
	}

	off := fillerDefectJudgeLedger{st: st, set: visionSet(t, map[string]string{"filler.defects.judge_look_cents": "0"})}
	if _, held, err := off.ReserveDefectJudge(ctx, "clip-2", hosted, at); err != nil || !held {
		t.Fatalf("a zero per-look allowance = held %v err %v, want hosted looks held", held, err)
	}
}

func TestDefectJudgeEndpoint_IsTheVisionModelWhenVisionIsOn(t *testing.T) {
	off := &hotVisionProvider{set: visionSet(t, map[string]string{
		"llm.provider": "ollama", "llm.url": "http://localhost:11434", "llm.model": "gemma",
	})}
	if _, err := off.defectJudgeEndpoint(); !errors.Is(err, filler.ErrNoDefectJudge) {
		t.Fatalf("vision off = %v, want no judge", err)
	}
	unreachable := &hotVisionProvider{set: visionSet(t, map[string]string{"filler.vision.enabled": "true"})}
	if _, err := unreachable.defectJudgeEndpoint(); !errors.Is(err, filler.ErrNoDefectJudge) {
		t.Fatalf("vision on with no endpoint = %v, want no judge", err)
	}
	on := &hotVisionProvider{set: visionSet(t, map[string]string{
		"filler.vision.enabled": "true", "filler.vision.provider": "ollama", "filler.vision.model": "gemma",
	})}
	e, err := on.defectJudgeEndpoint()
	if err != nil || e.Provider != "ollama" || e.Model != "gemma" || !e.SelfHosted {
		t.Fatalf("vision on = %+v %v, want ollama/gemma, self-hosted", e, err)
	}
}
