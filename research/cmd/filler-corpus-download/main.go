// Command filler-corpus-download downloads only independently rights-approved
// corpus media under explicit request, item, byte, and image-pixel ceilings.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

const maximumDownloadedImagePixels = fillercorpus.MaximumMaterializedImagePixels

type downloadLedger = fillercorpus.MaterializationLedger
type downloadedCase = fillercorpus.MaterializedCase

type plannedDownload struct {
	candidate fillercorpus.InventoryCase
	approval  fillercorpus.RightsDecision
	path      string
}

type options struct {
	inventoryPath, approvalsPath, outputDir, ledgerPath, userAgent string
	profile, quarantinePurpose, processorID, processorTermsSHA256  string
	inventorySHA256                                                string
	generatedAt                                                    time.Time
	maxRequests, maxItems                                          int
	maxBytes, maxImagePixels                                       int64
	delay                                                          time.Duration
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-download", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "frozen source inventory JSON")
	approvalsPath := flags.String("rights-approvals", "", "independent rights decisions JSONL")
	outputDir := flags.String("out-dir", "", "private corpus media directory")
	ledgerPath := flags.String("ledger", "", "content-addressed download ledger JSON")
	userAgent := flags.String("user-agent", "", "descriptive source User-Agent with contact")
	generatedAtText := flags.String("generated-at", "", "ledger generation time in RFC3339 format")
	maxRequests := flags.Int("max-requests", 0, "hard HTTP request ceiling")
	maxItems := flags.Int("max-items", 0, "hard approved item ceiling")
	maxBytes := flags.Int64("max-bytes", 0, "hard approved media-byte ceiling")
	maxImagePixels := flags.Int64("max-image-pixels", maximumDownloadedImagePixels, "hard decoded-image pixel ceiling")
	delay := flags.Duration("delay", time.Second, "minimum delay between HTTP requests")
	profile := flags.String("profile", "", "required rights profile: quarantine, development, or certification")
	quarantinePurpose := flags.String("quarantine-purpose", "", "required quarantine acquisition purpose")
	processorID := flags.String("processor-id", "", "exact approved inference processor identifier (certification only)")
	processorTermsSHA256 := flags.String("processor-terms-sha256", "", "SHA-256 of approved processor terms snapshot (certification only)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	profileValid := fillercorpus.KnownRightsProfile(*profile)
	purposeValid := (*profile == fillercorpus.RightsProfileQuarantine && fillercorpus.KnownQuarantinePurpose(*quarantinePurpose)) || (*profile != fillercorpus.RightsProfileQuarantine && *quarantinePurpose == "")
	certificationIdentityValid := *profile != fillercorpus.RightsProfileCertification || (strings.TrimSpace(*processorID) != "" && fillercorpus.IsSHA256(*processorTermsSHA256))
	if *inventoryPath == "" || *approvalsPath == "" || *outputDir == "" || *ledgerPath == "" || *userAgent == "" || *generatedAtText == "" || *maxRequests <= 0 || *maxItems <= 0 || *maxBytes <= 0 || *maxImagePixels <= 0 || *maxImagePixels > maximumDownloadedImagePixels || *delay < 500*time.Millisecond || !profileValid || !purposeValid || !certificationIdentityValid {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: inventory, rights approvals, private output, ledger, identified User-Agent, generation time, explicit quarantine/development/certification profile, positive ceilings, <=50m image pixels, >=500ms delay, and certification processor identity are required")
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
		profile: *profile, quarantinePurpose: *quarantinePurpose, processorID: *processorID, processorTermsSHA256: *processorTermsSHA256,
		maxRequests: *maxRequests, maxItems: *maxItems, maxBytes: *maxBytes,
		maxImagePixels: *maxImagePixels, delay: *delay,
	}
	inv, inventorySHA256, err := readInventory(opts.inventoryPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: read inventory:", err)
		return 1
	}
	opts.inventorySHA256 = inventorySHA256
	approvals, err := readStrictJSONL[fillercorpus.RightsDecision](opts.approvalsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: read approvals:", err)
		return 1
	}
	plan, err := planDownloads(inv, approvals, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download:", err)
		return 1
	}
	if err := requireNewLedger(opts.ledgerPath); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download:", err)
		return 1
	}
	ledger, err := executeDownloadsWithAccounting(context.Background(), &http.Client{Timeout: 5 * time.Minute}, plan, opts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download:", err)
		return 1
	}
	ledger.InventorySHA256 = inventorySHA256
	var published any = ledger
	if opts.profile == fillercorpus.RightsProfileQuarantine {
		quarantine := quarantineDownloadLedger(ledger)
		if err := fillercorpus.ValidateQuarantineDownloadLedger(inv, inventorySHA256, quarantine); err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-download: validate ledger:", err)
			return 1
		}
		published = quarantine
	} else if err := fillercorpus.ValidateMaterializationLedger(ledger, inv, inventorySHA256); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: validate ledger:", err)
		return 1
	}
	if err := writeImmutableDownloadLedger(opts.ledgerPath, published); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-download: write ledger:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-download: locked %d files (%d bytes) in %d requests\n", len(ledger.Cases), ledger.Bytes, ledger.RequestsUsed)
	return 0
}

func validateDecisionProfile(approval fillercorpus.RightsDecision, opts options) error {
	switch opts.profile {
	case fillercorpus.RightsProfileQuarantine:
		if approval.HoldoutContract != nil || approval.QuarantineContract == nil || approval.Redistributable {
			return fmt.Errorf("decision is not restricted to quarantine")
		}
		reasons := fillercorpus.QuarantineAcquisitionHoldReasons(approval.QuarantineContract)
		if len(reasons) != 0 {
			return fmt.Errorf("decision lacks exact quarantine authority")
		}
		if approval.Decision == "approved" && len(approval.QuarantineContract.HoldReasons) != 0 {
			return fmt.Errorf("approval lacks exact quarantine authority")
		}
		if approval.Decision == "held" && len(approval.QuarantineContract.HoldReasons) == 0 {
			return fmt.Errorf("held quarantine decision has no hold reasons")
		}
		if approval.QuarantineContract.Purpose != opts.quarantinePurpose {
			return fmt.Errorf("decision is bound to quarantine purpose %q; want %q", approval.QuarantineContract.Purpose, opts.quarantinePurpose)
		}
	case fillercorpus.RightsProfileCertification:
		if approval.QuarantineContract != nil || approval.HoldoutContract == nil {
			return fmt.Errorf("decision lacks a certification holdout contract")
		}
		contract := approval.HoldoutContract
		reasons := fillercorpus.HoldoutRightsHoldReasons(contract, opts.generatedAt)
		if len(reasons) != 0 || contract.ProcessorID != opts.processorID || contract.ProcessorTermsSHA256 != opts.processorTermsSHA256 {
			return fmt.Errorf("decision lacks the exact certification holdout authority")
		}
		if approval.Decision == "approved" && len(contract.HoldReasons) != 0 {
			return fmt.Errorf("approval lacks the exact certification holdout authority")
		}
	case fillercorpus.RightsProfileDevelopment:
		if approval.QuarantineContract != nil || approval.HoldoutContract != nil || approval.Redistributable != (approval.Decision == "approved") {
			return fmt.Errorf("decision is not exact development authority")
		}
	default:
		return fmt.Errorf("unknown rights profile %q", opts.profile)
	}
	return nil
}

type requestCounter struct {
	mu        sync.Mutex
	max, used int
}

func (counter *requestCounter) consume() error {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if counter.max <= 0 || counter.used >= counter.max {
		return fmt.Errorf("request ceiling exhausted")
	}
	counter.used++
	return nil
}

func (counter *requestCounter) count() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.used
}

type accountingTransport struct {
	next    http.RoundTripper
	counter *requestCounter
}

func (transport accountingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := transport.counter.consume(); err != nil {
		return nil, err
	}
	return transport.next.RoundTrip(request)
}

func executeDownloadsWithAccounting(ctx context.Context, client *http.Client, plan []plannedDownload, opts options) (downloadLedger, error) {
	if client == nil {
		client = &http.Client{}
	}
	accounted := *client
	next := accounted.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	counter := &requestCounter{max: opts.maxRequests}
	accounted.Transport = accountingTransport{next: next, counter: counter}
	ledger, err := executeDownloads(ctx, &accounted, plan, opts)
	if err != nil {
		return downloadLedger{}, err
	}
	ledger.RequestsUsed = counter.count()
	return ledger, nil
}

func quarantineDownloadLedger(materialized downloadLedger) fillercorpus.DownloadLedger {
	ledger := fillercorpus.DownloadLedger{
		SchemaVersion: fillercorpus.DownloadLedgerSchemaVersion,
		Profile:       fillercorpus.RightsProfileQuarantine, InventorySHA256: materialized.InventorySHA256,
		GeneratedAt: materialized.GeneratedAt, MaxRequests: materialized.MaxRequests,
		RequestsUsed: materialized.RequestsUsed, MaxItems: materialized.MaxItems,
		MaxBytes: materialized.MaxBytes, Bytes: materialized.Bytes,
		Cases: make([]fillercorpus.DownloadCase, 0, len(materialized.Cases)),
	}
	for _, item := range materialized.Cases {
		ledger.Cases = append(ledger.Cases, fillercorpus.DownloadCase{
			CaseID: item.CaseID, Authority: item.Authority, ItemID: item.ItemID,
			LicenseURL: item.LicenseURL, ItemURL: item.ItemURL, MetadataURL: item.MetadataURL,
			MetadataRetrievedAt: item.MetadataRetrievedAt, MetadataSHA256: item.MetadataSHA256,
			Representation: item.Representation, LocalFile: item.LocalFile, ContentSHA256: item.ContentSHA256,
			Approval: item.Approval, VerifiedAt: item.VerifiedAt,
		})
	}
	return ledger
}

func readStrictJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var values []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var value T
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			if err == nil {
				err = fmt.Errorf("trailing JSON value")
			}
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		values = append(values, value)
	}
	return values, scanner.Err()
}

func requireNewLedger(path string) error {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return fmt.Errorf("ledger output already exists")
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("inspect ledger output: %w", err)
	}
}

func writeImmutableDownloadLedger(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-corpus-ledger-*")
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
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable download ledger: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}
