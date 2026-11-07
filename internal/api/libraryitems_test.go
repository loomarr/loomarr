package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// Library → Items lists present titles with their readiness, finds them by name, opens a series
// with its episodes, and is for admins only.
func TestLibraryItemsListAndPage(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := h.Store.SaveLibraryItems(ctx, []store.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Alpha Movie", Year: 2001},
		{ID: "90000002", Kind: "series", Name: "Bravo Show", Year: 2010},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string) store.LibraryFile {
		return store.LibraryFile{Path: path, Root: "/media", Size: 7, ModifiedNs: 1, Kind: "movie", Header: header,
			SeenAt: at, ItemID: item, SeriesID: series}
	}
	if err := h.Store.SaveLibraryFiles(ctx, []store.LibraryFile{
		file("/media/alpha.mkv", "90000001", "", store.LibraryHeaderRead),
		file("/media/bravo/e1.mkv", "90000011", "90000002", store.LibraryHeaderRead),
		file("/media/bravo/e2.mkv", "90000012", "90000002", store.LibraryHeaderPending),
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

	if resp := h.Do(http.MethodGet, "/v1/library/items/90000099", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown title = %d, want 404", resp.StatusCode)
	}
	if resp := h.Do(http.MethodGet, "/v1/library/items", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member = %d, want 403", resp.StatusCode)
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
