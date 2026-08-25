// Command filler-corpus-rights-review prepares a deterministic, non-authorizing
// worksheet from a frozen source inventory. It never downloads media.
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
	"sort"
	"strings"
	"time"
)

var (
	safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	sha256Digest   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type inventory struct {
	SchemaVersion int         `json:"schemaVersion"`
	Source        string      `json:"source"`
	Collection    string      `json:"collection"`
	SnapshotAt    time.Time   `json:"snapshotAt"`
	Cases         []candidate `json:"cases"`
}

type candidate struct {
	Identifier              string     `json:"identifier"`
	Title                   string     `json:"title"`
	Creator                 []string   `json:"creator,omitempty"`
	Date                    string     `json:"date,omitempty"`
	LicenseURL              string     `json:"licenseUrl"`
	Rights                  []string   `json:"rights,omitempty"`
	PossibleCopyrightStatus []string   `json:"possibleCopyrightStatus,omitempty"`
	ItemURL                 string     `json:"itemUrl"`
	MetadataURL             string     `json:"metadataUrl"`
	MetadataRetrievedAt     time.Time  `json:"metadataRetrievedAt"`
	MetadataSHA256          string     `json:"metadataSha256"`
	File                    sourceFile `json:"file"`
}

type sourceFile struct {
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

type worksheet struct {
	SchemaVersion   int         `json:"schemaVersion"`
	InventorySHA256 string      `json:"inventorySha256"`
	Source          string      `json:"source"`
	Collection      string      `json:"collection"`
	SnapshotAt      time.Time   `json:"snapshotAt"`
	PreparedAt      time.Time   `json:"preparedAt"`
	MinItems        int         `json:"minItems"`
	MaxItems        int         `json:"maxItems"`
	Instructions    []string    `json:"instructions"`
	Cases           []reviewRow `json:"cases"`
}

type reviewRow struct {
	Rank                    int        `json:"rank"`
	InventorySHA256         string     `json:"inventorySha256"`
	Identifier              string     `json:"identifier"`
	Title                   string     `json:"title"`
	Creator                 []string   `json:"creator,omitempty"`
	Date                    string     `json:"date,omitempty"`
	LicenseURL              string     `json:"licenseUrl"`
	Rights                  []string   `json:"rights,omitempty"`
	PossibleCopyrightStatus []string   `json:"possibleCopyrightStatus,omitempty"`
	ItemURL                 string     `json:"itemUrl"`
	MetadataURL             string     `json:"metadataUrl"`
	MetadataRetrievedAt     time.Time  `json:"metadataRetrievedAt"`
	MetadataSHA256          string     `json:"metadataSha256"`
	File                    sourceFile `json:"file"`
	ReviewerID              string     `json:"reviewerId"`
	ReviewedAt              string     `json:"reviewedAt"`
	Decision                string     `json:"decision"`
	Basis                   string     `json:"basis"`
	Redistributable         bool       `json:"redistributable"`
	RequiredCredit          string     `json:"requiredCredit,omitempty"`
	Restrictions            []string   `json:"restrictions"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-rights-review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "frozen source inventory JSON")
	outputPath := flags.String("out", "", "non-authorizing rights worksheet JSON")
	preparedAtText := flags.String("prepared-at", "", "worksheet preparation time in RFC3339 format")
	minItems := flags.Int("min-items", 0, "required minimum worksheet cases")
	maxItems := flags.Int("max-items", 0, "hard worksheet case ceiling")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *inventoryPath == "" || *outputPath == "" || *preparedAtText == "" || *minItems <= 0 || *maxItems < *minItems {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: inventory, output, preparation time, and positive min/max item bounds are required")
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: parse --prepared-at:", err)
		return 2
	}
	raw, err := os.ReadFile(*inventoryPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: read inventory:", err)
		return 1
	}
	var inv inventory
	if err := json.Unmarshal(raw, &inv); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: decode inventory:", err)
		return 1
	}
	result, err := prepareWorksheet(inv, sha256Hex(raw), preparedAt, *minItems, *maxItems)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review:", err)
		return 1
	}
	if err := writeJSON(*outputPath, result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-review: write worksheet:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-rights-review: prepared %d inert review rows for inventory %s\n", len(result.Cases), result.InventorySHA256)
	return 0
}

func prepareWorksheet(inv inventory, digest string, preparedAt time.Time, minItems, maxItems int) (worksheet, error) {
	if inv.SchemaVersion != 1 || inv.Source != "archive.org" || inv.Collection == "" || inv.SnapshotAt.IsZero() || preparedAt.Before(inv.SnapshotAt) {
		return worksheet{}, fmt.Errorf("inventory identity or worksheet time is invalid")
	}
	if len(inv.Cases) < minItems {
		return worksheet{}, fmt.Errorf("inventory has %d cases; minimum is %d", len(inv.Cases), minItems)
	}
	cases := append([]candidate(nil), inv.Cases...)
	sort.Slice(cases, func(i, j int) bool {
		return sha256Hex([]byte(digest+"/"+cases[i].Identifier)) < sha256Hex([]byte(digest+"/"+cases[j].Identifier))
	})
	if len(cases) > maxItems {
		cases = cases[:maxItems]
	}
	result := worksheet{
		SchemaVersion: 1, InventorySHA256: digest, Source: inv.Source, Collection: inv.Collection,
		SnapshotAt: inv.SnapshotAt.UTC(), PreparedAt: preparedAt.UTC(), MinItems: minItems, MaxItems: maxItems,
		Instructions: []string{
			"This worksheet is not download authority; blank rows fail closed.",
			"Review the exact item, metadata, selected file, license assertion, rights prose, embedded material, attribution, and non-copyright restrictions.",
			"Set decision to approved only with explicit redistributable=true and a reasoned basis; otherwise set decision to held.",
		},
	}
	seen := map[string]struct{}{}
	for index, item := range cases {
		if !safeIdentifier.MatchString(item.Identifier) || strings.Contains(item.Identifier, "..") || !sha256Digest.MatchString(item.MetadataSHA256) || item.MetadataRetrievedAt.IsZero() || item.File.Bytes <= 0 {
			return worksheet{}, fmt.Errorf("candidate %q has incomplete frozen identity", item.Identifier)
		}
		if _, exists := seen[item.Identifier]; exists {
			return worksheet{}, fmt.Errorf("duplicate candidate %s", item.Identifier)
		}
		seen[item.Identifier] = struct{}{}
		result.Cases = append(result.Cases, reviewRow{
			Rank: index + 1, InventorySHA256: digest, Identifier: item.Identifier, Title: item.Title,
			Creator: item.Creator, Date: item.Date, LicenseURL: item.LicenseURL, Rights: item.Rights,
			PossibleCopyrightStatus: item.PossibleCopyrightStatus, ItemURL: item.ItemURL, MetadataURL: item.MetadataURL,
			MetadataRetrievedAt: item.MetadataRetrievedAt, MetadataSHA256: item.MetadataSHA256, File: item.File,
			Restrictions: []string{},
		})
	}
	return result, nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-rights-review-*")
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
