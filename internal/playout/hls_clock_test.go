package playout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveHLSManifestUsesTheSessionMediaClock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, hlsPlaylistName)
	raw := "#EXTM3U\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-09-21T19:13:34.793-0400\n" +
		"#EXTINF:4.000000,\nseg-0.ts\n" +
		"#EXT-X-PROGRAM-DATE-TIME:2026-09-21T19:13:38.793-0400\n" +
		"#EXTINF:4.000000,\nseg-1.ts\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	origin := time.Date(2026, 9, 21, 19, 13, 19, 656_000_000, time.FixedZone("EDT", -4*60*60))
	probes := 0
	remux := &hlsRemux{
		playlist: path,
		source:   &Process{timelineOrigin: origin},
		clockProbe: func(context.Context, string) (time.Duration, error) {
			probes++
			return 10*time.Second + 24*time.Millisecond, nil
		},
	}

	first, err := remux.snapshotManifest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := remux.snapshotManifest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := "#EXT-X-PROGRAM-DATE-TIME:2026-09-21T19:13:29.68-04:00"
	wantSecond := "#EXT-X-PROGRAM-DATE-TIME:2026-09-21T19:13:33.68-04:00"
	for index, body := range [][]byte{first, second} {
		if !strings.Contains(string(body), wantFirst) || !strings.Contains(string(body), wantSecond) {
			t.Fatalf("snapshot %d retained segment-write time instead of session media time:\n%s", index, body)
		}
	}
	if probes != 1 {
		t.Fatalf("first-video timestamp probes = %d, want one per remux", probes)
	}
}

func TestLiveHLSManifestFailsClosedWithoutAProgrammeDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, hlsPlaylistName)
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:4,\nseg-0.ts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	remux := &hlsRemux{
		playlist: path,
		source:   &Process{timelineOrigin: time.Now()},
		clockProbe: func(context.Context, string) (time.Duration, error) {
			return 10 * time.Second, nil
		},
	}
	if _, err := remux.snapshotManifest(t.Context()); err == nil {
		t.Fatal("live manifest without programme-date-time was served with no authoritative frame clock")
	}
}
