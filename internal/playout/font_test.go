package playout

import (
	"testing"
)

// parseHasFilter must read the NAME column of `ffmpeg -filters`: a build can carry a font file
// but not the drawtext filter, and a substring match would call that a pass.

// Real `ffmpeg -filters` rows, verbatim from ffmpeg 8.1.2 — flags, name, signature, description.
const filtersWithDrawText = `Filters:
  T.. colorhold          V->V       Turns a certain color range into gray.
 TS. drawtext            V->V       Draw text on top of video frames.
 ... anullsrc            |->A       Null audio source, return empty audio frames.
`

// The same build minus drawtext. `drawbox` stays: it shares a prefix, so a substring match
// would call this a pass.
const filtersWithoutDrawText = `Filters:
  T.. colorhold          V->V       Turns a certain color range into gray.
 T.. drawbox             V->V       Draw a colored box on the input video.
 ... anullsrc            |->A       Null audio source, return empty audio frames.
`

func TestParseHasDrawText_ReadsTheNameColumn(t *testing.T) {
	if !parseHasFilter([]byte(filtersWithDrawText), "drawtext") {
		t.Error("drawtext is listed in this build and was not found")
	}
	if parseHasFilter([]byte(filtersWithoutDrawText), "drawtext") {
		t.Error("drawtext is absent from this build and was reported present")
	}
}

// The name must match the NAME COLUMN, not the line. `-filters` carries a prose description per
// row, and a filter whose description merely mentions drawtext is not drawtext.
func TestParseHasDrawText_IgnoresTheDescriptionColumn(t *testing.T) {
	mentionOnly := ` T.. subtitles  V->V  Render text subtitles, unlike drawtext, onto the video.
`
	if parseHasFilter([]byte(mentionOnly), "drawtext") {
		t.Error("matched the description column — a substring search, not a column match")
	}
}

func TestParseHasDrawText_EmptyOutputIsNotAYes(t *testing.T) {
	if parseHasFilter(nil, "drawtext") {
		t.Error("no output must never resolve to 'the filter is present'")
	}
}
