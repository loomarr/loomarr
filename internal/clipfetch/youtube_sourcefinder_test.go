package clipfetch_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestYouTubeSourceFinderSuggestsUniqueChannelsWithoutDownloading(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
case "$*" in
  *--no-config*--flat-playlist*--skip-download*--dump-single-json*--playlist-end\ 4*ytsearch4:retro\ commercials*) ;;
  *) exit 9 ;;
esac
printf '%s\n' '{"entries":[
  {"title":"Retro ad one","channel":"Retro Reels","channel_id":"UC-retro","channel_url":"https://www.youtube.com/channel/UC-retro"},
  {"title":"Retro ad two","channel":"Retro Reels","channel_id":"UC-retro","channel_url":"https://www.youtube.com/channel/UC-retro"},
  {"title":"Station break","uploader":"Broadcast Vault","uploader_id":"UC-vault","uploader_url":"https://www.youtube.com/channel/UC-vault"}
]}'
`)

	got, err := clipfetch.NewYouTubeSourceFinder(ytdlp).Suggest(context.Background(), "retro commercials", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("suggestions = %#v, want two unique channels", got)
	}
	if got[0].Provider != "youtube" || got[0].TargetType != "channel" ||
		got[0].CanonicalID != "UC-retro" || got[0].CanonicalURL != "https://www.youtube.com/channel/UC-retro/videos" ||
		got[0].Title != "Retro Reels" || !strings.Contains(got[0].Description, "Retro ad one") {
		t.Fatalf("first suggestion = %#v", got[0])
	}
	if got[1].CanonicalID != "UC-vault" || got[1].Title != "Broadcast Vault" {
		t.Fatalf("second suggestion = %#v", got[1])
	}
}

func TestYouTubeSourceFinderResolvesAPlaylistToARegistrableSource(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
case "$*" in
  *--skip-download*--dump-single-json*--playlist-end\ 3*playlist?list=PL123*)
    printf '%s\n' '{"id":"PL123","title":"Favorite station breaks","description":"Curated playlist","playlist_count":42,"entries":[
      {"id":"video-one","title":"Station break one","duration":31.5},
      {"id":"video-two","title":"Station break two","duration":45},
      {"id":"video-three","title":"Station break three"}
    ]}' ;;
  *) exit 9 ;;
esac
`)
	finder := clipfetch.NewYouTubeSourceFinder(ytdlp)

	playlist, err := finder.Resolve(context.Background(), "youtube.com/playlist?list=PL123")
	if err != nil {
		t.Fatal(err)
	}
	if playlist.TargetType != "playlist" || playlist.CanonicalID != "PL123" ||
		playlist.CanonicalURL != "https://www.youtube.com/playlist?list=PL123" ||
		playlist.Title != "Favorite station breaks" || playlist.ItemCount != 42 {
		t.Fatalf("playlist = %#v", playlist)
	}
	if len(playlist.PreviewItems) != 3 || playlist.PreviewItems[0].Title != "Station break one" ||
		playlist.PreviewItems[0].URL != "https://www.youtube.com/watch?v=video-one" ||
		playlist.PreviewItems[0].DurationMS != 31500 {
		t.Fatalf("playlist preview = %#v", playlist.PreviewItems)
	}
}

func TestYouTubeSourceFinderResolvesHandlesAndCanonicalIDsBeforeSearch(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
case "$*" in
  *https://www.youtube.com/@retroads/videos*)
    printf '%s\n' '{"id":"UC1234567890123456789012","title":"Retro Ads - Videos"}' ;;
  *https://www.youtube.com/channel/UC1234567890123456789012/videos*)
    printf '%s\n' '{"id":"UC1234567890123456789012","title":"Retro Ads - Videos"}' ;;
  *https://www.youtube.com/playlist?list=PL1234567890*)
    printf '%s\n' '{"id":"PL1234567890","title":"Favorite ads"}' ;;
  *) exit 9 ;;
esac
`)
	finder := clipfetch.NewYouTubeSourceFinder(ytdlp)

	for _, input := range []string{"@retroads", "UC1234567890123456789012"} {
		got, err := finder.Resolve(context.Background(), input)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", input, err)
		}
		if got.TargetType != "channel" || got.CanonicalURL != "https://www.youtube.com/channel/UC1234567890123456789012/videos" {
			t.Fatalf("Resolve(%q) = %#v", input, got)
		}
	}
	playlist, err := finder.Resolve(context.Background(), "PL1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if playlist.TargetType != "playlist" || playlist.CanonicalURL != "https://www.youtube.com/playlist?list=PL1234567890" {
		t.Fatalf("playlist = %#v", playlist)
	}
}

func TestYouTubeSourceFinderRejectsInvalidInputBeforeStartingYtDlp(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", "#!/bin/sh\nexit 88\n")
	finder := clipfetch.NewYouTubeSourceFinder(ytdlp)

	for _, input := range []string{
		"not a URL",
		"https://example.com/@retroads",
		"https://youtube.com/results?search_query=retro",
		"https://youtu.be/video123",
		"https://www.youtube.com/watch?v=video123",
	} {
		if _, err := finder.Resolve(context.Background(), input); !strings.Contains(err.Error(), filler.ErrInvalidSourceReference.Error()) {
			t.Errorf("Resolve(%q) = %v, want invalid source reference", input, err)
		}
	}
}

func TestYouTubeSourceFinderRefusesAnUnavailableTool(t *testing.T) {
	_, err := clipfetch.NewYouTubeSourceFinder("").Suggest(context.Background(), "retro commercials", 8)
	if err == nil || !strings.Contains(err.Error(), "yt-dlp is unavailable") {
		t.Fatalf("Suggest without yt-dlp = %v", err)
	}
}

func TestYouTubeSourceFinderBoundsToolOutput(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
dd if=/dev/zero bs=2097153 count=1 2>/dev/null
`)
	_, err := clipfetch.NewYouTubeSourceFinder(ytdlp).Suggest(context.Background(), "retro commercials", 8)
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized yt-dlp output = %v, want bounded refusal", err)
	}
}

func TestYouTubeSourceFinderCollapsesConcurrentIdenticalSearchesAndCachesThem(t *testing.T) {
	dir := t.TempDir()
	calls, release := dir+"/calls", dir+"/release"
	ytdlp := testkit.Executable(t, "yt-dlp", fmt.Sprintf(`#!/bin/sh
printf 'call\n' >> %q
while [ ! -f %q ]; do sleep 0.01; done
printf '%%s\n' '{"entries":[]}'
`, calls, release))
	finder := clipfetch.NewYouTubeSourceFinder(ytdlp)

	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := finder.Suggest(context.Background(), "retro commercials", 6); results <- err }()
	}
	for attempts := 0; attempts < 100; attempts++ {
		if raw, err := os.ReadFile(calls); err == nil && len(raw) > 0 {
			break
		}
	}
	if err := os.WriteFile(release, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := finder.Suggest(context.Background(), "  retro   commercials ", 6); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "call\n"); got != 1 {
		t.Fatalf("yt-dlp calls = %d, want one singleflight call and one cache hit", got)
	}
}

func TestYouTubeSourceFinderRunsOnlyOneDistinctSearchAtATime(t *testing.T) {
	dir := t.TempDir()
	lock, overlap := dir+"/lock", dir+"/overlap"
	ytdlp := testkit.Executable(t, "yt-dlp", fmt.Sprintf(`#!/bin/sh
if ! mkdir %q 2>/dev/null; then printf 'overlap\n' >> %q; fi
sleep 0.1
rmdir %q 2>/dev/null || true
printf '%%s\n' '{"entries":[]}'
`, lock, overlap, lock))
	finder := clipfetch.NewYouTubeSourceFinder(ytdlp)

	var group sync.WaitGroup
	for _, query := range []string{"retro commercials", "station breaks"} {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := finder.Suggest(context.Background(), query, 6); err != nil {
				t.Errorf("Suggest(%q): %v", query, err)
			}
		}()
	}
	group.Wait()
	if raw, err := os.ReadFile(overlap); err == nil && len(raw) > 0 {
		t.Fatalf("distinct searches overlapped: %s", raw)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestYouTubeSourceFinderRejectsMalformedOrIdentitylessResults(t *testing.T) {
	for name, output := range map[string]string{
		"malformed":    "not-json",
		"identityless": `{"entries":[{"title":"A video without a channel"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			ytdlp := testkit.Executable(t, "yt-dlp", "#!/bin/sh\nprintf '%s\\n' '"+output+"'\n")
			_, err := clipfetch.NewYouTubeSourceFinder(ytdlp).Suggest(context.Background(), "retro commercials", 6)
			if err == nil || !strings.Contains(err.Error(), filler.ErrSourceProvider.Error()) {
				t.Fatalf("result = %v, want provider error", err)
			}
		})
	}
}
