package app

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/activity"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/scheduler"
)

const (
	// Provider protection and compliance are policy, not settings. Letting an operator
	// raise either value can only cause throttling or a terms violation.
	imageRemoteConcurrency = 12
	imageRemoteTTL         = 180 * 24 * time.Hour
)

// The image service's job wiring (§22, §18.1 — V52 phase 3b).
//
// ⚠ **Registration is the whole point of this file, and it is the step with no unit test.** Phase
// 3a shipped three routes that 404'd in a running instance because `Server.images` was nil —
// the "a built component nobody imported" defect this repo has now recorded against V1, V17a, V23
// and V52 phase 2. Four jobs constructed here and never handed to the registry would fail the same
// way and look identical from inside every test that constructs them directly, which is why the
// guard for this lives in jobset_test.go, against the real BuildHandler.

// registerImageJobs constructs the artwork jobs and adds them to the registry.
// It returns the fetcher so interactive callers can share it — the icon picker adopts a poster on
// an operator's request and warms it synchronously (iconAdapter). Sharing the instance rather than
// building a second one keeps the SSRF allowlist and the concurrency cap identical whoever asks.
func registerImageJobs(ctx context.Context, reg *scheduler.Registry, svc *images.Service, st imageStore, set resolved, rec *activity.Recorder, log *slog.Logger) *images.Fetcher {
	fetcher := images.NewFetcher(svc, st,
		func() bool { return set.boolv("images.remote_fetch_enabled") },
		func() int { return imageRemoteConcurrency },
		func() []string { return imageFetchHosts(set) },
		log)

	reg.Add(images.FetchJob(fetcher))

	// The release handshake already performed a real Rust AVIF encode. There is no second probe or
	// ffmpeg path here: the worker is required, and a mismatch prevents readiness.
	reg.Add(images.AVIFJobSpec(images.NewAVIFJob(svc, st, log), ""))

	// ⚠ The adoption pass, and it is what makes the clip half of §22 real: without it, artwork
	// keeps living only as files under FILLER_DIR and every clip surface stays on the legacy route.
	// `st.st` is the underlying store rather than the image-shaped adapter — this job speaks to the
	// CLIPS table, which the image adapter deliberately knows nothing about.
	reg.Add(images.AdoptJobSpec(images.NewAdoptJob(svc, artworkAdoptStore{
		st:        st.st,
		fillerDir: func() string { return set.str("filler.dir") },
	}, nil, log)))

	// ⚠ `rec` may be nil (no store-backed activity feed), and NewGC takes the interface, so this
	// cannot be handed over unconditionally: a nil *activity.Recorder assigned into a non-nil
	// interface is the same trap `imageService` exists to avoid one file over.
	var notify images.Notifier
	if rec != nil {
		notify = rec
	}
	reg.Add(images.MaintenanceJob(fetcher, images.NewGC(svc, st,
		func() time.Duration { return imageRemoteTTL },
		func() int { return set.intv("images.cache_budget_mb") },
		notify, log)))

	return fetcher
}

// imageFetchHosts is the SSRF allowlist the fetcher enforces (§22).
//
// ⚠ **Derived from what this install is configured to talk to, never an operator free-text list.**
// A host allowlist is only a control if something other than the attacker decides what is on it,
// and the two legitimate origins are already configured elsewhere: TMDB's image CDN, which is a
// constant, and the media server, whose URL the operator set in the wizard. A settings key here
// would be a third place to get it wrong and a knob whose only correct values are these two.
//
// The media server is normally on a PRIVATE address — that is what self-hosted means — which is
// why the fetcher's private-range rule applies to redirects rather than to the first hop. See
// Fetcher.checkURL.
func imageFetchHosts(set resolved) []string {
	hosts := []string{"tmdb.org"} // covers image.tmdb.org and any sibling CDN name
	if raw := strings.TrimSpace(set.str("library.url")); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	return hosts
}
