// Package storeadapter is the image service's store bridge (§22, V52): a type-for-type
// translation between the store's image rows and `internal/media/images`' own types.
//
// `internal/media/images` declares the persistence it needs with its OWN types (images.Store), and no
// domain package imports internal/platform/store directly. This package is the whole cost of that
// rule.
//
// ⚠ The translation is not merely structural — it crosses a TYPING boundary. The domain has
// closed types (images.Origin, images.Visibility, images.Role, images.Format) where the store has
// plain strings, because a database column is a string no matter what a Go program believes. Every
// value going down is `string(...)`; every value coming up is re-typed. That asymmetry is the
// point: an invalid role read from a row becomes an images.Role the domain can reason about
// (Role.NearestWidth falls back for an unknown value), rather than a panic at the seam.
package storeadapter

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Store is the slice of the store the image service reads and writes.
type Store interface {
	PutImage(ctx context.Context, img clipstore.Image) error
	GetImage(ctx context.Context, hash string) (clipstore.Image, error)
	GetFetchedImageBySourceURL(ctx context.Context, src string) (clipstore.Image, error)
	TouchImage(ctx context.Context, hash string, at time.Time) error
	PutImageRef(ctx context.Context, ref clipstore.ImageRef) error
	PutImageDerivative(ctx context.Context, d clipstore.ImageDerivative) error
	PutImageDerivatives(ctx context.Context, rows []clipstore.ImageDerivative) error
	ListImageDerivatives(ctx context.Context, hash string) ([]clipstore.ImageDerivative, error)
	ListImagesAwaitingFetch(ctx context.Context, limit int) ([]clipstore.Image, error)
	ListImagesByOrigin(ctx context.Context, origin string, limit int) ([]clipstore.Image, error)
	ListImagesExpiredBefore(ctx context.Context, cutoff time.Time, limit int) ([]clipstore.Image, error)
	ListOrphanImages(ctx context.Context, limit int) ([]clipstore.Image, error)
	ListUnrecoverableImages(ctx context.Context, limit int) ([]clipstore.Image, error)
	ListImagesMissingFormat(ctx context.Context, recipe, format string, limit int) ([]clipstore.Image, error)
	ListColdestDerivatives(ctx context.Context, limit int) ([]clipstore.ImageDerivative, error)
	TotalImageDerivativeBytes(ctx context.Context) (int64, error)
	DeleteImageDerivative(ctx context.Context, hash, recipe, format string, width int) error
	DeleteImageDerivatives(ctx context.Context, hash string) error
	DeleteImage(ctx context.Context, hash string) error
	RepointImageRefs(ctx context.Context, from, to string) error
}

// Images adapts the store to images.Store.
type Images struct{ st Store }

// New returns the images.Store over st.
func New(st Store) Images { return Images{st: st} }

func (a Images) PutImage(ctx context.Context, img images.Image) error {
	return a.st.PutImage(ctx, toStoreImage(img))
}

func (a Images) GetImage(ctx context.Context, hash string) (images.Image, error) {
	rec, err := a.st.GetImage(ctx, hash)
	if err != nil {
		return images.Image{}, mapErr(err)
	}
	return fromStoreImage(rec), nil
}

func (a Images) GetFetchedBySourceURL(ctx context.Context, src string) (images.Image, error) {
	rec, err := a.st.GetFetchedImageBySourceURL(ctx, src)
	if err != nil {
		return images.Image{}, mapErr(err)
	}
	return fromStoreImage(rec), nil
}

func (a Images) TouchImage(ctx context.Context, hash string, at time.Time) error {
	return a.st.TouchImage(ctx, hash, at)
}

func (a Images) PutRef(ctx context.Context, ref images.Ref) error {
	return a.st.PutImageRef(ctx, clipstore.ImageRef{
		ImageHash: ref.ImageHash,
		OwnerKind: ref.OwnerKind,
		OwnerID:   ref.OwnerID,
		Role:      string(ref.Role),
	})
}

func (a Images) PutDerivative(ctx context.Context, d images.Derivative) error {
	return a.st.PutImageDerivative(ctx, toStoreDerivative(d))
}

func (a Images) PutDerivatives(ctx context.Context, derivatives []images.Derivative) error {
	rows := make([]clipstore.ImageDerivative, 0, len(derivatives))
	for _, d := range derivatives {
		rows = append(rows, toStoreDerivative(d))
	}
	return a.st.PutImageDerivatives(ctx, rows)
}

func (a Images) ListDerivatives(ctx context.Context, hash string) ([]images.Derivative, error) {
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
// conversions above perform, one level up: `internal/media/images` reads as a subsystem talking about
// its own subject, and `internal/platform/store` reads as a schema, because neither had to compromise to
// match the other.

func (a Images) ListAwaitingFetch(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesAwaitingFetch(ctx, limit)
	return fromStoreImages(rows), err
}

func (a Images) ListByOrigin(ctx context.Context, origin images.Origin, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesByOrigin(ctx, string(origin), limit)
	return fromStoreImages(rows), err
}

func (a Images) ListExpiredBefore(ctx context.Context, cutoff time.Time, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesExpiredBefore(ctx, cutoff, limit)
	return fromStoreImages(rows), err
}

func (a Images) ListOrphans(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListOrphanImages(ctx, limit)
	return fromStoreImages(rows), err
}

func (a Images) ListUnrecoverable(ctx context.Context, limit int) ([]images.Image, error) {
	rows, err := a.st.ListUnrecoverableImages(ctx, limit)
	return fromStoreImages(rows), err
}

func (a Images) ListMissingFormat(ctx context.Context, recipe string, f images.Format, limit int) ([]images.Image, error) {
	rows, err := a.st.ListImagesMissingFormat(ctx, recipe, string(f), limit)
	return fromStoreImages(rows), err
}

func (a Images) ListColdestDerivatives(ctx context.Context, limit int) ([]images.Derivative, error) {
	rows, err := a.st.ListColdestDerivatives(ctx, limit)
	if err != nil {
		return nil, err
	}
	return fromStoreDerivatives(rows), nil
}

func (a Images) TotalDerivativeBytes(ctx context.Context) (int64, error) {
	return a.st.TotalImageDerivativeBytes(ctx)
}

func (a Images) DeleteDerivative(ctx context.Context, hash, recipe string, f images.Format, width int) error {
	return a.st.DeleteImageDerivative(ctx, hash, recipe, string(f), width)
}

func (a Images) DeleteDerivatives(ctx context.Context, hash string) error {
	return a.st.DeleteImageDerivatives(ctx, hash)
}

func (a Images) DeleteImage(ctx context.Context, hash string) error {
	return a.st.DeleteImage(ctx, hash)
}

func (a Images) RepointRefs(ctx context.Context, from, to string) error {
	return a.st.RepointImageRefs(ctx, from, to)
}

func toStoreDerivative(d images.Derivative) clipstore.ImageDerivative {
	return clipstore.ImageDerivative{
		ImageHash:  d.ImageHash,
		Recipe:     d.Recipe,
		Format:     string(d.Format),
		Width:      d.Width,
		Bytes:      d.Bytes,
		OutputHash: d.OutputHash,
		Path:       d.Path,
		Animated:   d.Animated,
		CreatedAt:  d.CreatedAt,
	}
}

func fromStoreDerivatives(rows []clipstore.ImageDerivative) []images.Derivative {
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

func fromStoreImages(rows []clipstore.Image) []images.Image {
	out := make([]images.Image, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromStoreImage(r))
	}
	return out
}

// mapErr translates the store's not-found into the domain's sentinel.
//
// ⚠ Load-bearing rather than cosmetic. `Adopt` branches on `errors.Is(err, images.ErrNotFound)` to
// decide "already adopted" from "a real failure", and the serve handler maps the sentinel to 404.
// A store error that arrived unmapped would make an adopt of an existing image look like a broken
// database, and would turn a missing image into a 500 on a public route.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return images.ErrNotFound
	}
	return err
}

func toStoreImage(img images.Image) clipstore.Image {
	return clipstore.Image{
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

func fromStoreImage(rec clipstore.Image) images.Image {
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
