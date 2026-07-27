package catalog_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

func seed(id int, name string) catalog.Candidate {
	return catalog.Candidate{MediaType: provision.Movie, TMDBID: id, Name: name, Year: 1994}
}

// CONSENSUS IS THE SIGNAL (programming-design §8.3). One seed pointing at a title is
// noise — TMDB's graph connects almost anything to something. A title several of the
// channel's own films independently recommend is what "on-theme" means here, so a
// one-vote candidate must NOT be offered.
func TestAdjacent_RequiresConsensusAcrossSeeds(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		100: {603, 101}, // Speed  → Matrix, The Rock
		101: {603},      // Rock   → Matrix       (Matrix now has 2 votes)
	})
	c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))

	got, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(100, "Speed"), seed(101, "The Rock")},
		map[provision.Key]bool{}, 10)
	if err != nil {
		t.Fatalf("Adjacent: %v", err)
	}

	names := map[string]bool{}
	for _, a := range got {
		names[a.Candidate.Name] = true
	}
	if !names["The Matrix"] {
		t.Errorf("The Matrix had 2 votes and must be offered; got %v", names)
	}
	if names["The Rock"] {
		t.Errorf("The Rock had 1 vote (noise) and must not be offered; got %v", names)
	}
}

// A neighbour the channel ALREADY has is not a discovery. Without this the top of every
// adjacency run would be the channel's own lineup recommending itself.
func TestAdjacent_ExcludesWhatTheChannelAlreadyHas(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		100: {603}, 101: {603},
	})
	c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))

	matrixKey, err := seed(603, "The Matrix").Key()
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	got, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(100, "Speed"), seed(101, "The Rock")},
		map[provision.Key]bool{matrixKey: true}, 10)
	if err != nil {
		t.Fatalf("Adjacent: %v", err)
	}
	for _, a := range got {
		if a.Candidate.TMDBID == 603 {
			t.Fatalf("The Matrix is already on the channel and must be excluded; got %v", got)
		}
	}
}

// Ranking is by consensus, and ties break deterministically (§7 reproducibility): the
// SAME seeds must yield the SAME order every run, not map-iteration order.
func TestAdjacent_IsDeterministic(t *testing.T) {
	graph := map[int][]int{100: {603, 101}, 101: {603, 100}, 603: {100, 101}}
	var first []string
	for range 5 {
		mt := testkit.NewTMDB(t).WithRecommendations(graph)
		c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))
		got, err := c.Adjacent(context.Background(),
			[]catalog.Candidate{seed(100, "Speed"), seed(101, "The Rock"), seed(603, "The Matrix")},
			map[provision.Key]bool{}, 10)
		if err != nil {
			t.Fatalf("Adjacent: %v", err)
		}
		names := make([]string, 0, len(got))
		for _, a := range got {
			names = append(names, a.Candidate.Name)
		}
		if first == nil {
			first = names
			continue
		}
		if len(names) != len(first) {
			t.Fatalf("run differed in length: %v vs %v", names, first)
		}
		for i := range names {
			if names[i] != first[i] {
				t.Fatalf("non-deterministic order: %v vs %v", names, first)
			}
		}
	}
}

// A seed with no graph (obscure title, or TMDB has nothing) must be SKIPPED, not fatal:
// adjacency is a bonus corpus, and degrading to fewer candidates is right where failing
// the whole re-curation would not be.
func TestAdjacent_UnproductiveSeedIsSkipped(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		100: {603}, 101: {603}, // 999 deliberately unwired
	})
	c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))

	got, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(999, "Nonexistent"), seed(100, "Speed"), seed(101, "The Rock")},
		map[provision.Key]bool{}, 10)
	if err != nil {
		t.Fatalf("an unproductive seed must not fail the walk: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the productive seeds must still yield candidates")
	}
}

// No adjacency corpus wired (no TMDB client) ⇒ no candidates, no error. An install
// without TMDB keeps working; it simply has one fewer corpus.
func TestAdjacent_NoCorpusIsEmptyNotAnError(t *testing.T) {
	c := catalog.New(realLibrary(t), nil)
	got, err := c.Adjacent(context.Background(), []catalog.Candidate{seed(100, "Speed")},
		map[provision.Key]bool{}, 10)
	if err != nil {
		t.Fatalf("no corpus must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("no corpus must yield nothing; got %v", got)
	}
}

// ONE VOTE PER SEED, not per occurrence. TMDB can list the same neighbour more than once
// for a single seed (paged/duplicated rows), and counting each would let ONE enthusiastic
// seed manufacture "consensus" on its own — defeating the whole signal. A candidate named
// twice by one seed must still hold exactly one vote, and so stay below the threshold.
func TestAdjacent_OneVotePerSeedNotPerOccurrence(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		100: {603, 603, 603}, // Speed names The Matrix three times — still ONE seed
	})
	c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))

	got, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(100, "Speed")}, map[provision.Key]bool{}, 10)
	if err != nil {
		t.Fatalf("Adjacent: %v", err)
	}
	for _, a := range got {
		if a.Candidate.TMDBID == 603 {
			t.Fatalf("one seed repeated a neighbour into false consensus; got %v", got)
		}
	}
}

// The VOTE COUNT is the explainability payload — "recommended by N of your films" is what
// the approval card shows and what makes an adjacency pick defensible where an LLM pick can
// only paraphrase. Returning the candidates without it would leave the ranking legible only
// to the ranker.
func TestAdjacent_ReportsTheConsensusCount(t *testing.T) {
	mt := testkit.NewTMDB(t).WithRecommendations(map[int][]int{
		100: {603}, 101: {603}, 603: {603}, // three seeds all name The Matrix
	})
	c := catalog.New(realLibrary(t), tmdb.NewWithBase(mt.URL, "test-key"))

	got, err := c.Adjacent(context.Background(),
		[]catalog.Candidate{seed(100, "Speed"), seed(101, "The Rock"), seed(603, "The Matrix")},
		map[provision.Key]bool{}, 10)
	if err != nil {
		t.Fatalf("Adjacent: %v", err)
	}
	var found bool
	for _, a := range got {
		if a.Candidate.TMDBID != 603 {
			continue
		}
		found = true
		if a.Votes != 3 {
			t.Errorf("votes = %d, want 3 (one per seed that named it)", a.Votes)
		}
	}
	if !found {
		t.Fatalf("The Matrix should have been offered; got %v", got)
	}
}
