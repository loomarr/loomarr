package recurate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
)

// fakeRefiner records which jobs were asked to refine + with what intent.
type fakeRefiner struct {
	calls   []string
	intents map[string]suggest.Intent
	err     error
}

func (f *fakeRefiner) Refine(_ context.Context, jobID string, intent suggest.Intent) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.calls = append(f.calls, jobID)
	if f.intents == nil {
		f.intents = map[string]suggest.Intent{}
	}
	f.intents[jobID] = intent
	return jobID, nil
}

// seedJob writes a suggestion job carrying the original intent (the runner re-uses it).
func seedJob(t *testing.T, st store.Store, id, description string) {
	t.Helper()
	blob, _ := json.Marshal(suggest.Intent{Description: description, Era: "1990s"})
	if err := st.CreateJob(context.Background(), store.Job{ID: id, IntentJSON: string(blob), IntentHash: "h-" + id, Status: "done"}); err != nil {
		t.Fatal(err)
	}
}

// seedChannelStatus writes a channel with the given status + auto-curate opt-in.
func seedChannelStatus(t *testing.T, st store.Store, id, jobID string, status schedule.ChannelStatus, ac *schedule.AutoCurate, number int) {
	t.Helper()
	ch := store.Channel{}
	ch.ID = id
	ch.IntentRef = jobID
	ch.Name = "Ch " + id
	ch.Number = number
	ch.Strategy = schedule.Sequential
	ch.Status = status
	ch.Policy = schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: ac}}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// The runner triggers a refine ONLY for eligible channels: live/building + intent-backed +
// auto-curate. Paused, detached, hand-made (no IntentRef), and non-opted-in channels are skipped.
func TestRunner_TriggersOnlyEligibleChannels(t *testing.T) {
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
	for _, j := range fr.calls {
		got[j] = true
	}
	if !got["job-live"] || !got["job-building"] || !got["job-drifted"] {
		t.Errorf("refined %v, want job-live + job-building + job-drifted", fr.calls)
	}
	if got["job-paused"] || got["job-notopted"] {
		t.Errorf("refined an ineligible channel: %v", fr.calls)
	}
}

// The refresh refine carries the channel's ORIGINAL intent (from its source job) with NO
// operator RefineText — a scheduled "re-evaluate against the library", not a human change.
func TestRunner_RefreshIntentKeepsOriginalNoRefineText(t *testing.T) {
	st := newStore(t)
	seedJob(t, st, "job1", "90s action heroes")
	seedChannelStatus(t, st, "c1", "job1", schedule.StatusLive, &schedule.AutoCurate{}, 1)

	fr := &fakeRefiner{}
	r := recurate.NewRunner(st, fr, testkit.Logger())
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	intent := fr.intents["job1"]
	if intent.Description != "90s action heroes" || intent.Era != "1990s" {
		t.Errorf("refresh intent lost the original constraints: %+v", intent)
	}
	if intent.RefineText != "" {
		t.Errorf("refresh intent must carry NO RefineText, got %q", intent.RefineText)
	}
}

// No eligible channels → the runner is a cheap no-op (0 kicked, no error).
func TestRunner_NoEligibleChannelsIsNoop(t *testing.T) {
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
