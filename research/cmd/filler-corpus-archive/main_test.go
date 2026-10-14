package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestParseCandidateRequiresMatchingAllowlistedItemLicense(t *testing.T) {
	raw := []byte(`{
  "metadata":{"identifier":"soda-ad","mediatype":"movies","title":"Soda advert","collection":["classic_tv_commercials"],"licenseurl":"http://creativecommons.org/publicdomain/mark/1.0/"},
  "files":[
    {"name":"master.mp4","format":"MPEG4","source":"original","size":"20000000"},
    {"name":"small.ia.mp4","format":"h.264 IA","source":"derivative","size":"4000000","sha1":"abc","length":"30.0","height":"480"}
  ]
}`)
	got, ok := parseCandidate(defaultBaseURL, "classic_tv_commercials", "soda-ad", "https://creativecommons.org/publicdomain/mark/1.0/", "soda-ad.json", time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC), raw)
	if !ok || got.File.Name != "small.ia.mp4" || got.File.Bytes != 4_000_000 || got.MetadataSHA256 == "" {
		t.Fatalf("candidate = %+v, %v", got, ok)
	}
	if _, ok := parseCandidate(defaultBaseURL, "classic_tv_commercials", "soda-ad", "https://creativecommons.org/licenses/by-nc/4.0/", "soda-ad.json", time.Time{}, raw); ok {
		t.Fatal("search/item license disagreement was accepted")
	}
}

func TestAllowedLicenseRejectsNCAndND(t *testing.T) {
	for _, license := range []string{
		"https://creativecommons.org/licenses/by-nc-sa/4.0/",
		"https://creativecommons.org/licenses/by-nd/4.0/",
		"",
	} {
		if allowedLicense(license) {
			t.Errorf("allowed %q", license)
		}
	}
	for _, license := range []string{
		"http://creativecommons.org/licenses/publicdomain/",
		"https://creativecommons.org/publicdomain/zero/1.0/",
		"https://creativecommons.org/licenses/by/4.0/",
		"https://creativecommons.org/licenses/by-sa/4.0/",
	} {
		if !allowedLicense(license) {
			t.Errorf("rejected %q", license)
		}
	}
}

func TestMetadataFieldsAcceptArchiveStringOrArrayShapes(t *testing.T) {
	if got := stringsFromRaw(json.RawMessage(`"one"`)); len(got) != 1 || got[0] != "one" {
		t.Fatalf("single = %v", got)
	}
	if got := stringsFromRaw(json.RawMessage(`["one","two"]`)); len(got) != 2 {
		t.Fatalf("many = %v", got)
	}
}

func TestPrelingerPilotLaneCarriesBoundedNonAuthorizingEvidence(t *testing.T) {
	retrieved := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	lane := prelingerPilotLane(inventory{
		MaxRequests: 20, RequestsUsed: 2, MaxResponseBytes: 1000, ResponseBytes: 500,
		MaxTotalBytes: 2000, SelectedBytes: 1000, MaxWallTimeMS: 60000, WallTimeMS: 100,
		Cases: []candidate{{
			Identifier: "soda-ad", Title: "Soda ad", ItemURL: "https://archive.org/details/soda-ad",
			MetadataURL: "https://archive.org/metadata/soda-ad", MetadataRetrievedAt: retrieved,
			MetadataSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			LicenseURL:     "http://creativecommons.org/publicdomain/mark/1.0/", Rights: []string{"uploader assertion"},
			File: selectedFile{Name: "soda.mp4", URL: "https://archive.org/download/soda-ad/soda.mp4", Bytes: 1000},
		}},
	}, "commercial")
	if lane.Authority != "archive.org/prelinger" || lane.PredictedMediaBytes != 1000 || len(lane.Cases) != 1 || lane.Cases[0].LicenseURL != "https://creativecommons.org/publicdomain/mark/1.0/" || lane.Cases[0].RoleHints[0] != "commercial" || len(lane.Cases[0].RightsAssertions) != 2 {
		t.Fatalf("lane = %+v", lane)
	}
}

func TestSourceNeutralInventoryEmitsOnlyCurrentSchema(t *testing.T) {
	retrieved := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	legacy := inventory{Collection: "prelinger", SnapshotAt: retrieved, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 1000, ResponseBytes: 500, MaxTotalBytes: 2000, SelectedBytes: 1000, MaxWallTimeMS: 60000, WallTimeMS: 100, SearchSHA256: strings.Repeat("b", 64), SearchRetrievedAt: retrieved, Cases: []candidate{{Identifier: "soda-ad", Title: "Soda ad", Collection: []string{"prelinger"}, ItemURL: "https://archive.org/details/soda-ad", MetadataURL: "https://archive.org/metadata/soda-ad", MetadataRetrievedAt: retrieved, MetadataSHA256: strings.Repeat("a", 64), LicenseURL: "https://creativecommons.org/publicdomain/mark/1.0/", Rights: []string{"public domain"}, File: selectedFile{Name: "soda.mp4", URL: "https://archive.org/download/soda-ad/soda.mp4", Format: "MPEG4", Source: "original", Bytes: 1000}}}}
	got := sourceNeutralInventory(legacy, "commercial")
	if failures := fillercorpus.ValidateInventory(got); len(failures) != 0 {
		t.Fatalf("inventory failures = %v", failures)
	}
	if got.SchemaVersion != fillercorpus.InventorySchemaVersion || got.Cases[0].CaseID != "archive.org/prelinger/soda-ad" || got.Cases[0].Representation.Origin != "original" {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestSourceNeutralInventoryPreservesArchiveCollectionAuthority(t *testing.T) {
	retrieved := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	inv := inventory{Collection: "vhscommercials", SnapshotAt: retrieved, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 1000, ResponseBytes: 500, MaxTotalBytes: 2000, SelectedBytes: 1000, MaxWallTimeMS: 60000, WallTimeMS: 100, SearchSHA256: strings.Repeat("b", 64), SearchRetrievedAt: retrieved, Cases: []candidate{{Identifier: "station-break", Title: "Station break", Collection: []string{"vhscommercials"}, ItemURL: "https://archive.org/details/station-break", MetadataURL: "https://archive.org/metadata/station-break", MetadataRetrievedAt: retrieved, MetadataSHA256: strings.Repeat("a", 64), LicenseURL: "https://creativecommons.org/publicdomain/mark/1.0/", Rights: []string{"public domain"}, File: selectedFile{Name: "break.mp4", URL: "https://archive.org/download/station-break/break.mp4", Format: "MPEG4", Source: "original", Bytes: 1000}}}}

	got := sourceNeutralInventory(inv, "station_id")
	if failures := fillercorpus.ValidateInventory(got); len(failures) != 0 {
		t.Fatalf("inventory failures = %v", failures)
	}
	if got.Cases[0].Authority != "archive.org/vhscommercials" || got.Cases[0].CaseID != "archive.org/vhscommercials/station-break" {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestRunRequiresHardCeilingsAndIdentity(t *testing.T) {
	if code := run(nil, testWriter{t}, testWriter{t}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
}

func TestRunRejectsNonPrelingerPilotOutput(t *testing.T) {
	args := []string{
		"--collection", "vhscommercials", "--out", "inventory.json", "--pilot-out", "pilot.json",
		"--role-hint", "promo", "--cache-dir", "cache", "--user-agent", "identified client",
		"--snapshot-at", "2026-08-27T12:00:00Z", "--max-requests", "2", "--max-items", "1",
		"--max-item-bytes", "1", "--max-total-bytes", "1", "--delay", "500ms", "--max-wall-time", "1s",
	}
	if code := run(args, testWriter{t}, testWriter{t}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }
