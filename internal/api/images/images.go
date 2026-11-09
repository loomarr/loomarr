package images

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/media/images"
)

// The image service's HTTP surface (§22, V52).
//
// Three operations: serve one rendition's bytes, read one image's record, upload one image. All
// three are Huma operations — the byte route through `httpcore.RawOp`, which is what keeps it
// behind the single authorization middleware and the CSRF check while still owning the
// ResponseWriter that `http.ServeContent` needs for Range and conditional requests.

// ImageService is this package's view of internal/media/images. Narrow by design: the handlers need
// to resolve a rendition, read a record, and accept an upload — nothing about disk layout,
// encoders, or the ladder crosses this line.
type ImageService interface {
	Get(ctx context.Context, hash string) (images.Image, error)
	Rendition(ctx context.Context, hash string, f images.Format, width int) (images.Rendition, error)
	Ingest(ctx context.Context, r io.Reader, req images.IngestRequest) (images.Image, error)
	// Adopt records a remote https image for the fetch job and returns at once; the image has its
	// bytes only once the job fetched them.
	Adopt(ctx context.Context, srcURL string, req images.IngestRequest) (images.Image, error)
	PathFor(hash string, width int, f images.Format) string
	URLFor(hash string, width int, f images.Format) string
	SrcSet(hash string, role images.Role, f images.Format) string
	// HasFormat reports whether a rendition in this format exists RIGHT NOW. Required because
	// advertising a job-produced AVIF that does not exist yet is a broken image, not a missed
	// optimisation — see the method's own comment in internal/media/images.
	HasFormat(ctx context.Context, hash string, f images.Format) (bool, error)
}

// registerImages mounts the image operations.
//
// ⚠ Mounted through `Handlers.Register`, which BOTH `api.go` and `export.go` call. Two parallel
// register lists exist and a handler set in one but not the other is invisible in the direction
// it was missed — live but undocumented (orval cannot type it) or documented but unmounted (the
// generated client calls a 404). `TestRegisterListsMatchBetweenRouterAndExporter` guards it.
func (s *Handlers) registerImages(api huma.API) {
	// ⚠ **RolePublic at the MOUNT, per-row visibility in the HANDLER**, and this is the one place
	// the image service departs from every other raw-byte route in §7.1.
	//
	// Visibility is a property of the IMAGE, not the route: a channel icon must be fetchable by
	// Tunarr machine-to-machine with no credentials, exactly as it would fetch a TMDB poster,
	// while a clip still must not be. A fixed role cannot express both, and two routes over one
	// serve path would be two authorization implementations to keep in step — which is how
	// `backupHandler` and `eventsHandler` ended up disagreeing about nil authorizers.
	httpcore.RawOp[serveImageInput](api, httpcore.BytesResponse(huma.Operation{
		OperationID: "serve-image",
		Method:      http.MethodGet,
		Path:        "/v1/images/{hash}/{rendition}",
		Summary:     "Serve one image rendition",
		Description: "Returns the bytes of one rendition (a width in a format) of a stored image. " +
			"Content-addressed, so the response is immutable and cacheable forever.",
		Tags: []string{"Images"},
	}, "The image bytes", "image/avif", "image/webp", "image/jpeg"),
		httpcore.RolePublic, s.serveImage)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-image",
		Method:      http.MethodGet,
		Path:        "/v1/images/{hash}",
		Summary:     "Read an image's record",
		Description: "Dimensions, placeholder and per-format srcset strings for one image — " +
			"everything an <img> needs to render without layout shift.",
		Tags: []string{"Images"},
	}, httpcore.RoleMember), s.getImage)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "upload-image",
		Method:      http.MethodPost,
		Path:        "/v1/images",
		Summary:     "Upload an image",
		Description: "Stores an image and returns its record. Raster only — SVG is refused.",
		Tags:        []string{"Images"},
	}, httpcore.RoleAdmin), s.uploadImage)
}

type serveImageInput struct {
	Hash string `path:"hash" doc:"The image's content hash" example:"9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503"`
	// ⚠ Declared even though the handler reads it via r.PathValue: Huma builds the spec's
	// parameter list from this struct and does NOT cross-check it against the {placeholders} in
	// Path, so an undeclared one emits a path template whose parameter is defined nowhere —
	// invalid OpenAPI 3.1, and a generated client that cannot fill in the URL.
	Rendition string `path:"rendition" doc:"Width and format, as w<width>.<ext>" example:"w342.webp"`
}

// serveImage streams one rendition.
func (s *Handlers) serveImage(w http.ResponseWriter, r *http.Request) {
	if s.images == nil {
		http.NotFound(w, r)
		return
	}
	hash := r.PathValue("hash")

	width, format, ok := parseRendition(r.PathValue("rendition"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	rec, err := s.images.Get(r.Context(), hash)
	if err != nil {
		// ⚠ Every miss is 404 — absent, malformed, and not-visible-to-you alike. A distinct
		// status would confirm which hashes exist to someone enumerating them, and the hash space
		// being unguessable is part of what makes the public mount safe.
		http.NotFound(w, r)
		return
	}

	// The per-row visibility check. `roleFrom` reads the role the shared authorization middleware
	// already resolved onto the request context, so this is the same identity every typed
	// operation sees — not a second auth implementation.
	//
	// A member-visible image without a session is a 404 for the same reason as above, never a 403.
	if rec.Visibility != images.VisibilityPublic && httpcore.RoleFrom(r.Context()) == httpcore.RoleAnonymous {
		http.NotFound(w, r)
		return
	}

	rend, err := s.images.Rendition(r.Context(), hash, format, width)
	if err != nil {
		// A missing AVIF is the NORMAL case, not an error: it is job-produced, so the frontend's
		// <picture> simply omits that <source> and the browser takes WebP. Logging here would
		// fill the log with the system working correctly.
		if !errors.Is(err, images.ErrNotFound) && s.log != nil {
			s.log.Error("serve image rendition", "hash", hash, "err", err)
		}
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filepath.Clean(rend.Path)) //nolint:gosec // path built from a validated hash
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", rend.ContentType)
	// ⚠ Immutable is HONEST here, unlike on a mutable path: the URL contains the content hash, so
	// different bytes are a different URL by construction. A strong ETag equal to the hash costs
	// nothing (we already have it) and is the correct validator for the cold-cache case.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+rend.Hash+`"`)

	// ⚠ The zero time suppresses Last-Modified deliberately. It invites heuristic freshness and
	// adds nothing when the URL already changes with the content — and ServeContent would
	// otherwise emit one from the file's mtime, which is a RE-ENCODE date rather than a content
	// date and would differ between two hosts holding byte-identical images.
	//
	// ServeContent still handles Range and If-None-Match against the ETag set above, which is
	// what makes a 304 cost nothing.
	_ = info
	http.ServeContent(w, r, filepath.Base(rend.Path), time.Time{}, f)
}

// parseRendition splits "w342.webp" into a width and a format.
//
// ⚠ The format is matched against a CLOSED SET rather than derived from the extension. An
// arbitrary extension would let a caller name a file we would then try to open — and the width is
// snapped to the role's ladder downstream, so between them the set of files one image can produce
// is finite and known.
func parseRendition(s string) (int, images.Format, bool) {
	ext := strings.TrimPrefix(filepath.Ext(s), ".")
	name := strings.TrimSuffix(s, "."+ext)
	if !strings.HasPrefix(name, "w") {
		return 0, "", false
	}
	width, err := strconv.Atoi(strings.TrimPrefix(name, "w"))
	if err != nil || width <= 0 {
		return 0, "", false
	}
	switch ext {
	case "avif":
		return width, images.FormatAVIF, true
	case "webp":
		return width, images.FormatWebP, true
	case "jpg":
		return width, images.FormatJPEG, true
	}
	return 0, "", false
}

type getImageInput struct {
	Hash string `path:"hash" example:"9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503"`
}

type getImageOutput struct{ Body httpcore.ImageDTO }

func (s *Handlers) getImage(ctx context.Context, in *getImageInput) (*getImageOutput, error) {
	if s.images == nil {
		return nil, httpcore.NotFound("Image not found", "That image doesn't exist.")
	}
	rec, err := s.images.Get(ctx, in.Hash)
	if err != nil {
		return nil, httpcore.NotFound("Image not found", "That image doesn't exist — it may have been removed.")
	}
	return &getImageOutput{Body: s.imageToDTO(ctx, rec)}, nil
}

// imageToDTO renders an image record for the wire.
//
// ⚠ Extracted so there is ONE construction of this DTO. It is embedded on a channel as well as
// served standalone (V52 phase 5), and two hand-written copies of the same projection is the drift
// class this repo has already been bitten by more than once — the second copy is the one that
// forgets `srcSetAvif` when the AVIF job lands, and nothing fails, it just quietly serves WebP
// forever on one surface.
func (s *Handlers) imageToDTO(ctx context.Context, rec images.Image) httpcore.ImageDTO {
	// ⚠ AVIF is advertised only when a rendition actually EXISTS. <picture> commits to the source
	// it selects by type and does not fall back on a 404, so a job-produced AVIF that has not been
	// generated yet breaks the image outright rather than degrading to WebP. A lookup failure is
	// treated as "no AVIF" for the same reason: the safe direction is to under-advertise.
	avif := ""
	if !rec.Animated {
		if ok, err := s.images.HasFormat(ctx, rec.Hash, images.FormatAVIF); err == nil && ok {
			avif = s.images.SrcSet(rec.Hash, rec.Role, images.FormatAVIF)
		}
	}
	webp := s.images.SrcSet(rec.Hash, rec.Role, images.FormatWebP)
	if rec.Animated {
		// One URL and its honest intrinsic width. The animated original is already card-sized and
		// the service deliberately does not resize it; advertising the still-image ladder would
		// cache the same loop under several width URLs and give the browser false descriptors.
		webp = s.images.PathFor(rec.Hash, rec.Width, images.FormatWebP) + " " + strconv.Itoa(rec.Width) + "w"
	}
	return httpcore.ImageDTO{
		Hash:        rec.Hash,
		Role:        string(rec.Role),
		Width:       rec.Width,
		Height:      rec.Height,
		Placeholder: rec.Placeholder,
		DominantHex: rec.DominantHex,
		Animated:    rec.Animated,
		SrcSetAVIF:  avif,
		SrcSetWebP:  webp,
		Src:         s.images.PathFor(rec.Hash, rec.Role.NearestWidth(rec.Width), rec.Role.Fallback()),
	}
}

// libraryImage renders one of a title's images by hash: nil for none, or while its bytes have not
// arrived, so a surface draws without it rather than showing a broken image.
func (s *Handlers) LibraryImage(ctx context.Context, hash string) *httpcore.ImageDTO {
	if s.images == nil || hash == "" {
		return nil
	}
	rec, err := s.images.Get(ctx, hash)
	if err != nil || !rec.HasBytes() {
		return nil
	}
	dto := s.imageToDTO(ctx, rec)
	return &dto
}

// libraryCandidateImageOwner owns the posters of a title's candidates, so the image service keeps
// them while the title is listed.
const libraryCandidateImageOwner = "library_candidate"

// candidatePoster adopts a candidate's poster, so the image service fetches it, and renders it once
// its bytes are here. Until then, or when it cannot be adopted, the card shows the name.
func (s *Handlers) CandidatePoster(ctx context.Context, itemID, url string) *httpcore.ImageDTO {
	if s.images == nil || url == "" {
		return nil
	}
	rec, err := s.images.Adopt(ctx, url, images.IngestRequest{
		Role: images.RolePoster, Visibility: images.VisibilityMember,
		OwnerKind: libraryCandidateImageOwner, OwnerID: itemID,
	})
	if err != nil || !rec.HasBytes() {
		return nil
	}
	dto := s.imageToDTO(ctx, rec)
	return &dto
}

// imageDTOsByHash resolves a set of image hashes in ONE pass, deduplicated.
//
// ⚠ **Every list endpoint that renders images must go through this rather than looking one up per
// row.** A per-row lookup is an N+1 — the shape a profile here has already caught once — and it
// gets worse as the surface gets bigger: a channel list is tens of rows, but the filler catalog is
// a GRID of clips, each with a still and a hover loop, so the naive version is two queries per
// tile. Distinct hashes are fetched once each even when many rows share one.
//
// ⚠ A failed lookup is recorded as a nil ENTRY rather than skipped, so a shared broken hash costs
// one lookup instead of one per referencing row. And a miss is never an error: §22's Durability
// section makes "the row is here, the file is not" an accepted state, so a surface must still
// render with its own fallback artwork.
func (s *Handlers) ImageDTOsByHash(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO {
	if s.images == nil || len(hashes) == 0 {
		return nil
	}
	byHash := make(map[string]*httpcore.ImageDTO, len(hashes))
	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		if _, seen := byHash[hash]; seen {
			continue
		}
		byHash[hash] = nil
		rec, err := s.images.Get(ctx, hash)
		if err != nil {
			continue
		}
		dto := s.imageToDTO(ctx, rec)
		byHash[hash] = &dto
	}
	return byHash
}

// imageUploadForm is the multipart body.
//
// ⚠ The `contentType` tag is a first-pass filter, NOT the security boundary. Huma's validator
// trusts the Content-Type the client DECLARES on the part and only sniffs when it is absent, so a
// caller can label anything `image/png` and pass. The authority is the byte sniff inside
// images.Ingest. Listing them here still earns its place: it puts the accepted set in the spec and
// rejects obvious mistakes before the body is read.
type imageUploadForm struct {
	File huma.FormFile `form:"file" contentType:"image/png,image/jpeg,image/webp,image/gif" required:"true" doc:"The image. Raster only — SVG is refused."`
}

type uploadImageInput struct {
	Role       string `query:"role" doc:"poster | backdrop | icon | thumb" example:"icon"`
	Visibility string `query:"visibility" doc:"public | member" example:"member"`
	OwnerKind  string `query:"ownerKind" enum:"clip,filler_split_proposal,channel,library_item,library_candidate" doc:"What kind of thing this image decorates; ownerId names which one, and it must exist"`
	OwnerID    string `query:"ownerId" doc:"The id of the thing it decorates"`
	RawBody    huma.MultipartFormFiles[imageUploadForm]
}

type uploadImageOutput struct{ Body httpcore.ImageDTO }

func (s *Handlers) uploadImage(ctx context.Context, in *uploadImageInput) (*uploadImageOutput, error) {
	if s.images == nil {
		return nil, httpcore.Problem(http.StatusNotImplemented, "Images aren't configured",
			"The image service isn't wired on this instance.")
	}

	file := in.RawBody.Data().File
	rec, err := s.images.Ingest(ctx, file, images.IngestRequest{
		Role:       images.Role(in.Role),
		Visibility: images.Visibility(in.Visibility),
		Origin:     images.OriginUpload,
		OwnerKind:  in.OwnerKind,
		OwnerID:    in.OwnerID,
	})
	if err != nil {
		// Ingest refuses on size and on format, and both are the caller's problem rather than
		// ours — a 4xx with the reason, never a 500.
		return nil, httpcore.Problem(http.StatusUnsupportedMediaType, "Couldn't accept that image",
			"Use a PNG, JPEG, WebP or GIF under the size limit. SVG isn't accepted.")
	}

	// ⚠ Through imageToDTO, NOT a second hand-written copy: with the projection in two places, a
	// rule gated in one (the AVIF gate) does nothing for the other. One constructor covers the class.
	return &uploadImageOutput{Body: s.imageToDTO(ctx, rec)}, nil
}
