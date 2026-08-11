// Command config-docs generates docs/configuration.md from the settings registry
// (config-design §2: the same committed-artifact discipline as OpenAPI). `make
// config-docs` runs this; `make config-docs-verify` diffs the result so the tree
// goes red when the registry and the doc drift. The registry is the contract;
// this file is its generated reference.
package main

import (
	"log/slog"
	"os"

	"github.com/mantonx/loomarr/internal/settings"
)

func main() {
	out := "docs/configuration.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	md := settings.NewRegistry().Markdown()
	if err := os.WriteFile(out, md, 0o644); err != nil {
		slog.Error("write config docs", "err", err)
		os.Exit(1)
	}
}
