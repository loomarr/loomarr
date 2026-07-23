package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/requester"
	"github.com/mantonx/loomarr/internal/store"
)

// QueueStatuser is the narrow capability the queue poller needs (§18.1 arr-queue-poll): report
// download status for a set of in-flight titles. Only the direct arr requester implements it
// (Seerr has no queue), so the poll job is registered only when requester.provider=arr.
type QueueStatuser interface {
	QueueStatus(ctx context.Context, titles []provision.Title) ([]requester.QueueItem, error)
}

// QueuePoll drives the download-progress path for the direct arr requester (§4, §18.1). It reads
// each arr's queue, promotes a title with a live download record to `downloading` (Grabbed), and
// persists its progress on the title record. It does NOT flip anything to `available` — that
// stays the library scan's job (a file in the download client isn't yet in the media library).
type QueuePoll struct {
	store store.Store
	queue QueueStatuser
	emit  Emitter
	now   func() time.Time
	log   *slog.Logger
	dlTTL time.Duration // deadline granted on Grabbed (the shorter downloading TTL)
}

// NewQueuePoll builds the poller. now defaults to time.Now; downloadingTTL defaults to 12h (the
// same shorter deadline the reconciler uses once a title is genuinely downloading).
func NewQueuePoll(st store.Store, queue QueueStatuser, emit Emitter, downloadingTTL time.Duration, now func() time.Time, log *slog.Logger) *QueuePoll {
	if now == nil {
		now = time.Now
	}
	if downloadingTTL <= 0 {
		downloadingTTL = 12 * time.Hour
	}
	return &QueuePoll{store: st, queue: queue, emit: emit, now: now, dlTTL: downloadingTTL, log: log}
}

// Poll runs one queue pass: gather in-flight titles (requested + downloading), ask the arr(s)
// for their download status, and for each grabbed title apply Grabbed (→downloading, resetting
// the deadline) if it isn't already downloading, then persist its progress. Returns the number
// of titles observed as actively downloading.
func (p *QueuePoll) Poll(ctx context.Context) (int, error) {
	inflight, err := p.inflight(ctx)
	if err != nil {
		return 0, err
	}
	if len(inflight) == 0 {
		return 0, nil
	}

	titles := make([]provision.Title, 0, len(inflight))
	for _, r := range inflight {
		titles = append(titles, r.Title)
	}
	items, err := p.queue.QueueStatus(ctx, titles)
	if err != nil {
		return 0, err
	}

	now := p.now()
	active := 0
	for _, it := range items {
		if !it.Grabbed {
			continue // not in the download client's queue — nothing to promote or record
		}
		active++
		rec, ok := inflight[it.Key]
		if !ok {
			continue // moved on between list and poll
		}
		// Promote requested→downloading exactly once (Grabbed is a no-op on a downloading
		// record, but we skip the write when already downloading to avoid needless churn).
		if rec.State == provision.Requested {
			next, emitted := provision.Apply(rec, provision.Event{Kind: provision.Grabbed, Deadline: now.Add(p.dlTTL)}, now)
			p.persist(ctx, next, emitted)
		}
		// Persist progress via the targeted write (never clobbers state-machine columns).
		if err := p.store.UpdateTitleProgress(ctx, it.Key, it.Progress, it.ETAText, it.Status); err != nil {
			p.log.Error("queue-poll: progress", "key", it.Key, "err", err)
		}
	}
	return active, nil
}

// inflight loads requested + downloading titles indexed by key — the only states a download can
// advance or report progress for.
func (p *QueuePoll) inflight(ctx context.Context) (map[provision.Key]provision.Record, error) {
	out := make(map[provision.Key]provision.Record)
	for _, st := range []provision.State{provision.Requested, provision.Downloading} {
		recs, err := p.store.ListTitlesByState(ctx, st)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			out[r.Key] = r
		}
	}
	return out, nil
}

// persist mirrors the reconciler/scan persist — write the record, fan terminal events. (Grabbed
// is not terminal, so it emits nothing; the write is what matters, plus any future event.)
func (p *QueuePoll) persist(ctx context.Context, rec provision.Record, emitted []provision.DomainEvent) {
	if err := p.store.UpsertTitle(ctx, rec); err != nil {
		p.log.Error("queue-poll: persist", "key", rec.Key, "err", err)
		return
	}
	for _, ev := range emitted {
		if p.emit != nil {
			p.emit.Emit(ctx, ev)
		}
	}
}
