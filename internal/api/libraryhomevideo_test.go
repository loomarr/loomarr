package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// An admin keeps a waiting title with no guess as a home video and can take that back; a title with
// a guess is refused; members read a home video but cannot answer, and a disabled session cannot
// either (design §19).
func TestLibraryKeepHomeVideo(t *testing.T) {
	h := newLibrarySessionHarness(t)
	srv := h.Server
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	var items []librarystore.LibraryItem
	var files []librarystore.LibraryFile
	for _, f := range [][3]string{
		{"90000021", "Beach Day", "/media/home/a.mkv"},
		{"90000022", "Guessed Film", "/media/home/b.mkv"},
	} {
		items = append(items, librarystore.LibraryItem{ID: f[0], Kind: "movie", Name: f[1],
			Origin: librarystore.LibraryOrigin{Authority: string(scan.Authority("/media")), Key: f[2]}})
		files = append(files, librarystore.LibraryFile{Path: f[2], Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: f[0]})
	}
	if err := h.Store.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.SaveLibraryCandidates(ctx, "90000022", "guessed film", []librarystore.LibraryCandidate{
		{Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000301"}, Name: "Guessed Film"},
	}); err != nil {
		t.Fatal(err)
	}

	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")
	call := func(cookie *http.Cookie, method, path, body string, want int, into any) {
		t.Helper()
		resp := authed(t, method, srv.URL+path, cookie, body)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != want {
			t.Fatalf("%s %s = %d, want %d", method, path, resp.StatusCode, want)
		}
		if into != nil {
			if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
				t.Fatal(err)
			}
		}
	}
	waiting := func() int {
		t.Helper()
		var got struct {
			Needs []struct{} `json:"needs"`
		}
		call(admin, http.MethodGet, "/v1/library/needs", "", http.StatusOK, &got)
		return len(got.Needs)
	}

	// flags is each listed title's, and the detail read's, for the caller.
	type flags struct {
		ID           string `json:"id"`
		HomeVideo    bool   `json:"homeVideo"`
		Unidentified bool   `json:"unidentified"`
	}
	listed := func(cookie *http.Cookie) map[string]flags {
		t.Helper()
		var got struct {
			Items []flags `json:"items"`
		}
		call(cookie, http.MethodGet, "/v1/library/items", "", http.StatusOK, &got)
		out := map[string]flags{}
		for _, i := range got.Items {
			out[i.ID] = i
		}
		return out
	}
	if got := listed(admin)["90000021"]; !got.Unidentified || got.HomeVideo {
		t.Fatalf("a waiting title reads %+v", got)
	}

	const keep, answer = "/v1/library/items/90000021/keep", "/v1/library/items/90000021/answer"
	call(member, http.MethodPost, keep, "", http.StatusForbidden, nil)
	if n := waiting(); n != 2 {
		t.Fatalf("waiting after a member's keep = %d, want 2", n)
	}
	call(member, http.MethodGet, "/v1/library/items/90000021", "", http.StatusNotFound, nil)
	call(admin, http.MethodPost, "/v1/library/items/90000099/keep", "", http.StatusNotFound, nil)
	call(admin, http.MethodPost, "/v1/library/items/90000022/keep", "", http.StatusConflict, nil)

	call(admin, http.MethodPost, keep, "", http.StatusNoContent, nil)
	if n := waiting(); n != 1 {
		t.Fatalf("waiting after the keep = %d, want 1", n)
	}
	call(admin, http.MethodPost, keep, "", http.StatusConflict, nil)
	var detail struct {
		Item flags `json:"item"`
	}
	call(member, http.MethodGet, "/v1/library/items/90000021", "", http.StatusOK, &detail)
	if !detail.Item.HomeVideo {
		t.Fatalf("a member's title page reads %+v, want homeVideo", detail)
	}
	for who, cookie := range map[string]*http.Cookie{"admin": admin, "member": member} {
		if got := listed(cookie)["90000021"]; !got.HomeVideo || got.Unidentified {
			t.Fatalf("the %s's list reads the home video as %+v", who, got)
		}
	}
	if got := listed(member); len(got) != 1 {
		t.Fatalf("a member lists %v, want only the home video", got)
	}

	call(member, http.MethodDelete, answer, "", http.StatusForbidden, nil)
	call(admin, http.MethodDelete, answer, "", http.StatusNoContent, nil)
	if n := waiting(); n != 2 {
		t.Fatalf("waiting after taking it back = %d, want 2", n)
	}

	call(admin, http.MethodPatch, "/v1/users/u-kid", `{"role":"admin"}`, http.StatusOK, nil)
	second := login(t, srv, "kid", "pw")
	call(admin, http.MethodPatch, "/v1/users/u-kid", `{"disabled":true}`, http.StatusOK, nil)
	call(second, http.MethodPost, keep, "", http.StatusUnauthorized, nil)
	if n := waiting(); n != 2 {
		t.Fatalf("waiting after a disabled admin's keep = %d, want 2", n)
	}
}
