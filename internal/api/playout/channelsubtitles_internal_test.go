package playout

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout/subtitles"
)

// subtitleResolver answers one window, and records what it was asked.
type subtitleResolver struct {
	PlayoutResolver
	window   subtitles.Window
	err      error
	from, to time.Time
	key      string
}

func (r *subtitleResolver) Subtitles(_ context.Context, _ string, from, to time.Time, key string) (subtitles.Window, error) {
	r.from, r.to, r.key = from, to, key
	return r.window, r.err
}

func TestChannelSubtitlesAnswersTheWindow(t *testing.T) {
	from := time.UnixMilli(1_800_000_000_000)
	resolver := &subtitleResolver{window: subtitles.Window{
		Tracks: []subtitles.Track{{Key: "en-sdh", Language: "en", Title: "English SDH", HearingImpaired: true}},
		Cues:   []subtitles.Timed{{Start: from.Add(time.Second), End: from.Add(2 * time.Second), Text: "[door creaks]\nWho's there?"}},
	}}
	s := &Handlers{playoutResolver: resolver}

	out, err := s.channelSubtitles(t.Context(), &channelSubtitlesInput{ID: "ch1", From: from.UnixMilli(), Track: "en-sdh"})
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.from.Equal(from) || !resolver.to.Equal(from.Add(subtitleWindowDefault)) || resolver.key != "en-sdh" {
		t.Fatalf("asked %v..%v %q, want two minutes from `from` for en-sdh", resolver.from, resolver.to, resolver.key)
	}
	body := out.Body
	if len(body.Tracks) != 1 || body.Tracks[0] != (SubtitleTrackDTO{Key: "en-sdh", Language: "en", Title: "English SDH", HearingImpaired: true}) {
		t.Fatalf("tracks = %+v", body.Tracks)
	}
	if len(body.Cues) != 1 || body.Cues[0] != (SubtitleCueDTO{StartMs: from.UnixMilli() + 1000, EndMs: from.UnixMilli() + 2000, Text: "[door creaks]\nWho's there?"}) {
		t.Fatalf("cues = %+v", body.Cues)
	}
}

func TestChannelSubtitlesRefusesABadStretch(t *testing.T) {
	from := int64(1_800_000_000_000)
	s := &Handlers{playoutResolver: &subtitleResolver{}}
	for name, to := range map[string]int64{
		"ends before it starts":   from - 1,
		"longer than ten minutes": from + subtitleWindowMax.Milliseconds() + 1,
	} {
		_, err := s.channelSubtitles(t.Context(), &channelSubtitlesInput{ID: "ch1", From: from, To: to})
		var status huma.StatusError
		if !errors.As(err, &status) || status.GetStatus() != http.StatusBadRequest {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}
}

func TestChannelSubtitlesMissingChannel(t *testing.T) {
	s := &Handlers{playoutResolver: &subtitleResolver{err: fmt.Errorf("read: %w", store.ErrNotFound)}}
	_, err := s.channelSubtitles(t.Context(), &channelSubtitlesInput{ID: "gone"})
	var status huma.StatusError
	if !errors.As(err, &status) || status.GetStatus() != http.StatusNotFound {
		t.Fatalf("err = %v, want 404", err)
	}
}

// With no resolver (an install with no playout), empty lists rather than null or an error.
func TestChannelSubtitlesWithoutPlayout(t *testing.T) {
	out, err := (&Handlers{}).channelSubtitles(t.Context(), &channelSubtitlesInput{ID: "ch1"})
	if err != nil || out.Body.Tracks == nil || out.Body.Cues == nil || len(out.Body.Tracks)+len(out.Body.Cues) != 0 {
		t.Fatalf("out = %+v, err = %v; want empty lists", out, err)
	}
}
