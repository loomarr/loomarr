// Command filler-reference-audit creates a deterministic, read-only pre-screen of the
// rights-locked development corpus. It never edits or copies media.
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
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreference"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-reference-audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "locked 300-case development manifest")
	packetsPath := flags.String("packets", "", "content-bound packet JSONL")
	mappingPath := flags.String("mapping", "", "reviewer-product mapping artifact")
	downloadLedgerPath := flags.String("download-ledger", "", "content-bound acquisition ledger")
	contentReviewPath := flags.String("content-review", "", "manifest-bound negative content review")
	sourceRoot := flags.String("source-root", "", "root containing the acquired source files")
	outputPath := flags.String("output", "", "new immutable audit JSON")
	generatedText := flags.String("generated-at", "", "fixed RFC3339 audit time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedText)
	if err != nil || *manifestPath == "" || *packetsPath == "" || *mappingPath == "" || *downloadLedgerPath == "" || *contentReviewPath == "" || *sourceRoot == "" || *outputPath == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "filler-reference-audit: manifest, packets, mapping, download-ledger, content-review, source-root, output, and fixed generated-at are required")
		return 2
	}
	manifestRaw, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fail(stderr, err)
	}
	packetsRaw, err := os.ReadFile(*packetsPath)
	if err != nil {
		return fail(stderr, err)
	}
	mappingRaw, err := os.ReadFile(*mappingPath)
	if err != nil {
		return fail(stderr, err)
	}
	downloadLedgerRaw, err := os.ReadFile(*downloadLedgerPath)
	if err != nil {
		return fail(stderr, err)
	}
	contentReviewRaw, err := os.ReadFile(*contentReviewPath)
	if err != nil {
		return fail(stderr, err)
	}
	downloads, err := fillerreference.DecodeDownloadLedger(downloadLedgerRaw)
	if err != nil {
		return fail(stderr, fmt.Errorf("download ledger: %w", err))
	}
	if err := validateAcquiredSources(*sourceRoot, downloads); err != nil {
		return fail(stderr, err)
	}
	audit, err := fillerreference.BuildAudit(fillerreference.RawAuditInputs{
		Manifest: manifestRaw, Packets: packetsRaw, Mapping: mappingRaw,
		DownloadLedger: downloadLedgerRaw, ContentReview: contentReviewRaw,
	}, generatedAt)
	if err != nil {
		return fail(stderr, err)
	}
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	if err := publish(*outputPath, append(data, '\n')); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "filler-reference-audit: %d candidates, %d holds, %d excluded across %d bound cases\n",
		audit.Summary.Candidates, audit.Summary.Holds, audit.Summary.Excluded, audit.Summary.Cases)
	return 0
}

// validateAcquiredSources reads each opened file once and derives both its byte count
// and SHA-256 from that same stream. Resolving the root and each source prevents a
// content-addressed ledger entry from escaping through a symlink.
func validateAcquiredSources(root string, downloads fillerreference.DownloadLedger) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return fmt.Errorf("absolute source root: %w", err)
	}
	for _, item := range downloads.Cases {
		relative := filepath.Clean(filepath.FromSlash(item.LocalFile))
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("case %q source path escapes source root", item.CaseID)
		}
		path, err := filepath.EvalSymlinks(filepath.Join(realRoot, relative))
		if err != nil {
			return fmt.Errorf("case %q resolve acquired source: %w", item.CaseID, err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("case %q absolute acquired source: %w", item.CaseID, err)
		}
		contained, err := filepath.Rel(realRoot, path)
		if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
			return fmt.Errorf("case %q source path escapes source root", item.CaseID)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("case %q open acquired source: %w", item.CaseID, err)
		}
		info, statErr := file.Stat()
		hash := sha256.New()
		readBytes, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if statErr != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("case %q acquired source is not a regular file", item.CaseID)
		}
		if readErr != nil {
			return fmt.Errorf("case %q hash acquired source: %w", item.CaseID, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("case %q close acquired source: %w", item.CaseID, closeErr)
		}
		if readBytes != item.Representation.Bytes || hex.EncodeToString(hash.Sum(nil)) != item.ContentSHA256 {
			return fmt.Errorf("case %q acquired source bytes do not match the ledger", item.CaseID)
		}
	}
	return nil
}

func publish(path string, data []byte) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-reference-audit-*")
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
	if err := temp.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable audit: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "filler-reference-audit:", err)
	return 1
}
