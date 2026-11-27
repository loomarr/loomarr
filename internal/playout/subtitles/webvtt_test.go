package subtitles

import (
	"testing"
	"time"
)

func TestParseWebVTT(t *testing.T) {
	doc := "\xEF\xBB\xBFWEBVTT\n\nNOTE written by ffmpeg\n\n" +
		"1\n00:00:01.000 --> 00:00:03.500 align:start\n<i>Hello</i> &amp; welcome\r\nback\n\n" +
		"01:02.250 --> 01:04.000\nShort form\n\n" +
		"01:00:00.000 --> 01:00:01.000\n<c.yellow></c>\n\n" +
		"00:00:05.000 --> 00:00:04.000\nEnds before it starts\n"
	cues, err := ParseWebVTT([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{
		{Start: time.Second, End: 3500 * time.Millisecond, Text: "Hello & welcome\nback"},
		{Start: time.Minute + 2250*time.Millisecond, End: time.Minute + 4*time.Second, Text: "Short form"},
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

func TestParseWebVTTRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"no header":       "1\n00:00:01.000 --> 00:00:02.000\nText\n",
		"bad timestamp":   "WEBVTT\n\n00:00:01 --> 00:00:02.000\nText\n",
		"no end":          "WEBVTT\n\n00:00:01.000 -->\nText\n",
		"letters in time": "WEBVTT\n\n00:aa:01.000 --> 00:00:02.000\nText\n",
	} {
		if _, err := ParseWebVTT([]byte(doc)); err == nil {
			t.Errorf("%s: parsed without an error", name)
		}
	}
}

func TestParseWebVTTEmpty(t *testing.T) {
	cues, err := ParseWebVTT([]byte("WEBVTT\n"))
	if err != nil || len(cues) != 0 {
		t.Fatalf("cues = %v, err = %v; want none", cues, err)
	}
}
