package playout

import "context"

// Track discovery for the Watch surface (§9.1, V46) — what audio and subtitle tracks a source
// ACTUALLY carries.
//
// The Watch tab's Audio and Subtitle pickers are populated from real media, not a hardcoded list
// and not an app setting: what a viewer can choose depends on the FILE that is airing (one film
// has English + Russian 5.1, the next has English stereo and French subtitles). So the FE asks the
// backend "what tracks does the current programme have?" and offers exactly those.
//
// This is a superset of what PickAudioTrack needs (which only cares about audio languages, to pick
// one for the shared encode). It stays a separate prober because its CALLER is different — the
// broadcast path probes audio once per programme to choose a track (audio.go), while this serves a
// UI request describing every track for a human to pick from. Merging them would put subtitle
// probing on the broadcast path, which does not need it.

// Track is one audio or subtitle stream as the picker describes it.
type Track struct {
	// Index is the stream's position AMONG ITS OWN TYPE — the `N` a `-map 0:a:N` / `0:s:N` would
	// use — matching PickAudioTrack's contract so a chosen audio track maps the same way the
	// encoder already selects one.
	Index int
	// Language is the ISO 639-2 tag, lowercased; empty when the container left it untagged (common
	// enough that the UI must render it as "Unknown", never hide the track).
	Language string
	// Title is the container's free-text stream title when present ("Director's Commentary",
	// "Forced"), shown beside the language because two English tracks are otherwise indistinct.
	Title string
}

// MediaTracks is a source's selectable tracks, split by type.
type MediaTracks struct {
	Audio     []Track
	Subtitles []Track
}

// TrackProber reports a source's audio + subtitle tracks. An interface so the API handler is
// testable without exec, mirroring AudioProber. The concrete prober (FFprobeTracksNextTo) and the
// stream→tracks derivation (tracksOf) live in probe.go, where all ffprobe use is consolidated.
type TrackProber func(ctx context.Context, input string) (MediaTracks, error)
