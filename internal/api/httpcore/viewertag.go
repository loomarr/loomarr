package httpcore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/playout"
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

// ViewerQueryParam is the query parameter a play URL carries its viewer tag in.
const ViewerQueryParam = "viewer"

// viewerLabelMax bounds the label a URL carries.
const viewerLabelMax = 64

// SignViewerTag signs v for channelID under key; it is "" when there is no key or no person.
func SignViewerTag(key, channelID string, v viewing.Viewer) string {
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

// VerifyViewerTag reads the viewer a tag names, if its signature holds for channelID under key.
func VerifyViewerTag(key, channelID, raw string) (viewing.Viewer, bool) {
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

// WithViewerTag appends the tag to a minted play URL (which always has a query: its `sig`).
func WithViewerTag(u, tag string) string {
	if u == "" || tag == "" {
		return u
	}
	return u + "&" + ViewerQueryParam + "=" + url.QueryEscape(tag)
}

// WithPlayoutViewer names the request's viewer to playout (playout.WithViewer), which learns from it
// whether that viewer's client takes the premium (#1037). The key is the person and the device,
// never the label, which the household may rename.
func WithPlayoutViewer(ctx context.Context, v viewing.Viewer, ok bool) context.Context {
	if !ok {
		return ctx
	}
	return playout.WithViewer(ctx, v.UserID+"\n"+v.DeviceKey)
}
