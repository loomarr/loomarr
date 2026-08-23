package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// Signed playout URLs — how a PERSON's browser plays a channel without holding the device
// token (§9.1 Watch, §11).
//
// The playout routes authenticate a DEVICE by `playout_token` (playout.go). That token is the
// media server's long-lived secret and must never reach a browser. But the Web UI's Watch
// surface is used by a logged-in person, and their `<video>`/hls.js is handed a URL and
// nothing else — the same shape the media server needs. So a session-authenticated endpoint
// mints a SHORT-LIVED SIGNED URL: a capability scoped to one channel that expires on its own.
//
// The signature is an HMAC keyed by `playout_token` itself. Two properties fall out of that
// choice, both wanted:
//
//   - NO NEW SECRET. There is exactly one playout secret to manage, redact, and rotate. A
//     second key would be a second thing to leak and a second lifecycle to get wrong.
//   - ROTATION CUTS BROWSER SESSIONS TOO. Regenerating `playout_token` changes the HMAC key,
//     so every outstanding signed URL stops verifying — which is correct: the same rotation
//     that re-wires the media server should also end in-app viewing, not leave signed URLs
//     valid against a token nobody uses anymore.
//
// The signed material is `channel + expiry`, so a URL for one channel cannot be replayed
// against another, and a captured URL stops working at its deadline rather than forever.

// signQueryParam carries the "channel:exp:sig" capability in the URL, distinct from the raw
// device `token` param so a handler can tell which credential it was handed and never confuse
// a scoped, expiring capability for the break-glass device secret.
const signQueryParam = "sig"

// playURLTTL is how long a minted play URL stays valid. Long enough to watch a programme or
// three without re-minting mid-view; short enough that a URL leaked into a log or a shared
// screen is a bounded exposure, not a standing one. The mock's copy ("good for 8 hours")
// is this value.
const playURLTTL = 8 * time.Hour

// signPlayout builds the "channel:exp:sig" capability string for a channel, valid until `exp`.
//
// Returns "" when there is no token to key the HMAC with — the same "not configured" signal
// the device-token path uses, so a caller renders "playout isn't set up" rather than minting a
// capability that can never verify.
func (s *Server) signPlayout(channelID string, exp time.Time) string {
	return signPlayoutWithKey(s.playoutToken(), channelID, exp)
}

func signPlayoutWithKey(key, channelID string, exp time.Time) string {
	if key == "" {
		return ""
	}
	unix := exp.Unix()
	mac := playoutSignature(key, channelID, unix)
	return fmt.Sprintf("%s:%d:%s", channelID, unix, mac)
}

// verifyPlayoutSignature checks a "channel:exp:sig" capability against `channelID` and the
// current time. Constant-time on the MAC, like the device-token check, and for the same reason:
// this is a credential comparison on a LAN-reachable route.
//
// Every failure returns false with no distinction between "malformed", "wrong channel",
// "expired", and "bad signature" — the caller renders one 404, because telling an attacker
// WHICH check failed (does this channel exist? is this the right shape?) is exactly the
// enumeration the device path's 404 already refuses to give.
func (s *Server) verifyPlayoutSignature(raw, channelID string, now time.Time) bool {
	return verifyPlayoutSignatureWithKey(s.playoutToken(), raw, channelID, now)
}

func verifyPlayoutSignatureWithKey(key, raw, channelID string, now time.Time) bool {
	if key == "" {
		return false
	}
	// channel:exp:sig — split from the RIGHT twice, because a channel id could in principle
	// contain a colon and the two trailing fields never can (a base-10 int and a base64 MAC).
	parts := strings.Split(raw, ":")
	if len(parts) < 3 {
		return false
	}
	gotSig := parts[len(parts)-1]
	expStr := parts[len(parts)-2]
	gotChannel := strings.Join(parts[:len(parts)-2], ":")

	// The signed channel must be the one being requested — a URL minted for ch1 cannot pull ch2.
	if subtle.ConstantTimeCompare([]byte(gotChannel), []byte(channelID)) != 1 {
		return false
	}
	unix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	if now.After(time.Unix(unix, 0)) {
		return false // expired
	}
	want := playoutSignature(key, gotChannel, unix)
	return subtle.ConstantTimeCompare([]byte(gotSig), []byte(want)) == 1
}

// playoutSignature is the raw HMAC-SHA256 over "channel:exp", base64url without padding.
//
// The delimiter is inside the signed bytes, not just the wire format: signing "ch1:2" and
// "ch12:" as the same string would let a channel-id/expiry boundary shift without changing the
// MAC. A fixed separator the id is known not to end on keeps the two fields unambiguous under
// the hash.
func playoutSignature(key, channelID string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(key))
	// hash.Hash.Write never returns an error (documented), but errcheck cannot know that.
	_, _ = fmt.Fprintf(mac, "%s:%d", channelID, exp)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// playoutHLSURL builds the absolute, signed HLS playlist URL a browser/native player loads.
//
// Built on the same base + escaping as playoutURL (the device-token URLs), but carries `sig`
// instead of `token`: the segment URLs the playlist references inherit the same signature, so
// one mint covers the playlist and all its segments for the TTL.
//
// ABSOLUTE, from `server.public_url` — this is the form a NATIVE app or a media server needs: a
// client with no "current origin" that fetches the URL cold. The web browser uses the RELATIVE
// form (playoutHLSPathURL) instead, because it is already on Loomarr's origin. Returns "" when the
// base or the signature is unavailable.
func (s *Server) playoutHLSURL(channelID string, quality string, plan playout.EncodePlan, exp time.Time) string {
	return s.playoutHLSURLWithKey(s.playoutToken(), channelID, quality, plan, exp)
}

func (s *Server) playoutHLSURLWithKey(key, channelID string, quality string, plan playout.EncodePlan, exp time.Time) string {
	base := s.playoutBaseURL()
	path := s.playoutHLSPathURLWithKey(key, channelID, quality, plan, exp)
	if base == "" || path == "" {
		return ""
	}
	return base + path
}

// playoutHLSPathURL is the RELATIVE (same-origin) signed master-playlist URL, for the in-app WEB
// player (§9.1 Watch, V46).
//
// Same-origin is strictly better for the browser: the page already runs on Loomarr's origin, so a
// relative `/playout/hls/...` needs no CORS (hls.js fetches by XHR, which a cross-origin absolute
// URL would trip) AND does not depend on `server.public_url` being set or reachable. Native apps
// cannot use this — they have no origin to be relative to — which is exactly why the absolute form
// above still exists. Returns "" only when the signature can't be minted (no token).
func (s *Server) playoutHLSPathURL(channelID string, quality string, plan playout.EncodePlan, exp time.Time) string {
	return s.playoutHLSPathURLWithKey(s.playoutToken(), channelID, quality, plan, exp)
}

func (s *Server) playoutHLSPathURLWithKey(key, channelID string, quality string, plan playout.EncodePlan, exp time.Time) string {
	sig := signPlayoutWithKey(key, channelID, exp)
	if sig == "" {
		return ""
	}
	q := url.Values{}
	q.Set(signQueryParam, sig)
	if quality != "" {
		q.Set("quality", quality)
	}
	// `?plan=` is UNSIGNED, deliberately — like `quality`. The signature authorizes the CHANNEL
	// (channel:exp); the plan only picks a copy bucket for a channel the sig already grants, so it
	// needs no signing, and a client can re-probe its capabilities and switch plans without re-minting.
	// Omitted for PlanBaseline (the default clientPlan reads on absence) so the common URL stays clean;
	// a richer plan is written explicitly. clientPlan on the read side treats only a recognized token
	// as capable — anything else is the safe baseline.
	if plan != playout.PlanBaseline {
		q.Set(playoutPlanParam, plan.String())
	}
	return fmt.Sprintf("/v1/playout/hls/%s/master.m3u8?%s", url.PathEscape(channelID), q.Encode())
}
