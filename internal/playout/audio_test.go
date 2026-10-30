package playout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
	"github.com/loomarr/loomarr/internal/playout/packager"
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

// The alternate (#2088 decision 6): the first track in another known language, never a
// commentary in the default's own language or an untagged track; the default again when there is none.
func TestPickAudioTracks_TheAlternateIsAnotherLanguage(t *testing.T) {
	for _, c := range []struct {
		name   string
		tracks []AudioTrack
		prefer string
		want   AudioPick
	}{
		{"a dub beside the original", []AudioTrack{{Language: "rus"}, {Language: "eng"}}, "eng", AudioPick{1, 0}},
		{"commentary is not a language", []AudioTrack{{Language: "eng"}, {Language: "eng"}, {Language: "spa"}}, "eng", AudioPick{0, 2}},
		{"untagged is not choosable", []AudioTrack{{Language: "eng"}, {}, {Language: "und"}}, "eng", AudioPick{0, 0}},
		{"one language only", []AudioTrack{{Language: "eng"}}, "eng", AudioPick{0, 0}},
		{"no audio", nil, "eng", AudioPick{0, 0}},
		{"tags compare like the default's", []AudioTrack{{Language: "ENG "}, {Language: "eng"}, {Language: "Fra"}}, "eng", AudioPick{0, 2}},
	} {
		if got := PickAudioTracks(c.tracks, c.prefer); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// itemArgsWithTracks is one packaged item's encoder args with an explicit default and alternate.
func itemArgsWithTracks(t *testing.T, track, alt int) []string {
	t.Helper()
	out := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	_, args, err := packagerItemArgs(encode.HostFor(encode.EncoderSoftware, true, encode.GPUFilters{}), out,
		PackagerItem{Input: "http://library.invalid/stream", Format: testSources()["h264-1080p-sdr-25"], AudioTrack: track, AltAudioTrack: alt},
		nil, nil, packager.Slot{Frames: 250, AudioFrames: 469}, itemFault{})
	if err != nil {
		t.Fatal(err)
	}
	return args
}

func itemArgsWithTrack(t *testing.T, track int) []string { return itemArgsWithTracks(t, track, track) }

// The invariant the channel's init depends on, and the reason the selection could not be expressed
// as optional ffmpeg maps: EXACTLY TWO audio tracks, the default and then the alternate, whatever
// was chosen. A varying track count would change the init and break -c copy.
func TestItemArgsWithAudio_MapsTheDefaultThenTheAlternate(t *testing.T) {
	for _, c := range []struct{ track, alt int }{{0, 0}, {1, 0}, {3, 1}} {
		args := itemArgsWithTracks(t, c.track, c.alt)

		var audioMaps []string
		for i, a := range args {
			if a == "-map" && i+1 < len(args) && strings.HasPrefix(args[i+1], "0:a:") {
				audioMaps = append(audioMaps, args[i+1])
			}
		}
		want := []string{"0:a:" + strconv.Itoa(c.track), "0:a:" + strconv.Itoa(c.alt)}
		if strings.Join(audioMaps, " ") != strings.Join(want, " ") {
			t.Errorf("tracks %+v: mapped %v, want %v", c, audioMaps, want)
		}
	}
}

// An item with no preference maps the first track.
func TestItemArgs_DefaultsToTheFirstAudioTrack(t *testing.T) {
	args := itemArgsWithTrack(t, 0)
	if !containsPair(args, "-map", "0:a:0") {
		t.Fatalf("item args did not map 0:a:0; args=%v", args)
	}
}

// Video mapping is untouched by the audio work — the one thing most likely to be broken by a
// careless edit to the same line.
func TestItemArgsWithAudio_StillMapsTheFirstVideoStream(t *testing.T) {
	args := itemArgsWithTrack(t, 2)
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
