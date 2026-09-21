package filler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/loomarr/loomarr/internal/mediatools"
)

// HostedLanguage detects with the configured speech-recognition service. Language identification
// and timed transcription are one audio capability: asking the chat model a separate audio
// question made installations configure two models for the same clip and failed against dedicated
// ASR services that correctly expose only /audio/transcriptions.
type HostedLanguage struct {
	// Client resolves the speech client PER CALL rather than holding one.
	//
	// ⚠ **A func, not a value, and this cost a live debugging session.** The first cut captured
	// `llm.NewOpenAI(url, model, key)` once at boot, so the URL, model and key were frozen at
	// whatever they were when the process started. Changing `llm.model` in Settings then did
	// nothing: the detector kept calling the model configured at startup, which had no audio
	// input, and every clip failed with "No endpoints found that support input audio" — a real
	// 404 about a request the operator thought they had already fixed.
	//
	// The language policy reads live while the storage layout is generation-scoped; this client
	// selection has to remain live too, or the one setting that decides whether the backend can
	// work at all would need an unrelated restart.
	//
	// Returning nil ⇒ LangUndetermined, so an install that selected `hosted` without configuring
	// a service URL is inert rather than broken. A key is optional for Custom endpoints.
	Client func() mediatools.AudioTranscriptionClient
	// Model is read per call for the same reason.
	Model func() string
	// FFmpegPath extracts the span; the wire format is 16kHz mono wav for the same reason whisper
	// wants it — small, universally decodable, and ~430KB of base64 for ten seconds.
	FFmpegPath string
	tmpDir     string
}

// NewHostedLanguage builds the hosted detector.
//
// ⚠ `asker` and `model` are FUNCS, resolved per call — see the field comments. Either may be nil
// or return a zero value; the result reports LangUndetermined rather than erroring, because
// "we cannot tell" is an answer the gate already knows how to handle.
func NewHostedLanguage(client func() mediatools.AudioTranscriptionClient, model func() string, ffmpegPath, tmpDir string) *HostedLanguage {
	return &HostedLanguage{Client: client, Model: model, FFmpegPath: ffmpegPath, tmpDir: tmpDir}
}

// UnavailableReason checks configuration only; reachability and model capability remain work-time
// failures and therefore keep the retry protection. The closures are deliberately resolved on
// every call so an in-app hosted selection becomes ready without reconstructing this detector.
func (h *HostedLanguage) UnavailableReason() string {
	switch {
	case h.FFmpegPath == "":
		return "audio extraction is not configured (set playout.ffmpeg_path)"
	case h.Model == nil || h.Model() == "":
		return "the connected speech model is not configured"
	case h.Client == nil || h.Client() == nil:
		return "the connected speech service is not configured"
	default:
		return ""
	}
}

func (h *HostedLanguage) DetectLanguage(ctx context.Context, file string, startMs, endMs int64) (string, error) {
	// Resolved HERE, per call, not captured at construction — see the field comment. An install
	// that has not configured a hosted client yields nil, which keeps every clip.
	if h.Client == nil {
		return LangUndetermined, nil
	}
	client := h.Client()
	if client == nil {
		return LangUndetermined, nil
	}
	dir, err := os.MkdirTemp(h.tmpDir, "loomarr-lang-hosted-")
	if err != nil {
		return LangUndetermined, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Shared with the local whisper backend (`extractSpanWAV`): both feed a model that requires
	// 16 kHz mono, so the extraction is the same job whoever runs the inference.
	wav := mediatools.SpanWAVPath(dir)
	if err := mediatools.ExtractSpanWAV(ctx, h.FFmpegPath, file, startMs, endMs, wav); err != nil {
		return LangUndetermined, err
	}
	audio, err := os.ReadFile(wav)
	if err != nil {
		return LangUndetermined, fmt.Errorf("read extracted audio: %w", err)
	}
	// ⚠ A tiny wav is a HEADER and no samples — ffmpeg writes one when the span is past the end of
	// the file, or the stream is empty. Sending it costs money to be told nothing.
	if len(audio) < 1024 {
		return LangNone, nil
	}
	// ⚠ **A FULL-SIZE file of silence is the case that actually bit.** The size check above only
	// catches an empty wav; ten seconds of leader is 320KB of near-zero samples and sails through.
	// Asked what language silence is in, a model does not decline — it guesses.
	//
	// Found live: a 978s recorded ad break whose first 10s measure -70 LUFS was answered `ar` and
	// tombstoned. `LanguageSpan` now samples long recordings from the middle, but that fixes WHERE
	// we look; this is the guard that holds wherever we land, including on a genuinely silent clip.
	if silent, err := spanIsSilent(ctx, h.FFmpegPath, wav); err == nil && silent {
		return LangNone, nil
	}

	model := ""
	if h.Model != nil {
		model = h.Model()
	}
	result, err := client.TranscribeAudio(ctx, model, "wav", "", audio)
	if err != nil {
		return LangUndetermined, fmt.Errorf("connected speech language detection: %w", err)
	}
	spoken := false
	for _, segment := range result.Segments {
		if strings.TrimSpace(segment.Text) != "" {
			spoken = true
			break
		}
	}
	if !spoken {
		return LangNone, nil
	}
	language := NormalizeLanguage(result.Language)
	if len(language) != 2 {
		return LangUndetermined, nil
	}
	return language, nil
}
