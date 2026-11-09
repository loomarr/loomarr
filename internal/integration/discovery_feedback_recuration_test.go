package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// actionDiscovery is the model's genre discovery, as the prompt asks for a genre intent: TMDB's
// Action films, marked in-library from the Library's titles. A bare "action" query would match no
// title.
func actionDiscovery() llm.Response {
	return testkit.ToolCallResponse("catalog_search", map[string]any{
		"genres": []any{"Action"}, "media_type": "movie",
		"dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}},
	})
}

func TestDiscoveryFeedback_DurableRecurationKeepsChannelScope(t *testing.T) {
	llmMock := testkit.NewLLM(t,
		actionDiscovery(),
		testkit.FinalResponse(`{"dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"},
			{"mediaType":"movie","key":"movie:tmdb:90000100","name":"Quiet Engines"},
			{"mediaType":"movie","key":"movie:tmdb:90000101","name":"Lemon Tree Heist"}
		]}`),
		actionDiscovery(),
		testkit.FinalResponse(`{"dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"},
			{"mediaType":"movie","key":"movie:tmdb:90000100","name":"Quiet Engines"},
			{"mediaType":"movie","key":"movie:tmdb:90000101","name":"Lemon Tree Heist"}
		]}`),
		actionDiscovery(),
		testkit.FinalResponse(`{"dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"},
			{"mediaType":"movie","key":"movie:tmdb:90000100","name":"Quiet Engines"},
			{"mediaType":"movie","key":"movie:tmdb:90000101","name":"Lemon Tree Heist"}
		]}`),
		actionDiscovery(),
		testkit.FinalResponse(`{"dateMeaning":{"kind":"none","anchors":[],"axes":[]},"picks":[
			{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"},
			{"mediaType":"movie","key":"movie:tmdb:90000100","name":"Quiet Engines"},
			{"mediaType":"movie","key":"movie:tmdb:90000101","name":"Lemon Tree Heist"}
		]}`),
	)
	toolOrders := &toolOrderRecorder{}
	llmMock.OnChat = func() { toolOrders.capture(llmMock.LastMessages) }
	h := newHarness(t, withLLM(llmMock), withJobWorkers(1))
	admin := h.asAdmin()
	seedLibraryMovies(t, h.store,
		libraryMovie{ID: "lib-marigold", Name: "Marigold Protocol", Year: 1999, TMDBID: 90000603, Genres: []string{"Action", "Science Fiction"}},
		libraryMovie{ID: "lib-engines", Name: "Quiet Engines", Year: 1994, TMDBID: 90000100, Genres: []string{"Action", "Thriller"}},
		libraryMovie{ID: "lib-heist", Name: "Lemon Tree Heist", Year: 1996, TMDBID: 90000101, Genres: []string{"Action", "Thriller"}},
	)

	ctx := context.Background()
	// The channels own their request, as approval leaves them (FromRequest): there is no job
	// behind them, so the weekly run seeds from the channel alone.
	for i, channelID := range []string{"channel-a", "channel-b"} {
		if _, err := h.store.SaveChannel(ctx, channelstore.Channel{Channel: schedule.Channel{
			ID: channelID, Number: 80 + i, Name: channelID, Status: schedule.StatusBuilding,
			FromRequest: true, Strategy: schedule.Sequential,
		}, IntentJSON: `{"description":"action"}`,
			Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}}}); err != nil {
			t.Fatal(err)
		}
	}

	for _, body := range []string{
		`{"scope":"household","targetKey":"movie:tmdb:90000603","action":"surprise"}`,
		`{"scope":"channel","scopeId":"channel-a","targetKey":"movie:tmdb:90000603","action":"never"}`,
		`{"scope":"channel","scopeId":"channel-b","targetKey":"movie:tmdb:90000100","action":"never"}`,
	} {
		if code := h.status(http.MethodPost, "/v1/discovery/feedback", body, admin); code != http.StatusOK {
			t.Fatalf("record feedback %s = %d, want 200", body, code)
		}
	}
	if code := h.status(http.MethodPost, "/v1/jobs/channel-recurate/run", "", admin); code != http.StatusAccepted {
		t.Fatalf("run channel-recurate = %d, want 202", code)
	}

	channelA, attemptA := awaitRecuratedProposalAfter(t, h, "channel-a", 0)
	channelB, attemptB := awaitRecuratedProposalAfter(t, h, "channel-b", 0)
	assertProposalKeys(t, channelA, []provision.Key{"movie:tmdb:90000100", "movie:tmdb:90000101"})
	assertProposalKeys(t, channelB, []provision.Key{"movie:tmdb:90000101", "movie:tmdb:90000603"})

	clear := `{"scope":"channel","scopeId":"channel-a","targetKey":"movie:tmdb:90000603"}`
	if code := h.status(http.MethodPost, "/v1/discovery/feedback/clear", clear, admin); code != http.StatusOK {
		t.Fatalf("clear channel feedback = %d, want 200", code)
	}
	if code := h.status(http.MethodPost, "/v1/jobs/channel-recurate/run", "", admin); code != http.StatusAccepted {
		t.Fatalf("rerun channel-recurate = %d, want 202", code)
	}
	clearedA, _ := awaitRecuratedProposalAfter(t, h, "channel-a", attemptA)
	clearedB, _ := awaitRecuratedProposalAfter(t, h, "channel-b", attemptB)
	assertProposalKeys(t, clearedA, []provision.Key{"movie:tmdb:90000100", "movie:tmdb:90000101", "movie:tmdb:90000603"})
	assertProposalKeys(t, clearedB, []provision.Key{"movie:tmdb:90000101", "movie:tmdb:90000603"})
	if llmMock.Calls != 8 {
		t.Fatalf("generator calls = %d, want 8 for four durable recurate attempts", llmMock.Calls)
	}
	toolOrders.capture(llmMock.LastMessages)
	// The household surprise target is Marigold (90000603). It never boosts Marigold itself: when a
	// Channel override does not exclude it, the grounded Thriller neighbors widen the batch
	// ahead of the marked Action/Sci-Fi anchor. Channel-scoped never still wins first.
	if got, want := toolOrders.all(), [][]provision.Key{{"movie:tmdb:90000100", "movie:tmdb:90000101"}, {"movie:tmdb:90000101", "movie:tmdb:90000603"}, {"movie:tmdb:90000100", "movie:tmdb:90000101", "movie:tmdb:90000603"}, {"movie:tmdb:90000101", "movie:tmdb:90000603"}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("grounded rank orders = %v, want %v", got, want)
	}
}

type toolOrderRecorder struct {
	mu     sync.Mutex
	orders [][]provision.Key
}

func (r *toolOrderRecorder) capture(messages []llm.Message) {
	for _, message := range messages {
		if message.Role != llm.Tool {
			continue
		}
		var candidates []struct {
			Key provision.Key `json:"key"`
		}
		if json.Unmarshal([]byte(message.Content), &candidates) != nil {
			return
		}
		order := make([]provision.Key, 0, len(candidates))
		for _, candidate := range candidates {
			order = append(order, candidate.Key)
		}
		r.mu.Lock()
		r.orders = append(r.orders, order)
		r.mu.Unlock()
		return
	}
}

func (r *toolOrderRecorder) all() [][]provision.Key {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]provision.Key, len(r.orders))
	for i := range r.orders {
		out[i] = append([]provision.Key(nil), r.orders[i]...)
	}
	return out
}

// awaitRecuratedProposalAfter waits until the channel has more re-curation proposals than
// `previous` and returns the newest one with the new count. Every re-curation is its own job
// that names the channel, so the proposals are found through the jobs that produced them.
func awaitRecuratedProposalAfter(t *testing.T, h *harness, channelID string, previous int) (proposals.Proposal, int) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(workerWait)
	for time.Now().Before(deadline) {
		var found []proposalstore.Proposal
		for _, status := range []string{"submitted", "approved"} {
			proposals, err := h.store.ListProposalsByStatus(ctx, status)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range proposals {
				job, err := h.store.GetJob(ctx, p.JobID)
				if err == nil && job.Kind == "recurate" && job.ChannelID == channelID {
					found = append(found, p)
				}
			}
		}
		if len(found) > previous {
			slices.SortFunc(found, func(a, b proposalstore.Proposal) int { return a.CreatedAt.Compare(b.CreatedAt) })
			return found[len(found)-1].Body, len(found)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("recurate for %s did not complete", channelID)
	return proposals.Proposal{}, 0
}

func assertProposalKeys(t *testing.T, prop proposals.Proposal, want []provision.Key) {
	t.Helper()
	got := make([]provision.Key, 0, len(prop.Lineup)+len(prop.Acquisitions))
	items := append(append([]proposals.ProposalItem(nil), prop.Lineup...), prop.Acquisitions...)
	for _, item := range items {
		key, err := item.Key()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("proposal keys = %v, want %v", got, want)
	}
}
