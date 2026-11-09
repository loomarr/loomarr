package viewing

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// The watch history (#3010): the Recorder writes the tracker's viewings to the store, one row per
// person, device, channel and programme, so an admin can read who watched what afterwards. It
// keeps no retention: rows stay until an admin clears them.
//
// A row is written when a viewing starts and its end is moved forward on every sync while the
// player keeps polling, so a crash or restart loses at most one sync interval of a viewing's end.
// When the guide says the next programme has begun, the row ends and the next one starts. A gap
// (filler between programmes) or a guide that can't be read keeps the current row going.

// SyncInterval is how often the Recorder writes. Shorter than TTL, so a viewing's last poll is
// written before the tracker forgets it.
const SyncInterval = 15 * time.Second

// HistoryStore is what the Recorder writes through.
type HistoryStore interface {
	InsertViewing(ctx context.Context, v store.Viewing) error
	ExtendViewing(ctx context.Context, id, programme string, endedAt time.Time) (bool, error)
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
}

// Programmes is the title airing on each channel at `at`. A channel in a gap, or with no guide,
// is absent or "".
type Programmes func(ctx context.Context, at time.Time) (map[string]string, error)

type openViewing struct {
	id        string
	programme string
	lastSeen  time.Time
}

// Recorder turns the tracker's live viewings into watch-history rows. Sync is not safe for
// concurrent use; Run calls it from one goroutine.
type Recorder struct {
	tracker    *Tracker
	store      HistoryStore
	programmes Programmes
	log        *slog.Logger
	open       map[key]*openViewing
}

// NewRecorder records tracker's viewings into st. programmes may be nil: rows then carry no
// programme and one row spans a whole viewing.
func NewRecorder(tracker *Tracker, st HistoryStore, programmes Programmes, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{tracker: tracker, store: st, programmes: programmes, log: log, open: make(map[key]*openViewing)}
}

// Run syncs every SyncInterval until ctx ends.
func (r *Recorder) Run(ctx context.Context) {
	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			if err := r.Sync(ctx, at); err != nil && ctx.Err() == nil {
				r.log.Warn("watch history: sync failed", "err", err)
			}
		}
	}
}

// Sync writes the viewings live at `at`.
func (r *Recorder) Sync(ctx context.Context, at time.Time) error {
	watches := r.tracker.Watching(at)
	if len(watches) == 0 && len(r.open) == 0 {
		return nil
	}
	var titles map[string]string
	if len(watches) > 0 && r.programmes != nil {
		var err error
		if titles, err = r.programmes(ctx, at); err != nil {
			r.log.Debug("watch history: guide unavailable; keeping current programmes", "err", err)
			titles = nil
		}
	}
	var names map[string]string
	var errs []error
	seen := make(map[key]bool, len(watches))
	for _, w := range watches {
		k := key{w.UserID, w.DeviceKey, w.ChannelID}
		seen[k] = true
		title := titles[w.ChannelID]
		started := w.Since
		if o := r.open[k]; o != nil {
			same := title == "" || o.programme == "" || title == o.programme
			adopt := ""
			if same && o.programme == "" && title != "" {
				adopt = title
			}
			end := w.LastSeen
			if same && adopt == "" && !end.After(o.lastSeen) {
				continue
			}
			found, err := r.store.ExtendViewing(ctx, o.id, adopt, end)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if found && same {
				o.lastSeen = end
				if adopt != "" {
					o.programme = adopt
				}
				continue
			}
			// The next programme began, or an admin cleared the history: a new row starts here.
			started = end
			delete(r.open, k)
		}
		if names == nil {
			names = r.channelNames(ctx)
		}
		v := store.Viewing{ID: strings.ToLower(rand.Text()), UserID: w.UserID, Device: w.DeviceLabel,
			ChannelID: w.ChannelID, ChannelName: names[w.ChannelID], Programme: title,
			StartedAt: started, EndedAt: w.LastSeen}
		if err := r.store.InsertViewing(ctx, v); err != nil {
			errs = append(errs, err)
			continue
		}
		r.open[k] = &openViewing{id: v.ID, programme: title, lastSeen: w.LastSeen}
	}
	// A viewing the tracker no longer has has ended; its last sync already wrote its end.
	for k := range r.open {
		if !seen[k] {
			delete(r.open, k)
		}
	}
	return errors.Join(errs...)
}

// channelNames reads every channel's name once per sync that opens a row. A failed read leaves the
// names empty rather than losing the row.
func (r *Recorder) channelNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	channels, err := r.store.ListChannels(ctx)
	if err != nil {
		r.log.Debug("watch history: channel names unavailable", "err", err)
		return names
	}
	for _, ch := range channels {
		names[ch.ID] = ch.Name
	}
	return names
}
