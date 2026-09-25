package suggest_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

// streamingLLM plays a scripted conversation like a real streaming provider: it hands the
// final turn's content to OnContentDelta in fragments BEFORE returning, and records the
// tracker's picks after each fragment so a test can see WHEN a pick became visible.
type streamingLLM struct {
	turns    []llm.Response
	fragment []string // deltas for the final turn; nil = a provider that never streams
	failAt   int      // return an error after delivering this many fragments (0 = never)
	seen     func()   // called after each delivered fragment
	calls    int
}

func (m *streamingLLM) Name() string { return "streaming" }

func (m *streamingLLM) Chat(_ context.Context, _ []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	turn := m.turns[m.calls]
	m.calls++
	if m.calls < len(m.turns) {
		return turn, nil // a tool-call turn: no content to stream
	}
	for i, f := range m.fragment {
		if opts.OnContentDelta != nil {
			opts.OnContentDelta(f)
		}
		if m.seen != nil {
			m.seen()
		}
		if m.failAt > 0 && i+1 == m.failAt {
			return llm.Response{}, errors.New("stream dropped mid-reply")
		}
	}
	return turn, nil
}

const speedFinal = `{"rationale":"90s action","picks":[` +
	`{"mediaType":"movie","key":"movie:tmdb:100","name":"Speed"},` +
	`{"mediaType":"movie","key":"movie:tmdb:77777","name":"Totally Made Up Film"}` +
	`]}`

// split cuts s into fragments that land inside the second pick's key, the way tokens do.
func split(s string, at ...int) []string {
	var out []string
	prev := 0
	for _, i := range at {
		out = append(out, s[prev:i])
		prev = i
	}
	return append(out, s[prev:])
}

func runTracked(t *testing.T, m *streamingLLM) (*suggest.ProgressTracker, []suggest.ProgressSnapshot, suggest.Proposal, error) {
	t.Helper()
	var history []suggest.ProgressSnapshot
	var tracker *suggest.ProgressTracker
	tracker = suggest.NewProgressTracker(time.Now(), func() { history = append(history, tracker.Snapshot()) })
	m.seen = func() {}
	ctx := suggest.WithProgressTracker(context.Background(), tracker)
	prop, err := buildSuggester(t, m).Suggest(ctx, suggest.Intent{Description: "90s action"})
	return tracker, history, prop, err
}

// The event sequence of a normal run: reading → searching (with the real query) → choosing,
// with each resolved pick appearing DURING the streamed turn, an unresolvable pick never
// appearing, and everything shown being something the proposal actually kept.
func TestProgress_StreamsStageTermsAndResolvedPicks(t *testing.T) {
	cut := strings.Index(speedFinal, `"key":"movie:tmdb:77777"`) + 12 // inside the second pick's key
	m := &streamingLLM{
		turns:    []llm.Response{catalogSearchResponse(map[string]any{"query": "speed"}), finalResponseWithNone(speedFinal)},
		fragment: split(speedFinal, 30, cut),
	}
	var sawDuringStream []int
	var tracker *suggest.ProgressTracker
	m.seen = func() { sawDuringStream = append(sawDuringStream, len(tracker.Snapshot().Picks)) }
	var history []suggest.ProgressSnapshot
	tracker = suggest.NewProgressTracker(time.Now(), func() { history = append(history, tracker.Snapshot()) })
	ctx := suggest.WithProgressTracker(context.Background(), tracker)
	// Progress phases come from the same ctx the worker sets; a bare tracker still tracks them.
	prop, err := buildSuggester(t, m).Suggest(suggest.WithProgress(ctx, func(suggest.Phase, int) {}), suggest.Intent{Description: "90s action"})
	if err != nil {
		t.Fatal(err)
	}

	var stages []suggest.Stage
	for _, h := range history {
		if len(stages) == 0 || stages[len(stages)-1] != h.Stage {
			stages = append(stages, h.Stage)
		}
	}
	if want := []suggest.Stage{suggest.StageSearching, suggest.StageChoosing, suggest.StageBuilding}; !slices.Equal(stages, want) {
		t.Errorf("stages = %v, want %v", stages, want)
	}
	final := tracker.Snapshot()
	if !slices.Equal(final.Terms, []string{"speed"}) {
		t.Errorf("terms = %v, want the catalog_search query", final.Terms)
	}
	if final.Target != suggest.LineupPickCap {
		t.Errorf("target = %d, want the prompt's cap %d", final.Target, suggest.LineupPickCap)
	}
	if len(final.Picks) != 1 || final.Picks[0].Key != "movie:tmdb:100" || final.Picks[0].Name != "Speed" {
		t.Fatalf("picks = %+v, want only Speed (the fabricated id must never appear)", final.Picks)
	}
	// Speed is visible before the stream finishes: after the fragment that closed its object
	// (fragment 2) and while the fabricated pick's key was still being written.
	if len(sawDuringStream) < 3 || sawDuringStream[0] != 0 || sawDuringStream[1] != 1 {
		t.Errorf("picks visible after each fragment = %v, want 0 then 1 (streamed, not batched at the end)", sawDuringStream)
	}
	// Nothing shown is dropped by the final proposal.
	kept := map[string]bool{}
	for _, it := range append(append(append([]suggest.ProposalItem{}, prop.Lineup...), prop.Acquisitions...), prop.Alternates...) {
		if key, err := it.Key(); err == nil {
			kept[string(key)] = true
		}
	}
	for _, p := range final.Picks {
		if !kept[p.Key] {
			t.Errorf("shown pick %s was dropped by the final proposal", p.Key)
		}
	}
}

// A provider that never streams (Ollama, a plain-JSON reply) never calls the hook: the run
// is unchanged and picks are simply absent from the snapshot until the turn's proposal lands.
func TestProgress_NonStreamingProviderShowsNoPicksButStillRuns(t *testing.T) {
	m := &streamingLLM{
		turns: []llm.Response{catalogSearchResponse(map[string]any{"query": "speed"}), finalResponseWithNone(speedFinal)},
	}
	tracker, _, prop, err := runTracked(t, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.Snapshot().Picks) != 0 {
		t.Errorf("picks = %+v, want none without a stream", tracker.Snapshot().Picks)
	}
	if len(prop.Acquisitions)+len(prop.Lineup) == 0 {
		t.Error("the proposal must still be built from the completed turn")
	}
}

// Failure mid-stream: the run fails cleanly and whatever resolved before the drop stays a
// grounded pick (never a half-parsed one).
func TestProgress_FailureMidStreamKeepsOnlyResolvedPicks(t *testing.T) {
	cut := strings.Index(speedFinal, `"key":"movie:tmdb:77777"`) + 12
	m := &streamingLLM{
		turns:    []llm.Response{catalogSearchResponse(map[string]any{"query": "speed"}), finalResponseWithNone(speedFinal)},
		fragment: split(speedFinal, 30, cut),
		failAt:   3,
	}
	tracker, _, _, err := runTracked(t, m)
	if err == nil {
		t.Fatal("a dropped stream must fail the run")
	}
	picks := tracker.Snapshot().Picks
	if len(picks) != 1 || picks[0].Key != "movie:tmdb:100" {
		t.Errorf("picks = %+v, want just the fully-streamed, resolved Speed", picks)
	}
}

// gatedLLM streams the final turn's first fragment, then parks until released, so a test can
// read the service's live snapshot while the run is genuinely mid-stream.
type gatedLLM struct {
	streamingLLM
	midStream chan struct{}
	release   chan struct{}
}

func (m *gatedLLM) Chat(ctx context.Context, msgs []llm.Message, opts llm.ChatOptions) (llm.Response, error) {
	if m.calls < len(m.turns)-1 {
		return m.streamingLLM.Chat(ctx, msgs, opts)
	}
	m.calls++
	opts.OnContentDelta(m.fragment[0])
	close(m.midStream)
	select {
	case <-m.release:
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
	opts.OnContentDelta(m.fragment[1])
	return m.turns[len(m.turns)-1], nil
}

// The API reads Service.Progress while a job runs: mid-stream it shows the resolved pick, and
// once the run ends the tracker is gone (the Proposal is the record).
func TestWorker_ProgressIsReadableMidStreamAndDroppedAfter(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	model := &gatedLLM{
		streamingLLM: streamingLLM{
			turns:    []llm.Response{catalogSearchResponse(map[string]any{"query": "speed"}), finalResponseWithNone(speedFinal)},
			fragment: split(speedFinal, strings.Index(speedFinal, `,{"mediaType"`)),
		},
		midStream: make(chan struct{}), release: make(chan struct{}),
	}
	svc := buildService(t, st, model)
	terminal := newDoneEmitter()
	svc.WithProgressEmitter(terminal)
	jobID, err := svc.Submit(ctx, suggest.Intent{Description: "90s action"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)

	select {
	case <-model.midStream:
	case <-time.After(10 * time.Second):
		t.Fatal("run never reached the streamed turn")
	}
	snap, ok := svc.Progress(jobID)
	if !ok {
		t.Fatal("a running job must have a live snapshot")
	}
	if snap.Stage != suggest.StageChoosing || len(snap.Picks) != 1 || snap.Picks[0].Name != "Speed" {
		t.Fatalf("mid-stream snapshot = %+v, want choosing with Speed already picked", snap)
	}
	close(model.release)
	select {
	case <-terminal.done:
	case <-time.After(10 * time.Second):
		t.Fatal("run never finished")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := svc.Progress(jobID); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tracker outlived the run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The stage must follow what HAPPENED, not what the search arguments happened to spell out: a
// discovery call filtered by keywords (or a collection call by titles) has no free-text query
// and no genres, and the live Add-a-channel run sat on "Reading your request…" for its whole
// second turn because the stage was derived from a non-empty term list.
func TestProgress_StageAdvancesAfterASearchWithoutAQueryOrGenres(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"keywords": {"keywords": []any{"heist"}, "media_type": "movie"},
		"decade":   {"media_type": "movie", "vote_count_min": 100},
	} {
		t.Run(name, func(t *testing.T) {
			m := &streamingLLM{
				turns:    []llm.Response{catalogSearchResponse(args), finalResponseWithNone(speedFinal)},
				fragment: split(speedFinal, 30),
			}
			var tracker *suggest.ProgressTracker
			var stages []suggest.Stage
			tracker = suggest.NewProgressTracker(time.Now(), func() {
				if s := tracker.Snapshot().Stage; len(stages) == 0 || stages[len(stages)-1] != s {
					stages = append(stages, s)
				}
			})
			ctx := suggest.WithProgressTracker(context.Background(), tracker)
			if _, err := buildSuggester(t, m).Suggest(suggest.WithProgress(ctx, func(suggest.Phase, int) {}), suggest.Intent{Description: "90s action"}); err != nil {
				t.Fatal(err)
			}
			want := []suggest.Stage{suggest.StageSearching, suggest.StageChoosing, suggest.StageBuilding}
			if !slices.Equal(stages, want) {
				t.Errorf("stages = %v, want %v", stages, want)
			}
		})
	}
}

// frameProbe records, for every SSE frame the service emits, what a client refetching the job
// on that frame would read at that instant.
type frameProbe struct {
	mu     sync.Mutex
	svc    *suggest.Service
	frames []suggest.ProgressSnapshot
}

func (p *frameProbe) SuggestionPhase(jobID, _ string, _ int) {
	snap, _ := p.svc.Progress(jobID)
	p.mu.Lock()
	p.frames = append(p.frames, snap)
	p.mu.Unlock()
}

func (p *frameProbe) last() suggest.ProgressSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.frames) == 0 {
		return suggest.ProgressSnapshot{}
	}
	return p.frames[len(p.frames)-1]
}

// A resolved pick and every stage change must reach the client as an SSE frame while the model
// is still streaming — the frame is what makes the client refetch instead of waiting for its poll.
func TestWorker_EveryProgressChangeIsAnnouncedWhileStreaming(t *testing.T) {
	ctx := context.Background()
	model := &gatedLLM{
		streamingLLM: streamingLLM{
			turns:    []llm.Response{catalogSearchResponse(map[string]any{"query": "speed"}), finalResponseWithNone(speedFinal)},
			fragment: split(speedFinal, strings.Index(speedFinal, `,{"mediaType"`)),
		},
		midStream: make(chan struct{}), release: make(chan struct{}),
	}
	svc := buildService(t, newStore(t), model)
	probe := &frameProbe{svc: svc}
	svc.WithProgressEmitter(probe)
	if _, err := svc.Submit(ctx, suggest.Intent{Description: "90s action"}, "alice"); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	select {
	case <-model.midStream:
	case <-time.After(10 * time.Second):
		t.Fatal("run never reached the streamed turn")
	}
	defer close(model.release)
	got := probe.last()
	if got.Stage != suggest.StageChoosing || len(got.Picks) != 1 {
		t.Errorf("latest frame's snapshot = %+v, want choosing with Speed: the last change before the stream parked was not announced", got)
	}
}
