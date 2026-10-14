package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/viewing"
)

// The viewer tag: who a signed play URL was minted for, so household viewing (#1662) can say who
// is watching from the players' own playlist polls.
//
// It rides the signed URL as its own `viewer` parameter, and like every parameter but `mode` the
// master playlist copies it onto the media-playlist URLs, so each poll carries it with no client
// change. It is NOT a credential and grants nothing: `sig` alone authorizes the stream, and a bad
// or missing tag only means the poll isn't attributed. It is signed anyway (same key and HMAC as
// `sig`, bound to the channel) so nobody can make the household believe someone else is watching.
//
// Payload: base64url("user\ndevice-key\ndevice-label"). No secrets: the user id is opaque, the
// device key is a paired device's non-secret revocation id or the browser label, and the label is
// the device's name as the household sees it.

const viewerQueryParam = "viewer"

// viewerLabelMax bounds the label a URL carries.
const viewerLabelMax = 64

func signViewerTag(key, channelID string, v viewing.Viewer) string {
	if key == "" || v.UserID == "" {
		return ""
	}
	label := v.DeviceLabel
	if len(label) > viewerLabelMax {
		label = label[:viewerLabelMax]
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(v.UserID + "\n" + v.DeviceKey + "\n" + label))
	return payload + "." + viewerTagMAC(key, channelID, payload)
}

func verifyViewerTag(key, channelID, raw string) (viewing.Viewer, bool) {
	payload, mac, ok := strings.Cut(raw, ".")
	if key == "" || !ok {
		return viewing.Viewer{}, false
	}
	if subtle.ConstantTimeCompare([]byte(mac), []byte(viewerTagMAC(key, channelID, payload))) != 1 {
		return viewing.Viewer{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return viewing.Viewer{}, false
	}
	parts := strings.SplitN(string(decoded), "\n", 3)
	if len(parts) != 3 || parts[0] == "" {
		return viewing.Viewer{}, false
	}
	return viewing.Viewer{UserID: parts[0], DeviceKey: parts[1], DeviceLabel: parts[2]}, true
}

// viewerTagMAC is domain-separated from the `sig` MAC ("viewer\n" prefix), so a viewer tag can
// never be replayed as a stream signature or the other way round.
func viewerTagMAC(key, channelID, payload string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = fmt.Fprintf(mac, "viewer\n%s\n%s", channelID, payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// withViewerTag appends the tag to a minted play URL (which always has a query: its `sig`).
func withViewerTag(u, tag string) string {
	if u == "" || tag == "" {
		return u
	}
	return u + "&" + viewerQueryParam + "=" + url.QueryEscape(tag)
}

// viewerOf is the person and device minting a play URL. A paired device is keyed by its revocation
// id and named as it was paired; a browser is keyed and named by its browser family and system.
// A break-glass API_TOKEN caller has no person, so it is never attributed.
func (s *Server) viewerOf(ctx context.Context) (viewing.Viewer, bool) {
	user, ok := userFrom(ctx)
	if !ok {
		return viewing.Viewer{}, false
	}
	if deviceID, ok := deviceIDFrom(ctx); ok {
		label := "Paired device"
		if s.store != nil {
			if dt, err := s.store.GetDeviceToken(ctx, deviceID); err == nil && strings.TrimSpace(dt.DeviceName) != "" {
				label = strings.TrimSpace(dt.DeviceName)
			}
		}
		key := deviceID
		if len(key) > 16 {
			key = key[:16]
		}
		return viewing.Viewer{UserID: user.ID, DeviceKey: "device:" + key, DeviceLabel: label}, true
	}
	label := "Browser"
	if r := requestFrom(ctx); r != nil {
		label = clientLabel(r.UserAgent())
	}
	return viewing.Viewer{UserID: user.ID, DeviceKey: "browser:" + label, DeviceLabel: label}, true
}

// requestViewer is the viewer a playout request's URL was minted for, from its signed tag.
// Best-effort by design: no key or a bad tag names nobody, and the request is served all the same.
func (s *Server) requestViewer(ctx context.Context, channelID string, query url.Values) (viewing.Viewer, bool) {
	raw := query.Get(viewerQueryParam)
	if raw == "" {
		return viewing.Viewer{}, false
	}
	key, err := s.currentPlayoutToken(ctx)
	if err != nil {
		return viewing.Viewer{}, false
	}
	return verifyViewerTag(key, channelID, raw)
}

// withPlayoutViewer names the request's viewer to playout (playout.WithViewer), which learns from it
// whether that viewer's client takes the premium (#1037). The key is the person and the device,
// never the label, which the household may rename.
func withPlayoutViewer(ctx context.Context, v viewing.Viewer, ok bool) context.Context {
	if !ok {
		return ctx
	}
	return playout.WithViewer(ctx, v.UserID+"\n"+v.DeviceKey)
}
