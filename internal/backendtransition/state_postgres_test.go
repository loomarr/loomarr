//go:build integration

package backendtransition

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/testkit"
)

func TestPostgresStateRoundTrip(t *testing.T) {
	st := testkit.PostgresStore(t)
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
}
