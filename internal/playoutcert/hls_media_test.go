package playoutcert

import (
	"strings"
	"testing"
	"time"
)

func TestParsePreparedHLSAcceptsDateTimeAfterDuration(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1.25,\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T12:00:00.125+0000\na.m4s\n"
	segments, err := parsePreparedHLS([]byte(body))
	if err != nil || len(segments) != 1 {
		t.Fatalf("valid production tag ordering rejected: %v", err)
	}
	want := time.Date(2026, 9, 8, 12, 0, 0, 125000000, time.UTC)
	if !segments[0].startedAt.Equal(want) || segments[0].duration != 1250*time.Millisecond {
		t.Fatalf("segment coordinates = %+v", segments[0])
	}
}

func TestParseHLSMediaBindsTransportSegmentsWithoutInventingMap(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:41\n#EXTINF:1.04,\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T08:00:00.125-0400\na.ts?signature=x\n#EXTINF:0.96,\nb.ts?signature=x\n#EXT-X-DISCONTINUITY\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T12:00:02.125Z\n#EXTINF:1,\nc.ts?signature=x\n"
	segments, err := parseHLSMedia([]byte(body), false)
	if err != nil || len(segments) != 3 {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
	start := time.Date(2026, 9, 8, 12, 0, 0, 125000000, time.UTC)
	if segments[0].init != "" || segments[0].id != "41" || segments[1].id != "42" || segments[2].id != "43" || segments[2].boundary != "1" {
		t.Fatalf("transport identity=%+v", segments)
	}
	if !segments[0].startedAt.Equal(start) || !segments[1].startedAt.Equal(start.Add(1040*time.Millisecond)) || !segments[2].startedAt.Equal(start.Add(2*time.Second)) {
		t.Fatalf("transport time intervals=%+v", segments)
	}
	if _, err := parsePreparedHLS([]byte(body)); err == nil {
		t.Fatal("prepared-only parser accepted a missing initialization map")
	}
}

func TestParseHLSMediaRejectsAmbiguousCoordinates(t *testing.T) {
	const prefix = "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T12:00:00Z\n"
	for name, body := range map[string]string{
		"missing header":               strings.TrimPrefix(prefix, "#EXTM3U\n") + "#EXTINF:1,\na.ts\n",
		"missing initial anchor":       "#EXTM3U\n#EXTINF:1,\na.ts\n",
		"missing discontinuity anchor": prefix + "#EXTINF:1,\na.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:1,\nb.ts\n",
		"invalid date":                 "#EXTM3U\n#EXTINF:1,\n#EXT-X-PROGRAM-DATE-TIME:no\na.ts\n",
		"missing URI":                  prefix + "#EXTINF:1,\n",
		"missing duration":             prefix + "a.ts\n",
		"duplicate duration":           prefix + "#EXTINF:1,\n#EXTINF:2,\na.ts\n",
		"zero duration":                prefix + "#EXTINF:0,\na.ts\n",
		"negative duration":            prefix + "#EXTINF:-1,\na.ts\n",
		"nan duration":                 prefix + "#EXTINF:NaN,\na.ts\n",
		"infinite duration":            prefix + "#EXTINF:Inf,\na.ts\n",
		"overflow duration":            prefix + "#EXTINF:99999999999999999,\na.ts\n",
		"duplicate sequence":           prefix + "#EXT-X-MEDIA-SEQUENCE:1\n#EXT-X-MEDIA-SEQUENCE:2\n#EXTINF:1,\na.ts\n",
		"late sequence":                prefix + "#EXTINF:1,\na.ts\n#EXT-X-MEDIA-SEQUENCE:2\n#EXTINF:1,\nb.ts\n",
		"overflow sequence":            prefix + "#EXT-X-MEDIA-SEQUENCE:9223372036854775807\n#EXTINF:1,\na.ts\n#EXTINF:1,\nb.ts\n",
		"late epoch sequence":          prefix + "#EXT-X-DISCONTINUITY\n#EXT-X-DISCONTINUITY-SEQUENCE:2\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T12:00:00Z\n#EXTINF:1,\na.ts\n",
		"overflow epoch":               prefix + "#EXT-X-DISCONTINUITY-SEQUENCE:9223372036854775807\n#EXT-X-DISCONTINUITY\n#EXTINF:1,\na.ts\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseHLSMedia([]byte(body), false); err == nil {
				t.Fatal("accepted malformed segment coordinates")
			}
		})
	}
}

func TestParseHLSMediaRejectsUnsafeMediaAndMapReferences(t *testing.T) {
	for _, reference := range []string{"https://example.test/a.ts", "//example.test/a.ts", "../a.ts", "%2e%2e/a.ts", "a.ts#fragment", "?signature=x"} {
		for _, mapReference := range []bool{false, true} {
			body := "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-09-08T12:00:00Z\n"
			if mapReference {
				body += "#EXT-X-MAP:URI=\"" + reference + "\"\n#EXTINF:1,\na.m4s\n"
			} else {
				body += "#EXTINF:1,\n" + reference + "\n"
			}
			if _, err := parseHLSMedia([]byte(body), mapReference); err == nil {
				t.Fatalf("accepted unsafe reference=%q map=%v", reference, mapReference)
			}
		}
	}
}
