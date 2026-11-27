package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AudioTranscriptionClient is the provider seam HostedTranscriber needs. It deliberately speaks
// in this package's timed segment type; the composition-root adapter maps the hosted client's wire
// type once, and the filler pipeline stays unaware of the vendor.
type AudioTranscriptionClient interface {
	TranscribeAudio(ctx context.Context, model, format, language string, audio []byte) (AudioTranscription, error)
}

// AudioTranscription is the provider-neutral speech result. Language is the detected language,
// not a requested hint; callers leave the hint empty when detection is the purpose of the call.
type AudioTranscription struct {
	Language string
	Segments []TranscriptSegment
}

// HostedTranscriber extracts bounded WAV spans and sends them to an STT provider. OpenRouter's
// processing timeout is roughly 60 seconds, so long source spans are chunked below it and the
// returned timestamps are reassembled relative to the original requested span.
type HostedTranscriber struct {
	FFmpegPath string
	Client     func() AudioTranscriptionClient
	Model      func() string
	Language   func() string
	TmpDir     string
	// Extract is an internal seam for tests. Production leaves it nil and uses ffmpeg; keeping
	// extraction behind one function lets timing/chunk logic be tested without shelling out.
	Extract func(ctx context.Context, file string, startMs, endMs int64) ([]byte, error)
}

const hostedTranscriptionChunkMs int64 = 45_000

func (h *HostedTranscriber) Transcribe(ctx context.Context, file string, startMs, endMs int64) ([]TranscriptSegment, error) {
	language := ""
	if h.Language != nil {
		language = h.Language()
	}
	speech, err := h.TranscribeLanguage(ctx, file, startMs, endMs, language)
	return speech.Segments, err
}

// TranscribeLanguage transcribes [startMs,endMs) of file in language, or lets the provider detect
// it when language is empty, and reports the language of the first chunk that names one. Segment
// offsets are relative to startMs. A provider refusal (an HTTP 429 included) is returned as the
// error, once: nothing here retries.
func (h *HostedTranscriber) TranscribeLanguage(ctx context.Context, file string, startMs, endMs int64, language string) (AudioTranscription, error) {
	if endMs <= startMs {
		return AudioTranscription{}, fmt.Errorf("hosted transcription span [%d,%d) is empty", startMs, endMs)
	}
	if h.Client == nil || h.Client() == nil {
		return AudioTranscription{}, fmt.Errorf("hosted transcription provider is not configured")
	}
	model := ""
	if h.Model != nil {
		model = h.Model()
	}
	if model == "" {
		return AudioTranscription{}, fmt.Errorf("hosted transcription model is not configured")
	}
	dir := ""
	if h.Extract == nil {
		var err error
		dir, err = os.MkdirTemp(h.TmpDir, "loomarr-hosted-stt-")
		if err != nil {
			return AudioTranscription{}, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
	}

	var out AudioTranscription
	for chunkStart := startMs; chunkStart < endMs; chunkStart += hostedTranscriptionChunkMs {
		chunkEnd := min(chunkStart+hostedTranscriptionChunkMs, endMs)
		var audio []byte
		if h.Extract != nil {
			var err error
			audio, err = h.Extract(ctx, file, chunkStart, chunkEnd)
			if err != nil {
				return AudioTranscription{}, err
			}
		} else {
			wav := filepath.Join(dir, "span.wav")
			if err := ExtractSpanWAV(ctx, h.FFmpegPath, file, chunkStart, chunkEnd, wav); err != nil {
				return AudioTranscription{}, err
			}
			var err error
			audio, err = os.ReadFile(wav)
			if err != nil {
				return AudioTranscription{}, fmt.Errorf("read hosted transcription audio: %w", err)
			}
		}
		client := h.Client()
		if client == nil {
			return AudioTranscription{}, fmt.Errorf("hosted transcription provider became unavailable")
		}
		transcription, err := client.TranscribeAudio(ctx, model, "wav", language, audio)
		if err != nil {
			return AudioTranscription{}, err
		}
		if out.Language == "" {
			out.Language = strings.ToLower(strings.TrimSpace(transcription.Language))
		}
		offset := chunkStart - startMs
		chunkDuration := chunkEnd - chunkStart
		for _, seg := range transcription.Segments {
			seg.StartMs = max(0, min(seg.StartMs, chunkDuration)) + offset
			seg.EndMs = max(0, min(seg.EndMs, chunkDuration)) + offset
			if seg.EndMs > seg.StartMs {
				out.Segments = append(out.Segments, seg)
			}
		}
	}
	return out, nil
}

var _ SpanTranscriber = (*HostedTranscriber)(nil)
