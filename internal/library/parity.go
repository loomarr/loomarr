package library

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library/inventory"
)

// ServerFile is one media file the media server lists, as the scanner's parity report (#2151
// step 14) compares it with what the scanner found on disk. A title with several versions is one
// ServerFile per version, each with its own path.
type ServerFile struct {
	ItemID string
	// Kind is "movie" or "episode".
	Kind string
	Name string
	// Path is the file as the media server sees it; the path map makes it Loomarr's.
	Path        string
	ExternalIDs []inventory.ExternalID
	// SeriesIDs are an episode's show's ids, SeriesItemID the show's media-server item id; Season
	// and Episode its numbering.
	SeriesIDs       []inventory.ExternalID
	SeriesItemID    string
	Season, Episode int
	DurationMillis  int64
}

// serverFilesPage is how many items one /Items request reads. Large libraries are tens of
// thousands of episodes, so the list is paged rather than read in one response.
const serverFilesPage = 500

const serverFilesFields = "ProviderIds,Path,RunTimeTicks,SeriesId,IndexNumber,ParentIndexNumber,MediaSources"

type serverFilesResponse struct {
	Items            []serverFileWire `json:"Items"`
	TotalRecordCount int              `json:"TotalRecordCount"`
}

type serverFileWire struct {
	ID                string            `json:"Id"`
	Type              string            `json:"Type"`
	Name              string            `json:"Name"`
	Path              string            `json:"Path"`
	RunTimeTicks      int64             `json:"RunTimeTicks"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	SeriesID          string            `json:"SeriesId"`
	IndexNumber       int               `json:"IndexNumber"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	MediaSources      []struct {
		Path         string `json:"Path"`
		RunTimeTicks int64  `json:"RunTimeTicks"`
	} `json:"MediaSources"`
}

// ServerFiles lists every movie and episode file the media server holds, read-only. It is the
// media server's side of the parity report, and goes with the catalog when the catalog retires.
func (c *Client) ServerFiles(ctx context.Context) ([]ServerFile, error) {
	op, err := c.operation()
	if err != nil {
		return nil, err
	}
	var items []serverFileWire
	for start := 0; ; start += serverFilesPage {
		page, err := op.serverFilesPage(ctx, start)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if len(page.Items) == 0 || start+len(page.Items) >= page.TotalRecordCount {
			break
		}
	}
	series := map[string][]inventory.ExternalID{}
	for _, item := range items {
		if strings.EqualFold(item.Type, "Series") {
			series[item.ID] = externalIDs(item.ProviderIDs)
		}
	}
	var out []ServerFile
	for _, item := range items {
		kind := strings.ToLower(item.Type)
		if kind != "movie" && kind != "episode" {
			continue
		}
		file := ServerFile{
			ItemID: item.ID, Kind: kind, Name: item.Name, ExternalIDs: externalIDs(item.ProviderIDs),
		}
		if kind == "episode" {
			file.SeriesIDs, file.SeriesItemID = series[item.SeriesID], item.SeriesID
			file.Season, file.Episode = item.ParentIndexNumber, item.IndexNumber
		}
		if len(item.MediaSources) == 0 {
			file.Path, file.DurationMillis = item.Path, item.RunTimeTicks/10_000
			out = append(out, file)
			continue
		}
		for _, source := range item.MediaSources {
			version := file
			version.Path, version.DurationMillis = source.Path, source.RunTimeTicks/10_000
			out = append(out, version)
		}
	}
	return out, nil
}

func (c *Client) serverFilesPage(ctx context.Context, start int) (serverFilesResponse, error) {
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Movie,Episode,Series")
	q.Set("Fields", serverFilesFields)
	q.Set("EnableUserData", "false")
	q.Set("EnableImages", "false")
	q.Set("SortBy", "SortName")
	q.Set("StartIndex", strconv.Itoa(start))
	q.Set("Limit", strconv.Itoa(serverFilesPage))
	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return serverFilesResponse{}, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)
	var page serverFilesResponse
	if err := c.do(req, &page); err != nil {
		return serverFilesResponse{}, fmt.Errorf("list server files: %w", err)
	}
	return page, nil
}

// externalIDs is a server item's provider ids, namespaces lowercased and blanks dropped.
func externalIDs(ids map[string]string) []inventory.ExternalID {
	out := make([]inventory.ExternalID, 0, len(ids))
	for namespace, value := range ids {
		if strings.TrimSpace(value) != "" {
			out = append(out, inventory.ExternalID{Namespace: strings.ToLower(namespace), Value: value})
		}
	}
	return out
}
