package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

type collectionBody struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Titles      []struct {
		ID string `json:"id"`
	} `json:"titles"`
	UsedBy []struct {
		Name   string `json:"name"`
		Number int    `json:"number"`
	} `json:"usedBy"`
}

func (c collectionBody) titleIDs() string {
	ids := []string{}
	for _, t := range c.Titles {
		ids = append(ids, t.ID)
	}
	return fmt.Sprint(ids)
}

func decodeCollection(t *testing.T, resp *http.Response, want int) collectionBody {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	var body collectionBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// Ready-made sets: a set the library has movies of is listed with its total and those movies in
// its order, and copying it makes a collection of the household's own.
func TestReadyMadeSets(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	set := store.LibraryCatalogKey{Provider: "tmdb_collection", Kind: "collection", ExternalID: "90000611"}
	movie := func(id string) store.LibraryCatalogKey {
		return store.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: id}
	}
	if err := h.Store.SaveLibraryItems(ctx, []store.LibraryItem{
		{ID: "90000021", Kind: "movie", Name: "Returns", Catalog: movie("90000613"), Set: set},
		{ID: "90000022", Kind: "movie", Name: "Keeper", Catalog: movie("90000612"), Set: set},
		{ID: "90000023", Kind: "movie", Name: "Harbour", Catalog: movie("90000620")},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []store.LibraryFile
	for _, id := range []string{"90000021", "90000022", "90000023"} {
		files = append(files, store.LibraryFile{Path: "/media/" + id + ".mkv", Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "movie", Header: store.LibraryHeaderRead, SeenAt: at, ItemID: id})
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	record := `{"Kind":"collection","Descriptive":{"Title":"The Lantern Keeper Collection"},"Children":[
		{"Kind":"movie","IDs":[{"Namespace":"tmdb","Value":"90000612"}]},
		{"Kind":"movie","IDs":[{"Namespace":"tmdb","Value":"90000613"}]},
		{"Kind":"movie","IDs":[{"Namespace":"tmdb","Value":"90000614"}]}]}`
	if err := h.Store.WantLibraryCatalog(ctx, []store.LibraryCatalogKey{set}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Store.SaveLibraryCatalog(ctx, set, record, 0, "en-US", at); err != nil {
		t.Fatal(err)
	}

	resp := h.Do(http.MethodGet, "/v1/collections", adminToken, "")
	var list struct {
		Sets []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Total  int    `json:"total"`
			Titles []struct {
				ID string `json:"id"`
			} `json:"titles"`
		} `json:"sets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(list.Sets) != 1 {
		t.Fatalf("sets = %+v", list.Sets)
	}
	s := list.Sets[0]
	if s.ID != "90000611" || s.Name != "The Lantern Keeper Collection" || s.Total != 3 ||
		len(s.Titles) != 2 || s.Titles[0].ID != "90000022" || s.Titles[1].ID != "90000021" {
		t.Fatalf("set = %+v, want Keeper then Returns of 3", s)
	}

	copied := decodeCollection(t, h.Do(http.MethodPost, "/v1/collections/sets/90000611/copy", adminToken, ""), http.StatusCreated)
	if copied.Name != "The Lantern Keeper Collection" || copied.titleIDs() != "[90000022 90000021]" {
		t.Fatalf("copy = %+v", copied)
	}
	if resp := h.Do(http.MethodPost, "/v1/collections/sets/90000699/copy", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("copy unknown set = %d, want 404", resp.StatusCode)
	}
}

// Collections: the household makes one, orders it, and can't delete it while a channel uses it;
// a member can't see them.
func TestCollections(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	if err := h.Store.SaveLibraryItems(ctx, []store.LibraryItem{
		{ID: "90000011", Kind: "movie", Name: "Alpha"},
		{ID: "90000012", Kind: "movie", Name: "Bravo"},
		{ID: "90000013", Kind: "series", Name: "Charlie"},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []store.LibraryFile
	for _, id := range []string{"90000011", "90000012", "90000013"} {
		files = append(files, store.LibraryFile{Path: "/media/" + id + ".mkv", Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "movie", Header: store.LibraryHeaderRead, SeenAt: at, ItemID: id})
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}

	if resp := h.Do(http.MethodPost, "/v1/collections", adminToken, `{"name":"  ","titles":[]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no name = %d, want 422", resp.StatusCode)
	}
	made := decodeCollection(t, h.Do(http.MethodPost, "/v1/collections", adminToken,
		`{"name":"Rainy days","titles":["90000013","90000099","90000011"]}`), http.StatusCreated)
	if made.ID == "" || made.titleIDs() != "[90000013 90000011]" || len(made.UsedBy) != 0 {
		t.Fatalf("made %+v, want Charlie then Alpha, the unknown id left out", made)
	}

	edited := decodeCollection(t, h.Do(http.MethodPut, "/v1/collections/"+made.ID, adminToken,
		`{"name":"Rainy days","description":"For the weekend","titles":["90000012","90000011"]}`), http.StatusOK)
	if edited.titleIDs() != "[90000012 90000011]" || edited.Description != "For the weekend" {
		t.Fatalf("edited %+v", edited)
	}

	if _, err := h.Store.SaveChannel(ctx, channelstore.Channel{Channel: schedule.Channel{
		ID: "ch-4", Name: "Saturday Morning", Number: 4, Strategy: schedule.Shuffle, Status: schedule.StatusLive,
	}, Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Rules: []schedule.SchedulingRule{
		{What: &schedule.ScopePolicy{Collections: []string{made.ID}}},
	}}}}); err != nil {
		t.Fatal(err)
	}
	got := decodeCollection(t, h.Do(http.MethodGet, "/v1/collections/"+made.ID, adminToken, ""), http.StatusOK)
	if len(got.UsedBy) != 1 || got.UsedBy[0].Name != "Saturday Morning" || got.UsedBy[0].Number != 4 {
		t.Fatalf("used by %+v, want Saturday Morning", got.UsedBy)
	}
	if resp := h.Do(http.MethodDelete, "/v1/collections/"+made.ID, adminToken, ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete in use = %d, want 409", resp.StatusCode)
	}

	resp := h.Do(http.MethodGet, "/v1/collections", adminToken, "")
	var list struct {
		Collections []collectionBody `json:"collections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(list.Collections) != 1 || list.Collections[0].titleIDs() != "[90000012 90000011]" {
		t.Fatalf("list %+v", list.Collections)
	}

	if resp := h.Do(http.MethodGet, "/v1/collections", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member list = %d, want 403", resp.StatusCode)
	}

	other := decodeCollection(t, h.Do(http.MethodPost, "/v1/collections", adminToken,
		`{"name":"Unused","titles":[]}`), http.StatusCreated)
	if resp := h.Do(http.MethodDelete, "/v1/collections/"+other.ID, adminToken, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", resp.StatusCode)
	}
	if resp := h.Do(http.MethodGet, "/v1/collections/"+other.ID, adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read deleted = %d, want 404", resp.StatusCode)
	}
}
