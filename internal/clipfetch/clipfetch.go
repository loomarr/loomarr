// Package clipfetch downloads filler clips into the drop-folder (design §10, §16).
//
// It runs IN THE CORE, and the tooling it shells out to ships in the single image (§16) —
// so the `ingest` feature gate now reports a DEGRADED install (a binary that will not run)
// rather than an opt-in an operator has to take. ⚠ This doc used to describe both a
// `loomarr-ingest` sidecar and a `loomarr:filler` image variant (retired-ok);
// §10 records why each was
// reversed, and neither exists.
//
// ⚠ Named clipfetch, NOT ingest — but the reason has itself been retired-ok. This doc said
// "because internal/ingest is the Sonarr/Radarr WEBHOOK handler"; there is no internal/ingest
// package and there is no inbound arr hook (acquisition state comes from polling — see
// internal/reconcile). The name still earns its keep, for a smaller reason: `ingest` is what
// §10 calls the CLIP pipeline in internal/filler, so a package named ingest here would collide
// with that one autocomplete apart.
//
// The orchestration here is testable with fake downloaders; the real yt-dlp exec +
// Archive HTTP live behind the Downloader interface, so unit tests never touch the
// network or the yt-dlp binary (AGENTS.md testing rules).
package clipfetch

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
	// ID is the registered source policy responsible for this acquisition. Empty means a manual
	// ingest whose arrivals use the folder admission policy.
	ID string
	// AcquisitionID ties every downloaded clip back to the durable run that requested it.
	// It is metadata only: admission continues to be governed by the registered source ID.
	AcquisitionID string
	Kind          Kind
	URL           string
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
	// Empty counts sources that returned no clips and no error — a
	// nonexistent/typo'd id or empty source (surfaced so it isn't silent).
	Empty int
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
		if fetched == 0 && skipped == 0 {
			// A source that yielded no clips AND no error is almost always operator
			// error — a typo'd/nonexistent Archive id (Archive returns 200 {} for an
			// unknown item, so it's not a download failure), an empty collection, or a
			// YouTube source with no downloadable video. Without this the operator sees
			// "fetched:0 failed:0" and no reason why. Warn + count it so it's not silent.
			i.logf("ingest %s yielded no clips (nonexistent id / empty source?)", src.URL)
			res.Empty++
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
