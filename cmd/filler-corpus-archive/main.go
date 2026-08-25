// Command filler-corpus-archive freezes a bounded Archive.org metadata
// inventory before any corpus media is downloaded or reviewed.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://archive.org"
	maxSearchBody   = 8 << 20
	maxMetadataBody = 16 << 20
)

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type inventory struct {
	SchemaVersion     int         `json:"schemaVersion"`
	Source            string      `json:"source"`
	Collection        string      `json:"collection"`
	SnapshotAt        time.Time   `json:"snapshotAt"`
	MaxRequests       int         `json:"maxRequests"`
	RequestsUsed      int         `json:"requestsUsed"`
	MaxItems          int         `json:"maxItems"`
	MaxItemBytes      int64       `json:"maxItemBytes"`
	MaxTotalBytes     int64       `json:"maxTotalBytes"`
	SelectedBytes     int64       `json:"selectedBytes"`
	CacheHits         int         `json:"cacheHits"`
	SearchSHA256      string      `json:"searchSha256"`
	SearchRetrievedAt time.Time   `json:"searchRetrievedAt"`
	Cases             []candidate `json:"cases"`
}

type candidate struct {
	Identifier              string       `json:"identifier"`
	Title                   string       `json:"title"`
	Collection              []string     `json:"collection"`
	Creator                 []string     `json:"creator,omitempty"`
	Date                    string       `json:"date,omitempty"`
	LicenseURL              string       `json:"licenseUrl"`
	Rights                  []string     `json:"rights,omitempty"`
	PossibleCopyrightStatus []string     `json:"possibleCopyrightStatus,omitempty"`
	ItemURL                 string       `json:"itemUrl"`
	MetadataURL             string       `json:"metadataUrl"`
	MetadataCache           string       `json:"metadataCache"`
	MetadataRetrievedAt     time.Time    `json:"metadataRetrievedAt"`
	MetadataSHA256          string       `json:"metadataSha256"`
	File                    selectedFile `json:"file"`
}

type selectedFile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	Bytes    int64  `json:"bytes"`
	SHA1     string `json:"sha1,omitempty"`
	MD5      string `json:"md5,omitempty"`
	Duration string `json:"duration,omitempty"`
	Width    string `json:"width,omitempty"`
	Height   string `json:"height,omitempty"`
}

type searchResponse struct {
	Response struct {
		NumFound int `json:"numFound"`
		Docs     []struct {
			Identifier string `json:"identifier"`
			LicenseURL string `json:"licenseurl"`
		} `json:"docs"`
	} `json:"response"`
}

type metadataResponse struct {
	Metadata map[string]json.RawMessage `json:"metadata"`
	Files    []archiveFile              `json:"files"`
}

type archiveFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Source string `json:"source"`
	Size   string `json:"size"`
	SHA1   string `json:"sha1"`
	MD5    string `json:"md5"`
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

type options struct {
	baseURL, collection, outputPath, cacheDir, userAgent string
	snapshotAt                                           time.Time
	maxRequests, maxItems                                int
	maxItemBytes, maxTotalBytes                          int64
	delay                                                time.Duration
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-archive", flag.ContinueOnError)
	flags.SetOutput(stderr)
	collection := flags.String("collection", "", "Archive.org collection identifier")
	outputPath := flags.String("out", "", "output inventory JSON")
	cacheDir := flags.String("cache-dir", "", "raw metadata cache directory")
	userAgent := flags.String("user-agent", "", "descriptive Archive.org User-Agent with contact")
	snapshotAtText := flags.String("snapshot-at", "", "snapshot time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 0, "hard selected item ceiling")
	maxItemBytes := flags.Int64("max-item-bytes", 0, "hard per-item predicted media-byte ceiling")
	maxTotalBytes := flags.Int64("max-total-bytes", 0, "hard predicted media-byte ceiling")
	delay := flags.Duration("delay", time.Second, "minimum delay between HTTP requests")
	baseURL := flags.String("base-url", defaultBaseURL, "Archive.org API base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *collection == "" || *outputPath == "" || *cacheDir == "" || *userAgent == "" || *snapshotAtText == "" || *maxRequests < 2 || *maxItems <= 0 || *maxItemBytes <= 0 || *maxTotalBytes < *maxItemBytes || *delay < 500*time.Millisecond {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-archive: collection, output, cache, identified User-Agent, snapshot time, >=2 requests, positive item/total byte ceilings, and >=500ms delay are required")
		return 2
	}
	if !safeIdentifier.MatchString(*collection) || strings.Contains(*collection, "..") {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-archive: collection identifier is unsafe")
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotAtText)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-archive: parse --snapshot-at: %v\n", err)
		return 2
	}
	opts := options{
		baseURL: strings.TrimRight(*baseURL, "/"), collection: *collection,
		outputPath: *outputPath, cacheDir: *cacheDir, userAgent: *userAgent,
		snapshotAt: snapshotAt, maxRequests: *maxRequests, maxItems: *maxItems,
		maxItemBytes: *maxItemBytes, maxTotalBytes: *maxTotalBytes, delay: *delay,
	}
	result, err := buildInventory(context.Background(), &http.Client{Timeout: time.Minute}, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-archive:", err)
		return 1
	}
	if err := writeJSON(opts.outputPath, result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-archive: write inventory:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-archive: froze %d candidates (%d bytes) in %d requests\n", len(result.Cases), result.SelectedBytes, result.RequestsUsed)
	return 0
}

func buildInventory(ctx context.Context, client *http.Client, opts options) (inventory, error) {
	if err := os.MkdirAll(opts.cacheDir, 0o750); err != nil {
		return inventory{}, err
	}
	requests := 0
	searchURL, err := archiveSearchURL(opts.baseURL, opts.collection)
	if err != nil {
		return inventory{}, err
	}
	searchCache := filepath.Join(opts.cacheDir, "search-"+sha256Hex([]byte(searchURL))[:16]+".json")
	searchBytes, used, searchRetrievedAt, err := cachedGet(ctx, client, searchURL, searchCache, opts.userAgent, maxSearchBody, requests, opts.maxRequests)
	requests += used
	if err != nil {
		return inventory{}, err
	}
	var search searchResponse
	if err := json.Unmarshal(searchBytes, &search); err != nil {
		return inventory{}, fmt.Errorf("decode search: %w", err)
	}
	if search.Response.NumFound > len(search.Response.Docs) {
		return inventory{}, fmt.Errorf("search returned %d of %d items; refuse a silently truncated inventory", len(search.Response.Docs), search.Response.NumFound)
	}
	type itemRef struct{ id, license string }
	refs := make([]itemRef, 0, len(search.Response.Docs))
	for _, doc := range search.Response.Docs {
		if safeIdentifier.MatchString(doc.Identifier) && !strings.Contains(doc.Identifier, "..") && allowedLicense(doc.LicenseURL) {
			refs = append(refs, itemRef{id: doc.Identifier, license: doc.LicenseURL})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return sha256Hex([]byte(opts.collection+"/"+refs[i].id)) < sha256Hex([]byte(opts.collection+"/"+refs[j].id))
	})
	result := inventory{
		SchemaVersion: 1, Source: "archive.org", Collection: opts.collection, SnapshotAt: opts.snapshotAt.UTC(),
		MaxRequests: opts.maxRequests, MaxItems: opts.maxItems, MaxItemBytes: opts.maxItemBytes, MaxTotalBytes: opts.maxTotalBytes,
		SearchSHA256: sha256Hex(searchBytes), SearchRetrievedAt: searchRetrievedAt,
	}
	lastRequestAt := time.Time{}
	if used > 0 {
		lastRequestAt = time.Now()
	}
	for _, ref := range refs {
		if len(result.Cases) >= opts.maxItems {
			break
		}
		cacheName := filepath.Join(opts.cacheDir, ref.id+".json")
		_, cacheErr := os.Stat(cacheName)
		cacheMissing := errors.Is(cacheErr, os.ErrNotExist)
		if cacheErr != nil && !cacheMissing {
			return inventory{}, cacheErr
		}
		if cacheMissing && requests >= opts.maxRequests {
			break
		}
		if cacheMissing && !lastRequestAt.IsZero() {
			wait := opts.delay - time.Since(lastRequestAt)
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return inventory{}, ctx.Err()
				case <-timer.C:
				}
			}
		}
		metadataURL := opts.baseURL + "/metadata/" + url.PathEscape(ref.id)
		raw, used, metadataRetrievedAt, err := cachedGet(ctx, client, metadataURL, cacheName, opts.userAgent, maxMetadataBody, requests, opts.maxRequests)
		if used > 0 {
			lastRequestAt = time.Now()
		} else {
			result.CacheHits++
		}
		requests += used
		if err != nil {
			return inventory{}, err
		}
		item, ok := parseCandidate(opts.baseURL, opts.collection, ref.id, ref.license, filepath.Base(cacheName), metadataRetrievedAt, raw)
		if !ok || item.File.Bytes > opts.maxItemBytes || item.File.Bytes > opts.maxTotalBytes-result.SelectedBytes {
			continue
		}
		result.Cases = append(result.Cases, item)
		result.SelectedBytes += item.File.Bytes
	}
	result.RequestsUsed = requests
	if len(result.Cases) == 0 {
		return inventory{}, fmt.Errorf("no rights-allowlisted video candidates fit the item and byte ceilings")
	}
	return result, nil
}

func archiveSearchURL(baseURL, collection string) (string, error) {
	u, err := url.Parse(baseURL + "/advancedsearch.php")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("q", "collection:"+collection+" AND mediatype:movies AND licenseurl:*")
	q.Add("fl[]", "identifier")
	q.Add("fl[]", "licenseurl")
	q.Set("rows", "1000")
	q.Set("page", "1")
	q.Add("sort[]", "identifier asc")
	q.Set("output", "json")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func cachedGet(ctx context.Context, client *http.Client, rawURL, cachePath, userAgent string, maxBody int64, used, maxRequests int) ([]byte, int, time.Time, error) {
	if data, err := os.ReadFile(cachePath); err == nil {
		info, statErr := os.Stat(cachePath)
		if statErr != nil {
			return nil, 0, time.Time{}, statErr
		}
		return data, 0, info.ModTime().UTC(), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, 0, time.Time{}, err
	}
	if used >= maxRequests {
		return nil, 0, time.Time{}, fmt.Errorf("request ceiling %d exhausted before %s", maxRequests, rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 1, time.Time{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 1, time.Time{}, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, 1, time.Time{}, err
	}
	if int64(len(data)) > maxBody {
		return nil, 1, time.Time{}, fmt.Errorf("GET %s exceeded %d-byte response ceiling", rawURL, maxBody)
	}
	if err := writeBytes(cachePath, data); err != nil {
		return nil, 1, time.Time{}, err
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, 1, time.Time{}, err
	}
	return data, 1, info.ModTime().UTC(), nil
}

func parseCandidate(baseURL, collection, identifier, searchLicense, cacheName string, retrievedAt time.Time, raw []byte) (candidate, bool) {
	var response metadataResponse
	if json.Unmarshal(raw, &response) != nil {
		return candidate{}, false
	}
	license := firstString(response.Metadata["licenseurl"])
	collections := stringsFromRaw(response.Metadata["collection"])
	if firstString(response.Metadata["identifier"]) != identifier || firstString(response.Metadata["mediatype"]) != "movies" || !slices.Contains(collections, collection) || !allowedLicense(license) || normalizeLicense(license) != normalizeLicense(searchLicense) {
		return candidate{}, false
	}
	file, ok := pickFile(response.Files, baseURL, identifier)
	if !ok {
		return candidate{}, false
	}
	return candidate{
		Identifier: identifier, Title: firstString(response.Metadata["title"]),
		Collection: collections, Creator: stringsFromRaw(response.Metadata["creator"]),
		Date: firstString(response.Metadata["date"]), LicenseURL: license,
		Rights: stringsFromRaw(response.Metadata["rights"]), PossibleCopyrightStatus: stringsFromRaw(response.Metadata["possible-copyright-status"]),
		ItemURL: baseURL + "/details/" + url.PathEscape(identifier), MetadataURL: baseURL + "/metadata/" + url.PathEscape(identifier),
		MetadataCache: cacheName, MetadataRetrievedAt: retrievedAt, MetadataSHA256: sha256Hex(raw), File: file,
	}, true
}

func pickFile(files []archiveFile, baseURL, identifier string) (selectedFile, bool) {
	type ranked struct {
		file archiveFile
		size int64
		rank int
	}
	var choices []ranked
	for _, file := range files {
		size, err := strconv.ParseInt(file.Size, 10, 64)
		if err != nil || size <= 0 || !isVideo(file) {
			continue
		}
		rank := 3
		format := strings.ToLower(file.Format)
		switch {
		case strings.Contains(format, "h.264 ia") || strings.Contains(format, "512kb"):
			rank = 0
		case file.Source == "derivative":
			rank = 1
		case file.Source == "original":
			rank = 2
		}
		choices = append(choices, ranked{file: file, size: size, rank: rank})
	}
	if len(choices) == 0 {
		return selectedFile{}, false
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].rank != choices[j].rank {
			return choices[i].rank < choices[j].rank
		}
		return choices[i].size < choices[j].size
	})
	choice := choices[0]
	return selectedFile{
		Name: choice.file.Name, URL: baseURL + "/download/" + url.PathEscape(identifier) + "/" + url.PathEscape(choice.file.Name),
		Format: choice.file.Format, Source: choice.file.Source, Bytes: choice.size,
		SHA1: choice.file.SHA1, MD5: choice.file.MD5, Duration: choice.file.Length,
		Width: choice.file.Width, Height: choice.file.Height,
	}, true
}

func isVideo(file archiveFile) bool {
	ext := strings.ToLower(filepath.Ext(file.Name))
	if !slices.Contains([]string{".mp4", ".m4v", ".mov", ".mpeg", ".mpg", ".ogv", ".webm", ".mkv"}, ext) {
		return false
	}
	format := strings.ToLower(file.Format)
	return !strings.Contains(format, "metadata") && !strings.Contains(format, "thumbnail")
}

func allowedLicense(value string) bool {
	normalized := normalizeLicense(value)
	if normalized == "" || strings.Contains(normalized, "/by-nc") || strings.Contains(normalized, "/by-nd") {
		return false
	}
	return strings.Contains(normalized, "/publicdomain/") || strings.Contains(normalized, "/licenses/publicdomain") || strings.Contains(normalized, "/licenses/by/") || strings.Contains(normalized, "/licenses/by-sa/")
}

func normalizeLicense(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return strings.TrimSuffix(value, "/")
}

func stringsFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	return nil
}

func firstString(raw json.RawMessage) string {
	values := stringsFromRaw(raw)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(path, append(data, '\n'))
}

func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-archive-*")
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
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
