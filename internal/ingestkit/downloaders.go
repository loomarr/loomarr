package ingestkit

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
// (no special tooling). v1 is a stub of the real HTTP walk — the manual smoke
// wires the actual Archive metadata API + file fetch; the interface + dispatch
// are what the core repo needs.
type ArchiveDownloader struct{}

// NewArchiveDownloader builds the Archive.org downloader.
func NewArchiveDownloader() *ArchiveDownloader { return &ArchiveDownloader{} }

// Download fetches an Archive.org source into dropDir. The real implementation
// walks the item's file list via the Archive metadata API and downloads the
// video files + writes an info-JSON sidecar; wired in the sidecar's manual smoke.
func (d *ArchiveDownloader) Download(ctx context.Context, src Source, dropDir string) (int, int, error) {
	// Placeholder for the manual-smoke wiring — the orchestration + dispatch are
	// tested; the Archive HTTP walk is exercised live, not in the core gate.
	return 0, 0, fmt.Errorf("archive downloader not yet wired for %s (manual-smoke)", src.URL)
}

var (
	_ Downloader = (*YtDlpDownloader)(nil)
	_ Downloader = (*ArchiveDownloader)(nil)
)
