package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"

	"github.com/loomarr/loomarr/internal/library/scan"
)

// The cards under one folder heading are ignored together, listed as ignored, and brought back
// together; a request with a stale id changes nothing; and both are for admins whose session is
// live (design §19).
func TestLibraryNeedsFolderBatch(t *testing.T) {
	h := newLibrarySessionHarness(t)
	srv := h.Server
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	var items []librarystore.LibraryItem
	var files []librarystore.LibraryFile
	// By name the order is B, A, C; by folder it is A and C (Loose), then B (Shorts).
	for _, f := range [][3]string{
		{"90000011", "Charlie Film", "/media/loose/c.mkv"},
		{"90000012", "Alpha Film", "/media/shorts/b.mkv"},
		{"90000013", "Bravo Film", "/media/loose/a.mkv"},
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

	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")
	call := func(cookie *http.Cookie, method, path, body string, want int, into any) {
		t.Helper()
		resp := authed(t, method, srv.URL+path, cookie, body)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != want {
			t.Fatalf("%s %s %s = %d, want %d", method, path, body, resp.StatusCode, want)
		}
		if into != nil {
			if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
				t.Fatal(err)
			}
		}
	}
	waiting := func() []string {
		t.Helper()
		var got struct {
			Needs []struct {
				Item struct {
					ID string `json:"id"`
				} `json:"item"`
				Folder string `json:"folder"`
			} `json:"needs"`
		}
		call(admin, http.MethodGet, "/v1/library/needs", "", http.StatusOK, &got)
		out := []string{}
		for _, n := range got.Needs {
			out = append(out, n.Folder+" "+n.Item.ID)
		}
		return out
	}
	ignored := func() []string {
		t.Helper()
		var got struct {
			Items []struct {
				ID     string `json:"id"`
				Folder string `json:"folder"`
			} `json:"items"`
		}
		call(admin, http.MethodGet, "/v1/library/ignored", "", http.StatusOK, &got)
		out := []string{}
		for _, i := range got.Items {
			out = append(out, i.Folder+" "+i.ID)
		}
		slices.Sort(out)
		return out
	}
	check := func(step string, got []string, want ...string) {
		t.Helper()
		if strings.Join(got, ", ") != strings.Join(want, ", ") {
			t.Fatalf("%s = %q, want %q", step, got, want)
		}
	}
	all := []string{"Media/loose 90000013", "Media/loose 90000011", "Media/shorts 90000012"}
	check("needs", waiting(), all...)

	const ignore, restore = "/v1/library/needs/ignore", "/v1/library/ignored/restore"
	heading := `{"ids":["90000013","90000011"]}`
	call(member, http.MethodPost, ignore, heading, http.StatusForbidden, nil)
	check("needs after a member's ignore", waiting(), all...)
	var problem struct {
		Title string `json:"title"`
	}
	call(admin, http.MethodPost, ignore, `{"ids":["90000013","90000099"]}`, http.StatusConflict, &problem)
	if problem.Title != "Needs you has changed" {
		t.Errorf("stale ignore says %q", problem.Title)
	}
	check("needs after a stale ignore", waiting(), all...)

	var done struct {
		IDs []string `json:"ids"`
	}
	call(admin, http.MethodPost, ignore, heading, http.StatusOK, &done)
	check("ignored ids", done.IDs, "90000013", "90000011")
	check("needs after the ignore", waiting(), "Media/shorts 90000012")
	check("the ignored list", ignored(), "Media/loose 90000011", "Media/loose 90000013")

	call(member, http.MethodPost, restore, heading, http.StatusForbidden, nil)
	call(admin, http.MethodPost, restore, `{"ids":["90000013","90000012"]}`, http.StatusConflict, &problem)
	if problem.Title != "Your ignored list has changed" {
		t.Errorf("stale restore says %q", problem.Title)
	}
	check("the ignored list after refusals", ignored(), "Media/loose 90000011", "Media/loose 90000013")
	call(admin, http.MethodPost, restore, heading, http.StatusNoContent, nil)
	check("needs after the restore", waiting(), all...)
	check("the ignored list after the restore", ignored())

	call(admin, http.MethodPatch, "/v1/users/u-kid", `{"role":"admin"}`, http.StatusOK, nil)
	second := login(t, srv, "kid", "pw")
	call(admin, http.MethodPatch, "/v1/users/u-kid", `{"disabled":true}`, http.StatusOK, nil)
	call(second, http.MethodPost, ignore, heading, http.StatusUnauthorized, nil)
	call(second, http.MethodPost, restore, heading, http.StatusUnauthorized, nil)
	check("needs after a disabled admin's calls", waiting(), all...)
}
