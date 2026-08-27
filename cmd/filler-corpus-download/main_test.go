package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func downloadableInventory(retrieved time.Time, id, license string) fillercorpus.Inventory {
	authority := "loc.gov/national-screening-room"
	captureID := fillercorpus.NewCaptureID(authority, "", "commercial")
	return fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: retrieved, Captures: []fillercorpus.Capture{{CaptureID: captureID, Transport: fillercorpus.TransportHTTPS, Authority: authority, RoleHint: "commercial", SnapshotAt: retrieved, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 2048, ResponseBytes: 10, MaxPredictedMediaBytes: 2048, PredictedMediaBytes: 1024, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []fillercorpus.InventoryCase{{CaseID: fillercorpus.CaseID(authority, id), CaptureID: captureID, Authority: authority, ItemID: id, Title: "Clip", RoleHints: []string{"commercial"}, LicenseURL: license, RightsAssertions: []string{"review required"}, ItemURL: "https://www.loc.gov/item/" + id, MetadataURL: "https://www.loc.gov/item/" + id + "/?fo=json", MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved, AllowedMediaHosts: []string{"tile.loc.gov"}, Representation: fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportHTTPS, Name: id + ".mp4", URL: "https://tile.loc.gov/" + id + ".mp4?download=1", MIMEType: "video/mp4", Bytes: 1024}}}}
}

func approvalFor(inv fillercorpus.Inventory, retrieved time.Time) fillercorpus.RightsDecision {
	c := inv.Cases[0]
	return fillercorpus.RightsDecision{InventorySHA256: strings.Repeat("f", 64), CaseID: c.CaseID, CaptureID: c.CaptureID, Authority: c.Authority, ItemID: c.ItemID, MetadataSHA256: c.MetadataSHA256, ReviewerID: "rights-reviewer", ReviewedAt: retrieved.Add(time.Minute), Decision: "approved", Basis: "item license and source reviewed", Redistributable: true}
}

func TestPlanDownloadsSkipsRightsApprovedLocalMedia(t *testing.T) {
	retrieved := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	inv := downloadableInventory(retrieved, "remote", "")
	local := inv.Cases[0]
	local.ItemID = "local"
	local.CaseID = fillercorpus.CaseID(local.Authority, local.ItemID)
	local.CaptureID = fillercorpus.NewCaptureID(local.Authority, "direct", "commercial")
	local.Representation = fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportLocal, Name: "local.mp4", Path: "media/local.mp4", MIMEType: "video/mp4", Bytes: 10, SHA256: strings.Repeat("b", 64)}
	local.AllowedMediaHosts = nil
	local.ItemURL = ""
	local.MetadataURL = ""
	local.Evidence = []fillercorpus.InventoryEvidence{{Kind: "rights", Path: "evidence/rights.txt", Bytes: 1, SHA256: strings.Repeat("c", 64)}, {Kind: "provenance", Path: "evidence/provenance.txt", Bytes: 1, SHA256: strings.Repeat("d", 64)}}
	inv.Captures = append(inv.Captures, fillercorpus.Capture{CaptureID: local.CaptureID, Transport: fillercorpus.TransportLocal, Authority: local.Authority, Collection: "direct", RoleHint: "commercial", SnapshotAt: retrieved, MaxPredictedMediaBytes: 10, PredictedMediaBytes: 10, MaxWallTimeMS: 1000})
	inv.Cases = append(inv.Cases, local)
	remoteApproval := approvalFor(inv, retrieved)
	localApproval := remoteApproval
	localApproval.CaseID, localApproval.CaptureID, localApproval.ItemID = local.CaseID, local.CaptureID, local.ItemID
	opts := options{inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(2 * time.Minute), maxItems: 2, maxBytes: 4096, outputDir: t.TempDir()}
	plan, err := planDownloads(inv, []fillercorpus.RightsDecision{remoteApproval, localApproval}, opts)
	if err != nil || len(plan) != 1 || plan[0].candidate.ItemID != "remote" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}

func planOptions(retrieved time.Time) options {
	return options{outputDir: "/tmp/corpus", inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(2 * time.Minute), maxItems: 1, maxBytes: 1024}
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

func TestRedirectPolicyRejectsBeforeFollowingUnallowlistedHost(t *testing.T) {
	policy := redirectPolicy([]string{"archive.org", ".archive.org"})
	allowed, _ := url.Parse("https://ia801.example.archive.org/file.mp4")
	if err := policy(&http.Request{URL: allowed}, nil); err != nil {
		t.Fatalf("allowed redirect: %v", err)
	}
	outside, _ := url.Parse("https://attacker.invalid/file.mp4")
	if err := policy(&http.Request{URL: outside}, nil); err == nil {
		t.Fatal("outside redirect accepted")
	}
	credentialed, _ := url.Parse("https://user:secret@archive.org/file.mp4")
	if err := policy(&http.Request{URL: credentialed}, nil); err == nil {
		t.Fatal("credentialed redirect accepted")
	}
}
