// Command filler-corpus-dvids freezes a bounded DVIDS metadata inventory
// before any corpus media is downloaded or reviewed.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL  = "https://api.dvidshub.net"
	defaultSiteBaseURL = "https://www.dvidshub.net"
	maxSearchBody      = 8 << 20
	maxDetailBody      = 4 << 20
	maxPageBody        = 4 << 20
)

var allowedVideoCategories = map[string]struct{}{
	"B-Roll": {}, "Briefings": {}, "Commercials": {}, "Greetings": {}, "In The Fight": {},
	"Interviews": {}, "Newscasts": {}, "Package": {}, "PSA": {}, "Series": {},
}

var (
	publicDomainBlock = regexp.MustCompile(`(?is)<h4[^>]*class=["'][^"']*\bpublic-domain\b[^"']*["'][^>]*>\s*PUBLIC\s+DOMAIN\b`)
	copyrightBlock    = regexp.MustCompile(`(?is)<p[^>]*class=["']copyright_info["'][^>]*>(.*?)</p>`)
	htmlTag           = regexp.MustCompile(`(?s)<[^>]+>`)
)

type searchAsset struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Duration  int    `json:"duration"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
}

type searchResponse struct {
	PageInfo struct {
		TotalResults   int `json:"total_results"`
		ResultsPerPage int `json:"results_per_page"`
	} `json:"page_info"`
	Results []searchAsset `json:"results"`
}

type inventory struct {
	SchemaVersion int              `json:"schemaVersion"`
	Source        string           `json:"source"`
	Collection    string           `json:"collection"`
	SnapshotAt    time.Time        `json:"snapshotAt"`
	Categories    []string         `json:"categories"`
	MaxRequests   int              `json:"maxRequests"`
	RequestsUsed  int              `json:"requestsUsed"`
	MaxItems      int              `json:"maxItems"`
	MaxDuration   int              `json:"maxDurationSeconds"`
	MaxItemBytes  int64            `json:"maxItemBytes"`
	MaxTotalBytes int64            `json:"maxTotalBytes"`
	SelectedBytes int64            `json:"selectedBytes"`
	CacheHits     int              `json:"cacheHits"`
	SearchPages   []sourceEvidence `json:"searchPages"`
	Cases         []candidate      `json:"cases"`
}

type sourceEvidence struct {
	URL         string    `json:"url"`
	Cache       string    `json:"cache"`
	RetrievedAt time.Time `json:"retrievedAt"`
	SHA256      string    `json:"sha256"`
	Total       int       `json:"total"`
	Returned    int       `json:"returned"`
}

type options struct {
	apiBaseURL, siteBaseURL, apiKey, outputPath, cacheDir, userAgent string
	categories                                                       []string
	snapshotAt                                                       time.Time
	maxRequests, maxItems, maxDuration                               int
	maxItemBytes, maxTotalBytes                                      int64
	delay                                                            time.Duration
}

type detailResponse struct {
	Results detailAsset `json:"results"`
}

type detailAsset struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Date        string      `json:"date"`
	Category    string      `json:"category"`
	UnitID      string      `json:"unit_id"`
	UnitName    string      `json:"unit_name"`
	Timestamp   string      `json:"timestamp"`
	URL         string      `json:"url"`
	Credit      []credit    `json:"credit"`
	VIRIN       string      `json:"virin"`
	Duration    int         `json:"duration"`
	Files       []dvidsFile `json:"files"`
}

type credit struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Rank string `json:"rank"`
}

type dvidsFile struct {
	Src     string `json:"src"`
	Type    string `json:"type"`
	Height  int    `json:"height"`
	Width   int    `json:"width"`
	Size    int64  `json:"size"`
	Bitrate int    `json:"bitrate"`
}

type candidate struct {
	Identifier            string       `json:"identifier"`
	Title                 string       `json:"title"`
	Collection            []string     `json:"collection"`
	Creator               []string     `json:"creator,omitempty"`
	Date                  string       `json:"date,omitempty"`
	LicenseURL            string       `json:"licenseUrl"`
	Rights                []string     `json:"rights,omitempty"`
	ItemURL               string       `json:"itemUrl"`
	MetadataURL           string       `json:"metadataUrl"`
	MetadataCache         string       `json:"metadataCache"`
	MetadataRetrievedAt   time.Time    `json:"metadataRetrievedAt"`
	MetadataSHA256        string       `json:"metadataSha256"`
	RightsPageCache       string       `json:"rightsPageCache"`
	RightsPageRetrievedAt time.Time    `json:"rightsPageRetrievedAt"`
	RightsPageSHA256      string       `json:"rightsPageSha256"`
	Unit                  string       `json:"unit"`
	VIRIN                 string       `json:"virin"`
	Category              string       `json:"category"`
	File                  selectedFile `json:"file"`
}

type fetcher struct {
	client        *http.Client
	apiKey        string
	userAgent     string
	delay         time.Duration
	maxRequests   int
	requests      int
	cacheHits     int
	lastRequestAt time.Time
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-dvids", flag.ContinueOnError)
	flags.SetOutput(stderr)
	apiKey := flags.String("api-key", "", "DVIDS API key (never persisted)")
	categoriesText := flags.String("categories", "", "comma-separated DVIDS video categories")
	outputPath := flags.String("out", "", "output inventory JSON")
	cacheDir := flags.String("cache-dir", "", "raw search, detail, and item-rights cache directory")
	userAgent := flags.String("user-agent", "", "descriptive User-Agent with contact")
	snapshotAtText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 0, "hard selected item ceiling")
	maxDuration := flags.Int("max-duration", 0, "hard candidate duration ceiling in seconds")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard per-item predicted media-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", time.Second, "minimum delay between HTTP requests")
	apiBaseURL := flags.String("api-base-url", defaultAPIBaseURL, "DVIDS API base URL")
	siteBaseURL := flags.String("site-base-url", defaultSiteBaseURL, "DVIDS item-page base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	categories, err := parseCategories(*categoriesText)
	if *apiKey == "" || len(categories) == 0 || *outputPath == "" || *cacheDir == "" || *userAgent == "" || *snapshotAtText == "" ||
		*maxRequests < 3 || *maxItems <= 0 || *maxDuration <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 500*time.Millisecond {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-dvids: API key, declared categories, output, cache, identified User-Agent, snapshot, >=3 requests, positive item/duration/byte ceilings, and >=500ms delay are required")
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-dvids:", err)
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-dvids: parse --snapshot-at:", err)
		return 2
	}
	opts := options{
		apiBaseURL: strings.TrimRight(*apiBaseURL, "/"), siteBaseURL: strings.TrimRight(*siteBaseURL, "/"), apiKey: *apiKey,
		outputPath: *outputPath, cacheDir: *cacheDir, userAgent: *userAgent, categories: categories, snapshotAt: snapshotAt,
		maxRequests: *maxRequests, maxItems: *maxItems, maxDuration: *maxDuration, maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay,
	}
	result, err := buildInventory(context.Background(), &http.Client{Timeout: time.Minute}, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-dvids:", err)
		return 1
	}
	if err := writeJSON(opts.outputPath, result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-dvids: write inventory:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-dvids: froze %d candidates (%d bytes) in %d requests\n", len(result.Cases), result.SelectedBytes, result.RequestsUsed)
	return 0
}

func parseCategories(value string) ([]string, error) {
	seen := map[string]struct{}{}
	var result []string
	for _, raw := range strings.Split(value, ",") {
		category := strings.TrimSpace(raw)
		if category == "" {
			continue
		}
		if _, ok := allowedVideoCategories[category]; !ok {
			return nil, fmt.Errorf("unsupported DVIDS video category %q", category)
		}
		if _, duplicate := seen[category]; duplicate {
			continue
		}
		seen[category] = struct{}{}
		result = append(result, category)
	}
	sort.Strings(result)
	return result, nil
}

func buildInventory(ctx context.Context, client *http.Client, opts options) (inventory, error) {
	if err := os.MkdirAll(opts.cacheDir, 0o750); err != nil {
		return inventory{}, err
	}
	fetch := &fetcher{client: client, apiKey: opts.apiKey, userAgent: opts.userAgent, delay: opts.delay, maxRequests: opts.maxRequests}
	result := inventory{
		SchemaVersion: 1, Source: "dvids", Collection: "dvids:" + strings.Join(opts.categories, "+"), SnapshotAt: opts.snapshotAt.UTC(), Categories: append([]string(nil), opts.categories...),
		MaxRequests: opts.maxRequests, MaxItems: opts.maxItems, MaxDuration: opts.maxDuration, MaxItemBytes: opts.maxItemBytes, MaxTotalBytes: opts.maxTotalBytes,
	}
	seen := map[string]struct{}{}
	var refs []searchAsset
	for _, category := range opts.categories {
		for pageNumber := 1; ; pageNumber++ {
			requestURL, evidenceURL, err := dvidsSearchURL(opts.apiBaseURL, opts.apiKey, category, opts.maxDuration, pageNumber)
			if err != nil {
				return inventory{}, err
			}
			cacheName := "search-" + cacheSlug(category) + "-" + strconv.Itoa(pageNumber) + "-" + sha256Hex([]byte(evidenceURL))[:12] + ".json"
			raw, retrievedAt, err := fetch.get(ctx, requestURL, evidenceURL, filepath.Join(opts.cacheDir, cacheName), maxSearchBody)
			if err != nil {
				return inventory{}, err
			}
			var search searchResponse
			if err := json.Unmarshal(raw, &search); err != nil {
				return inventory{}, fmt.Errorf("decode DVIDS search evidence %s: %w", evidenceURL, err)
			}
			if search.PageInfo.TotalResults < len(search.Results) || search.PageInfo.ResultsPerPage <= 0 || search.PageInfo.ResultsPerPage > 50 || search.PageInfo.TotalResults > 1000 {
				return inventory{}, fmt.Errorf("DVIDS search %s has invalid or unsupported pagination", evidenceURL)
			}
			result.SearchPages = append(result.SearchPages, sourceEvidence{URL: evidenceURL, Cache: cacheName, RetrievedAt: retrievedAt, SHA256: sha256Hex(raw), Total: search.PageInfo.TotalResults, Returned: len(search.Results)})
			for _, ref := range search.Results {
				if ref.Type != "video" || ref.Category != category || ref.Duration <= 0 || ref.Duration > opts.maxDuration || ref.ID == "" || ref.URL == "" {
					continue
				}
				if _, duplicate := seen[ref.ID]; duplicate {
					continue
				}
				seen[ref.ID] = struct{}{}
				refs = append(refs, ref)
			}
			if pageNumber*search.PageInfo.ResultsPerPage >= search.PageInfo.TotalResults {
				break
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return sha256Hex([]byte(result.Collection+"/"+refs[i].ID)) < sha256Hex([]byte(result.Collection+"/"+refs[j].ID))
	})
	for _, ref := range refs {
		if len(result.Cases) >= opts.maxItems {
			break
		}
		if fetch.requests+2 > fetch.maxRequests {
			break
		}
		if !sameOriginPrefix(ref.URL, opts.siteBaseURL+"/video/") {
			continue
		}
		safeID := strings.ReplaceAll(ref.ID, ":", "-")
		detailRequestURL, detailEvidenceURL, err := dvidsDetailURL(opts.apiBaseURL, opts.apiKey, ref.ID)
		if err != nil {
			return inventory{}, err
		}
		detailCache := safeID + ".json"
		detailRaw, detailRetrievedAt, err := fetch.get(ctx, detailRequestURL, detailEvidenceURL, filepath.Join(opts.cacheDir, detailCache), maxDetailBody)
		if err != nil {
			return inventory{}, err
		}
		pageCache := safeID + ".html"
		pageRaw, pageRetrievedAt, err := fetch.get(ctx, ref.URL, ref.URL, filepath.Join(opts.cacheDir, pageCache), maxPageBody)
		if err != nil {
			return inventory{}, err
		}
		item, ok := parseCandidate(ref, detailRaw, pageRaw, detailCache, pageCache, detailRetrievedAt, opts.maxItemBytes)
		if !ok || item.File.Bytes > opts.maxTotalBytes-result.SelectedBytes {
			continue
		}
		item.RightsPageRetrievedAt = pageRetrievedAt
		result.Cases = append(result.Cases, item)
		result.SelectedBytes += item.File.Bytes
	}
	result.RequestsUsed = fetch.requests
	result.CacheHits = fetch.cacheHits
	if len(result.Cases) == 0 {
		return inventory{}, fmt.Errorf("no item-marked public-domain DVIDS videos fit the declared categories and ceilings")
	}
	return result, nil
}

func cacheSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(value)
}

func dvidsSearchURL(baseURL, apiKey, category string, maxDuration, pageNumber int) (string, string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/search")
	if err != nil {
		return "", "", err
	}
	query := u.Query()
	query.Set("api_key", apiKey)
	query.Set("type", "video")
	query.Set("category", category)
	query.Set("to_duration", strconv.Itoa(maxDuration))
	query.Set("max_results", "50")
	query.Set("page", strconv.Itoa(pageNumber))
	query.Set("sort", "timestamp")
	query.Set("sortdir", "asc")
	u.RawQuery = query.Encode()
	requestURL := u.String()
	query.Del("api_key")
	u.RawQuery = query.Encode()
	return requestURL, u.String(), nil
}

func dvidsDetailURL(baseURL, apiKey, id string) (string, string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/asset")
	if err != nil {
		return "", "", err
	}
	query := u.Query()
	query.Set("id", id)
	query.Set("api_key", apiKey)
	u.RawQuery = query.Encode()
	requestURL := u.String()
	query.Del("api_key")
	u.RawQuery = query.Encode()
	return requestURL, u.String(), nil
}

func (f *fetcher) get(ctx context.Context, requestURL, evidenceURL, cachePath string, maxBody int64) ([]byte, time.Time, error) {
	if raw, err := os.ReadFile(cachePath); err == nil {
		info, statErr := os.Stat(cachePath)
		if statErr != nil {
			return nil, time.Time{}, statErr
		}
		f.cacheHits++
		return raw, info.ModTime().UTC(), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, err
	}
	if f.requests >= f.maxRequests {
		return nil, time.Time{}, fmt.Errorf("request ceiling %d exhausted before %s", f.maxRequests, evidenceURL)
	}
	if !f.lastRequestAt.IsZero() {
		wait := f.delay - time.Since(f.lastRequestAt)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, time.Time{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("build GET %s: %s", evidenceURL, redactSecret(err.Error(), f.apiKey))
	}
	req.Header.Set("User-Agent", f.userAgent)
	response, err := f.client.Do(req)
	f.requests++
	f.lastRequestAt = time.Now()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("GET %s: %s", evidenceURL, redactSecret(err.Error(), f.apiKey))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("GET %s: status %d", evidenceURL, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, time.Time{}, err
	}
	if int64(len(raw)) > maxBody {
		return nil, time.Time{}, fmt.Errorf("GET %s exceeded %d-byte response ceiling", evidenceURL, maxBody)
	}
	if err := writeBytes(cachePath, raw); err != nil {
		return nil, time.Time{}, err
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	return raw, info.ModTime().UTC(), nil
}

func redactSecret(message, secret string) string {
	if secret == "" {
		return message
	}
	for _, representation := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
		message = strings.ReplaceAll(message, representation, "[redacted]")
	}
	return message
}

func writeBytes(cachePath string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(cachePath), ".dvids-cache-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
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
	if err := os.Rename(tempName, cachePath); err != nil {
		return err
	}
	ok = true
	return nil
}

func writeJSON(outputPath string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(outputPath, append(raw, '\n'))
}

func sameOriginPrefix(rawURL, prefix string) bool {
	u, err := url.Parse(rawURL)
	p, prefixErr := url.Parse(prefix)
	return err == nil && prefixErr == nil && u.Scheme == "https" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Scheme == p.Scheme && strings.EqualFold(u.Host, p.Host) && strings.HasPrefix(u.Path, p.Path)
}

type selectedFile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	Bytes    int64  `json:"bytes"`
	Duration string `json:"duration,omitempty"`
	Width    string `json:"width,omitempty"`
	Height   string `json:"height,omitempty"`
}

func parseCandidate(ref searchAsset, detailRaw, pageRaw []byte, detailCache, pageCache string, retrievedAt time.Time, maxItemBytes int64) (candidate, bool) {
	var response detailResponse
	if json.Unmarshal(detailRaw, &response) != nil {
		return candidate{}, false
	}
	item := response.Results
	if ref.ID == "" || item.ID != ref.ID || item.Type != "video" || ref.Type != "video" || item.URL != ref.URL || item.Category != ref.Category || item.Duration != ref.Duration || item.Timestamp != ref.Timestamp ||
		strings.TrimSpace(item.UnitID) == "" || strings.TrimSpace(item.UnitName) == "" || strings.TrimSpace(item.VIRIN) == "" || len(item.Credit) == 0 || !publicDomainBlock.Match(pageRaw) {
		return candidate{}, false
	}
	rightsMatch := copyrightBlock.FindSubmatch(pageRaw)
	if len(rightsMatch) != 2 {
		return candidate{}, false
	}
	rights := strings.Join(strings.Fields(html.UnescapeString(htmlTag.ReplaceAllString(string(rightsMatch[1]), " "))), " ")
	if rights == "" || !strings.Contains(rights, "DVIDS") || !strings.Contains(rights, "https://www.dvidshub.net/about/copyright") {
		return candidate{}, false
	}
	file, ok := pickFile(item.Files, item.Duration, maxItemBytes)
	if !ok {
		return candidate{}, false
	}
	id := strings.ReplaceAll(item.ID, ":", "-")
	creators := make([]string, 0, len(item.Credit))
	for _, value := range item.Credit {
		name := strings.TrimSpace(strings.TrimSpace(value.Rank) + " " + strings.TrimSpace(value.Name))
		if value.ID <= 0 || name == "" {
			return candidate{}, false
		}
		creators = append(creators, name)
	}
	return candidate{
		Identifier: "dvids-" + id, Title: item.Title, Collection: []string{"dvids:" + item.Category}, Creator: creators,
		Date: item.Date, LicenseURL: "https://www.dvidshub.net/about/copyright", Rights: []string{rights},
		ItemURL: item.URL, MetadataURL: "https://api.dvidshub.net/asset?id=" + url.QueryEscape(item.ID),
		MetadataCache: detailCache, MetadataRetrievedAt: retrievedAt.UTC(), MetadataSHA256: sha256Hex(detailRaw),
		RightsPageCache: pageCache, RightsPageSHA256: sha256Hex(pageRaw), Unit: item.UnitName, VIRIN: item.VIRIN, Category: item.Category, File: file,
	}, true
}

func pickFile(files []dvidsFile, duration int, maxItemBytes int64) (selectedFile, bool) {
	choices := make([]dvidsFile, 0, len(files))
	for _, file := range files {
		u, err := url.Parse(file.Src)
		if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !allowedMediaHost(u.Hostname()) ||
			file.Type != "video/mp4" || file.Size <= 0 || file.Size > maxItemBytes || file.Width <= 0 || file.Height <= 0 || file.Width > 1280 || file.Height > 720 || duration <= 0 {
			continue
		}
		choices = append(choices, file)
	}
	if len(choices) == 0 {
		return selectedFile{}, false
	}
	sort.Slice(choices, func(i, j int) bool {
		left, right := choices[i].Width*choices[i].Height, choices[j].Width*choices[j].Height
		if left != right {
			return left > right
		}
		if choices[i].Bitrate != choices[j].Bitrate {
			return choices[i].Bitrate > choices[j].Bitrate
		}
		return choices[i].Src < choices[j].Src
	})
	chosen := choices[0]
	u, _ := url.Parse(chosen.Src)
	return selectedFile{
		Name: path.Base(u.Path), URL: chosen.Src, Format: chosen.Type, Source: "dvids", Bytes: chosen.Size,
		Duration: strconv.Itoa(duration), Width: strconv.Itoa(chosen.Width), Height: strconv.Itoa(chosen.Height),
	}, true
}

func allowedMediaHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".dvidshub.net") || host == "d34w7g4gy10iej.cloudfront.net"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
