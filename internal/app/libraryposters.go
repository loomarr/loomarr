package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/store/catalogstore"
)

// posterFileMax bounds a poster file read from a share. A real one is a few hundred KB; the image
// service applies its own, smaller upload cap to what it keeps.
const posterFileMax = 32 << 20

// posterRefs is the image_refs half of the store: which image a library item owns as its poster.
type posterRefs interface {
	PutImageRef(ctx context.Context, ref catalogstore.ImageRef) error
	DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error
}

// libraryPosters gives the scanner's titles their posters (#2151 step 10b). A file beside the
// media is imported now, keyed by its content; a manager's https URL is adopted and fetched by the
// image fetch job, which moves the title's ref onto the fetched bytes.
type libraryPosters struct {
	images *images.Service
	refs   posterRefs
}

// libraryPostersOf is the scanner's Posters, nil (not a typed nil) without an image service.
func libraryPostersOf(svc *images.Service, refs posterRefs) scan.Posters {
	if svc == nil {
		return nil
	}
	return libraryPosters{images: svc, refs: refs}
}

func (p libraryPosters) Poster(ctx context.Context, itemID, ref string) error {
	if ref == "" {
		return p.refs.DeleteImageRefs(ctx, store.LibraryItemImageOwner, itemID)
	}
	var (
		rec images.Image
		err error
	)
	switch {
	case strings.HasPrefix(ref, "https://"):
		rec, err = p.images.Adopt(ctx, ref, images.IngestRequest{Role: images.RolePoster, Visibility: images.VisibilityMember})
	case filepath.IsAbs(ref):
		rec, err = p.importFile(ctx, ref)
	default:
		return fmt.Errorf("library poster: %q is neither a file nor an https URL", ref)
	}
	if err != nil {
		return err
	}
	if err := p.refs.DeleteImageRefs(ctx, store.LibraryItemImageOwner, itemID); err != nil {
		return err
	}
	return p.refs.PutImageRef(ctx, catalogstore.ImageRef{
		ImageHash: rec.Hash, OwnerKind: store.LibraryItemImageOwner, OwnerID: itemID, Role: string(images.RolePoster),
	})
}

// importFile reads a poster beside the media. It opens only a regular file, never through a
// symlink, so a link cannot point the import outside the share. Bytes the service already holds
// are not imported again.
func (p libraryPosters) importFile(ctx context.Context, path string) (images.Image, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return images.Image{}, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil {
		return images.Image{}, err
	} else if !info.Mode().IsRegular() {
		return images.Image{}, fmt.Errorf("library poster: %s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, posterFileMax+1))
	if err != nil {
		return images.Image{}, err
	}
	if len(data) > posterFileMax {
		return images.Image{}, fmt.Errorf("library poster: %s is over %d bytes", path, posterFileMax)
	}
	if rec, err := p.images.Get(ctx, images.HashBytes(data)); err == nil {
		return rec, nil
	} else if !errors.Is(err, images.ErrNotFound) {
		return images.Image{}, err
	}
	// Extracted, not upload: losing the image directory loses nothing, the file is still beside
	// the media.
	return p.images.Ingest(ctx, bytes.NewReader(data), images.IngestRequest{
		Role: images.RolePoster, Visibility: images.VisibilityMember, Origin: images.OriginExtracted,
	})
}
