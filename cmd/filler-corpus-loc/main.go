// Command filler-corpus-loc freezes a bounded, metadata-only Library of
// Congress lane and optional full inventory. It grants no rights authority.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const defaultBaseURL = "https://www.loc.gov"

type searchResponse struct {
	Pagination struct {
		Of int `json:"of"`
	} `json:"pagination"`
	Results []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"results"`
}

type itemResponse struct {
	Item struct {
		ID               string   `json:"id"`
		Title            string   `json:"title"`
		Rights           []string `json:"rights"`
		AccessRestricted bool     `json:"access_restricted"`
	} `json:"item"`
	Resources []struct {
		DownloadRestricted bool `json:"download_restricted"`
		Files              [][]struct {
			MIMEType string `json:"mimetype"`
			URL      string `json:"url"`
			Size     int64  `json:"size"`
		} `json:"files"`
	} `json:"resources"`
}

type options struct {
	baseURL, query, roleHint, outputPath, cacheDir, userAgent string
	snapshotAt                                                time.Time
	maxRequests, maxItems                                     int
	maxResponseBytes, maxItemBytes, maxTotalBytes             int64
	delay, maxWallTime                                        time.Duration
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-loc", flag.ContinueOnError)
	flags.SetOutput(stderr)
	query := flags.String("query", "", "LOC collection search text")
	roleHint := flags.String("role-hint", "", "discovery-only role hint")
	output := flags.String("out", "", "bounded source lane JSON")
	inventoryOutput := flags.String("inventory-out", "", "optional strict full-corpus inventory JSON")
	cache := flags.String("cache-dir", "", "raw response cache")
	userAgent := flags.String("user-agent", "", "descriptive User-Agent with contact")
	snapshotText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 10, "hard candidate ceiling")
	maxResponseBytes := flags.Int64("max-response-bytes", 0, "hard aggregate response-byte ceiling")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard predicted item-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", 3100*time.Millisecond, "minimum delay between requests")
	maxWallTime := flags.Duration("max-wall-time", time.Minute, "hard wall-clock ceiling")
	baseURL := flags.String("base-url", defaultBaseURL, "LOC API base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *query == "" || *roleHint == "" || *output == "" || *cache == "" || *userAgent == "" || *snapshotText == "" || *maxItems <= 0 || *maxItems > 50 || *maxRequests < *maxItems+1 || *maxRequests > 1000 || *maxResponseBytes <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 3*time.Second || *maxWallTime <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-loc: query, role, output, cache, identity, snapshot, 1-50 items, sufficient bounded requests, positive byte ceilings, >=3s delay, and wall ceiling are required")
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-loc: parse snapshot:", err)
		return 2
	}
	opts := options{baseURL: strings.TrimRight(*baseURL, "/"), query: *query, roleHint: *roleHint, outputPath: *output, cacheDir: *cache, userAgent: *userAgent, snapshotAt: snapshotAt, maxRequests: *maxRequests, maxItems: *maxItems, maxResponseBytes: *maxResponseBytes, maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay, maxWallTime: *maxWallTime}
	ctx, cancel := context.WithTimeout(context.Background(), opts.maxWallTime)
	defer cancel()
	lane, err := capture(ctx, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-loc:", err)
		return 1
	}
	var inventory *fillercorpus.Inventory
	if *inventoryOutput != "" {
		value, err := fillercorpus.InventoryFromLane(lane, fillercorpus.LaneInventoryOptions{SnapshotAt: opts.snapshotAt, Collection: opts.query, AllowedMediaHosts: []string{"tile.loc.gov"}})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-loc: inventory:", err)
			return 1
		}
		inventory = &value
	}
	if err := writeJSON(opts.outputPath, lane); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-loc: write:", err)
		return 1
	}
	if inventory != nil {
		if err := writeJSON(*inventoryOutput, inventory); err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-loc: write inventory:", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-loc: froze %d candidates (%d predicted bytes) in %d requests\n", len(lane.Cases), lane.PredictedMediaBytes, lane.RequestsUsed)
	return 0
}

func capture(ctx context.Context, opts options) (fillercorpus.Lane, error) {
	started := time.Now()
	if err := os.MkdirAll(opts.cacheDir, 0o750); err != nil {
		return fillercorpus.Lane{}, err
	}
	base, err := url.Parse(opts.baseURL)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	f, err := fillercorpus.NewSourceClient(fillercorpus.SourceClientConfig{HTTP: &http.Client{Timeout: 30 * time.Second}, CacheDir: opts.cacheDir, UserAgent: opts.userAgent, MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, Delay: opts.delay, AllowedHosts: []string{base.Hostname()}})
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	searchURL, err := locSearchURL(opts.baseURL, opts.query)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	raw, _, err := f.Get(ctx, searchURL)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	var search searchResponse
	if err := json.Unmarshal(raw, &search); err != nil {
		return fillercorpus.Lane{}, fmt.Errorf("decode search: %w", err)
	}
	if search.Pagination.Of > len(search.Results) {
		return fillercorpus.Lane{}, fmt.Errorf("search returned %d of %d results", len(search.Results), search.Pagination.Of)
	}
	ids := make([]string, 0, len(search.Results))
	for _, result := range search.Results {
		if id := locItemID(result.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return sha256Hex(opts.query+"/"+ids[i]) < sha256Hex(opts.query+"/"+ids[j]) })
	lane := fillercorpus.Lane{Authority: "loc.gov/national-screening-room", MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, MaxPredictedMediaBytes: opts.maxTotalBytes, MaxWallTimeMS: opts.maxWallTime.Milliseconds()}
	for _, id := range ids {
		if len(lane.Cases) == opts.maxItems {
			break
		}
		itemURL := opts.baseURL + "/item/" + url.PathEscape(id) + "/"
		raw, retrievedAt, err := f.Get(ctx, itemURL+"?fo=json")
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		var item itemResponse
		if err := json.Unmarshal(raw, &item); err != nil {
			return fillercorpus.Lane{}, fmt.Errorf("decode item %s: %w", id, err)
		}
		media, ok := selectMP4(item, opts.maxItemBytes, opts.maxTotalBytes-lane.PredictedMediaBytes)
		if !ok || item.Item.AccessRestricted || len(item.Item.Rights) == 0 {
			continue
		}
		lane.Cases = append(lane.Cases, fillercorpus.Candidate{ItemID: id, Title: item.Item.Title, RoleHints: []string{opts.roleHint}, ItemURL: itemURL, MetadataURL: itemURL + "?fo=json", MetadataRetrievedAt: retrievedAt, MetadataSHA256: sha256Hex(string(raw)), RightsAssertions: item.Item.Rights, Representation: fillercorpus.Representation{Name: filepath.Base(media.URL), URL: media.URL, MIMEType: media.MIMEType, Bytes: media.Size}})
		lane.PredictedMediaBytes += media.Size
	}
	lane.RequestsUsed, lane.ResponseBytes, lane.WallTimeMS = f.RequestsUsed(), f.ResponseBytes(), time.Since(started).Milliseconds()
	if len(lane.Cases) != opts.maxItems {
		return fillercorpus.Lane{}, fmt.Errorf("captured %d of %d candidates", len(lane.Cases), opts.maxItems)
	}
	return lane, nil
}

type mediaFile struct {
	MIMEType, URL string
	Size          int64
}

func selectMP4(item itemResponse, maxItem, remaining int64) (mediaFile, bool) {
	for _, resource := range item.Resources {
		if resource.DownloadRestricted {
			continue
		}
		for _, group := range resource.Files {
			for _, file := range group {
				if file.MIMEType == "video/mp4" && file.Size > 0 && file.Size <= maxItem && file.Size <= remaining && strings.HasPrefix(file.URL, "https://tile.loc.gov/") {
					return mediaFile{file.MIMEType, file.URL, file.Size}, true
				}
			}
		}
	}
	return mediaFile{}, false
}

func locSearchURL(base, query string) (string, error) {
	u, err := url.Parse(base + "/collections/national-screening-room/")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("fo", "json")
	q.Set("at", "results,pagination")
	q.Set("c", "50")
	q.Set("sp", "1")
	q.Set("q", query)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func locItemID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "www.loc.gov" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "item" || parts[1] == "" {
		return ""
	}
	return parts[1]
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-loc-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
