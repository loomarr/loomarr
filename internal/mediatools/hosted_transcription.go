package mediatools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// AudioTranscriptionClient is the provider seam HostedTranscriber needs. It deliberately speaks
// in mediatools' timed segment type; the composition-root adapter maps the hosted client's wire
// type once, and the filler pipeline stays unaware of the vendor.
type AudioTranscriptionClient interface {
	TranscribeAudio(ctx context.Context, model, format, language string, audio []byte) ([]TranscriptSegment, error)
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
	if endMs <= startMs {
		return nil, fmt.Errorf("hosted transcription span [%d,%d) is empty", startMs, endMs)
	}
	if h.Client == nil || h.Client() == nil {
		return nil, fmt.Errorf("hosted transcription provider is not configured")
	}
	model := ""
	if h.Model != nil {
		model = h.Model()
	}
	if model == "" {
		return nil, fmt.Errorf("hosted transcription model is not configured")
	}
	language := ""
	if h.Language != nil {
		language = h.Language()
	}
	dir := ""
	if h.Extract == nil {
		var err error
		dir, err = os.MkdirTemp(h.TmpDir, "loomarr-hosted-stt-")
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
	}

	var out []TranscriptSegment
	for chunkStart := startMs; chunkStart < endMs; chunkStart += hostedTranscriptionChunkMs {
		chunkEnd := min(chunkStart+hostedTranscriptionChunkMs, endMs)
		var audio []byte
		if h.Extract != nil {
			var err error
			audio, err = h.Extract(ctx, file, chunkStart, chunkEnd)
			if err != nil {
				return nil, err
			}
		} else {
			wav := filepath.Join(dir, "span.wav")
			if err := ExtractSpanWAV(ctx, h.FFmpegPath, file, chunkStart, chunkEnd, wav); err != nil {
				return nil, err
			}
			var err error
			audio, err = os.ReadFile(wav)
			if err != nil {
				return nil, fmt.Errorf("read hosted transcription audio: %w", err)
			}
		}
		client := h.Client()
		if client == nil {
			return nil, fmt.Errorf("hosted transcription provider became unavailable")
		}
		segments, err := client.TranscribeAudio(ctx, model, "wav", language, audio)
		if err != nil {
			return nil, err
		}
		offset := chunkStart - startMs
		chunkDuration := chunkEnd - chunkStart
		for _, seg := range segments {
			seg.StartMs = max(0, min(seg.StartMs, chunkDuration)) + offset
			seg.EndMs = max(0, min(seg.EndMs, chunkDuration)) + offset
			if seg.EndMs > seg.StartMs {
				out = append(out, seg)
			}
		}
	}
	return out, nil
}

var _ SpanTranscriber = (*HostedTranscriber)(nil)
