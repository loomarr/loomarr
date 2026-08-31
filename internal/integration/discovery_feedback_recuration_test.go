package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestDiscoveryFeedback_DurableRecurationKeepsChannelScope(t *testing.T) {
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "action"}),
		testkit.FinalResponse(`{"picks":[
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix"},
			{"mediaType":"movie","tmdbId":100,"name":"Speed"},
			{"mediaType":"movie","tmdbId":101,"name":"The Rock"}
		]}`),
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "action"}),
		testkit.FinalResponse(`{"picks":[
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix"},
			{"mediaType":"movie","tmdbId":100,"name":"Speed"},
			{"mediaType":"movie","tmdbId":101,"name":"The Rock"}
		]}`),
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "action"}),
		testkit.FinalResponse(`{"picks":[
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix"},
			{"mediaType":"movie","tmdbId":100,"name":"Speed"},
			{"mediaType":"movie","tmdbId":101,"name":"The Rock"}
		]}`),
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "action"}),
		testkit.FinalResponse(`{"picks":[
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix"},
			{"mediaType":"movie","tmdbId":100,"name":"Speed"},
			{"mediaType":"movie","tmdbId":101,"name":"The Rock"}
		]}`),
	)
	toolOrders := &toolOrderRecorder{}
	llmMock.OnChat = func() { toolOrders.capture(llmMock.LastMessages) }
	h := newHarness(t, withLLM(llmMock), withJobWorkers(1))
	admin := h.asAdmin()
	h.ms.SetSearchItems(
		testkit.SearchStub{Terms: []string{"action"}, LibraryItemID: "lib-matrix", Name: "The Matrix", Type: "Movie", Year: 1999, TMDBID: 603, Genres: []string{"Action", "Science Fiction"}},
		testkit.SearchStub{Terms: []string{"action"}, LibraryItemID: "lib-speed", Name: "Speed", Type: "Movie", Year: 1994, TMDBID: 100, Genres: []string{"Action", "Thriller"}},
		testkit.SearchStub{Terms: []string{"action"}, LibraryItemID: "lib-rock", Name: "The Rock", Type: "Movie", Year: 1996, TMDBID: 101, Genres: []string{"Action", "Thriller"}},
	)

	ctx := context.Background()
	now := time.Now().UTC()
	for i, id := range []string{"job-channel-a", "job-channel-b"} {
		if err := h.store.CreateJob(ctx, store.Job{
			ID: id, Kind: "suggest", Status: "done", CreatedBy: "owner",
			IntentJSON: `{"description":"action"}`, IntentHash: id,
			WorkflowVersion: proposalworkflow.WorkflowVersion1, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		channelID := "channel-a"
		if i == 1 {
			channelID = "channel-b"
		}
		if _, err := h.store.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
			ID: channelID, Number: 80 + i, Name: channelID, Status: schedule.StatusBuilding,
			IntentRef: id, Strategy: schedule.Sequential,
		}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}}}); err != nil {
			t.Fatal(err)
		}
	}

	for _, body := range []string{
		`{"scope":"household","targetKey":"movie:tmdb:603","action":"surprise"}`,
		`{"scope":"channel","scopeId":"channel-a","targetKey":"movie:tmdb:603","action":"never"}`,
		`{"scope":"channel","scopeId":"channel-b","targetKey":"movie:tmdb:100","action":"never"}`,
	} {
		if code := h.status(http.MethodPost, "/v1/discovery/feedback", body, admin); code != http.StatusOK {
			t.Fatalf("record feedback %s = %d, want 200", body, code)
		}
	}
	if code := h.status(http.MethodPost, "/v1/jobs/channel-recurate/run", "", admin); code != http.StatusAccepted {
		t.Fatalf("run channel-recurate = %d, want 202", code)
	}

	channelA, attemptA := awaitRecuratedProposalAfter(t, h, "job-channel-a", 0)
	channelB, attemptB := awaitRecuratedProposalAfter(t, h, "job-channel-b", 0)
	assertProposalKeys(t, channelA, []provision.Key{"movie:tmdb:100", "movie:tmdb:101"})
	assertProposalKeys(t, channelB, []provision.Key{"movie:tmdb:101", "movie:tmdb:603"})

	clear := `{"scope":"channel","scopeId":"channel-a","targetKey":"movie:tmdb:603"}`
	if code := h.status(http.MethodPost, "/v1/discovery/feedback/clear", clear, admin); code != http.StatusOK {
		t.Fatalf("clear channel feedback = %d, want 200", code)
	}
	if code := h.status(http.MethodPost, "/v1/jobs/channel-recurate/run", "", admin); code != http.StatusAccepted {
		t.Fatalf("rerun channel-recurate = %d, want 202", code)
	}
	clearedA, _ := awaitRecuratedProposalAfter(t, h, "job-channel-a", attemptA)
	clearedB, _ := awaitRecuratedProposalAfter(t, h, "job-channel-b", attemptB)
	assertProposalKeys(t, clearedA, []provision.Key{"movie:tmdb:100", "movie:tmdb:101", "movie:tmdb:603"})
	assertProposalKeys(t, clearedB, []provision.Key{"movie:tmdb:101", "movie:tmdb:603"})
	if llmMock.Calls != 8 {
		t.Fatalf("generator calls = %d, want 8 for four durable recurate attempts", llmMock.Calls)
	}
	toolOrders.capture(llmMock.LastMessages)
	if got, want := toolOrders.all(), [][]int{{100, 101}, {603, 101}, {603, 100, 101}, {603, 101}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("grounded rank orders = %v, want %v", got, want)
	}
}

type toolOrderRecorder struct {
	mu     sync.Mutex
	orders [][]int
}

func (r *toolOrderRecorder) capture(messages []llm.Message) {
	for _, message := range messages {
		if message.Role != llm.Tool {
			continue
		}
		var candidates []struct {
			TMDBID int `json:"tmdbId"`
		}
		if json.Unmarshal([]byte(message.Content), &candidates) != nil {
			return
		}
		order := make([]int, 0, len(candidates))
		for _, candidate := range candidates {
			order = append(order, candidate.TMDBID)
		}
		r.mu.Lock()
		r.orders = append(r.orders, order)
		r.mu.Unlock()
		return
	}
}

func (r *toolOrderRecorder) all() [][]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]int, len(r.orders))
	for i := range r.orders {
		out[i] = append([]int(nil), r.orders[i]...)
	}
	return out
}

func awaitRecuratedProposalAfter(t *testing.T, h *harness, jobID string, previousAttempt int) (suggest.Proposal, int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := h.store.GetProposalJob(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Job.Status == "failed" {
			t.Fatalf("recurate %s failed: %s", jobID, snapshot.Job.LastError)
		}
		if snapshot.Job.Status == "done" && snapshot.Job.Attempts > previousAttempt && snapshot.Proposal != nil {
			var proposal suggest.Proposal
			if err := json.Unmarshal([]byte(snapshot.Proposal.ProposalJSON), &proposal); err != nil {
				t.Fatal(err)
			}
			return proposal, snapshot.Job.Attempts
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("recurate %s did not complete", jobID)
	return suggest.Proposal{}, 0
}

func assertProposalKeys(t *testing.T, proposal suggest.Proposal, want []provision.Key) {
	t.Helper()
	got := make([]provision.Key, 0, len(proposal.Lineup)+len(proposal.Acquisitions))
	items := append(append([]suggest.ProposalItem(nil), proposal.Lineup...), proposal.Acquisitions...)
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
