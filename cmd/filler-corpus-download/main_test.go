package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func downloadableInventory(retrieved time.Time, id, license string) fillercorpus.Inventory {
	authority := "loc.gov/national-screening-room"
	captureID := fillercorpus.NewCaptureID(authority, "", "commercial")
	return fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: retrieved, Captures: []fillercorpus.Capture{{CaptureID: captureID, Transport: fillercorpus.TransportHTTPS, Authority: authority, RoleHint: "commercial", SnapshotAt: retrieved, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 2048, ResponseBytes: 10, MaxPredictedMediaBytes: 2048, PredictedMediaBytes: 1024, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []fillercorpus.InventoryCase{{CaseID: fillercorpus.CaseID(authority, id), CaptureIDs: []string{captureID}, Authority: authority, ItemID: id, Title: "Clip", RoleHints: []string{"commercial"}, LicenseURL: license, RightsAssertions: []string{"review required"}, ItemURL: "https://www.loc.gov/item/" + id, MetadataURL: "https://www.loc.gov/item/" + id + "/?fo=json", MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved, AllowedMediaHosts: []string{"tile.loc.gov"}, Representation: fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportHTTPS, Name: id + ".mp4", URL: "https://tile.loc.gov/" + id + ".mp4?download=1", MIMEType: "video/mp4", Bytes: 1024}}}}
}

func approvalFor(inv fillercorpus.Inventory, retrieved time.Time) fillercorpus.RightsDecision {
	c := inv.Cases[0]
	return fillercorpus.RightsDecision{InventorySHA256: strings.Repeat("f", 64), CaseID: c.CaseID, CaptureIDs: c.CaptureIDs, Authority: c.Authority, ItemID: c.ItemID, MetadataSHA256: c.MetadataSHA256, ReviewerID: "rights-reviewer", ReviewedAt: retrieved.Add(time.Minute), Decision: "approved", Basis: "item license and source reviewed", Redistributable: true}
}

func TestPlanDownloadsSkipsRightsApprovedLocalMedia(t *testing.T) {
	retrieved := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "remote", "")
	local := inv.Cases[0]
	local.ItemID = "local"
	local.CaseID = fillercorpus.CaseID(local.Authority, local.ItemID)
	local.CaptureIDs = []string{fillercorpus.NewCaptureID(local.Authority, "direct", "commercial")}
	local.Representation = fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportLocal, Name: "local.mp4", Path: "media/local.mp4", MIMEType: "video/mp4", Bytes: 10, SHA256: strings.Repeat("b", 64)}
	local.AllowedMediaHosts = nil
	local.ItemURL = ""
	local.MetadataURL = ""
	local.Evidence = []fillercorpus.InventoryEvidence{{Kind: "rights", Path: "evidence/rights.txt", Bytes: 1, SHA256: strings.Repeat("c", 64)}, {Kind: "provenance", Path: "evidence/provenance.txt", Bytes: 1, SHA256: strings.Repeat("d", 64)}}
	inv.Captures = append(inv.Captures, fillercorpus.Capture{CaptureID: local.CaptureIDs[0], Transport: fillercorpus.TransportLocal, Authority: local.Authority, Collection: "direct", RoleHint: "commercial", SnapshotAt: retrieved, MaxPredictedMediaBytes: 10, PredictedMediaBytes: 10, MaxWallTimeMS: 1000})
	inv.Cases = append(inv.Cases, local)
	remoteApproval := approvalFor(inv, retrieved)
	localApproval := remoteApproval
	localApproval.CaseID, localApproval.CaptureIDs, localApproval.ItemID = local.CaseID, local.CaptureIDs, local.ItemID
	opts := options{profile: fillercorpus.RightsProfileDevelopment, inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(2 * time.Minute), maxItems: 2, maxBytes: 4096, outputDir: t.TempDir()}
	plan, err := planDownloads(inv, []fillercorpus.RightsDecision{remoteApproval, localApproval}, opts)
	if err != nil || len(plan) != 1 || plan[0].candidate.ItemID != "remote" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}

func planOptions(retrieved time.Time) options {
	return options{profile: fillercorpus.RightsProfileDevelopment, outputDir: "/tmp/corpus", inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(2 * time.Minute), maxItems: 1, maxBytes: 1024}
}

func TestPlanDownloadsRequiresMetadataBoundRightsReview(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "soda-ad", "https://creativecommons.org/publicdomain/mark/1.0/")
	approval, opts := approvalFor(inv, retrieved), planOptions(retrieved)
	if plan, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err != nil || len(plan) != 1 {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	approval.MetadataSHA256 = strings.Repeat("b", 64)
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err == nil {
		t.Fatal("stale review accepted")
	}
	approval = approvalFor(inv, retrieved)
	approval.InventorySHA256 = strings.Repeat("e", 64)
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err == nil {
		t.Fatal("foreign inventory accepted")
	}
}

func TestPlanDownloadsRequiresAttributionAndRedistribution(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "by-ad", "https://creativecommons.org/licenses/by/4.0/")
	approval, opts := approvalFor(inv, retrieved), planOptions(retrieved)
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err == nil {
		t.Fatal("attribution-free approval accepted")
	}
	approval.RequiredCredit = "Creator, CC BY 4.0"
	approval.Redistributable = false
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err == nil {
		t.Fatal("non-redistributable approval accepted")
	}
}

func TestPlanDownloadsRejectsUnallowlistedMediaHost(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "clip", "https://creativecommons.org/publicdomain/mark/1.0/")
	inv.Cases[0].Representation.URL = "https://example.com/clip.mp4"
	if _, err := planDownloads(inv, nil, planOptions(retrieved)); err == nil {
		t.Fatal("unallowlisted host accepted")
	}
}

func TestPlanDownloadsCertificationRejectsLegacyAndProcessorDrift(t *testing.T) {
	retrieved := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "holdout", "")
	approval := approvalFor(inv, retrieved)
	approval.Redistributable = false
	approval.HoldoutContract = downloadHoldoutContract()
	opts := planOptions(retrieved)
	opts.profile = fillercorpus.RightsProfileCertification
	opts.processorID = approval.HoldoutContract.ProcessorID
	opts.processorTermsSHA256 = approval.HoldoutContract.ProcessorTermsSHA256
	if plan, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err != nil || len(plan) != 1 {
		t.Fatalf("certification plan = %v, %v", plan, err)
	}
	legacy := approval
	legacy.HoldoutContract = nil
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{legacy}, opts); err == nil {
		t.Fatal("schema-v3 approval authorized certification acquisition")
	}
	drifted := opts
	drifted.processorTermsSHA256 = strings.Repeat("f", 64)
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, drifted); err == nil {
		t.Fatal("changed processor terms authorized certification acquisition")
	}
}

func TestPlanDownloadsQuarantineCannotGrantOrInheritDownstreamUse(t *testing.T) {
	retrieved := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "quarantine", "")
	approval := approvalFor(inv, retrieved)
	approval.Redistributable = false
	approval.QuarantineContract = &fillercorpus.QuarantineAcquisitionContract{
		SchemaVersion:  fillercorpus.QuarantineAcquisitionContractSchemaVersion,
		Purpose:        fillercorpus.QuarantinePurposeLocalInspection,
		CopyAndStorage: true, LocalTechnicalInspection: true,
	}
	opts := planOptions(retrieved)
	opts.profile = fillercorpus.RightsProfileQuarantine
	if plan, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, opts); err != nil || len(plan) != 1 {
		t.Fatalf("quarantine plan = %v, %v", plan, err)
	}
	for name, mutate := range map[string]func(*fillercorpus.RightsDecision){
		"provider transfer":  func(value *fillercorpus.RightsDecision) { value.QuarantineContract.ProviderTransfer = true },
		"redistribution":     func(value *fillercorpus.RightsDecision) { value.QuarantineContract.Redistribution = true },
		"corpus preparation": func(value *fillercorpus.RightsDecision) { value.QuarantineContract.CorpusPreparation = true },
		"training":           func(value *fillercorpus.RightsDecision) { value.QuarantineContract.Training = true },
		"catalog ingestion":  func(value *fillercorpus.RightsDecision) { value.QuarantineContract.CatalogIngestion = true },
		"scheduling":         func(value *fillercorpus.RightsDecision) { value.QuarantineContract.Scheduling = true },
		"production":         func(value *fillercorpus.RightsDecision) { value.QuarantineContract.ProductionAdmission = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := approval
			contract := *approval.QuarantineContract
			changed.QuarantineContract = &contract
			mutate(&changed)
			if _, err := planDownloads(inv, []fillercorpus.RightsDecision{changed}, opts); err == nil {
				t.Fatal("broadened quarantine authority was accepted")
			}
		})
	}
	development := opts
	development.profile = fillercorpus.RightsProfileDevelopment
	if _, err := planDownloads(inv, []fillercorpus.RightsDecision{approval}, development); err == nil {
		t.Fatal("quarantine decision authorized development acquisition")
	}
}

func TestExecuteDownloadsRecordsQuarantineProfile(t *testing.T) {
	data := []byte("exact quarantine bytes")
	path := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	item := downloadableInventory(time.Now().UTC(), "quarantine", "").Cases[0]
	item.Representation.Bytes = int64(len(data))
	ledger, err := executeDownloads(t.Context(), &http.Client{}, []plannedDownload{{candidate: item, path: path}}, options{
		profile: fillercorpus.RightsProfileQuarantine, generatedAt: time.Now().UTC(), maxRequests: 1, maxItems: 1, maxBytes: int64(len(data)), outputDir: filepath.Dir(path),
	})
	if err != nil || ledger.SchemaVersion != 2 || ledger.Profile != fillercorpus.RightsProfileQuarantine || ledger.RequestsUsed != 0 || len(ledger.Cases) != 1 {
		t.Fatalf("ledger = %+v, %v", ledger, err)
	}
}

func downloadHoldoutContract() *fillercorpus.HoldoutRightsContract {
	return &fillercorpus.HoldoutRightsContract{
		SchemaVersion: fillercorpus.HoldoutRightsContractSchemaVersion,
		AgreementID:   "agreement-v1", AgreementSHA256: strings.Repeat("a", 64), ScheduleID: "schedule-v1", ScheduleSHA256: strings.Repeat("b", 64),
		SignerAuthorityStatus: fillercorpus.RightsStatusCleared, SignerAuthorityEvidenceSHA256: strings.Repeat("c", 64), ProcessorID: "openrouter/vertex", ProcessorTermsSHA256: strings.Repeat("d", 64),
		Grants:                       fillercorpus.HoldoutRightsGrants{CommercialEvaluation: true, CopyAndStorage: true, TechnicalModification: true, EvidenceExtraction: true, ProviderTransfer: true},
		EmbeddedRights:               fillercorpus.EmbeddedRightsStatus{Music: fillercorpus.RightsStatusNotPresent, PerformersAndVoices: fillercorpus.RightsStatusCleared, StockAndArtwork: fillercorpus.RightsStatusNotPresent, Trademarks: fillercorpus.RightsStatusCleared, PrivacyAndPublicity: fillercorpus.RightsStatusCleared, Locations: fillercorpus.RightsStatusNotPresent},
		EmbeddedRightsEvidenceSHA256: strings.Repeat("e", 64), RedistributionScope: fillercorpus.RedistributionExternalOnly, Territory: fillercorpus.RightsTerritoryWorldwide, Term: fillercorpus.RightsTermPerpetualIrrevocable, Withdrawal: fillercorpus.RightsWithdrawalDefectRetirement,
	}
}

func TestRedirectPolicyRejectsBeforeFollowingUnallowlistedHost(t *testing.T) {
	budget := requestBudget{max: 2}
	policy := redirectPolicy([]string{"archive.org", ".archive.org"}, &budget)
	allowed, _ := url.Parse("https://ia801.example.archive.org/file.mp4")
	if err := policy(&http.Request{URL: allowed}, nil); err != nil {
		t.Fatalf("allowed redirect: %v", err)
	}
	if budget.used != 1 {
		t.Fatalf("allowed redirect used %d requests; want 1", budget.used)
	}
	outside, _ := url.Parse("https://attacker.invalid/file.mp4")
	if err := policy(&http.Request{URL: outside}, nil); err == nil {
		t.Fatal("outside redirect accepted")
	}
	credentialed, _ := url.Parse("https://user:secret@archive.org/file.mp4")
	if err := policy(&http.Request{URL: credentialed}, nil); err == nil {
		t.Fatal("credentialed redirect accepted")
	}
	if budget.used != 1 {
		t.Fatalf("rejected redirects changed request count to %d", budget.used)
	}
	if err := policy(&http.Request{URL: allowed}, nil); err != nil {
		t.Fatalf("second allowed redirect: %v", err)
	}
	if err := policy(&http.Request{URL: allowed}, nil); err == nil {
		t.Fatal("redirect beyond request ceiling accepted")
	}
}

func TestDownloadCountsInitialRequestAndRedirect(t *testing.T) {
	data := []byte("exact media")
	item := downloadableInventory(time.Now().UTC(), "redirect", "").Cases[0]
	item.Representation.Bytes = int64(len(data))
	item.Representation.URL = "https://tile.loc.gov/start.mp4"
	path := filepath.Join(t.TempDir(), "source.mp4")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusFound, Status: "302 Found", Request: request,
				Header: http.Header{"Location": []string{"https://tile.loc.gov/final.mp4"}}, Body: io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK", Request: request, ContentLength: int64(len(data)),
			Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data))),
		}, nil
	})}
	budget := requestBudget{max: 2}
	if _, size, err := download(t.Context(), client, plannedDownload{candidate: item, path: path}, "loomarr-test", &budget); err != nil || size != int64(len(data)) {
		t.Fatalf("download size=%d budget=%+v calls=%d err=%v", size, budget, calls, err)
	}
	if budget.used != 2 || calls != 2 {
		t.Fatalf("budget=%+v calls=%d; want two requests", budget, calls)
	}

	calls = 0
	exhausted := requestBudget{max: 1}
	blockedPath := filepath.Join(t.TempDir(), "blocked.mp4")
	if _, _, err := download(t.Context(), client, plannedDownload{candidate: item, path: blockedPath}, "loomarr-test", &exhausted); err == nil || !strings.Contains(err.Error(), "request ceiling exhausted") {
		t.Fatalf("redirect beyond ceiling error=%v", err)
	}
	if exhausted.used != 1 || calls != 1 {
		t.Fatalf("exhausted budget=%+v calls=%d; redirected request was sent", exhausted, calls)
	}
	if _, err := os.Stat(blockedPath); !os.IsNotExist(err) {
		t.Fatalf("blocked download published a file: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
