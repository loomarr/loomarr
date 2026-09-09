package app

import (
	"context"
	"log/slog"
)

// Temporary bounded startup probe. Only known non-secret startup fields leave
// the isolated target; all other diagnostic records remain discarded.
type certificationStartupProbe struct{ slog.Handler }

func (h certificationStartupProbe) Handle(ctx context.Context, r slog.Record) error {
	switch r.Message {
	case "source open begins", "source open returns", "parent spawn returned", "prepared startup lookup", "playout: session.start spawning encoder", "playout: block first bytes from child", "playout: session first bytes from parent":
		r.Message = "[DEBUG-beta5-raw] " + r.Message
		return h.Handler.Handle(ctx, r)
	}
	return nil
}
func (h certificationStartupProbe) WithAttrs(a []slog.Attr) slog.Handler {
	return certificationStartupProbe{h.Handler.WithAttrs(a)}
}
func (h certificationStartupProbe) WithGroup(s string) slog.Handler {
	return certificationStartupProbe{h.Handler.WithGroup(s)}
}
