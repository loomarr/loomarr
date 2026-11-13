package playoutresolve

import (
	"context"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/subtitles"
)

// maxSubtitleParts bounds the programme parts and breaks one window walks. A window is minutes
// long, so this is only a guard against a lineup of zero-length slots.
const maxSubtitleParts = 64

// LibrarySubtitleReader is the store's read of a file's subtitle tracks.
type LibrarySubtitleReader interface {
	LibrarySubtitleTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) ([]librarystore.LibraryStream, bool, error)
}

// SubtitleCues answers a track's cues once read (subtitles.Extractor).
type SubtitleCues interface {
	Cues(src subtitles.Source) ([]subtitles.Cue, subtitles.State)
}

// SetSubtitles attaches the Library's subtitle tracks and the reader of their cues. Until it is
// set, every window is empty.
func (r *Resolver) SetSubtitles(tracks LibrarySubtitleReader, cues SubtitleCues) {
	r.librarySubtitles = tracks
	r.subtitleCues = cues
}

// Subtitles is what a player shows from `from` to `to` on a channel's wall clock (#2088 decision
// 5): the text tracks of the programme on air at `from`, and the cues of the track `key` picks in
// each programme part of the window, placed on the wall clock. It walks the accepted cycle the
// encoder airs, so a cue lands where the encoder put its picture; a break has none, and a cue cut
// by a break never airs.
//
// Best-effort past the channel read: a programme whose file or tracks can't be read has none.
func (r *Resolver) Subtitles(ctx context.Context, channelID string, from, to time.Time, key string) (subtitles.Window, error) {
	window := subtitles.Window{Tracks: []subtitles.Track{}, Cues: []subtitles.Timed{}}
	slots, epoch, err := r.acceptedCycle(ctx, channelID, from)
	if err != nil {
		return window, err
	}
	if r.librarySubtitles == nil || r.libraryFile == nil {
		return window, nil
	}
	at := from
	for part := 0; at.Before(to) && part < maxSubtitleParts; part++ {
		airing := playout.AiringAt(slots, epoch, at)
		if airing.Remaining <= 0 {
			break // nothing airable in the cycle
		}
		// A movie in parts is walked a part at a time: each file has its own tracks and its own
		// clock.
		step := airing.Remaining
		var input programmeInput
		if airing.Kind == schedule.SlotProgram && airing.LibraryItemID != "" {
			input = r.programmeInput(ctx, airing.LibraryItemID, airing.Offset)
			if input.End > 0 {
				step = min(step, input.End-airing.Offset)
			}
		}
		until := at.Add(step)
		if until.After(to) {
			until = to
		}
		if input.Path != "" {
			tracks := r.textSubtitles(ctx, airing.LibraryItemID, input.Path)
			if part == 0 {
				window.Tracks = tracks
			}
			if track, ok := subtitles.Pick(tracks, key); ok && r.subtitleCues != nil {
				switch cues, state := r.subtitleCues.Cues(track.Source); state {
				case subtitles.Ready:
					window.Cues = append(window.Cues, subtitles.Place(cues, airing.Offset-input.Start, at, until)...)
				case subtitles.Pending:
					window.Pending = true
				case subtitles.Failed:
				}
			}
		}
		at = at.Add(step)
	}
	return window, nil
}

// textSubtitles is the text subtitle tracks of path, a file library item itemID plays, as the
// Library lists them, one per choice key (the first wins). An image track isn't offered.
func (r *Resolver) textSubtitles(ctx context.Context, itemID, path string) []subtitles.Track {
	tracks := []subtitles.Track{}
	info, err := os.Stat(path)
	if err != nil {
		return tracks
	}
	streams, ok, err := r.librarySubtitles.LibrarySubtitleTracks(ctx, path, info.Size(), info.ModTime().UnixNano())
	if err != nil || !ok {
		if err != nil && r.log != nil {
			r.log.Debug("playout: subtitle tracks unavailable", "item", itemID, "err", err)
		}
		return tracks
	}
	seen := map[string]bool{}
	for _, s := range streams {
		if !subtitles.IsText(s.Codec) {
			continue
		}
		key := subtitles.KeyOf(s.Language, s.HearingImpaired, s.Forced)
		if seen[key] {
			continue
		}
		src := subtitles.Source{Input: path, Stream: s.Index, Size: info.Size(), ModifiedNs: info.ModTime().UnixNano()}
		if s.ExternalRef != "" {
			beside, err := os.Stat(s.ExternalRef)
			if err != nil {
				continue // the file beside it has gone since the scan
			}
			src = subtitles.Source{Input: s.ExternalRef, Stream: -1, Size: beside.Size(), ModifiedNs: beside.ModTime().UnixNano()}
		}
		seen[key] = true
		tracks = append(tracks, subtitles.Track{
			Key: key, Language: s.Language, Title: s.Title, Forced: s.Forced, HearingImpaired: s.HearingImpaired, Source: src,
		})
	}
	return tracks
}
