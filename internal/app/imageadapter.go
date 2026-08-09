package app

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/store"
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
func newImageService(st store.Store, set resolved) *images.Service {
	return images.New(images.Config{
		Dir:            set.str("images.dir"),
		MaxUploadBytes: func() int64 { return int64(set.intv("images.max_upload_bytes")) },
		PublicBaseURL:  func() string { return set.str("server.public_url") },
		Formats:        func() []images.Format { return imageFormats(set) },
	}, imageStore{st}, nil)
}

// imageFormats reads `images.formats`, falling back to the declared ladder.
//
// ⚠ Unknown entries are DROPPED rather than passed through. Format is a closed set that decides
// which encoder runs and which MIME type is served; a typo like "webm" reaching the AVIF job as a
// format would be a work-list that never empties. Silently ignoring it degrades to a smaller
// ladder, which is a supported configuration.
func imageFormats(set resolved) []images.Format {
	raw := set.strlist("images.formats")
	if len(raw) == 0 {
		return images.DefaultFormats()
	}
	out := make([]images.Format, 0, len(raw))
	for _, s := range raw {
		switch f := images.Format(s); f {
		case images.FormatAVIF, images.FormatWebP, images.FormatJPEG:
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return images.DefaultFormats()
	}
	return out
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
		ImageHash: d.ImageHash,
		Format:    string(d.Format),
		Width:     d.Width,
		Bytes:     d.Bytes,
		Path:      d.Path,
		CreatedAt: d.CreatedAt,
	})
}

func (a imageStore) ListDerivatives(ctx context.Context, hash string) ([]images.Derivative, error) {
	rows, err := a.st.ListImageDerivatives(ctx, hash)
	if err != nil {
		return nil, err
	}
	out := make([]images.Derivative, 0, len(rows))
	for _, d := range rows {
		out = append(out, images.Derivative{
			ImageHash: d.ImageHash,
			Format:    images.Format(d.Format),
			Width:     d.Width,
			Bytes:     d.Bytes,
			Path:      d.Path,
			CreatedAt: d.CreatedAt,
		})
	}
	return out, nil
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
		Placeholder:     rec.Placeholder,
		DominantHex:     rec.DominantHex,
		Meta:            rec.Meta,
		OriginFetchedAt: rec.OriginFetchedAt,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.UpdatedAt,
		LastUsedAt:      rec.LastUsedAt,
	}
}
