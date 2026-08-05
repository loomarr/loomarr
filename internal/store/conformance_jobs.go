package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// Jobs and proposals (§8): the suggester job lifecycle, the scheduled-job table behind
// the cron runner, and the proposal queues the approval gate reads.
//
// ⚠ The three scheduled-job tests sit TOGETHER here. `ScheduledJobPaused` used to live ~900
// lines from its two siblings, in a file ordered by when each test was written.

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

// testScheduledJobRoundTrip: upsert creates then updates a job's state row; list + get read
// it back; a missing row is ErrNotFound.
func testScheduledJobRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	if _, err := s.GetScheduledJob(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing scheduled job = %v, want ErrNotFound", err)
	}
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{Name: "reconcile", NextRun: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Update in place (same name) — last_result + next_run change.
	next := now.Add(5 * time.Minute)
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{
		Name: "reconcile", LastRun: now, LastResult: "ok", NextRun: next, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastResult != "ok" || !got.NextRun.Equal(next) || !got.LastRun.Equal(now) {
		t.Errorf("round-tripped scheduled job = %+v, want ok/next=%v/last=%v", got, next, now)
	}
	all, _ := s.ListScheduledJobs(ctx)
	if len(all) != 1 || all[0].Name != "reconcile" {
		t.Errorf("list = %+v, want one 'reconcile'", all)
	}
}

// testClaimDueScheduledJobs: only due rows (next_run <= now) are claimed, and claiming leases
// next_run forward so a second claim returns nothing until rescheduled.
func testClaimDueScheduledJobs(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.UpsertScheduledJob(ctx, ScheduledJob{Name: "due", NextRun: now.Add(-time.Minute), UpdatedAt: now})
	_ = s.UpsertScheduledJob(ctx, ScheduledJob{Name: "future", NextRun: now.Add(time.Hour), UpdatedAt: now})

	claimed, err := s.ClaimDueScheduledJobs(ctx, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Name != "due" {
		t.Fatalf("ClaimDueScheduledJobs = %d, want just 'due': %+v", len(claimed), claimed)
	}
	// Leased forward → an immediate re-claim returns nothing.
	again, _ := s.ClaimDueScheduledJobs(ctx, now, time.Minute)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased jobs, want 0", len(again))
	}
}

// testScheduledJobPaused: the pause flag persists, survives an ordinary state write, and keeps
// the job out of the due-claim (§18.1). One suite, both dialects — the claim SQL differs
// (guarded UPDATE vs FOR UPDATE SKIP LOCKED) and both must skip paused rows.
func testScheduledJobPaused(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	// Due NOW, not paused: the control case — without it, "did not run" proves nothing.
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{Name: "reconcile", NextRun: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetScheduledJobPaused(ctx, "reconcile", true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Fatal("pause did not persist")
	}

	// ⚠ An ordinary state write must NOT clear it. This runs after every execution, so if
	// `paused` rode in UpsertScheduledJob's DO UPDATE list, the next run would silently resume
	// a job the operator paused.
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{
		Name: "reconcile", LastResult: "ok", LastRun: now, NextRun: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetScheduledJob(ctx, "reconcile"); !got.Paused {
		t.Error("a routine state write cleared paused — it must be absent from ON CONFLICT DO UPDATE")
	}

	// ⚠ The behaviour: a paused row is never claimed, even though it is due.
	due, err := s.ClaimDueScheduledJobs(ctx, now.Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range due {
		if j.Name == "reconcile" {
			t.Error("a paused job was claimed; it would then run on its schedule")
		}
	}

	// Resuming makes it claimable again, or pause is a one-way door.
	if err := s.SetScheduledJobPaused(ctx, "reconcile", false); err != nil {
		t.Fatal(err)
	}
	due, err = s.ClaimDueScheduledJobs(ctx, now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range due {
		if j.Name == "reconcile" {
			found = true
		}
	}
	if !found {
		t.Error("a resumed job was still not claimed")
	}

	// Pausing a job that has never run creates the row, so a task can be paused before its
	// first execution rather than only after it has already gone off once.
	if err := s.SetScheduledJobPaused(ctx, "never-ran", true); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetScheduledJob(ctx, "never-ran"); err != nil || !got.Paused {
		t.Errorf("pausing an unseen job = (%+v, %v), want a created paused row", got, err)
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

// testLookupByNonID pins the two "find the row by something other than its id" queries (V41).
//
// ⚠ Both replaced a full-table read plus a linear walk in Go, so the properties that matter are
// the ones a scan gave for free and a WHERE clause has to be told: not-found is ErrNotFound
// (not a zero struct), the filter actually discriminates, and — for proposals — NEWEST wins.
func testLookupByNonID(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	// --- channels by intent_ref ---
	mk := func(id, intentRef string, number int) Channel {
		ch := Channel{}
		ch.ID = id
		ch.IntentRef = intentRef
		ch.Name = "Channel " + id
		ch.Number = number
		ch.Strategy = schedule.Sequential
		ch.Status = schedule.StatusLive
		return ch
	}
	for _, ch := range []Channel{mk("c1", "job-a", 1), mk("c2", "job-b", 2), mk("c3", "", 3)} {
		if err := s.UpsertChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GetChannelByIntentRef(ctx, "job-b")
	if err != nil {
		t.Fatalf("by intent ref: %v", err)
	}
	if got.ID != "c2" {
		t.Errorf("intent job-b resolved to %q, want c2 — the filter is not discriminating", got.ID)
	}
	// ⚠ ErrNotFound, NOT a zero Channel. The former scan returned an empty struct for "no
	// match", whose blank ID reads as a valid channel to a caller that forgets to check.
	if _, err := s.GetChannelByIntentRef(ctx, "job-nope"); err != ErrNotFound {
		t.Errorf("unknown intent = %v, want ErrNotFound", err)
	}
	// A channel with NO intent ref must not answer an empty-string lookup as if it matched
	// something meaningful — "" is the default for every hand-made channel.
	if ch, err := s.GetChannelByIntentRef(ctx, ""); err == nil && ch.ID != "c3" {
		t.Errorf(`empty intent ref resolved to %q, want the un-bound channel or ErrNotFound`, ch.ID)
	}

	// --- proposals: newest approved for a job ---
	older := Proposal{
		ID: "p-old", JobID: "job-a", Status: "approved", ProposalJSON: `{"rationale":"first"}`,
		CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now,
	}
	newer := Proposal{
		ID: "p-new", JobID: "job-a", Status: "approved", ProposalJSON: `{"rationale":"refined"}`,
		CreatedAt: now, UpdatedAt: now,
	}
	// Same job, still SUBMITTED — must never be returned for an "approved" ask, however new.
	pending := Proposal{
		ID: "p-pending", JobID: "job-a", Status: "submitted", ProposalJSON: `{}`,
		CreatedAt: now.Add(time.Hour), UpdatedAt: now,
	}
	// A different job's approved proposal — the row a job_id-blind query would wrongly return.
	other := Proposal{
		ID: "p-other", JobID: "job-b", Status: "approved", ProposalJSON: `{}`,
		CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now,
	}
	// Inserted oldest-last so a query relying on insertion order rather than created_at fails.
	for _, p := range []Proposal{other, pending, newer, older} {
		if err := s.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ NEWEST wins, and it is load-bearing: a refine re-runs the channel's own job, so the
	// channel must bind to the LATEST approved lineup rather than the original one.
	p, err := s.NewestProposalByStatusForJob(ctx, "job-a", "approved")
	if err != nil {
		t.Fatalf("newest approved: %v", err)
	}
	if p.ID != "p-new" {
		t.Errorf("newest approved for job-a = %q, want p-new — a refine would bind the ORIGINAL lineup", p.ID)
	}
	// The status filter holds even though the submitted one is newer still.
	if p.Status != "approved" {
		t.Errorf("returned a %q proposal for an approved-only ask — the §8 gate leaks", p.Status)
	}
	// The job filter holds even though another job has a newer approved proposal.
	if p.JobID != "job-a" {
		t.Errorf("returned job %q, want job-a", p.JobID)
	}
	if _, err := s.NewestProposalByStatusForJob(ctx, "job-none", "approved"); err != ErrNotFound {
		t.Errorf("unknown job = %v, want ErrNotFound", err)
	}
	// A job with only a submitted proposal has no approved one — the binder refuses to build.
	if _, err := s.NewestProposalByStatusForJob(ctx, "job-b", "denied"); err != ErrNotFound {
		t.Errorf("status with no rows = %v, want ErrNotFound", err)
	}
}
