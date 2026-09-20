package mediatools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// OpenAITranscriptionClient calls a dedicated OpenAI-compatible speech service. Its URL and key
// are independent closures so settings hot-apply to the next chunk without sending a chat-provider
// credential to a newly selected host.
type OpenAITranscriptionClient struct {
	BaseURL func() string
	APIKey  func() string
	HTTP    *http.Client
}

type openAITranscriptionResponse struct {
	Text     string `json:"text"`
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const maxOpenAITranscriptionResponseBytes = 2 << 20

func (c *OpenAITranscriptionClient) TranscribeAudio(
	ctx context.Context,
	model, format, language string,
	audio []byte,
) ([]TranscriptSegment, error) {
	if len(audio) == 0 {
		return nil, fmt.Errorf("OpenAI-compatible transcription request carries no audio")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("OpenAI-compatible transcription model is not configured")
	}
	baseURL := ""
	if c.BaseURL != nil {
		baseURL = strings.TrimSpace(c.BaseURL())
	}
	endpoint, err := transcriptionEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	key := ""
	if c.APIKey != nil {
		key = strings.TrimSpace(c.APIKey())
	}
	if key == "" {
		return nil, fmt.Errorf("OpenAI-compatible transcription API key is not configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"model":                     model,
		"response_format":           "verbose_json",
		"timestamp_granularities[]": "segment",
	} {
		if err := writer.WriteField(name, value); err != nil {
			return nil, fmt.Errorf("build transcription request: %w", err)
		}
	}
	if language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return nil, fmt.Errorf("build transcription request: %w", err)
		}
	}
	if format == "" {
		format = "wav"
	}
	file, err := writer.CreateFormFile("file", "span."+format)
	if err != nil {
		return nil, fmt.Errorf("build transcription request: %w", err)
	}
	if _, err := file.Write(audio); err != nil {
		return nil, fmt.Errorf("build transcription request: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("build transcription request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible audio transcription: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf(
			"OpenAI-compatible audio transcription: status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(message)),
		)
	}
	var decoded openAITranscriptionResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxOpenAITranscriptionResponseBytes+1))
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible transcription response: %w", err)
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("OpenAI-compatible audio transcription: %s", decoded.Error.Message)
	}
	segments := make([]TranscriptSegment, 0, len(decoded.Segments))
	for _, segment := range decoded.Segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		if segment.Start < 0 || segment.End <= segment.Start {
			return nil, fmt.Errorf("OpenAI-compatible transcription returned an invalid segment timestamp")
		}
		segments = append(segments, TranscriptSegment{
			StartMs: int64(segment.Start * 1000),
			EndMs:   int64(segment.End * 1000),
			Text:    text,
		})
	}
	if len(segments) == 0 && strings.TrimSpace(decoded.Text) != "" {
		return nil, fmt.Errorf("OpenAI-compatible transcription returned text without segment timestamps")
	}
	return segments, nil
}

func transcriptionEndpoint(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("OpenAI-compatible transcription URL is not configured")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("OpenAI-compatible transcription URL must use HTTP or HTTPS")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", fmt.Errorf("OpenAI-compatible transcription URL must be an API root")
	}
	parsed.Path = path.Join(strings.TrimSuffix(parsed.Path, "/"), "audio", "transcriptions")
	return parsed.String(), nil
}

var _ AudioTranscriptionClient = (*OpenAITranscriptionClient)(nil)
