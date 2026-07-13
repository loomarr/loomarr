// Command openapi exports Loomarr's OpenAPI 3.1 spec to api/openapi.yaml from the
// same Huma operation definitions that serve the API (§7.1 single source of
// truth). `make openapi` runs this; `make openapi-verify` diffs the result so CI
// goes red on drift.
package main

import (
	"log/slog"
	"os"

	"github.com/mantonx/loomarr/internal/api"
)

func main() {
	out := "api/openapi.yaml"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	spec, err := api.ExportOpenAPI(slog.New(slog.DiscardHandler))
	if err != nil {
		slog.Error("export openapi", "err", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, spec, 0o644); err != nil {
		slog.Error("write spec", "err", err)
		os.Exit(1)
	}
}
