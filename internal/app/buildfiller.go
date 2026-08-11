package app

import (
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/store"
)

// This file is the first of BuildHandler's per-subsystem builders (§14.1).
//
// ⚠ **The shape matters, and it is NOT the one §14.1 rejected.** That rejection was for methods
// on a shared builder struct, which would convert ~70 of BuildHandler's locals into fields on a
// mutable carrier — widening their scope and trading compile-time use-before-assignment errors
// for runtime nils. A `buildX` FUNCTION takes only what it needs and RETURNS values, so nothing
// widens and the compiler still catches use-before-assignment. §14.1 records the distinction.
//
// The measurement that makes this tractable: BuildHandler's 434-line filler section reads only
// EIGHT locals from earlier sections, while its 133-line suggester section reads far more.
// Coupling, not size, predicts extraction cost — extract along the seam, not the line count.

// buildTagger constructs the AI-tagging provider and the manual whole-catalog tagger (§10).
//
// Returns BOTH because they have different lifetimes and different consumers: the provider is
// also handed to the ingest pipeline's tag rung (§10 V51b), which is why it is hoisted out of
// the `if` rather than living inside the tagger. Nil for both is the honest un-opted-in state,
// and every reader treats it that way — the manual sweep becomes a no-op, and the rung reports
// "no language model is configured" on each clip's ladder rather than silently doing nothing.
func buildTagger(st store.Store, set resolved, log *slog.Logger) (llm.Provider, *filler.Tagger) {
	if !set.boolv("filler.ai_tagging") || set.str("llm.url") == "" {
		return nil, nil
	}

	provider := llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), set.str("llm.model"), set.str("llm.api_key"))

	// The drop-folder as an fs.FS so tagging can read the info-JSON sidecars ingest writes
	// beside each clip (§10). An unset FILLER_DIR yields a nil FS and tagging falls back to
	// filenames — the same result as a drop-folder clip that never had a sidecar.
	var drop fs.FS
	if dir := set.str("filler.dir"); dir != "" {
		drop = os.DirFS(dir)
	}

	tagger := filler.NewTagger(fillerTagStoreAdapter{st}, provider, drop, time.Now, log).
		// Auto-filing (§10 V38): a held clip whose grounding-capped score clears the threshold
		// is filed without a human. Closures, not captured values, so a changed threshold
		// applies on the next run rather than the next restart.
		//
		// ⚠ `boolv`, NOT `boolOn`. The two differ only when the settings service cannot answer,
		// and here that difference is the whole safety property: `boolOn` fails OPEN (returns
		// true), which would publish unreviewed clips to live channels exactly when the install
		// is degraded. Holding is the safe failure.
		WithAutoFile(filler.AutoFilePolicy{
			Enabled:       func() bool { return set.boolv("filler.autofile.enabled") },
			MinConfidence: func() int { return set.intv("filler.autofile.min_confidence") },
		})

	return provider, tagger
}
