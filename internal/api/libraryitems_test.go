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
