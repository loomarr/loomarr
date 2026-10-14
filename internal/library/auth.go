package library

import (
	"fmt"
	"net/http"
)

// clientName is how Loomarr identifies itself to the media server (§11) — one
// device in the server dashboard, not hundreds.
const clientName = "Loomarr"

// applyTokenAuth sets the admin-token header for library calls (Lookup,
// ListUsers). Verified against real Emby 4.10 in Phase 0:
//   - Emby:     X-Emby-Token: <token>
//   - Jellyfin: Authorization: MediaBrowser Token="<token>", plus the client id
//
// §6: header auth only — never the api_key query param (leaks to logs).
func (f Flavor) applyTokenAuth(r *http.Request, token, deviceID string) {
	switch f {
	case Emby:
		r.Header.Set("X-Emby-Token", token)
	case Jellyfin:
		r.Header.Set("Authorization", mediaBrowserHeader(deviceID, token))
	}
}

// applyLoginAuth sets the client-identification header required on the
// AuthenticateByName request itself — before any token exists (§11 flavor
// quirk). Jellyfin *requires* it; Emby accepts the X-Emby-Authorization
// equivalent. deviceID is stable per install (§11).
func (f Flavor) applyLoginAuth(r *http.Request, deviceID string) {
	switch f {
	case Emby:
		r.Header.Set("X-Emby-Authorization", mediaBrowserHeader(deviceID, ""))
	case Jellyfin:
		r.Header.Set("Authorization", mediaBrowserHeader(deviceID, ""))
	}
}

// mediaBrowserParams builds the Client/Device/DeviceId/Version parameter list
// shared by both flavors' auth schemes (verified in Phase 0).
func mediaBrowserParams(deviceID string) string {
	return fmt.Sprintf(`Client="%s", Device="%s", DeviceId="%s", Version="%s"`,
		clientName, clientName, deviceID, version)
}

// mediaBrowserHeader is the Jellyfin/Emby "MediaBrowser" Authorization value,
// with an optional Token appended when one exists.
func mediaBrowserHeader(deviceID, token string) string {
	h := "MediaBrowser " + mediaBrowserParams(deviceID)
	if token != "" {
		h += fmt.Sprintf(`, Token="%s"`, token)
	}
	return h
}

// version is Loomarr's client version reported to the media server. Emby wants
// a value here (Phase 0 used 1.0.0); kept a constant until wired to build info.
const version = "0.1.0"
