//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

func TestPostgresLifecycleInvalidationsAreCommitOrdered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dsn := startPostgres(t)
	writer, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	listener, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	ready := make(chan struct{})
	events := make(chan Invalidation, 8)
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- ListenInvalidations(ctx, listener, func(context.Context) error {
			close(ready)
			return nil
		}, func(_ context.Context, event Invalidation) error {
			events <- event
			return nil
		})
	}()
	select {
	case <-ready:
	case err := <-listenErr:
		t.Fatalf("listener before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not become ready")
	}

	pg := writer.(*sqlStore)
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := pg.saveChannel(ctx, tx, lifecycleTestChannel("committed", 1))
	if err != nil {
		t.Fatal(err)
	}
	assertNoInvalidation(t, events)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	event := awaitInvalidation(t, events)
	if event.Kind != InvalidationChannel || event.ChannelID != committed.ID ||
		event.Status != schedule.StatusLive {
		t.Fatalf("commit event = %+v", event)
	}

	tx, err = pg.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.saveChannel(ctx, tx, lifecycleTestChannel("rolled-back", 2)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertNoInvalidation(t, events)

	committed.Status = schedule.StatusPaused
	committed, err = writer.SaveChannel(ctx, committed)
	if err != nil {
		t.Fatal(err)
	}
	event = awaitInvalidation(t, events)
	if event.ChannelID != committed.ID || event.Status != schedule.StatusPaused {
		t.Fatalf("pause event = %+v", event)
	}

	checkpoint := `{"version":1,"applied":"tunarr","prepared":""}`
	if err := writer.SetSetting(ctx, backendTransitionSettingKey, checkpoint); err != nil {
		t.Fatal(err)
	}
	event = awaitInvalidation(t, events)
	if event.Kind != InvalidationBackend || event.Key != backendTransitionSettingKey ||
		event.Value != checkpoint {
		t.Fatalf("backend event = %+v", event)
	}

	if err := writer.DeleteChannel(ctx, committed.ID, committed.Revision); err != nil {
		t.Fatal(err)
	}
	event = awaitInvalidation(t, events)
	if event.Kind != InvalidationChannel || event.ChannelID != committed.ID ||
		event.Status != schedule.StatusDetached {
		t.Fatalf("delete event = %+v", event)
	}
}

func lifecycleTestChannel(id string, number int) Channel {
	return Channel{Channel: schedule.Channel{
		ID: id, Name: id, Number: number, Strategy: schedule.Sequential, Status: schedule.StatusLive,
	}}
}

func awaitInvalidation(t *testing.T, events <-chan Invalidation) Invalidation {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for durable invalidation")
		return Invalidation{}
	}
}

func assertNoInvalidation(t *testing.T, events <-chan Invalidation) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("received invalidation before commit/after rollback: %+v", event)
	case <-time.After(200 * time.Millisecond):
	}
}
