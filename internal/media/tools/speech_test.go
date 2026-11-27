package tools

import (
	"context"
	"errors"
	"testing"
)

func TestParseWhisperSpeech_ReportsTheDetectedLanguage(t *testing.T) {
	out := []byte(`{"result":{"language":"ES"},"transcription":[
		{"offsets":{"from":0,"to":1500},"text":" Hola"},
		{"offsets":{"from":1500,"to":1500},"text":" sin duración"}
	]}`)
	speech, err := parseWhisperSpeech(out)
	if err != nil {
		t.Fatal(err)
	}
	if speech.Language != "es" || len(speech.Segments) != 1 || speech.Segments[0].Text != "Hola" {
		t.Fatalf("speech = %+v", speech)
	}
}

type languageSTT struct {
	calls     int
	languages []string
	err       error
}

func (l *languageSTT) TranscribeAudio(_ context.Context, _, _, language string, _ []byte) (AudioTranscription, error) {
	l.calls++
	l.languages = append(l.languages, language)
	if l.err != nil {
		return AudioTranscription{}, l.err
	}
	return AudioTranscription{Language: "FR", Segments: []TranscriptSegment{{StartMs: 0, EndMs: 900, Text: "bonjour"}}}, nil
}

func hostedOver(client AudioTranscriptionClient) *HostedTranscriber {
	return &HostedTranscriber{
		Client:   func() AudioTranscriptionClient { return client },
		Model:    func() string { return "whisper-1" },
		Language: func() string { return "en" },
		Extract:  func(context.Context, string, int64, int64) ([]byte, error) { return []byte("wav"), nil },
	}
}

func TestHostedTranscriber_TranscribeLanguageAsksForTheGivenLanguageAndReportsTheProviders(t *testing.T) {
	client := &languageSTT{}
	h := hostedOver(client)
	speech, err := h.TranscribeLanguage(context.Background(), "show.mkv", 0, 30_000, "")
	if err != nil {
		t.Fatal(err)
	}
	if speech.Language != "fr" || len(speech.Segments) != 1 {
		t.Fatalf("speech = %+v", speech)
	}
	// An empty language asks the provider to detect it; the configured hint is Transcribe's only.
	if len(client.languages) != 1 || client.languages[0] != "" {
		t.Fatalf("languages sent = %q", client.languages)
	}
}

func TestHostedTranscriber_ARefusalIsReturnedOnceWithoutRetry(t *testing.T) {
	busy := errors.New("audio transcription: 429 Too Many Requests")
	client := &languageSTT{err: busy}
	_, err := hostedOver(client).TranscribeLanguage(context.Background(), "show.mkv", 0, 30_000, "en")
	if !errors.Is(err, busy) || client.calls != 1 {
		t.Fatalf("err = %v after %d calls, want the refusal after one", err, client.calls)
	}
}

func TestParseSilenceSpans_RelativeToTheWindowAndClosedAtItsEnd(t *testing.T) {
	stderr := "[silencedetect @ 0x1] silence_start: 2.5\n" +
		"[silencedetect @ 0x1] silence_end: 4 | silence_duration: 1.5\n" +
		"[silencedetect @ 0x1] silence_start: 27\n"
	spans := ParseSilenceSpans(stderr, 30_000)
	want := []Interval{{StartMs: 2_500, EndMs: 4_000}, {StartMs: 27_000, EndMs: 30_000}}
	if len(spans) != len(want) || spans[0] != want[0] || spans[1] != want[1] {
		t.Fatalf("spans = %+v, want %+v", spans, want)
	}
}
