package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
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
	t.Run("ChannelRoundTrip", func(t *testing.T) { testChannelRoundTrip(t, newStore) })
	t.Run("ChannelListAndDelete", func(t *testing.T) { testChannelListDelete(t, newStore) })
	t.Run("ClaimDueChannels", func(t *testing.T) { testClaimDueChannels(t, newStore) })
	t.Run("ClaimDueChannelsConcurrent", func(t *testing.T) { testClaimChannelsConcurrent(t, newStore) })
	t.Run("JobRoundTrip", func(t *testing.T) { testJobRoundTrip(t, newStore) })
	t.Run("ClaimDueJobs", func(t *testing.T) { testClaimDueJobs(t, newStore) })
	t.Run("ClaimDueJobsConcurrent", func(t *testing.T) { testClaimJobsConcurrent(t, newStore) })
	t.Run("JobCacheByIntentHash", func(t *testing.T) { testJobCacheByHash(t, newStore) })
	t.Run("ProposalRoundTripAndQueues", func(t *testing.T) { testProposalQueues(t, newStore) })
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

func sampleChannel(id string, number int, deadline time.Time) Channel {
	ch := Channel{}
	ch.ID = id
	ch.IntentRef = "intent-" + id
	ch.Name = "Channel " + id
	ch.Number = number
	ch.Group = "Loomarr"
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Shuffle.Seed = 7
	ch.UpdatedAt = 1_700_000_000
	ch.Lineup = []schedule.LineupEntry{
		{Key: "movie:tmdb:1", Title: "A", DurationMs: 3600000},
		{Key: "movie:tmdb:2", Title: "B"},
	}
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "lib-1", Title: "A", DurationMs: 3600000},
		{Kind: schedule.SlotFiller, Key: "movie:tmdb:2", Title: "B"},
	}
	ch.ReconcileDeadline = deadline
	return ch
}

func testChannelRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	want := sampleChannel("ch-a", 5, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertChannel(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetChannel(ctx, "ch-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Number != want.Number || got.Strategy != want.Strategy || got.Status != want.Status {
		t.Errorf("channel scalar round-trip mismatch: got %+v", got.Channel)
	}
	if len(got.Lineup) != 2 || got.Lineup[0].Key != "movie:tmdb:1" || got.Lineup[0].DurationMs != 3600000 {
		t.Errorf("lineup JSON round-trip lost data: %+v", got.Lineup)
	}
	if len(got.Desired) != 2 || got.Desired[0].Kind != schedule.SlotProgram || got.Desired[1].Kind != schedule.SlotFiller {
		t.Errorf("desired JSON round-trip lost data: %+v", got.Desired)
	}
	if !got.ReconcileDeadline.Equal(want.ReconcileDeadline) {
		t.Errorf("reconcile deadline round-trip: got %v want %v", got.ReconcileDeadline, want.ReconcileDeadline)
	}
	// Upsert is idempotent: a second write with an edited field updates in place.
	want.Status = schedule.StatusDrifted
	if err := s.UpsertChannel(ctx, want); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetChannel(ctx, "ch-a")
	if got2.Status != schedule.StatusDrifted {
		t.Errorf("upsert didn't update status: %s", got2.Status)
	}
	all, _ := s.ListChannels(ctx)
	if len(all) != 1 {
		t.Errorf("upsert created a duplicate channel: %d rows", len(all))
	}
	// GetChannelByNumber resolves the same row.
	byNum, err := s.GetChannelByNumber(ctx, 5)
	if err != nil || byNum.ID != "ch-a" {
		t.Errorf("GetChannelByNumber(5) = %q,%v want ch-a", byNum.ID, err)
	}
	// Missing lookups are ErrNotFound.
	if _, err := s.GetChannel(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetChannel(missing) = %v, want ErrNotFound", err)
	}
	if _, err := s.GetChannelByNumber(ctx, 999); err != ErrNotFound {
		t.Errorf("GetChannelByNumber(missing) = %v, want ErrNotFound", err)
	}
}

func testChannelListDelete(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertChannel(ctx, sampleChannel("ch-2", 2, time.Time{}))
	_ = s.UpsertChannel(ctx, sampleChannel("ch-1", 1, time.Time{}))
	_ = s.UpsertChannel(ctx, sampleChannel("ch-3", 3, time.Time{}))

	all, err := s.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ListChannels = %d, want 3", len(all))
	}
	// Ordered by number.
	if all[0].Number != 1 || all[1].Number != 2 || all[2].Number != 3 {
		t.Errorf("ListChannels not ordered by number: %d,%d,%d", all[0].Number, all[1].Number, all[2].Number)
	}
	if err := s.DeleteChannel(ctx, "ch-2"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.ListChannels(ctx)
	if len(after) != 2 {
		t.Errorf("after delete: %d channels, want 2", len(after))
	}
	if _, err := s.GetChannel(ctx, "ch-2"); err != ErrNotFound {
		t.Errorf("deleted channel still present: %v", err)
	}
}

func testClaimDueChannels(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	// Due: deadline in the past, live.
	_ = s.UpsertChannel(ctx, sampleChannel("ch-due", 1, now.Add(-time.Hour)))
	// Not due: future deadline.
	_ = s.UpsertChannel(ctx, sampleChannel("ch-future", 2, now.Add(time.Hour)))
	// Not eligible: detached, even with a past deadline.
	detached := sampleChannel("ch-detached", 3, now.Add(-time.Hour))
	detached.Status = schedule.StatusDetached
	_ = s.UpsertChannel(ctx, detached)

	claimed, err := s.ClaimDueChannels(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != "ch-due" {
		t.Fatalf("ClaimDueChannels = %d rows, want just ch-due: %+v", len(claimed), claimed)
	}
	// Leased: a second claim at the same now returns nothing.
	again, _ := s.ClaimDueChannels(ctx, now, time.Minute, 10)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased channels, want 0", len(again))
	}
	got, _ := s.GetChannel(ctx, "ch-due")
	if !got.ReconcileDeadline.Equal(now.Add(time.Minute)) {
		t.Errorf("claimed channel deadline = %v, want leased %v", got.ReconcileDeadline, now.Add(time.Minute))
	}
}

// testClaimChannelsConcurrent is the §18 guarantee: two replicas never reconcile
// the same channel. On Postgres it exercises FOR UPDATE SKIP LOCKED.
func testClaimChannelsConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		_ = s.UpsertChannel(ctx, sampleChannel("chan-"+string(rune('a'+i)), i+1, now.Add(-time.Hour)))
	}

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueChannels(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, c := range batch {
					seen[c.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Errorf("claimed %d distinct channels, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("channel %s claimed %d times, want exactly 1", id, c)
		}
	}
}

func sampleJob(id, hash string, deadline, createdAt time.Time) Job {
	return Job{
		ID: id, Kind: "suggest", Status: "queued",
		IntentJSON: `{"description":"90s action"}`, IntentHash: hash,
		CreatedBy: "user-1", Deadline: deadline, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func testJobRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	want := sampleJob("job-1", "hash-abc", now, now)
	if err := s.CreateJob(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetJob(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" || got.IntentHash != "hash-abc" || got.CreatedBy != "user-1" {
		t.Errorf("job round-trip mismatch: %+v", got)
	}
	// Update transitions status.
	got.Status = "done"
	got.UpdatedAt = now
	if err := s.UpdateJob(ctx, got); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetJob(ctx, "job-1")
	if after.Status != "done" {
		t.Errorf("update didn't persist status: %s", after.Status)
	}
	if _, err := s.GetJob(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetJob(missing) = %v, want ErrNotFound", err)
	}
}

func testClaimDueJobs(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.CreateJob(ctx, sampleJob("due", "h1", now.Add(-time.Hour), now))
	future := sampleJob("future", "h2", now.Add(time.Hour), now)
	_ = s.CreateJob(ctx, future)
	running := sampleJob("running", "h3", now.Add(-time.Hour), now)
	running.Status = "running"
	_ = s.CreateJob(ctx, running)

	claimed, err := s.ClaimDueJobs(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != "due" {
		t.Fatalf("ClaimDueJobs = %d, want just 'due': %+v", len(claimed), claimed)
	}
	// Leased: second claim returns nothing.
	again, _ := s.ClaimDueJobs(ctx, now, time.Minute, 10)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased jobs, want 0", len(again))
	}
}

func testClaimJobsConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		_ = s.CreateJob(ctx, sampleJob("job-"+string(rune('a'+i)), "h", now.Add(-time.Hour), now))
	}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueJobs(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, j := range batch {
					seen[j.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Errorf("claimed %d distinct jobs, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("job %s claimed %d times, want 1", id, c)
		}
	}
}

func testJobCacheByHash(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.CreateJob(ctx, sampleJob("cached", "hash-X", now, now))

	// A search within TTL finds it.
	got, err := s.FindJobByIntentHash(ctx, "hash-X", now.Add(-24*time.Hour))
	if err != nil || got.ID != "cached" {
		t.Fatalf("FindJobByIntentHash = %q,%v want cached", got.ID, err)
	}
	// A search with `since` after the job's creation misses (TTL expired).
	if _, err := s.FindJobByIntentHash(ctx, "hash-X", now.Add(time.Hour)); err != ErrNotFound {
		t.Errorf("expired cache lookup = %v, want ErrNotFound", err)
	}
	// A different hash misses.
	if _, err := s.FindJobByIntentHash(ctx, "hash-other", now.Add(-24*time.Hour)); err != ErrNotFound {
		t.Errorf("miss lookup = %v, want ErrNotFound", err)
	}
}

func testProposalQueues(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	mk := func(id, status, creator string) Proposal {
		return Proposal{ID: id, JobID: "job-1", Status: status, CreatedBy: creator,
			ProposalJSON: `{"lineup":[]}`, CreatedAt: now, UpdatedAt: now}
	}
	_ = s.CreateProposal(ctx, mk("p1", "submitted", "alice"))
	_ = s.CreateProposal(ctx, mk("p2", "submitted", "bob"))
	_ = s.CreateProposal(ctx, mk("p3", "approved", "alice"))

	// The approval queue = submitted proposals.
	sub, err := s.ListProposalsByStatus(ctx, "submitted")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 2 {
		t.Errorf("submitted queue = %d, want 2", len(sub))
	}
	// My proposals = by creator.
	aliceProps, _ := s.ListProposalsByCreator(ctx, "alice")
	if len(aliceProps) != 2 {
		t.Errorf("alice's proposals = %d, want 2", len(aliceProps))
	}
	// Approve p1: status + approved_by persist (survives restart — it's in the store).
	p1, _ := s.GetProposal(ctx, "p1")
	p1.Status = "approved"
	p1.ApprovedBy = "admin"
	p1.UpdatedAt = now
	if err := s.UpdateProposal(ctx, p1); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetProposal(ctx, "p1")
	if after.Status != "approved" || after.ApprovedBy != "admin" {
		t.Errorf("approve didn't persist: %+v", after)
	}
	if _, err := s.GetProposal(ctx, "missing"); err != ErrNotFound {
		t.Errorf("GetProposal(missing) = %v, want ErrNotFound", err)
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
