// Command filler-corpus-nasa freezes a bounded, metadata-only NASA pilot lane.
// It grants no rights or download authority.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const (
	defaultSearchBase = "https://images-api.nasa.gov"
	defaultAssetHost  = "images-assets.nasa.gov"
)

type searchResponse struct {
	Collection struct {
		Metadata struct {
			TotalHits int `json:"total_hits"`
		} `json:"metadata"`
		Items []json.RawMessage `json:"items"`
	} `json:"collection"`
}

type searchItem struct {
	Href string `json:"href"`
	Data []struct {
		Center           string `json:"center"`
		MediaType        string `json:"media_type"`
		NASAID           string `json:"nasa_id"`
		Title            string `json:"title"`
		Photographer     string `json:"photographer"`
		SecondaryCreator string `json:"secondary_creator"`
	} `json:"data"`
}

type candidateItem struct {
	raw  json.RawMessage
	item searchItem
}

type options struct {
	searchBase, assetHost, query, roleHint, outputPath, cacheDir, userAgent string
	snapshotAt                                                              time.Time
	maxRequests, maxItems                                                   int
	maxResponseBytes, maxItemBytes, maxTotalBytes                           int64
	delay, maxWallTime                                                      time.Duration
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-nasa", flag.ContinueOnError)
	flags.SetOutput(stderr)
	query := flags.String("query", "", "NASA image-library search text")
	roleHint := flags.String("role-hint", "", "discovery-only role hint")
	output := flags.String("out", "", "source-neutral pilot lane JSON")
	cache := flags.String("cache-dir", "", "raw response and HEAD cache")
	userAgent := flags.String("user-agent", "", "descriptive User-Agent with contact")
	snapshotText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 10, "hard candidate ceiling")
	maxResponseBytes := flags.Int64("max-response-bytes", 0, "hard aggregate response-byte ceiling")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard predicted item-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", 250*time.Millisecond, "minimum delay between requests")
	maxWallTime := flags.Duration("max-wall-time", time.Minute, "hard wall-clock ceiling")
	searchBase := flags.String("search-base", defaultSearchBase, "NASA image-library API base URL")
	assetHost := flags.String("asset-host", defaultAssetHost, "exact trusted NASA asset host")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *query == "" || *roleHint == "" || *output == "" || *cache == "" || *userAgent == "" || *snapshotText == "" || *assetHost == "" || *maxRequests < 21 || *maxRequests > 100 || *maxItems != 10 || *maxResponseBytes <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 0 || *maxWallTime <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-nasa: query, role, output, cache, identity, snapshot, exact asset host, exactly 10 items, 21-100 bounded requests, positive byte ceilings, non-negative delay, and wall ceiling are required")
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-nasa: parse snapshot:", err)
		return 2
	}
	opts := options{searchBase: strings.TrimRight(*searchBase, "/"), assetHost: *assetHost, query: *query, roleHint: *roleHint, outputPath: *output, cacheDir: *cache, userAgent: *userAgent, snapshotAt: snapshotAt, maxRequests: *maxRequests, maxItems: *maxItems, maxResponseBytes: *maxResponseBytes, maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay, maxWallTime: *maxWallTime}
	ctx, cancel := context.WithTimeout(context.Background(), opts.maxWallTime)
	defer cancel()
	lane, err := capture(ctx, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-nasa:", err)
		return 1
	}
	if err := writeJSON(opts.outputPath, lane); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-nasa: write:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-nasa: froze %d candidates (%d predicted bytes) in %d requests\n", len(lane.Cases), lane.PredictedMediaBytes, lane.RequestsUsed)
	return 0
}

func capture(ctx context.Context, opts options) (fillercorpus.Lane, error) {
	started := time.Now()
	client, err := fillercorpus.NewSourceClient(fillercorpus.SourceClientConfig{HTTP: &http.Client{Timeout: 30 * time.Second}, CacheDir: opts.cacheDir, UserAgent: opts.userAgent, MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, Delay: opts.delay})
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	searchURL, err := nasaSearchURL(opts.searchBase, opts.query)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	raw, _, err := client.Get(ctx, searchURL)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	items, err := decodeSearch(raw)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	lane := fillercorpus.Lane{Authority: "images.nasa.gov", MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, MaxPredictedMediaBytes: opts.maxTotalBytes, MaxWallTimeMS: opts.maxWallTime.Milliseconds()}
	for _, candidate := range items {
		if len(lane.Cases) == opts.maxItems {
			break
		}
		data := candidate.item.Data[0]
		manifestURL, ok := trustedURL(candidate.item.Href, opts.assetHost)
		if !ok {
			continue
		}
		manifestRaw, retrievedAt, err := client.Get(ctx, manifestURL)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		mediaURL, err := selectMP4(manifestRaw, opts.assetHost)
		if err != nil {
			continue
		}
		head, _, err := client.Head(ctx, mediaURL)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		mediaType, _, _ := mime.ParseMediaType(head.ContentType)
		if mediaType != "video/mp4" || head.ContentLength > opts.maxItemBytes || head.ContentLength > opts.maxTotalBytes-lane.PredictedMediaBytes {
			continue
		}
		assertions := []string{"NASA center assertion: " + data.Center, "NASA media usage guidelines require item-level review for third-party copyright, identifiable people, logos, music, and endorsement restrictions."}
		if creator := joinedNonEmpty(data.Photographer, data.SecondaryCreator); creator != "" {
			assertions = append(assertions, "NASA creator assertion: "+creator)
		}
		digestInput := append(append(append([]byte(nil), candidate.raw...), '\n'), manifestRaw...)
		lane.Cases = append(lane.Cases, fillercorpus.Candidate{ItemID: data.NASAID, Title: data.Title, RoleHints: []string{opts.roleHint}, ItemURL: "https://images.nasa.gov/details/" + url.PathEscape(data.NASAID), MetadataURL: manifestURL, MetadataRetrievedAt: retrievedAt, MetadataSHA256: sha256Hex(digestInput), RightsAssertions: assertions, Representation: fillercorpus.Representation{Name: representationName(mediaURL), URL: mediaURL, MIMEType: mediaType, Bytes: head.ContentLength}})
		lane.PredictedMediaBytes += head.ContentLength
	}
	lane.RequestsUsed, lane.ResponseBytes, lane.WallTimeMS = client.RequestsUsed(), client.ResponseBytes(), time.Since(started).Milliseconds()
	if len(lane.Cases) != opts.maxItems {
		return fillercorpus.Lane{}, fmt.Errorf("captured %d of %d candidates", len(lane.Cases), opts.maxItems)
	}
	return lane, nil
}

func decodeSearch(raw []byte) ([]candidateItem, error) {
	var search searchResponse
	if err := json.Unmarshal(raw, &search); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}
	if search.Collection.Metadata.TotalHits > len(search.Collection.Items) {
		return nil, fmt.Errorf("search returned %d of %d results", len(search.Collection.Items), search.Collection.Metadata.TotalHits)
	}
	items := make([]candidateItem, 0, len(search.Collection.Items))
	for _, rawItem := range search.Collection.Items {
		var item searchItem
		if err := json.Unmarshal(rawItem, &item); err != nil || len(item.Data) != 1 || item.Data[0].MediaType != "video" || item.Data[0].NASAID == "" || item.Data[0].Title == "" || item.Data[0].Center == "" {
			continue
		}
		items = append(items, candidateItem{raw: rawItem, item: item})
	}
	sort.Slice(items, func(i, j int) bool {
		return sha256Hex([]byte(items[i].item.Data[0].NASAID)) < sha256Hex([]byte(items[j].item.Data[0].NASAID))
	})
	return items, nil
}

func nasaSearchURL(base, query string) (string, error) {
	u, err := url.Parse(base + "/search")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("media_type", "video")
	q.Set("page_size", "50")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func selectMP4(raw []byte, assetHost string) (string, error) {
	var assets []string
	if err := json.Unmarshal(raw, &assets); err != nil {
		return "", fmt.Errorf("decode asset manifest: %w", err)
	}
	for _, suffix := range []string{"~medium.mp4", "~mobile.mp4", "~large.mp4", "~orig.mp4"} {
		for _, asset := range assets {
			if normalized, ok := trustedURL(asset, assetHost); ok && strings.HasSuffix(strings.ToLower(normalized), suffix) {
				return normalized, nil
			}
		}
	}
	return "", fmt.Errorf("manifest has no trusted MP4 derivative")
}

func trustedURL(raw, host string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() != host || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	u.Scheme = "https"
	return u.String(), true
}

func joinedNonEmpty(values ...string) string {
	var kept []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			kept = append(kept, value)
		}
	}
	return strings.Join(kept, "; ")
}

func representationName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Base(rawURL)
	}
	return path.Base(u.Path)
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
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
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-nasa-*")
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
