// Command filler-corpus-direct freezes an authored, locally licensed corpus
// cohort into the same source-neutral inventory used by remote adapters.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const (
	directManifestSchema  = 2
	directAuthorityPrefix = "direct-license/"
)

var directID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Authority     string         `json:"authority"`
	Cohort        string         `json:"cohort"`
	RoleQuotas    map[string]int `json:"roleQuotas"`
	Cases         []manifestCase `json:"cases"`
}

type manifestCase struct {
	ItemID           string             `json:"itemId"`
	Title            string             `json:"title"`
	RoleHints        []string           `json:"roleHints"`
	MediaPath        string             `json:"mediaPath"`
	MIMEType         string             `json:"mimeType"`
	RightsAssertions []string           `json:"rightsAssertions"`
	LicenseURL       string             `json:"licenseUrl,omitempty"`
	ItemURL          string             `json:"itemUrl,omitempty"`
	Creator          []string           `json:"creator,omitempty"`
	Campaign         string             `json:"campaign"`
	SourceFamily     string             `json:"sourceFamily"`
	Date             string             `json:"date,omitempty"`
	Evidence         []manifestEvidence `json:"evidence"`
}

type manifestEvidence struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type options struct {
	manifestPath  string
	root          string
	out           string
	snapshotAt    time.Time
	expectedItems int
	maxBytes      int64
	maxWallTime   time.Duration
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-direct", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "authored direct-cohort manifest JSON")
	root := flags.String("root", "", "root containing media and evidence files")
	out := flags.String("out", "", "strict source-neutral inventory JSON")
	snapshot := flags.String("snapshot-at", "", "fixed RFC3339 snapshot time")
	expectedItems := flags.Int("expected-items", 0, "exact predeclared item count")
	maxBytes := flags.Int64("max-bytes", 0, "maximum aggregate media and evidence bytes")
	maxWall := flags.Duration("max-wall-time", 0, "maximum local capture time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshot)
	if err != nil || *manifestPath == "" || *root == "" || *out == "" || *expectedItems <= 0 || *maxBytes <= 0 || *maxWall <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-direct: --manifest, --root, --out, valid --snapshot-at, positive --expected-items, and positive ceilings are required")
		return 2
	}
	opts := options{manifestPath: *manifestPath, root: *root, out: *out, snapshotAt: snapshotAt.UTC(), expectedItems: *expectedItems, maxBytes: *maxBytes, maxWallTime: *maxWall}
	started := time.Now()
	inv, err := freeze(opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-direct:", err)
		return 1
	}
	if time.Since(started) > opts.maxWallTime {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-direct: wall-time ceiling exceeded")
		return 1
	}
	if err := writeJSON(opts.out, inv); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-direct: write:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-direct: froze %d locally licensed cases (%d role captures)\n", len(inv.Cases), len(inv.Captures))
	return 0
}

func freeze(opts options) (fillercorpus.Inventory, error) {
	started := time.Now()
	file, err := os.Open(opts.manifestPath)
	if err != nil {
		return fillercorpus.Inventory{}, err
	}
	defer func() { _ = file.Close() }()
	var source manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return fillercorpus.Inventory{}, fmt.Errorf("decode manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fillercorpus.Inventory{}, fmt.Errorf("decode manifest: trailing JSON value")
	}
	if source.SchemaVersion != directManifestSchema || !directID.MatchString(source.Authority) || !directID.MatchString(source.Cohort) || len(source.Cases) != opts.expectedItems {
		return fillercorpus.Inventory{}, fmt.Errorf("manifest schema, cohort, or exact item count is invalid")
	}
	if err := validateRoleQuotas(source.RoleQuotas, opts.expectedItems); err != nil {
		return fillercorpus.Inventory{}, err
	}
	root, err := filepath.EvalSymlinks(opts.root)
	if err != nil {
		return fillercorpus.Inventory{}, fmt.Errorf("resolve root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return fillercorpus.Inventory{}, err
	}

	roleCounts := map[string]int{}
	roleBytes := map[string]int64{}
	seen := map[string]struct{}{}
	seenMedia := map[string]string{}
	inv := fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: opts.snapshotAt}
	authority := directAuthorityPrefix + source.Authority
	var totalBytes int64
	for _, authored := range source.Cases {
		if !directID.MatchString(authored.ItemID) || strings.TrimSpace(authored.Title) == "" || len(authored.RoleHints) == 0 || strings.TrimSpace(authored.RoleHints[0]) == "" || len(authored.Creator) == 0 || slices.Contains(authored.Creator, "") || strings.TrimSpace(authored.Campaign) == "" || strings.TrimSpace(authored.SourceFamily) == "" || len(authored.RightsAssertions) == 0 || slices.Contains(authored.RightsAssertions, "") || strings.TrimSpace(authored.MIMEType) == "" {
			return fillercorpus.Inventory{}, fmt.Errorf("case %q has incomplete authored identity", authored.ItemID)
		}
		if _, duplicate := seen[authored.ItemID]; duplicate {
			return fillercorpus.Inventory{}, fmt.Errorf("duplicate item %q", authored.ItemID)
		}
		seen[authored.ItemID] = struct{}{}
		role := authored.RoleHints[0]
		roleCounts[role]++

		media, mediaBytes, mediaDigest, err := hashInside(root, authored.MediaPath)
		if err != nil {
			return fillercorpus.Inventory{}, fmt.Errorf("case %q media: %w", authored.ItemID, err)
		}
		if prior := seenMedia[mediaDigest]; prior != "" {
			return fillercorpus.Inventory{}, fmt.Errorf("case %q duplicates media bytes from %q", authored.ItemID, prior)
		}
		seenMedia[mediaDigest] = authored.ItemID
		totalBytes += mediaBytes
		roleBytes[role] += mediaBytes
		evidence := make([]fillercorpus.InventoryEvidence, 0, len(authored.Evidence))
		for _, authoredEvidence := range authored.Evidence {
			_, size, digest, err := hashInside(root, authoredEvidence.Path)
			if err != nil {
				return fillercorpus.Inventory{}, fmt.Errorf("case %q evidence: %w", authored.ItemID, err)
			}
			totalBytes += size
			evidence = append(evidence, fillercorpus.InventoryEvidence{Kind: authoredEvidence.Kind, Path: authoredEvidence.Path, Bytes: size, SHA256: digest})
		}
		if totalBytes > opts.maxBytes {
			return fillercorpus.Inventory{}, fmt.Errorf("aggregate bytes exceed ceiling %d", opts.maxBytes)
		}
		identity, err := json.Marshal(struct {
			Authored manifestCase                     `json:"authored"`
			Media    string                           `json:"mediaSha256"`
			Evidence []fillercorpus.InventoryEvidence `json:"evidence"`
		}{authored, mediaDigest, evidence})
		if err != nil {
			return fillercorpus.Inventory{}, err
		}
		captureID := fillercorpus.NewCaptureID(authority, source.Cohort, role)
		inv.Cases = append(inv.Cases, fillercorpus.InventoryCase{
			CaseID: fillercorpus.CaseID(authority, authored.ItemID), CaptureIDs: []string{captureID}, Authority: authority,
			ItemID: authored.ItemID, Title: authored.Title, RoleHints: append([]string(nil), authored.RoleHints...), Creator: append([]string(nil), authored.Creator...), Campaign: authored.Campaign, SourceFamily: authored.SourceFamily, Date: authored.Date,
			LicenseURL: authored.LicenseURL, RightsAssertions: append([]string(nil), authored.RightsAssertions...), ItemURL: authored.ItemURL,
			MetadataRetrievedAt: opts.snapshotAt, MetadataSHA256: fillercorpus.InventorySHA256(identity), Evidence: evidence,
			Representation: fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportLocal, Name: filepath.Base(media), Path: authored.MediaPath, MIMEType: authored.MIMEType, Bytes: mediaBytes, SHA256: mediaDigest},
		})
	}
	if len(source.RoleQuotas) == 0 || len(roleCounts) != len(source.RoleQuotas) {
		return fillercorpus.Inventory{}, fmt.Errorf("role quotas do not cover the cohort")
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed > opts.maxWallTime.Milliseconds() {
		return fillercorpus.Inventory{}, fmt.Errorf("wall-time ceiling exceeded")
	}
	for role, quota := range source.RoleQuotas {
		if quota <= 0 || roleCounts[role] != quota {
			return fillercorpus.Inventory{}, fmt.Errorf("role %q has %d cases; want %d", role, roleCounts[role], quota)
		}
		inv.Captures = append(inv.Captures, fillercorpus.Capture{
			CaptureID: fillercorpus.NewCaptureID(authority, source.Cohort, role), Transport: fillercorpus.TransportLocal,
			Authority: authority, Collection: source.Cohort, RoleHint: role, SnapshotAt: opts.snapshotAt,
			MaxPredictedMediaBytes: opts.maxBytes, PredictedMediaBytes: roleBytes[role], MaxWallTimeMS: opts.maxWallTime.Milliseconds(), WallTimeMS: elapsed,
		})
	}
	slices.SortFunc(inv.Captures, func(a, b fillercorpus.Capture) int { return strings.Compare(a.CaptureID, b.CaptureID) })
	slices.SortFunc(inv.Cases, func(a, b fillercorpus.InventoryCase) int { return strings.Compare(a.CaseID, b.CaseID) })
	if failures := fillercorpus.ValidateInventory(inv); len(failures) != 0 {
		return fillercorpus.Inventory{}, fmt.Errorf("invalid inventory: %s", strings.Join(failures, "; "))
	}
	return inv, nil
}

func validateRoleQuotas(got map[string]int, expectedItems int) error {
	if len(got) == 0 {
		return fmt.Errorf("at least one role quota is required")
	}
	total := 0
	for role, quota := range got {
		if !filleradmission.KnownRole(role) {
			return fmt.Errorf("role quota %q is not a known corpus role", role)
		}
		if quota <= 0 {
			return fmt.Errorf("role %q quota must be positive", role)
		}
		total += quota
	}
	if total != expectedItems {
		return fmt.Errorf("role quotas total %d; want exact item count %d", total, expectedItems)
	}
	return nil
}

func hashInside(root, relative string) (string, int64, string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || filepath.Clean(relative) != relative || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", 0, "", fmt.Errorf("unsafe relative path %q", relative)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", 0, "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", 0, "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", 0, "", fmt.Errorf("path escapes declared root")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", 0, "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", 0, "", fmt.Errorf("path is not a non-empty regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, "", err
	}
	return resolved, info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-direct-*")
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
