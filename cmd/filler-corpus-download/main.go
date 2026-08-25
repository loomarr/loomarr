// Command filler-corpus-download downloads only independently rights-approved
// corpus media under explicit request, item, and byte ceilings.
package main

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type inventory struct {
	Source string               `json:"source"`
	Cases  []inventoryCandidate `json:"cases"`
}

type inventoryCandidate struct {
	Identifier            string     `json:"identifier"`
	LicenseURL            string     `json:"licenseUrl"`
	ItemURL               string     `json:"itemUrl"`
	MetadataURL           string     `json:"metadataUrl"`
	MetadataRetrievedAt   time.Time  `json:"metadataRetrievedAt"`
	MetadataSHA256        string     `json:"metadataSha256"`
	RightsPageRetrievedAt time.Time  `json:"rightsPageRetrievedAt,omitempty"`
	RightsPageSHA256      string     `json:"rightsPageSha256,omitempty"`
	File                  sourceFile `json:"file"`
}

type sourceFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Format string `json:"format"`
	Source string `json:"source"`
	Bytes  int64  `json:"bytes"`
	SHA1   string `json:"sha1,omitempty"`
	MD5    string `json:"md5,omitempty"`
}

type rightsApproval struct {
	InventorySHA256  string    `json:"inventorySha256"`
	Identifier       string    `json:"identifier"`
	MetadataSHA256   string    `json:"metadataSha256"`
	RightsPageSHA256 string    `json:"rightsPageSha256,omitempty"`
	ReviewerID       string    `json:"reviewerId"`
	ReviewedAt       time.Time `json:"reviewedAt"`
	Decision         string    `json:"decision"`
	Basis            string    `json:"basis"`
	Redistributable  bool      `json:"redistributable"`
	RequiredCredit   string    `json:"requiredCredit,omitempty"`
	Restrictions     []string  `json:"restrictions,omitempty"`
}

type downloadLedger struct {
	SchemaVersion int              `json:"schemaVersion"`
	Source        string           `json:"source"`
	GeneratedAt   time.Time        `json:"generatedAt"`
	MaxRequests   int              `json:"maxRequests"`
	RequestsUsed  int              `json:"requestsUsed"`
	MaxItems      int              `json:"maxItems"`
	MaxBytes      int64            `json:"maxBytes"`
	Bytes         int64            `json:"bytes"`
	Cases         []downloadedCase `json:"cases"`
}

type downloadedCase struct {
	Identifier            string         `json:"identifier"`
	LicenseURL            string         `json:"licenseUrl"`
	ItemURL               string         `json:"itemUrl"`
	MetadataURL           string         `json:"metadataUrl"`
	MetadataRetrievedAt   time.Time      `json:"metadataRetrievedAt"`
	MetadataSHA256        string         `json:"metadataSha256"`
	RightsPageRetrievedAt time.Time      `json:"rightsPageRetrievedAt,omitempty"`
	RightsPageSHA256      string         `json:"rightsPageSha256,omitempty"`
	File                  sourceFile     `json:"file"`
	LocalFile             string         `json:"localFile"`
	ContentSHA256         string         `json:"contentSha256"`
	Approval              rightsApproval `json:"approval"`
	VerifiedAt            time.Time      `json:"verifiedAt"`
}

var (
	safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	sha256Digest   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type plannedDownload struct {
	candidate inventoryCandidate
	approval  rightsApproval
	path      string
}

type options struct {
	inventoryPath, approvalsPath, outputDir, ledgerPath, userAgent string
	inventorySHA256                                                string
	generatedAt                                                    time.Time
	maxRequests, maxItems                                          int
	maxBytes                                                       int64
	delay                                                          time.Duration
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-download", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "frozen source inventory JSON")
	approvalsPath := flags.String("rights-approvals", "", "independent rights decisions JSONL")
	outputDir := flags.String("out-dir", "", "external corpus media directory")
	ledgerPath := flags.String("ledger", "", "content-addressed download ledger JSON")
	userAgent := flags.String("user-agent", "", "descriptive source User-Agent with contact")
	generatedAtText := flags.String("generated-at", "", "ledger generation time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 0, "hard approved item ceiling")
	maxBytes := flags.Int64("max-bytes", 0, "hard approved media-byte ceiling")
	delay := flags.Duration("delay", time.Second, "minimum delay between HTTP requests")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *inventoryPath == "" || *approvalsPath == "" || *outputDir == "" || *ledgerPath == "" || *userAgent == "" || *generatedAtText == "" || *maxRequests <= 0 || *maxItems <= 0 || *maxBytes <= 0 || *delay < 500*time.Millisecond {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: inventory, rights approvals, output, ledger, identified User-Agent, generation time, positive ceilings, and >=500ms delay are required")
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: parse --generated-at:", err)
		return 2
	}
	opts := options{
		inventoryPath: *inventoryPath, approvalsPath: *approvalsPath, outputDir: *outputDir,
		ledgerPath: *ledgerPath, userAgent: *userAgent, generatedAt: generatedAt,
		maxRequests: *maxRequests, maxItems: *maxItems, maxBytes: *maxBytes, delay: *delay,
	}
	inv, inventorySHA256, err := readInventory(opts.inventoryPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: read inventory:", err)
		return 1
	}
	opts.inventorySHA256 = inventorySHA256
	approvals, err := readJSONL[rightsApproval](opts.approvalsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: read approvals:", err)
		return 1
	}
	plan, err := planDownloads(inv, approvals, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download:", err)
		return 1
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !allowedRedirect(inv.Source, req.URL, via) {
				return fmt.Errorf("refuse %s redirect to %s", inv.Source, req.URL.Hostname())
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	ledger, err := executeDownloads(context.Background(), client, plan, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download:", err)
		return 1
	}
	ledger.Source = inv.Source
	if err := writeJSON(opts.ledgerPath, ledger); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: write ledger:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-download: locked %d files (%d bytes) in %d requests\n", len(ledger.Cases), ledger.Bytes, ledger.RequestsUsed)
	return 0
}

func planDownloads(inv inventory, approvals []rightsApproval, opts options) ([]plannedDownload, error) {
	if inv.Source != "archive.org" && inv.Source != "dvids" {
		return nil, fmt.Errorf("unsupported inventory source %q", inv.Source)
	}
	byID := make(map[string]inventoryCandidate, len(inv.Cases))
	for _, candidate := range inv.Cases {
		if !safeIdentifier.MatchString(candidate.Identifier) || strings.Contains(candidate.Identifier, "..") {
			return nil, fmt.Errorf("unsafe inventory identifier %q", candidate.Identifier)
		}
		if !sha256Digest.MatchString(candidate.MetadataSHA256) || candidate.MetadataRetrievedAt.IsZero() || candidate.File.Bytes <= 0 || strings.TrimSpace(candidate.File.Name) == "" {
			return nil, fmt.Errorf("inventory candidate %s has incomplete frozen metadata or media identity", candidate.Identifier)
		}
		if inv.Source == "dvids" && (!sha256Digest.MatchString(candidate.RightsPageSHA256) || candidate.RightsPageRetrievedAt.IsZero()) {
			return nil, fmt.Errorf("DVIDS inventory candidate %s lacks frozen item-page rights evidence", candidate.Identifier)
		}
		if _, duplicate := byID[candidate.Identifier]; duplicate {
			return nil, fmt.Errorf("duplicate inventory candidate %s", candidate.Identifier)
		}
		byID[candidate.Identifier] = candidate
	}
	seen := map[string]struct{}{}
	var plan []plannedDownload
	var bytes int64
	for _, approval := range approvals {
		if _, duplicate := seen[approval.Identifier]; duplicate {
			return nil, fmt.Errorf("duplicate rights decision for %s", approval.Identifier)
		}
		seen[approval.Identifier] = struct{}{}
		candidate, ok := byID[approval.Identifier]
		if !ok {
			return nil, fmt.Errorf("rights-reviewed item %s is absent from the inventory", approval.Identifier)
		}
		if approval.InventorySHA256 != opts.inventorySHA256 {
			return nil, fmt.Errorf("rights-reviewed item %s is not tied to the frozen inventory", approval.Identifier)
		}
		if approval.Decision != "approved" && approval.Decision != "held" {
			return nil, fmt.Errorf("rights-reviewed item %s has unknown decision %q", approval.Identifier, approval.Decision)
		}
		evidenceRetrievedAt := candidate.MetadataRetrievedAt
		if candidate.RightsPageRetrievedAt.After(evidenceRetrievedAt) {
			evidenceRetrievedAt = candidate.RightsPageRetrievedAt
		}
		if approval.MetadataSHA256 != candidate.MetadataSHA256 || approval.ReviewerID == "" || approval.ReviewedAt.IsZero() || approval.ReviewedAt.Before(evidenceRetrievedAt) || approval.ReviewedAt.After(opts.generatedAt) || strings.TrimSpace(approval.Basis) == "" {
			return nil, fmt.Errorf("rights-reviewed item %s is not tied to its metadata and complete review", approval.Identifier)
		}
		if inv.Source == "dvids" && approval.RightsPageSHA256 != candidate.RightsPageSHA256 {
			return nil, fmt.Errorf("rights-reviewed item %s is not tied to its item-page rights evidence", approval.Identifier)
		}
		if approval.Decision == "held" {
			continue
		}
		if !approval.Redistributable {
			return nil, fmt.Errorf("approved item %s is not explicitly redistributable", approval.Identifier)
		}
		if requiresCredit(inv.Source, candidate.LicenseURL) && strings.TrimSpace(approval.RequiredCredit) == "" {
			return nil, fmt.Errorf("approved item %s requires attribution", approval.Identifier)
		}
		if candidate.File.Source != "" && candidate.File.Source != inv.Source {
			return nil, fmt.Errorf("inventory candidate %s media source does not match %s", candidate.Identifier, inv.Source)
		}
		if err := validateSourceURL(inv.Source, candidate.Identifier, candidate.File.URL); err != nil {
			return nil, err
		}
		bytes += candidate.File.Bytes
		ext := filepath.Ext(candidate.File.Name)
		plan = append(plan, plannedDownload{candidate: candidate, approval: approval, path: filepath.Join(opts.outputDir, candidate.Identifier+ext)})
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("rights ledger approves no media")
	}
	if len(plan) > opts.maxItems || bytes > opts.maxBytes {
		return nil, fmt.Errorf("approved plan has %d items and %d bytes; ceilings are %d and %d", len(plan), bytes, opts.maxItems, opts.maxBytes)
	}
	sort.Slice(plan, func(i, j int) bool { return plan[i].candidate.Identifier < plan[j].candidate.Identifier })
	return plan, nil
}

func executeDownloads(ctx context.Context, client *http.Client, plan []plannedDownload, opts options) (downloadLedger, error) {
	if err := os.MkdirAll(opts.outputDir, 0o750); err != nil {
		return downloadLedger{}, err
	}
	ledger := downloadLedger{SchemaVersion: 1, GeneratedAt: opts.generatedAt.UTC(), MaxRequests: opts.maxRequests, MaxItems: opts.maxItems, MaxBytes: opts.maxBytes}
	lastRequestAt := time.Time{}
	for _, item := range plan {
		verifiedAt := opts.generatedAt.UTC()
		hashes, size, err := hashFile(item.path)
		if errors.Is(err, os.ErrNotExist) {
			if ledger.RequestsUsed >= opts.maxRequests {
				return downloadLedger{}, fmt.Errorf("request ceiling exhausted before %s", item.candidate.Identifier)
			}
			if !lastRequestAt.IsZero() {
				wait := opts.delay - time.Since(lastRequestAt)
				if wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-ctx.Done():
						timer.Stop()
						return downloadLedger{}, ctx.Err()
					case <-timer.C:
					}
				}
			}
			hashes, size, err = download(ctx, client, item, opts.userAgent)
			ledger.RequestsUsed++
			lastRequestAt = time.Now()
			verifiedAt = lastRequestAt.UTC()
		}
		if err != nil {
			return downloadLedger{}, fmt.Errorf("%s: %w", item.candidate.Identifier, err)
		}
		if size != item.candidate.File.Bytes || !matchesOptional(hashes.sha1, item.candidate.File.SHA1) || !matchesOptional(hashes.md5, item.candidate.File.MD5) {
			return downloadLedger{}, fmt.Errorf("%s: downloaded bytes or source checksums do not match inventory", item.candidate.Identifier)
		}
		ledger.Bytes += size
		ledger.Cases = append(ledger.Cases, downloadedCase{
			Identifier: item.candidate.Identifier, LicenseURL: item.candidate.LicenseURL,
			ItemURL: item.candidate.ItemURL, MetadataURL: item.candidate.MetadataURL,
			MetadataRetrievedAt: item.candidate.MetadataRetrievedAt, MetadataSHA256: item.candidate.MetadataSHA256,
			RightsPageRetrievedAt: item.candidate.RightsPageRetrievedAt, RightsPageSHA256: item.candidate.RightsPageSHA256,
			File: item.candidate.File, LocalFile: filepath.Base(item.path), ContentSHA256: hashes.sha256,
			Approval: item.approval, VerifiedAt: verifiedAt,
		})
	}
	return ledger, nil
}

type fileHashes struct{ sha256, sha1, md5 string }

func download(ctx context.Context, client *http.Client, item plannedDownload, userAgent string) (fileHashes, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.candidate.File.URL, nil)
	if err != nil {
		return fileHashes{}, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return fileHashes{}, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fileHashes{}, 0, fmt.Errorf("GET: %s", resp.Status)
	}
	if resp.ContentLength > item.candidate.File.Bytes && resp.ContentLength >= 0 {
		return fileHashes{}, 0, fmt.Errorf("Content-Length exceeds inventory size")
	}
	temp, err := os.CreateTemp(filepath.Dir(item.path), ".filler-corpus-download-*")
	if err != nil {
		return fileHashes{}, 0, err
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
		return fileHashes{}, 0, err
	}
	hashes, n, err := copyAndHash(temp, io.LimitReader(resp.Body, item.candidate.File.Bytes+1))
	if err != nil {
		return fileHashes{}, 0, err
	}
	if n != item.candidate.File.Bytes {
		return fileHashes{}, 0, fmt.Errorf("received %d bytes, want %d", n, item.candidate.File.Bytes)
	}
	if !matchesOptional(hashes.sha1, item.candidate.File.SHA1) || !matchesOptional(hashes.md5, item.candidate.File.MD5) {
		return fileHashes{}, 0, fmt.Errorf("source checksums do not match inventory")
	}
	if err := temp.Close(); err != nil {
		return fileHashes{}, 0, err
	}
	if err := os.Rename(tempName, item.path); err != nil {
		return fileHashes{}, 0, err
	}
	ok = true
	return hashes, n, nil
}

func hashFile(path string) (fileHashes, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileHashes{}, 0, err
	}
	defer func() { _ = file.Close() }()
	return copyAndHash(io.Discard, file)
}

func copyAndHash(destination io.Writer, source io.Reader) (fileHashes, int64, error) {
	sha256Hash, sha1Hash, md5Hash := sha256.New(), sha1.New(), md5.New()
	writers := []io.Writer{destination, sha256Hash, sha1Hash, md5Hash}
	n, err := io.Copy(io.MultiWriter(writers...), source)
	if err != nil {
		return fileHashes{}, n, err
	}
	return fileHashes{sha256: sumHex(sha256Hash), sha1: sumHex(sha1Hash), md5: sumHex(md5Hash)}, n, nil
}

func sumHex(value hash.Hash) string { return hex.EncodeToString(value.Sum(nil)) }

func matchesOptional(actual, expected string) bool {
	return expected == "" || strings.EqualFold(actual, expected)
}

func validateSourceURL(source, identifier, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s: invalid %s media URL", identifier, source)
	}
	switch source {
	case "archive.org":
		if !archiveHost(u.Hostname()) || !strings.HasPrefix(u.EscapedPath(), "/download/"+url.PathEscape(identifier)+"/") {
			return fmt.Errorf("%s: invalid Archive.org media URL", identifier)
		}
	case "dvids":
		if !dvidsMediaHost(u.Hostname()) || !strings.HasSuffix(strings.ToLower(u.EscapedPath()), ".mp4") {
			return fmt.Errorf("%s: invalid DVIDS media URL", identifier)
		}
	default:
		return fmt.Errorf("%s: unsupported media source %q", identifier, source)
	}
	return nil
}

func archiveHost(host string) bool {
	return host == "archive.org" || strings.HasSuffix(host, ".archive.org")
}

func dvidsMediaHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".dvidshub.net") || host == "d34w7g4gy10iej.cloudfront.net"
}

func allowedRedirect(source string, destination *url.URL, via []*http.Request) bool {
	if len(via) >= 5 || destination == nil || destination.Scheme != "https" {
		return false
	}
	switch source {
	case "archive.org":
		return archiveHost(destination.Hostname())
	case "dvids":
		return len(via) > 0 && strings.EqualFold(destination.Hostname(), via[0].URL.Hostname()) && dvidsMediaHost(destination.Hostname())
	default:
		return false
	}
}

func requiresCredit(source, license string) bool {
	if source == "dvids" {
		return true
	}
	normalized := strings.ToLower(license)
	return strings.Contains(normalized, "/licenses/by/") || strings.Contains(normalized, "/licenses/by-sa/")
}

func readInventory(path string) (inventory, string, error) {
	var value inventory
	data, err := os.ReadFile(path)
	if err != nil {
		return value, "", err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, "", err
	}
	sum := sha256.Sum256(data)
	return value, hex.EncodeToString(sum[:]), nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var values []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		values = append(values, value)
	}
	return values, scanner.Err()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-ledger-*")
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
	if _, err := temp.Write(append(data, '\n')); err != nil {
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
