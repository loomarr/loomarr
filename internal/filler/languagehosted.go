package filler

import (
	"context"
	"fmt"
	"github.com/mantonx/loomarr/internal/mediatools"
	"os"
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
	// Asker resolves the hosted client PER CALL rather than holding one.
	//
	// ⚠ **A func, not a value, and this cost a live debugging session.** The first cut captured
	// `llm.NewOpenAI(url, model, key)` once at boot, so the URL, model and key were frozen at
	// whatever they were when the process started. Changing `llm.model` in Settings then did
	// nothing: the detector kept calling the model configured at startup, which had no audio
	// input, and every clip failed with "No endpoints found that support input audio" — a real
	// 404 about a request the operator thought they had already fixed.
	//
	// Everything else in this feature reads live (`filler.dir`, `filler.language`); this has to
	// as well, or the one setting that decides whether the backend can work at all is the one
	// that needs a restart.
	//
	// Returning nil ⇒ LangUndetermined, so an install that selected `hosted` without configuring
	// a key is inert rather than broken.
	Asker func() AudioAsker
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
func NewHostedLanguage(asker func() AudioAsker, model func() string, ffmpegPath, tmpDir string) *HostedLanguage {
	return &HostedLanguage{Asker: asker, Model: model, FFmpegPath: ffmpegPath, tmpDir: tmpDir}
}

// UnavailableReason checks configuration only; reachability and model capability remain work-time
// failures and therefore keep the retry protection. The closures are deliberately resolved on
// every call so an in-app hosted selection becomes ready without reconstructing this detector.
func (h *HostedLanguage) UnavailableReason() string {
	switch {
	case h.FFmpegPath == "":
		return "audio extraction is not configured (set playout.ffmpeg_path)"
	case h.Model == nil || h.Model() == "":
		return "the hosted language model is not configured"
	case h.Asker == nil || h.Asker() == nil:
		return "the hosted language service is not configured"
	default:
		return ""
	}
}

func (h *HostedLanguage) DetectLanguage(ctx context.Context, file string, startMs, endMs int64) (string, error) {
	// Resolved HERE, per call, not captured at construction — see the field comment. An install
	// that has not configured a hosted client yields nil, which keeps every clip.
	if h.Asker == nil {
		return LangUndetermined, nil
	}
	asker := h.Asker()
	if asker == nil {
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
	answer, err := asker.AskAboutAudio(ctx, AudioAsk{
		Model: model, Prompt: languagePrompt, Audio: audio,
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
