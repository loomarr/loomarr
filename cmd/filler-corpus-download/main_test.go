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
	if err := validateSourceURL("clip", "https://example.com/download/clip/video.mp4"); err == nil {
		t.Fatal("non-Archive URL was accepted")
	}
}

func TestValidateSourceURLRejectsMutableOrCredentialedArchiveURL(t *testing.T) {
	for _, rawURL := range []string{
		"https://user:secret@archive.org/download/clip/video.mp4",
		"https://archive.org/download/clip/video.mp4?download=1",
		"https://archive.org/download/clip/video.mp4#fragment",
	} {
		if err := validateSourceURL("clip", rawURL); err == nil {
			t.Fatalf("unsafe Archive URL %q was accepted", rawURL)
		}
	}
}
