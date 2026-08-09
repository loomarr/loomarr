package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Language detection for the quality gate (§10 V40).
//
// The question is narrow: "is this clip's SPEECH in the language filler is expected to be in?"
// Not "transcribe it" — a classification over the first few seconds is enough, which is why the
// local backend can use a far smaller model than compilation splitting needs.
//
// ⚠ **Three answers, not two**, and collapsing any pair breaks the gate:
//
//	""      could not tell (backend unavailable, or it failed). NEVER a reason to reject —
//	        the job retries, and a clip is not guilty of being foreign because whisper is missing.
//	"none"  there is no speech to judge. A wordless visual spot. ALWAYS KEPT.
//	"en"/…  this is what was heard.
//
// The middle one is the one people merge with the first, because both mean "we are not rejecting
// this". They are different facts: one is a retry, the other is a final answer, and a silent
// advert is often the best filler there is.

// LangUndetermined and LangNone are the two non-language answers. Constants rather than bare
// strings so a caller cannot typo `"None"` into a language code that matches nothing.
const (
	LangUndetermined = ""
	LangNone         = "none"
)

// LanguageDetector answers what language a clip's speech is in.
//
// ONE method, and it returns a plain code (`en`, `es`, `none`, or `""`) rather than a transcript.
// That is what lets the whisper backend and the hosted backend be interchangeable: they arrive at
// the answer completely differently — one runs a local model and reads a field, the other asks a
// multimodal model a question — but the job, the reject rule and every test see only the code.
type LanguageDetector interface {
	// DetectLanguage inspects [startMs,endMs) of file. It returns LangUndetermined with a nil
	// error when it genuinely could not tell, so a caller can distinguish "unavailable" from
	// "failed" only when it cares — most do not, because both mean "do not reject".
	DetectLanguage(ctx context.Context, file string, startMs, endMs int64) (string, error)
}

// LanguageSampleMs is how much audio a detector inspects.
//
// Ten seconds, not the whole clip. Language identification reads phonetics, not meaning, so it
// converges in a couple of seconds — and on the local backend every extra second is real compute
// (~341s per clip under QEMU is the constraint that made this a background job at all).
const LanguageSampleMs int64 = 10_000

// LanguageSpan returns the window a detector should inspect for a clip of the given length.
//
// ⚠ Starts at 1s rather than 0. The first moments of an advert are very often a musical sting or
// a silent logo card, and a detector handed only that answers "none" for a clip that talks for the
// remaining 28 seconds. One second in is past the sting on the spots this catalog holds.
func LanguageSpan(durationMs int64) (startMs, endMs int64) {
	const skipMs int64 = 1_000
	if durationMs <= 0 {
		// Unprobed: ask for the window anyway and let the backend take what exists. Guessing a
		// proportion of an unknown length is meaningless.
		return skipMs, skipMs + LanguageSampleMs
	}
	if durationMs <= LanguageSampleMs {
		// Shorter than the window — take all of it, including the sting. A 6s bumper has no
		// spare second to skip, and taking nothing would report "none" for every short clip.
		return 0, durationMs
	}
	// ⚠ **A LONG recording is sampled from its MIDDLE, not its opening.** A fixed 1s offset is
	// right for a 30-second spot and wrong for a 978-second one: recorded ad breaks and VHS
	// transfers begin with leader — dead air, a blank frame, tape run-up — so the first ten
	// seconds are silence, and a detector handed silence produces a guess.
	//
	// Found live: "USA Network Commercial Breaks (10-6-1994)", 978s, whose first 10s measure
	// **-70 LUFS**. The model was asked what language the silence was in, answered `ar`, and the
	// gate tombstoned a perfectly good American ad break.
	//
	// The threshold is where a clip stops being an advert and starts being a recording of several.
	if durationMs > longRecordingMs {
		start := durationMs/2 - LanguageSampleMs/2
		return start, start + LanguageSampleMs
	}

	start := skipMs
	if end := start + LanguageSampleMs; end > durationMs {
		start = durationMs - LanguageSampleMs
	}
	return start, start + LanguageSampleMs
}

// longRecordingMs is where a clip stops being one advert and starts being a recording of several.
//
// Two minutes: no single commercial runs that long, so anything past it is a compilation, a break
// recording or a tape transfer — all of which open with leader rather than content.
const longRecordingMs int64 = 120_000

// NormalizeLanguage reduces a code to its base tag for comparison: `en-US`, `EN`, `eng ` all
// become `en`.
//
// ⚠ Comparing raw codes is the obvious implementation and it is wrong in a way that only shows up
// on real data: whisper answers `en`, a hosted model may answer `en-US` or `English`, and an
// operator may type `EN` into the setting. Any of those mismatching the others means the gate
// rejects every English clip in the catalog — the loudest possible failure, arrived at honestly.
func NormalizeLanguage(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if c == "" {
		return ""
	}
	// Strip a region or script subtag: en-US, zh-Hans, pt_BR.
	for _, sep := range []string{"-", "_"} {
		if i := strings.Index(c, sep); i > 0 {
			c = c[:i]
			break
		}
	}
	// A few spellings a model may return in prose rather than as a code. Deliberately SHORT:
	// this is a safety net for the common case, not a language database — an unrecognised name
	// stays as-is and simply will not match, which is the honest outcome.
	switch c {
	case "english":
		return "en"
	case "spanish", "espanol", "español":
		return "es"
	case "french", "francais", "français":
		return "fr"
	case "german", "deutsch":
		return "de"
	}
	// ISO 639-2 → 639-1 for the handful whisper can emit in three-letter form.
	if len(c) == 3 {
		switch c {
		case "eng":
			return "en"
		case "spa":
			return "es"
		case "fra", "fre":
			return "fr"
		case "deu", "ger":
			return "de"
		}
	}
	return c
}

// LanguageRejects decides whether a detected code means "drop this clip".
//
// ⚠ **The polarity is deliberately conservative: it rejects only on a POSITIVE mismatch.** Every
// uncertain state — not yet checked, no speech, backend unavailable, an unparseable answer — keeps
// the clip. A gate over a probabilistic signal has to fail toward keeping, because the cost of the
// two mistakes is not symmetric: a foreign advert that slips through plays once and annoys, while
// a wrongly-rejected clip is deleted from the catalog and the operator is never told which.
func LanguageRejects(detected, want string) bool {
	d, w := NormalizeLanguage(detected), NormalizeLanguage(want)
	if w == "" || d == "" || d == LangNone {
		return false // gate off, not yet checked, or nothing to judge
	}
	return d != w
}

// --- the local backend ---

// WhisperLanguage detects with the vendored whisper-cli.
//
// ⚠ **It needs a MULTILINGUAL model, and the shipped `ggml-small.en.bin` is not one.** An
// English-only whisper build does not identify languages at all — it assumes English and
// transcribes accordingly, so asked about a Spanish advert it answers `en`. Pointed at that model
// this gate would silently never reject anything: a feature that looks shipped and does nothing.
// The image therefore vendors `ggml-tiny.bin` alongside it, wired through `filler.language_model`.
//
// `tiny` is adequate here in a way it was NOT for splitting (where `tiny.en` measurably dropped a
// whole 20s advert): language ID is classification over a few seconds, not transcription.
type WhisperLanguage struct {
	// WhisperPath is the same binary compilation splitting uses.
	WhisperPath string
	// Model MUST be multilingual. Empty ⇒ every call returns LangUndetermined, which the gate
	// treats as "do not reject" — so a source build with no model configured is inert, not broken.
	Model string
	// FFmpegPath extracts the audio span; whisper.cpp wants 16kHz mono wav.
	FFmpegPath string
	// tmpDir holds the wav/json intermediates; os.MkdirTemp when empty.
	tmpDir string
}

// NewWhisperLanguage builds the local detector. Any path may be empty — the result is a detector
// that reports LangUndetermined rather than one that errors, because "we cannot tell" is a
// legitimate answer this gate already knows how to handle.
func NewWhisperLanguage(whisperPath, model, ffmpegPath, tmpDir string) *WhisperLanguage {
	return &WhisperLanguage{
		WhisperPath: whisperPath, Model: model, FFmpegPath: ffmpegPath, tmpDir: tmpDir,
	}
}

// whisperLangJSON is the subset of `whisper-cli -oj` this needs.
//
// ⚠ `result.language` is the DETECTED language; `params.language` is what was REQUESTED (usually
// "auto"). Reading the wrong one gives you your own input back — a detector that always agrees
// with itself. Verified against whisper.cpp v1.9.1's cli.cpp, which emits both.
type whisperLangJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Text string `json:"text"`
	} `json:"transcription"`
}

func (w *WhisperLanguage) DetectLanguage(ctx context.Context, file string, startMs, endMs int64) (string, error) {
	if w.WhisperPath == "" || w.Model == "" {
		// Not configured. LangUndetermined + nil: the gate keeps the clip, and the job can retry
		// once an operator wires a model. An error here would fill the log with a condition that
		// is a configuration choice rather than a fault.
		return LangUndetermined, nil
	}
	dir, err := os.MkdirTemp(w.tmpDir, "loomarr-lang-")
	if err != nil {
		return LangUndetermined, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// whisper.cpp wants 16kHz mono wav; ffmpeg extracts just the span. Shared with the hosted
	// backend (`extractSpanWAV`) — the two carried byte-identical copies of this until V41.
	wav := spanWAVPath(dir)
	if err := extractSpanWAV(ctx, w.FFmpegPath, file, startMs, endMs, wav); err != nil {
		return LangUndetermined, err
	}

	base := filepath.Join(dir, "out")
	// ⚠ `-l auto` is the whole point: without it whisper uses its default (English) and reports
	// that back, so the detector would agree with itself for every clip on earth.
	run := exec.CommandContext(ctx, w.WhisperPath,
		"-m", w.Model, "-f", wav, "-l", "auto", "-oj", "-of", base, "-np")
	if out, runErr := run.CombinedOutput(); runErr != nil {
		return LangUndetermined, fmt.Errorf("whisper-cli language: %w: %s", runErr, truncate(string(out), 200))
	}
	raw, err := os.ReadFile(base + ".json")
	if err != nil {
		return LangUndetermined, fmt.Errorf("whisper-cli wrote no JSON: %w", err)
	}
	return parseWhisperLanguage(raw)
}

// parseWhisperLanguage reads the detected code, and decides whether there was any speech at all.
func parseWhisperLanguage(raw []byte) (string, error) {
	var w whisperLangJSON
	if err := json.Unmarshal(raw, &w); err != nil {
		return LangUndetermined, fmt.Errorf("whisper language output not JSON: %w", err)
	}
	// ⚠ NO transcribed text means NO speech — `none`, which always keeps the clip. Without this
	// branch a silent visual spot inherits whatever `result.language` guessed from noise, and a
	// confident-sounding wrong answer is exactly what rejects a clip that should have been kept.
	spoke := false
	for _, t := range w.Transcription {
		if strings.TrimSpace(t.Text) != "" {
			spoke = true
			break
		}
	}
	if !spoke {
		return LangNone, nil
	}
	lang := NormalizeLanguage(w.Result.Language)
	if lang == "" || lang == "auto" {
		return LangUndetermined, nil
	}
	return lang, nil
}

// silenceFloorLUFS is the loudness below which a span carries nothing worth asking about.
//
// -50 LUFS is far below anything audible — the quietest real clip measured in this catalog was
// -32.6 LUFS (§10 V40's loudness work), and the leader that caused the false positive measured
// **-70**. The gap between those two numbers is wide enough that this cannot mistake a quiet
// advert for silence, which is the failure that would matter: a quiet clip must still be judged.
const silenceFloorLUFS = -50.0

// spanIsSilent reports whether an extracted span is effectively silent.
//
// ⚠ **The guard that the byte-size check could not be.** A ten-second wav of leader is 320KB of
// near-zero samples — full size, entirely empty. Asked what language that is, a model guesses
// rather than declining, and the gate then deletes a clip on the strength of a guess.
//
// Uses `ebur128`, the same measurement the loudness half of V40 already relies on, so "silent"
// means the same thing in both places rather than being a second opinion.
//
// An error is reported as NOT silent: failing to measure must not become grounds for a verdict in
// either direction, and the caller keeps the clip either way.
func spanIsSilent(ctx context.Context, ffmpegPath, wav string) (bool, error) {
	out, err := exec.CommandContext(ctx, ffmpegOr(ffmpegPath),
		"-nostdin", "-i", wav, "-af", "ebur128=framelog=quiet", "-f", "null", "-").CombinedOutput()
	if err != nil {
		return false, err
	}
	// ffmpeg prints the summary to stderr; the integrated line reads "    I:         -70.0 LUFS".
	idx := strings.Index(string(out), "Integrated loudness")
	if idx < 0 {
		return false, fmt.Errorf("no integrated loudness in ffmpeg output")
	}
	for _, line := range strings.Split(string(out)[idx:], "\n") {
		fields := strings.Fields(line)
		// "I: -70.0 LUFS"
		if len(fields) >= 3 && fields[0] == "I:" && fields[2] == "LUFS" {
			lufs, parseErr := strconv.ParseFloat(fields[1], 64)
			if parseErr != nil {
				return false, parseErr
			}
			return lufs < silenceFloorLUFS, nil
		}
	}
	return false, fmt.Errorf("no integrated loudness value in ffmpeg output")
}

// msToFFmpegTime renders milliseconds as ffmpeg's seconds-with-decimals.
func msToFFmpegTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%d.%03d", ms/1000, ms%1000)
}
