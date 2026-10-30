package playout

import (
	"testing"
	"time"
)

func TestParseWebVTT_ReadsCuesAndSkipsHeaderBlocks(t *testing.T) {
	vtt := "WEBVTT\r\n\r\nNOTE made by ffmpeg\r\n\r\nSTYLE\r\n::cue { color: red }\r\n\r\n" +
		"1\r\n00:01:02.500 --> 00:01:04.000 align:start\r\n<i>Hello</i>\r\nthere\r\n\r\n" +
		"65:00.250 --> 65:01.000\r\nNo hours\r\n"
	cues, err := ParseWebVTT([]byte(vtt))
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleCue{
		{Start: 62500 * time.Millisecond, End: 64 * time.Second, Text: "<i>Hello</i>\nthere"},
		{Start: 65*time.Minute + 250*time.Millisecond, End: 65*time.Minute + time.Second, Text: "No hours"},
	}
	if len(cues) != len(want) {
		t.Fatalf("cues = %+v, want %+v", cues, want)
	}
	for i := range want {
		if cues[i] != want[i] {
			t.Errorf("cue %d = %+v, want %+v", i, cues[i], want[i])
		}
	}
}

func TestParseWebVTT_RejectsABadTimestamp(t *testing.T) {
	if _, err := ParseWebVTT([]byte("WEBVTT\n\n00:01.5 --> 00:02.000\nx\n")); err == nil {
		t.Fatal("a two-digit fraction parsed")
	}
}

func TestPickSubtitle_PrefersLanguageAndTitleThenLanguage(t *testing.T) {
	tracks := []Track{{Language: "eng"}, {Language: "eng", Title: "SDH"}, {Language: "spa"}}
	for _, tc := range []struct {
		pick SubtitlePick
		want int
	}{
		{SubtitlePick{Language: "eng", Title: "SDH"}, 1},
		{SubtitlePick{Language: "eng", Title: "Forced"}, 0},
		{SubtitlePick{Language: "spa"}, 2},
		{SubtitlePick{Language: "fre"}, -1},
	} {
		if got := PickSubtitle(tracks, tc.pick); got != tc.want {
			t.Errorf("PickSubtitle(%+v) = %d, want %d", tc.pick, got, tc.want)
		}
	}
}

func TestIsTextSubtitle_OffersTextNotImages(t *testing.T) {
	for _, c := range []string{"subrip", "ass", "mov_text", "webvtt"} {
		if !IsTextSubtitle(c) {
			t.Errorf("%s is text", c)
		}
	}
	for _, c := range []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", ""} {
		if IsTextSubtitle(c) {
			t.Errorf("%s is not text", c)
		}
	}
}
