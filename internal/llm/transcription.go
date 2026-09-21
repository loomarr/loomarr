package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// Hosted speech-to-text over the OpenAI-compatible transcription endpoint (§10). OpenRouter uses
// the same base URL and bearer key as chat/vision, but this is a distinct capability and model:
// an STT model cannot answer the grounded chat loop, and a chat model does not imply timestamps.
// OpenRouter's transcription endpoint does not currently apply chat provider-routing controls;
// callers must not infer pinned-provider, disabled-fallback, or per-request ZDR authority from a
// route configured on OpenAI. Capability snapshots support diagnostics, not that inference.

type TranscriptionRequest struct {
	Model    string
	Audio    []byte
	Format   string
	Language string
}

type TranscriptionSegment struct {
	StartMs int64
	EndMs   int64
	Text    string
}

type TranscriptionResult struct {
	Segments    []TranscriptionSegment
	Language    string
	Attribution Attribution
}

type transcriptionResponse struct {
	ID                 string             `json:"id"`
	Model              string             `json:"model"`
	Text               string             `json:"text"`
	Language           string             `json:"language"`
	Duration           float64            `json:"duration"`
	Usage              openAIUsage        `json:"usage"`
	OpenRouterMetadata openRouterMetadata `json:"openrouter_metadata"`
	Segments           []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// TranscribeAudio requests verbose JSON because compilation rescue needs timed utterances. A
// provider that returns only plain text is rejected rather than assigning invented timestamps;
// the caller can retry or fall back to review without manufacturing cut evidence.
func (o *OpenAI) TranscribeAudio(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResult{}, fmt.Errorf("transcription request carries no audio")
	}
	started := time.Now()
	model := req.Model
	if model == "" {
		model = o.model
	}
	format := req.Format
	if format == "" {
		format = "wav"
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"model": model, "response_format": "verbose_json", "timestamp_granularities[]": "segment",
	} {
		if err := writer.WriteField(name, value); err != nil {
			return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
		}
	}
	if req.Language != "" {
		if err := writer.WriteField("language", req.Language); err != nil {
			return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, "audio."+format))
	header.Set("Content-Type", "audio/"+format)
	part, err := writer.CreatePart(header)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
	}
	if _, err := part.Write(req.Audio); err != nil {
		return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
	}
	if err := writer.Close(); err != nil {
		return TranscriptionResult{}, fmt.Errorf("build transcription request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return TranscriptionResult{}, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	o.addMetadataHeader(httpReq)
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("audio transcription: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(io.LimitReader(resp.Body, 512))
		return TranscriptionResult{}, fmt.Errorf("audio transcription: status %d: %s", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	var out transcriptionResponse
	if err := decodeOpenAIJSON(resp, &out, "transcription response"); err != nil {
		return TranscriptionResult{}, err
	}
	if out.Error != nil {
		return TranscriptionResult{}, fmt.Errorf("audio transcription: %s", out.Error.Message)
	}
	segments := make([]TranscriptionSegment, 0, len(out.Segments))
	for _, seg := range out.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" || seg.End <= seg.Start {
			continue
		}
		segments = append(segments, TranscriptionSegment{
			StartMs: int64(seg.Start * 1000), EndMs: int64(seg.End * 1000), Text: text,
		})
	}
	if len(segments) == 0 && strings.TrimSpace(out.Text) != "" {
		return TranscriptionResult{}, fmt.Errorf("audio transcription returned text without segment timestamps")
	}
	if o.metrics != nil {
		o.metrics.LLMTokens(out.Usage.PromptTokens, out.Usage.CompletionTokens)
	}
	generationID := strings.TrimSpace(out.ID)
	if generationID == "" {
		generationID = strings.TrimSpace(resp.Header.Get("X-Generation-Id"))
	}
	return TranscriptionResult{
		Segments: segments, Language: strings.TrimSpace(strings.ToLower(out.Language)),
		Attribution: attributionFromWire(o.provider, model, generationID, out.Model, out.Usage,
			out.OpenRouterMetadata, []string{"audio", "text"}, time.Since(started)),
	}, nil
}
