// Command filler-corpus-pages freezes an explicitly authored first-party-page
// lane and optional full inventory. It records evidence but grants no rights.
package main

import (
	"bytes"
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
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const seedSchemaVersion = 1

type seed struct {
	SchemaVersion int        `json:"schemaVersion"`
	Authority     string     `json:"authority"`
	Cases         []seedCase `json:"cases"`
}

type seedCase struct {
	ItemID           string   `json:"itemId"`
	Title            string   `json:"title"`
	RoleHints        []string `json:"roleHints"`
	ItemURL          string   `json:"itemUrl"`
	MediaURL         string   `json:"mediaUrl"`
	RightsAssertions []string `json:"rightsAssertions"`
	RequiredPageText []string `json:"requiredPageText"`
}

type options struct {
	inputPath, outputPath, cacheDir, userAgent, pageHost, mediaHost string
	snapshotAt                                                      time.Time
	maxRequests, maxItems                                           int
	maxResponseBytes, maxItemBytes, maxTotalBytes                   int64
	delay, maxWallTime                                              time.Duration
	http                                                            *http.Client
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-pages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("in", "", "authored first-party page seed JSON")
	output := flags.String("out", "", "bounded source lane JSON")
	inventoryOutput := flags.String("inventory-out", "", "optional strict full-corpus inventory JSON")
	cache := flags.String("cache-dir", "", "raw page and HEAD cache")
	userAgent := flags.String("user-agent", "", "descriptive User-Agent with contact")
	snapshotText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	pageHost := flags.String("page-host", "", "exact trusted first-party page host")
	mediaHost := flags.String("media-host", "", "exact trusted first-party media host")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 10, "hard candidate ceiling")
	maxResponseBytes := flags.Int64("max-response-bytes", 0, "hard aggregate response-byte ceiling")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard predicted item-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", 250*time.Millisecond, "minimum delay between requests")
	maxWallTime := flags.Duration("max-wall-time", time.Minute, "hard wall-clock ceiling")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *output == "" || *cache == "" || *userAgent == "" || *snapshotText == "" || *pageHost == "" || *mediaHost == "" || *maxItems <= 0 || *maxItems > 100 || *maxRequests < 2*(*maxItems) || *maxRequests > 1000 || *maxResponseBytes <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 0 || *maxWallTime <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: input, output, cache, identity, snapshot, exact hosts, 1-100 items, sufficient bounded requests, positive byte ceilings, non-negative delay, and wall ceiling are required")
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: parse snapshot:", err)
		return 2
	}
	opts := options{inputPath: *input, outputPath: *output, cacheDir: *cache, userAgent: *userAgent, pageHost: *pageHost, mediaHost: *mediaHost, snapshotAt: snapshotAt, maxRequests: *maxRequests, maxItems: *maxItems, maxResponseBytes: *maxResponseBytes, maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay, maxWallTime: *maxWallTime}
	draft, err := readSeed(opts.inputPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: seed:", err)
		return 1
	}
	if len(draft.Cases) != opts.maxItems {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-pages: seed has %d cases; --max-items is %d\n", len(draft.Cases), opts.maxItems)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.maxWallTime)
	defer cancel()
	lane, err := capture(ctx, draft, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pages:", err)
		return 1
	}
	var inventory *fillercorpus.Inventory
	if *inventoryOutput != "" {
		value, err := fillercorpus.InventoryFromLane(lane, fillercorpus.LaneInventoryOptions{SnapshotAt: opts.snapshotAt, Collection: filepath.Base(opts.inputPath), AllowedMediaHosts: []string{opts.mediaHost}})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: inventory:", err)
			return 1
		}
		inventory = &value
	}
	if err := writeJSON(opts.outputPath, lane); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: write:", err)
		return 1
	}
	if inventory != nil {
		if err := writeJSON(*inventoryOutput, inventory); err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-pages: write inventory:", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-pages: froze %d %s candidates (%d predicted bytes) in %d requests\n", len(lane.Cases), lane.Authority, lane.PredictedMediaBytes, lane.RequestsUsed)
	return 0
}

func readSeed(filename string) (seed, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return seed{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value seed
	if err := decoder.Decode(&value); err != nil {
		return seed{}, err
	}
	if value.SchemaVersion != seedSchemaVersion || value.Authority == "" || len(value.Cases) == 0 {
		return seed{}, fmt.Errorf("schema 1, authority, and at least one case are required")
	}
	return value, nil
}

func capture(ctx context.Context, draft seed, opts options) (fillercorpus.Lane, error) {
	started := time.Now()
	hosts := []string{opts.pageHost}
	if opts.mediaHost != opts.pageHost {
		hosts = append(hosts, opts.mediaHost)
	}
	httpClient := opts.http
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	client, err := fillercorpus.NewSourceClient(fillercorpus.SourceClientConfig{HTTP: httpClient, CacheDir: opts.cacheDir, UserAgent: opts.userAgent, MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, Delay: opts.delay, AllowedHosts: hosts})
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	lane := fillercorpus.Lane{Authority: draft.Authority, MaxRequests: opts.maxRequests, MaxResponseBytes: opts.maxResponseBytes, MaxPredictedMediaBytes: opts.maxTotalBytes, MaxWallTimeMS: opts.maxWallTime.Milliseconds()}
	seen := map[string]struct{}{}
	for _, candidate := range draft.Cases {
		if err := validateSeedCase(candidate, opts.pageHost, opts.mediaHost); err != nil {
			return fillercorpus.Lane{}, fmt.Errorf("seed case %q: %w", candidate.ItemID, err)
		}
		if _, exists := seen[candidate.ItemID]; exists {
			return fillercorpus.Lane{}, fmt.Errorf("duplicate seed item %q", candidate.ItemID)
		}
		seen[candidate.ItemID] = struct{}{}
		pageRaw, retrievedAt, err := client.Get(ctx, candidate.ItemURL)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		media, _ := url.Parse(candidate.MediaURL)
		for _, required := range append(append([]string(nil), candidate.RequiredPageText...), media.EscapedPath()) {
			if !bytes.Contains(pageRaw, []byte(required)) {
				return fillercorpus.Lane{}, fmt.Errorf("page %s omitted required frozen text %q", candidate.ItemURL, required)
			}
		}
		head, _, err := client.Head(ctx, candidate.MediaURL)
		if err != nil {
			return fillercorpus.Lane{}, err
		}
		mediaType, _, _ := mime.ParseMediaType(head.ContentType)
		if mediaType != "video/mp4" || head.ContentLength > opts.maxItemBytes || head.ContentLength > opts.maxTotalBytes-lane.PredictedMediaBytes {
			return fillercorpus.Lane{}, fmt.Errorf("media %s violates type or byte ceiling", candidate.MediaURL)
		}
		lane.Cases = append(lane.Cases, fillercorpus.Candidate{ItemID: candidate.ItemID, Title: candidate.Title, RoleHints: candidate.RoleHints, ItemURL: candidate.ItemURL, MetadataURL: candidate.ItemURL, MetadataRetrievedAt: retrievedAt, MetadataSHA256: sha256Hex(pageRaw), RightsAssertions: candidate.RightsAssertions, Representation: fillercorpus.Representation{Name: path.Base(media.Path), URL: candidate.MediaURL, MIMEType: mediaType, Bytes: head.ContentLength}})
		lane.PredictedMediaBytes += head.ContentLength
	}
	lane.RequestsUsed, lane.ResponseBytes, lane.WallTimeMS = client.RequestsUsed(), client.ResponseBytes(), time.Since(started).Milliseconds()
	return lane, nil
}

func validateSeedCase(candidate seedCase, pageHost, mediaHost string) error {
	if strings.TrimSpace(candidate.ItemID) == "" || strings.TrimSpace(candidate.Title) == "" || len(candidate.RoleHints) == 0 || len(candidate.RightsAssertions) == 0 || len(candidate.RequiredPageText) == 0 {
		return fmt.Errorf("identity, role hint, source assertion, and page evidence are required")
	}
	if !exactHTTPSHost(candidate.ItemURL, pageHost) || !exactHTTPSHost(candidate.MediaURL, mediaHost) {
		return fmt.Errorf("page and media URLs require HTTPS and their exact declared hosts")
	}
	return nil
}

func exactHTTPSHost(raw, host string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Hostname() == host
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
	temp, err := os.CreateTemp(filepath.Dir(filename), ".filler-corpus-pages-*")
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
