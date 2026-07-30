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
		"filler-sync | 0 */15 * * * * | job.filler_sync.schedule",
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
