package testkit

import "context"

// This models Tunarr's media-source surface for the tunarr-connect flow (§6): the
// double implements setup.MediaSourceProgrammer so the integration test drives the
// REAL MediaSourceConnector against it. It models the two facts that matter: a
// source is created idempotently with the token Loomarr passes (the admin key, no
// user login), and enabling a library is what makes it "ready" for programming.

// EnsureEmbySource creates (or returns) the double's media source. On first call it
// enumerates the media server's libraries (a movie + show + music lib, disabled) the
// way Tunarr would. Idempotent. Records the access token for assertions.
func (m *Tunarr) EnsureEmbySource(_ context.Context, _, _, token, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MediaSourceToken = token
	if m.sourceID == "" {
		m.sourceID = "src-1"
		m.msLibs = []*msLibrary{{mediaType: "movies"}, {mediaType: "shows"}, {mediaType: "music"}}
	}
	return m.sourceID, nil
}

// ConnectLibraries enables the movie + show libraries and counts the scans it
// triggers (a second call re-enables nothing → no new scan; idempotent).
func (m *Tunarr) ConnectLibraries(_ context.Context, _ string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled := 0
	for _, l := range m.msLibs {
		if l.mediaType != "movies" && l.mediaType != "shows" {
			continue
		}
		enabled++
		if !l.enabled {
			l.enabled = true
			m.Scans++
		}
	}
	return enabled, nil
}

// MediaLibrariesReady reports whether a movie/show library is enabled — the
// tunarr_library setup check.
func (m *Tunarr) MediaLibrariesReady(_ context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.msLibs {
		if (l.mediaType == "movies" || l.mediaType == "shows") && l.enabled {
			return true, nil
		}
	}
	return false, nil
}
