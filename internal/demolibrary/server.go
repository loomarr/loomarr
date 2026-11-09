package demolibrary

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Server is an Emby-compatible stand-in media server for a demo Library.
//
// Why a server and not an adapter behind the library port: Loomarr talks to its media server
// through one concrete HTTP client (internal/library) whose whole surface is a handful of Emby
// endpoints, mostly GET /Items. Serving those endpoints means every consumer (search, scan,
// episode expansion, runtime lookups, direct-file input, inventory, filler) runs its production
// code against the demo library, configured only by library.url. An in-process adapter would need
// a second implementation of each of those consumers' ports and a build flag to swap it in.
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
	dateCreated       = "2026-01-01T00:00:00.0000000Z"
	ticksPerSecond    = 10_000_000
)

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	// Emby also serves /Users/{id}/Items; the client uses both forms.
	if i := strings.Index(path, "/Items"); strings.HasPrefix(path, "/Users/") && i > 0 {
		path = path[i:]
	}
	switch {
	case path == "/System/Info/Public":
		writeJSON(w, map[string]any{"ServerName": "Loomarr Demo Library", "Version": "4.10.0.0", "Id": "loomarr-demo"})
		return
	case path == "/Users/AuthenticateByName" && r.Method == http.MethodPost:
		s.authenticate(w, r)
		return
	case strings.HasPrefix(path, "/Items/") && strings.Contains(path, "/Images/"):
		s.image(w, r, path)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "Access token is invalid or expired.", http.StatusUnauthorized)
		return
	}
	switch {
	case path == "/System/Info":
		writeJSON(w, map[string]any{"ServerName": "Loomarr Demo Library", "Version": "4.10.0.0", "Id": "loomarr-demo"})
	case path == "/Users":
		writeJSON(w, []any{demoUserJSON()})
	case path == "/Library/VirtualFolders":
		// A bare array, as Emby returns it (internal/library/filler.go). Locations are the real
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
	case path == "/Items" && r.Method == http.MethodGet:
		items := s.items(r)
		total := len(items)
		q := r.URL.Query()
		if start, err := strconv.Atoi(q.Get("StartIndex")); err == nil && start > 0 {
			items = items[min(start, total):]
		}
		if n, err := strconv.Atoi(q.Get("Limit")); err == nil && n > 0 && len(items) > n {
			items = items[:n]
		}
		writeJSON(w, map[string]any{"Items": items, "TotalRecordCount": total})
	case path == "/LiveTv/TunerHosts" || path == "/LiveTv/ListingProviders" || path == "/ScheduledTasks":
		// Live TV registration never runs against the demo (lane backends set no public URL);
		// answer the reads with nothing registered so an accidental call fails soft.
		writeJSON(w, []any{})
	case path == "/System/Configuration/livetv":
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

// image serves /Items/{id}/Images/{Primary|Backdrop}[/{index}].
func (s Server) image(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.TrimPrefix(path, "/Items/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	var p string
	switch parts[2] {
	case "Primary":
		p = s.Layout.Poster(parts[0])
	case "Backdrop":
		p = s.Layout.Backdrop(parts[0])
	default:
		http.NotFound(w, r)
		return
	}
	if _, ok := s.Library.ByID(parts[0]); !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

// items answers GET /Items for the query shapes internal/library sends.
func (s Server) items(r *http.Request) []map[string]any {
	q := r.URL.Query()
	types := splitList(q.Get("IncludeItemTypes"))
	wantType := func(t string) bool { return len(types) == 0 || slices.Contains(types, t) }
	var out []map[string]any

	if slices.Contains(types, "BoxSet") {
		return []map[string]any{} // no collections in the demo library
	}
	// The cutover's id map lists every film and episode in one recursive, paged read.
	for _, t := range s.Library.Titles {
		switch q.Get("ParentId") {
		case moviesLibraryID:
			if t.Kind != Movie {
				continue
			}
		case showsLibraryID:
			if t.Kind != Series {
				continue
			}
		}
		if wantType(string(t.Kind)) {
			out = append(out, s.titleJSON(t))
		}
		if t.Kind == Series && wantType("Episode") {
			for _, e := range s.Library.EpisodesOf(t) {
				out = append(out, s.episodeJSON(t, e))
			}
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

func (s Server) titleJSON(t Title) map[string]any {
	// A series reports its first episode's runtime, as its typical episode length.
	shapeID := t.ID()
	if eps := s.Library.EpisodesOf(t); t.Kind == Series && len(eps) > 0 {
		shapeID = eps[0].ID
	}
	f, _ := s.Library.Shape(shapeID)
	item := map[string]any{
		"Id": t.ID(), "Name": t.Name, "SortName": t.Name, "Type": string(t.Kind), "ProductionYear": t.Year,
		"PremiereDate": strconv.Itoa(t.Year) + "-01-01T00:00:00.0000000Z", "Genres": t.Genres,
		"Overview": t.Overview, "OfficialRating": t.Rating, "RunTimeTicks": int64(f.Duration) * ticksPerSecond,
		"DateCreated": dateCreated, "DateLastSaved": dateCreated,
		"ImageTags": map[string]string{"Primary": "demo1"}, "BackdropImageTags": []string{"demo1"},
	}
	if t.Kind == Series {
		item["ProviderIds"] = map[string]string{"Tvdb": strconv.Itoa(t.ProviderID())}
		item["ParentId"] = showsLibraryID
		return item
	}
	item["ProviderIds"] = map[string]string{"Tmdb": strconv.Itoa(t.ProviderID())}
	item["ParentId"] = moviesLibraryID
	item["Path"] = s.abs(s.Layout.FilmFile(t))
	s.addMedia(item, t.ID(), f)
	return item
}

func (s Server) episodeJSON(t Title, e Episode) map[string]any {
	f, _ := s.Library.Shape(e.ID)
	year, overview := t.Year, t.Overview
	if e.Year != 0 {
		year = e.Year
	}
	if e.Overview != "" {
		overview = e.Overview
	}
	item := map[string]any{
		"Id": e.ID, "Name": e.Name, "Type": "Episode", "SeriesId": t.ID(), "SeriesName": t.Name, "ParentId": t.ID(),
		"ParentIndexNumber": e.Season, "IndexNumber": e.Number, "RunTimeTicks": int64(f.Duration) * ticksPerSecond,
		"OfficialRating": t.Rating, "ProductionYear": year, "Overview": overview, "Genres": t.Genres,
		"DateCreated": dateCreated, "DateLastSaved": dateCreated, "Path": s.abs(s.Layout.EpisodeFile(t, e)),
	}
	s.addMedia(item, e.ID, f)
	return item
}

// addMedia describes the file the way Emby's probe would, so format-aware surfaces (4K/HDR
// premium detection, the media inventory) see the real shape of each generated file.
func (s Server) addMedia(item map[string]any, id string, f Format) {
	pix, transfer, primaries, rng, profile := "yuv420p", "bt709", "bt709", "SDR", "High"
	if f.Codec == "hevc" {
		profile = "Main"
	}
	if f.TenBit {
		pix, profile = "yuv420p10le", "Main 10"
	}
	if f.HDR10 {
		transfer, primaries, rng = "smpte2084", "bt2020", "HDR10"
	}
	audio := f.Audio
	if audio == "" {
		audio = "aac"
	}
	streams := []map[string]any{
		{"Index": 0, "Type": "Video", "Codec": f.Codec, "Profile": profile, "Width": f.Width, "Height": f.Height,
			"RealFrameRate": 24, "PixelFormat": pix, "ColorTransfer": transfer, "ColorPrimaries": primaries,
			"VideoRangeType": rng, "IsDefault": true},
		{"Index": 1, "Type": "Audio", "Codec": audio, "Channels": 2, "ChannelLayout": "stereo", "SampleRate": 48000,
			"Language": "eng", "IsDefault": true},
	}
	item["MediaStreams"] = streams
	source := map[string]any{
		"Id": id, "Protocol": "File", "Container": "mp4", "Path": item["Path"],
		"RunTimeTicks": int64(f.Duration) * ticksPerSecond, "DateLastSaved": dateCreated, "MediaStreams": streams,
	}
	// Emby states the file's byte count on the source and copies it to the item's top-level Size,
	// which is what an owned title's approval size reads (#1885). The generated file's real size,
	// and only once it exists: Emby has no size for a file it has not probed, and neither does this.
	if path, ok := item["Path"].(string); ok {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			source["Size"] = info.Size()
			item["Size"] = info.Size()
		}
	}
	item["MediaSources"] = []map[string]any{source}
}

func (s Server) abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
