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
	"github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// artFileMax bounds an artwork file read from a share. A real one is a few hundred KB to a few MB
// (a 4K fanart); the image service applies its own upload cap to what it keeps.
const artFileMax = 32 << 20

// artRefs is the image-ref half of the store (library_item_images): which image a library item owns in each role.
type artRefs interface {
	PutImageRef(ctx context.Context, ref clipstore.ImageRef) error
	DeleteImageRefsByRole(ctx context.Context, ownerKind, ownerID, role string) error
}

// artRoles is the image role each imported artwork kind takes (#2567): fanart is a title's
// backdrop, and a clear logo keeps its alpha on the logo ladder.
var artRoles = map[string]images.Role{
	metadataprovider.KindPoster:    images.RolePoster,
	metadataprovider.KindFanart:    images.RoleBackdrop,
	metadataprovider.KindClearlogo: images.RoleLogo,
}

// libraryArtwork gives the scanner's titles their artwork (#2151 step 10b, #2567). A file beside
// the media is imported now, keyed by its content; a provider's https URL is adopted and fetched by
// the image fetch job, which moves the title's ref onto the fetched bytes.
type libraryArtwork struct {
	images *images.Service
	refs   artRefs
}

// libraryArtworkOf is the scanner's Artwork, nil (not a typed nil) without an image service.
func libraryArtworkOf(svc *images.Service, refs artRefs) scan.Artwork {
	if svc == nil {
		return nil
	}
	return libraryArtwork{images: svc, refs: refs}
}

func (p libraryArtwork) Art(ctx context.Context, itemID, kind, ref string) error {
	role, ok := artRoles[kind]
	if !ok {
		return fmt.Errorf("library artwork: %q is not an imported kind", kind)
	}
	if ref == "" {
		return p.refs.DeleteImageRefsByRole(ctx, librarystore.LibraryItemImageOwner, itemID, string(role))
	}
	var (
		rec images.Image
		err error
	)
	switch {
	case strings.HasPrefix(ref, "https://"):
		rec, err = p.images.Adopt(ctx, ref, images.IngestRequest{Role: role, Visibility: images.VisibilityMember})
	case filepath.IsAbs(ref):
		rec, err = p.importFile(ctx, ref, role)
	default:
		return fmt.Errorf("library %s: %q is neither a file nor an https URL", kind, ref)
	}
	if err != nil {
		return err
	}
	if err := p.refs.DeleteImageRefsByRole(ctx, librarystore.LibraryItemImageOwner, itemID, string(role)); err != nil {
		return err
	}
	return p.refs.PutImageRef(ctx, clipstore.ImageRef{
		ImageHash: rec.Hash, OwnerKind: librarystore.LibraryItemImageOwner, OwnerID: itemID, Role: string(role),
	})
}

// importFile reads an artwork file beside the media. It opens only a regular file, never through
// a symlink, so a link cannot point the import outside the share. Bytes the service already holds
// are not imported again.
func (p libraryArtwork) importFile(ctx context.Context, path string, role images.Role) (images.Image, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return images.Image{}, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil {
		return images.Image{}, err
	} else if !info.Mode().IsRegular() {
		return images.Image{}, fmt.Errorf("library artwork: %s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, artFileMax+1))
	if err != nil {
		return images.Image{}, err
	}
	if len(data) > artFileMax {
		return images.Image{}, fmt.Errorf("library artwork: %s is over %d bytes", path, artFileMax)
	}
	if rec, err := p.images.Get(ctx, images.HashBytes(data)); err == nil {
		return rec, nil
	} else if !errors.Is(err, images.ErrNotFound) {
		return images.Image{}, err
	}
	// Extracted, not upload: losing the image directory loses nothing, the file is still beside
	// the media.
	return p.images.Ingest(ctx, bytes.NewReader(data), images.IngestRequest{
		Role: role, Visibility: images.VisibilityMember, Origin: images.OriginExtracted,
	})
}
