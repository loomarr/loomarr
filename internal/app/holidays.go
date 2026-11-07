package app

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// holidayRefresh is how often a replica rereads the stored holidays (#2030 decision 33): another
// replica's edit reaches this one's engine within it. This replica's own edits reload at once.
const holidayRefresh = 30 * time.Second

// holidaySnapshot is the stored holidays as one validated Vocabulary, the one Source every holiday
// consumer reads. A load that fails, or whose rows do not validate, keeps the last good snapshot
// (the shipped definitions before the first load), so a bad row degrades nothing that already airs.
type holidaySnapshot struct {
	store   holidayLister
	log     *slog.Logger
	current atomic.Pointer[holidayvocab.Vocabulary]
}

type holidayLister interface {
	ListHolidays(context.Context) ([]holidayvocab.Holiday, error)
}

func newHolidaySnapshot(store holidayLister, log *slog.Logger) *holidaySnapshot {
	h := &holidaySnapshot{store: store, log: log}
	shipped := holidayvocab.Default()
	h.current.Store(&shipped)
	return h
}

// Current is the Source: the last good snapshot.
func (h *holidaySnapshot) Current() holidayvocab.Vocabulary { return *h.current.Load() }

// Reload reads the stored holidays into the snapshot.
func (h *holidaySnapshot) Reload(ctx context.Context) {
	rows, err := h.store.ListHolidays(ctx)
	if err != nil {
		h.log.Warn("holidays: could not read the stored holidays; keeping the last ones loaded", "err", err)
		return
	}
	v, err := holidayvocab.New(rows)
	if err != nil {
		h.log.Error("holidays: the stored holidays are invalid; keeping the last ones loaded", "err", err)
		return
	}
	h.current.Store(&v)
}

// Run rereads the stored holidays every holidayRefresh until ctx ends.
func (h *holidaySnapshot) Run(ctx context.Context) {
	ticker := time.NewTicker(holidayRefresh)
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
