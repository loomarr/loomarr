//go:build eval

package eval

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// buildSuggester constructs the REAL suggester exactly as main.go does — real LLM
// provider (from LLM_*), real catalog (real library search + real TMDB), real
// validator — so the eval exercises the production path, not a mock. Returns an
// error (skips the eval) when the required env isn't configured.
func buildSuggester() (*suggest.Suggester, *tmdb.Client, error) {
	libURL := os.Getenv("LIBRARY_URL")
	libTok := os.Getenv("LIBRARY_TOKEN")
	if libURL == "" || libTok == "" {
		return nil, nil, fmt.Errorf("LIBRARY_URL + LIBRARY_TOKEN required for the eval")
	}
	flavor := library.Emby
	if os.Getenv("LIBRARY_FLAVOR") == "jellyfin" {
		flavor = library.Jellyfin
	}
	lib := library.New(flavor, libURL, libTok, "loomarr-eval")

	tmdbKey := os.Getenv("TMDB_API_KEY")
	if tmdbKey == "" {
		return nil, nil, fmt.Errorf("TMDB_API_KEY required for the eval (grounding + discovery)")
	}
	tm := tmdb.New(tmdbKey)
	cat := catalog.New(lib, tm, nil).WithPresence(libPresence{lib})

	provider := buildProvider()
	if provider == nil {
		return nil, nil, fmt.Errorf("LLM not configured (set LLM_PROVIDER/LLM_URL/LLM_MODEL[/LLM_API_KEY])")
	}
	return suggest.New(provider, cat, tm, 10), tm, nil
}

// buildProvider mirrors cmd/loomarr's buildProviderFor: Ollama for local, the
// OpenAI-compatible client otherwise. Driven by env (the same knobs production uses).
func buildProvider() llm.Provider {
	prov := os.Getenv("LLM_PROVIDER")
	url := os.Getenv("LLM_URL")
	model := os.Getenv("LLM_MODEL")
	key := os.Getenv("LLM_API_KEY")
	if model == "" {
		return nil
	}
	if prov == "ollama" || prov == "" {
		return llm.NewOllama(url, model)
	}
	return llm.NewOpenAI(url, model, key)
}

// libPresence adapts the library client to catalog.LibraryPresence (mirrors main.go).
type libPresence struct{ lib *library.Client }

func (a libPresence) Present(ctx context.Context, mt provision.MediaType, tmdbID, tvdbID int) (string, bool, error) {
	kind, id := library.TMDB, strconv.Itoa(tmdbID)
	if tmdbID == 0 && tvdbID != 0 {
		kind, id = library.TVDB, strconv.Itoa(tvdbID)
	}
	lmt := library.Movie
	if mt == provision.Series {
		lmt = library.Series
	}
	return a.lib.Lookup(ctx, kind, id, lmt)
}

// mapIntent converts the corpus Intent to a suggest.Intent.
func mapIntent(i Intent) suggest.Intent {
	return suggest.Intent{
		Description: i.Description, Era: i.Era, Tone: i.Tone, RuntimeTgt: i.RuntimeTgt,
		MustInclude: i.MustInclude, MustExclude: i.MustExclude, MaxAcquire: i.MaxAcquire,
	}
}

// Result is the scored outcome of one case.
type Result struct {
	Case         string
	Failures     []string // deterministic gate failures (empty = passed the hard gates)
	ThemeFit     float64
	Lineup       int
	Acquisitions int
	Ceiling      string  // the extracted policy ceiling
	JudgeScore   float64 // -1 when the judge didn't run
	JudgeNote    string
}

func (r Result) Passed() bool { return len(r.Failures) == 0 }

// deterministicChecks applies the corpus case's hard expectations to a proposal.
// These are the assertions a mock could never make — they run against the real
// grounded output. Returns the list of failures (empty = all gates passed).
func deterministicChecks(c Case, prop suggest.Proposal, groundErr error) []string {
	var f []string

	// No-fabrication case: the grounded set may be any size (even 0), but EVERY item
	// must have a real, resolvable id — the grounding guarantee. A clean empty/failed
	// result also satisfies it (nothing fabricated).
	if c.NoFabrication {
		if groundErr != nil {
			return nil // a clean grounding failure fabricated nothing → passes
		}
		for _, it := range allItems(prop) {
			if _, err := it.Key(); err != nil {
				f = append(f, fmt.Sprintf("FABRICATION: grounded item %q has no resolvable id: %v", it.Name, err))
			}
		}
		return f
	}
	if groundErr != nil {
		return []string{fmt.Sprintf("grounding failed: %v", groundErr)}
	}

	if len(prop.Lineup) < c.MinLineup {
		f = append(f, fmt.Sprintf("lineup %d < required %d", len(prop.Lineup), c.MinLineup))
	}
	if len(prop.Acquisitions) < c.MinAcquisitions {
		f = append(f, fmt.Sprintf("acquisitions %d < required %d", len(prop.Acquisitions), c.MinAcquisitions))
	}
	if c.ExpectCeiling != "" && string(prop.Policy.Audience.Ceiling) != c.ExpectCeiling {
		f = append(f, fmt.Sprintf("expected ceiling %q, extracted %q", c.ExpectCeiling, prop.Policy.Audience.Ceiling))
	}
	if c.ExpectSeasonalMode != "" && string(prop.Policy.Seasonal.Mode) != c.ExpectSeasonalMode {
		f = append(f, fmt.Sprintf("expected seasonal mode %q, extracted %q", c.ExpectSeasonalMode, prop.Policy.Seasonal.Mode))
	}
	if c.MinThemeFit > 0 && prop.Scores.ThemeFit < c.MinThemeFit {
		f = append(f, fmt.Sprintf("themeFit %.2f < required %.2f", prop.Scores.ThemeFit, c.MinThemeFit))
	}

	// SAFETY: no grounded item may carry a rating above the forbidden ceiling — the
	// end-to-end audience guarantee, measured on the real grounded output.
	if c.ForbidRatingsAbove != "" {
		ceiling := schedule.NormalizeRating(c.ForbidRatingsAbove)
		for _, it := range allItems(prop) {
			r := schedule.NormalizeRating(it.OfficialRating)
			// A rated item ABOVE the ceiling is a hard failure. An unrated item is not
			// asserted here (the enforcer fails it closed at reconcile; the corpus's
			// ExpectCeiling covers the extraction side).
			if r != "" && !ratingAtOrBelow(r, ceiling) {
				f = append(f, fmt.Sprintf("grounded item %q is rated %q, above the forbidden ceiling %q",
					it.Name, it.OfficialRating, c.ForbidRatingsAbove))
			}
		}
	}

	// Era binding: every item with a known year must fall within the expected range
	// (with a small slack, since discovery era is fuzzy).
	if c.ExpectEraFrom > 0 && c.ExpectEraTo > 0 {
		const slack = 3
		for _, it := range allItems(prop) {
			if it.Year > 0 && (it.Year < c.ExpectEraFrom-slack || it.Year > c.ExpectEraTo+slack) {
				f = append(f, fmt.Sprintf("grounded item %q (%d) is outside the expected era %d-%d",
					it.Name, it.Year, c.ExpectEraFrom, c.ExpectEraTo))
			}
		}
	}
	return f
}

func allItems(p suggest.Proposal) []suggest.ProposalItem {
	out := append([]suggest.ProposalItem{}, p.Lineup...)
	return append(out, p.Acquisitions...)
}

// ratingAtOrBelow reports whether item rating r is at or below ceiling on the
// shared ladder. Both must be mappable; an unmappable r returns false (caller
// handles the unrated case separately).
func ratingAtOrBelow(r, ceiling schedule.Rating) bool {
	// NormalizeRating already mapped both onto the ladder; reuse the schedule
	// package's own comparison by round-tripping through a tiny policy check would
	// be heavy — instead rank locally via the exported normalize + a fixed order.
	return ladderIndex(r) >= 0 && ladderIndex(ceiling) >= 0 && ladderIndex(r) <= ladderIndex(ceiling)
}

// ladderIndex mirrors schedule's rating ladder order for the eval's own comparison
// (kept local so the eval doesn't need an exported rank). Unmapped → -1.
func ladderIndex(r schedule.Rating) int {
	switch r {
	case "TV-Y":
		return 0
	case "TV-Y7":
		return 1
	case "TV-G", "G":
		return 2
	case "TV-PG", "PG":
		return 3
	case "TV-14", "PG-13":
		return 4
	case "TV-MA", "R", "NC-17":
		return 5
	}
	return -1
}
