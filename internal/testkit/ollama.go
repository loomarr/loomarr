package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Ollama is a minimal, scriptable Ollama HTTP double (CLAUDE.md: one mock per
// service). It serves exactly the endpoints the llm.Prober calls for the §8.1
// model picker — GET /api/version, GET /api/tags, POST /api/pull (NDJSON stream)
// — so the picker probe/select/pull flow can be driven end to end through the
// real systemLLMService. It intentionally does NOT implement /api/chat: the
// suggester runs on the injected in-process testkit.LLM; only the picker probes
// this host.
type Ollama struct {
	*httptest.Server
	mu         sync.Mutex
	version    string
	pulled     []string         // tags reported present by /api/tags
	pullFrames []map[string]any // finite NDJSON stream /api/pull writes
	pullHits   int
}

// NewOllama starts the double. Defaults: version "0.13.5", no models pulled, and a
// short FINITE pull stream ending in {"status":"success"} (finite matters — the
// pull job outlives its request, so an endless stream would leak a goroutine).
func NewOllama(t testing.TB) *Ollama {
	t.Helper()
	o := &Ollama{
		version: "0.13.5",
		pullFrames: []map[string]any{
			{"status": "pulling manifest"},
			{"status": "downloading", "completed": 500_000_000, "total": 1_000_000_000},
			{"status": "success"},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		o.mu.Lock()
		v := o.version
		o.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"version": v})
	})
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, _ *http.Request) {
		o.mu.Lock()
		pulled := append([]string(nil), o.pulled...)
		o.mu.Unlock()
		models := make([]map[string]any, len(pulled))
		for i, name := range pulled {
			models[i] = map[string]any{"name": name}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
	})
	mux.HandleFunc("POST /api/pull", func(w http.ResponseWriter, _ *http.Request) {
		o.mu.Lock()
		o.pullHits++
		frames := o.pullFrames
		o.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		fl, _ := w.(http.Flusher)
		enc := json.NewEncoder(w) // Encode writes the trailing newline (NDJSON)
		for _, f := range frames {
			if err := enc.Encode(f); err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	})
	o.Server = httptest.NewServer(mux)
	t.Cleanup(o.Close)
	return o
}

// SetPulled sets the model tags /api/tags reports as present (drives the picker's
// "pulled" flags and the select-must-be-pulled gate).
func (o *Ollama) SetPulled(tags ...string) {
	o.mu.Lock()
	o.pulled = append([]string(nil), tags...)
	o.mu.Unlock()
}

// SetVersion overrides the reported Ollama version.
func (o *Ollama) SetVersion(v string) {
	o.mu.Lock()
	o.version = v
	o.mu.Unlock()
}

// SetPullError makes /api/pull stream a single error frame (the failure path).
func (o *Ollama) SetPullError(msg string) {
	o.mu.Lock()
	o.pullFrames = []map[string]any{{"status": "pulling", "error": msg}}
	o.mu.Unlock()
}

// PullHits reports how many times /api/pull was called (assertion helper).
func (o *Ollama) PullHits() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.pullHits
}
