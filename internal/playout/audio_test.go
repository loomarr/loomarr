package playout

import (
	"testing"
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

// One language in two spellings is a match: the Library keeps "en" where the container wrote "eng",
// and an operator may type "EN" or "en-US".
func TestPickAudioTrack_MatchesALanguageWhateverItsSpelling(t *testing.T) {
	tracks := []AudioTrack{{Language: "ru"}, {Language: "en"}}
	for _, prefer := range []string{"eng", "EN", "en-US", "English"} {
		if got := PickAudioTrack(tracks, prefer); got != 1 {
			t.Errorf("prefer %q: picked audio track %d, want 1 (the en track)", prefer, got)
		}
	}
}

// Untagged tracks are common and must not be read as a mismatch that selects something worse.
func TestPickAudioTrack_UntaggedTracksFallBackToTheFirst(t *testing.T) {
	if got := PickAudioTrack([]AudioTrack{{}, {}}, "eng"); got != 0 {
		t.Fatalf("picked audio track %d, want 0 for an entirely untagged file", got)
	}
}

// "Unknown language" on the Audio sheet saves `und` (#3409): it picks the track with no language,
// whether the container left the tag off or wrote `und`, and never a tagged one.
func TestPickAudioTrack_UndPicksTheUntaggedTrack(t *testing.T) {
	for _, tag := range []string{"", "und", "UND"} {
		tracks := []AudioTrack{{Language: "eng"}, {Language: tag}}
		if got := PickAudioTrack(tracks, "und"); got != 1 {
			t.Errorf("untagged as %q: picked audio track %d, want 1", tag, got)
		}
	}
	if got := PickAudioTrack([]AudioTrack{{Language: "fra"}, {Language: "eng"}}, "und"); got != 0 {
		t.Fatalf("no untagged track: picked audio track %d, want the first", got)
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
