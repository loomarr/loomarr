package clipfetch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/proctree"
)

// This file holds the REAL downloaders — the ones that touch the network and the
// yt-dlp binary. They are behind the Downloader interface so the Ingestor's
// orchestration is unit-tested with fakes. The yt-dlp process boundary is tested
// offline with a repository-built executable; Archive HTTP remains behind its
// test server boundary (AGENTS.md: unit tests never touch the network).

// YtDlpDownloader shells out to the bundled yt-dlp (with ffmpeg for
// post-processing) to fetch a YouTube playlist/video into the drop-folder,
// preserving the info-JSON sidecar yt-dlp writes (the source title/description
// the core's AI tagging reads as text signals, §10). The shared process-tree
// supervisor owns yt-dlp and every ffmpeg or Deno descendant (§9.1).
type YtDlpDownloader struct {
	ytDlpPath  string
	ffmpegPath string
}

const ytDlpDiagnosticLimit = 64 << 10

type diagnosticTail struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *diagnosticTail) Write(p []byte) (int, error) {
	written := len(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= b.limit {
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
		b.truncated = true
		return written, nil
	}
	if overflow := len(b.data) + len(p) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return written, nil
}

func (b *diagnosticTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return "[... yt-dlp output truncated ...]\n" + string(b.data)
	}
	return string(b.data)
}

// NewYtDlpDownloader builds the yt-dlp downloader.
func NewYtDlpDownloader(ytDlpPath, ffmpegPath string) *YtDlpDownloader {
	return &YtDlpDownloader{ytDlpPath: ytDlpPath, ffmpegPath: ffmpegPath}
}

// Download runs supervised yt-dlp for one source. It writes video + `.info.json` sidecars
// into dropDir and uses --download-archive so a re-run skips already-fetched
// items (idempotent ingest). It parses only yt-dlp's final paths so provenance
// reaches the exact sidecars created by this invocation; the media server still
// scans the folder and the core syncs from there. It returns (0,0,nil) on success,
// surfacing only exec failure.
func (d *YtDlpDownloader) Download(ctx context.Context, src Source, dropDir string) (int, int, error) {
	absDropDir, err := filepath.Abs(dropDir)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve yt-dlp drop directory: %w", err)
	}
	if err := os.MkdirAll(absDropDir, 0o750); err != nil {
		return 0, 0, fmt.Errorf("create yt-dlp drop directory: %w", err)
	}
	archiveFile := filepath.Join(absDropDir, ".yt-dlp-archive.txt")
	resultFile, err := os.CreateTemp(absDropDir, ".loomarr-ytdlp-results-*")
	if err != nil {
		return 0, 0, fmt.Errorf("create yt-dlp result file: %w", err)
	}
	resultPath := resultFile.Name()
	if err := resultFile.Close(); err != nil {
		_ = os.Remove(resultPath)
		return 0, 0, fmt.Errorf("close yt-dlp result file: %w", err)
	}
	defer func() { _ = os.Remove(resultPath) }()
	// -o with a sanitized template into the drop folder; --write-info-json so the
	// title/description survive for AI tagging (§10); --download-archive for
	// idempotent re-runs; --ffmpeg-location for the bundled binary.
	args := []string{
		"--no-progress",
		"--write-info-json",
		"--download-archive", archiveFile,
		"--ffmpeg-location", d.ffmpegPath,
		"-o", filepath.Join(absDropDir, "%(title)s [%(id)s].%(ext)s"),
		"--print-to-file", "after_move:%(filepath)j", resultPath,
		src.URL,
	}
	cmd := exec.Command(d.ytDlpPath, args...)
	out := diagnosticTail{limit: ytDlpDiagnosticLimit}
	cmd.Stdout = &out
	cmd.Stderr = &out
	supervisor, err := proctree.Start(ctx, cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("yt-dlp %s: %w: %s", src.URL, err, out.String())
	}
	err = supervisor.Wait()
	if supervisor.Stopped() {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, 0, fmt.Errorf("yt-dlp %s: %w: %s", src.URL, ctxErr, out.String())
		}
	}
	if err != nil {
		return 0, 0, fmt.Errorf("yt-dlp %s: %w: %s", src.URL, err, out.String())
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
	root, err := os.OpenRoot(absDropDir)
	if err == nil {
		defer func() { _ = root.Close() }()
		stampFetched(root, ytDlpSidecars(root, resultPath), src.ID, src.AcquisitionID)
	}
	return 0, 0, nil
}

// ytDlpSidecars reads yt-dlp's JSON-encoded after-move paths from the private result file for this
// invocation. Ordinary stdout/stderr diagnostics are deliberately never considered provenance.
// A skipped retry writes no records. Candidates are opened through root, so their descriptor stays
// bound to the file yt-dlp named even if its path changes before stamping.
func ytDlpSidecars(root *os.Root, resultPath string) []fetchedSidecar {
	file, err := os.Open(resultPath)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var sidecars []fetchedSidecar
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4*1024), 1024*1024)
	for scanner.Scan() {
		var media string
		if err := json.Unmarshal(scanner.Bytes(), &media); err != nil || media == "" {
			continue
		}
		if !filepath.IsAbs(media) {
			media = filepath.Join(root.Name(), media)
		}
		sidecar := strings.TrimSuffix(media, filepath.Ext(media)) + ".info.json"
		relativeSidecar, err := filepath.Rel(root.Name(), sidecar)
		if err != nil || !withinDir(relativeSidecar) {
			continue
		}
		if _, ok := seen[relativeSidecar]; ok {
			continue
		}
		sidecarFile, err := root.OpenFile(relativeSidecar, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		seen[relativeSidecar] = struct{}{}
		sidecars = append(sidecars, fetchedSidecar{path: relativeSidecar, file: sidecarFile})
	}
	return sidecars
}

func withinDir(path string) bool {
	return path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator)) && !filepath.IsAbs(path)
}

type fetchedSidecar struct {
	path string
	file *os.File
}

// stampFetched adds Loomarr's `fetchedBy` mark to sidecars yt-dlp demonstrably produced during
// this invocation. It must never sweep the drop folder: a later invocation must not claim an
// unrelated operator drop or a previous download as its own acquisition.
//
// Anything already stamped is left alone, so a re-run is cheap and idempotent.
func stampFetched(root *os.Root, sidecars []fetchedSidecar, sourceID, acquisitionID string) {
	for _, sidecar := range sidecars {
		func() {
			defer func() { _ = sidecar.file.Close() }()

			fileInfo, err := sidecar.file.Stat()
			if err != nil {
				return
			}
			pathInfo, err := root.Lstat(sidecar.path)
			if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(fileInfo, pathInfo) {
				return
			}
			if _, err := sidecar.file.Seek(0, io.SeekStart); err != nil {
				return
			}
			raw, err := io.ReadAll(sidecar.file)
			if err != nil {
				return
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				// A sidecar we cannot read is left ALONE rather than replaced — it is yt-dlp's file,
				// and overwriting something unparseable is the one move guaranteed to lose data.
				return
			}
			if _, done := doc[filler.SidecarLoomarrKey()]; done {
				return
			}
			doc[filler.SidecarLoomarrKey()] = filler.SidecarFetchedMarkForAcquisition(sourceID, acquisitionID)
			out, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return
			}
			// ⚠ A failed write is swallowed rather than logged: this type carries no logger, and the
			// consequence is bounded — the clip files without review instead of being lost. The
			// Ingestor above it reports the download itself.
			if err := sidecar.file.Truncate(0); err != nil {
				return
			}
			if _, err := sidecar.file.Seek(0, io.SeekStart); err != nil {
				return
			}
			_, _ = sidecar.file.Write(out) //nolint:gosec // metadata beside media the operator owns
		}()
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
	return d.client.walk(withAcquisition(ctx, src.ID, src.AcquisitionID), src.URL, dropDir)
}

var (
	_ Downloader = (*YtDlpDownloader)(nil)
	_ Downloader = (*ArchiveDownloader)(nil)
)
