package playoutcert

import "testing"

func TestParsePreparedHLSAssociatesMapPDTAndDiscontinuity(t *testing.T) {
	segments, err := parsePreparedHLS([]byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-MAP:URI=\"init-a.mp4?sig=x\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:00Z\n#EXTINF:1,\na.m4s?sig=x\n#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"init-b.mp4?sig=x\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:01Z\n#EXTINF:1,\nb.m4s?sig=x\n"))
	if err != nil || len(segments) != 2 || segments[0].boundary != "0" || segments[1].boundary != "1" || segments[1].init != "init-b.mp4?sig=x" {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
}

func TestParsePreparedHLSRejectsUnboundOrMalformedMetadata(t *testing.T) {
	for _, body := range []string{
		"#EXTM3U\n#EXTINF:1,\na.m4s\n",
		"#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-PROGRAM-DATE-TIME:no\n#EXTINF:1,\na.m4s\n",
		"#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1,\n#bad\n",
	} {
		if _, err := parsePreparedHLS([]byte(body)); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestParsePreparedHLSSegmentIdentityIsAbsoluteSequence(t *testing.T) {
	first, err := parsePreparedHLS([]byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:00Z\n#EXTINF:1,\na.m4s\n#EXTINF:1,\nb.m4s\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := parsePreparedHLS([]byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:8\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-PROGRAM-DATE-TIME:2026-09-07T12:00:01Z\n#EXTINF:1,\nb.m4s\n#EXTINF:1,\nc.m4s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if first[1].id != second[0].id || second[0].id != "8" {
		t.Fatalf("overlap identities first=%+v second=%+v", first, second)
	}
}
