package scheduler

import (
	"context"
	"log/slog"
	"strings"
)

// riverChatter are River's per-tick housekeeping messages. They carry no signal an operator can
// act on but fire constantly: the periodic enqueuer logs every cron tick whose constructor
// returned nil (our live-cron gate, so most ticks) and each producer logs its job counts. In
// production they were ~80% of the container log and rotated away hours of useful evidence.
var riverChatter = []string{
	"nil returned from periodic job constructor",
	"Producer job counts",
}

// quietRiverLogger wraps log so River's chatter is emitted at DEBUG instead of INFO. Errors and
// warnings are never demoted, and every other INFO line passes through untouched.
func quietRiverLogger(log *slog.Logger) *slog.Logger {
	if log == nil {
		return nil
	}
	return slog.New(&riverQuietHandler{inner: log.Handler()})
}

type riverQuietHandler struct{ inner slog.Handler }

func (h *riverQuietHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *riverQuietHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelInfo && isRiverChatter(r.Message) {
		r.Level = slog.LevelDebug
		if !h.inner.Enabled(ctx, slog.LevelDebug) {
			return nil
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h *riverQuietHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &riverQuietHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *riverQuietHandler) WithGroup(name string) slog.Handler {
	return &riverQuietHandler{inner: h.inner.WithGroup(name)}
}

func isRiverChatter(message string) bool {
	for _, fragment := range riverChatter {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
