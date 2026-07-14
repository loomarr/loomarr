package library

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/filler"
)

// Live TV filler catalog read (§10): loomarr syncs its clip catalog FROM the
// media server's dedicated filler library. Item ids, names, and DURATION come
// from the server (it already probes media) — the core never touches ffprobe or
// downloads anything. This reuses the same /Items surface as Lookup/Search (§6),
// scoped to the filler library via ParentId.

// FillerClip is a raw clip as read from the media server's filler library. The
// sync (§10) maps these into store clips; era/audience/category start from the
// filename convention and are refined by AI tagging.
type FillerClip struct {
	LibraryItemID string
	Name          string
	DurationMs    int64
	Kind          filler.Kind // inferred from filename/folder convention (initial tag)
	Era           int         // year parsed from the filename, 0 if none (initial tag)
}

// fillerItem mirrors the /Items slice we need for filler. RunTimeTicks is the
// server-probed duration in 100-ns units (Emby/Jellyfin convention).
type fillerItem struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RunTimeTicks int64  `json:"RunTimeTicks"`
	Path         string `json:"Path"`
}

type fillerItemsResponse struct {
	Items []fillerItem `json:"Items"`
}

// ticksPerMs converts Emby/Jellyfin RunTimeTicks (100-ns units) to milliseconds.
const ticksPerMs = 10_000

// ListFillerClips reads every item in the media server's filler library (§10):
//
//	GET /Items?Recursive=true&ParentId=<fillerLibraryID>&IncludeItemTypes=Video
//	           &Fields=Path&Limit=<n>
//
// Duration comes from RunTimeTicks. fillerLibraryID is the library's item id
// (FILLER_LIBRARY, §15); the caller resolves a name→id if configured by name.
func (c *Client) ListFillerClips(ctx context.Context, fillerLibraryID string) ([]FillerClip, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("ParentId", fillerLibraryID)
	q.Set("IncludeItemTypes", "Video")
	q.Set("Fields", "Path")
	q.Set("Limit", "5000")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)

	var out fillerItemsResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	clips := make([]FillerClip, 0, len(out.Items))
	for _, it := range out.Items {
		clips = append(clips, FillerClip{
			LibraryItemID: it.ID,
			Name:          it.Name,
			DurationMs:    it.RunTimeTicks / ticksPerMs,
			Kind:          kindFromName(it.Name, it.Path),
			Era:           eraFromName(it.Name),
		})
	}
	return clips, nil
}

// kindFromName infers a clip Kind from filename/folder convention (§10 — the
// cheapest tagging tier). It's a starting point; AI tagging (§10) refines
// era/audience/category. Falls back to commercial (the common case) so unknown
// clips are still placeable as ads, never programs.
func kindFromName(name, filePath string) filler.Kind {
	hay := strings.ToLower(name + " " + path.Dir(filePath))
	switch {
	case strings.Contains(hay, "bumper"):
		return filler.Bumper
	case strings.Contains(hay, "station") || strings.Contains(hay, "ident"):
		return filler.StationID
	case strings.Contains(hay, "psa"):
		return filler.PSA
	case strings.Contains(hay, "trailer"):
		return filler.Trailer
	default:
		return filler.Commercial
	}
}

// Client satisfies the filler-list read capability.
var _ FillerLister = (*Client)(nil)

// FillerLister is the filler-catalog read the sync depends on (§10). Abstracted so
// the sync can be tested with a fake.
type FillerLister interface {
	ListFillerClips(ctx context.Context, fillerLibraryID string) ([]FillerClip, error)
}

// eraFromName is a best-effort year extractor from a filename ("clip_1994_ad.mp4"
// → 1994), used as an initial era tag before AI refinement. Returns 0 if none.
func eraFromName(name string) int {
	for i := 0; i+4 <= len(name); i++ {
		if y, err := strconv.Atoi(name[i : i+4]); err == nil && y >= 1930 && y <= 2035 {
			return y
		}
	}
	return 0
}
