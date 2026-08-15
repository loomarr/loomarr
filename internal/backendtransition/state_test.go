package backendtransition

import (
	"context"
	"errors"
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestLoadInitializesEmptyFleetFromDesired(t *testing.T) {
	tests := []struct {
		desired           string
		publishedInternal bool
		wantJSON          string
	}{
		{desired: BackendInternal, publishedInternal: true, wantJSON: `{"version":1,"applied":"internal","prepared":""}`},
		{desired: BackendTunarr, publishedInternal: false, wantJSON: `{"version":1,"applied":"tunarr","prepared":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.desired, func(t *testing.T) {
			st := testkit.SQLiteStore(t)
			state, err := Load(context.Background(), st, tt.desired)
			if err != nil {
				t.Fatal(err)
			}
			assertState(t, state, tt.desired, "", tt.publishedInternal)

			raw, err := st.GetSetting(context.Background(), stateKey)
			if err != nil {
				t.Fatal(err)
			}
			if raw != tt.wantJSON {
				t.Fatalf("persisted state = %q, want %q", raw, tt.wantJSON)
			}
		})
	}
}

func TestLoadInitializesPreExistingFleetAsLegacyTunarr(t *testing.T) {
	st := testkit.SQLiteStore(t)
	_, err := st.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: "legacy", Name: "Legacy", Number: 1, Strategy: schedule.Sequential, Status: schedule.StatusLive,
	}})
	if err != nil {
		t.Fatal(err)
	}

	state, err := Load(context.Background(), st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, state, BackendTunarr, "", false)

	state, err = state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, state, BackendTunarr, BackendInternal, true)
}

func TestStatePreparesBeforePublishingAndSurvivesReload(t *testing.T) {
	st := testkit.SQLiteStore(t)
	ctx := context.Background()
	state, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.MarkPrepared(BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, st, state); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, reloaded, BackendInternal, BackendTunarr, true)

	reloaded, err = reloaded.PublishPrepared()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, st, reloaded); err != nil {
		t.Fatal(err)
	}
	final, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, final, BackendTunarr, "", false)
}

func TestLoadCorruptStateFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed", raw: `{`},
		{name: "unknown version", raw: `{"version":2,"applied":"tunarr","prepared":"tunarr"}`},
		{name: "missing backend", raw: `{"version":1,"applied":"tunarr"}`},
		{name: "unknown backend", raw: `{"version":1,"applied":"other","prepared":"tunarr"}`},
		{name: "unknown field", raw: `{"version":1,"applied":"tunarr","prepared":"tunarr","desired":"internal"}`},
		{name: "trailing value", raw: `{"version":1,"applied":"tunarr","prepared":"tunarr"}{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := testkit.SQLiteStore(t)
			ctx := context.Background()
			if err := st.SetSetting(ctx, stateKey, tt.raw); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(ctx, st, BackendInternal); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("Load error = %v, want ErrInvalidState", err)
			}
			got, err := st.GetSetting(ctx, stateKey)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.raw {
				t.Fatalf("corrupt row was replaced: got %q, want %q", got, tt.raw)
			}
		})
	}
}

func TestStateRejectsUnknownBackendAndZeroState(t *testing.T) {
	st := testkit.SQLiteStore(t)
	ctx := context.Background()
	state, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.MarkPrepared("other"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("MarkPrepared error = %v, want ErrInvalidState", err)
	}
	if _, err := (State{}).PublishPrepared(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("zero PublishPrepared error = %v, want ErrInvalidState", err)
	}
	if _, err := (State{}).CancelPrepared(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("zero CancelPrepared error = %v, want ErrInvalidState", err)
	}
	if err := Save(ctx, st, State{}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("zero Save error = %v, want ErrInvalidState", err)
	}
	if _, err := state.PublishPrepared(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("steady-state PublishPrepared error = %v, want ErrInvalidState", err)
	}
}

func TestCancelPreparedPreservesAppliedAndIsSteadyStateNoOp(t *testing.T) {
	st := testkit.SQLiteStore(t)
	state, err := Load(context.Background(), st, BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}

	steady, err := state.CancelPrepared()
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, steady, BackendTunarr, "", false)

	pending, err := state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, pending, BackendTunarr, BackendInternal, true)

	cancelled, err := pending.CancelPrepared()
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, cancelled, BackendTunarr, "", false)
}

func assertState(t testing.TB, state State, applied, prepared string, publishedInternal bool) {
	t.Helper()
	if state.Applied() != applied || state.Prepared() != prepared || state.PublishedInternal() != publishedInternal {
		t.Fatalf("state = applied %q prepared %q publishedInternal %v; want %q %q %v",
			state.Applied(), state.Prepared(), state.PublishedInternal(), applied, prepared, publishedInternal)
	}
}
