package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Library → Items lists present titles with their readiness, finds them by name, opens a series
// with its episodes, and is for admins only.
func TestLibraryItemsListAndPage(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie", Year: 2001},
		{ID: "90000002", Kind: "series", Name: "Bravo Show", Year: 2010},
		{ID: "90000011", Kind: "episode", Name: "Bravo E1", SeriesID: "90000002"},
		{ID: "90000012", Kind: "episode", Name: "Bravo E2", SeriesID: "90000002"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 7, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	if err := h.Store.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/alpha.mkv", "90000001", "", librarystore.LibraryHeaderRead),
		file("/media/bravo/e1.mkv", "90000011", "90000002", librarystore.LibraryHeaderRead),
		file("/media/bravo/e2.mkv", "90000012", "90000002", librarystore.LibraryHeaderPending),
	}); err != nil {
		t.Fatal(err)
	}

	var list struct {
		Items []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Name  string `json:"name"`
			Year  int    `json:"year"`
			Files int    `json:"files"`
			Ready bool   `json:"ready"`
		} `json:"items"`
		Total int `json:"total"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items", adminToken, ""), &list)
	if list.Total != 2 || len(list.Items) != 2 || !list.Items[0].Ready || list.Items[1].Ready || list.Items[1].Files != 2 {
		t.Fatalf("list = %+v", list)
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items?search=brav", adminToken, ""), &list)
	if list.Total != 1 || list.Items[0].Name != "Bravo Show" {
		t.Fatalf("search = %+v", list)
	}

	var page struct {
		Item struct {
			Kind string `json:"kind"`
		} `json:"item"`
		Files   []any `json:"files"`
		Genres  []any `json:"genres"`
		Seasons []struct {
			Episodes []struct {
				ID    string `json:"id"`
				Ready bool   `json:"ready"`
			} `json:"episodes"`
		} `json:"seasons"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items/90000002", adminToken, ""), &page)
	if page.Item.Kind != "series" || page.Files == nil || page.Genres == nil || len(page.Seasons) != 1 ||
		len(page.Seasons[0].Episodes) != 2 {
		t.Fatalf("series page = %+v", page)
	}

	// A file whose header nothing has measured still lists its subtitles, as none.
	var movie struct {
		Files []struct {
			Subtitles []any `json:"subtitles"`
		} `json:"files"`
		DetailsFrom *struct{} `json:"detailsFrom"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items/90000001", adminToken, ""), &movie)
	if len(movie.Files) != 1 || movie.Files[0].Subtitles == nil || movie.DetailsFrom == nil {
		t.Fatalf("movie page = %+v", movie)
	}

	if resp := h.Do(http.MethodGet, "/v1/library/items/90000099", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown title = %d, want 404", resp.StatusCode)
	}
	if resp := h.Do(http.MethodGet, "/v1/library/items", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member = %d, want 403", resp.StatusCode)
	}
}

// A title's page carries the facts the Library stores (#4476): a movie's score, tagline, studios,
// countries, original title, premiere date and credits, where its year, score and runtime came
// from, and each file's edition, container, bit rate, every track and its chapters; a series'
// episodes with their stories and runtimes; an episode's own page with its file's tracks. A title
// with none of them reads empty lists, never null, and no value pretending to be known.
func TestLibraryItemPageCarriesStoredFacts(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	streams := []librarystore.LibraryStream{
		{Index: 0, Kind: "video", Codec: "hevc", Profile: "Main 10", Width: 3840, Height: 2160, FrameRate: "24000/1001",
			HDR: "Dolby Vision", BitDepth: 10, BitRate: 20_000_000},
		{Index: 1, Kind: "audio", Codec: "truehd", Channels: 6, ChannelLayout: "5.1(side)", Language: "en",
			LanguageName: "English", Default: true, BitDepth: 24, Title: "Main", Commentary: true, Descriptive: true},
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "es", LanguageName: "Spanish", Forced: true,
			HearingImpaired: true, ExternalRef: "/media/alpha.es.srt"},
	}
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie", Year: 2001, OriginalTitle: "Alpha Original",
			Tagline: "One line.", PremiereDate: "2001-02-03", Score: 7.5, RuntimeMs: 5_400_000,
			Studios: []string{"North Studio"}, Countries: []string{"France", "Spain"},
			Credits: []librarystore.LibraryCredit{
				{Name: "Ana Actor", Role: "actor", Character: "Lead", Photo: "https://images.example/ana.jpg"},
				{Name: "Di Director", Role: "director"},
			},
			From: librarystore.LibraryFrom{Year: "nfo", Score: "tmdb", Runtime: "radarr"},
			Sources: []librarystore.LibrarySource{{Path: "/media/alpha.mkv", Container: "matroska,webm",
				Bitrate: 24_000_000, DurationMs: 5_432_100, Edition: "Extended", Plays: true, Streams: streams,
				Chapters: []librarystore.LibraryChapter{{StartMs: 0, EndMs: 60_000, Title: "Opening"}, {StartMs: 60_000, EndMs: -1}}}}},
		{ID: "90000003", Kind: "movie", Name: "Charlie Movie",
			Sources: []librarystore.LibrarySource{{Path: "/media/charlie.mkv", Plays: true}}},
		{ID: "90000002", Kind: "series", Name: "Bravo Show"},
		{ID: "90000011", Kind: "episode", Name: "Bravo E1", SeriesID: "90000002", Season: 1, Episode: 1,
			Overview: "It begins.", RuntimeMs: 1_380_000,
			Sources: []librarystore.LibrarySource{{Path: "/media/bravo/e1.mkv", Container: "mov,mp4", Plays: true,
				Streams: streams[:2]}}},
		{ID: "90000012", Kind: "episode", Name: "Bravo E2", SeriesID: "90000002", Season: 1, Episode: 2},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 7, ModifiedNs: 1, Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	if err := h.Store.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/alpha.mkv", "90000001", ""), file("/media/charlie.mkv", "90000003", ""),
		file("/media/bravo/e1.mkv", "90000011", "90000002"), file("/media/bravo/e2.mkv", "90000012", "90000002"),
	}); err != nil {
		t.Fatal(err)
	}
	page := func(id string) map[string]any {
		t.Helper()
		var out map[string]any
		decode(t, h.Do(http.MethodGet, "/v1/library/items/"+id, adminToken, ""), &out)
		return out
	}
	asJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	movie := page("90000001")
	for field, want := range map[string]string{
		"originalTitle": `"Alpha Original"`, "tagline": `"One line."`, "premiereDate": `"2001-02-03"`, "score": `7.5`,
		"studios": `["North Studio"]`, "countries": `["France","Spain"]`,
		"credits":     `[{"character":"Lead","name":"Ana Actor","role":"actor"},{"name":"Di Director","role":"director"}]`,
		"detailsFrom": `{"runtime":"radarr","score":"tmdb","year":"nfo"}`,
	} {
		if got := asJSON(movie[field]); got != want {
			t.Errorf("movie %s = %s, want %s", field, got, want)
		}
	}
	files, _ := movie["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("movie files = %s", asJSON(movie["files"]))
	}
	alpha, _ := files[0].(map[string]any)
	for field, want := range map[string]string{
		"edition": `"Extended"`, "container": `"matroska,webm"`, "bitrate": `24000000`,
		"chapters": `[{"name":"Opening","startMillis":0},{"startMillis":60000}]`,
		"streams": `[{"bitDepth":10,"bitrate":20000000,"codec":"hevc","commentary":false,"default":false,` +
			`"descriptive":false,"external":false,"forced":false,"frameRate":"24000/1001","hdr":"Dolby Vision",` +
			`"hearingImpaired":false,"kind":"video","profile":"Main 10"},` +
			`{"bitDepth":24,"channelLayout":"5.1(side)","codec":"truehd","commentary":true,"default":true,` +
			`"descriptive":true,"external":false,"forced":false,"hearingImpaired":false,"kind":"audio",` +
			`"language":"en","languageName":"English","title":"Main"},` +
			`{"codec":"subrip","commentary":false,"default":false,"descriptive":false,"external":true,"forced":true,` +
			`"hearingImpaired":true,"kind":"subtitle","language":"es","languageName":"Spanish"}]`,
	} {
		if got := asJSON(alpha[field]); got != want {
			t.Errorf("movie file %s = %s, want %s", field, got, want)
		}
	}
	if body := asJSON(movie); strings.Contains(body, "images.example") || strings.Contains(body, "/media/alpha.es.srt") {
		t.Errorf("the page carries a photo URL or a sidecar's path: %s", body)
	}

	// A title with none of the optional facts: empty lists, and nothing unknown written as a value.
	bare := page("90000003")
	for _, field := range []string{"studios", "countries", "credits"} {
		if got := asJSON(bare[field]); got != `[]` {
			t.Errorf("bare %s = %s, want []", field, got)
		}
	}
	for _, field := range []string{"originalTitle", "tagline", "premiereDate", "score"} {
		if v, ok := bare[field]; ok {
			t.Errorf("bare %s = %v, want absent", field, v)
		}
	}
	if got := asJSON(bare["detailsFrom"]); got != `{}` {
		t.Errorf("bare detailsFrom = %s, want {}", got)
	}
	bareFiles, _ := bare["files"].([]any)
	if len(bareFiles) != 1 {
		t.Fatalf("bare files = %s", asJSON(bare["files"]))
	}
	charlie, _ := bareFiles[0].(map[string]any)
	if asJSON(charlie["streams"]) != `[]` || asJSON(charlie["chapters"]) != `[]` {
		t.Errorf("bare file = %s, want streams and chapters []", asJSON(charlie))
	}
	for _, field := range []string{"edition", "container", "bitrate"} {
		if v, ok := charlie[field]; ok {
			t.Errorf("bare file %s = %v, want absent", field, v)
		}
	}

	// A series' episodes carry their stories and runtimes; one with none has neither.
	show := page("90000002")
	if got, want := asJSON(show["seasons"]), `[{"episodes":[`+
		`{"fromFile":false,"id":"90000011","name":"Bravo E1","number":1,"overview":"It begins.","ready":true,"runtimeMillis":1380000},`+
		`{"fromFile":false,"id":"90000012","name":"Bravo E2","number":2,"ready":true}],"number":1}]`; got != want {
		t.Errorf("series seasons = %s, want %s", got, want)
	}

	// An episode's own page reads its file's tracks.
	ep := page("90000011")
	epFiles, _ := ep["files"].([]any)
	if len(epFiles) != 1 {
		t.Fatalf("episode files = %s", asJSON(ep["files"]))
	}
	e1, _ := epFiles[0].(map[string]any)
	if streams, _ := e1["streams"].([]any); len(streams) != 2 || asJSON(e1["container"]) != `"mov,mp4"` {
		t.Errorf("episode file = %s, want its two tracks in mov,mp4", asJSON(e1))
	}
	if asJSON(ep["overview"]) != `"It begins."` || asJSON(ep["runtimeMillis"]) != `1380000` {
		t.Errorf("episode page overview %s, runtime %s", asJSON(ep["overview"]), asJSON(ep["runtimeMillis"]))
	}
}

// A title's poster rides on its row once its bytes are here; an adopted poster still being
// fetched is left out, so the grid shows the name instead of a broken image.
func TestLibraryItemsCarryPosters(t *testing.T) {
	imgs := newFakeImageService()
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, Images: imgs,
			Library: api.LibraryOptions{Roots: newScanLibrary(t, defaults.Store)},
		})
	})
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie"},
		{ID: "90000002", Kind: "movie", Name: "Bravo Movie"},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, id := range []string{"90000001", "90000002"} {
		files = append(files, librarystore.LibraryFile{Path: "/media/" + id + ".mkv", Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: id})
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	held := images.Image{Hash: strings.Repeat("a1", 32), Origin: images.OriginExtracted, Role: images.RolePoster,
		Width: 20, Height: 30, OriginFetchedAt: at}
	pending := images.Image{Hash: strings.Repeat("b2", 32), Origin: images.OriginRemote, Role: images.RolePoster,
		SourceURL: "https://image.example.org/bravo.jpg"}
	for item, img := range map[string]images.Image{"90000001": held, "90000002": pending} {
		imgs.records[img.Hash] = img
		row := clipstore.Image{Hash: img.Hash, Origin: string(img.Origin), Role: string(img.Role), Visibility: "member",
			SourceURL: img.SourceURL, CreatedAt: at, UpdatedAt: at, LastUsedAt: at}
		if err := h.Store.PutImage(ctx, row); err != nil {
			t.Fatal(err)
		}
		if err := h.Store.PutImageRef(ctx, clipstore.ImageRef{ImageHash: img.Hash, OwnerKind: librarystore.LibraryItemImageOwner,
			OwnerID: item, Role: "poster"}); err != nil {
			t.Fatal(err)
		}
	}

	var list struct {
		Items []struct {
			Name   string `json:"name"`
			Poster *struct {
				Hash   string `json:"hash"`
				Height int    `json:"height"`
			} `json:"poster"`
		} `json:"items"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items", adminToken, ""), &list)
	if len(list.Items) != 2 || list.Items[0].Poster == nil || list.Items[0].Poster.Hash != held.Hash ||
		list.Items[0].Poster.Height != 30 || list.Items[1].Poster != nil {
		t.Fatalf("list = %+v", list)
	}
	// Alpha also holds a clear logo (#2567).
	logo := images.Image{Hash: strings.Repeat("c3", 32), Origin: images.OriginExtracted, Role: images.RoleLogo,
		Width: 800, Height: 310, OriginFetchedAt: at}
	imgs.records[logo.Hash] = logo
	if err := h.Store.PutImage(ctx, clipstore.Image{Hash: logo.Hash, Origin: string(logo.Origin), Role: string(logo.Role),
		Visibility: "member", CreatedAt: at, UpdatedAt: at, LastUsedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.PutImageRef(ctx, clipstore.ImageRef{ImageHash: logo.Hash, OwnerKind: librarystore.LibraryItemImageOwner,
		OwnerID: "90000001", Role: "logo"}); err != nil {
		t.Fatal(err)
	}

	var page struct {
		Item struct {
			Poster *struct {
				Hash string `json:"hash"`
			} `json:"poster"`
		} `json:"item"`
		LogoImage *struct {
			Hash string `json:"hash"`
			Src  string `json:"src"`
		} `json:"logoImage"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items/90000001", adminToken, ""), &page)
	if page.Item.Poster == nil || page.Item.Poster.Hash != held.Hash {
		t.Fatalf("page poster = %+v", page.Item.Poster)
	}
	if page.LogoImage == nil || page.LogoImage.Hash != logo.Hash || !strings.HasSuffix(page.LogoImage.Src, ".webp") {
		t.Fatalf("page logo = %+v, want the logo with a WebP fallback that keeps its alpha", page.LogoImage)
	}

	// A title with no logo and no backdrop has neither field: not null, not an empty object.
	res := h.Do(http.MethodGet, "/v1/library/items/90000002", adminToken, "")
	var raw map[string]json.RawMessage
	decode(t, res, &raw)
	for _, field := range []string{"logoImage", "backdropImage"} {
		if v, ok := raw[field]; ok {
			t.Errorf("%s = %s on a title with none, want the field absent", field, v)
		}
	}
}

// A title's page says what decides which title it is, for "Wrong title?"; Check files again puts
// its files' headers back to read, and Update details writes it again without a catalog.
func TestLibraryItemMatchAndActions(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "series", Name: "Alpha Show",
			Match: librarystore.LibraryMatch{Basis: "manager", App: "sonarr", Page: "http://sonarr.invalid/series/alpha-show"}},
		{ID: "90000002", Kind: "movie", Name: "Bravo Movie", Match: librarystore.LibraryMatch{Basis: "sidecar", App: "nfo"}},
		{ID: "90000003", Kind: "movie", Name: "Charlie Movie"},
		{ID: "90000004", Kind: "movie", Name: "Delta Movie", Sources: []librarystore.LibrarySource{
			{Path: "/media/delta.mkv", Plays: true}, {Path: "/media/delta-dvd.mkv"},
		}},
		{ID: "90000011", Kind: "episode", Name: "Alpha E1", SeriesID: "90000001"},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/media", Size: 7, ModifiedNs: 1, Kind: "movie", Identity: "digest",
			Header: librarystore.LibraryHeaderFailed, SeenAt: at, ItemID: item, SeriesID: series}
	}
	if err := h.Store.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/media/alpha/e1.mkv", "90000011", "90000001"),
		file("/media/bravo.mkv", "90000002", ""),
		file("/media/charlie.mkv", "90000003", ""),
		file("/media/delta.mkv", "90000004", ""), file("/media/delta-dvd.mkv", "90000004", ""),
	}); err != nil {
		t.Fatal(err)
	}

	type match struct {
		By   string `json:"by"`
		App  string `json:"app"`
		Page string `json:"page"`
	}
	for id, want := range map[string]match{
		"90000001": {By: "app", App: "sonarr", Page: "http://sonarr.invalid/series/alpha-show"},
		"90000002": {By: "file"},
		"90000003": {By: "none"},
	} {
		var page struct {
			Match match `json:"match"`
		}
		decode(t, h.Do(http.MethodGet, "/v1/library/items/"+id, adminToken, ""), &page)
		if page.Match != want {
			t.Errorf("%s match = %+v, want %+v", id, page.Match, want)
		}
	}

	if resp := h.Do(http.MethodPost, "/v1/library/items/90000001/check", adminToken, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("check = %d", resp.StatusCode)
	}
	files, err := h.Store.LibraryItemFiles(ctx, "90000001")
	if err != nil || len(files) != 1 || files[0].Header != librarystore.LibraryHeaderPending {
		t.Fatalf("checked files = %+v, %v; want the episode's header back to read", files, err)
	}
	if files, _ := h.Store.LibraryItemFiles(ctx, "90000002"); files[0].Header != librarystore.LibraryHeaderFailed {
		t.Errorf("another title's file was checked too: %+v", files)
	}

	if resp := h.Do(http.MethodPost, "/v1/library/items/90000002/refresh", adminToken, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("update details = %d", resp.StatusCode)
	}
	if files, _ := h.Store.LibraryItemFiles(ctx, "90000002"); files[0].Identity != "" {
		t.Errorf("updated title's file = %+v, want it written again", files[0])
	}

	// A title's only file is the copy that plays; a file it doesn't have isn't there to remove.
	for path, want := range map[string]int{"/media/bravo.mkv": http.StatusConflict, "/media/charlie.mkv": http.StatusNotFound} {
		body := `{"path":"` + path + `"}`
		if resp := h.Do(http.MethodPost, "/v1/library/items/90000002/remove-copy", adminToken, body); resp.StatusCode != want {
			t.Errorf("remove %s = %d, want %d", path, resp.StatusCode, want)
		}
	}
	// An extra copy whose playing copy is listed but not on disk is kept, and the answer says why.
	resp := h.Do(http.MethodPost, "/v1/library/items/90000004/remove-copy", adminToken, `{"path":"/media/delta-dvd.mkv"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("remove an extra whose playing copy is gone = %d, want 409", resp.StatusCode)
	}
	kept := decodeProblem(t, resp.Body)
	_ = resp.Body.Close()
	if kept.Title != "Kept this copy" || !strings.Contains(kept.Detail, "can't find the copy that plays right now") {
		t.Errorf("kept copy answer = %+v", kept)
	}
	if resp := h.Do(http.MethodPost, "/v1/library/items/90000002/remove-copy", memberToken, `{"path":"/media/bravo.mkv"}`); resp.StatusCode != http.StatusForbidden {
		t.Errorf("remove by a member = %d, want 403", resp.StatusCode)
	}

	for _, action := range []string{"check", "refresh"} {
		if resp := h.Do(http.MethodPost, "/v1/library/items/90000099/"+action, adminToken, ""); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown title = %d, want 404", action, resp.StatusCode)
		}
		if resp := h.Do(http.MethodPost, "/v1/library/items/90000001/"+action, memberToken, ""); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s by a member = %d, want 403", action, resp.StatusCode)
		}
	}
}

func decode(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}
