package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Against the pinned Ollama capture: a request carrying a tool yields a response
// whose tool_calls[].function.arguments is a PARSED OBJECT (FINDINGS finding 1),
// which the provider surfaces as ToolCall.Arguments (a map).
func TestOllama_ToolCallArgumentsAreObject(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_toolcall_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llama3.1:8b")
	got, err := o.Chat(context.Background(), []llm.Message{
		{Role: llm.System, Content: "no inventing"},
		{Role: llm.User, Content: "build a channel"},
	}, llm.ChatOptions{
		Tools: []llm.ToolSchema{{
			Name: "catalog_search", Description: "search",
			Parameters: map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.WantsTools() {
		t.Fatal("expected the model to request a tool call")
	}
	tc := got.ToolCalls[0]
	if tc.Name != "catalog_search" {
		t.Errorf("tool name = %q", tc.Name)
	}
	// Arguments came through as a decoded object, not a string.
	if tc.Arguments["query"] != "90s action movie" {
		t.Errorf("tool arguments not decoded as object: %+v", tc.Arguments)
	}
	// The request carried stream:false and the tool.
	if sentReq["stream"] != false {
		t.Errorf("request stream = %v, want false", sentReq["stream"])
	}
	if _, ok := sentReq["tools"]; !ok {
		t.Error("request missing tools array")
	}
}

// Against the pinned final response: after a tool result, the model returns
// grounded JSON content and no tool calls.
func TestOllama_FinalContentAndJSONMode(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "")
	got, err := o.Chat(context.Background(), []llm.Message{
		{Role: llm.Tool, Content: `[{"tmdb_id":100,"title":"Speed"}]`, ToolCallID: "call_1"},
	}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.WantsTools() {
		t.Error("final turn should have no tool calls")
	}
	if got.Content == "" {
		t.Fatal("expected final JSON content")
	}
	// JSONMode sets format:json on the request.
	if sentReq["format"] != "json" {
		t.Errorf("JSONMode should set format=json, got %v", sentReq["format"])
	}
	// The content is valid JSON with the grounded ids.
	var parsed struct {
		Picks []struct {
			TMDBID int `json:"tmdb_id"`
		} `json:"picks"`
	}
	if err := json.Unmarshal([]byte(got.Content), &parsed); err != nil {
		t.Fatalf("final content not valid JSON: %v", err)
	}
	if len(parsed.Picks) == 0 || parsed.Picks[0].TMDBID != 100 {
		t.Errorf("final content lost the grounded id: %+v", parsed.Picks)
	}
}

// T0.1: the Ollama request carries an `options` block with num_ctx always set (so
// the growing tool loop isn't truncated) and the forwarded sampling controls.
func TestOllama_SendsOptionsBlock(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "")
	temp := 0.2
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}},
		llm.ChatOptions{Temperature: &temp, MaxTokens: 512}); err != nil {
		t.Fatal(err)
	}
	opts, ok := sentReq["options"].(map[string]any)
	if !ok {
		t.Fatalf("request has no options block: %v", sentReq["options"])
	}
	if opts["num_ctx"] == nil {
		t.Error("num_ctx must always be set (tool loop must not be truncated)")
	}
	if opts["temperature"] != 0.2 {
		t.Errorf("temperature = %v, want 0.2 forwarded", opts["temperature"])
	}
	if opts["num_predict"] != float64(512) {
		t.Errorf("num_predict = %v, want 512 (from MaxTokens)", opts["num_predict"])
	}
}

// Thinking is disabled when tools are present: a reasoning model's chain-of-thought
// breaks tool-calls/JSON on Ollama (bugs #10976/#14601). Asserts think:false is sent
// with tools, and NOT sent (field omitted) without them.
func TestOllama_DisablesThinkingWithTools(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()
	o := llm.NewOllama(srv.URL, "")

	// With tools → think:false.
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}},
		llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search"}}}); err != nil {
		t.Fatal(err)
	}
	if think, ok := sentReq["think"]; !ok || think != false {
		t.Errorf("with tools, think should be false, got %v (present=%v)", think, ok)
	}

	// Without tools → think omitted (don't touch a non-thinking model's default).
	sentReq = nil
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sentReq["think"]; ok {
		t.Error("without tools, think should be omitted")
	}
}

// KEEP-ALIVE (§8.2): the residency hint rides on every chat request when set, and is
// OMITTED when unset so Ollama's own default applies rather than us asserting one.
//
// This is the difference between a ~9s cold load and a ~0.5s answer (measured, 8B
// local model), and it matters BETWEEN the rounds of a single tool loop as well as
// between a suggestion and its refine — Ollama unloads after 5m idle by default.
func TestOllama_KeepAliveSentWhenSet(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	// Set → sent verbatim (Ollama parses the duration string itself).
	o := llm.NewOllama(srv.URL, "").WithKeepAlive("30m")
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if sentReq["keep_alive"] != "30m" {
		t.Errorf("keep_alive = %v, want %q", sentReq["keep_alive"], "30m")
	}

	// Unset → omitted entirely. An empty string must not be sent: Ollama would have to
	// interpret it, and inheriting its documented default is the honest "no opinion".
	sentReq = nil
	plain := llm.NewOllama(srv.URL, "")
	if _, err := plain.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sentReq["keep_alive"]; ok {
		t.Errorf("keep_alive should be omitted when unset, got %v", sentReq["keep_alive"])
	}
}

// WARM (§8.2) preloads the model: it POSTs to /api/chat with the model + keep_alive
// and NO messages, which is Ollama's documented preload — it loads the weights and
// returns without generating. Sending messages would burn a real generation on a
// warm-up nobody asked for.
func TestOllama_WarmPreloadsWithoutGenerating(t *testing.T) {
	var sentReq map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""}}`))
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "qwen3:8b").WithKeepAlive("30m")
	if err := o.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/api/chat" {
		t.Errorf("warm path = %q, want /api/chat", path)
	}
	if sentReq["model"] != "qwen3:8b" {
		t.Errorf("warm model = %v, want qwen3:8b", sentReq["model"])
	}
	// The keep_alive must ride along, or the preload would evaporate at Ollama's
	// 5-minute idle unload long before anyone reached the suggest screen.
	if sentReq["keep_alive"] != "30m" {
		t.Errorf("warm keep_alive = %v, want 30m", sentReq["keep_alive"])
	}
	// No messages: the field serializes as JSON null (it has no omitempty), so the
	// check is that it carries no ENTRIES — `ok` alone would be true for that null.
	if msgs, _ := sentReq["messages"].([]any); len(msgs) != 0 {
		t.Errorf("warm should send no messages (preload only), got %v", msgs)
	}
}

// A warm-up failure is reported, never swallowed — callers log it and carry on
// (§8.2: best-effort, the next Chat loads the model itself). Returning nil here
// would make a permanently-cold host look like a working warm-up.
func TestOllama_WarmReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := llm.NewOllama(srv.URL, "m").Warm(context.Background()); err == nil {
		t.Fatal("expected an error from a failing warm-up")
	}
}

// A GUESSED model is never warmed (§8.2). On a fresh install nobody has picked a model
// yet — §8.1 deliberately leaves LLM_MODEL blank so the wizard's ranked picker owns the
// choice — so warming would fire the built-in fallback tag at a host that has almost
// certainly never pulled it. That is a 404 logged as a warm-up failure, on every boot,
// for a model the operator never asked for.
//
// Observed on a real fresh boot before this guard: `llm warm-up skipped err="ollama warm:
// status 404"`. Chat is deliberately NOT gated the same way — a misconfigured install
// should fail loudly on a real request, not silently.
func TestOllama_WarmSkipsAGuessedModel(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound) // what a real host says about an unpulled tag
	}))
	defer srv.Close()

	// No model configured → the constructor substitutes its fallback tag. The sentinel
	// distinguishes "nothing to warm" from both a success and a failure, so the caller
	// can stay silent instead of claiming either.
	err := llm.NewOllama(srv.URL, "").Warm(context.Background())
	if !errors.Is(err, llm.ErrNothingToWarm) {
		t.Errorf("warming a guessed model = %v, want ErrNothingToWarm", err)
	}
	if calls != 0 {
		t.Errorf("warm made %d request(s) for a model nobody picked; want 0", calls)
	}

	// An EXPLICIT model is still warmed — the guard must not disable warm-up wholesale.
	err = llm.NewOllama(srv.URL, "qwen3:8b").Warm(context.Background())
	if err == nil || errors.Is(err, llm.ErrNothingToWarm) {
		t.Errorf("an explicitly chosen model should be warmed and report the 404, got %v", err)
	}
	if calls != 1 {
		t.Errorf("explicit model: %d warm request(s), want 1", calls)
	}
}

// EVICT (§9.1 V47) is the mirror of Warm: it POSTs to /api/chat with the model, NO messages, and
// keep_alive=0 — Ollama's documented immediate-unload. This is what playout calls to free VRAM for
// its hardware encoders when they can't fit alongside a resident model.
func TestOllama_EvictUnloadsWithKeepAliveZero(t *testing.T) {
	var sentReq map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""}}`))
	}))
	defer srv.Close()

	// keep_alive is set to 30m on the client — Evict must OVERRIDE it with 0, or the "unload" would
	// instead re-arm a long residency, the exact opposite of the intent.
	o := llm.NewOllama(srv.URL, "qwen3:8b").WithKeepAlive("30m")
	if err := o.Evict(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/api/chat" {
		t.Errorf("evict path = %q, want /api/chat", path)
	}
	if sentReq["model"] != "qwen3:8b" {
		t.Errorf("evict model = %v, want qwen3:8b", sentReq["model"])
	}
	// The whole point: keep_alive must be "0" (unload now), not the client's 30m.
	if sentReq["keep_alive"] != "0" {
		t.Errorf("evict keep_alive = %v, want \"0\" (immediate unload)", sentReq["keep_alive"])
	}
	if msgs, _ := sentReq["messages"].([]any); len(msgs) != 0 {
		t.Errorf("evict should send no messages, got %v", msgs)
	}
}

// A guessed model is never evicted — nothing was resident to unload (mirror of the warm guard).
func TestOllama_EvictSkipsAGuessedModel(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := llm.NewOllama(srv.URL, "").Evict(context.Background())
	if !errors.Is(err, llm.ErrNothingToEvict) {
		t.Errorf("evicting a guessed model = %v, want ErrNothingToEvict", err)
	}
	if calls != 0 {
		t.Errorf("evict made %d request(s) for a guessed model; want 0", calls)
	}
}

// An evict failure is reported (best-effort by contract, but not swallowed) so the caller can log
// it — the encode still falls back to software, so a failed evict costs VRAM, never correctness.
func TestOllama_EvictReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := llm.NewOllama(srv.URL, "m").Evict(context.Background()); err == nil {
		t.Fatal("expected an error from a failing evict")
	}
}
