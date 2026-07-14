package ingestkit

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the sidecar's OWN configuration (§10 — the core has no download
// config). All from the sidecar's environment; not part of the core's §15.
type Config struct {
	DropDir    string   // INGEST_DROP_DIR — the media server's filler drop-folder
	Sources    []Source // INGEST_SOURCES — comma-separated playlist/collection URLs
	Every      time.Duration
	YtDlpPath  string // path to the bundled yt-dlp binary
	FfmpegPath string // path to the bundled ffmpeg binary
}

// LoadConfig reads the sidecar env. INGEST_DROP_DIR is required; sources may be
// empty (a no-op pass). INGEST_EVERY unset ⇒ single-shot.
func LoadConfig() (Config, error) {
	drop := os.Getenv("INGEST_DROP_DIR")
	if drop == "" {
		return Config{}, fmt.Errorf("INGEST_DROP_DIR is required")
	}
	cfg := Config{
		DropDir:    drop,
		Sources:    parseSources(os.Getenv("INGEST_SOURCES")),
		YtDlpPath:  envOr("YT_DLP_PATH", "yt-dlp"),
		FfmpegPath: envOr("FFMPEG_PATH", "ffmpeg"),
	}
	if v := os.Getenv("INGEST_EVERY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("INGEST_EVERY: %w", err)
		}
		cfg.Every = d
	}
	return cfg, nil
}

// parseSources splits a comma-separated URL list into typed Sources, inferring
// the kind from each URL's host.
func parseSources(raw string) []Source {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []Source
	for _, part := range strings.Split(raw, ",") {
		url := strings.TrimSpace(part)
		if url == "" {
			continue
		}
		out = append(out, Source{Kind: KindForURL(url), URL: url})
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
