package recurate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/recurate"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/testkit"
)

// fakeRefiner records which channels were asked to re-curate + with what intent.
type fakeRefiner struct {
	calls   []string
	intents map[string]proposals.Intent
	err     error
}

func (f *fakeRefiner) Recurate(_ context.Context, channelID string, intent proposals.Intent) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.calls = append(f.calls, channelID)
	if f.intents == nil {
		f.intents = map[string]proposals.Intent{}
	}
	f.intents[channelID] = intent
	return "job-for-" + channelID, nil
}

// seedJob writes the suggestion job a channel is made from, carrying the original request.
func seedJob(t *testing.T, st store.Store, id, description string) {
	t.Helper()
	blob, _ := json.Marshal(proposals.Intent{Description: description, Era: "1990s"})
	if err := st.CreateJob(context.Background(), store.Job{ID: id, Kind: "suggest", IntentJSON: string(blob), IntentHash: "h-" + id, Status: "done"}); err != nil {
		t.Fatal(err)
	}
}

// ownIntent is the request a channel made from jobID owns, as approval copies it onto the
// channel. A channel with no job (hand-made) owns none.
func ownIntent(t *testing.T, st store.Store, jobID string) string {
	t.Helper()
	job, err := st.GetJob(context.Background(), jobID)
	if err != nil {
		return ""
	}
	return job.IntentJSON
}

// seedChannelStatus writes a channel with the given status + auto-curate opt-in.
func seedChannelStatus(t *testing.T, st store.Store, id, jobID string, status schedule.ChannelStatus, ac *schedule.AutoCurate, number int) {
	t.Helper()
	ch := store.Channel{}
	ch.ID = id
	ch.IntentRef = jobID
	ch.IntentJSON = ownIntent(t, st, jobID)
	ch.Name = "Ch " + id
	ch.Number = number
	ch.Strategy = schedule.Sequential
	ch.Status = status
	ch.Policy = schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: ac}}
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// The runner triggers a refine ONLY for eligible channels: live/building + intent-backed +
// auto-curate. Paused, detached, hand-made (no IntentRef), and non-opted-in channels are skipped.
func TestRunner_TriggersOnlyEligibleChannels(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	seedJob(t, st, "job-live", "90s action")
	seedJob(t, st, "job-building", "90s scifi")
	seedJob(t, st, "job-paused", "90s comedy")
	seedJob(t, st, "job-notopted", "90s drama")

	seedJob(t, st, "job-drifted", "90s horror")

	seedChannelStatus(t, st, "c-live", "job-live", schedule.StatusLive, &schedule.AutoCurate{}, 1)
	seedChannelStatus(t, st, "c-building", "job-building", schedule.StatusBuilding, &schedule.AutoCurate{}, 2)
	seedChannelStatus(t, st, "c-drifted", "job-drifted", schedule.StatusDrifted, &schedule.AutoCurate{}, 6) // ELIGIBLE: drifted is a transient managed state
	seedChannelStatus(t, st, "c-paused", "job-paused", schedule.StatusPaused, &schedule.AutoCurate{}, 3)    // skipped: paused
	seedChannelStatus(t, st, "c-notopted", "job-notopted", schedule.StatusLive, nil, 4)                     // skipped: no opt-in
	seedChannelStatus(t, st, "c-handmade", "", schedule.StatusLive, &schedule.AutoCurate{}, 5)              // skipped: no IntentRef

	fr := &fakeRefiner{}
	r := recurate.NewRunner(st, fr, testkit.Logger())
	kicked, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if kicked != 3 {
		t.Fatalf("kicked %d, want 3 (live + building + drifted auto-curate channels)", kicked)
	}
	got := map[string]bool{}
	for _, c := range fr.calls {
		got[c] = true
	}
	if !got["c-live"] || !got["c-building"] || !got["c-drifted"] {
		t.Errorf("refined %v, want c-live + c-building + c-drifted", fr.calls)
	}
	if got["c-paused"] || got["c-notopted"] || got["c-handmade"] {
		t.Errorf("refined an ineligible channel: %v", fr.calls)
	}
}

// Housekeeping purges finished jobs, so a channel past `jobs.retention` is re-curated from the
// intent it owns: the job it was made from is gone and the weekly run must still happen. A
// channel whose request was already gone before channels owned it is skipped with a reason, not
// guessed at.
func TestRunner_RecuratesFromTheChannelsOwnIntentAfterItsJobIsPurged(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	old := time.Now().Add(-31 * 24 * time.Hour)
	blob, _ := json.Marshal(proposals.Intent{Description: "90s action", Era: "1990s"})
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-old", Kind: "suggest", Status: "done", IntentJSON: string(blob), UpdatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	seedChannelStatus(t, st, "c-owned", "job-old", schedule.StatusLive, &schedule.AutoCurate{}, 1)
	seedChannelStatus(t, st, "c-orphan", "job-already-gone", schedule.StatusLive, &schedule.AutoCurate{}, 2)

	if purged, err := st.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour)); err != nil || purged != 1 {
		t.Fatalf("purge = %d, %v; want the 31-day-old job purged", purged, err)
	}

	var logs bytes.Buffer
	fr := &fakeRefiner{}
	kicked, err := recurate.NewRunner(st, fr, slog.New(slog.NewTextHandler(&logs, nil))).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kicked != 1 || len(fr.calls) != 1 || fr.calls[0] != "c-owned" {
		t.Fatalf("re-curated %v (kicked %d), want only c-owned", fr.calls, kicked)
	}
	if got := fr.intents["c-owned"]; got.Description != "90s action" || got.Era != "1990s" {
		t.Errorf("re-curation lost the channel's intent: %+v", got)
	}
	if !strings.Contains(logs.String(), "original request is unavailable") || !strings.Contains(logs.String(), "c-orphan") {
		t.Errorf("skipping c-orphan left no clear reason in the log:\n%s", logs.String())
	}
}

// The refresh refine carries the channel's ORIGINAL intent (the one it owns) with NO
// operator RefineText — a scheduled "re-evaluate against the library", not a human change.
func TestRunner_RefreshIntentKeepsOriginalNoRefineText(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	seedJob(t, st, "job1", "90s action heroes")
	seedChannelStatus(t, st, "c1", "job1", schedule.StatusLive, &schedule.AutoCurate{}, 1)

	fr := &fakeRefiner{}
	r := recurate.NewRunner(st, fr, testkit.Logger())
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	intent := fr.intents["c1"]
	if intent.Description != "90s action heroes" || intent.Era != "1990s" {
		t.Errorf("refresh intent lost the original constraints: %+v", intent)
	}
	if intent.RefineText != "" {
		t.Errorf("refresh intent must carry NO RefineText, got %q", intent.RefineText)
	}
}

// The scheduler submits only durable editorial intent. Channel feedback scope is
// restored later from the claimed Proposal Job id, so this transient handoff must
// never pretend to be execution authority.
func TestRunner_RefreshIntentDoesNotClaimFeedbackScope(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	seedJob(t, st, "job1", "90s action heroes")
	seedChannelStatus(t, st, "channel-a", "job1", schedule.StatusLive, &schedule.AutoCurate{}, 1)

	fr := &fakeRefiner{}
	if _, err := recurate.NewRunner(st, fr, testkit.Logger()).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if scope := fr.intents["channel-a"].DiscoveryScopeID; scope != "" {
		t.Fatalf("scheduler supplied transient feedback scope %q; execution scope must be derived after durable claim", scope)
	}
}

// No eligible channels → the runner is a cheap no-op (0 kicked, no error).
func TestRunner_NoEligibleChannelsIsNoop(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	seedJob(t, st, "job1", "x")
	seedChannelStatus(t, st, "c1", "job1", schedule.StatusLive, nil, 1) // not opted in
	fr := &fakeRefiner{}
	r := recurate.NewRunner(st, fr, testkit.Logger())
	kicked, err := r.Run(context.Background())
	if err != nil || kicked != 0 {
		t.Fatalf("kicked=%d err=%v, want 0,nil", kicked, err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("no channel should have been refined, got %v", fr.calls)
	}
}

// A channel with an unfinished channel job (a refine being generated, or a re-curation proposal
// nobody has decided yet) is skipped with an Info line, so unapproved re-curations never pile up
// week after week and a run never replaces a refine still in flight.
func TestRunner_SkipsAChannelWithAnUnfinishedChannelJob(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	seedJob(t, st, "job-busy", "90s action")
	seedJob(t, st, "job-idle", "90s comedy")
	seedChannelStatus(t, st, "c-busy", "job-busy", schedule.StatusLive, &schedule.AutoCurate{}, 1)
	seedChannelStatus(t, st, "c-idle", "job-idle", schedule.StatusLive, &schedule.AutoCurate{}, 2)
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-refine", Kind: "suggest", Status: "queued", ChannelID: "c-busy",
	}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	fr := &fakeRefiner{}
	kicked, err := recurate.NewRunner(st, fr, slog.New(slog.NewTextHandler(&logs, nil))).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kicked != 1 || len(fr.calls) != 1 || fr.calls[0] != "c-idle" {
		t.Fatalf("re-curated %v (kicked %d), want only c-idle", fr.calls, kicked)
	}
	if !strings.Contains(logs.String(), "level=INFO") || !strings.Contains(logs.String(), "unfinished") || !strings.Contains(logs.String(), "c-busy") {
		t.Errorf("skipping c-busy left no Info line naming it:\n%s", logs.String())
	}
}
