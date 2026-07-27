package playout

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// The bug this file exists for, stated as a test: a release whose Russian dub is written before
// its English track played in Russian, because the args hardcoded `-map 0:a:0`.
func TestPickAudioTrack_PrefersTheStatedLanguageOverTheFirstTrack(t *testing.T) {
	tracks := []AudioTrack{{Language: "rus"}, {Language: "eng"}}
	if got := PickAudioTrack(tracks, "eng"); got != 1 {
		t.Fatalf("picked audio track %d, want 1 (the eng track, not the first one)", got)
	}
}

// The fallback half, and the reason this is a preference rather than a requirement: a film with
// no track in the preferred language must still play.
func TestPickAudioTrack_FallsBackToTheFirstTrackWhenTheLanguageIsAbsent(t *testing.T) {
	tracks := []AudioTrack{{Language: "rus"}, {Language: "fra"}}
	if got := PickAudioTrack(tracks, "eng"); got != 0 {
		t.Fatalf("picked audio track %d, want 0 — no eng track means play the first, never nothing", got)
	}
}

// Untagged tracks are common and must not be read as a mismatch that selects something worse.
func TestPickAudioTrack_UntaggedTracksFallBackToTheFirst(t *testing.T) {
	if got := PickAudioTrack([]AudioTrack{{}, {}}, "eng"); got != 0 {
		t.Fatalf("picked audio track %d, want 0 for an entirely untagged file", got)
	}
}

// Clearing the setting restores ffmpeg's historical behaviour — an operator who wants the old
// selection can have it, and that has to keep working.
func TestPickAudioTrack_NoPreferenceKeepsTheFirstTrack(t *testing.T) {
	tracks := []AudioTrack{{Language: "rus"}, {Language: "eng"}}
	if got := PickAudioTrack(tracks, ""); got != 0 {
		t.Fatalf("picked audio track %d, want 0 when no preference is set", got)
	}
}

// Container tags are written by hundreds of tools; "ENG" and "eng " are the same language.
func TestPickAudioTrack_MatchesCaseInsensitivelyAndTrimsSpace(t *testing.T) {
	for _, tag := range []string{"ENG", " eng", "Eng "} {
		tracks := []AudioTrack{{Language: "rus"}, {Language: tag}}
		if got := PickAudioTrack(tracks, "eng"); got != 1 {
			t.Errorf("tag %q: picked audio track %d, want 1", tag, got)
		}
	}
}

// A file with no audio at all must still yield a valid index rather than panicking or -1.
func TestPickAudioTrack_EmptyTrackListIsZero(t *testing.T) {
	if got := PickAudioTrack(nil, "eng"); got != 0 {
		t.Fatalf("picked audio track %d, want 0 for a file with no audio streams", got)
	}
}

// The invariant the parent's `-c copy` depends on, and the reason the selection could not be
// expressed as two ffmpeg maps: EXACTLY ONE audio track, whatever was chosen.
func TestProgramArgsWithAudio_MapsExactlyOneAudioTrack(t *testing.T) {
	for _, track := range []int{0, 1, 3} {
		args := ProgramArgsWithAudio(DefaultProfile(), testStreamURL, 0, time.Hour, track)

		var audioMaps []string
		for i, a := range args {
			if a == "-map" && i+1 < len(args) && strings.HasPrefix(args[i+1], "0:a:") {
				audioMaps = append(audioMaps, args[i+1])
			}
		}
		if len(audioMaps) != 1 {
			t.Fatalf("track %d: %d audio maps %v, want exactly 1 — a varying track count breaks -c copy",
				track, len(audioMaps), audioMaps)
		}
		want := "0:a:" + strconv.Itoa(track)
		if audioMaps[0] != want {
			t.Errorf("track %d: mapped %q, want %q", track, audioMaps[0], want)
		}
	}
}

// The wrapper must keep the pre-existing behaviour for every caller that has no preference.
func TestProgramArgs_DefaultsToTheFirstAudioTrack(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour)
	if !containsPair(args, "-map", "0:a:0") {
		t.Fatalf("ProgramArgs did not map 0:a:0; args=%v", args)
	}
}

// Video mapping is untouched by the audio work — the one thing most likely to be broken by a
// careless edit to the same line.
func TestProgramArgsWithAudio_StillMapsTheFirstVideoStream(t *testing.T) {
	args := ProgramArgsWithAudio(DefaultProfile(), testStreamURL, 0, time.Hour, 2)
	if !containsPair(args, "-map", "0:v:0") {
		t.Fatalf("video map missing or changed; args=%v", args)
	}
}

func containsPair(args []string, flag, val string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}
	return false
}
