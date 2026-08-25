package main

import (
	"strings"
	"testing"
	"time"
)

func TestPlanDownloadsRequiresMetadataBoundRightsReview(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{Source: "archive.org", Cases: []inventoryCandidate{{
		Identifier: "soda-ad", LicenseURL: "https://creativecommons.org/publicdomain/mark/1.0/",
		MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved,
		File: sourceFile{Name: "soda.mp4", URL: "https://archive.org/download/soda-ad/soda.mp4", Bytes: 1024},
	}}}
	approval := rightsApproval{
		InventorySHA256: strings.Repeat("f", 64), Identifier: "soda-ad", MetadataSHA256: strings.Repeat("a", 64), ReviewerID: "rights-reviewer",
		ReviewedAt: retrieved.Add(time.Minute), Decision: "approved", Basis: "item license and source reviewed",
		Redistributable: true,
	}
	opts := options{outputDir: "/tmp/corpus", inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(2 * time.Minute), maxItems: 1, maxBytes: 1024}
	plan, err := planDownloads(inv, []rightsApproval{approval}, opts)
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	approval.MetadataSHA256 = strings.Repeat("b", 64)
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("stale rights review was accepted")
	}
	approval.MetadataSHA256 = strings.Repeat("a", 64)
	approval.InventorySHA256 = strings.Repeat("e", 64)
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("approval from a different inventory was accepted")
	}
}

func TestPlanDownloadsRequiresAttributionForBY(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{Source: "archive.org", Cases: []inventoryCandidate{{
		Identifier: "by-ad", LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
		MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved,
		File: sourceFile{Name: "by.mp4", URL: "https://archive.org/download/by-ad/by.mp4", Bytes: 1024},
	}}}
	approval := rightsApproval{
		InventorySHA256: strings.Repeat("f", 64), Identifier: "by-ad", MetadataSHA256: strings.Repeat("a", 64), ReviewerID: "rights-reviewer",
		ReviewedAt: retrieved, Decision: "approved", Basis: "CC BY source reviewed", Redistributable: true,
	}
	if _, err := planDownloads(inv, []rightsApproval{approval}, options{outputDir: "/tmp/corpus", inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(time.Minute), maxItems: 1, maxBytes: 1024}); err == nil {
		t.Fatal("attribution-free CC BY approval was accepted")
	}
}

func TestPlanDownloadsRequiresExplicitRedistributionApproval(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{Source: "archive.org", Cases: []inventoryCandidate{{
		Identifier: "held-rights", LicenseURL: "https://creativecommons.org/publicdomain/mark/1.0/",
		MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved,
		File: sourceFile{Name: "clip.mp4", URL: "https://archive.org/download/held-rights/clip.mp4", Bytes: 1024},
	}}}
	approval := rightsApproval{
		InventorySHA256: strings.Repeat("f", 64), Identifier: "held-rights", MetadataSHA256: strings.Repeat("a", 64), ReviewerID: "rights-reviewer",
		ReviewedAt: retrieved, Decision: "approved", Basis: "source reviewed",
	}
	if _, err := planDownloads(inv, []rightsApproval{approval}, options{outputDir: "/tmp/corpus", inventorySHA256: strings.Repeat("f", 64), generatedAt: retrieved.Add(time.Minute), maxItems: 1, maxBytes: 1024}); err == nil {
		t.Fatal("non-redistributable approval was accepted")
	}
}

func TestPlanDownloadsRejectsUnsafeInventoryIdentity(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{Source: "archive.org", Cases: []inventoryCandidate{{
		Identifier: "../escape", MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved,
		File: sourceFile{Name: "clip.mp4", URL: "https://archive.org/download/..%2Fescape/clip.mp4", Bytes: 1024},
	}}}
	if _, err := planDownloads(inv, nil, options{outputDir: "/tmp/corpus", generatedAt: retrieved, maxItems: 1, maxBytes: 1024}); err == nil {
		t.Fatal("unsafe inventory identifier was accepted")
	}
}

func TestValidateSourceURLRejectsNonArchiveHost(t *testing.T) {
	if err := validateSourceURL("archive.org", "clip", "https://example.com/download/clip/video.mp4"); err == nil {
		t.Fatal("non-Archive URL was accepted")
	}
}

func TestPlanDownloadsSupportsDVIDSOnlyWithInstitutionalCredit(t *testing.T) {
	retrieved := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
	digest := strings.Repeat("f", 64)
	metadataDigest := strings.Repeat("a", 64)
	rightsDigest := strings.Repeat("b", 64)
	inv := inventory{Source: "dvids", Cases: []inventoryCandidate{{
		Identifier: "dvids-video-123", LicenseURL: "https://www.dvidshub.net/about/copyright", MetadataRetrievedAt: retrieved, MetadataSHA256: metadataDigest,
		RightsPageSHA256: rightsDigest, RightsPageRetrievedAt: retrieved.Add(2 * time.Minute),
		File: sourceFile{Name: "DOD_123-720.mp4", URL: "https://d34w7g4gy10iej.cloudfront.net/video/2608/DOD_123/DOD_123-720.mp4", Format: "video/mp4", Source: "dvids", Bytes: 1024},
	}}}
	approval := rightsApproval{
		InventorySHA256: digest, Identifier: "dvids-video-123", MetadataSHA256: metadataDigest, ReviewerID: "rights-reviewer", ReviewedAt: retrieved.Add(3 * time.Minute),
		Decision: "approved", Basis: "DVIDS item page marks the federal work public domain.", Redistributable: true,
	}
	opts := options{outputDir: "/tmp/corpus", inventorySHA256: digest, generatedAt: retrieved.Add(time.Hour), maxItems: 1, maxBytes: 2048}
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("DVIDS approval without rights-page binding was accepted")
	}
	approval.RightsPageSHA256 = strings.Repeat("c", 64)
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("DVIDS approval tied to different rights-page evidence was accepted")
	}
	approval.RightsPageSHA256 = rightsDigest
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("DVIDS approval without requested institutional credit was accepted")
	}
	approval.RequiredCredit = "Defense Media Activity / DVIDS"
	approval.ReviewedAt = retrieved.Add(time.Minute)
	if _, err := planDownloads(inv, []rightsApproval{approval}, opts); err == nil {
		t.Fatal("DVIDS review completed before item-page evidence retrieval was accepted")
	}
	approval.ReviewedAt = retrieved.Add(3 * time.Minute)
	plan, err := planDownloads(inv, []rightsApproval{approval}, opts)
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}
