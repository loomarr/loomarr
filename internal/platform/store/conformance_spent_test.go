package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/proposals"
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
		{ID: "a1", Target: "movie:tmdb:90000101", Action: FeedbackKeep, CreatedAt: at(0)},
		{ID: "a2", Target: "movie:tmdb:90000101", Action: FeedbackNever, CreatedAt: at(1)},
		{ID: "b1", Target: "movie:tmdb:90000102", Action: FeedbackKeep, CreatedAt: at(0)},
		{ID: "b2", Target: "movie:tmdb:90000102", Action: FeedbackClear, CreatedAt: at(1)},
		{ID: "c1", Target: "movie:tmdb:90000103", Action: FeedbackLess, CreatedAt: at(0)},
		{ID: "d1", Target: "movie:tmdb:90000103", Action: FeedbackClear, CreatedAt: at(2), Scope: FeedbackChannel, ScopeID: "ch-1"},
		{ID: "e1", Target: "movie:tmdb:90000104", Action: FeedbackKeep, CreatedAt: at(3), Scope: FeedbackChannel, ScopeID: "ch-1"},
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

	// Requesters: what was asked for and who asked are foreign keys (#2793), so no sweep is
	// needed. A request for a title that isn't there is refused, and a person's requests go with them.
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		return
	}
	for _, id := range []string{"u-1", "u-2"} {
		if err := st.UpsertUser(ctx, User{ID: id, Name: id, Role: RoleMember, CreatedAt: old, UpdatedAt: old}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.UpsertTitle(ctx, sampleRecord(provision.Key("movie:tmdb:90000105"), provision.Requested, time.Time{})); err != nil {
		t.Fatal(err)
	}
	request := func(kind notifications.ReferenceKind, id, person string) error {
		return identitystore.InsertNotificationReferenceRecipient(ctx, sqlSt.db, sqlSt.ph, kind, id, person, old.Unix())
	}
	for _, r := range []struct {
		kind   notifications.ReferenceKind
		id     string
		person string
	}{
		{notifications.ReferenceTitle, "movie:tmdb:90000105", "u-1"},
		{notifications.ReferenceTitle, "movie:tmdb:90000105", "u-2"},
		{notifications.ReferenceChannel, "ch-1", "u-2"},
	} {
		if err := request(r.kind, r.id, r.person); err != nil {
			t.Fatal(err)
		}
	}
	if err := request(notifications.ReferenceTitle, "movie:tmdb:90000106", "u-1"); err == nil {
		t.Error("a request for a title that isn't written was recorded")
	}
	if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`DELETE FROM users WHERE id = ?`), "u-2"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind notifications.ReferenceKind
		id   string
		want []string
	}{
		{notifications.ReferenceTitle, "movie:tmdb:90000105", []string{"u-1"}},
		{notifications.ReferenceChannel, "ch-1", []string{}},
	} {
		if got, err := st.ListNotificationReferenceRecipients(ctx, c.kind, c.id); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s requesters after u-2 went = %v, %v; want %v", c.kind, got, err, c.want)
		}
	}
	if _, err := st.GetDevicePairing(ctx, "expired", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired pairing lookup = %v, want ErrNotFound", err)
	}
}

// A settled title nothing names goes once it is old; one a channel's lineup or an open request
// names stays at any age, and so does one still in flight.
func testUnusedTitlePurge(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	before := time.Unix(1_750_000_000, 0).UTC() // sampleRecord's titles changed at 1_700_000_000

	if _, err := st.SaveChannel(ctx, sampleChannel("ch-1", 7, time.Time{})); err != nil {
		t.Fatal(err)
	}
	picked := proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000107, Name: "Picked"}
	if err := st.CreateProposal(ctx, Proposal{ID: "open", Status: "submitted", CreatedAt: before,
		Body: proposals.Proposal{Alternates: []proposals.ProposalItem{picked}}}); err != nil {
		t.Fatal(err)
	}
	recent := sampleRecord("movie:tmdb:90000104", provision.Available, time.Time{})
	recent.UpdatedAt = before.Add(time.Hour)
	for _, r := range []provision.Record{
		sampleRecord("movie:tmdb:90000001", provision.Available, time.Time{}),   // kept: in ch-1's lineup
		sampleRecord("movie:tmdb:90000107", provision.Available, time.Time{}),   // kept: an open request's pick
		sampleRecord("movie:tmdb:90000101", provision.Available, time.Time{}),   // gone
		sampleRecord("movie:tmdb:90000102", provision.Unavailable, time.Time{}), // gone
		sampleRecord("movie:tmdb:90000103", provision.Requested, time.Time{}),   // kept: in flight
		recent, // kept: changed inside the window
	} {
		if err := st.UpsertTitle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.PurgeUnusedTitles(ctx, before); err != nil || n != 2 {
		t.Fatalf("purge unused titles = %d, %v; want 2", n, err)
	}
	for key, kept := range map[provision.Key]bool{
		"movie:tmdb:90000001": true, "movie:tmdb:90000107": true, "movie:tmdb:90000101": false,
		"movie:tmdb:90000102": false, "movie:tmdb:90000103": true, "movie:tmdb:90000104": true,
	} {
		_, err := st.GetTitle(ctx, key)
		if kept && err != nil {
			t.Errorf("%s was purged: %v", key, err)
		}
		if !kept && !errors.Is(err, ErrNotFound) {
			t.Errorf("%s lookup = %v, want ErrNotFound", key, err)
		}
	}
}
