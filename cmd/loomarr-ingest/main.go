// Command loomarr-ingest is the optional filler-ingestion sidecar (design §10,
// §14, §16). It is a deliberately DUMB Go binary: it reads its own source list
// (YouTube playlists + Archive.org collections), shells out to the bundled
// yt-dlp (+ ffmpeg) for YouTube and plain net/http for Archive, and writes media
// files + info-JSON sidecars into a drop-folder that the media server scans as
// its filler library. The CORE never downloads or probes media — that's the
// whole point of the separate image (§16): the ~170MB of yt-dlp+ffmpeg tooling
// never touches the distroless core. Written in Go for repo consistency (shares
// the module + testkit), shipped as its own image behind the `filler` compose
// profile.
//
// The core has NO download configuration; this binary owns its own config
// (INGEST_SOURCES, INGEST_DROP_DIR, INGEST_EVERY).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mantonx/loomarr/internal/ingestkit"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := ingestkit.LoadConfig()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	log.Info("loomarr-ingest starting", "drop_dir", cfg.DropDir, "sources", len(cfg.Sources), "every", cfg.Every)

	// The real downloaders: yt-dlp (exec) for YouTube, net/http for Archive.
	ingestor := ingestkit.New(
		ingestkit.NewYtDlpDownloader(cfg.YtDlpPath, cfg.FfmpegPath),
		ingestkit.NewArchiveDownloader(cfg.PreferOriginal),
		cfg.DropDir, log,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// One pass immediately, then on the interval, until signalled.
	runOnce(ctx, ingestor, cfg.Sources, log)
	if cfg.Every <= 0 {
		log.Info("INGEST_EVERY unset — single-shot ingest done")
		return
	}
	t := time.NewTicker(cfg.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("loomarr-ingest stopped")
			return
		case <-t.C:
			runOnce(ctx, ingestor, cfg.Sources, log)
		}
	}
}

func runOnce(ctx context.Context, ing *ingestkit.Ingestor, sources []ingestkit.Source, log *slog.Logger) {
	res := ing.Run(ctx, sources)
	log.Info("ingest pass complete", "fetched", res.Fetched, "skipped", res.Skipped, "failed", res.Failed)
}
