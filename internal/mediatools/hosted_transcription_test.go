package mediatools

import (
	"context"
	"fmt"
	"testing"
)

type recordingSTT struct {
	calls int
	model string
	lang  string
}

type fixedSpanTranscriber struct{ calls int }

func (f *fixedSpanTranscriber) Transcribe(context.Context, string, int64, int64) ([]TranscriptSegment, error) {
	f.calls++
	return []TranscriptSegment{{StartMs: 0, EndMs: 1000, Text: "hosted"}}, nil
}

func (r *recordingSTT) TranscribeAudio(_ context.Context, model, format, language string, audio []byte) ([]TranscriptSegment, error) {
	r.calls++
	r.model, r.lang = model, language
	if format != "wav" || string(audio) != fmt.Sprintf("chunk-%d", r.calls) {
		return nil, fmt.Errorf("request %d = %q/%q", r.calls, format, audio)
	}
	return []TranscriptSegment{{StartMs: 100, EndMs: 1_100, Text: fmt.Sprintf("part %d", r.calls)}}, nil
}

func TestHostedTranscriber_ChunksAndReassemblesTimestamps(t *testing.T) {
	client := &recordingSTT{}
	extractions := 0
	h := &HostedTranscriber{
		Client:   func() AudioTranscriptionClient { return client },
		Model:    func() string { return "openai/whisper-large-v3" },
		Language: func() string { return "en" },
		Extract: func(_ context.Context, _ string, startMs, endMs int64) ([]byte, error) {
			extractions++
			if endMs-startMs > hostedTranscriptionChunkMs {
				t.Fatalf("oversized chunk [%d,%d)", startMs, endMs)
			}
			return []byte(fmt.Sprintf("chunk-%d", extractions)), nil
		},
	}

	segments, err := h.Transcribe(context.Background(), "reel.mp4", 10_000, 106_000)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 || client.model != "openai/whisper-large-v3" || client.lang != "en" {
		t.Fatalf("client = %+v", client)
	}
	wantStarts := []int64{100, 45_100, 90_100}
	if len(segments) != len(wantStarts) {
		t.Fatalf("segments = %+v", segments)
	}
	for i, want := range wantStarts {
		if segments[i].StartMs != want {
			t.Errorf("segment %d start = %d, want %d", i, segments[i].StartMs, want)
		}
	}
}

func TestHostedTranscriber_RequiresConfiguredProvider(t *testing.T) {
	h := &HostedTranscriber{Client: func() AudioTranscriptionClient { return nil }}
	if _, err := h.Transcribe(context.Background(), "clip.mp4", 0, 30_000); err == nil {
		t.Fatal("missing provider accepted")
	}
}

func TestFFmpegTools_TranscriberSelectionIsLive(t *testing.T) {
	hosted := &fixedSpanTranscriber{}
	useHosted := false
	tools := NewFFmpegTools("ffmpeg", "ffprobe", "", "", "").WithTranscriber(func() SpanTranscriber {
		if useHosted {
			return hosted
		}
		return nil
	})
	if _, err := tools.Transcribe(context.Background(), "clip.mp4", 0, 30_000); err == nil {
		t.Fatal("local path without whisper unexpectedly succeeded")
	}
	useHosted = true
	segments, err := tools.Transcribe(context.Background(), "clip.mp4", 0, 30_000)
	if err != nil {
		t.Fatal(err)
	}
	if hosted.calls != 1 || len(segments) != 1 || segments[0].Text != "hosted" {
		t.Fatalf("hosted result = %+v, calls %d", segments, hosted.calls)
	}
}
