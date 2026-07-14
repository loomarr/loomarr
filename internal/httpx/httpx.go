// Package httpx is the shared outbound HTTP client factory (design §6, §21 phase 1).
//
// Every adapter that talks to an external service builds its client here so the
// resilience defaults live in exactly one place: hard per-service timeouts, and
// jittered-backoff retries for idempotent GETs only. Writes never client-retry —
// write recovery is owned by the idempotent reconcile loops and periodic sweeps
// (§6). A down dependency degrades one feature; it never wedges the process.
package httpx

import (
	"net/http"
	"time"
)

// Per-service hard timeouts (§6). Verified reachable at these budgets in Phase 0.
const (
	TimeoutLibrary = 10 * time.Second // Emby/Jellyfin
	TimeoutSeerr   = 10 * time.Second
	TimeoutTMDB    = 10 * time.Second
	TimeoutArr     = 10 * time.Second // Sonarr/Radarr
	TimeoutTunarr  = 20 * time.Second // lineup pushes are chunky
	TimeoutLLM     = 120 * time.Second
	TimeoutProbe   = 5 * time.Second // quick LLM-host probes (version/tags, §8.1)
)

// New returns an *http.Client with the given hard timeout. The timeout is a
// whole-request budget (connect + headers + body), which is what we want for a
// homelab service that may hang rather than refuse.
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// Conservative pooled defaults; a homelab has few upstreams.
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}
