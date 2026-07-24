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

// allowedIconTypes is the MIME allowlist for an uploaded icon. Kept tight (raster + svg) so
// the serve endpoint only ever emits image content Tunarr/Emby render.
var allowedIconTypes = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/webp":    true,
	"image/gif":     true,
	"image/svg+xml": true,
}

// uploadChannelIcon handles POST /v1/channels/{id}/icon (multipart, field "file"). Admin-only
// (an authoring action; the server enforces regardless of the FE). Stores the bytes, points
// the channel's logo at the serve URL, and persists — the auto-reconcile on the next sweep (or
// an explicit reconcile) pushes it to Tunarr. Best-effort content-type sniff falls back to the
// declared form type; both are checked against the allowlist.
func (s *Server) uploadChannelIcon(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || s.auth.Authorize(r) != RoleAdmin {
		s.writeProblem(w, r, http.StatusForbidden, "Not allowed", "This action needs an admin account.")
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
	ch.Logo = iconServeURL(r, id, now)
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
	// A stored icon is immutable for its updated_at; the ?v= cache-bust changes on re-upload,
	// so a long cache is safe and keeps Tunarr/Emby from refetching every guide build.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Last-Modified", updatedAt.UTC().Format(http.TimeFormat))
	_, _ = w.Write(data)
}

// iconServeURL builds the absolute icon URL from the upload request's scheme + Host (the
// reachable address the admin's browser used — the same host family Tunarr resolves on a
// homelab). The ?v= is the upload epoch, so a replaced icon gets a fresh URL that busts any
// downstream cache.
func iconServeURL(r *http.Request, channelID string, at time.Time) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return fmt.Sprintf("%s://%s/v1/channels/%s/icon?v=%d", scheme, host, channelID, at.Unix())
}

// iconContentType prefers a byte-signature sniff (http.DetectContentType) but keeps SVG,
// which sniffs as text/xml or text/plain: an ".svg" name with an XML/SVG signature maps to
// image/svg+xml. Everything else uses the sniffed type, checked against the allowlist by the
// caller.
func iconContentType(filename string, data []byte) string {
	if strings.HasSuffix(strings.ToLower(filename), ".svg") {
		head := strings.ToLower(string(data[:min(len(data), 256)]))
		if strings.Contains(head, "<svg") || strings.Contains(head, "<?xml") {
			return "image/svg+xml"
		}
	}
	// DetectContentType returns e.g. "image/png; charset=..." for some types; take the media
	// type before the parameter.
	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}
