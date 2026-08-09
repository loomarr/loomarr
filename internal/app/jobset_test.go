package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/app"
	"github.com/mantonx/loomarr/internal/store"
)

// ⚠ **THE JOB SET IS THE CONTRACT (§18.1).** Every job's name, cron default and settings key
// now live in the package that owns the work rather than in the composition root — which is
// better, and also means a rename or a dropped registration is a one-line change in a file
// nobody else is reading.
//
// This pins the whole set through the API an operator actually sees. It was written by
// capturing the output BEFORE the jobs moved and diffing it after: identical, which is the
// only evidence that a 14-declaration move changed nothing.
//
// A job legitimately added or renamed updates this list — deliberately, in the same commit,
// which is the point. A job that vanishes because its `Jobs()` stopped being called fails
// here instead of silently never running.
func TestJobSet(t *testing.T) {
	want := []string{
		"activity-purge | 0 15 4 * * * | job.activity_purge.schedule",
		"backup | 0 30 3 * * * | backup.schedule",
		"channel-recurate | 0 0 4 * * 0 | job.recurate.schedule",
		"channel-sweep | 0 */10 * * * * | job.channel_sweep.schedule",
		// ⚠ Auto-fetch (§10 V38b) is the first job that reaches OUT to the internet unattended.
		// Its presence here is the deliberate record of that: §15 previously said "there is no
		// unattended crawler", and this row is what a future reader sees when they check whether
		// that is still true.
		"filler-fetch | 0 0 */6 * * * | job.filler_fetch.schedule",
		// ⚠ **The ingest pipeline (§10 V51b) — this ONE row replaced FOUR**: `filler-language`
		// (:30), `filler-split` (:45), `filler-transcribe` (:15) and `filler-vision` (:50). Their
		// staggered minutes were a hand-maintained scheduling discipline that kept four expensive
		// sweeps off each other's runner, and the note that used to sit here spelled it out. The
		// pipeline runs ONE clip at a time through all the rungs in order, so there is nothing
		// left to stagger.
		//
		// ⚠ It also inherits the record the language row carried: this is the job that DELETES
		// catalog rows unattended — now for several reasons rather than one (a wrong language, a
		// file with no audio, a clip nothing could identify), each recorded with a reason and each
		// reversible for the soft cases. A future reader asking "does anything remove clips on its
		// own?" is looking at this row.
		//
		// Every two minutes, far tighter than the hourly sweeps: a pass is bounded by the budget
		// rather than the catalog, and this is the only thing that advances a new download.
		"filler-pipeline | 0 */2 * * * * | job.filler_pipeline.schedule",
		// ⚠ The taxonomy reindex (§10 V45a). A lifecycle sibling of the media jobs (its own row,
		// registered unconditionally, default-off) but NOT an expensive one — its body is two bulk SQL
		// statements (rebuild the closure, then every clip's rollups), no whisper/vision, no per-clip
		// loop. It stays a cron job rather than a rung deliberately: it is bulk SQL over the whole
		// catalog after a GRAPH edit, not per-clip work, and folding it in would make a taxonomy
		// edit wait behind a whisper backlog.
		"filler-reindex | 0 5 * * * * | job.filler_reindex.schedule",
		"filler-sync | 0 */15 * * * * | job.filler_sync.schedule",
		// ⚠ The image service's four (§22, V52). `images-fetch` is the SECOND job to reach out to
		// the internet unattended — the record `filler-fetch` above carries — and it differs in a
		// way worth writing down: filler-fetch pulls from sources an operator ADDED, while this
		// one fetches whatever URL a row happens to hold, which is why it is the only job in this
		// list behind a host allowlist and an SSRF guard (imagejobs.go).
		//
		// ⚠ `images-gc` is the second job that DELETES unattended. It removes image rows nothing
		// references and evicts derivative FILES, and one of its duties is not tidying at all: the
		// six-month TMDB ceiling is a licence term, so this row is the only thing keeping the
		// install compliant. A future reader asking "what enforces the TTL?" is looking at it.
		// ⚠ `images-adopt-artwork` runs every FIVE MINUTES, the tightest cadence in this list after
		// images-fetch, and that is deliberate: it is the step between a clip's thumbnail being
		// rendered and that thumbnail being visible through the image service. An hourly cadence
		// would mean an operator importing clips watches the legacy artwork for an hour and reads
		// it as the feature not working. Its work list is empty on a healthy install.
		"images-adopt-artwork | 0 */5 * * * * | job.images_adopt_artwork.schedule",
		"images-avif | 0 20 * * * * | job.images_avif.schedule",
		"images-fetch | 0 * * * * * | job.images_fetch.schedule",
		"images-gc | 0 0 5 * * * | job.images_gc.schedule",
		"images-rehydrate | 0 45 4 * * * | job.images_rehydrate.schedule",
		"library-full-scan | 0 0 3 * * * | job.library_full_scan.schedule",
		"library-scan | 0 */5 * * * * | job.library_scan.schedule",
		"reconcile | 0 */5 * * * * | job.reconcile.schedule",
		"retention-purge | 0 30 4 * * * | job.retention_purge.schedule",
		"seerr-queue-poll | 0 * * * * * | job.seerr_queue_poll.schedule",
		"series-episode-refresh | 0 0 * * * * | job.series_episode_refresh.schedule",
		"session-sweep | 0 0 * * * * | job.session_sweep.schedule",
	}

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/jobs.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	t.Setenv("API_TOKEN", "jobset-token")
	h, err := app.BuildHandler(context.Background(), st, slog.New(slog.DiscardHandler), app.Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer jobset-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/jobs → %d, want 200", resp.StatusCode)
	}

	var body struct {
		Jobs []struct {
			Name        string `json:"name"`
			Schedule    string `json:"schedule"`
			ScheduleKey string `json:"scheduleKey"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, j := range body.Jobs {
		got = append(got, j.Name+" | "+j.Schedule+" | "+j.ScheduleKey)
	}
	sort.Strings(got)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("job set changed.\n--- got (%d) ---\n%s\n--- want (%d) ---\n%s",
			len(got), strings.Join(got, "\n"), len(want), strings.Join(want, "\n"))
	}
}
