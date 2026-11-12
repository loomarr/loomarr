package subtitles

import (
	"testing"
	"time"
)

func TestIsText(t *testing.T) {
	for codec, want := range map[string]bool{
		"subrip": true, "ASS": true, "webvtt": true, "mov_text": true,
		"hdmv_pgs_subtitle": false, "dvd_subtitle": false, "": false,
	} {
		if got := IsText(codec); got != want {
			t.Errorf("IsText(%q) = %v, want %v", codec, got, want)
		}
	}
}

func TestKeyOf(t *testing.T) {
	for _, tc := range []struct {
		language   string
		sdh, force bool
		want       string
	}{
		{"en", false, false, "en"},
		{" EN ", true, false, "en-sdh"},
		{"", false, true, "und-forced"},
		{"fr", true, true, "fr-sdh-forced"},
	} {
		if got := KeyOf(tc.language, tc.sdh, tc.force); got != tc.want {
			t.Errorf("KeyOf(%q, %v, %v) = %q, want %q", tc.language, tc.sdh, tc.force, got, tc.want)
		}
	}
}

func TestPick(t *testing.T) {
	track := func(language string, sdh, forced bool) Track {
		return Track{Key: KeyOf(language, sdh, forced), Language: language, HearingImpaired: sdh, Forced: forced}
	}
	tracks := []Track{track("en", false, true), track("en", true, false), track("fr", false, false)}
	for _, tc := range []struct {
		key, want string
		ok        bool
	}{
		{"en-sdh", "en-sdh", true},
		{"fr", "fr", true},
		// No plain English here: the language's first unforced track, never the forced one.
		{"en", "en-sdh", true},
		{"fr-sdh", "fr", true},
		{"de", "", false},
		{"", "", false},
	} {
		got, ok := Pick(tracks, tc.key)
		if ok != tc.ok || got.Key != tc.want {
			t.Errorf("Pick(%q) = %q, %v; want %q, %v", tc.key, got.Key, ok, tc.want, tc.ok)
		}
	}
}

func TestPlace(t *testing.T) {
	at := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	until := at.Add(time.Minute)
	// The part airs from the media's 10-minute mark.
	offset := 10 * time.Minute
	cues := []Cue{
		{Start: 9 * time.Minute, End: 9*time.Minute + time.Second, Text: "before the part"},
		{Start: 10*time.Minute - time.Second, End: 10*time.Minute + 2*time.Second, Text: "straddles the start"},
		{Start: 10*time.Minute + 30*time.Second, End: 10*time.Minute + 32*time.Second, Text: "inside"},
		{Start: 10*time.Minute + 59*time.Second, End: 11*time.Minute + 3*time.Second, Text: "runs past the end"},
		{Start: 12 * time.Minute, End: 12*time.Minute + time.Second, Text: "after the part"},
	}
	got := Place(cues, offset, at, until)
	want := []Timed{
		{Start: at, End: at.Add(2 * time.Second), Text: "straddles the start"},
		{Start: at.Add(30 * time.Second), End: at.Add(32 * time.Second), Text: "inside"},
		{Start: at.Add(59 * time.Second), End: until, Text: "runs past the end"},
	}
	if len(got) != len(want) {
		t.Fatalf("Place = %+v, want %+v", got, want)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) || got[i].Text != want[i].Text {
			t.Errorf("cue %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
