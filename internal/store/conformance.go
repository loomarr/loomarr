package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// NewStoreFunc builds a fresh, migrated, empty Store for one test. The SQLite
// and Postgres test files each supply one; RunConformance runs the SAME
// assertions against both (CLAUDE.md: one suite, two backends — never forked).
type NewStoreFunc func(t *testing.T) Store

// RunConformance is the single store conformance suite. Every backend must pass
// it identically. Kept in a non-_test.go file so both backends' test packages
// can call it.
func RunConformance(t *testing.T, newStore NewStoreFunc) {
	t.Run("TitleRoundTrip", func(t *testing.T) { testTitleRoundTrip(t, newStore) })
	t.Run("UpsertIsIdempotent", func(t *testing.T) { testUpsertIdempotent(t, newStore) })
	t.Run("GetMissingIsNotFound", func(t *testing.T) { testGetMissing(t, newStore) })
	t.Run("ListByState", func(t *testing.T) { testListByState(t, newStore) })
	t.Run("ClaimDueTitles", func(t *testing.T) { testClaimDue(t, newStore) })
	t.Run("ClaimDueConcurrent", func(t *testing.T) { testClaimConcurrent(t, newStore) })
	t.Run("SettingsKV", func(t *testing.T) { testSettings(t, newStore) })
}

func sampleRecord(key provision.Key, state provision.State, deadline time.Time) provision.Record {
	return provision.Record{
		Key:         key,
		Title:       provision.Title{MediaType: provision.Movie, TMDBID: 1111867, Name: "In Flames", Year: 2023},
		State:       state,
		Deadline:    deadline,
		RequestedAt: time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt:   time.Unix(1_700_000_000, 0).UTC(),
	}
}

func testTitleRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	want := sampleRecord("movie:tmdb:1111867", provision.Requested, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertTitle(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTitle(ctx, want.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != want.Key || got.State != want.State || got.Title.TMDBID != want.Title.TMDBID {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if !got.Deadline.Equal(want.Deadline) || !got.RequestedAt.Equal(want.RequestedAt) {
		t.Errorf("epoch time round-trip lost precision: got dl=%v ra=%v", got.Deadline, got.RequestedAt)
	}
}

func testUpsertIdempotent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	rec := sampleRecord("movie:tmdb:1", provision.Wanted, time.Time{})
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.State = provision.Available
	rec.LibraryID = "999"
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err) // second upsert on same key must update, not error
	}
	got, _ := s.GetTitle(ctx, rec.Key)
	if got.State != provision.Available || got.LibraryID != "999" {
		t.Errorf("upsert didn't overwrite: %+v", got)
	}
	all, _ := s.ListTitlesByState(ctx, provision.Available)
	if len(all) != 1 {
		t.Errorf("upsert created a duplicate row: %d available", len(all))
	}
}

func testGetMissing(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	if _, err := s.GetTitle(context.Background(), "movie:tmdb:404"); err != ErrNotFound {
		t.Errorf("GetTitle(missing) = %v, want ErrNotFound", err)
	}
}

func testListByState(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:10", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:11", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:12", provision.Available, time.Time{}))
	req, err := s.ListTitlesByState(ctx, provision.Requested)
	if err != nil {
		t.Fatal(err)
	}
	if len(req) != 2 {
		t.Errorf("ListTitlesByState(requested) = %d, want 2", len(req))
	}
}

func testClaimDue(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	// Due: requested, deadline in the past.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:20", provision.Requested, now.Add(-time.Hour)))
	// Not due: deadline in the future.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:21", provision.Requested, now.Add(time.Hour)))
	// Not eligible: terminal state, even though deadline is past.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:22", provision.Available, now.Add(-time.Hour)))

	claimed, err := s.ClaimDueTitles(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Key != "movie:tmdb:20" {
		t.Errorf("ClaimDueTitles returned %d rows, want just movie:tmdb:20: %+v", len(claimed), claimed)
	}
	// The claimed row is now leased (deadline pushed to now+lease); a second
	// claim at the same `now` must NOT re-return it.
	again, err := s.ClaimDueTitles(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased rows, want 0 (lease not honored)", len(again))
	}
	// The claim leased the row: its stored deadline is now pushed to now+lease.
	got, _ := s.GetTitle(ctx, "movie:tmdb:20")
	if !got.Deadline.Equal(now.Add(time.Minute)) {
		t.Errorf("claimed row deadline = %v, want leased %v", got.Deadline, now.Add(time.Minute))
	}
}

// testClaimConcurrent is the reason ClaimDue is a distinct method (§5): under
// concurrency, each due row is claimed at most once across callers. On SQLite
// (single-writer) this is trivially true; on Postgres it exercises SKIP LOCKED.
func testClaimConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		key := provision.Key("movie:tmdb:" + string(rune('a'+i%26)) + string(rune('0'+i/26)))
		_ = s.UpsertTitle(ctx, sampleRecord(key, provision.Requested, now.Add(-time.Hour)))
	}

	var mu sync.Mutex
	seen := map[provision.Key]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueTitles(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, r := range batch {
					seen[r.Key]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Errorf("claimed %d distinct rows, want %d", len(seen), n)
	}
	for k, c := range seen {
		if c != 1 {
			t.Errorf("row %s claimed %d times, want exactly 1", k, c)
		}
	}
}

func testSettings(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetSetting(ctx, "instance_id"); err != ErrNotFound {
		t.Errorf("GetSetting(missing) = %v, want ErrNotFound", err)
	}
	if err := s.SetSetting(ctx, "instance_id", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "instance_id", "def456"); err != nil {
		t.Fatal(err) // upsert
	}
	v, err := s.GetSetting(ctx, "instance_id")
	if err != nil || v != "def456" {
		t.Errorf("GetSetting = %q,%v want def456,nil", v, err)
	}
}
