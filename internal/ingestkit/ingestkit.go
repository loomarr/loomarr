// Package ingestkit is the loomarr-ingest sidecar's logic (design §10, §16). It
// is separate from the core: nothing in internal/ (except this package, used only
// by cmd/loomarr-ingest) imports it, and the core never downloads media. The
// orchestration here is testable with fake downloaders; the real yt-dlp exec +
// Archive HTTP live behind the Downloader interface so unit tests never touch the
// network or the yt-dlp binary (CLAUDE.md testing rules).
package ingestkit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Kind is the source type — it selects the downloader.
type Kind string

const (
	YouTube Kind = "youtube" // a YouTube playlist/video (yt-dlp)
	Archive Kind = "archive" // an Archive.org collection/item (net/http)
)

// Source is one ingestion target the sidecar pulls into the drop-folder.
type Source struct {
	Kind Kind
	URL  string
}

// KindForURL infers the source kind from a URL (youtube.com / youtu.be →
// youtube; archive.org → archive). Unknown hosts default to youtube (yt-dlp
// handles the widest set of sites).
func KindForURL(url string) Kind {
	l := strings.ToLower(url)
	switch {
	case strings.Contains(l, "archive.org"):
		return Archive
	default:
		return YouTube
	}
}

// Downloader pulls one source's media into a drop directory, returning how many
// items it fetched (new) vs skipped (already present). Implementations: the real
// yt-dlp exec + Archive HTTP; tests use fakes.
type Downloader interface {
	// Download fetches `src` into dropDir. Returns (fetched, skipped, error).
	Download(ctx context.Context, src Source, dropDir string) (fetched, skipped int, err error)
}

// Ingestor orchestrates a pass over all sources, dispatching each to the right
// downloader. Deliberately thin — the value is in "which downloader, into where",
// not in the download mechanics.
type Ingestor struct {
	youtube Downloader
	archive Downloader
	dropDir string
	log     *slog.Logger
}

// New builds an Ingestor from the two real (or fake) downloaders + the drop dir.
func New(youtube, archive Downloader, dropDir string, log *slog.Logger) *Ingestor {
	return &Ingestor{youtube: youtube, archive: archive, dropDir: dropDir, log: log}
}

// Result aggregates one pass.
type Result struct {
	Fetched int
	Skipped int
	Failed  int
}

// Run ingests every source once. A failed source is logged and counted, never
// fatal — one bad playlist must not stop the rest (§6 resilience spirit).
func (i *Ingestor) Run(ctx context.Context, sources []Source) Result {
	var res Result
	for _, src := range sources {
		select {
		case <-ctx.Done():
			return res
		default:
		}
		dl := i.downloaderFor(src.Kind)
		if dl == nil {
			i.logf("no downloader for kind %q (%s)", src.Kind, src.URL)
			res.Failed++
			continue
		}
		fetched, skipped, err := dl.Download(ctx, src, i.dropDir)
		if err != nil {
			i.logf("ingest %s failed: %v", src.URL, err)
			res.Failed++
			continue
		}
		res.Fetched += fetched
		res.Skipped += skipped
	}
	return res
}

func (i *Ingestor) downloaderFor(k Kind) Downloader {
	switch k {
	case YouTube:
		return i.youtube
	case Archive:
		return i.archive
	default:
		return nil
	}
}

func (i *Ingestor) logf(format string, args ...any) {
	if i.log != nil {
		i.log.Warn(fmt.Sprintf(format, args...))
	}
}
