package filler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The hosted language detector (§10 V40) — the other half of `filler.language_provider`.
//
// ⚠ **It asks a QUESTION, it does not transcribe.** OpenAI's `/v1/audio/transcriptions` is a
// separate API that gateways generally do not proxy (OpenRouter, the provider this repo wires,
// does not). What OpenRouter offers is chat models that take audio as an input modality, so this
// hands one a ten-second span and asks what language is being spoken. The answer is a word.
//
// That makes it strictly cheaper than the local path for this task: the model never has to produce
// a transcript nobody reads. It is also architecture-blind — a network call costs the same on
// arm64, where the local backend needs ~341s per clip under QEMU.

// AudioAsker is the narrow slice of the hosted LLM this needs.
//
// Declared HERE rather than imported from `internal/llm` so the filler domain does not depend on
// the LLM package for one method — the same seam-shaped dependency inversion `MediaTools` uses.
// `llm.OpenAI.AskAboutAudio` satisfies it.
type AudioAsker interface {
	AskAboutAudio(ctx context.Context, req AudioAsk) (string, error)
}

// AudioAsk mirrors llm.AudioRequest without importing it. Small enough that duplicating the shape
// costs less than the coupling would.
type AudioAsk struct {
	Model     string
	Prompt    string
	Audio     []byte
	Format    string
	MaxTokens int
}

// languagePrompt is what the model is asked.
//
// ⚠ It has to license "none" explicitly, or the model will always name a language. Asked "what
// language is this?" about ten seconds of music a model reliably guesses — usually English — and a
// confident wrong answer is exactly what rejects a wordless advert that should have been kept.
//
// ⚠ It also has to forbid prose. "The speaker appears to be speaking English" normalises to
// nothing and lands as LangUndetermined, which silently disables the gate for that clip.
const languagePrompt = `What language is being SPOKEN in this audio?

Answer with ONLY a two-letter ISO 639-1 code (en, es, fr, de, ...).
If there is no speech at all — music, sound effects, or silence only — answer exactly: none
Do not explain. Do not write a sentence. Answer with one word.`

// hostedLanguageMaxTokens caps the reply. The answer is one word; anything longer is the model
// ignoring the instruction, and paying for it is pointless.
const hostedLanguageMaxTokens = 8

// HostedLanguage detects with an audio-capable model through the §8.1 hosted provider.
type HostedLanguage struct {
	// Asker is the hosted client. Nil ⇒ every call returns LangUndetermined, so an install that
	// selected `hosted` without configuring a key is inert rather than broken.
	Asker AudioAsker
	// Model is the audio-capable model id. Empty ⇒ the client's configured default.
	Model string
	// FFmpegPath extracts the span; the wire format is 16kHz mono wav for the same reason whisper
	// wants it — small, universally decodable, and ~430KB of base64 for ten seconds.
	FFmpegPath string
	tmpDir     string
}

// NewHostedLanguage builds the hosted detector. Every field may be empty; the result reports
// LangUndetermined rather than erroring, because "we cannot tell" is an answer the gate handles.
func NewHostedLanguage(asker AudioAsker, model, ffmpegPath, tmpDir string) *HostedLanguage {
	return &HostedLanguage{Asker: asker, Model: model, FFmpegPath: ffmpegPath, tmpDir: tmpDir}
}

func (h *HostedLanguage) DetectLanguage(ctx context.Context, file string, startMs, endMs int64) (string, error) {
	if h.Asker == nil {
		return LangUndetermined, nil
	}
	dir, err := os.MkdirTemp(h.tmpDir, "loomarr-lang-hosted-")
	if err != nil {
		return LangUndetermined, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	wav := filepath.Join(dir, "span.wav")
	ff := h.FFmpegPath
	if ff == "" {
		ff = "ffmpeg"
	}
	cut := exec.CommandContext(ctx, ff,
		"-nostdin", "-v", "error",
		"-ss", msToFFmpegTime(startMs), "-t", msToFFmpegTime(endMs-startMs),
		"-i", file, "-vn", "-ac", "1", "-ar", "16000", "-y", wav)
	if out, cutErr := cut.CombinedOutput(); cutErr != nil {
		return LangUndetermined, fmt.Errorf("extract audio for language: %w: %s", cutErr, truncate(string(out), 200))
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

	answer, err := h.Asker.AskAboutAudio(ctx, AudioAsk{
		Model: h.Model, Prompt: languagePrompt, Audio: audio,
		Format: "wav", MaxTokens: hostedLanguageMaxTokens,
	})
	if err != nil {
		return LangUndetermined, fmt.Errorf("hosted language detect: %w", err)
	}
	return parseHostedLanguage(answer), nil
}

// parseHostedLanguage turns a model's reply into one of the three answers.
//
// ⚠ Deliberately strict about LENGTH. A model that ignores "one word" and replies with a sentence
// must land on LangUndetermined (keep the clip), never on a language guessed from the first token
// of prose — "The audio is in Spanish" starts with "the", and a lenient parser that took the first
// word would reject on it.
func parseHostedLanguage(answer string) string {
	a := strings.TrimSpace(strings.ToLower(answer))
	a = strings.Trim(a, `."'`)
	if a == "" {
		return LangUndetermined
	}
	// One word only. Anything longer is the model explaining rather than answering.
	if fields := strings.Fields(a); len(fields) == 1 {
		a = fields[0]
	} else {
		return LangUndetermined
	}
	if a == LangNone || a == "silence" || a == "music" {
		return LangNone
	}
	code := NormalizeLanguage(a)
	// A plausible code is two letters. Longer survived normalisation as an unrecognised word,
	// which is not an answer.
	if len(code) != 2 {
		return LangUndetermined
	}
	return code
}
