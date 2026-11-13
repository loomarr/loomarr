package demolibrary

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
)

// Server is an Emby-compatible stand-in media server for a demo Library.
//
// Loomarr scans the demo's files itself (#2151); what it still asks a media server is its library
// folders (scan-root discovery and the filler library), its users (sign-in and user import) and
// its Live TV registration. Serving those endpoints lets that production code run against the demo,
// configured only by library.url.
//
// It answers exactly the queries internal/library sends; anything else is 404.
type Server struct {
	Layout Layout
	// Library is what it serves: Invented() or the open pack (Load).
	Library *Library
	// Token is the API key clients must present (library.token). Empty accepts any request.
	Token string
}

// Demo account on the stand-in server, for media-server sign-in and user import.
const (
	DemoUser     = "demo"
	DemoPassword = "demo"
	demoUserID   = "demo-user-1"
)

const (
	moviesLibraryID = "demo-lib-movies"
	showsLibraryID  = "demo-lib-shows"
	// FillerLibraryName is the stand-in's interstitials library; its folder is the demo's filler
	// library source.
	FillerLibraryName = "Demo Filler"
	fillerLibraryID   = "demo-lib-filler"
)

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case path == "/System/Info/Public":
		writeJSON(w, map[string]any{"ServerName": "Loomarr Demo Library", "Version": "4.10.0.0", "Id": "loomarr-demo"})
		return
	case path == "/Users/AuthenticateByName" && r.Method == http.MethodPost:
		s.authenticate(w, r)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "Access token is invalid or expired.", http.StatusUnauthorized)
		return
	}
	switch path {
	case "/System/Info":
		writeJSON(w, map[string]any{"ServerName": "Loomarr Demo Library", "Version": "4.10.0.0", "Id": "loomarr-demo"})
	case "/Users":
		writeJSON(w, []any{demoUserJSON()})
	case "/Library/VirtualFolders":
		// A bare array, as Emby returns it (internal/library/mediaserver/filler.go). Locations are the real
		// folders the demo's files sit in, so scan-root discovery (#2151) finds them.
		// Absolute, as a media server reports them; the layout may be relative to the working dir.
		dir, err := filepath.Abs(s.Layout.Dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		abs := Layout{Dir: dir}
		writeJSON(w, []map[string]any{
			{"Name": "Movies", "ItemId": moviesLibraryID, "CollectionType": "movies", "Locations": []string{abs.Movies()}},
			{"Name": "Shows", "ItemId": showsLibraryID, "CollectionType": "tvshows", "Locations": []string{abs.Shows()}},
			{"Name": FillerLibraryName, "ItemId": fillerLibraryID, "Locations": []string{s.Library.FillerDir(abs)}},
		})
	case "/LiveTv/TunerHosts", "/LiveTv/ListingProviders", "/ScheduledTasks":
		// Live TV registration never runs against the demo (lane backends set no public URL);
		// answer the reads with nothing registered so an accidental call fails soft.
		writeJSON(w, []any{})
	case "/System/Configuration/livetv":
		writeJSON(w, map[string]any{"TunerHosts": []any{}, "ListingProviders": []any{}})
	default:
		http.NotFound(w, r)
	}
}

func (s Server) authorized(r *http.Request) bool {
	if s.Token == "" {
		return true
	}
	for _, v := range []string{r.Header.Get("X-Emby-Token"), r.Header.Get("X-MediaBrowser-Token"), r.URL.Query().Get("api_key")} {
		if v == s.Token {
			return true
		}
	}
	for _, h := range []string{r.Header.Get("Authorization"), r.Header.Get("X-Emby-Authorization")} {
		if strings.Contains(h, `Token="`+s.Token+`"`) {
			return true
		}
	}
	return false
}

func (s Server) authenticate(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Pw string }
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Username != DemoUser || body.Pw != DemoPassword {
		http.Error(w, "Invalid username or password entered.", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"AccessToken": "demo-session", "User": demoUserJSON()})
}

func demoUserJSON() map[string]any {
	return map[string]any{"Id": demoUserID, "Name": DemoUser, "Policy": map[string]any{"IsAdministrator": true, "IsDisabled": false}}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
