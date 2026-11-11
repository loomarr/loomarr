package app

import (
	"context"
	"testing"

	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// settingsEventAdapter wires the real settings service, a migrated store and the real emitter, so
// the assertion reads the frame off the bus the browser's /v1/events stream is fed from.
func settingsEventAdapter(t *testing.T) (settingsAdapter, <-chan events.Event) {
	t.Helper()
	bus := events.NewBus()
	frames, unsubscribe := bus.Subscribe()
	t.Cleanup(unsubscribe)
	emit := &eventEmitter{bus: bus}
	set := visionSet(t, nil)
	st := testkit.MigratedSQLiteStore(t)
	// The tests patch as "admin", and a setting's author is a foreign key to users (#2793).
	if err := st.UpsertUser(context.Background(), store.User{ID: "admin", Name: "admin", Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	return settingsAdapter{svc: set.svc, store: st, changed: emit.SettingsChanged}, frames
}

func requireSettingsFrame(t *testing.T, frames <-chan events.Event) {
	t.Helper()
	select {
	case got := <-frames:
		if got.Type != "settings" {
			t.Fatalf("event type = %q, want settings", got.Type)
		}
		if _, ok := got.Payload.(opsapi.SettingsEvent); !ok {
			t.Fatalf("payload type = %T, want api.SettingsEvent", got.Payload)
		}
	default:
		t.Fatal("a saved settings change published no settings frame")
	}
}

func requireNoFrame(t *testing.T, frames <-chan events.Event) {
	t.Helper()
	select {
	case got := <-frames:
		t.Fatalf("unexpected %q frame: nothing was saved", got.Type)
	default:
	}
}

func TestSettingsAdapterPatchPublishesSettingsFrameWhenSaved(t *testing.T) {
	t.Parallel()
	adapter, frames := settingsEventAdapter(t)

	results := adapter.Patch(context.Background(), map[string]string{"llm.provider": "openai"}, "admin")

	if len(results) != 1 || results[0].Status != "saved" {
		t.Fatalf("Patch results = %+v, want one saved key", results)
	}
	requireSettingsFrame(t, frames)
}

func TestSettingsAdapterPatchPublishesNothingWhenNothingSaved(t *testing.T) {
	t.Parallel()
	adapter, frames := settingsEventAdapter(t)

	results := adapter.Patch(context.Background(), map[string]string{"llm.provider": "not-a-provider"}, "admin")

	if len(results) != 1 || results[0].Status != "invalid" {
		t.Fatalf("Patch results = %+v, want one invalid key", results)
	}
	requireNoFrame(t, frames)
}

func TestSettingsAdapterClearPublishesSettingsFrame(t *testing.T) {
	t.Parallel()
	adapter, frames := settingsEventAdapter(t)
	adapter.Patch(context.Background(), map[string]string{"llm.provider": "openai"}, "admin")
	requireSettingsFrame(t, frames)

	res := adapter.Clear(context.Background(), "llm.provider")

	if res.Status != "saved" {
		t.Fatalf("Clear status = %q, want saved", res.Status)
	}
	requireSettingsFrame(t, frames)
}

// Model select and pull persist llm.* through the settings service rather than the adapter, so
// the picker's own writes must announce themselves for other tabs to refetch.
func TestLLMSettingsSaverPublishesSettingsFrameWhenSaved(t *testing.T) {
	t.Parallel()
	bus := events.NewBus()
	frames, unsubscribe := bus.Subscribe()
	t.Cleanup(unsubscribe)
	set := visionSet(t, nil)
	save := llmSettingsSaver(set, testkit.MigratedSQLiteStore(t), bus)

	if err := save(context.Background(), map[string]string{"llm.provider": "openai"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	requireSettingsFrame(t, frames)

	if err := save(context.Background(), map[string]string{"llm.provider": "not-a-provider"}); err == nil {
		t.Fatal("an invalid value saved without error")
	}
	requireNoFrame(t, frames)
}
