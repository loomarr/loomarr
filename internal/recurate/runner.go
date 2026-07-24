package recurate

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// Refiner triggers a re-run of a channel's own suggestion job against a refreshed intent
// (the §7 refine mechanism). Satisfied by *suggest.Service. The runner declares the interface
// so recurate doesn't depend on the concrete worker beyond the one method it calls.
type Refiner interface {
	Refine(ctx context.Context, jobID string, intent suggest.Intent) (string, error)
}

// RunnerStore is the slice of the store the runner needs.
type RunnerStore interface {
	ListChannels(ctx context.Context) ([]store.Channel, error)
	GetJob(ctx context.Context, id string) (store.Job, error)
}

// Runner is the `channel-recurate` scheduler job (§8.2): on each tick it finds the channels
// opted into auto-curate and triggers a refresh refine on each. The worker then produces the
// proposal that the Curator (wired as the worker's ChannelAutoCurator) considers, so the
// runner itself never approves or writes titles — it only kicks the existing pipeline. Cheap
// when nothing is eligible; the LLM cost is paid per eligible channel inside the worker, and
// the intent-hash cache short-circuits a channel whose intent+lineup are unchanged.
type Runner struct {
	store   RunnerStore
	refiner Refiner
	log     *slog.Logger
}

// NewRunner wires the job.
func NewRunner(st RunnerStore, r Refiner, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{store: st, refiner: r, log: log}
}

// Run triggers a refresh refine for every eligible channel. Returns the number of channels
// re-curation was kicked for (for the job's log/metrics). A single channel's failure is logged
// and skipped — one bad channel must not stop the sweep (the §18.1 job-resilience contract).
func (r *Runner) Run(ctx context.Context) (kicked int, err error) {
	channels, err := r.store.ListChannels(ctx)
	if err != nil {
		return 0, err
	}
	for _, ch := range channels {
		if !eligible(ch) {
			continue
		}
		intent, ok := r.refreshIntent(ctx, ch)
		if !ok {
			continue // no readable source job → can't refine; skip (logged in refreshIntent)
		}
		if _, rerr := r.refiner.Refine(ctx, ch.IntentRef, intent); rerr != nil {
			r.log.Warn("re-curation refine failed for a channel; skipping",
				"channel", ch.ID, "err", rerr)
			continue
		}
		kicked++
	}
	if kicked > 0 {
		r.log.Info("re-curation kicked", "channels", kicked)
	}
	return kicked, nil
}

// eligible reports whether a channel participates in scheduled re-curation (§8.2): it must be
// intent-backed (has a suggestion job to re-run), opted into auto-curate, and actively managed
// (live/building — not paused, not detached, not hand-made).
func eligible(ch store.Channel) bool {
	if ch.IntentRef == "" || ch.Policy.AutoCurate == nil {
		return false
	}
	return ch.Status == schedule.StatusLive || ch.Status == schedule.StatusBuilding
}

// refreshIntent builds the intent for a re-curation refine: the channel's ORIGINAL intent
// (description + era/tone/constraints, from its source job) plus the current lineup as context,
// and NO RefineText — the "change" is simply "re-evaluate against the library as it is now".
// Returns ok=false when the source job can't be read (nothing to re-run).
func (r *Runner) refreshIntent(ctx context.Context, ch store.Channel) (suggest.Intent, bool) {
	job, err := r.store.GetJob(ctx, ch.IntentRef)
	if err != nil {
		r.log.Warn("re-curation: source job unreadable; skipping channel",
			"channel", ch.ID, "job", ch.IntentRef, "err", err)
		return suggest.Intent{}, false
	}
	var intent suggest.Intent
	if uerr := json.Unmarshal([]byte(job.IntentJSON), &intent); uerr != nil {
		r.log.Warn("re-curation: source intent malformed; skipping channel",
			"channel", ch.ID, "err", uerr)
		return suggest.Intent{}, false
	}
	// No operator RefineText — a scheduled refresh, not a human-requested change. Seed the
	// current lineup so the suggester reasons from what's already on the channel (and can
	// prefer keeping it). CurrentLineup drives the refine framing in the prompt (§7).
	intent.RefineText = ""
	intent.CurrentLineup = lineupContext(ch.Lineup)
	return intent, true
}

// lineupContext turns a channel's stored lineup into the lightweight name/year/key context the
// refiner reasons about — the same shape the API's channel-refine handler builds.
func lineupContext(entries []schedule.LineupEntry) []suggest.LineupContext {
	out := make([]suggest.LineupContext, 0, len(entries))
	for _, e := range entries {
		out = append(out, suggest.LineupContext{Name: e.Title, Year: e.Year, Key: string(e.Key)})
	}
	return out
}
