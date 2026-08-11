package api

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

	"github.com/mantonx/loomarr/internal/images"
)

// The image service's HTTP surface (§22, V52).
//
// Three operations: serve one rendition's bytes, read one image's record, upload one image. All
// three are Huma operations — the byte route through `rawOp` (rawop.go), which is what keeps it
// behind the single authorization middleware and the CSRF check while still owning the
// ResponseWriter that `http.ServeContent` needs for Range and conditional requests.

// ImageService is the api package's view of internal/images. Narrow by design: the handlers need
// to resolve a rendition, read a record, and accept an upload — nothing about disk layout,
// encoders, or the ladder crosses this line.
type ImageService interface {
	Get(ctx context.Context, hash string) (images.Image, error)
	Rendition(ctx context.Context, hash string, f images.Format, width int) (images.Rendition, error)
	Ingest(ctx context.Context, r io.Reader, req images.IngestRequest) (images.Image, error)
	URLFor(hash string, width int, f images.Format) string
	SrcSet(hash string, role images.Role, f images.Format) string
	// HasFormat reports whether a rendition in this format exists RIGHT NOW. Required because
	// advertising a job-produced AVIF that does not exist yet is a broken image, not a missed
	// optimisation — see the method's own comment in internal/images.
	HasFormat(ctx context.Context, hash string, f images.Format) (bool, error)
}

// registerImages mounts the image operations.
//
// ⚠ Must be added to BOTH `api.go` and `export.go`. Two parallel register lists exist and a route
// in one but not the other is invisible in the direction it was missed — live but undocumented
// (orval cannot type it) or documented but unmounted (the generated client calls a 404).
// `TestRegisterListsMatchBetweenRouterAndExporter` guards it.
func (s *Server) registerImages(api huma.API) {
	// ⚠ **RolePublic at the MOUNT, per-row visibility in the HANDLER**, and this is the one place
	// the image service departs from every other raw-byte route in §7.1.
	//
	// Visibility is a property of the IMAGE, not the route: a channel icon must be fetchable by
	// Tunarr machine-to-machine with no credentials, exactly as it would fetch a TMDB poster,
	// while a clip still must not be. A fixed role cannot express both, and two routes over one
	// serve path would be two authorization implementations to keep in step — which is how
	// `backupHandler` and `eventsHandler` ended up disagreeing about nil authorizers.
	rawOp[serveImageInput](api, bytesResponse(huma.Operation{
		OperationID: "serve-image",
		Method:      http.MethodGet,
		Path:        "/v1/images/{hash}/{rendition}",
		Summary:     "Serve one image rendition",
		Description: "Returns the bytes of one rendition (a width in a format) of a stored image. " +
			"Content-addressed, so the response is immutable and cacheable forever.",
		Tags: []string{"Images"},
	}, "The image bytes", "image/avif", "image/webp", "image/jpeg"),
		RolePublic, s.serveImage)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-image",
		Method:      http.MethodGet,
		Path:        "/v1/images/{hash}",
		Summary:     "Read an image's record",
		Description: "Dimensions, placeholder and per-format srcset strings for one image — " +
			"everything an <img> needs to render without layout shift.",
		Tags: []string{"Images"},
	}, RoleMember), s.getImage)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "upload-image",
		Method:      http.MethodPost,
		Path:        "/v1/images",
		Summary:     "Upload an image",
		Description: "Stores an image and returns its record. Raster only — SVG is refused.",
		Tags:        []string{"Images"},
	}, RoleAdmin), s.uploadImage)
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
func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
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
	if rec.Visibility != images.VisibilityPublic && roleFrom(r.Context()) == RoleAnonymous {
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
	// nosniff so a browser cannot reinterpret the bytes as HTML/script regardless of the declared
	// type — defence in depth even though the input allowlist is raster-only.
	w.Header().Set("X-Content-Type-Options", "nosniff")
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

// ImageDTO is one image as the frontend sees it.
type ImageDTO struct {
	Hash string `json:"hash" doc:"Content hash — the image's identity and its ETag"`
	Role string `json:"role" doc:"poster | backdrop | icon | thumb — decides the width ladder"`
	// Width/Height are the ORIGINAL's. ⚠ Served so an <img> can carry real width/height
	// attributes: browsers derive aspect-ratio from them, which takes cumulative layout shift to
	// zero without the frontend guessing a shape per surface.
	Width  int `json:"width" doc:"The original's width in pixels"`
	Height int `json:"height" doc:"The original's height in pixels"`
	// Placeholder is a base64 ThumbHash (~25 bytes) — the blur preview rendered while the real
	// image loads. Carries alpha, which is why it is not a BlurHash.
	Placeholder string `json:"placeholder" doc:"Base64 ThumbHash for the low-quality placeholder"`
	DominantHex string `json:"dominantHex" doc:"Average colour, for tinting a skeleton"`
	Animated    bool   `json:"animated" doc:"True when the original carries motion; such images have one rendition"`
	// SrcSetAVIF may be empty while the AVIF job catches up — that is a normal state, not an
	// error, and the frontend omits the <source> rather than waiting.
	SrcSetAVIF string `json:"srcSetAvif" doc:"srcset for AVIF; empty until the AVIF job has run"`
	SrcSetWebP string `json:"srcSetWebp" doc:"srcset for WebP"`
	Src        string `json:"src" doc:"A single JPEG URL for the <img> fallback"`
}

type getImageInput struct {
	Hash string `path:"hash" example:"9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503"`
}

type getImageOutput struct{ Body ImageDTO }

func (s *Server) getImage(ctx context.Context, in *getImageInput) (*getImageOutput, error) {
	if s.images == nil {
		return nil, errNotFound("Image not found", "That image doesn't exist.")
	}
	rec, err := s.images.Get(ctx, in.Hash)
	if err != nil {
		return nil, errNotFound("Image not found", "That image doesn't exist — it may have been removed.")
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
func (s *Server) imageToDTO(ctx context.Context, rec images.Image) ImageDTO {
	// ⚠ AVIF is advertised only when a rendition actually EXISTS. <picture> commits to the source
	// it selects by type and does not fall back on a 404, so a job-produced AVIF that has not been
	// generated yet breaks the image outright rather than degrading to WebP. A lookup failure is
	// treated as "no AVIF" for the same reason: the safe direction is to under-advertise.
	avif := ""
	if ok, err := s.images.HasFormat(ctx, rec.Hash, images.FormatAVIF); err == nil && ok {
		avif = s.images.SrcSet(rec.Hash, rec.Role, images.FormatAVIF)
	}
	return ImageDTO{
		Hash:        rec.Hash,
		Role:        string(rec.Role),
		Width:       rec.Width,
		Height:      rec.Height,
		Placeholder: rec.Placeholder,
		DominantHex: rec.DominantHex,
		Animated:    rec.Animated,
		SrcSetAVIF:  avif,
		SrcSetWebP:  s.images.SrcSet(rec.Hash, rec.Role, images.FormatWebP),
		Src:         s.images.URLFor(rec.Hash, rec.Role.NearestWidth(rec.Width), images.FormatJPEG),
	}
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
	OwnerKind  string `query:"ownerKind" doc:"What this image decorates, e.g. channel"`
	OwnerID    string `query:"ownerId" doc:"The id of the thing it decorates"`
	RawBody    huma.MultipartFormFiles[imageUploadForm]
}

type uploadImageOutput struct{ Body ImageDTO }

func (s *Server) uploadImage(ctx context.Context, in *uploadImageInput) (*uploadImageOutput, error) {
	if s.images == nil {
		return nil, apiErr(http.StatusNotImplemented, "Images aren't configured",
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
		return nil, apiErr(http.StatusUnsupportedMediaType, "Couldn't accept that image",
			"Use a PNG, JPEG, WebP or GIF under the size limit. SVG isn't accepted.")
	}

	// ⚠ Through imageToDTO, NOT a second hand-written copy. This handler HAD one, and that is
	// exactly how the AVIF bug survived: the projection existed twice, so gating one did nothing
	// for the other. One constructor is the fix for the class, not just for this instance.
	return &uploadImageOutput{Body: s.imageToDTO(ctx, rec)}, nil
}
