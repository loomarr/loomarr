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
}
