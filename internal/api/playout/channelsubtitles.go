package playout

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// GET /v1/channels/{id}/subtitles — a channel's soft subtitles, beside its stream (#2088 decision 5).
//
// Not an HLS rendition: a player reads the master playlist once per tune, while a channel's
// subtitle languages change programme by programme. So the player asks for a stretch of the
// channel's wall clock (where its playhead is, a little ahead) and gets the tracks of the
// programme on air and the chosen track's cues on that clock, which it draws over the picture.

const (
	// subtitleWindowDefault is the stretch answered when the player names no end.
	subtitleWindowDefault = 2 * time.Minute
	// subtitleWindowMax bounds a window, so one ask never walks a long stretch of the schedule.
	subtitleWindowMax = 10 * time.Minute
)

func (s *Handlers) registerChannelSubtitles(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-subtitles", Method: http.MethodGet, Path: "/v1/channels/{id}/subtitles",
		Summary: "Subtitles of what a channel airs over a stretch of its clock",
		Description: "Any signed-in user. The text subtitle tracks of the programme on air at `from`, and, when `track` " +
			"names one, that track's cues from `from` to `to` on the channel's wall clock, across every programme part in " +
			"the stretch (a break has none). A track is read from the media once; until it has been, `pending` is true " +
			"and the player asks again shortly. Image subtitles (PGS, VOBSUB) are not offered.",
		Tags: []string{"channels"},
	}, httpcore.RoleMember), s.channelSubtitles)
}

type channelSubtitlesInput struct {
	ID string `path:"id" example:"ch_abc123"`
	// From and To are Unix milliseconds on the channel's wall clock (the stream's programme date
	// time). Zero From is now; zero To is From plus two minutes.
	From int64 `query:"from" minimum:"0" doc:"Start of the stretch, Unix milliseconds; 0 or absent for now"`
	To   int64 `query:"to" minimum:"0" doc:"End of the stretch, Unix milliseconds; 0 or absent for two minutes after from. At most ten minutes after from"`
	// Track is a choice key from a track list ("en", "en-sdh"). A programme without that track
	// plays the key's language's first unforced track, else none.
	Track string `query:"track" maxLength:"40" pattern:"^[a-z0-9-]*$" doc:"The chosen track's key, as tracks list it; absent for the tracks alone"`
}

// SubtitleTrackDTO is one text subtitle track a viewer can pick.
type SubtitleTrackDTO struct {
	Key             string `json:"key" doc:"Names the choice across programmes: the language ('und' when untagged), then -sdh and -forced"`
	Language        string `json:"language" doc:"The track's language, a two-letter code where one exists; empty when untagged"`
	Title           string `json:"title" doc:"The track's own title, when the file gives one"`
	Forced          bool   `json:"forced" doc:"A forced track: only the lines in another language"`
	HearingImpaired bool   `json:"hearingImpaired" doc:"Sounds described too (SDH)"`
}

// SubtitleCueDTO is one cue on the channel's wall clock.
type SubtitleCueDTO struct {
	StartMs int64  `json:"startMs" doc:"When it shows, Unix milliseconds on the channel's clock"`
	EndMs   int64  `json:"endMs" doc:"When it goes, Unix milliseconds"`
	Text    string `json:"text" doc:"Plain text; a newline separates its lines"`
}

type channelSubtitlesOutput struct {
	Body struct {
		Tracks  []SubtitleTrackDTO `json:"tracks" doc:"Text tracks of the programme on air at from; empty for a break or none"`
		Cues    []SubtitleCueDTO   `json:"cues" doc:"The chosen track's cues in the stretch, in order"`
		Pending bool               `json:"pending" doc:"A track in the stretch is still being read; ask again shortly"`
	}
}

func (s *Handlers) channelSubtitles(ctx context.Context, in *channelSubtitlesInput) (*channelSubtitlesOutput, error) {
	from := time.UnixMilli(in.From)
	if in.From == 0 {
		from = time.Now()
	}
	to := time.UnixMilli(in.To)
	if in.To == 0 {
		to = from.Add(subtitleWindowDefault)
	}
	if !to.After(from) || to.Sub(from) > subtitleWindowMax {
		return nil, httpcore.BadRequest("Ask for a shorter stretch",
			"`to` must be after `from`, and at most ten minutes after it.")
	}

	out := &channelSubtitlesOutput{}
	out.Body.Tracks = []SubtitleTrackDTO{}
	out.Body.Cues = []SubtitleCueDTO{}
	if s.playoutResolver == nil {
		return out, nil
	}
	window, err := s.playoutResolver.Subtitles(ctx, in.ID, from, to, in.Track)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	for _, t := range window.Tracks {
		out.Body.Tracks = append(out.Body.Tracks, SubtitleTrackDTO{
			Key: t.Key, Language: t.Language, Title: t.Title, Forced: t.Forced, HearingImpaired: t.HearingImpaired,
		})
	}
	for _, c := range window.Cues {
		out.Body.Cues = append(out.Body.Cues, SubtitleCueDTO{StartMs: c.Start.UnixMilli(), EndMs: c.End.UnixMilli(), Text: c.Text})
	}
	out.Body.Pending = window.Pending
	return out, nil
}
