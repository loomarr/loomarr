package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Audio input over the OpenAI-compatible chat API (§8.1, for §10 V40's language gate).
//
// ⚠ **This is NOT a transcription endpoint.** `/audio/transcriptions` is a separate capability
// (implemented in transcription.go) that turns speech into timed text. This path asks a model to
// REASON about audio — currently the narrow language question — through chat completions. OpenRouter
// exposes both under one key, but the request shapes and model modalities remain distinct.
//
// That difference is why this is a small separate file rather than a flag on `Chat`: the request
// shape is genuinely different (content becomes an ARRAY of typed parts, not a string), and
// forcing that into the text path would complicate every existing caller for one feature.

// AudioRequest is one question about one audio clip.
type AudioRequest struct {
	// Model is the audio-capable model id (e.g. "google/gemini-3.5-flash-lite").
	Model string
	// Prompt is what to ask about the audio.
	Prompt string
	// Audio is the raw bytes; Format is the container ("wav", "mp3").
	//
	// ⚠ Sent INLINE as base64, which bounds what this is for. A ten-second 16kHz mono wav is
	// ~320KB and encodes to ~430KB — fine in a JSON body. Do not reach for this with a whole
	// clip: the language gate samples a span precisely so it stays small.
	Audio  []byte
	Format string
	// MaxTokens caps the answer. The language gate wants a word, not an essay.
	MaxTokens int
}

// audioChatReq mirrors openaiChatReq but with multimodal content parts.
//
// A separate type rather than making `openaiMessage.Content` an `any`: that field is marshalled on
// every text request in the app, and widening its type to serve one caller would put a
// runtime-typed field on the hot path for no benefit there.
type audioChatReq struct {
	Model     string         `json:"model"`
	Messages  []audioMessage `json:"messages"`
	MaxTokens int            `json:"max_tokens,omitempty"`
}

type audioMessage struct {
	Role    string      `json:"role"`
	Content []audioPart `json:"content"`
}

// audioPart is one element of a multimodal content array. Exactly one of Text/InputAudio is set,
// selected by Type — the shape OpenAI defined and every compatible gateway copies.
type audioPart struct {
	Type       string          `json:"type"` // "text" | "input_audio"
	Text       string          `json:"text,omitempty"`
	InputAudio *audioPartAudio `json:"input_audio,omitempty"`
}

type audioPartAudio struct {
	// Data is base64 WITHOUT a data: URI prefix — the audio part takes bare base64, unlike the
	// image part next to it in the same spec, which takes a full data URI. Getting this wrong
	// produces a 400 that names neither field.
	Data   string `json:"data"`
	Format string `json:"format"`
}

// AskAboutAudio sends one audio question and returns the model's text answer.
//
// It deliberately does NOT use the Provider interface: that models a tool-calling chat loop, and
// this is a single stateless question whose answer is one word. Reusing it would mean satisfying
// Tools/JSONMode/streaming for a call that wants none of them.
func (o *OpenAI) AskAboutAudio(ctx context.Context, req AudioRequest) (string, error) {
	if len(req.Audio) == 0 {
		return "", fmt.Errorf("audio request carries no audio")
	}
	model := req.Model
	if model == "" {
		model = o.model
	}
	format := req.Format
	if format == "" {
		format = "wav"
	}

	body, err := json.Marshal(audioChatReq{
		Model:     model,
		MaxTokens: req.MaxTokens,
		Messages: []audioMessage{{
			Role: "user",
			Content: []audioPart{
				{Type: "text", Text: req.Prompt},
				{Type: "input_audio", InputAudio: &audioPartAudio{
					Data:   base64.StdEncoding.EncodeToString(req.Audio),
					Format: format,
				}},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("marshal audio request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("audio chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// ⚠ The body carries WHY, and it matters more here than on the text path: the common
		// failures are "this model has no audio input" and "your key has no credit", which are an
		// operator's to fix and are indistinguishable from a bare status code.
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("audio chat: status %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}

	var out openaiChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode audio response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("audio chat: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("audio chat: no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
