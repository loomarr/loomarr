package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Operational tables: the settings KV, session lifecycle, the /metrics count queries,
// the activity feed, and the retention purge that sweeps the first three.

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

	// --- audited overrides (config-design §3): ListSettings + UpsertSetting +
	// DeleteSetting round-trip with audit metadata, one suite both backends. ---
	when := time.Unix(1_700_000_000, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:8096", UpdatedAt: when, UpdatedBy: "matt"}); err != nil {
		t.Fatal(err)
	}
	// A system write (SetSetting) leaves updated_by NULL — surfaced as "".
	if err := s.SetSetting(ctx, "llm.model", "qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; r.Value != "http://emby:8096" || r.UpdatedBy != "matt" || !r.UpdatedAt.Equal(when) {
		t.Errorf("audited override: %+v want value/matt/%v", r, when)
	}
	if r := got["llm.model"]; r.Value != "qwen3:8b" || r.UpdatedBy != "" {
		t.Errorf("system write should have empty updated_by: %+v", r)
	}
	// Upsert overwrites value + audit.
	later := time.Unix(1_700_000_100, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:9096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" && (r.Value != "http://emby:9096" || r.UpdatedBy != "ana") {
			t.Errorf("upsert did not overwrite audit: %+v", r)
		}
	}

	// A settings PATCH is one durable commit, including a mix of upserts and clears.
	// The same suite pins this contract for SQLite and Postgres.
	batchAt := time.Unix(1_700_000_200, 0).UTC()
	if err := s.ApplySettingBatch(ctx, SettingBatch{
		Upserts: []SettingMutation{
			{Key: "library.flavor", Value: "emby"},
			{Key: "library.url", Value: "http://batch:8096"},
			{Key: "library.token", Value: "batch-token"},
		},
		Deletes:   []string{"llm.model"},
		UpdatedAt: batchAt,
		UpdatedBy: "grace",
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	for _, key := range []string{"library.flavor", "library.url", "library.token"} {
		if r := got[key]; r.UpdatedBy != "grace" || !r.UpdatedAt.Equal(batchAt) {
			t.Errorf("batch audit for %s: %+v want grace/%v", key, r, batchAt)
		}
	}
	if got["library.url"].Value != "http://batch:8096" {
		t.Errorf("batch url = %q, want http://batch:8096", got["library.url"].Value)
	}
	if _, ok := got["llm.model"]; ok {
		t.Error("batch delete left llm.model present")
	}

	// Ambiguous batches fail before opening a transaction or changing any row.
	if err := s.ApplySettingBatch(ctx, SettingBatch{
		Upserts: []SettingMutation{{Key: "library.url", Value: "http://must-not-save:8096"}},
		Deletes: []string{"library.url"},
	}); err == nil {
		t.Fatal("duplicate setting key in batch succeeded")
	}
	if v, err := s.GetSetting(ctx, "library.url"); err != nil || v != "http://batch:8096" {
		t.Errorf("rejected batch mutated url: %q %v", v, err)
	}
	// --- the env-override claim (config-design §3.1), one suite both backends ---
	//
	// ⚠ The bug this guards: env_override must NOT be in UpsertSetting's DO UPDATE list.
	// If it were, an ordinary save would silently re-lock a key the operator had just
	// unlocked — at the exact moment they are certain to be editing it.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://seed:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	got = map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; !r.EnvOverride {
		t.Errorf("claim not persisted: %+v", r)
	}
	// A plain value save must leave the claim standing.
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://edited:8096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if !r.EnvOverride {
				t.Error("an ordinary save cleared the env-override claim — the key silently re-locked")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("value did not save alongside the claim: %+v", r)
			}
		}
	}
	// Seeding only applies when the row is absent; an existing value is never clobbered.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://should-not-apply:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, "library.url"); v != "http://edited:8096" {
		t.Errorf("re-claiming overwrote the stored value with the seed: %q", v)
	}
	// Handing the key back keeps the value, so the round trip loses nothing.
	if err := s.SetSettingEnvOverride(ctx, "library.url", false, "", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if r.EnvOverride {
				t.Error("claim not cleared on re-lock")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("re-lock discarded the stored value: %+v", r)
			}
		}
	}
	// Claiming a key with NO existing row creates one carrying the seed — the ordinary
	// unlock case, since an env-pinned key usually has nothing stored.
	if err := s.SetSettingEnvOverride(ctx, "seerr.url", true, "http://seerr:5055", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.GetSetting(ctx, "seerr.url"); err != nil || v != "http://seerr:5055" {
		t.Errorf("unlock did not seed a new row: %q %v", v, err)
	}

	// Delete reverts the key (config-design §9).
	if err := s.DeleteSetting(ctx, "library.url"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSetting(ctx, "library.url"); err != ErrNotFound {
		t.Errorf("after delete, GetSetting = %v want ErrNotFound", err)
	}
}

// testSessionLifecycle covers the §11 session rules that authentication depends on:
// sessions are revocable rows, expiry is immediate for reads even though purging is
// eventual, and disabling a user kills every session at once. None of this had store
// conformance coverage before — it is the one area where a dialect difference would be
// a security bug rather than a correctness bug, so it belongs in the shared suite.
func testSessionLifecycle(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, User{ID: "u2", Name: "Grace", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}

	live := Session{TokenHash: "h-live", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	older := Session{TokenHash: "h-older", UserID: "u1", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	dead := Session{TokenHash: "h-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)}
	other := Session{TokenHash: "h-other", UserID: "u2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	for _, sess := range []Session{live, older, dead, other} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListSessionsForUser(ctx, "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	// Expired excluded, another user's session excluded, newest first.
	if len(got) != 2 {
		t.Fatalf("ListSessionsForUser = %d sessions, want 2 (expired and other-user excluded)", len(got))
	}
	if got[0].TokenHash != "h-live" || got[1].TokenHash != "h-older" {
		t.Errorf("order = [%s %s], want newest first [h-live h-older]", got[0].TokenHash, got[1].TokenHash)
	}
	if !got[0].CreatedAt.Equal(now) || !got[0].ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("timestamps did not round-trip: created=%v expires=%v", got[0].CreatedAt, got[0].ExpiresAt)
	}

	// Revoking one leaves the rest — the admin "sign out this device" path.
	if err := s.RevokeSession(ctx, "h-live"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 1 {
		t.Errorf("after RevokeSession: %d sessions, want 1", len(got))
	}

	// Disabling a user kills their sessions immediately (§11) — and only theirs.
	if err := s.RevokeSessionsForUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 0 {
		t.Errorf("after RevokeSessionsForUser: %d sessions, want 0", len(got))
	}
	if got, _ = s.ListSessionsForUser(ctx, "u2", now); len(got) != 1 {
		t.Errorf("RevokeSessionsForUser hit another user's sessions: u2 has %d, want 1", len(got))
	}
}

// testCounts covers the §17 observability gauges: grouped counts must reflect
// the rows present, and (for sessions) honor the same expiry predicate the read
// path uses. Same assertions on both backends (one suite, two dialects).
func testCounts(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	// Titles across three states: two requested, one available.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:20", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:21", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:22", provision.Available, time.Time{}))

	titles, err := s.CountTitlesByState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if titles[provision.Requested] != 2 || titles[provision.Available] != 1 {
		t.Errorf("CountTitlesByState = %v, want requested:2 available:1", titles)
	}
	// A state with no rows is absent, not zero — the collector zero-fills it.
	if _, ok := titles[provision.Downloading]; ok {
		t.Errorf("CountTitlesByState included an empty state: %v", titles)
	}

	// Jobs: two queued (sampleJob defaults to queued), one flipped to running.
	_ = s.CreateJob(ctx, sampleJob("job-1", "h1", now, now))
	_ = s.CreateJob(ctx, sampleJob("job-2", "h2", now, now))
	running := sampleJob("job-3", "h3", now, now)
	running.Status = "running"
	_ = s.CreateJob(ctx, running)

	jobs, err := s.CountJobsByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jobs["queued"] != 2 || jobs["running"] != 1 {
		t.Errorf("CountJobsByStatus = %v, want queued:2 running:1", jobs)
	}

	// Sessions: two live, one expired — only the live ones count as of now.
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	for _, sess := range []Session{
		{TokenHash: "c-live1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-live2", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
	} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.CountActiveSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if active != 2 {
		t.Errorf("CountActiveSessions = %d, want 2 (expired excluded)", active)
	}
}

// testActivityFeed covers the Dashboard feed's three operations on both backends (§5, V32):
// append, newest-first read, and age-based purge.
func testActivityFeed(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	for i, tc := range []struct {
		text  string
		level string
		age   time.Duration
	}{
		{"oldest — CH 12 reconciled", ActivityInfo, 90 * time.Minute},
		{"middle — Seerr timed out, retrying", ActivityWarn, 60 * time.Minute},
		{"newest — Darkwing Duck landed", ActivityInfo, 30 * time.Minute},
	} {
		if err := st.RecordActivity(ctx, Activity{
			At: base.Add(-tc.age).Unix(), Kind: ActivityKindTitle, Level: tc.level,
			Text: tc.text, SubjectID: fmt.Sprintf("subj-%d", i),
		}); err != nil {
			t.Fatalf("record activity %d: %v", i, err)
		}
	}

	// Newest first — the ONLY ordering the feed has.
	got, err := st.ListActivity(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("listed %d rows, want 3", len(got))
	}
	if !strings.HasPrefix(got[0].Text, "newest") || !strings.HasPrefix(got[2].Text, "oldest") {
		t.Errorf("wrong order: %q ... %q", got[0].Text, got[2].Text)
	}
	if got[1].Level != ActivityWarn {
		t.Errorf("level = %q, want warn — it drives the UI dot", got[1].Level)
	}
	if got[0].ID == "" {
		t.Error("row has no id; the store must mint one")
	}

	// The limit is honoured, or a dashboard panel would render the whole table.
	if two, err := st.ListActivity(ctx, 2); err != nil || len(two) != 2 {
		t.Errorf("ListActivity(2) = %d rows, %v; want 2", len(two), err)
	}

	// An unrecognised level is normalised on the way in — the UI has no colour for it, so
	// storing it would render an invisible dot.
	if err := st.RecordActivity(ctx, Activity{Text: "odd", Level: "banana"}); err != nil {
		t.Fatalf("record odd level: %v", err)
	}
	after, _ := st.ListActivity(ctx, 1)
	if len(after) != 1 || after[0].Level != ActivityInfo {
		t.Errorf("level %q was stored as-is; want normalisation to info", after[0].Level)
	}

	// An empty text is dropped rather than stored: a blank row occupies a slot in a list
	// the operator is scanning.
	before, _ := st.ListActivity(ctx, 100)
	if err := st.RecordActivity(ctx, Activity{Text: ""}); err != nil {
		t.Fatalf("record empty: %v", err)
	}
	if all, _ := st.ListActivity(ctx, 100); len(all) != len(before) {
		t.Errorf("an empty-text row was stored (%d -> %d)", len(before), len(all))
	}

	// Purge is by AGE — the feed is the one append-only table here, so it is the one that
	// would otherwise grow without bound.
	n, err := st.PurgeActivity(ctx, base.Add(-45*time.Minute))
	if err != nil {
		t.Fatalf("purge activity: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d rows, want 2 (the two older than the horizon)", n)
	}
	left, _ := st.ListActivity(ctx, 100)
	for _, a := range left {
		if strings.HasPrefix(a.Text, "oldest") || strings.HasPrefix(a.Text, "middle") {
			t.Errorf("row %q survived a purge that should have removed it", a.Text)
		}
	}
}

// testRetentionPurge covers §5's retention on both backends — and, more importantly, what it
// must NOT remove.
func testRetentionPurge(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)

	// Jobs: one of each status, all OLD, so status is the only thing distinguishing them.
	for _, j := range []Job{
		{ID: "j-done", Kind: "suggest", Status: "done", UpdatedAt: old},
		{ID: "j-failed", Kind: "suggest", Status: "failed", UpdatedAt: old},
		{ID: "j-queued", Kind: "suggest", Status: "queued", UpdatedAt: old},
		{ID: "j-running", Kind: "suggest", Status: "running", UpdatedAt: old},
		{ID: "j-fresh", Kind: "suggest", Status: "done", UpdatedAt: recent},
	} {
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatalf("seed job %s: %v", j.ID, err)
		}
	}

	n, err := st.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("purge jobs: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d jobs, want 2 (done + failed, both old)", n)
	}
	// ⚠ The invariants: in-flight work survives regardless of age, and a recent finished job
	// is inside the window.
	for _, id := range []string{"j-queued", "j-running", "j-fresh"} {
		if _, err := st.GetJob(ctx, id); err != nil {
			t.Errorf("job %s was purged; only finished jobs past the window may go (%v)", id, err)
		}
	}
	for _, id := range []string{"j-done", "j-failed"} {
		if _, err := st.GetJob(ctx, id); err == nil {
			t.Errorf("job %s survived a purge that should have removed it", id)
		}
	}

	// Proposals: one of each status, all OLD.
	for _, p := range []Proposal{
		{ID: "p-denied", Status: "denied", UpdatedAt: old},
		{ID: "p-approved", Status: "approved", UpdatedAt: old},
		{ID: "p-submitted", Status: "submitted", UpdatedAt: old},
		{ID: "p-fresh-denied", Status: "denied", UpdatedAt: recent},
	} {
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatalf("seed proposal %s: %v", p.ID, err)
		}
	}

	n, err = st.PurgeDeniedProposals(ctx, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("purge proposals: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d proposals, want 1 (the old denied one)", n)
	}
	// ⚠ THE AUDIT TRAIL. An approved proposal is the record that someone authorized spending
	// real resources (§7); a submitted one is a member still waiting for an answer. Neither
	// may be aged out.
	for _, id := range []string{"p-approved", "p-submitted", "p-fresh-denied"} {
		if _, err := st.GetProposal(ctx, id); err != nil {
			t.Errorf("proposal %s was purged; only OLD DENIED proposals may go (%v)", id, err)
		}
	}
	if _, err := st.GetProposal(ctx, "p-denied"); err == nil {
		t.Error("the old denied proposal survived a purge that should have removed it")
	}
}
