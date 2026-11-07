// Command settings-keys generates internal/platform/settings/keys_gen.go, the typed handle for
// every setting in the registry. `go generate ./internal/platform/settings` runs it; the settings package's
// TestKeysGenIsCurrent fails when the registry and the generated file drift.
package main

import (
	"log/slog"
	"os"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

func main() {
	out := "internal/platform/settings/keys_gen.go"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	src, err := settings.NewRegistry().KeysSource()
	if err != nil {
		slog.Error("render settings keys", "err", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		slog.Error("write settings keys", "err", err)
		os.Exit(1)
	}
}
