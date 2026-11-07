package recurate

import (
	"context"
	"log/slog"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// Refiner submits a re-curation job for a channel against a refreshed intent (the §7 refine
// mechanism). Satisfied by *suggest.Service. The runner declares the interface so recurate
// doesn't depend on the concrete worker beyond the one method it calls.
type Refiner interface {
	Recurate(ctx context.Context, channelID string, intent proposals.Intent) (string, error)
}

// Adjacencer walks the recommendation graph from a channel's own lineup (§8.3). Optional:
// nil ⇒ re-curation runs the LLM corpus alone, exactly as it did before adjacency existed.
// Narrowed to the one method, like every other dependency here.
type Adjacencer interface {
	Adjacent(ctx context.Context, seeds []catalog.Candidate, exclude map[provision.Key]bool, limit int) ([]catalog.Adjacency, error)
}

// RunnerStore is the slice of the store the runner needs.
type RunnerStore interface {
	ListChannels(ctx context.Context) ([]store.Channel, error)
	ChannelHasUnfinishedJob(ctx context.Context, channelID string) (bool, error)
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
	adj     Adjacencer // optional (§8.3); nil ⇒ LLM corpus only
	log     *slog.Logger
}

// WithAdjacency wires the §8.3 adjacency corpus. Returns the runner for chaining, keeping
// NewRunner's signature stable — the same pattern suggest.WithRatings uses.
func (r *Runner) WithAdjacency(a Adjacencer) *Runner {
	r.adj = a
	return r
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
		busy, berr := r.store.ChannelHasUnfinishedJob(ctx, ch.ID)
		if berr != nil {
			r.log.Warn("re-curation: could not check the channel's unfinished jobs; skipping",
				"channel", ch.ID, "err", berr)
			continue
		}
		if busy {
			r.log.Info("re-curation: the channel has an unfinished refine or re-curation; skipping",
				"channel", ch.ID)
			continue
		}
		intent, ok := r.refreshIntent(ctx, ch)
		if !ok {
			continue // no readable original request → can't re-curate; skip (logged in refreshIntent)
		}
		if _, rerr := r.refiner.Recurate(ctx, ch.ID, intent); rerr != nil {
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
// intent-backed (made from a suggestion, not by hand), opted into auto-curate, and actively managed.
// "Actively managed" is a DENY-LIST of the two hands-off states — paused (the operator
// deliberately took it off the sweep) and detached (soft-deleted) — so every managed state
// qualifies: live, building, AND drifted. drifted is a TRANSIENT reconcile state ("the guide is
// catching up to the desired lineup"), not a reason to stop curating; excluding it would freeze
// re-curation on any channel whose Tunarr push is momentarily behind.
func eligible(ch store.Channel) bool {
	if ch.IntentRef == "" || ch.Policy.AutoCurate == nil {
		return false
	}
	return ch.Status.Reconcilable()
}

// refreshIntent builds the intent for a re-curation refine: the channel's OWN approved intent
// (description + era/tone/constraints) plus the current lineup as context, and NO RefineText —
// the "change" is simply "re-evaluate against the library as it is now". Returns ok=false when
// the channel has no readable original request (nothing to re-run).
func (r *Runner) refreshIntent(ctx context.Context, ch store.Channel) (proposals.Intent, bool) {
	intent, err := suggest.IntentOfChannel(ch)
	if err != nil {
		r.log.Warn("re-curation: the channel's original request is unavailable; skipping channel",
			"channel", ch.ID, "err", err)
		return proposals.Intent{}, false
	}
	// No operator RefineText — a scheduled refresh, not a human-requested change. Seed the
	// current lineup so the suggester reasons from what's already on the channel (and can
	// prefer keeping it). CurrentLineup drives the refine framing in the prompt (§7).
	intent.CurrentLineup = lineupContext(ch.Lineup)
	intent.Adjacent = r.adjacentFor(ctx, ch)
	return intent, true
}

// adjacentLimit bounds how many adjacency offers ride into one refine.
//
// They are prompt context, and an unbounded list would both crowd the intent and hand the
// model more to weigh than it can use. The re-curation title cap (§8.2) bounds what can
// actually be ADDED regardless; this bounds what is offered.
const adjacentLimit = 12

// adjacentFor walks the channel's own lineup through the recommendation graph and returns
// the consensus picks as prompt context (§8.3).
//
// BEST-EFFORT and non-fatal in every direction: no corpus wired, a walk that errors, or a
// channel whose lineup yields no TMDB seeds all return nil, and re-curation proceeds with
// the LLM corpus alone. Adjacency widens a run; it must never fail one.
func (r *Runner) adjacentFor(ctx context.Context, ch store.Channel) []proposals.AdjacentContext {
	if r.adj == nil || len(ch.Lineup) == 0 {
		return nil
	}
	seeds := make([]catalog.Candidate, 0, len(ch.Lineup))
	exclude := make(map[provision.Key]bool, len(ch.Lineup))
	for _, e := range ch.Lineup {
		exclude[e.Key] = true
		mt, provider, id, ok := provision.ParseKey(e.Key)
		if !ok || provider != "tmdb" {
			continue // only TMDB ids have a graph to walk
		}
		seeds = append(seeds, catalog.Candidate{
			MediaType: mt, TMDBID: id, Name: e.Title, Year: e.Year,
		})
	}
	if len(seeds) == 0 {
		return nil
	}
	found, err := r.adj.Adjacent(ctx, seeds, exclude, adjacentLimit)
	if err != nil {
		r.log.Warn("re-curation: adjacency walk failed; continuing with the LLM corpus alone",
			"channel", ch.ID, "err", err)
		return nil
	}
	out := make([]proposals.AdjacentContext, 0, len(found))
	for _, a := range found {
		k, kerr := a.Candidate.Key()
		if kerr != nil {
			continue
		}
		out = append(out, proposals.AdjacentContext{
			Name: a.Candidate.Name, Year: a.Candidate.Year, Key: string(k), Votes: a.Votes,
		})
	}
	return out
}

// lineupContext turns a channel's stored lineup into the lightweight name/year/key context the
// refiner reasons about — the same shape the API's channel-refine handler builds.
func lineupContext(entries []schedule.LineupEntry) []proposals.LineupContext {
	out := make([]proposals.LineupContext, 0, len(entries))
	for _, e := range entries {
		out = append(out, proposals.LineupContext{Name: e.Title, Year: e.Year, Key: string(e.Key)})
	}
	return out
}
