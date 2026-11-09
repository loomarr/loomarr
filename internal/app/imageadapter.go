package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	imagesapi "github.com/loomarr/loomarr/internal/api/images"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/media/images/rustgen"
	"github.com/loomarr/loomarr/internal/media/images/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/config"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// imageService boxes the concrete service into the api interface, keeping a nil one nil.
//
// ⚠ Not ceremony. Assigning a nil `*images.Service` straight into an interface field yields a
// NON-nil interface holding a nil pointer, so `if s.images == nil` in the handlers would be false
// and the first call would panic on a store-less boot — the exact state §9's "keep serving so
// /readyz can explain why" is designed to survive. app.go already carries this warning for
// `*tmdb.Client`; this is the same trap, one type over.
func imageService(s *images.Service) imagesapi.ImageService {
	if s == nil {
		return nil
	}
	return s
}

// newImageService builds the image service from live settings (§22).
//
// Gated on the store alone — unlike the suggester or the icon picker it needs no TMDB key and no
// media server, because ingest, storage and serving are entirely local. An install with no TMDB
// still uploads channel icons and serves clip stills through it.
//
// ⚠ `images.dir` is read ONCE, here, and the other knobs are read per call. That split is the
// Config contract, not an oversight: the blob store is built from the directory, so re-pointing it
// at runtime would orphan every file already written, while the public base URL genuinely must
// hot-apply (an operator who sets `server.public_url` in the wizard needs absolute icon URLs on
// the next request, because Tunarr fetches them machine-to-machine).
func newImageService(
	st store.Store,
	set resolved,
	explicitWorker, release string,
	recorder *metrics.Recorder,
) (*images.Service, error) {
	worker, err := resolveImageWorker(explicitWorker)
	if err != nil {
		return nil, err
	}
	renderer, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: release, Recipe: "loomarr-rendition-v2",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	})
	if err != nil {
		return nil, err
	}
	return images.New(images.Config{
		Dir:            settings.ImagesDir.Get(set),
		MaxUploadBytes: func() int64 { return int64(settings.ImagesMaxUploadBytes.Get(set)) },
		PublicBaseURL:  func() string { return settings.ServerPublicURL.Get(set) },
		Observer: images.Observer{
			QueueWait: recorder.ImageWorkerQueueWait,
			InFlight:  recorder.ImageWorkerInFlight,
			Worker:    recorder.ImageWorkerObserved,
		},
	}, storeadapter.New(st), renderer, nil), nil
}

func resolveImageWorker(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if configured, _ := config.LookupEnv("LOOMARR_IMAGE_WORKER"); configured != "" {
		return configured, nil
	}
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), "loomarr-image")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	// A source checkout prefers the release worker production ships: the debug one is ~25x slower.
	for _, candidate := range []string{
		filepath.Join("target", "release", "loomarr-image"),
		filepath.Join("target", "debug", "loomarr-image"),
		filepath.Join("bin", "loomarr-image"),
	} {
		if absolute, err := filepath.Abs(candidate); err == nil {
			if info, statErr := os.Stat(absolute); statErr == nil && info.Mode().IsRegular() {
				return absolute, nil
			}
		}
	}
	if _, source, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
		for _, profile := range []string{"release", "debug"} {
			candidate := filepath.Join(root, "target", profile, "loomarr-image")
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	if found, err := exec.LookPath("loomarr-image"); err == nil {
		return found, nil
	}
	return "", fmt.Errorf("images: required loomarr-image worker not found")
}

// artworkAdoptStore bridges the clips table onto images.ArtworkAdoptStore (§22, V52 phase 6).
//
// It stays in the composition root because it joins three areas: the clip rows (the store), the
// filler's artwork-cache layout, and the image domain's pending-artwork type. Media may not import
// filler, so no one domain owns that join.
//
// ⚠ **Resolving the artwork cache path is THIS layer's job, and that is why the store returns
// relative paths.** `internal/platform/store` must not know where FILLER_DIR is — a query that joined a
// filesystem location onto a row would make the same SQL wrong the moment an operator moved the
// folder. `internal/media/images` must not know either; it takes absolute paths and never asks what
// produced them.
type artworkAdoptStore struct {
	st        store.Store
	fillerDir string
}

func (a artworkAdoptStore) ListPendingArtwork(ctx context.Context, limit int) ([]images.PendingArtwork, error) {
	dir := a.fillerDir
	if dir == "" {
		// No filler directory configured: there is no artwork cache to adopt from. Not an error —
		// an install with no filler is a supported shape (§10).
		return nil, nil
	}
	cache := filepath.Join(dir, cliplibrary.ThumbDirName)

	rows, err := a.st.ListClipsPendingArtworkAdoption(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]images.PendingArtwork, 0, len(rows))
	for _, r := range rows {
		p := images.PendingArtwork{OwnerID: r.Hash}
		// ⚠ Empty stays EMPTY rather than becoming the cache directory itself. filepath.Join("x","")
		// returns "x", so joining unconditionally would hand the job a directory to open as an
		// image — which fails as an ingest error rather than as the "nothing rendered" state it is.
		if r.Thumbnail != "" {
			p.StillPath = filepath.Join(cache, r.Thumbnail)
		}
		if r.Preview != "" {
			p.AnimPath = filepath.Join(cache, r.Preview)
		}
		out = append(out, p)
	}
	return out, nil
}

func (a artworkAdoptStore) SetAdoptedArtwork(ctx context.Context, ownerID, stillHash, animHash string, at time.Time) error {
	return a.st.SetClipArtworkImages(ctx, ownerID, stillHash, animHash, at)
}
