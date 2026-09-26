package suggest

import (
	"context"
	"slices"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// #1499: a request that names example titles but no era ("movies like Back to
// the Future and Gremlins for a family night") gave the model nothing to search
// by except the titles' words, so its retrieval pool was title-word noise (Sister
// Act 2: Back in the Habit, To Be or Not to Be) and the lineup either stopped at
// the anchors or filled with off-era picks. The era and genre are still there:
// the resolved anchors carry them. This derives both deterministically, runs one
// in-library discovery with no LLM call, drops off-era non-anchor picks and tops
// the lineup up from the neighbours.

const (
	// maxExampleEraSpan is the widest spread of anchor release years that still
	// reads as one era; Casablanca and Avatar name none.
	maxExampleEraSpan = 12
	// exampleEraPad widens the anchors' years on both sides: a neighbour a few
	// years either side of the anchors is on-era.
	exampleEraPad = 5
	// maxExampleNeighbours bounds the pool kept for top-up.
	maxExampleNeighbours = 2 * maxFinalSelectionPicks
)

// exampleFill is the era and neighbour pool derived from resolved example
// anchors. It is shared by pointer across Intent copies, like requiredTitleKeys.
type exampleFill struct {
	from, to   int
	neighbours []catalog.Candidate
}

// groundExampleFill derives the era window and genre of the resolved example
// anchors and discovers in-library neighbours inside them. It applies only when
// the request states no date of its own (meaning kind none): a stated era is
// already the model's search constraint. Discovery is best-effort; a failed or
// empty one leaves the lineup exactly as the model chose it.
func (s *Suggester) groundExampleFill(ctx context.Context, intent *Intent, meaning ValidatedDateMeaning, anchors []catalog.Candidate) {
	if meaning.DateMeaning().Kind != DateMeaningNone {
		return
	}
	var movies []catalog.Candidate
	for _, anchor := range anchors {
		if anchor.MediaType == provision.Movie && anchor.Year > 0 {
			movies = append(movies, anchor)
		}
	}
	if len(movies) == 0 {
		return
	}
	first, last := movies[0].Year, movies[0].Year
	for _, movie := range movies {
		first, last = min(first, movie.Year), max(last, movie.Year)
	}
	if last-first > maxExampleEraSpan {
		return
	}
	fill := &exampleFill{from: first - exampleEraPad, to: last + exampleEraPad}
	intent.exampleFill = fill
	genre := sharedAnchorGenre(movies)
	if genre == "" {
		return
	}
	base := catalog.DiscoveryQuery{MediaType: provision.Movie, Genres: []string{genre}, YearFrom: fill.from, YearTo: fill.to}
	queries := []catalog.DiscoveryQuery{base}
	if intentSignalsKids(*intent) && genre != "Family" {
		family := base
		family.Genres = []string{genre, "Family"}
		queries = []catalog.DiscoveryQuery{family, base}
	}
	found, err := s.catalog.DiscoverUnion(ctx, queries)
	if err != nil {
		return
	}
	isAnchor := make(map[provision.Key]bool, len(anchors))
	for _, anchor := range anchors {
		if key, keyErr := anchor.Key(); keyErr == nil {
			isAnchor[key] = true
		}
	}
	var family, rest []catalog.Candidate
	for _, candidate := range found.Candidates {
		key, keyErr := candidate.Key()
		if keyErr != nil || isAnchor[key] || !candidate.InLibrary || candidate.MediaType != provision.Movie ||
			candidate.Year < fill.from || candidate.Year > fill.to || titleExplicitlyExcluded(*intent, candidate.Name) {
			continue
		}
		if slices.Contains(candidate.Genres, "Family") && intentSignalsKids(*intent) {
			family = append(family, candidate)
		} else {
			rest = append(rest, candidate)
		}
	}
	fill.neighbours = append(family, rest...)
	if len(fill.neighbours) > maxExampleNeighbours {
		fill.neighbours = fill.neighbours[:maxExampleNeighbours]
	}
}

// sharedAnchorGenre is the genre most anchors carry (first listed wins a tie).
// With several anchors a genre only one of them has is not shared, so none is
// chosen and the era alone shapes the lineup.
func sharedAnchorGenre(anchors []catalog.Candidate) string {
	counts := map[string]int{}
	var order []string
	for _, anchor := range anchors {
		for _, genre := range anchor.Genres {
			if counts[genre] == 0 {
				order = append(order, genre)
			}
			counts[genre]++
		}
	}
	best := ""
	for _, genre := range order {
		if best == "" || counts[genre] > counts[best] {
			best = genre
		}
	}
	if best == "" || len(anchors) > 1 && counts[best] < 2 {
		return ""
	}
	return best
}

// completeExampleSelection drops non-anchor picks outside the anchors' era and
// tops the lineup up to the pick target from the era's neighbours. Required
// anchors are never dropped.
func completeExampleSelection(intent Intent, picks []pick, surfaced map[provision.Key]catalog.Candidate) []pick {
	fill := intent.exampleFill
	if fill == nil {
		return picks
	}
	required := make(map[provision.Key]bool, len(intent.requiredTitleKeys))
	for _, key := range intent.requiredTitleKeys {
		required[key] = true
	}
	kept := make([]pick, 0, maxFinalSelectionPicks)
	chosen := make(map[provision.Key]bool, maxFinalSelectionPicks)
	for _, proposed := range picks {
		key := provision.Key(proposed.key())
		candidate, found := surfaced[key]
		// A pick the retrieval never surfaced cannot ground later, so it must not hold a slot.
		if !found || !required[key] && candidate.Year > 0 && (candidate.Year < fill.from || candidate.Year > fill.to) {
			continue
		}
		if len(kept) < maxFinalSelectionPicks {
			kept = append(kept, proposed)
			chosen[key] = true
		}
	}
	for _, candidate := range fill.neighbours {
		if len(kept) == maxFinalSelectionPicks {
			break
		}
		key, err := candidate.Key()
		if err != nil || chosen[key] {
			continue
		}
		if _, found := surfaced[key]; !found {
			continue
		}
		kept = append(kept, pick{
			MediaType: string(candidate.MediaType), Key: string(key), Name: candidate.Name,
			Year: candidate.Year, Confidence: 0.6,
		})
		chosen[key] = true
	}
	return kept
}
