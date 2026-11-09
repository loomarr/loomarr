package holidayvocab

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// RefreshEvery is how often a replica rereads the stored holidays (#2030 decision 33): another
// replica's edit reaches this one's engine within it. This replica's own edits reload at once.
const RefreshEvery = 30 * time.Second

// Snapshot is the stored holidays as one validated Vocabulary, the one Source every holiday
// consumer reads. A load that fails, or whose rows do not validate, keeps the last good snapshot
// (the shipped definitions before the first load), so a bad row degrades nothing that already airs.
type Snapshot struct {
	store   Lister
	log     *slog.Logger
	current atomic.Pointer[Vocabulary]
}

// Lister reads the stored holidays.
type Lister interface {
	ListHolidays(context.Context) ([]Holiday, error)
}

// NewSnapshot returns a snapshot holding the shipped definitions until the first Reload.
func NewSnapshot(store Lister, log *slog.Logger) *Snapshot {
	h := &Snapshot{store: store, log: log}
	shipped := Default()
	h.current.Store(&shipped)
	return h
}

// Current is the Source: the last good snapshot.
func (h *Snapshot) Current() Vocabulary { return *h.current.Load() }

// Reload reads the stored holidays into the snapshot.
func (h *Snapshot) Reload(ctx context.Context) {
	rows, err := h.store.ListHolidays(ctx)
	if err != nil {
		h.log.Warn("holidays: could not read the stored holidays; keeping the last ones loaded", "err", err)
		return
	}
	v, err := New(rows)
	if err != nil {
		h.log.Error("holidays: the stored holidays are invalid; keeping the last ones loaded", "err", err)
		return
	}
	h.current.Store(&v)
}

// Run rereads the stored holidays every holidayRefresh until ctx ends.
func (h *Snapshot) Run(ctx context.Context) {
	ticker := time.NewTicker(RefreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.Reload(ctx)
		}
	}
}
