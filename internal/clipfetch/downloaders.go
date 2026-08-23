package clipfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler"
)

// This file holds the REAL downloaders — the ones that touch the network and the
// yt-dlp binary. They are behind the Downloader interface so the Ingestor's
// orchestration is unit-tested with fakes and these are exercised only in the
// sidecar's manual smoke (they are not part of the core `make check` gate;
// AGENTS.md: unit tests never touch the network).

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
	// ⚠ **Mark what we just downloaded as OURS** — the held/filed fork's only signal (§10 V38c).
	// A clip Loomarr fetched waits in Incoming for a human; one an operator dropped in is filed on
	// sight. Only the downloader can tell them apart.
	//
	// Stamped AFTERWARDS rather than written here, because yt-dlp owns this sidecar
	// (`--write-info-json`) and re-creating it would throw away the title and description that
	// are the tagger's real text signals. `stampFetched` merges into what yt-dlp wrote.
	//
	// ⚠ Best-effort: a stamp that fails must not fail the download. The clip is on disk and
	// catalogueable; the cost is that it files without review rather than being lost.
	stampFetched(dropDir)
	return 0, 0, nil
}

// stampFetched adds Loomarr's `fetchedBy` mark to every info-JSON in dropDir that lacks one.
//
// ⚠ Sweeps the folder rather than tracking filenames, because yt-dlp names its own output from a
// template (`%(title)s [%(id)s]`) after sanitising, and guessing that name back is how the mark
// would silently miss the files it was written for. Anything already stamped is left alone, so a
// re-run is cheap and idempotent.
func stampFetched(dropDir string) {
	entries, err := os.ReadDir(dropDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".info.json") {
			continue
		}
		path := filepath.Join(dropDir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			// A sidecar we cannot read is left ALONE rather than replaced — it is yt-dlp's file,
			// and overwriting something unparseable is the one move guaranteed to lose data.
			continue
		}
		if _, done := doc[filler.SidecarLoomarrKey()]; done {
			continue
		}
		doc[filler.SidecarLoomarrKey()] = filler.SidecarFetchedMark()
		out, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			continue
		}
		// ⚠ A failed write is swallowed rather than logged: this type carries no logger, and the
		// consequence is bounded — the clip files without review instead of being lost. The
		// Ingestor above it reports the download itself.
		_ = os.WriteFile(path, out, 0o644) //nolint:gosec // metadata beside media the operator owns
	}
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
