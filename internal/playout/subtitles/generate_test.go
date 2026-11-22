package subtitles_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/subtitles"
	"github.com/loomarr/loomarr/internal/testkit"
)

const s = time.Second

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

var episode = subtitles.Media{Input: "/media/show/s01e01.mkv", Size: 1 << 30, ModifiedNs: 1_700_000_000, Duration: 22 * time.Minute}

// speaking answers every span with one line at 5–7 s into it, in language.
func speaking(language string) func(testkit.SpeechCall) (subtitles.Speech, error) {
	return func(call testkit.SpeechCall) (subtitles.Speech, error) {
		return subtitles.Speech{Language: language, Cues: []subtitles.Cue{
			{Start: 5 * s, End: 7 * s, Text: fmt.Sprintf("line at %v", call.Start+5*s)},
		}}, nil
	}
}

func newGenerator(t *testing.T, eng subtitles.Engines, dir string) (*subtitles.Generator, *clock) {
	t.Helper()
	g := subtitles.NewGenerator(eng, dir, nil)
	c := &clock{t: time.Date(2026, 11, 22, 20, 0, 0, 0, time.UTC)}
	g.SetClock(c.now)
	return g, c
}

// drain runs the worker until nothing is wanted.
func drain(t *testing.T, g *subtitles.Generator) {
	t.Helper()
	for range 100 {
		if !g.Step(context.Background()) {
			return
		}
	}
	t.Fatal("the queue never emptied")
}

func spans(calls []testkit.SpeechCall) [][2]time.Duration {
	out := [][2]time.Duration{}
	for _, c := range calls {
		out = append(out, [2]time.Duration{c.Start, c.End})
	}
	return out
}

func TestGenerator_FirstRequestLeadsLocallyAtThePositionThenTranscribesGridBlocks(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: speaking("en")}
	hosted := &testkit.SpeechEngine{Respond: speaking("en")}
	g, _ := newGenerator(t, subtitles.Engines{
		Local: local, Hosted: func() subtitles.Transcriber { return hosted }, Budget: &testkit.MediaBudget{},
	}, "")

	if _, language, state := g.Cues(episode, 95*s, 105*s); state != subtitles.Pending || language != "" {
		t.Fatalf("first answer = %v %q, want pending with no language", state, language)
	}
	drain(t, g)

	// The lead block is ~8 s at the position and local, even with a hosted service configured.
	if got, want := spans(local.Calls()), [][2]time.Duration{{95 * s, 103 * s}}; !slices.Equal(got, want) {
		t.Fatalf("local spans = %v, want %v", got, want)
	}
	// Then 30 s blocks on the file-start grid: the one holding the window, and two ahead.
	want := [][2]time.Duration{{90 * s, 120 * s}, {120 * s, 150 * s}, {150 * s, 180 * s}}
	if got := spans(hosted.Calls()); !slices.Equal(got, want) {
		t.Fatalf("hosted spans = %v, want %v", got, want)
	}
	cues, language, state := g.Cues(episode, 95*s, 105*s)
	if state != subtitles.Ready || language != "en" || len(cues) != 1 || cues[0].Start != 95*s || cues[0].Text != "line at 1m35s" {
		t.Fatalf("answer = %v %q %+v", state, language, cues)
	}
}

func TestGenerator_LeadCuesStandInWhileTheGridBlockIsMade(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: speaking("en")}
	g, _ := newGenerator(t, subtitles.Engines{Local: local, Budget: &testkit.MediaBudget{}}, "")
	g.Cues(episode, 95*s, 105*s)
	if !g.Step(context.Background()) {
		t.Fatal("no lead block ran")
	}
	cues, _, state := g.Cues(episode, 95*s, 105*s)
	if state != subtitles.Pending || len(cues) != 1 || cues[0].Start != 100*s {
		t.Fatalf("answer = %v %+v, want pending with the lead's cue at 1m40s", state, cues)
	}
}

func TestGenerator_ConcurrentRequestsTranscribeEachBlockOnce(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: speaking("en"), Hold: make(chan struct{})}
	g, _ := newGenerator(t, subtitles.Engines{Local: local, Budget: &testkit.MediaBudget{}}, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { g.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	var viewers sync.WaitGroup
	for range 2 {
		viewers.Go(func() { g.Cues(episode, 0, 10*s) })
	}
	viewers.Wait()
	for len(local.Calls()) == 0 {
		time.Sleep(time.Millisecond)
	}
	// A second viewer asks while the first block is in flight.
	g.Cues(episode, 0, 10*s)
	close(local.Hold)

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, _, state := g.Cues(episode, 0, 10*s); state == subtitles.Ready && len(local.Calls()) == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("blocks never made; calls %v", spans(local.Calls()))
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	got := spans(local.Calls())
	for i, span := range got {
		if slices.Contains(got[i+1:], span) {
			t.Fatalf("span %v transcribed twice: %v", span, got)
		}
	}
}

func TestGenerator_NoRequestForOneBlockLengthStopsTheQueue(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: speaking("en")}
	g, c := newGenerator(t, subtitles.Engines{Local: local, Budget: &testkit.MediaBudget{}}, "")
	g.Cues(episode, 0, 10*s)
	g.Step(context.Background())
	c.advance(subtitles.BlockLength + s)
	if g.Step(context.Background()) {
		t.Fatal("a block was transcribed with nobody asking")
	}
	if n := len(local.Calls()); n != 1 {
		t.Fatalf("transcriptions = %d, want only the lead", n)
	}
}

func TestGenerator_HostedRefusalOrAbsenceFallsBackToLocalForThatBlockOnly(t *testing.T) {
	busy := errors.New("audio transcription: status 429 Too Many Requests")
	hosted := &testkit.SpeechEngine{Respond: func(call testkit.SpeechCall) (subtitles.Speech, error) {
		if call.Start == 30*s {
			return subtitles.Speech{}, busy
		}
		return speaking("en")(call)
	}}
	local := &testkit.SpeechEngine{Respond: speaking("en")}
	g, _ := newGenerator(t, subtitles.Engines{
		Local: local, Hosted: func() subtitles.Transcriber { return hosted }, Budget: &testkit.MediaBudget{},
	}, "")
	g.Cues(episode, 0, 10*s)
	drain(t, g)
	if got, want := spans(hosted.Calls()), [][2]time.Duration{{0, 30 * s}, {30 * s, 60 * s}, {60 * s, 90 * s}}; !slices.Equal(got, want) {
		t.Fatalf("hosted spans = %v, want each block asked once: %v", got, want)
	}
	// The lead, and the one block the hosted service refused.
	if got, want := spans(local.Calls()), [][2]time.Duration{{0, 8 * s}, {30 * s, 60 * s}}; !slices.Equal(got, want) {
		t.Fatalf("local spans = %v, want %v", got, want)
	}
	if _, _, state := g.Cues(episode, 30*s, 40*s); state != subtitles.Ready {
		t.Fatalf("refused block state = %v, want ready from the local engine", state)
	}

	unconfigured := &testkit.SpeechEngine{Respond: speaking("en")}
	g2, _ := newGenerator(t, subtitles.Engines{
		Local: unconfigured, Hosted: func() subtitles.Transcriber { return nil }, Budget: &testkit.MediaBudget{},
	}, "")
	g2.Cues(episode, 0, 10*s)
	drain(t, g2)
	if got, want := spans(unconfigured.Calls()), [][2]time.Duration{{0, 8 * s}, {0, 30 * s}, {30 * s, 60 * s}, {60 * s, 90 * s}}; !slices.Equal(got, want) {
		t.Fatalf("local spans with no hosted service = %v, want %v", got, want)
	}
}

func TestGenerator_NoBudgetHeadroomRefusesLocalWorkAndStaysPending(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: speaking("en")}
	budget := &testkit.MediaBudget{}
	budget.SetTight(true)
	g, _ := newGenerator(t, subtitles.Engines{Local: local, Budget: budget}, "")

	g.Cues(episode, 0, 10*s)
	if !g.Step(context.Background()) {
		t.Fatal("the block was never offered to the budget")
	}
	// Refused, not queued: nothing more runs until the player asks again.
	if g.Step(context.Background()) {
		t.Fatal("refused work was retried without a request")
	}
	if _, _, state := g.Cues(episode, 0, 10*s); state != subtitles.Pending {
		t.Fatalf("state = %v, want pending", state)
	}
	if len(local.Calls()) != 0 || budget.Admitted() != 0 {
		t.Fatalf("local ran %d times past a tight budget", len(local.Calls()))
	}

	budget.SetTight(false)
	g.Cues(episode, 0, 10*s)
	drain(t, g)
	if _, _, state := g.Cues(episode, 0, 10*s); state != subtitles.Ready || len(local.Calls()) == 0 {
		t.Fatalf("state = %v after headroom returned, calls %d", state, len(local.Calls()))
	}
}

func TestGenerator_DropsCuesMostlyInsideSilenceAndKeepsQuietSpeech(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: func(testkit.SpeechCall) (subtitles.Speech, error) {
		return subtitles.Speech{Language: "en", Cues: []subtitles.Cue{
			{Start: 1 * s, End: 3 * s, Text: "Thank you."},                                        // all in silence
			{Start: 8 * s, End: 10*s + 500*time.Millisecond, Text: "Mm."},                         // 80% in silence
			{Start: 9 * s, End: 14 * s, Text: "we should go, quietly"},                            // 20% in silence
			{Start: 20 * s, End: 22 * s, Text: "where the quiet dialogue is, not silence at all"}, // none
		}}, nil
	}}
	g, _ := newGenerator(t, subtitles.Engines{
		Local: local, Budget: &testkit.MediaBudget{},
		Silence: testkit.SilenceSpans{Spans: []subtitles.Span{{Start: 0, End: 10 * s}}},
	}, "")
	g.Cues(episode, 0, 30*s)
	drain(t, g)
	cues, _, _ := g.Cues(episode, 0, 30*s)
	var texts []string
	for _, c := range cues {
		texts = append(texts, c.Text)
	}
	if want := []string{"we should go, quietly", "where the quiet dialogue is, not silence at all"}; !slices.Equal(texts, want) {
		t.Fatalf("kept %q, want %q", texts, want)
	}
}

func TestGenerator_KeepsTheLanguageOfTheFirstBlockWithSpeech(t *testing.T) {
	local := &testkit.SpeechEngine{Respond: func(call testkit.SpeechCall) (subtitles.Speech, error) {
		switch {
		case call.End == subtitles.LeadLength: // the lead: music, a confident guess and no line
			return subtitles.Speech{Language: "ja"}, nil
		case call.Start == 30*s:
			return speaking("en")(call) // a later block that hears something else
		default:
			return speaking("de")(call)
		}
	}}
	g, _ := newGenerator(t, subtitles.Engines{Local: local, Budget: &testkit.MediaBudget{}}, "")
	g.Cues(episode, 0, 10*s)
	drain(t, g)
	calls := local.Calls()
	if calls[0].Language != "" || calls[1].Language != "" {
		t.Fatalf("languages asked before speech = %q, %q; want detection", calls[0].Language, calls[1].Language)
	}
	for _, c := range calls[2:] {
		if c.Language != "de" {
			t.Fatalf("block at %v asked in %q, want the kept de", c.Start, c.Language)
		}
	}
	if _, language, _ := g.Cues(episode, 30*s, 40*s); language != "de" {
		t.Fatalf("language = %q, want de", language)
	}
}

func TestGenerator_ReadsCachedBlocksAfterARestartAndRedoesAReplacedFile(t *testing.T) {
	dir := t.TempDir()
	first := &testkit.SpeechEngine{Respond: speaking("fr")}
	g, _ := newGenerator(t, subtitles.Engines{Local: first, Budget: &testkit.MediaBudget{}}, dir)
	g.Cues(episode, 0, 10*s)
	drain(t, g)

	again := &testkit.SpeechEngine{Respond: speaking("fr")}
	restarted, _ := newGenerator(t, subtitles.Engines{Local: again, Budget: &testkit.MediaBudget{}}, dir)
	cues, language, state := restarted.Cues(episode, 0, 10*s)
	if state != subtitles.Ready || language != "fr" || len(cues) != 1 || cues[0].Text != "line at 5s" {
		t.Fatalf("after restart = %v %q %+v, want the cached block", state, language, cues)
	}
	drain(t, restarted)
	if n := len(again.Calls()); n != 0 {
		t.Fatalf("cached blocks transcribed again %d times", n)
	}

	for _, replaced := range []subtitles.Media{
		{Input: episode.Input, Size: episode.Size + 1, ModifiedNs: episode.ModifiedNs, Duration: episode.Duration},
		{Input: episode.Input, Size: episode.Size, ModifiedNs: episode.ModifiedNs + 1, Duration: episode.Duration},
	} {
		before := len(again.Calls())
		if _, _, state := restarted.Cues(replaced, 0, 10*s); state != subtitles.Pending {
			t.Fatalf("replaced file state = %v, want pending", state)
		}
		drain(t, restarted)
		if len(again.Calls()) == before {
			t.Fatal("a replaced file was not transcribed again")
		}
	}
}

func TestGenerator_FailsWithNoEngine(t *testing.T) {
	g, _ := newGenerator(t, subtitles.Engines{Budget: &testkit.MediaBudget{}}, "")
	if _, _, state := g.Cues(episode, 0, 10*s); state != subtitles.Failed {
		t.Fatalf("state = %v, want failed", state)
	}
}

func TestPick_AutoKeysPickOnlyAutoGeneratedTracks(t *testing.T) {
	english := subtitles.Track{Key: "en", Language: "en"}
	auto := subtitles.Track{Key: subtitles.AutoKey(""), Language: ""}
	tracks := []subtitles.Track{english, auto}
	if got, ok := subtitles.Pick(tracks, subtitles.AutoKey("en")); !ok || got.Key != "und-auto" {
		t.Fatalf("en-auto picked %+v, want the Auto-generated track", got)
	}
	if got, ok := subtitles.Pick([]subtitles.Track{english}, "en-auto"); ok {
		t.Fatalf("en-auto picked the file's %+v", got)
	}
	autoEnglish := subtitles.Track{Key: subtitles.AutoKey("en"), Language: "en"}
	if got, ok := subtitles.Pick([]subtitles.Track{autoEnglish}, "en-sdh"); ok {
		t.Fatalf("en-sdh picked the Auto-generated %+v", got)
	}
}
