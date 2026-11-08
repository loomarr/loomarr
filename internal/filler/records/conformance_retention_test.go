package records

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// The filler tables' retention (#2528) deletes each spent row and nothing a live read needs. Every
// table is seeded with a row that must go ("gone-") and rows that must stay ("kept-"), on both sides
// of each rule: clip present or not, inside or outside the window, in flight or settled.
func testFillerRetention(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	db := store.HandleOf(s)
	now := time.Date(2026, 11, 8, 4, 30, 0, 0, time.UTC)
	long := now.AddDate(-1, 0, 0).Unix() // older than every window
	recent := now.Add(-time.Hour).Unix()

	if err := s.UpsertClip(ctx, sampleClip("live", "live.mp4", filler.Commercial, 1990, filler.General, "")); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(sql), args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	for _, m := range []string{"2025-10", "2026-10"} { // older than 12 months, this year
		exec(`INSERT INTO filler_research_web_usage (month) VALUES (?)`, m)
	}
	for _, p := range []struct {
		hash string
		at   int64
	}{{"live", long}, {"gone", long}, {"gone", recent}} {
		exec(`INSERT INTO filler_enrichment_passes (clip_hash, producer, producer_version, completed_at)
			VALUES (?, 'transcript', ?, ?)`, p.hash, fmt.Sprint(p.at), p.at)
	}
	// One ready event per clip: the live clip's stays, an orphan's goes, a recent orphan's stays.
	for _, e := range []struct {
		hash string
		at   int64
	}{{"live", long}, {"gone", long}, {"gone-recently", recent}} {
		exec(`INSERT INTO filler_ready_events (id, clip_hash, enrollment_kind, enrollment_ref, placement, created_at)
			VALUES (?, ?, 'folder', 'x', 'any', ?)`, "ready:"+e.hash, e.hash, e.at)
	}
	// Research: the live clip's revision is 0 (sampleClip). An old cited report at revision 0 stays,
	// an older cited report at revision -1 goes (a newer cited one exists), an old guess at -1 stays
	// (no newer guess), and an orphan's report goes.
	report := func(hash, producer, version string, revision, at int64) {
		exec(`INSERT INTO filler_context_reports (clip_hash, producer, producer_version, adapter, adapter_version,
			input_revision, report_json, completed_at) VALUES (?, ?, ?, 'a', 'v', ?, '{}', ?)`, hash, producer, version, revision, at)
	}
	report("live", "context-model:m", "2", 0, recent)
	report("live", "context-model:m", "1", -1, long)
	report("live", research.ProducerObservation, "vision", -1, long)
	report("gone", "context-model:m", "1", 0, long)

	exec(`INSERT INTO filler_split_proposals (id, clip_hash, segments_json, created_at) VALUES ('sp-live', 'reel', '[]', ?)`, long)
	for _, d := range []struct{ proposal, id string }{{"sp-live", "kept-shadow"}, {"sp-gone", "gone-shadow"}} {
		exec(`INSERT INTO filler_split_shadow_decisions (id, proposal_id, clip_hash, policy_version, decision_json, observed_at)
			VALUES (?, ?, 'reel', 'v1', '{}', ?)`, d.id, d.proposal, long)
	}

	// Model calls: a settled window call and its evaluation go; a reserved evaluation, an open call
	// and its evaluation, and a recent settled one stay.
	evaluation := func(id, state string, at int64) {
		exec(`INSERT INTO filler_inference_evaluations (id, cache_key, clip_hash, role, state, requested_provider,
			requested_model, evidence_version, extractor_version, prompt_version, schema_version, taxonomy_version,
			admission_policy_version, role_policy_version, capability_snapshot, created_at, updated_at)
			VALUES (?, 'k', 'src', 'complete_timeline_structure', ?, 'p', 'm', 'v', 'v', 'v', 'v', 'v', 'v', 'v', 's', ?, ?)`,
			id, state, at, at)
	}
	call := func(request, evaluationID, state string, at int64) {
		record, assessed := "", int64(0)
		if state == "settled" {
			record, assessed = "{}", at
		}
		exec(`INSERT INTO filler_structure_window_call_ledger (request_sha256, evaluation_id, source_sha256,
			media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256, record_json,
			requested_at, assessed_at) VALUES (?, ?, 's', 'm', 0, 'a', ?, '{}', ?, ?, ?, ?)`,
			request, evaluationID, state, record, storedb.OptionalJSON(record), at, assessed)
	}
	evaluation("gone-eval", "completed", long)
	call("gone-call", "gone-eval", "settled", long)
	evaluation("kept-open-eval", "reserved", long)
	call("kept-open-call", "kept-open-eval", "open", long)
	evaluation("kept-reserved", "reserved", long)
	evaluation("kept-recent", "completed", recent)
	evaluation("gone-judge", "failed", long)

	// Downloads: run-gone ends with nothing pointing at it (its artifact's clip is gone and it has no
	// provider id); run-kept keeps an artifact with a provider id, so its approved pull stays too.
	run := func(id, pull string) {
		exec(`INSERT INTO filler_acquisition_runs (id, pull_id, trigger, status, started_at, completed_at, updated_at)
			VALUES (?, ?, 'pull', 'success', ?, ?, ?)`, id, pull, long, long, long)
	}
	artifact := func(id, runID, remote, hash string) {
		exec(`INSERT INTO filler_acquisition_artifacts (id, acquisition_id, provider, source_url, media_path, media_sha256,
			media_bytes, state, clip_hash, remote_id, completed_at, updated_at)
			VALUES (?, ?, 'archive', 'u', 'p', 'h', 1, 'consumed', ?, ?, ?, ?)`, id, runID, hash, remote, long, long)
	}
	pull := func(id, status string) {
		exec(`INSERT INTO filler_pulls (id, title, status, created_at, decided_at) VALUES (?, 't', ?, ?, ?)`, id, status, long, long)
	}
	pull("pull-gone", "approved")
	pull("pull-kept", "approved")
	pull("pull-dismissed", "dismissed")
	run("run-gone", "pull-gone")
	run("run-kept", "pull-kept")
	exec(`INSERT INTO filler_pull_commits (pull_id, acquisition_id) VALUES ('pull-gone', 'run-gone'), ('pull-kept', 'run-kept')`)
	artifact("art-gone", "run-gone", "", "gone")
	artifact("art-kept", "run-kept", "archive:item", "gone")

	// What a clip leaves behind: the live clip's rows stay, an orphan's old ones go, an orphan's
	// recent facts and airings stay, and a variant pair goes only once both its clips are gone.
	seedClipLeftovers(t, exec, long, recent)

	n, err := s.PurgeSpentFillerRecords(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	// 1 month + 1 pass + 1 ready event + 2 reports + 1 shadow + 1 call + 2 evaluations + 1 artifact
	// + 1 commit + 2 pulls + 1 run + 1 fact + 1 tag + 1 airing + 1 variant pair.
	if n != 18 {
		t.Errorf("purged %d rows, want 18", n)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM filler_research_web_usage WHERE month = '2026-10'`:                           1,
		`SELECT count(*) FROM filler_research_web_usage`:                                                   1,
		`SELECT count(*) FROM filler_enrichment_passes WHERE clip_hash = 'live'`:                           1,
		`SELECT count(*) FROM filler_enrichment_passes WHERE clip_hash = 'gone'`:                           1, // the recent one
		`SELECT count(*) FROM filler_ready_events`:                                                         2,
		`SELECT count(*) FROM filler_context_reports WHERE clip_hash = 'live'`:                             2,
		`SELECT count(*) FROM filler_context_reports WHERE producer = 'observation'`:                       1,
		`SELECT count(*) FROM filler_split_shadow_decisions WHERE id = 'kept-shadow'`:                      1,
		`SELECT count(*) FROM filler_split_shadow_decisions`:                                               1,
		`SELECT count(*) FROM filler_structure_window_call_ledger WHERE request_sha256 = 'kept-open-call'`: 1,
		`SELECT count(*) FROM filler_structure_window_call_ledger`:                                         1,
		`SELECT count(*) FROM filler_inference_evaluations WHERE id LIKE 'kept-%'`:                         3,
		`SELECT count(*) FROM filler_inference_evaluations`:                                                3,
		`SELECT count(*) FROM filler_acquisition_artifacts WHERE id = 'art-kept'`:                          1,
		`SELECT count(*) FROM filler_acquisition_artifacts`:                                                1,
		`SELECT count(*) FROM filler_acquisition_runs WHERE id = 'run-kept'`:                               1,
		`SELECT count(*) FROM filler_acquisition_runs`:                                                     1,
		`SELECT count(*) FROM filler_pull_commits WHERE pull_id = 'pull-kept'`:                             1,
		`SELECT count(*) FROM filler_pulls WHERE id = 'pull-kept'`:                                         1,
		`SELECT count(*) FROM filler_pulls`:                                                                1,
		`SELECT count(*) FROM filler_enrichment_axes WHERE clip_hash = 'live'`:                             1,
		`SELECT count(*) FROM filler_enrichment_axes WHERE clip_hash = 'gone'`:                             1, // the recent one
		`SELECT count(*) FROM clip_tags`:                                                                   1,
		`SELECT count(*) FROM clip_tags WHERE clip_hash = 'live'`:                                          1,
		`SELECT count(*) FROM filler_airings WHERE clip_hash = 'live'`:                                     1,
		`SELECT count(*) FROM filler_airings WHERE clip_hash = 'gone'`:                                     1, // the recent one
		`SELECT count(*) FROM filler_clip_variants WHERE b_hash = 'live'`:                                  1,
		`SELECT count(*) FROM filler_clip_variants`:                                                        1,
	} {
		var got int
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}

// An empty catalog is what an unmounted drop folder looks like: the sync has pruned every clip and
// they come back under the same hashes. Nothing is deleted for its clip being gone until then.
func testFillerRetentionEmptyCatalog(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	db := store.HandleOf(s)
	now := time.Date(2026, 11, 8, 4, 30, 0, 0, time.UTC)
	long := now.AddDate(-1, 0, 0).Unix()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(sql), args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO filler_enrichment_passes (clip_hash, producer, producer_version, completed_at)
		VALUES ('gone', 'transcript', '1', ?)`, long)
	seedClipLeftovers(t, exec, long, long-60)

	n, err := s.PurgeSpentFillerRecords(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purged %d rows from an empty catalog, want 0", n)
	}
}

// seedClipLeftovers writes the per-clip rows the sync leaves behind, for the clips "live", "gone"
// and "gone2": an old and a recent fact and airing for "gone", one of each for "live", a tag on
// "live" and "gone", and the variant pairs gone–gone2 and gone–live.
func seedClipLeftovers(t *testing.T, exec func(string, ...any), long, recent int64) {
	t.Helper()
	for _, f := range []struct {
		hash, axis string
		at         int64
	}{{"live", "era", long}, {"gone", "era", long}, {"gone", "brand", recent}} {
		exec(`INSERT INTO filler_enrichment_axes (clip_hash, axis, state, evidence_kind, evidence_rank,
			evidence_reference, confidence, producer, producer_version, observed_at, updated_at)
			VALUES (?, ?, 'complete', 'k', 1, 'r', 1, 'p', 'v', ?, ?)`, f.hash, f.axis, f.at, f.at)
	}
	exec(`INSERT INTO clip_tags (clip_hash, taxon) VALUES ('live', 'beer'), ('gone', 'beer')`)
	for _, a := range []struct {
		hash string
		at   int64
	}{{"live", long}, {"gone", long}, {"gone", recent}} {
		exec(`INSERT INTO filler_airings (channel_id, clip_hash, aired_at_ms, recorded_at_ms) VALUES ('ch', ?, ?, ?)`,
			a.hash, a.at*1000, a.at*1000)
	}
	exec(`INSERT INTO filler_clip_variants (a_hash, b_hash) VALUES ('gone', 'gone2'), ('gone', 'live')`)
}
