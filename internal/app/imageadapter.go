package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/images/rustgen"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/store"
)

// The image service's store bridge (§22, V52).
//
// `internal/images` declares the persistence it needs with its OWN types (images.Store), and no
// domain package in this codebase imports internal/store. This file is the whole cost of that
// rule: a type-for-type translation, in the composition root, where the two vocabularies are
// allowed to meet.
//
// ⚠ The translation is not merely structural — it crosses a TYPING boundary. The domain has
// closed types (images.Origin, images.Visibility, images.Role, images.Format) where the store has
// plain strings, because a database column is a string no matter what a Go program believes. Every
// value going down is `string(...)`; every value coming up is re-typed. That asymmetry is the
// point: an invalid role read from a row becomes an images.Role the domain can reason about
// (Role.NearestWidth falls back for an unknown value), rather than a panic at the seam.

// imageService boxes the concrete service into the api interface, keeping a nil one nil.
//
// ⚠ Not ceremony. Assigning a nil `*images.Service` straight into an interface field yields a
// NON-nil interface holding a nil pointer, so `if s.images == nil` in the handlers would be false
// and the first call would panic on a store-less boot — the exact state §9's "keep serving so
// /readyz can explain why" is designed to survive. app.go already carries this warning for
// `*tmdb.Client`; this is the same trap, one type over.
func imageService(s *images.Service) api.ImageService {
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
		Dir:            set.str("images.dir"),
		MaxUploadBytes: func() int64 { return int64(set.intv("images.max_upload_bytes")) },
		PublicBaseURL:  func() string { return set.str("server.public_url") },
		Observer: images.Observer{
			QueueWait: recorder.ImageWorkerQueueWait,
			InFlight:  recorder.ImageWorkerInFlight,
			Worker:    recorder.ImageWorkerObserved,
		},
	}, imageStore{st}, renderer, nil), nil
}

func resolveImageWorker(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if configured := os.Getenv("LOOMARR_IMAGE_WORKER"); configured != "" {
		return configured, nil
	}
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), "loomarr-image")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	for _, candidate := range []string{
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
		candidate := filepath.Join(root, "target", "debug", "loomarr-image")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	if found, err := exec.LookPath("loomarr-image"); err == nil {
		return found, nil
	}
	return "", fmt.Errorf("images: required loomarr-image worker not found")
}

// imageStore adapts store.Store to images.Store.
type imageStore struct{ st store.Store }

func (a imageStore) PutImage(ctx context.Context, img images.Image) error {
	return a.st.PutImage(ctx, toStoreImage(img))
}

func (a imageStore) GetImage(ctx context.Context, hash string) (images.Image, error) {
	rec, err := a.st.GetImage(ctx, hash)
	if err != nil {
		return images.Image{}, mapImageErr(err)
	}
	return fromStoreImage(rec), nil
}

func (a imageStore) GetFetchedBySourceURL(ctx context.Context, src string) (images.Image, error) {
	rec, err := a.st.GetFetchedImageBySourceURL(ctx, src)
	if err != nil {
		return images.Image{}, mapImageErr(err)
	}
	return fromStoreImage(rec), nil
}

func (a imageStore) TouchImage(ctx context.Context, hash string, at time.Time) error {
	return a.st.TouchImage(ctx, hash, at)
}

func (a imageStore) PutRef(ctx context.Context, ref images.Ref) error {
	return a.st.PutImageRef(ctx, store.ImageRef{
		ImageHash: ref.ImageHash,
		OwnerKind: ref.OwnerKind,
		OwnerID:   ref.OwnerID,
		Role:      string(ref.Role),
	})
}

func (a imageStore) PutDerivative(ctx context.Context, d images.Derivative) error {
	return a.st.PutImageDerivative(ctx, store.ImageDerivative{
		ImageHash:  d.ImageHash,
		Recipe:     d.Recipe,
		Format:     string(d.Format),
		Width:      d.Width,
		Bytes:      d.Bytes,
		OutputHash: d.OutputHash,
		Path:       d.Path,
		Animated:   d.Animated,
		CreatedAt:  d.CreatedAt,
	})
}

func (a imageStore) PutDerivatives(ctx context.Context, derivatives []images.Derivative) error {
	rows := make([]store.ImageDerivative, 0, len(derivatives))
	for _, d := range derivatives {
		rows = append(rows, store.ImageDerivative{
			ImageHash: d.ImageHash, Recipe: d.Recipe, Format: string(d.Format), Width: d.Width,
			Bytes: d.Bytes, OutputHash: d.OutputHash, Path: d.Path, Animated: d.Animated,
			CreatedAt: d.CreatedAt,
		})
	}
	return a.st.PutImageDerivatives(ctx, rows)
}

func (a imageStore) ListDerivatives(ctx context.Context, hash string) ([]images.Derivative, error) {
	rows, err := a.st.ListImageDerivatives(ctx, hash)
	if err != nil {
		return nil, err
	}
	return fromStoreDerivatives(rows), nil
}

// --- The background jobs' half of the contract (V52 phase 3b) ---
//
// ⚠ The domain names these methods for what they DO to images (ListOrphans, ListExpiredBefore)
// while the store names them for the table (ListOrphanImages, ListImagesExpiredBefore), and the
// rename happens here rather than in either package. That is the same translation the type
// conversions above perform, one level up: `internal/images` reads as a subsystem talking about
// its own subject, and `internal/store` reads as a schema, because neither had to compromise to
// match the other.

func (a imageStore) ListAwaitingFetch(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesAwaitingFetch(ctx, limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListByOrigin(ctx context.Context, origin images.Origin, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesByOrigin(ctx, string(origin), limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListExpiredBefore(ctx context.Context, cutoff time.Time, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesExpiredBefore(ctx, cutoff, limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListOrphans(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListOrphanImages(ctx, limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListUnrecoverable(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListUnrecoverableImages(ctx, limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListMissingFormat(ctx context.Context, recipe string, f images.Format, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesMissingFormat(ctx, recipe, string(f), limit)
	return fromStoreImages(rows), err
}

func (a imageStore) ListColdestDerivatives(ctx context.Context, limit int) ([]images.Derivative, error) {
	rows, err := a.st.ListColdestDerivatives(ctx, limit)
	if err != nil {
		return nil, err
	}
	return fromStoreDerivatives(rows), nil
}

func (a imageStore) TotalDerivativeBytes(ctx context.Context) (int64, error) {
	return a.st.TotalImageDerivativeBytes(ctx)
}

func (a imageStore) DeleteDerivative(ctx context.Context, hash, recipe string, f images.Format, width int) error {
	return a.st.DeleteImageDerivative(ctx, hash, recipe, string(f), width)
}

func (a imageStore) DeleteDerivatives(ctx context.Context, hash string) error {
	return a.st.DeleteImageDerivatives(ctx, hash)
}

func (a imageStore) DeleteImage(ctx context.Context, hash string) error {
	return a.st.DeleteImage(ctx, hash)
}

func (a imageStore) RepointRefs(ctx context.Context, from, to string) error {
	return a.st.RepointImageRefs(ctx, from, to)
}

func fromStoreDerivatives(rows []store.ImageDerivative) []images.Derivative {
	out := make([]images.Derivative, 0, len(rows))
	for _, d := range rows {
		out = append(out, images.Derivative{
			ImageHash:  d.ImageHash,
			Recipe:     d.Recipe,
			Format:     images.Format(d.Format),
			Width:      d.Width,
			Bytes:      d.Bytes,
			OutputHash: d.OutputHash,
			Path:       d.Path,
			Animated:   d.Animated,
			CreatedAt:  d.CreatedAt,
		})
	}
	return out
}

func fromStoreImages(rows []store.Image) []images.Image {
	out := make([]images.Image, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromStoreImage(r))
	}
	return out
}

// mapImageErr translates the store's not-found into the domain's sentinel.
//
// ⚠ Load-bearing rather than cosmetic. `Adopt` branches on `errors.Is(err, images.ErrNotFound)` to
// decide "already adopted" from "a real failure", and the serve handler maps the sentinel to 404.
// A store error that arrived unmapped would make an adopt of an existing image look like a broken
// database, and would turn a missing image into a 500 on a public route.
func mapImageErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return images.ErrNotFound
	}
	return err
}

func toStoreImage(img images.Image) store.Image {
	return store.Image{
		Hash:            img.Hash,
		Origin:          string(img.Origin),
		SourceURL:       img.SourceURL,
		Visibility:      string(img.Visibility),
		Role:            string(img.Role),
		MIME:            img.MIME,
		Width:           img.Width,
		Height:          img.Height,
		Bytes:           img.Bytes,
		Animated:        img.Animated,
		FrameCount:      img.FrameCount,
		DurationMS:      img.DurationMS,
		LoopCount:       img.LoopCount,
		Placeholder:     img.Placeholder,
		DominantHex:     img.DominantHex,
		Meta:            img.Meta,
		OriginFetchedAt: img.OriginFetchedAt,
		CreatedAt:       img.CreatedAt,
		UpdatedAt:       img.UpdatedAt,
		LastUsedAt:      img.LastUsedAt,
	}
}

func fromStoreImage(rec store.Image) images.Image {
	return images.Image{
		Hash:            rec.Hash,
		Origin:          images.Origin(rec.Origin),
		SourceURL:       rec.SourceURL,
		Visibility:      images.Visibility(rec.Visibility),
		Role:            images.Role(rec.Role),
		MIME:            rec.MIME,
		Width:           rec.Width,
		Height:          rec.Height,
		Bytes:           rec.Bytes,
		Animated:        rec.Animated,
		FrameCount:      rec.FrameCount,
		DurationMS:      rec.DurationMS,
		LoopCount:       rec.LoopCount,
		Placeholder:     rec.Placeholder,
		DominantHex:     rec.DominantHex,
		Meta:            rec.Meta,
		OriginFetchedAt: rec.OriginFetchedAt,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.UpdatedAt,
		LastUsedAt:      rec.LastUsedAt,
	}
}

// artworkAdoptStore bridges the clips table onto images.ArtworkAdoptStore (§22, V52 phase 6).
//
// ⚠ **Resolving the artwork cache path is THIS layer's job, and that is why the store returns
// relative paths.** `internal/store` must not know where FILLER_DIR is — a query that joined a
// filesystem location onto a row would make the same SQL wrong the moment an operator moved the
// folder. `internal/images` must not know either; it takes absolute paths and never asks what
// produced them. The two vocabularies meet here, like every other translation in this file.
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
	cache := filepath.Join(dir, filler.ThumbDirName)

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
