// Package subtitles serves a channel's soft subtitles beside its stream (#2088 decision 5): the
// text subtitle tracks the programme on air carries, and the chosen track's cues on the channel's
// wall clock. Not an HLS rendition: a player reads the master playlist once per tune, while a
// channel's languages change programme by programme. Each track is read once, as WebVTT, by a
// low-priority ffmpeg run, and kept; the schedule only shifts it.
//
// Text formats only. An image format (PGS, VOBSUB) is a picture per cue, which can't become text
// cheaply, so it isn't offered.
package subtitles

import (
	"slices"
	"strings"
	"time"
)

// textCodecs are the subtitle codecs, as ffprobe and the Library's header reader name them, that
// ffmpeg turns into WebVTT.
var textCodecs = []string{"subrip", "ass", "ssa", "webvtt", "mov_text", "text"}

// IsText reports whether a subtitle codec is text a viewer can be offered.
func IsText(codec string) bool { return slices.Contains(textCodecs, strings.ToLower(codec)) }

// Track is one subtitle track a viewer can pick for the programme on air.
type Track struct {
	// Key names the choice across programmes: the language ("und" when untagged), then "-sdh" for
	// a track with sounds described and "-forced" for a forced one. A viewer's remembered choice is
	// a key, so the next programme's English track is picked as this one's was.
	Key string
	// Language is the track's language as the Library gives it (a two-letter code where one
	// exists); empty when the file leaves it untagged.
	Language string
	// Title is the track's own title, when the file gives one.
	Title           string
	Forced          bool
	HearingImpaired bool
	// Source is where the track's text is read from.
	Source Source
}

// Source is a text subtitle track's file and stream: what is extracted, and its cache key.
type Source struct {
	// Input is the file ffmpeg reads: the programme's file, or a subtitle file beside it.
	Input string
	// Stream is the track's stream index in Input, as ffprobe numbers every stream; -1 for a
	// subtitle file's only track.
	Stream int
	// Size and ModifiedNs are Input's stat, so a replaced file is read again.
	Size, ModifiedNs int64
}

// KeyOf is a track's choice key (Track.Key).
func KeyOf(language string, hearingImpaired, forced bool) string {
	key := strings.ToLower(strings.TrimSpace(language))
	if key == "" {
		key = "und"
	}
	if hearingImpaired {
		key += "-sdh"
	}
	if forced {
		key += "-forced"
	}
	return key
}

// Pick is the track a choice key plays on this programme: the track with that key, else the first
// unforced track in the key's language (a remembered "en-sdh" plays plain English where a
// programme has no SDH track), else none.
func Pick(tracks []Track, key string) (Track, bool) {
	if key == "" {
		return Track{}, false
	}
	for _, t := range tracks {
		if t.Key == key {
			return t, true
		}
	}
	language, _, _ := strings.Cut(key, "-")
	for _, t := range tracks {
		if !t.Forced && KeyOf(t.Language, false, false) == language {
			return t, true
		}
	}
	return Track{}, false
}

// Window is what a player shows over a stretch of a channel's wall clock.
type Window struct {
	// Tracks are the text tracks of the programme on air at the window's start; empty for a break,
	// for nothing on air, and for a programme with none.
	Tracks []Track
	// Cues are the chosen track's cues in the window, in order, across every programme part in it.
	Cues []Timed
	// Pending is true while a track in the window is still being read: ask again shortly.
	Pending bool
}

// Timed is a cue on the channel's wall clock.
type Timed struct {
	Start, End time.Time
	Text       string
}

// Place puts a programme part's cues on the wall clock. The part airs from `at`, where the media is
// at `offset`, until `until`; a cue at media time c airs at at+(c−offset). A cue is clipped to the
// part, and one wholly outside it (in a cut for a break, or before the window) is dropped.
func Place(cues []Cue, offset time.Duration, at, until time.Time) []Timed {
	placed := []Timed{}
	for _, c := range cues {
		start, end := at.Add(c.Start-offset), at.Add(c.End-offset)
		if start.Before(at) {
			start = at
		}
		if end.After(until) {
			end = until
		}
		if end.After(start) {
			placed = append(placed, Timed{Start: start, End: end, Text: c.Text})
		}
	}
	return placed
}
