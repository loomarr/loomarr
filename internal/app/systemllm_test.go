package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

// Selecting a model must HOT-APPLY, not merely land in the settings table
// (config-design §3, which names the §8.1 selection as its example of hot-apply).
//
// The regression, caught by the maintainer smoke on a genuinely fresh install: persist
// wrote through store.SetSetting, bypassing the settings service, so SetDB never ran
// and the in-memory snapshot kept the old value. The picker reported "In use" and the
// suggester really did swap (selectLocal calls swap.Set itself), but every settings
// *read* — including the wizard's `llm` check — still saw the previous value until the
// process restarted. So the wizard said "select a model in AI settings" to an operator
// who had just selected one.
//
// This asserts through Resolve (what every reader actually calls), NOT through the
// store row — reading the row back would have passed against the broken code.
func TestPersistSelection_HotAppliesToSettingsReaders(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "smoke.db"), true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	reg := settings.NewRegistry()
	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := st.ListSettings(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]settings.SettingRow, len(rows))
		for i, r := range rows {
			out[i] = settings.SettingRow{Key: r.Key, Value: r.Value, UpdatedBy: r.UpdatedBy}
		}
		return out, nil
	}}
	svc, err := settings.New(ctx, reg, loader, nil)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	// Wired exactly as buildLLM wires it — the seam under test is which write path
	// persist takes, so faking it here would test nothing.
	sut := &systemLLMService{
		saveSettings: func(ctx context.Context, edits map[string]string) error {
			results, err := svc.Patch(ctx, storePersister{st: st}, edits, "system")
			if err != nil {
				return err
			}
			for _, r := range results {
				if r.Status != settings.PatchSaved {
					t.Fatalf("Patch refused %s: %s", r.Key, r.Problem)
				}
			}
			return nil
		},
		store: st,
	}

	sel := llm.Selection{Provider: "ollama", URL: "http://localhost:11434", Model: "qwen3.5:9b"}
	if err := sut.persist(ctx, sel); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := svc.Resolve("llm.model").Value; got != sel.Model {
		t.Errorf("llm.model reads back %q, want %q — the write did not hot-apply, so the "+
			"wizard's llm check stays red until a restart", got, sel.Model)
	}
	if got := svc.Resolve("llm.provider").Value; got != sel.Provider {
		t.Errorf("llm.provider reads back %q, want %q", got, sel.Provider)
	}
}
