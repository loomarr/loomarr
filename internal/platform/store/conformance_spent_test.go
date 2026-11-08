package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// The nightly pass deletes rows nothing will read again (#2528). Each purge is held to two
// things: what it deletes is spent, and everything a reader can still see survives.
func testSpentRowPurges(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	old, recent := now.Add(-40*24*time.Hour), now.Add(-time.Hour)

	// Device pairings: an expired code goes, a live one stays.
	for _, p := range []DevicePairing{
		{DeviceCodeHash: "expired", UserCode: "AAAA-1111", CreatedAt: old, ExpiresAt: old.Add(10 * time.Minute)},
		{DeviceCodeHash: "live", UserCode: "BBBB-2222", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)},
	} {
		if err := st.CreateDevicePairing(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.PurgeExpiredDevicePairings(ctx, now); err != nil || n != 1 {
		t.Fatalf("purge expired pairings = %d, %v; want 1", n, err)
	}
	if _, err := st.GetDevicePairing(ctx, "live", now); err != nil {
		t.Errorf("the live pairing code was purged: %v", err)
	}

	// Interactive operations: finished and old go; running, or finished recently, stay.
	for _, op := range []InteractiveOperation{
		{ID: "op-done", Status: InteractiveOperationSuccess, CompletedAt: old},
		{ID: "op-failed", Status: InteractiveOperationError, CompletedAt: old},
		{ID: "op-running", Status: InteractiveOperationRunning},
		{ID: "op-fresh", Status: InteractiveOperationSuccess, CompletedAt: recent},
	} {
		op.Kind, op.Subject, op.StartedAt, op.UpdatedAt = InteractiveOperationLLMPull, "model", old, old
		if err := st.UpsertInteractiveOperation(ctx, op); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.PurgeFinishedInteractiveOperations(ctx, now.Add(-7*24*time.Hour)); err != nil || n != 2 {
		t.Fatalf("purge finished operations = %d, %v; want 2", n, err)
	}
	for id, kept := range map[string]bool{"op-done": false, "op-failed": false, "op-running": true, "op-fresh": true} {
		if _, err := st.GetInteractiveOperation(ctx, id); (err == nil) != kept {
			t.Errorf("operation %s: kept = %v, want %v", id, err == nil, kept)
		}
	}

	// Discovery feedback: a replaced choice and a clear with nothing under it go; what every
	// reader resolves is exactly the same afterwards.
	mustSaveChannel(t, st, sampleChannel("ch-1", 7, now.Add(time.Hour)))
	at := func(s int) time.Time { return old.Add(time.Duration(s) * time.Second) }
	for _, f := range []DiscoveryFeedback{
		{ID: "a1", Target: "movie:tmdb:1", Action: FeedbackKeep, CreatedAt: at(0)},
		{ID: "a2", Target: "movie:tmdb:1", Action: FeedbackNever, CreatedAt: at(1)},
		{ID: "b1", Target: "movie:tmdb:2", Action: FeedbackKeep, CreatedAt: at(0)},
		{ID: "b2", Target: "movie:tmdb:2", Action: FeedbackClear, CreatedAt: at(1)},
		{ID: "c1", Target: "movie:tmdb:3", Action: FeedbackLess, CreatedAt: at(0)},
		{ID: "d1", Target: "movie:tmdb:3", Action: FeedbackClear, CreatedAt: at(2), Scope: FeedbackChannel, ScopeID: "ch-1"},
		{ID: "e1", Target: "movie:tmdb:4", Action: FeedbackKeep, CreatedAt: at(3), Scope: FeedbackChannel, ScopeID: "ch-1"},
	} {
		f.ActorID = "admin"
		if f.Scope == "" {
			f.Scope = FeedbackHousehold
		}
		if err := st.AppendDiscoveryFeedback(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	effective := func() []DiscoveryFeedback {
		t.Helper()
		household, err := st.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackHousehold})
		if err != nil {
			t.Fatal(err)
		}
		channel, err := st.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: "ch-1"})
		if err != nil {
			t.Fatal(err)
		}
		return EffectiveDiscoveryFeedback(household, channel)
	}
	before := effective()
	if n, err := st.PurgeSupersededDiscoveryFeedback(ctx); err != nil || n != 4 {
		t.Fatalf("purge superseded feedback = %d, %v; want 4 (a1, b1, b2, d1)", n, err)
	}
	if after := effective(); !reflect.DeepEqual(after, before) {
		t.Errorf("effective feedback changed:\nbefore %+v\nafter  %+v", before, after)
	}

	// Notification recipients: a requester of a title or channel that is gone, or who is gone,
	// goes once the grace has passed; one whose title isn't written yet stays.
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		return
	}
	if err := st.UpsertUser(ctx, User{ID: "u-1", Name: "Ada", Role: RoleMember, CreatedAt: old, UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTitle(ctx, sampleRecord(provision.Key("movie:tmdb:5"), provision.Requested, time.Time{})); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		kind, id, person string
		at               time.Time
	}{
		{"title", "movie:tmdb:5", "u-1", old},    // kept: the title exists
		{"channel", "ch-1", "u-1", old},          // kept: the channel exists
		{"title", "movie:tmdb:6", "u-1", recent}, // kept: inside the grace
		{"title", "movie:tmdb:6", "u-2", old},    // gone: no such title
		{"channel", "ch-gone", "u-1", old},       // gone: no such channel
		{"channel", "ch-1", "u-gone", old},       // gone: no such person
	} {
		if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`INSERT INTO notification_reference_recipients
			(reference_kind, reference_id, person_id, created_at) VALUES (?, ?, ?, ?)`),
			r.kind, r.id, r.person, r.at.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.PurgeOrphanNotificationReferenceRecipients(ctx, now.Add(-30*24*time.Hour)); err != nil || n != 3 {
		t.Fatalf("purge orphan recipients = %d, %v; want 3", n, err)
	}
	var left int
	if err := sqlSt.db.QueryRowContext(ctx, `SELECT count(*) FROM notification_reference_recipients`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 3 {
		t.Errorf("%d recipients left, want 3", left)
	}
	if _, err := st.GetDevicePairing(ctx, "expired", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired pairing lookup = %v, want ErrNotFound", err)
	}
}
