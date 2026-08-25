package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

// Hosted vision (§10 V44): AskAboutImages builds a multimodal user message whose content is an
// ARRAY of typed parts — the text prompt, then one image_url part per keyframe with a FULL
// data:image/jpeg;base64,… URI. This is the audio.go precedent applied to frames, and getting
// the data-URI prefix wrong is a 400 that names neither field, so the test pins it exactly.
func TestOpenAI_AskAboutImages_BuildsDataURIParts(t *testing.T) {
	var sentReq map[string]any
	var authHdr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write([]byte(`{"model":"gemini-vision","choices":[{"message":{"role":"assistant","content":"{\"brand\":\"Kellogg's\"}"}}],"usage":{"prompt_tokens":20,"completion_tokens":3,"prompt_tokens_details":{"image_tokens":11}}}`))
	}))
	defer srv.Close()

	o := llm.NewOpenAI(srv.URL, "gemini-vision", "sk-test")
	// Two distinct frames, so the test proves each becomes its own part AND that the two are
	// not collapsed to one — a fixture that reused one byte slice could hide an off-by-one.
	frameA := []byte{0xFF, 0xD8, 0x01} // pretend-JPEG bytes; base64 → "/9gB"
	frameB := []byte{0xFF, 0xD8, 0x02} // → "/9gC"
	resp, err := o.AskAboutImages(context.Background(), "what brand is this?", [][]byte{frameA, frameB})
	if err != nil {
		t.Fatal(err)
	}

	// The answer round-trips as Response.Content (vision never uses ToolCalls).
	if resp.WantsTools() {
		t.Error("vision response should carry no tool calls")
	}
	if resp.Content != `{"brand":"Kellogg's"}` {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Attribution.Tokens.Image != 11 || resp.Attribution.Tokens.Prompt != 20 || len(resp.Attribution.Modalities) != 2 {
		t.Errorf("attribution = %+v", resp.Attribution)
	}

	// The request carried the model and a single user message with an array content.
	if sentReq["model"] != "gemini-vision" {
		t.Errorf("model = %v, want gemini-vision", sentReq["model"])
	}
	msgs, _ := sentReq["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	parts, _ := msgs[0].(map[string]any)["content"].([]any)
	// text part + one image_url part per frame.
	if len(parts) != 3 {
		t.Fatalf("want 3 content parts (1 text + 2 images), got %d: %+v", len(parts), parts)
	}

	// Part 0 is the text prompt.
	p0, _ := parts[0].(map[string]any)
	if p0["type"] != "text" || p0["text"] != "what brand is this?" {
		t.Errorf("part 0 = %+v, want text prompt", p0)
	}

	// Parts 1 and 2 are image_url parts whose url is a full jpeg data URI matching each frame.
	wantURLs := []string{
		"data:image/jpeg;base64,/9gB",
		"data:image/jpeg;base64,/9gC",
	}
	for i, want := range wantURLs {
		p, _ := parts[i+1].(map[string]any)
		if p["type"] != "image_url" {
			t.Errorf("part %d type = %v, want image_url", i+1, p["type"])
		}
		iu, _ := p["image_url"].(map[string]any)
		if got, _ := iu["url"].(string); got != want {
			t.Errorf("part %d url = %q, want %q", i+1, got, want)
		}
	}

	// Bearer auth rides along, same as the text path.
	if authHdr != "Bearer sk-test" {
		t.Errorf("auth header = %q, want Bearer sk-test", authHdr)
	}
}

// An empty frame set is a caller bug, not a request to send: it must error before any HTTP,
// so a mis-sampled clip does not spend a multimodal token on a picture-free question.
func TestOpenAI_AskAboutImages_RejectsNoImages(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	o := llm.NewOpenAI(srv.URL, "m", "")
	if _, err := o.AskAboutImages(context.Background(), "?", nil); err == nil {
		t.Error("expected an error when no images are supplied")
	}
	if called {
		t.Error("no HTTP request should be made for an empty frame set")
	}
}

// Local vision (§10 V44): Ollama's AskAboutImages sets the per-message `images` array with bare
// base64 (NO data-URI prefix — that is the hosted shape) and posts to /api/chat.
func TestOllama_AskAboutImages_SetsImagesArray(t *testing.T) {
	var sentReq map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"a red sports car"},"prompt_eval_count":14,"eval_count":5}`))
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llava:13b")
	frame := []byte{0xFF, 0xD8, 0x03} // → "/9gD"
	resp, err := o.AskAboutImages(context.Background(), "describe the frame", [][]byte{frame})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "a red sports car" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Attribution.RequestedProvider != "ollama" || resp.Attribution.Tokens.Prompt != 14 || resp.Attribution.Tokens.Completion != 5 {
		t.Errorf("attribution = %+v", resp.Attribution)
	}
	if path != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", path)
	}
	if sentReq["format"] != "json" {
		t.Errorf("format = %v, want json so local vision cannot spend retries on malformed syntax", sentReq["format"])
	}

	msgs, _ := sentReq["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	m0, _ := msgs[0].(map[string]any)
	if m0["content"] != "describe the frame" {
		t.Errorf("content = %v, want the prompt", m0["content"])
	}
	imgs, _ := m0["images"].([]any)
	if len(imgs) != 1 {
		t.Fatalf("want 1 image, got %d: %+v", len(imgs), imgs)
	}
	// Bare base64 — no "data:" prefix. A hosted-style URI here would be a 400 on Ollama.
	if got, _ := imgs[0].(string); got != "/9gD" {
		t.Errorf("image[0] = %q, want bare base64 %q", got, "/9gD")
	}
}

// An empty frame set errors before any HTTP, same as the hosted path.
func TestOllama_AskAboutImages_RejectsNoImages(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llava")
	if _, err := o.AskAboutImages(context.Background(), "?", nil); err == nil {
		t.Error("expected an error when no images are supplied")
	}
	if called {
		t.Error("no HTTP request should be made for an empty frame set")
	}
}

// THE HOT-PATH GUARD (§10 V44): the `images` field is the one V44 change to the shared Chat
// wire type, so a text-only request must serialise EXACTLY as before — the field must be absent,
// not present-and-empty. This test drives a real text Chat and asserts the marshalled body of
// every message carries no "images" key. It goes RED the instant someone drops the omitempty (or
// sets the field on the text path), which is precisely the regression the field's presence risks.
func TestOllama_TextChat_OmitsImagesKey(t *testing.T) {
	var rawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rawBody = string(b)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llama3.1:8b")
	if _, err := o.Chat(context.Background(), []llm.Message{
		{Role: llm.System, Content: "no inventing"},
		{Role: llm.User, Content: "build a channel"},
	}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}

	// Byte-level assertion: the substring must not appear anywhere in the serialised request.
	// Checking the string (not a decoded map) is deliberate — omitempty is a MARSHAL property,
	// and a decode-then-inspect would pass even if the field serialised as `"images":null`.
	if strings.Contains(rawBody, "images") {
		t.Errorf("text-only Chat must not serialise an images key; body was:\n%s", rawBody)
	}
	// Sanity: it really was a full request, not an empty body that trivially lacks the key.
	if !strings.Contains(rawBody, `"content":"build a channel"`) {
		t.Errorf("guard is vacuous — the request body did not contain the message: %s", rawBody)
	}
}
