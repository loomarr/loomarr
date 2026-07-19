package clipfetch

import (
	"context"
	"fmt"
	"os/exec"
)

// This file holds the REAL downloaders — the ones that touch the network and the
// yt-dlp binary. They are behind the Downloader interface so the Ingestor's
// orchestration is unit-tested with fakes and these are exercised only in the
// sidecar's manual smoke (they are not part of the core `make check` gate;
// CLAUDE.md: unit tests never touch the network).

// YtDlpDownloader shells out to the bundled yt-dlp (with ffmpeg for
// post-processing) to fetch a YouTube playlist/video into the drop-folder,
// preserving the info-JSON sidecar yt-dlp writes (the source title/description
// the core's AI tagging reads as text signals, §10).
type YtDlpDownloader struct {
	ytDlpPath  string
	ffmpegPath string
}

// NewYtDlpDownloader builds the yt-dlp downloader.
func NewYtDlpDownloader(ytDlpPath, ffmpegPath string) *YtDlpDownloader {
	return &YtDlpDownloader{ytDlpPath: ytDlpPath, ffmpegPath: ffmpegPath}
}

// Download runs yt-dlp for one source. It writes video + `.info.json` sidecars
// into dropDir and uses --download-archive so a re-run skips already-fetched
// items (idempotent ingest). It does NOT parse per-item results (deliberately
// dumb — the media server scans the folder and the core syncs from there); it
// returns (0,0,nil) on success, surfacing only exec failure.
func (d *YtDlpDownloader) Download(ctx context.Context, src Source, dropDir string) (int, int, error) {
	archiveFile := dropDir + "/.yt-dlp-archive.txt"
	// -o with a sanitized template into the drop folder; --write-info-json so the
	// title/description survive for AI tagging (§10); --download-archive for
	// idempotent re-runs; --ffmpeg-location for the bundled binary.
	args := []string{
		"--no-progress",
		"--write-info-json",
		"--download-archive", archiveFile,
		"--ffmpeg-location", d.ffmpegPath,
		"-o", dropDir + "/%(title)s [%(id)s].%(ext)s",
		src.URL,
	}
	cmd := exec.CommandContext(ctx, d.ytDlpPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("yt-dlp %s: %w: %s", src.URL, err, string(out))
	}
	return 0, 0, nil
}

// ArchiveDownloader fetches an Archive.org item/collection via plain net/http
// (no special tooling — §10). It walks Archive's public JSON APIs (metadata +
// advancedsearch), picks the smallest video derivative, and writes the media +
// an info-JSON sidecar into the drop-folder. The walk logic lives in
// archiveClient (injectable HTTP + fs → unit-tested against a mock server).
type ArchiveDownloader struct {
	client *archiveClient
}

// NewArchiveDownloader builds the Archive.org downloader against the real host.
// preferOriginal keeps full-quality masters instead of the small derivative
// (default false — the derivative is right for filler; §10).
func NewArchiveDownloader(preferOriginal bool) *ArchiveDownloader {
	c := newArchiveClient("https://archive.org", nil, diskSink{})
	c.preferOriginal = preferOriginal
	return &ArchiveDownloader{client: c}
}

// Download fetches an Archive.org source into dropDir (§10).
func (d *ArchiveDownloader) Download(ctx context.Context, src Source, dropDir string) (int, int, error) {
	return d.client.walk(ctx, src.URL, dropDir)
}

var (
	_ Downloader = (*YtDlpDownloader)(nil)
	_ Downloader = (*ArchiveDownloader)(nil)
)
