package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// Titles and provisioning (§4, §5): the record round-trip, the state machine's list
// queries, and the ClaimDue lease that makes the reconciler safe to run twice.

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

// testTitlesAvailableSince: arrivals (#1663) round-trip, list newest first from `since`, and
// survive a later upsert that carries no stamp (suggest approve re-writes an in-library pick as
// available with a zero AvailableAt; that must not erase when the title arrived).
func testTitlesAvailableSince(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	day := func(n int) time.Time { return time.Unix(1_800_000_000, 0).UTC().Add(time.Duration(n) * 24 * time.Hour) }
	arrived := func(key provision.Key, at time.Time) provision.Record {
		r := sampleRecord(key, provision.Available, time.Time{})
		r.AvailableAt = at
		return r
	}
	for _, r := range []provision.Record{
		arrived("movie:tmdb:1", day(0)),
		arrived("movie:tmdb:2", day(5)),
		arrived("movie:tmdb:3", day(3)),
		sampleRecord("movie:tmdb:4", provision.Available, time.Time{}), // an in-library pick: never arrived
		sampleRecord("movie:tmdb:5", provision.Downloading, day(9)),
	} {
		if err := s.UpsertTitle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	// The approve path re-writes #2 without a stamp.
	if err := s.UpsertTitle(ctx, sampleRecord("movie:tmdb:2", provision.Available, time.Time{})); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTitle(ctx, "movie:tmdb:2")
	if err != nil {
		t.Fatal(err)
	}
	if !got.AvailableAt.Equal(day(5)) {
		t.Fatalf("AvailableAt after an unstamped upsert = %v, want %v", got.AvailableAt, day(5))
	}
	recs, err := s.ListTitlesAvailableSince(ctx, day(2))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, r := range recs {
		keys = append(keys, string(r.Key)+"@"+r.AvailableAt.Sub(day(0)).String())
	}
	if want := "movie:tmdb:2@120h0m0s movie:tmdb:3@72h0m0s"; strings.Join(keys, " ") != want {
		t.Fatalf("available since day 2 = %v, want %s", keys, want)
	}
}

// testUpdateTitleProgress proves the targeted progress write persists the download fields AND
// leaves the state-machine columns untouched (§18.1 — the poller must not clobber state).
func testUpdateTitleProgress(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	rec := sampleRecord("movie:tmdb:603", provision.Downloading, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTitleProgress(ctx, rec.Key, TitleProgress{
		Progress: 0.42, ETAText: "00:14:32", Status: "downloading", EpisodesHave: 8, EpisodesWanted: 36,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTitle(ctx, rec.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Progress != 0.42 || got.ETAText != "00:14:32" || got.DownloadStatus != "downloading" ||
		got.EpisodesHave != 8 || got.EpisodesWanted != 36 {
		t.Errorf("progress not persisted: got %+v", got)
	}
	// Every titles read carries the counts, not only GetTitle: Home's On the way lists by state.
	byState, err := s.ListTitlesByState(ctx, provision.Downloading)
	if err != nil {
		t.Fatal(err)
	}
	if len(byState) != 1 || byState[0].EpisodesHave != 8 || byState[0].EpisodesWanted != 36 {
		t.Errorf("ListTitlesByState lost the episode counts: %+v", byState)
	}
	// State-machine fields survive the targeted write.
	if got.State != provision.Downloading || !got.Deadline.Equal(rec.Deadline) {
		t.Errorf("progress write clobbered state/deadline: got state=%s dl=%v", got.State, got.Deadline)
	}
	// Updating with zeros clears it (e.g. an import completed) without touching state.
	if err := s.UpdateTitleProgress(ctx, rec.Key, TitleProgress{}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTitle(ctx, rec.Key)
	if got.Progress != 0 || got.ETAText != "" || got.EpisodesHave != 0 || got.EpisodesWanted != 0 ||
		got.State != provision.Downloading {
		t.Errorf("progress reset failed or clobbered state: got %+v", got)
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
