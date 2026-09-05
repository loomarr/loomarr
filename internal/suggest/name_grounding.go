package suggest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

type nameGroundingResult struct {
	proposed  pick
	candidate catalog.Candidate
	err       error
	found     bool
}

const maxPickNameQueries = 8

// groundPickNames treats model-authored names only as bounded Catalog search
// input. It replaces every claimed id with the one unambiguous exact candidate
// returned by the Catalog before adding that candidate to the surfaced set.
func (s *Suggester) groundPickNames(
	ctx context.Context,
	intent Intent,
	feedback []FeedbackSignal,
	picks []pick,
	surfaced map[provision.Key]catalog.Candidate,
	trace *DecisionTrace,
) ([]pick, error) {
	queries := make([]pick, 0, min(len(picks), maxPickNameQueries))
	seen := make(map[string]bool)
	for _, proposed := range picks {
		if len(queries) == maxPickNameQueries {
			break
		}
		mediaType := provision.MediaType(proposed.MediaType)
		name := strings.Join(strings.Fields(proposed.Name), " ")
		if !mediaType.Valid() || name == "" {
			continue
		}
		queryKey := fmt.Sprintf("%s\x00%s", mediaType, strings.ToLower(name))
		if seen[queryKey] {
			continue
		}
		seen[queryKey] = true
		proposed.Name = name
		queries = append(queries, proposed)
	}

	results := make([]nameGroundingResult, len(queries))
	var wg sync.WaitGroup
	for index, proposed := range queries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = s.groundPickName(ctx, proposed)
		}()
	}
	wg.Wait()

	grounded := make([]pick, 0, len(results))
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		if !result.found {
			continue
		}
		candidate := result.candidate
		ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), []catalog.Candidate{candidate}, feedback)
		mergeDecisionTrace(trace, &ranked.Trace)
		key, _ := candidate.Key()
		surfaced[key] = candidate
		proposed := result.proposed
		proposed.MediaType = string(candidate.MediaType)
		proposed.TMDBID = candidate.TMDBID
		proposed.TVDBID = candidate.TVDBID
		proposed.Name = candidate.Name
		grounded = append(grounded, proposed)
	}
	return grounded, nil
}

func (s *Suggester) groundPickName(ctx context.Context, proposed pick) nameGroundingResult {
	mediaType := provision.MediaType(proposed.MediaType)
	candidates, err := s.catalog.Search(ctx, proposed.Name, catalog.ScopeAll, catalogSearchLimit)
	if err != nil {
		return nameGroundingResult{proposed: proposed, err: fmt.Errorf("ground proposed title %q: %w", proposed.Name, err)}
	}
	byKey := make(map[provision.Key]catalog.Candidate)
	for _, candidate := range candidates {
		if candidate.MediaType != mediaType || !sameExactTitle(candidate.Name, proposed.Name) ||
			(proposed.Year > 0 && candidate.Year != proposed.Year) {
			continue
		}
		key, keyErr := candidate.Key()
		if keyErr == nil {
			byKey[key] = candidate
		}
	}
	if len(byKey) != 1 {
		return nameGroundingResult{proposed: proposed}
	}
	var candidate catalog.Candidate
	for _, exact := range byKey {
		candidate = exact
	}
	return nameGroundingResult{proposed: proposed, candidate: candidate, found: true}
}
