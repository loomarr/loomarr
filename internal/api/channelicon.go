package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Channel icon upload + serve. Both are Huma operations registered in registerChannels: the
// serve is a rawOp (it returns image bytes with their own Content-Type), the upload takes a
// huma.MultipartFormFiles body. The bytes live in the DB (channel_icons), so there's no
// writable-asset-dir config and the icon rides the §16 backup.
//
// Flow: the FE POSTs an image → uploadChannelIcon stores the bytes + sets the channel's
// logo to the absolute serve URL → the reconcile pushes that URL to Tunarr's channel icon →
// Tunarr fetches GET /v1/channels/{id}/icon directly. The serve URL is built from the upload
// request's Host (the reachable address the admin's browser used), which on a homelab is the
// same host/IP Tunarr resolves — so no new public-base-URL setting is needed. An operator on
// an unusual topology can always PATCH an explicit logo URL instead.

// maxIconBytes caps an upload at 2 MiB — a channel icon is a small poster/logo; this fences
// a hostile or accidental large upload out of the DB blob without a config knob.
const maxIconBytes = 2 << 20

// allowedIconTypes is the MIME allowlist for an uploaded icon — RASTER ONLY. SVG is
// deliberately excluded: the serve endpoint is public and returns the bytes with their
// image content-type, and an uploaded SVG can carry <script> that executes when the icon
// URL is opened in a browser (stored XSS in Loomarr's origin). Raster covers every real
// channel icon (TMDB posters are JPEG), so dropping SVG removes the XSS class entirely
// rather than trying to sanitize/sandbox it.
var allowedIconTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// iconUploadForm is the multipart body: one image in the `file` field.
//
// ⚠ **The `contentType` tag is a first-pass filter, NOT the security boundary.** Huma's
// MimeTypeValidator trusts the Content-Type the client DECLARES on the part and only sniffs
// when that header is absent — so a caller can label anything `image/png` and pass this check.
// The authority is the byte sniff below (iconContentType → http.DetectContentType), which is
// what actually keeps SVG out. Listing the types here still earns its place: it puts the
// accepted set in the spec and rejects the obvious mistakes before we read the body.
type iconUploadForm struct {
	File huma.FormFile `form:"file" contentType:"image/png,image/jpeg,image/webp,image/gif" required:"true" doc:"The icon image. Raster only, under 2 MB — SVG is refused."`
}

type uploadIconInput struct {
	ID      string `path:"id" example:"ch_abc123"`
	RawBody huma.MultipartFormFiles[iconUploadForm]
}

type uploadIconOutput struct {
	Body struct {
		Logo string `json:"logo" doc:"The absolute serve URL now stored on the channel"`
	}
}

// uploadChannelIcon handles POST /v1/channels/{id}/icon (multipart, field "file"). Admin-only
// (an authoring action; the server enforces regardless of the FE). Stores the bytes, points
// the channel's logo at the serve URL, and persists — the auto-reconcile on the next sweep (or
// an explicit reconcile) pushes it to Tunarr.
//
// ⚠ **The CSRF check that used to live here is gone because the middleware now covers it.**
// It existed only because this was a raw mux handler outside registerMiddleware; a multipart
// POST is the classic CSRF vector, and the guard was hand-rolled to compensate. As a Huma
// operation the same rule applies from one place (titles.go), so keeping a second copy would
// mean two implementations of one rule to keep in step — which is how backupHandler and
// eventsHandler ended up disagreeing about nil authorizers.
func (s *Server) uploadChannelIcon(ctx context.Context, in *uploadIconInput) (*uploadIconOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if err != nil {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}

	file := in.RawBody.Data().File
	if file.Size > maxIconBytes {
		return nil, apiErr(http.StatusRequestEntityTooLarge, "Image too large",
			"Channel icons must be under 2 MB.")
	}
	// Still bounded on read: Size is the multipart header's claim, and the operation's
	// MaxBodyBytes is the outer fence. This is the inner one.
	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes+1))
	if err != nil {
		return nil, errBadRequest("Couldn't read the image", "The upload was incomplete. Try again.")
	}
	if len(data) > maxIconBytes {
		return nil, apiErr(http.StatusRequestEntityTooLarge, "Image too large",
			"Channel icons must be under 2 MB.")
	}

	// ⚠ **THE security check.** Sniffs the actual bytes and ignores every client claim —
	// the declared part Content-Type, the filename, all of it. An SVG sniffs as text/xml and
	// is refused here even though huma's tag validator would have accepted it labelled
	// image/png. That matters because the serve half is PUBLIC and hands these bytes back
	// with the stored content type: an SVG carrying <script> would be stored XSS in Loomarr's
	// own origin. Raster-only removes the class instead of trying to sanitize it.
	ct := iconContentType("", data)
	if !allowedIconTypes[ct] {
		return nil, apiErr(http.StatusUnsupportedMediaType, "Unsupported image type",
			"Use a PNG, JPEG, WebP, or GIF image. SVG isn't accepted.")
	}

	now := time.Now()
	if err := s.store.PutChannelIcon(ctx, in.ID, ct, data, now); err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't save the icon",
			"Something went wrong storing the image. Try again.", err)
	}

	// Point the channel at the serve URL, cache-busted by the upload time so Tunarr/Emby
	// refetch a replaced icon instead of serving a stale cached image.
	ch.Logo = s.iconServeURL(in.ID, now)
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't update the channel",
			"The icon was saved but linking it to the channel failed. Try again.", err)
	}

	// Auto-reconcile so the new icon reaches Tunarr without a manual step (the seamless
	// edit model, §9). Best-effort — a reconcile failure doesn't fail the upload; the next
	// sweep pushes it. Only when a channel service is wired (nil in some unit setups).
	if s.channels != nil {
		if err := s.channels.Reconcile(ctx, in.ID); err != nil && s.log != nil {
			s.log.Warn("icon upload reconcile (icon saved; next sweep pushes it)", "channel", in.ID, "err", err)
		}
	}

	out := &uploadIconOutput{}
	out.Body.Logo = ch.Logo
	return out, nil
}

// serveChannelIcon handles GET /v1/channels/{id}/icon — the raw image Tunarr/Emby fetch.
// Public (no auth): Tunarr fetches it machine-to-machine with no credentials, exactly like
// it fetches a TMDB poster URL; the bytes are a non-secret channel icon. A missing icon 404s
// (the channel uses a TMDB/URL logo, or none).
func (s *Server) serveChannelIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ct, data, updatedAt, ok, err := s.store.GetChannelIcon(r.Context(), id)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't load the icon", "Something went wrong reading the image.")
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// nosniff so a browser can't re-interpret the bytes as HTML/script regardless of the
	// declared type — defense in depth even though the allowlist is raster-only (no SVG).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A stored icon is immutable for its updated_at; the ?v= cache-bust changes on re-upload,
	// so a long cache is safe and keeps Tunarr/Emby from refetching every guide build.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Last-Modified", updatedAt.UTC().Format(http.TimeFormat))
	_, _ = w.Write(data)
}

// iconServeURL builds the icon URL stored on the channel + pushed to Tunarr. It is derived
// from the OPERATOR-CONFIGURED server.public_url — NOT from request headers (r.Host /
// X-Forwarded-Host are attacker-controllable, and this URL is stored and fetched downstream
// by Tunarr, so a spoofed header could poison it). The ?v= is the upload epoch so a replaced
// icon gets a fresh URL that busts any downstream cache.
//
// When server.public_url is unset, fall back to a RELATIVE URL. That still works when Tunarr
// resolves Loomarr at the same origin the guide is served from, and is safe (no header
// trust); the setting's doc tells the operator to set it if uploaded icons don't appear.
func (s *Server) iconServeURL(channelID string, at time.Time) string {
	base := ""
	if s.liveConfig != nil {
		base = strings.TrimRight(strings.TrimSpace(s.liveConfig("server.public_url")), "/")
	}
	return fmt.Sprintf("%s/v1/channels/%s/icon?v=%d", base, channelID, at.Unix())
}

// iconContentType returns the media type from a byte-signature sniff
// (http.DetectContentType), which recognizes the raster types the allowlist permits. The
// caller rejects anything not on the allowlist — a mismatched extension or a text/SVG upload
// simply sniffs to a non-image type and is refused. `filename` is unused (kept for a clear
// call site) — we trust the SIGNATURE, not the extension, so a ".png" of script bytes can't
// slip past as an image.
func iconContentType(_ string, data []byte) string {
	// DetectContentType returns e.g. "image/png; charset=..." for some types; take the media
	// type before the parameter.
	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}
