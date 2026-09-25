package suggest_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// siteRecorder wraps a scripted provider and records the call site each Chat carried.
type siteRecorder struct {
	llm.Provider
	mu    sync.Mutex
	sites []string
}

func (r *siteRecorder) Chat(ctx context.Context, m []llm.Message, o llm.ChatOptions) (llm.Response, error) {
	r.mu.Lock()
	r.sites = append(r.sites, llm.CallSite(ctx))
	r.mu.Unlock()
	return r.Provider.Chat(ctx, m, o)
}

func TestSuggest_LLMCallsNameTheirCallSite(t *testing.T) {
	rec := &siteRecorder{Provider: testkit.NewLLM(finalResponseWithNone(`{"picks":[]}`), finalResponseWithNone(`{"picks":[]}`))}
	ms := testkit.NewMediaServer(t)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	tm := tmdb.NewWithBase(testkit.NewTMDB(t).URL, "key")
	sug := suggest.New(rec, catalog.New(lib, tm), tm, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = sug.Suggest(ctx, suggest.Intent{Description: "Classic Simpsons episodes"})

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.sites) == 0 {
		t.Fatal("suggester made no LLM call")
	}
	for _, site := range rec.sites {
		if site != "suggest.chat" {
			t.Errorf("call site = %q, want suggest.chat", site)
		}
	}
}
