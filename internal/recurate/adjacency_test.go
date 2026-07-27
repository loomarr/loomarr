package recurate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
)

// fakeAdjacencer records the seeds it was walked with and returns a fixed answer.
type fakeAdjacencer struct {
	seeds   []catalog.Candidate
	exclude map[provision.Key]bool
	out     []catalog.Adjacency
	err     error
}

func (f *fakeAdjacencer) Adjacent(_ context.Context, seeds []catalog.Candidate,
	exclude map[provision.Key]bool, _ int) ([]catalog.Adjacency, error) {
	f.seeds, f.exclude = seeds, exclude
	return f.out, f.err
}

func seedChannelWithLineup(t *testing.T, st store.Store, id, jobID string, lineup []schedule.LineupEntry) {
	t.Helper()
	ch := store.Channel{}
	ch.ID = id
	ch.IntentRef = jobID
	ch.Name = "Ch " + id
	ch.Number = 41
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Policy = schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}}
	ch.Lineup = lineup
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// The adjacency corpus is walked from the channel's OWN lineup and its results ride into the
// refine intent (§8.3) — that is what makes re-curation consult both corpora in one run
// rather than alternating between them.
func TestRunner_SeedsAdjacencyFromTheChannelsLineup(t *testing.T) {
	st := newStore(t)
	seedJob(t, st, "job-1", "80s action")
	seedChannelWithLineup(t, st, "ch-1", "job-1", []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:562"), Title: "Die Hard", Year: 1988},
		{Key: provision.Key("movie:tmdb:218"), Title: "The Terminator", Year: 1984},
	})

	adj := &fakeAdjacencer{out: []catalog.Adjacency{{
		Candidate: catalog.Candidate{MediaType: provision.Movie, TMDBID: 280, Name: "Terminator 2", Year: 1991},
		Votes:     3,
	}}}
	ref := &fakeRefiner{}
	r := recurate.NewRunner(st, ref, testkit.Logger()).WithAdjacency(adj)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(adj.seeds) != 2 {
		t.Fatalf("walked with %d seeds, want 2 (the channel's lineup)", len(adj.seeds))
	}
	// The channel's own titles must be excluded — a neighbour it already has is not a find.
	if !adj.exclude[provision.Key("movie:tmdb:562")] {
		t.Error("the channel's existing titles must be excluded from the walk")
	}
	got := ref.intents["job-1"].Adjacent
	if len(got) != 1 || got[0].Name != "Terminator 2" {
		t.Fatalf("adjacency did not reach the refine intent: %+v", got)
	}
	if got[0].Votes != 3 {
		t.Errorf("votes = %d, want 3 (the consensus that explains the pick)", got[0].Votes)
	}
	if got[0].Key != "movie:tmdb:280" {
		t.Errorf("key = %q, want movie:tmdb:280 (grounded identity)", got[0].Key)
	}
}

// A failing walk must NOT fail the run: adjacency widens a re-curation, it never gates one.
// The LLM corpus alone is the pre-adjacency behaviour and remains a correct outcome.
func TestRunner_AdjacencyFailureStillRefines(t *testing.T) {
	st := newStore(t)
	seedJob(t, st, "job-1", "80s action")
	seedChannelWithLineup(t, st, "ch-1", "job-1", []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:562"), Title: "Die Hard", Year: 1988},
	})

	ref := &fakeRefiner{}
	r := recurate.NewRunner(st, ref, testkit.Logger()).WithAdjacency(&fakeAdjacencer{err: errors.New("tmdb down")})

	kicked, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("an adjacency failure must not fail the run: %v", err)
	}
	if kicked != 1 {
		t.Fatalf("kicked = %d, want 1 (the refine still runs)", kicked)
	}
	if len(ref.intents["job-1"].Adjacent) != 0 {
		t.Error("a failed walk must contribute no candidates")
	}
}

// No adjacency wired at all ⇒ exactly the pre-§8.3 behaviour. An install without TMDB keeps
// re-curating on the LLM corpus alone.
func TestRunner_NoAdjacencyCorpusIsUnchangedBehaviour(t *testing.T) {
	st := newStore(t)
	seedJob(t, st, "job-1", "80s action")
	seedChannelWithLineup(t, st, "ch-1", "job-1", []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:562"), Title: "Die Hard", Year: 1988},
	})

	ref := &fakeRefiner{}
	kicked, err := recurate.NewRunner(st, ref, testkit.Logger()).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if kicked != 1 {
		t.Fatalf("kicked = %d, want 1", kicked)
	}
	if len(ref.intents["job-1"].Adjacent) != 0 {
		t.Error("no corpus must contribute no candidates")
	}
}

// A lineup with no TMDB-keyed titles has no graph to walk (a TVDB-only series channel), so
// the walk is skipped entirely rather than called with zero seeds.
func TestRunner_NonTMDBLineupSkipsTheWalk(t *testing.T) {
	st := newStore(t)
	seedJob(t, st, "job-1", "classic tv")
	seedChannelWithLineup(t, st, "ch-1", "job-1", []schedule.LineupEntry{
		{Key: provision.Key("series:tvdb:71470"), Title: "Star Trek", Year: 1966},
	})

	adj := &fakeAdjacencer{}
	r := recurate.NewRunner(st, &fakeRefiner{}, testkit.Logger()).WithAdjacency(adj)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if adj.seeds != nil {
		t.Fatalf("the walk should be skipped with no TMDB seeds; got %v", adj.seeds)
	}
}

// ── The unscored-adjacency floor (§8.3) ──────────────────────────────────────────────────

// adjItem is an acquisition candidate that came from the adjacency corpus.
func adjItem(tmdbID int, name string, confidence float64) suggest.ProposalItem {
	it := acqItem(tmdbID, name, confidence)
	it.Source = string(catalog.ScopeAdjacent)
	return it
}

// THE GAP THIS CLOSES: the bar reads a confidence the MODEL assigns, and an omitted one
// unmarshals to 0 — correctly fatal for a title the model searched for and then declined to
// stand behind. An adjacency pick was HANDED to the model with a consensus it neither computed
// nor can see, so a model that scores only what it found itself would silently zero out the
// entire second corpus — and the drop count would read as "the bar is working".
func TestCurator_UnscoredAdjacencyPickSurvivesTheBar(t *testing.T) {
	st := newStore(t)
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{})
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		adjItem(9659, "Mad Max", 0), // model returned no score for a title it was handed
	})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (an unscored adjacency pick rides its consensus)", d.Enqueued)
	}
	if titleState(t, st, 9659) != provision.Wanted {
		t.Errorf("state = %q, want wanted", titleState(t, st, 9659))
	}
}

// The floor lifts an ABSENT score, never a low one. A model that looked and judged the title
// weak keeps its judgement — otherwise this would be a bypass rather than a floor.
func TestCurator_ScoredAdjacencyPickKeepsTheModelsJudgement(t *testing.T) {
	st := newStore(t)
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{})
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		adjItem(111, "Weak Fit", 0.20), // the model DID score it, and scored it poorly
	})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 {
		t.Fatalf("enqueued = %d, want 0 (a model-scored 0.20 is still below a 60 bar)", d.Enqueued)
	}
}

// An LLM pick with no confidence keeps the original conservative treatment: the model searched
// for it and declined to stand behind it, which IS a real signal about spending. The floor is
// narrow on purpose — it applies to the adjacency corpus alone.
func TestCurator_UnscoredLLMPickIsStillDropped(t *testing.T) {
	st := newStore(t)
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{})
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(222, "Unscored LLM Pick", 0), // no Source ⇒ not adjacency
	})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 {
		t.Fatalf("enqueued = %d, want 0 (an unscored LLM pick stays conservative)", d.Enqueued)
	}
}
