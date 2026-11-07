package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/store"
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

	if _, err := h.Store.SaveChannel(ctx, store.Channel{Channel: schedule.Channel{
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
