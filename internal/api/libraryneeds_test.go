package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// externalID is a title's catalog id on the wire.
type externalID struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
}

// Needs you lists the titles nothing grounds with their candidates and files, counts them in the
// status, takes a link, an ignore and a take-back, searches TMDB when it is set up, and is for
// admins only.
func TestLibraryNeedsYou(t *testing.T) {
	imgs := newFakeImageService()
	var lib *scan.Library
	var searched []string
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib = newScanLibrary(t, defaults.Store)
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, Images: imgs, Library: api.LibraryOptions{Roots: lib},
		})
	})
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	if err := h.Store.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "90000001", Kind: "movie", Name: "Mystery Film", Origin: origin("file:a")},
		{ID: "90000002", Kind: "movie", Name: "Known Film", Origin: origin("file:b"), Match: librarystore.LibraryMatch{Basis: "embedded_id"}},
		{ID: "90000003", Kind: "movie", Name: "Arr Film"},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, id := range []string{"90000001", "90000002", "90000003"} {
		files = append(files, librarystore.LibraryFile{Path: "/media/" + id + ".mkv", Root: "/media", Size: 7, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: id})
	}
	// Arr Film's file doesn't parse: it needs a look.
	files[2].Header = librarystore.LibraryHeaderFailed
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	posterURL := "https://image.example.org/90000101.jpg"
	if err := h.Store.SaveLibraryCandidates(ctx, "90000001", "Mystery Film|0", []librarystore.LibraryCandidate{
		{Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000101"}, Name: "Mystery Film", Year: 2002,
			Poster: posterURL, NameMatch: true, LengthMatch: true},
		{Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000102"}, Name: "Mystery Films", Year: 2003, Poster: "https://image.example.org/pending.jpg"},
	}); err != nil {
		t.Fatal(err)
	}
	imgs.records[posterURL] = images.Image{Hash: strings.Repeat("c3", 32), Origin: images.OriginRemote,
		Role: images.RolePoster, SourceURL: posterURL, Width: 20, Height: 30, OriginFetchedAt: at}

	type needs struct {
		Needs []struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
			Searched   bool `json:"searched"`
			Candidates []struct {
				ExternalID    externalID `json:"externalId"`
				NameMatches   bool       `json:"nameMatches"`
				YearMatches   bool       `json:"yearMatches"`
				LengthMatches bool       `json:"lengthMatches"`
				Poster        *struct {
					Hash string `json:"hash"`
				} `json:"poster"`
			} `json:"candidates"`
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"needs"`
		Problems []struct {
			Item struct {
				ID          string `json:"id"`
				Unplayable  int    `json:"unplayable"`
				ExtraCopies int    `json:"extraCopies"`
			} `json:"item"`
			ExtraBytes int64 `json:"extraBytes"`
		} `json:"problems"`
	}
	var got needs
	decode(t, h.Do(http.MethodGet, "/v1/library/needs", adminToken, ""), &got)
	if len(got.Needs) != 1 || got.Needs[0].Item.ID != "90000001" || !got.Needs[0].Searched ||
		len(got.Needs[0].Files) != 1 || len(got.Needs[0].Candidates) != 2 {
		t.Fatalf("needs = %+v", got)
	}
	best, other := got.Needs[0].Candidates[0], got.Needs[0].Candidates[1]
	if best.ExternalID != (externalID{"tmdb", "90000101"}) || !best.NameMatches || best.YearMatches || !best.LengthMatches ||
		best.Poster == nil || other.Poster != nil {
		t.Fatalf("candidates = %+v, want the fetched poster only", got.Needs[0].Candidates)
	}
	if len(got.Problems) != 1 || got.Problems[0].Item.ID != "90000003" || got.Problems[0].Item.Unplayable != 1 ||
		got.Problems[0].Item.ExtraCopies != 0 || got.Problems[0].ExtraBytes != 0 {
		t.Fatalf("problems = %+v, want Arr Film with one file that can't be played", got.Problems)
	}
	var status struct {
		NeedsYou int `json:"needsYou"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/status", adminToken, ""), &status)
	if status.NeedsYou != 2 {
		t.Fatalf("status needsYou = %d, want the title to answer and the one that needs a look", status.NeedsYou)
	}

	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/v1/library/items/90000001/link", `{"externalId":{"value":"90000101"}}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "/v1/library/items/90000099/link", `{"externalId":{"namespace":"tmdb","value":"90000101"}}`, http.StatusNotFound},
		{http.MethodPost, "/v1/library/items/90000003/ignore", ``, http.StatusConflict},
		{http.MethodPost, "/v1/library/items/90000001/link", `{"externalId":{"namespace":"tmdb","value":"90000101"}}`, http.StatusNoContent},
	} {
		resp := h.Do(tc.method, tc.path, adminToken, tc.body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s %s = %d, want %d", tc.method, tc.path, tc.body, resp.StatusCode, tc.want)
		}
	}
	answer, ok, err := h.Store.LibraryAnswerFor(ctx, origin("file:a"))
	if err != nil || !ok || answer.ID != (librarystore.LibraryID{Namespace: "tmdb", Value: "90000101"}) || answer.Kind != "movie" {
		t.Fatalf("answer = %+v, %v, %v", answer, ok, err)
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/needs", adminToken, ""), &got)
	if len(got.Needs) != 0 {
		t.Fatalf("needs after the link = %+v", got)
	}

	if resp := h.Do(http.MethodDelete, "/v1/library/items/90000001/answer", adminToken, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("take back = %d", resp.StatusCode)
	}
	if resp := h.Do(http.MethodPost, "/v1/library/items/90000001/ignore", adminToken, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ignore = %d", resp.StatusCode)
	}
	var list struct {
		Total int `json:"total"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/items", adminToken, ""), &list)
	if list.Total != 2 {
		t.Fatalf("library after the ignore has %d titles, want 2", list.Total)
	}

	// No catalog: the search says so.
	if resp := h.Do(http.MethodGet, "/v1/library/search?q=mystery", adminToken, ""); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("search without TMDB = %d, want 503", resp.StatusCode)
	}
	lib.Search = func(_ context.Context, term string, _ int) ([]scan.SearchHit, error) {
		searched = append(searched, term)
		return []scan.SearchHit{
			{Kind: "series", ID: mp.ExternalID{Namespace: "tmdb", Value: "90000201"}, Name: "Mystery"},
			{Kind: "movie", ID: mp.ExternalID{Namespace: "imdb", Value: "tt90000202"}, Name: "Mystery", Year: 2002},
		}, nil
	}
	var results struct {
		Results []struct {
			ExternalID externalID `json:"externalId"`
		} `json:"results"`
	}
	decode(t, h.Do(http.MethodGet, "/v1/library/search?q=mystery&kind=movie", adminToken, ""), &results)
	if len(searched) != 1 || searched[0] != "mystery" || len(results.Results) != 1 ||
		results.Results[0].ExternalID != (externalID{"imdb", "tt90000202"}) {
		t.Fatalf("search = %v, %+v", searched, results)
	}

	if resp := h.Do(http.MethodGet, "/v1/library/needs", memberToken, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("member = %d, want 403", resp.StatusCode)
	}
}
