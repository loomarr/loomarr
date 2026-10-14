package suggest_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// callRecord is one Chat call as the AI server saw it.
type callRecord struct {
	duration time.Duration
	finish   string
	tokens   llm.TokenUsage
	tools    int
	body     string
	choice   string
}

// recordingProvider wraps a real provider and keeps per-call timing and token usage.
type recordingProvider struct {
	llm.Provider
	mu    sync.Mutex
	calls []callRecord
}

func (r *recordingProvider) Chat(ctx context.Context, m []llm.Message, o llm.ChatOptions) (llm.Response, error) {
	start := time.Now()
	resp, err := r.Provider.Chat(ctx, m, o)
	r.mu.Lock()
	body := resp.Content
	if len(resp.ToolCalls) > 0 {
		body = fmt.Sprintf("%s %v", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	}
	r.calls = append(r.calls, callRecord{duration: time.Since(start), finish: resp.FinishReason, tokens: resp.Attribution.Tokens, tools: len(o.Tools), choice: o.ToolChoice, body: body})
	r.mu.Unlock()
	return resp, err
}

func (r *recordingProvider) CachesPromptPrefix() bool { return llm.CachesPromptPrefix(r.Provider) }

func liveCorpus() *catalogfixture.Corpus {
	titles := []struct {
		name string
		year int
	}{
		{"Die Hard", 1988}, {"Terminator 2: Judgment Day", 1991}, {"Speed", 1994}, {"Lethal Weapon", 1987},
		{"Predator", 1987}, {"Heat", 1995}, {"The Rock", 1996}, {"Face/Off", 1997}, {"True Lies", 1994},
		{"Under Siege", 1992}, {"Point Break", 1991}, {"Con Air", 1997}, {"Broken Arrow", 1996},
		{"Bad Boys", 1995}, {"Cliffhanger", 1993}, {"Demolition Man", 1993}, {"Total Recall", 1990},
		{"Rush Hour", 1998}, {"The Fugitive", 1993}, {"Léon: The Professional", 1994}, {"Blade", 1998},
		{"The Matrix", 1999}, {"Mission: Impossible", 1996}, {"Air Force One", 1997},
	}
	cands := make([]catalog.Candidate, 0, len(titles))
	for i, t := range titles {
		cands = append(cands, catalog.Candidate{
			MediaType: "movie", TMDBID: 1000 + i, Name: t.name, Year: t.year, InLibrary: i%3 != 0,
			Genres:   []string{"Action", "Thriller"},
			Overview: "A hardened protagonist is forced into a race against time when a well-organised group of criminals takes control, and must rely on wit, improvisation and a handful of unlikely allies to survive the night.",
		})
	}
	return &catalogfixture.Corpus{Candidates: cands}
}

// TestLive_SuggestLatency drives the REAL Suggester (real prompt builder, tools, chat loop and
// client) against a live OpenAI-compatible server and prints one row per chat call. It is a dev
// tool for #1487, not a gate: it skips unless LOOMARR_LIVE_LLM_URL is set.
//
//	LOOMARR_LIVE_LLM_URL=http://host:8080/v1 LOOMARR_LIVE_LLM_KEY=... LOOMARR_LIVE_LLM_MODEL=flash-next \
//	LOOMARR_LIVE_RUNS=3 go test ./internal/suggest -run TestLive_SuggestLatency -count=1 -v
func TestLive_SuggestLatency(t *testing.T) {
	url := os.Getenv("LOOMARR_LIVE_LLM_URL")
	if url == "" {
		t.Skip("LOOMARR_LIVE_LLM_URL not set")
	}
	runs, _ := strconv.Atoi(os.Getenv("LOOMARR_LIVE_RUNS"))
	if runs < 1 {
		runs = 1
	}
	descriptions := []string{
		"Explosive 90s action movies with wisecracking heroes",
		"Loud 90s blockbusters where one cop takes on a building full of terrorists",
		"Tough-guy 90s action movies with big set pieces and rogue agents",
		"Buddy-cop action movies from the 1990s",
		"High-speed 90s thrillers with vehicles, heists and last-minute escapes",
		"Action movies from the 1990s starring a famous muscle-bound hero",
	}
	for i := 0; i < runs; i++ {
		rec := &recordingProvider{Provider: llm.NewOpenAI(url, os.Getenv("LOOMARR_LIVE_LLM_MODEL"), os.Getenv("LOOMARR_LIVE_LLM_KEY"))}
		s := suggest.New(rec, catalog.New(nil, liveCorpus()), referenceExistsValidator{}, 10)
		desc := descriptions[i%len(descriptions)]
		start := time.Now()
		prop, err := s.Suggest(context.Background(), suggest.Intent{Description: desc})
		total := time.Since(start)
		var rows []string
		for n, c := range rec.calls {
			rows = append(rows, fmt.Sprintf("  call%d %6.1fs finish=%-10s prompt=%d cached=%d out=%d tools=%d choice=%q",
				n+1, c.duration.Seconds(), c.finish, c.tokens.Prompt, c.tokens.Cached, c.tokens.Completion, c.tools, c.choice))
			if os.Getenv("LOOMARR_LIVE_DUMP") != "" {
				rows = append(rows, "    "+c.body)
			}
		}
		picks := 0
		if err == nil {
			picks = len(prop.Lineup) + len(prop.Acquisitions)
		}
		t.Logf("run %d %q total=%.1fs err=%v picks=%d\n%s", i+1, desc, total.Seconds(), err, picks, strings.Join(rows, "\n"))
	}
}
