package app

import (
	"context"
	"os/exec"

	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/llm"
)

// resolveTool turns a configured tool path into a usable one, falling back to a PATH lookup.
//
// ⚠ **The fallback is what makes §15's documented behaviour true.** §15 describes the
// ingest binaries as "defaulted to the vendored binaries", but the registry defaults are the empty
// string and only the Docker image sets them — so a source build had ingest off even with the
// tools installed. `settings.toolRunnable` does the same lookup for the FEATURE GATE; this is the
// wiring side of the same rule, and the two disagreeing is what let the UI offer a "Fetch now"
// button that always failed.
//
// Returns "" when the tool cannot be found, which the caller reads as "this downloader is
// unavailable" rather than as an error — an install without yt-dlp is a supported configuration.
func resolveTool(configured, name string) string {
	if configured != "" {
		return configured
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return found
}

// orNone renders an empty tool path for a log line. "none" says the tool is absent; an empty
// string in structured output reads as a logging bug.
func orNone(path string) string {
	if path == "" {
		return "none"
	}
	return path
}

// hostedSTTAdapter maps the OpenAI-compatible transcription wire into media/tools' timed segment
// seam, including the service's detected language.
type hostedSTTAdapter struct{ oa *llm.OpenAI }

func (a hostedSTTAdapter) TranscribeAudio(ctx context.Context, model, format, language string, audio []byte) (tools.AudioTranscription, error) {
	result, err := a.oa.TranscribeAudio(ctx, llm.TranscriptionRequest{
		Model: model, Audio: audio, Format: format, Language: language,
	})
	if err != nil {
		return tools.AudioTranscription{}, err
	}
	out := make([]tools.TranscriptSegment, 0, len(result.Segments))
	for _, seg := range result.Segments {
		out = append(out, tools.TranscriptSegment{
			StartMs: seg.StartMs, EndMs: seg.EndMs, Text: seg.Text,
		})
	}
	return tools.AudioTranscription{Language: result.Language, Segments: out}, nil
}
