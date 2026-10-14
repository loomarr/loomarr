package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

func clientCrashRecord(id string, receivedAt int64) diagnostics.ClientCrashRecord {
	return diagnostics.ClientCrashRecord{
		ID: id, ReceivedAt: receivedAt, OccurredAt: receivedAt - 1000, Kind: diagnostics.CrashJSFatal,
		Platform: diagnostics.CrashIOS, AppVersion: "1.4.0", AppBuild: "212", OSVersion: "26.1",
		DeviceModel: "iPhone17,3", Message: "boom " + id,
	}
}

// testClientCrashReports: a report round-trips whole on a single read, a page carries no heavy
// columns, pages run newest first across equal timestamps, and an API-token report has no user.
func testClientCrashReports(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustUpsertPreferenceUser(t, s, "u-kid")

	full := clientCrashRecord("crash_b", 2_000)
	full.UserID, full.DeviceID, full.DeviceName = "u-kid", "hash-1", "Phone"
	full.Kind, full.Stack = diagnostics.CrashNative, "frame 0"
	full.BreadcrumbsJSON = `[{"at":"2026-10-13T07:59:58Z","category":"log","message":"hi"}]`
	full.NativePayload = `{"callStackTree":{}}`
	for _, r := range []diagnostics.ClientCrashRecord{clientCrashRecord("crash_a", 1_000), full,
		clientCrashRecord("crash_c", 2_000), clientCrashRecord("crash_d", 3_000)} {
		if err := s.InsertClientCrashReport(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, ok, err := s.GetClientCrashReport(ctx, "crash_b")
	if err != nil || !ok {
		t.Fatalf("get = %v, %v", ok, err)
	}
	if got != full {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, full)
	}
	if anon, _, _ := s.GetClientCrashReport(ctx, "crash_a"); anon.UserID != "" || anon.BreadcrumbsJSON != "[]" {
		t.Fatalf("API-token report = user %q breadcrumbs %q, want none and []", anon.UserID, anon.BreadcrumbsJSON)
	}
	if _, ok, err := s.GetClientCrashReport(ctx, "crash_missing"); ok || err != nil {
		t.Fatalf("missing report = %v, %v; want absent without error", ok, err)
	}

	first, err := s.QueryClientCrashReports(ctx, diagnostics.ClientCrashStoreQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].ID != "crash_d" || first[1].ID != "crash_c" {
		t.Fatalf("first page = %v, want crash_d, crash_c", crashIDs(first))
	}
	if first[1].Stack != "" || first[1].NativePayload != "" || first[1].BreadcrumbsJSON != "" {
		t.Fatal("a page read loaded the heavy columns")
	}
	rest, err := s.QueryClientCrashReports(ctx, diagnostics.ClientCrashStoreQuery{
		Limit: 10, CursorReceivedAt: first[1].ReceivedAt, CursorID: first[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(crashIDs(rest)) != "[crash_b crash_a]" {
		t.Fatalf("second page = %v, want [crash_b crash_a]", crashIDs(rest))
	}
	if rest[0].UserID != "u-kid" || rest[0].DeviceName != "Phone" {
		t.Fatalf("page row reporter = %q/%q", rest[0].UserID, rest[0].DeviceName)
	}

	// A report naming a person who does not exist is refused, not orphaned.
	orphan := clientCrashRecord("crash_orphan", 4_000)
	orphan.UserID = "u-ghost"
	if err := s.InsertClientCrashReport(ctx, orphan); err == nil {
		t.Fatal("a report for an unknown user was stored")
	}
}

// testClientCrashRetention: the age window goes first, then the row cap keeps the newest rows,
// breaking a received-time tie by id exactly as the list does.
func testClientCrashRetention(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ms := func(d time.Duration) int64 { return base.Add(d).UnixMilli() }
	for _, r := range []diagnostics.ClientCrashRecord{
		clientCrashRecord("crash_old", ms(0)),
		clientCrashRecord("crash_1", ms(time.Hour)),
		clientCrashRecord("crash_2", ms(2*time.Hour)),
		clientCrashRecord("crash_3a", ms(3*time.Hour)),
		clientCrashRecord("crash_3b", ms(3*time.Hour)),
		clientCrashRecord("crash_4", ms(4*time.Hour)),
	} {
		if err := s.InsertClientCrashReport(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.PurgeClientCrashReports(ctx, base.Add(30*time.Minute), 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("purged %d, want 3 (one expired, two over the cap)", n)
	}
	kept, err := s.QueryClientCrashReports(ctx, diagnostics.ClientCrashStoreQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(crashIDs(kept)) != "[crash_4 crash_3b crash_3a]" {
		t.Fatalf("kept %v, want the newest three", crashIDs(kept))
	}
	if n, err := s.PurgeClientCrashReports(ctx, base, 3); err != nil || n != 0 {
		t.Fatalf("a purge within policy removed %d (%v), want 0", n, err)
	}
}

func crashIDs(records []diagnostics.ClientCrashRecord) []string {
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	return ids
}
