package api

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/langcode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/store"
)

// The subtitles sidecar (#2088 step 7): soft subtitles travel beside the stream, because a live
// channel's master playlist is read once per tune while subtitle languages change programme by
// programme (playout/subtitles.go).

// subtitleAhead is how far past live a window may reach: the player reads a little ahead of its
// playhead so a cue is in hand before it shows.
const subtitleAhead = 2 * time.Minute

// maxSubtitleWindow bounds one request's span.
const maxSubtitleWindow = 10 * time.Minute

type channelSubtitlesInput struct {
	ID       string `path:"id" example:"ch_abc123"`
	From     int64  `query:"from" required:"true" doc:"Window start, Unix milliseconds."`
	To       int64  `query:"to" required:"true" doc:"Window end (exclusive), Unix milliseconds."`
	Language string `query:"language" doc:"The track to read cues from, by ISO 639-2 language; omitted lists tracks only."`
	Title    string `query:"title" doc:"With language: the track's title, preferred over the language's first track."`
}

// SubtitleTrackDTO is one text subtitle track an airing's programme carries.
type SubtitleTrackDTO struct {
	Language     string `json:"language" doc:"ISO 639-2 code, lowercased; empty when the media left it untagged."`
	LanguageName string `json:"languageName" doc:"The language named in itself ('Español'), as the player shows it; empty when the code names no one language."`
	Title        string `json:"title" doc:"The stream's title when the media carries one; empty otherwise."`
}

// SubtitleCueDTO is one cue on the wall clock.
type SubtitleCueDTO struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text" doc:"WebVTT cue text: lines joined by a newline, with <i>/<b> kept."`
}

// SubtitleAiringDTO is one airing in the window.
type SubtitleAiringDTO struct {
	StartMs int64              `json:"startMs"`
	StopMs  int64              `json:"stopMs"`
	Tracks  []SubtitleTrackDTO `json:"tracks"`
	Cues    []SubtitleCueDTO   `json:"cues"`
}

type channelSubtitlesOutput struct {
	Body struct {
		Airings []SubtitleAiringDTO `json:"airings"`
	}
}

func (s *Server) channelSubtitles(ctx context.Context, in *channelSubtitlesInput) (*channelSubtitlesOutput, error) {
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	out := &channelSubtitlesOutput{}
	out.Body.Airings = []SubtitleAiringDTO{}
	from, to, ok := subtitleWindow(time.UnixMilli(in.From), time.UnixMilli(in.To), time.Now())
	if !ok || s.playoutResolver == nil {
		return out, nil
	}
	var pick *playout.SubtitlePick
	if in.Language != "" {
		pick = &playout.SubtitlePick{Language: in.Language, Title: in.Title}
	}
	airings, err := s.playoutResolver.Subtitles(ctx, in.ID, from, to, pick)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return out, nil // a lineup hiccup must not break the player: no subtitles this poll
	}
	for _, a := range airings {
		dto := SubtitleAiringDTO{StartMs: a.Start.UnixMilli(), StopMs: a.Stop.UnixMilli(),
			Tracks: []SubtitleTrackDTO{}, Cues: []SubtitleCueDTO{}}
		for _, t := range a.Tracks {
			dto.Tracks = append(dto.Tracks, SubtitleTrackDTO{Language: t.Language,
				LanguageName: langcode.SelfName(t.Language), Title: t.Title})
		}
		for _, c := range a.Cues {
			dto.Cues = append(dto.Cues, SubtitleCueDTO{StartMs: c.Start.UnixMilli(), EndMs: c.End.UnixMilli(), Text: c.Text})
		}
		out.Body.Airings = append(out.Body.Airings, dto)
	}
	return out, nil
}

// subtitleWindow clamps a request to what a player can be watching: the seekable past
// (DVRHorizon) to a little ahead of live, at most maxSubtitleWindow long. An empty result is ok=false.
func subtitleWindow(from, to, now time.Time) (time.Time, time.Time, bool) {
	if earliest := now.Add(-playout.DVRHorizon - time.Minute); from.Before(earliest) {
		from = earliest
	}
	if latest := now.Add(subtitleAhead); to.After(latest) {
		to = latest
	}
	if to.Sub(from) > maxSubtitleWindow {
		to = from.Add(maxSubtitleWindow)
	}
	return from, to, to.After(from)
}
