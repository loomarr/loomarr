package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Channel icon upload + serve (the upload icon source). These are plain mux handlers, NOT
// Huma ops, because they move raw image bytes: the serve returns an image with its own
// Content-Type, and the upload takes multipart/form-data — neither fits Huma's typed-JSON
// model (same reason /v1/backup and /v1/events are raw handlers). The bytes live in the DB
// (channel_icons), so there's no writable-asset-dir config and the icon rides the §16 backup.
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

// uploadChannelIcon handles POST /v1/channels/{id}/icon (multipart, field "file"). Admin-only
// (an authoring action; the server enforces regardless of the FE). Stores the bytes, points
// the channel's logo at the serve URL, and persists — the auto-reconcile on the next sweep (or
// an explicit reconcile) pushes it to Tunarr. Best-effort content-type sniff falls back to the
// declared form type; both are checked against the allowlist.
func (s *Server) uploadChannelIcon(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, RoleAdmin) {
		return
	}
	// CSRF (§11): this is a raw mux handler, so the Huma CSRF middleware doesn't cover it —
	// and a multipart POST is the classic CSRF vector (a cross-origin HTML form can send
	// it). Require the header the FE always sends; a cross-origin form cannot set it. Closes
	// form-based CSRF alongside SameSite=Strict, matching the middleware's contract.
	if r.Header.Get("X-Loomarr-Csrf") != "1" {
		s.writeProblem(w, r, http.StatusForbidden, "Missing CSRF header", "This request is missing the X-Loomarr-Csrf header.")
		return
	}
	id := r.PathValue("id")
	ch, err := s.store.GetChannel(r.Context(), id)
	if err != nil {
		s.writeProblem(w, r, http.StatusNotFound, "Channel not found", "That channel doesn't exist — it may have been removed.")
		return
	}

	// Bound the whole body before parsing so a huge upload can't buffer into memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxIconBytes+1024)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "No image", "Attach an image file in the `file` field (PNG, JPEG, WebP, GIF, or SVG).")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes+1))
	if err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "Couldn't read the image", "The upload was incomplete. Try again.")
		return
	}
	if len(data) > maxIconBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "Image too large", "Channel icons must be under 2 MB.")
		return
	}

	ct := iconContentType(hdr.Filename, data)
	if !allowedIconTypes[ct] {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "Unsupported image type",
			"Use a PNG, JPEG, WebP, GIF, or SVG image.")
		return
	}

	now := time.Now()
	if err := s.store.PutChannelIcon(r.Context(), id, ct, data, now); err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't save the icon", "Something went wrong storing the image. Try again.")
		return
	}

	// Point the channel at the serve URL, cache-busted by the upload time so Tunarr/Emby
	// refetch a replaced icon instead of serving a stale cached image.
	ch.Logo = s.iconServeURL(id, now)
	if err := s.store.UpsertChannel(r.Context(), ch); err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't update the channel", "The icon was saved but linking it to the channel failed. Try again.")
		return
	}

	// Auto-reconcile so the new icon reaches Tunarr without a manual step (the seamless
	// edit model, §9). Best-effort — a reconcile failure doesn't fail the upload; the next
	// sweep pushes it. Only when a channel service is wired (nil in some unit setups).
	if s.channels != nil {
		if err := s.channels.Reconcile(r.Context(), id); err != nil && s.log != nil {
			s.log.Warn("icon upload reconcile (icon saved; next sweep pushes it)", "channel", id, "err", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"logo": ch.Logo})
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
