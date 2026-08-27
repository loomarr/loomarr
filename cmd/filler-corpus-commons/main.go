// Command filler-corpus-commons freezes a bounded, metadata-only Wikimedia
// Commons lane and optional full inventory. It grants no rights authority.
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
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const (
	defaultAPIBase    = "https://commons.wikimedia.org/w/api.php"
	defaultUploadHost = "upload.wikimedia.org"
)

type apiError struct {
	Code string `json:"code"`
	Info string `json:"info"`
}

type categoryResponse struct {
	Error    *apiError `json:"error"`
	Continue struct {
		Category string `json:"gcmcontinue"`
	} `json:"continue"`
	Query struct {
		Pages []commonsPage `json:"pages"`
	} `json:"query"`
}

type commonsPage struct {
	PageID    int64       `json:"pageid"`
	Title     string      `json:"title"`
	ImageInfo []imageInfo `json:"imageinfo"`
}

type imageInfo struct {
	Timestamp      time.Time                `json:"timestamp"`
	User           string                   `json:"user"`
	Size           int64                    `json:"size"`
	URL            string                   `json:"url"`
	DescriptionURL string                   `json:"descriptionurl"`
	SHA1           string                   `json:"sha1"`
	MIME           string                   `json:"mime"`
	MediaType      string                   `json:"mediatype"`
	ExtMetadata    map[string]metadataValue `json:"extmetadata"`
}

type metadataValue struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

type mediaInfoResponse struct {
	Error    *apiError                  `json:"error"`
	Entities map[string]json.RawMessage `json:"entities"`
}

type mediaEntity struct {
	PageID     int64                      `json:"pageid"`
	LastRevID  int64                      `json:"lastrevid"`
	Modified   time.Time                  `json:"modified"`
	Statements map[string]json.RawMessage `json:"statements"`
}

type pageEvidence struct {
	page commonsPage
	raw  []byte
}

type options struct {
	apiBase, apiHost, uploadHost, category, roleHint, outputPath, cacheDir, userAgent string
	snapshotAt                                                                        time.Time
	maxRequests, maxPages, maxItems                                                   int
	maxResponseBytes, maxItemBytes, maxTotalBytes                                     int64
	delay, maxWallTime                                                                time.Duration
	http                                                                              *http.Client
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-commons", flag.ContinueOnError)
	flags.SetOutput(stderr)
	category := flags.String("category", "", "exact Commons category name without prefix")
	roleHint := flags.String("role-hint", "", "discovery-only role hint")
	output := flags.String("out", "", "bounded source lane JSON")
	inventoryOutput := flags.String("inventory-out", "", "optional strict full-corpus inventory JSON")
	cache := flags.String("cache-dir", "", "raw API response cache")
	userAgent := flags.String("user-agent", "", "descriptive User-Agent with contact")
	snapshotText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxPages := flags.Int("max-pages", 0, "hard category continuation ceiling")
	maxItems := flags.Int("max-items", 10, "hard candidate ceiling")
	maxResponseBytes := flags.Int64("max-response-bytes", 0, "hard aggregate response-byte ceiling")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard predicted item-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", 250*time.Millisecond, "minimum delay between requests")
	maxWallTime := flags.Duration("max-wall-time", time.Minute, "hard wall-clock ceiling")
	apiBase := flags.String("api-base", defaultAPIBase, "Commons API endpoint")
	uploadHost := flags.String("upload-host", defaultUploadHost, "exact trusted Commons media host")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	apiURL, err := url.Parse(*apiBase)
	if err != nil || apiURL.Scheme != "https" || apiURL.Hostname() == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: API endpoint must use HTTPS")
		return 2
	}
	if *category == "" || *roleHint == "" || *output == "" || *cache == "" || *userAgent == "" || *snapshotText == "" || *uploadHost == "" || *maxRequests < 2 || *maxRequests > 1000 || *maxPages <= 0 || *maxPages > 100 || *maxItems <= 0 || *maxItems > 50 || *maxResponseBytes <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 100*time.Millisecond || *maxWallTime <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: category, role, output, cache, identity, snapshot, exact hosts, 1-50 items, bounded requests/pages, positive byte ceilings, >=100ms delay, and wall ceiling are required")
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: parse snapshot:", err)
		return 2
	}
	opts := options{apiBase: apiURL.String(), apiHost: apiURL.Hostname(), uploadHost: *uploadHost, category: *category, roleHint: *roleHint, outputPath: *output, cacheDir: *cache, userAgent: *userAgent, snapshotAt: snapshotAt, maxRequests: *maxRequests, maxPages: *maxPages, maxItems: *maxItems, maxResponseBytes: *maxResponseBytes, maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay, maxWallTime: *maxWallTime}
	ctx, cancel := context.WithTimeout(context.Background(), opts.maxWallTime)
	defer cancel()
	lane, err := capture(ctx, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-commons:", err)
		return 1
	}
	var inventory *fillercorpus.Inventory
	if *inventoryOutput != "" {
		value, err := fillercorpus.InventoryFromLane(lane, fillercorpus.LaneInventoryOptions{SnapshotAt: opts.snapshotAt, Collection: "Category:" + opts.category, AllowedMediaHosts: []string{opts.uploadHost}})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: inventory:", err)
			return 1
		}
		inventory = &value
	}
	if err := writeJSON(opts.outputPath, lane); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: write:", err)
		return 1
	}
	if inventory != nil {
		if err := writeJSON(*inventoryOutput, inventory); err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-commons: write inventory:", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-commons: froze %d candidates (%d predicted bytes) in %d requests\n", len(lane.Cases), lane.PredictedMediaBytes, lane.RequestsUsed)
	return 0
}

func capture(ctx context.Context, opts options) (fillercorpus.Lane, error) {
	started := time.Now()
	httpClient := opts.http
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client, err := fillercorpus.NewSourceClient(fillercorpus.SourceClientConfig{HTTP: httpClient, CacheDir: opts.cacheDir, UserAgent: opts.userAgent, MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, Delay: opts.delay, AllowedHosts: []string{opts.apiHost}})
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	var evidence []pageEvidence
	continuation := ""
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber == opts.maxPages {
			return fillercorpus.Lane{}, fmt.Errorf("category continuation exceeded %d pages", opts.maxPages)
		}
		requestURL, err := categoryURL(opts.apiBase, opts.category, continuation)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		raw, _, err := client.Get(ctx, requestURL)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		var response categoryResponse
		if err := json.Unmarshal(raw, &response); err != nil {
			return fillercorpus.Lane{}, fmt.Errorf("decode category page: %w", err)
		}
		if response.Error != nil {
			return fillercorpus.Lane{}, fmt.Errorf("commons API %s: %s", response.Error.Code, response.Error.Info)
		}
		for _, page := range response.Query.Pages {
			if len(page.ImageInfo) != 1 || page.ImageInfo[0].MediaType != "VIDEO" {
				continue
			}
			pageRaw, err := json.Marshal(page)
			if err != nil {
				return fillercorpus.Lane{}, err
			}
			evidence = append(evidence, pageEvidence{page: page, raw: pageRaw})
		}
		continuation = response.Continue.Category
		if continuation == "" {
			break
		}
	}
	sort.Slice(evidence, func(i, j int) bool {
		return sha256Hex([]byte(opts.category+"/"+strconv.FormatInt(evidence[i].page.PageID, 10))) < sha256Hex([]byte(opts.category+"/"+strconv.FormatInt(evidence[j].page.PageID, 10)))
	})
	selected := make([]pageEvidence, 0, opts.maxItems)
	var predicted int64
	for _, item := range evidence {
		info := item.page.ImageInfo[0]
		if info.Size <= 0 || info.Size > opts.maxItemBytes || info.Size > opts.maxTotalBytes-predicted || !exactHTTPSHost(info.URL, opts.uploadHost) || !exactHTTPSHost(info.DescriptionURL, opts.apiHost) || !digest(info.SHA1, 40) || !strings.HasPrefix(info.MIME, "video/") {
			continue
		}
		selected = append(selected, item)
		predicted += info.Size
		if len(selected) == opts.maxItems {
			break
		}
	}
	if len(selected) != opts.maxItems {
		return fillercorpus.Lane{}, fmt.Errorf("selected %d of %d bounded video candidates", len(selected), opts.maxItems)
	}
	entityURL, err := mediaInfoURL(opts.apiBase, selected)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	entityRaw, retrievedAt, err := client.Get(ctx, entityURL)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	var media mediaInfoResponse
	if err := json.Unmarshal(entityRaw, &media); err != nil {
		return fillercorpus.Lane{}, fmt.Errorf("decode MediaInfo: %w", err)
	}
	if media.Error != nil {
		return fillercorpus.Lane{}, fmt.Errorf("commons MediaInfo %s: %s", media.Error.Code, media.Error.Info)
	}
	lane := fillercorpus.Lane{Authority: "commons.wikimedia.org", MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, MaxPredictedMediaBytes: opts.maxTotalBytes, PredictedMediaBytes: predicted, MaxWallTimeMS: opts.maxWallTime.Milliseconds()}
	for _, item := range selected {
		entityRaw, ok := media.Entities["M"+strconv.FormatInt(item.page.PageID, 10)]
		if !ok {
			return fillercorpus.Lane{}, fmt.Errorf("MediaInfo omitted M%d", item.page.PageID)
		}
		var entity mediaEntity
		if err := json.Unmarshal(entityRaw, &entity); err != nil || entity.PageID != item.page.PageID || entity.LastRevID <= 0 || entity.Modified.IsZero() {
			return fillercorpus.Lane{}, fmt.Errorf("MediaInfo identity is incomplete for M%d", item.page.PageID)
		}
		info := item.page.ImageInfo[0]
		assertions := rightsAssertions(info.ExtMetadata, entity.Statements)
		licenseURL, err := commonsLicenseURL(info.ExtMetadata)
		if err != nil {
			return fillercorpus.Lane{}, fmt.Errorf("MediaInfo licence for M%d: %w", item.page.PageID, err)
		}
		digestInput := append(append(append([]byte(nil), item.raw...), '\n'), entityRaw...)
		lane.Cases = append(lane.Cases, fillercorpus.Candidate{ItemID: strconv.FormatInt(item.page.PageID, 10), Title: strings.TrimPrefix(item.page.Title, "File:"), RoleHints: []string{opts.roleHint}, DiscoveryPath: []string{"Category:" + opts.category}, ItemURL: info.DescriptionURL, MetadataURL: entityURL, MetadataRetrievedAt: retrievedAt, MetadataSHA256: sha256Hex(digestInput), RightsAssertions: assertions, LicenseURL: licenseURL, Representation: fillercorpus.Representation{Name: path.Base(mustURL(info.URL).Path), URL: info.URL, MIMEType: info.MIME, Bytes: info.Size, SHA1: info.SHA1}})
	}
	lane.RequestsUsed, lane.ResponseBytes, lane.WallTimeMS = client.RequestsUsed(), client.ResponseBytes(), time.Since(started).Milliseconds()
	return lane, nil
}

func commonsLicenseURL(metadata map[string]metadataValue) (string, error) {
	value := strings.TrimSpace(metadata["LicenseUrl"].Value)
	if value == "" {
		return "", nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("LicenseUrl is not an absolute HTTPS URL")
	}
	return u.String(), nil
}

func categoryURL(base, category, continuation string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("action", "query")
	q.Set("format", "json")
	q.Set("formatversion", "2")
	q.Set("generator", "categorymembers")
	q.Set("gcmtitle", "Category:"+category)
	q.Set("gcmtype", "file")
	q.Set("gcmlimit", "50")
	q.Set("prop", "imageinfo")
	q.Set("iiprop", "url|size|sha1|mime|mediatype|timestamp|user|extmetadata")
	q.Set("iiextmetadatafilter", "LicenseShortName|LicenseUrl|UsageTerms|Credit|Artist|Copyrighted|Restrictions|Categories")
	q.Set("maxlag", "5")
	if continuation != "" {
		q.Set("gcmcontinue", continuation)
		q.Set("continue", "gcmcontinue||")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func mediaInfoURL(base string, selected []pageEvidence) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(selected))
	for _, item := range selected {
		ids = append(ids, "M"+strconv.FormatInt(item.page.PageID, 10))
	}
	q := u.Query()
	q.Set("action", "wbgetentities")
	q.Set("format", "json")
	q.Set("formatversion", "2")
	q.Set("ids", strings.Join(ids, "|"))
	q.Set("props", "claims|info")
	q.Set("maxlag", "5")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func rightsAssertions(metadata map[string]metadataValue, statements map[string]json.RawMessage) []string {
	assertions := make([]string, 0, 12)
	for _, field := range []string{"LicenseShortName", "LicenseUrl", "UsageTerms", "Credit", "Artist", "Copyrighted", "Restrictions", "Categories"} {
		if value, ok := metadata[field]; ok {
			assertions = append(assertions, fmt.Sprintf("Commons extmetadata %s (%s): %s", field, value.Source, value.Value))
		}
	}
	for _, property := range []string{"P275", "P6216", "P7482", "P170"} {
		value := statements[property]
		if len(value) == 0 {
			value = json.RawMessage("null")
		}
		assertions = append(assertions, "Commons MediaInfo "+property+": "+string(value))
	}
	return assertions
}

func exactHTTPSHost(raw, host string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Hostname() == host
}

func digest(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func writeJSON(filename string, value any) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temp, err := os.CreateTemp(filepath.Dir(filename), ".filler-corpus-commons-*")
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
	if err := os.Rename(name, filename); err != nil {
		return err
	}
	ok = true
	return nil
}
