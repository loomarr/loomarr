package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A show whose episodes keep their files' names lists in Needs you with how far each season runs
// on disk, its waiting files, and TMDB's own seasons then its other orders, the one whose sizes fit
// the disk marked; it counts in the status; an answer is checked against what TMDB lists and kept;
// and it is for admins only (#2151 decision 53).
func TestLibraryEpisodeOrderNeedsYou(t *testing.T) {
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, LibraryRoots: newScanLibrary(t, defaults.Store),
		})
	})
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	harbor := librarystore.LibraryOrigin{Authority: "library:/media", Key: "series:dir:harbor"}
	seriesID, err := inventory.New(h.Store).ApplySnapshot(ctx, inventory.Snapshot{
		Origin: inventory.OriginKey{Authority: inventory.AuthorityID(harbor.Authority), ExternalItemID: harbor.Key},
		Kind:   "series",
		Observation: inventory.Observation[inventory.ItemFacts]{SchemaVersion: 1, ObservedAt: at,
			Facts: inventory.ItemFacts{Name: "Harbor Lights"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	show := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000701"}
	episode := func(id string, number int) librarystore.LibraryItem {
		return librarystore.LibraryItem{ID: id, Kind: "episode", Name: id, Series: harbor, Season: 1, Episode: number, Unnumbered: true}
	}
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: string(seriesID), Kind: "series", Name: "Harbor Lights", Origin: harbor, Catalog: show,
			Match: librarystore.LibraryMatch{Basis: "embedded_id"}},
		episode("90000052", 2), episode("90000051", 1),
		{ID: "90000060", Kind: "movie", Name: "Evening Tides", Origin: librarystore.LibraryOrigin{Authority: "library:/media", Key: "file:tides"}},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, id := range []string{"90000051", "90000052"} {
		files = append(files, librarystore.LibraryFile{Path: "/media/Harbor Lights/" + id + ".mkv", Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "episode", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: id, SeriesID: string(seriesID)})
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	dvd := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: string(mp.KindEpisodeOrder), ExternalID: "90000799a"}
	cache := func(key librarystore.LibraryCatalogKey, rec mp.Record) {
		t.Helper()
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Store.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{key}); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Store.SaveLibraryCatalog(ctx, key, string(raw), 0, "en-US", at); err != nil {
			t.Fatal(err)
		}
	}
	cache(show, mp.Record{Kind: mp.KindSeries, Descriptive: &mp.Descriptive{
		Title: "Harbor Lights", Seasons: []mp.SeasonSize{{Number: 1, Episodes: 3}},
		Orders: []mp.EpisodeOrder{
			{ID: "90000799a", Name: "DVD order", Kind: mp.OrderDVD, Seasons: 1, Episodes: 2},
			{ID: "90000799b", Name: "Story arcs", Kind: mp.OrderStory, Seasons: 2, Episodes: 3},
		},
	}})
	cache(dvd, mp.Record{Kind: mp.KindEpisodeOrder, Descriptive: &mp.Descriptive{
		Title: "DVD order", Seasons: []mp.SeasonSize{{Number: 1, Episodes: 2}},
	}})

	type size struct {
		Season   int `json:"season"`
		Episodes int `json:"episodes"`
	}
	type choice struct {
		Choice   string `json:"choice"`
		OrderID  string `json:"orderId"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		Seasons  []size `json:"seasons"`
		Episodes int    `json:"episodes"`
		Fits     bool   `json:"fits"`
	}
	var got struct {
		Orders []struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
			Disk  []size `json:"disk"`
			Files []struct {
				Path    string `json:"path"`
				Season  int    `json:"season"`
				Episode int    `json:"episode"`
			} `json:"files"`
			Choices []choice `json:"choices"`
		} `json:"orders"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/needs", adminToken, ""), &got)
	if len(got.Orders) != 1 || got.Orders[0].Item.ID != string(seriesID) {
		t.Fatalf("orders = %+v, want Harbor Lights", got.Orders)
	}
	need := got.Orders[0]
	if !reflect.DeepEqual(need.Disk, []size{{1, 2}}) || len(need.Files) != 2 ||
		need.Files[0].Path != "/media/Harbor Lights/90000051.mkv" || need.Files[0].Season != 1 || need.Files[0].Episode != 1 {
		t.Fatalf("disk %+v, files %+v; want season 1 running to 2 and both files in path order", need.Disk, need.Files)
	}
	want := []choice{
		{Choice: "catalog", Seasons: []size{{1, 3}}, Episodes: 3},
		{Choice: "order", OrderID: "90000799a", Kind: "dvd", Name: "DVD order", Seasons: []size{{1, 2}}, Episodes: 2, Fits: true},
		// Its layout isn't fetched yet: what the show's record says of it, and no fit.
		{Choice: "order", OrderID: "90000799b", Kind: "story", Name: "Story arcs", Seasons: []size{}, Episodes: 3},
	}
	if !reflect.DeepEqual(need.Choices, want) {
		t.Fatalf("choices = %+v\nwant %+v", need.Choices, want)
	}
	var status struct {
		NeedsYou int `json:"needsYou"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), &status)
	if status.NeedsYou != 1 {
		t.Fatalf("status needsYou = %d, want the show", status.NeedsYou)
	}

	path := "/v1/library/items/" + string(seriesID) + "/episode-order"
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{path, `{"choice":"aired"}`, http.StatusUnprocessableEntity},
		{path, `{"choice":"order"}`, http.StatusBadRequest},
		{path, `{"choice":"files","orderId":"90000799a"}`, http.StatusBadRequest},
		{path, `{"choice":"order","orderId":"90000799z"}`, http.StatusConflict},
		{"/v1/library/items/90000099/episode-order", `{"choice":"files"}`, http.StatusNotFound},
		{"/v1/library/items/90000060/episode-order", `{"choice":"files"}`, http.StatusConflict},
		{path, `{"choice":"order","orderId":"90000799a"}`, http.StatusNoContent},
	} {
		resp := h.Do(http.MethodPost, tc.path, adminToken, tc.body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("POST %s %s = %d, want %d", tc.path, tc.body, resp.StatusCode, tc.want)
		}
	}
	answer, ok, err := h.Store.LibraryEpisodeOrderFor(ctx, show)
	if err != nil || !ok || answer.Choice != librarystore.OrderByOrder || answer.OrderID != "90000799a" {
		t.Fatalf("answer = %+v, %v, %v; want DVD order", answer, ok, err)
	}

	if resp := h.Do(http.MethodPost, path, memberToken, `{"choice":"files"}`); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member = %d, want 403", resp.StatusCode)
	}
}
