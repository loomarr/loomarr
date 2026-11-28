package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A title's credits carry an imported photo as an image reference, for a member as for an admin,
// and never the source's URL (#4476). A credit with no photo has no field.
func TestLibraryCreditsCarryPhotos(t *testing.T) {
	imgs := newFakeImageService()
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, Images: imgs,
			Library: api.LibraryOptions{Roots: newScanLibrary(t, defaults.Store)},
		})
	})
	seedMemberLibrary(t, h.Store)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	img := images.Image{Hash: strings.Repeat("c3", 32), Origin: images.OriginRemote, Role: images.RolePoster,
		SourceURL: "https://image.tmdb.org/t/p/w185/ada.jpg", Width: 185, Height: 278, OriginFetchedAt: at}
	imgs.records[img.Hash] = img
	if err := h.Store.PutImage(ctx, clipstore.Image{Hash: img.Hash, Origin: string(img.Origin), Role: string(img.Role),
		Visibility: "member", SourceURL: img.SourceURL, OriginFetchedAt: at, CreatedAt: at, UpdatedAt: at, LastUsedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.PutImageRef(ctx, clipstore.ImageRef{ImageHash: img.Hash, OwnerKind: librarystore.LibraryPersonImageOwner,
		OwnerID: "name:Ada Quill", Role: "poster"}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{adminToken, memberToken} {
		res := h.Do(http.MethodGet, "/v1/library/items/90000001", token, "")
		var page struct {
			Credits []struct {
				Name  string `json:"name"`
				Photo *struct {
					Hash string `json:"hash"`
				} `json:"photo"`
			} `json:"credits"`
		}
		decode(t, res, &page)
		if len(page.Credits) == 0 || page.Credits[0].Photo == nil || page.Credits[0].Photo.Hash != img.Hash {
			t.Fatalf("%s credits = %+v, want Ada's photo", token, page.Credits)
		}
		body := readBody(t, h.Do(http.MethodGet, "/v1/library/items/90000001", token, ""))
		if strings.Contains(body, "image.tmdb.org") {
			t.Errorf("%s page sends the source URL: %s", token, body)
		}
	}
	body := readBody(t, h.Do(http.MethodGet, "/v1/library/items/90000002", memberToken, ""))
	if strings.Contains(body, `"photo"`) {
		t.Errorf("a credit with no photo has a photo field: %s", body)
	}

	// A person credited only on a title a member doesn't read: their photo reaches no member.
	hidden := img
	hidden.Hash = strings.Repeat("d4", 32)
	imgs.records[hidden.Hash] = hidden
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{{ID: unidentifiedMovie, Kind: "movie",
		Name: "Charlie Folder Extras", Credits: []librarystore.LibraryCredit{{Name: "Hal Verity", Role: "director"}}}}, at); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.PutImage(ctx, clipstore.Image{Hash: hidden.Hash, Origin: string(hidden.Origin), Role: string(hidden.Role),
		Visibility: "member", OriginFetchedAt: at, CreatedAt: at, UpdatedAt: at, LastUsedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.PutImageRef(ctx, clipstore.ImageRef{ImageHash: hidden.Hash, OwnerKind: librarystore.LibraryPersonImageOwner,
		OwnerID: "name:Hal Verity", Role: "poster"}); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, h.Do(http.MethodGet, "/v1/library/items/"+unidentifiedMovie, adminToken, "")); !strings.Contains(body, hidden.Hash) {
		t.Fatalf("the admin's page has no photo to withhold: %s", body)
	}
	if res := h.Do(http.MethodGet, "/v1/library/items/"+unidentifiedMovie, memberToken, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("member's read of a hidden title = %d, want 404", res.StatusCode)
	}
	for _, id := range []string{"90000001", "90000002", "90000011"} {
		if body := readBody(t, h.Do(http.MethodGet, "/v1/library/items/"+id, memberToken, "")); strings.Contains(body, hidden.Hash) {
			t.Errorf("member's %s page carries a hidden title's person: %s", id, body)
		}
	}
}
