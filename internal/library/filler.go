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
	// Path is the file's location AS THE MEDIA SERVER SEES IT (§10 V38c).
	//
	// ⚠ **It is only useful when Loomarr and the media server share storage** — the common
	// homelab case (one box, or the same NFS mount), but not a guarantee. Loomarr checks whether
	// the path is readable before using it and reports honestly when it is not; it must never
	// assume this resolves locally.
	//
	// It was already being requested (`Fields=Path`) and used for the kind heuristic, then
	// discarded. V38c needs it because a library is now an ACQUISITION source: the clip has to
	// reach the clip folder as bytes, and a media-server item id is not something ffmpeg can open.
	Path string
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

// ItemDurationMs returns a single library item's runtime in milliseconds,
// resolved from the media server's server-probed RunTimeTicks (§9/§10 — the
// core never probes media itself). Used by the scheduler to give a program slot
// a real duration before pushing the lineup to Tunarr (which rejects ≤ 0).
// Returns (0, nil) if the server reports no runtime.
//
// Uses the LIST endpoint `GET /Items?Ids=<id>&Fields=RunTimeTicks` rather than
// `GET /Items/<id>`: Emby 4.10 rejects the bare single-item path with "could not
// be found" unless it's user-scoped (`/Users/<uid>/Items/<id>`), and Loomarr
// authenticates with a token, not a user context. The Ids-filtered list needs no
// user and is shared across the Emby/Jellyfin flavors. (Caught by the live smoke:
// the bare path 404'd → duration 0 → Tunarr rejected the lineup push.)
func (c *Client) ItemDurationMs(ctx context.Context, itemID string) (int64, error) {
	c, err := c.operation()
	if err != nil {
		return 0, err
	}
	q := url.Values{}
	q.Set("Ids", itemID)
	q.Set("Fields", "RunTimeTicks")
	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var resp fillerItemsResponse
	if err := c.do(req, &resp); err != nil {
		return 0, err
	}
	if len(resp.Items) == 0 {
		return 0, nil // item not found / no runtime — caller falls back (never dead air)
	}
	return resp.Items[0].RunTimeTicks / ticksPerMs, nil
}

// virtualFolder is one library as `GET /Library/VirtualFolders` reports it.
//
// ⚠ **That endpoint returns a BARE ARRAY**, not the `{"Items": […]}` envelope every other
// endpoint in this package uses. Captured from Emby 4.10.0.22 on 2026-08-02 rather than written
// from memory — see fixtures/emby/FINDINGS.md, which is also where the `CollectionType` finding
// below is recorded.
type virtualFolder struct {
	Name string `json:"Name"`
	// ItemId is what `ParentId` accepts. ⚠ `Guid` is a DIFFERENT identifier on the same object
	// and is NOT accepted by ParentId — picking it would 200 with an empty item list, which reads
	// as "your filler library is empty" rather than as a bug.
	ItemID string `json:"ItemId"`
}

// LibraryIDByName resolves a library's display NAME to the item id `ParentId` needs (§10 V38c).
//
// The operator types "Commercials" because that is what their media server shows them; making
// them hunt for an item id would be asking them to do a lookup Loomarr can do itself.
//
// ⚠ Returns ("", nil) for a name the server does not have — "found nothing", not a failure. An
// operator can rename or delete a library at any time, and that must degrade to a source that
// scans zero clips rather than to an error that stops the other sources from being scanned.
//
// ⚠ **Matched on NAME ALONE, never filtered by `CollectionType`.** Three of the seven libraries
// in the 2026-08-02 capture omit that key entirely — it is absent for mixed/unclassified
// libraries, which is exactly what a hand-made commercials library usually is. Filtering on it
// would hide the libraries an operator is most likely to point at.
func (c *Client) LibraryIDByName(ctx context.Context, name string) (string, error) {
	c, err := c.operation()
	if err != nil {
		return "", err
	}
	want := strings.TrimSpace(name)
	if want == "" {
		return "", nil
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/Library/VirtualFolders", nil)
	if err != nil {
		return "", err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var folders []virtualFolder
	if err := c.do(req, &folders); err != nil {
		return "", err
	}
	for _, f := range folders {
		// Case-insensitive: the operator is retyping a name they read off a screen, and
		// "commercials" failing where "Commercials" works is a puzzle with no useful diagnosis.
		if strings.EqualFold(strings.TrimSpace(f.Name), want) {
			return f.ItemID, nil
		}
	}
	return "", nil
}

// ListFillerClips reads every item in the media server's filler library (§10):
//
//	GET /Items?Recursive=true&ParentId=<fillerLibraryID>&IncludeItemTypes=Video
//	           &Fields=Path&Limit=<n>
//
// Duration comes from RunTimeTicks. fillerLibraryID is the library's item id
// (FILLER_LIBRARY, §15); the caller resolves a name→id if configured by name.
func (c *Client) ListFillerClips(ctx context.Context, fillerLibraryID string) ([]FillerClip, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
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
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

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
			Path:          it.Path,
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
