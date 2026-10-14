package testkit

import (
	"io"
	"log/slog"
)

// Logger returns a slog.Logger that discards all output, for wiring components
// under test without noise. Shared so tests don't each build their own.
func Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
