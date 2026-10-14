package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

// capture serves a canned reply and returns the decoded request bodies it received.
func capture(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

// tool_choice none keeps the tools on the wire (so the prompt prefix is unchanged) while
// forbidding a call; with no tools there is nothing to choose, so the field is absent.
func TestOpenAI_ToolChoiceNoneKeepsToolsOnTheWire(t *testing.T) {
	srv, bodies := capture(t)
	o := llm.NewOpenAIForProvider("openrouter", srv.URL, "m", "k")
	tools := []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object"}}}
	for _, opts := range []llm.ChatOptions{
		{Tools: tools},
		{Tools: tools, ToolChoice: llm.ToolChoiceNone, JSONMode: true},
		{ToolChoice: llm.ToolChoiceNone, JSONMode: true},
	} {
		if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, opts); err != nil {
			t.Fatal(err)
		}
	}
	b := *bodies
	if b[0]["tool_choice"] != nil || b[0]["tools"] == nil {
		t.Errorf("auto turn: tools=%v tool_choice=%v", b[0]["tools"], b[0]["tool_choice"])
	}
	if b[1]["tool_choice"] != "none" || b[1]["tools"] == nil {
		t.Errorf("finalization turn: tools=%v tool_choice=%v, want tools kept and tool_choice none", b[1]["tools"], b[1]["tool_choice"])
	}
	if b[0]["tools"] == nil || jsonOf(b[0]["tools"]) != jsonOf(b[1]["tools"]) {
		t.Error("tools bytes differ between auto and finalization turns")
	}
	if b[2]["tool_choice"] != nil {
		t.Errorf("tool_choice sent with no tools: %v", b[2]["tool_choice"])
	}
}

func jsonOf(v any) string { raw, _ := json.Marshal(v); return string(raw) }

// A self-hosted OpenAI-compatible server (llama.cpp, vLLM) gets thinking-off stated through
// the chat template on structured and tool turns; a hosted brand rejects the unknown field.
func TestOpenAI_ThinkingOffIsExplicitOnlyForSelfHosted(t *testing.T) {
	srv, bodies := capture(t)
	for _, provider := range []string{llm.CustomProviderKey, "openrouter"} {
		o := llm.NewOpenAIForProvider(provider, srv.URL, "m", "k")
		if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{JSONMode: true}); err != nil {
			t.Fatal(err)
		}
	}
	b := *bodies
	kwargs, _ := b[0]["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false {
		t.Errorf("custom endpoint: chat_template_kwargs=%v, want enable_thinking=false", b[0]["chat_template_kwargs"])
	}
	if b[1]["chat_template_kwargs"] != nil {
		t.Errorf("hosted brand received chat_template_kwargs: %v", b[1]["chat_template_kwargs"])
	}
}

// Vision answers are one bounded JSON object: without limits the server default (temperature 1.0,
// unlimited output) let a runaway answer hold the single slot until the request timeout.
func TestOpenAI_AskAboutImagesIsBoundedAndJSON(t *testing.T) {
	srv, bodies := capture(t)
	o := llm.NewOpenAI(srv.URL, "m", "k")
	if _, err := o.AskAboutImages(context.Background(), "tag", [][]byte{{0xFF, 0xD8}}); err != nil {
		t.Fatal(err)
	}
	sent := (*bodies)[0]
	rf, _ := sent["response_format"].(map[string]any)
	if sent["max_tokens"] != float64(llm.VisionMaxTokens) || sent["temperature"] != llm.StructuredTemperature || rf["type"] != "json_object" {
		t.Errorf("vision request unbounded: max_tokens=%v temperature=%v response_format=%v", sent["max_tokens"], sent["temperature"], sent["response_format"])
	}
}

func TestStructuredChatOptions(t *testing.T) {
	opts := llm.StructuredChatOptions(512)
	if !opts.JSONMode || opts.MaxTokens != 512 || opts.Temperature == nil || *opts.Temperature != llm.StructuredTemperature {
		t.Errorf("structured options = %+v", opts)
	}
}

// Ollama's native API has no tool_choice: tool_choice none is honoured by omitting the tools.
func TestOllama_ToolChoiceNoneOmitsTools(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sent)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{}"}}`))
	}))
	defer srv.Close()
	tools := []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object"}}}
	o := llm.NewOllama(srv.URL, "m")
	for _, tc := range []struct {
		choice    string
		wantTools bool
	}{{"", true}, {llm.ToolChoiceNone, false}} {
		sent = nil
		if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Tools: tools, ToolChoice: tc.choice}); err != nil {
			t.Fatal(err)
		}
		if (sent["tools"] != nil) != tc.wantTools {
			t.Errorf("tool_choice %q: tools sent = %v, want %v", tc.choice, sent["tools"] != nil, tc.wantTools)
		}
	}
}
