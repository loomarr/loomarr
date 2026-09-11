package fillercorpus

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDecodeInventoryRejectsLegacyAndUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"schemaVersion":1,"source":"archive.org","collection":"prelinger","snapshotAt":"2026-08-26T00:00:00Z","cases":[]}`,
		`{"schemaVersion":2,"snapshotAt":"2026-08-26T00:00:00Z","captures":[],"cases":[],"legacy":true}`,
		`{"schemaVersion":3,"snapshotAt":"2026-08-26T00:00:00Z","captures":[],"cases":[{"captureId":"legacy"}]}`,
	} {
		if _, err := DecodeInventory(strings.NewReader(raw)); err == nil {
			t.Fatalf("DecodeInventory(%s) succeeded", raw)
		}
	}
}

func TestHistoricalInventoryEvidenceReadsV4WithoutGrantingCurrentAuthority(t *testing.T) {
	value := validInventoryForMerge(time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), "archive.org/prelinger", "archive.org")
	value.SchemaVersion = HistoricalInventorySchemaVersion
	value.Cases[0].Representation.Soundtrack = SoundtrackExpectation{}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := DecodeHistoricalInventoryEvidence(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if evidence.SchemaVersion != HistoricalInventorySchemaVersion || evidence.SHA256 != InventorySHA256(raw) || !slices.Equal(evidence.CaseIDs, []string{value.Cases[0].CaseID}) {
		t.Fatalf("historical evidence = %+v", evidence)
	}
	if _, err := DecodeInventoryBytes(raw); err == nil {
		t.Fatal("historical inventory authorized a current workflow")
	}
}

func TestValidateInventoryAcceptsMixedAuthoritiesAndExactHosts(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	value := Inventory{SchemaVersion: InventorySchemaVersion, SnapshotAt: snapshot}
	for _, authority := range []string{"archive.org/prelinger", "loc.gov/national-screening-room"} {
		captureID := NewCaptureID(authority, "", "commercial")
		value.Captures = append(value.Captures, Capture{CaptureID: captureID, Transport: TransportHTTPS, Authority: authority, RoleHint: "commercial", SnapshotAt: snapshot, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 100, ResponseBytes: 50, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10})
		itemID := strings.ReplaceAll(authority, "/", "-")
		mediaHost := "archive.org"
		if authority == "loc.gov/national-screening-room" {
			mediaHost = "tile.loc.gov"
		}
		representation := testInventoryRepresentation(InventoryRepresentation{Transport: TransportHTTPS, Name: "clip.mp4", URL: "https://" + mediaHost + "/clip.mp4?frozen=1", MIMEType: "video/mp4", Bytes: 100}, strings.Repeat("a", 64))
		value.Cases = append(value.Cases, InventoryCase{
			CaseID: CaseID(authority, itemID), CaptureIDs: []string{captureID}, Authority: authority, ItemID: itemID, Title: "Clip", RoleHints: []string{"commercial"},
			RightsAssertions: []string{"public domain"}, ItemURL: "https://example.org/item", MetadataURL: "https://example.org/metadata",
			MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), AllowedMediaHosts: []string{mediaHost},
			Representation: representation,
		})
	}
	if failures := ValidateInventory(value); len(failures) != 0 {
		t.Fatalf("ValidateInventory() = %v", failures)
	}
	value.Cases[0].Representation.URL = "https://cdn.example.org/clip.mp4"
	if failures := ValidateInventory(value); len(failures) == 0 {
		t.Fatal("ValidateInventory accepted a host outside the case allowlist")
	}
}

func TestValidateInventoryRejectsUnboundSoundtrackAuthority(t *testing.T) {
	valid := validInventoryForMerge(time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), "archive.org/prelinger", "archive.org")
	for name, mutate := range map[string]func(*InventoryRepresentation){
		"missing":               func(value *InventoryRepresentation) { value.Soundtrack = SoundtrackExpectation{} },
		"unknown status":        func(value *InventoryRepresentation) { value.Soundtrack.Status = "maybe" },
		"unknown evidence kind": func(value *InventoryRepresentation) { value.Soundtrack.EvidenceKind = "title_guess" },
		"missing evidence":      func(value *InventoryRepresentation) { value.Soundtrack.EvidenceSHA256 = "" },
		"missing locator":       func(value *InventoryRepresentation) { value.Soundtrack.EvidenceLocator = "" },
		"representation drift":  func(value *InventoryRepresentation) { value.URL += "?different=1" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Cases = append([]InventoryCase(nil), valid.Cases...)
			mutate(&candidate.Cases[0].Representation)
			if failures := ValidateInventory(candidate); !slices.ContainsFunc(failures, func(failure string) bool { return strings.Contains(failure, "soundtrack expectation") }) {
				t.Fatalf("failures = %v", failures)
			}
		})
	}
}

func TestMergeInventoriesIsDeterministicAndRejectsDuplicateCapture(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	base := validInventoryForMerge(snapshot, "archive.org/prelinger", "archive.org")
	later := validInventoryForMerge(snapshot.Add(time.Minute), "loc.gov/national-screening-room", "tile.loc.gov")
	merged, err := MergeInventories(later, base)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.SnapshotAt.Equal(later.SnapshotAt) || merged.Captures[0].Authority != "archive.org/prelinger" || len(merged.Cases) != 2 {
		t.Fatalf("merged = %+v", merged)
	}
	if _, err := MergeInventories(base, base); err == nil {
		t.Fatal("duplicate capture was merged")
	}
}

func TestMergeInventoriesUnionsMatchingCaseDiscoveredByDifferentRoleCaptures(t *testing.T) {
	snapshot := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	commercial := validInventoryForMerge(snapshot, "archive.org/prelinger", "archive.org")
	promo := validInventoryForMerge(snapshot, "archive.org/prelinger", "archive.org")
	promoCaptureID := NewCaptureID("archive.org/prelinger", "", "promo")
	promo.Captures[0].CaptureID = promoCaptureID
	promo.Captures[0].RoleHint = "promo"
	promo.Cases[0].CaptureIDs = []string{promoCaptureID}
	promo.Cases[0].RoleHints = []string{"promo"}

	merged, err := MergeInventories(commercial, promo)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Captures) != 2 || len(merged.Cases) != 1 {
		t.Fatalf("merged = %+v", merged)
	}
	if got, want := merged.Cases[0].RoleHints, []string{"commercial", "promo"}; !slices.Equal(got, want) {
		t.Fatalf("role hints = %v; want %v", got, want)
	}
	if got, want := merged.Cases[0].CaptureIDs, []string{commercial.Captures[0].CaptureID, promoCaptureID}; !slices.Equal(got, want) {
		t.Fatalf("capture IDs = %v; want %v", got, want)
	}
	promo.Cases[0].Title = "Conflicting title"
	if _, err := MergeInventories(commercial, promo); err == nil || !strings.Contains(err.Error(), "conflicting frozen identity") {
		t.Fatalf("conflicting duplicate error = %v", err)
	}
}

func validInventoryForMerge(snapshot time.Time, authority, host string) Inventory {
	role := "commercial"
	captureID := NewCaptureID(authority, "", role)
	itemID := strings.ReplaceAll(authority, "/", "-")
	representation := testInventoryRepresentation(InventoryRepresentation{Transport: TransportHTTPS, Name: "clip.mp4", URL: "https://" + host + "/clip.mp4", MIMEType: "video/mp4", Bytes: 100}, strings.Repeat("a", 64))
	return Inventory{SchemaVersion: InventorySchemaVersion, SnapshotAt: snapshot, Captures: []Capture{{CaptureID: captureID, Transport: TransportHTTPS, Authority: authority, RoleHint: role, SnapshotAt: snapshot, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 100, ResponseBytes: 50, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []InventoryCase{{CaseID: CaseID(authority, itemID), CaptureIDs: []string{captureID}, Authority: authority, ItemID: itemID, Title: "Clip", RoleHints: []string{role}, RightsAssertions: []string{"public domain"}, ItemURL: "https://example.org/item", MetadataURL: "https://example.org/metadata", MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), AllowedMediaHosts: []string{host}, Representation: representation}}}
}

func TestInventoryFromLanePreservesCaptureEvidenceWithoutGrantingAuthority(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	representation := Representation{Name: "clip.mp4", URL: "https://images-assets.nasa.gov/clip.mp4", MIMEType: "video/mp4", Bytes: 100}
	representation.Soundtrack = BindRepresentationSoundtrack(representation, SoundtrackPresentExpected, SoundtrackEvidenceFirstPartyMetadata, strings.Repeat("a", 64), "fixture metadata")
	lane := Lane{Authority: "images.nasa.gov", MaxRequests: 3, RequestsUsed: 3, MaxResponseBytes: 1000, ResponseBytes: 500, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10, Cases: []Candidate{{ItemID: "nasa-1", Title: "Trailer", RoleHints: []string{"trailer"}, ItemURL: "https://images.nasa.gov/details/nasa-1", MetadataURL: "https://images-assets.nasa.gov/metadata.json", MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), RightsAssertions: []string{"NASA"}, Representation: representation}}}
	got, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "mission trailers", AllowedMediaHosts: []string{"images-assets.nasa.gov"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cases[0].CaseID != "images.nasa.gov/nasa-1" || got.Captures[0].RequestsUsed != 3 {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestInventoryFromFrozenUSGSLaneUsesExactAuthority(t *testing.T) {
	lane, snapshot := frozenUSGSLane()
	got, err := InventoryFromLane(lane, LaneInventoryOptions{
		SnapshotAt: snapshot, Collection: "usgs-audible-seed.json", AllowedMediaHosts: []string{usgsMediaHost},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != InventorySchemaVersion || len(got.Cases) != 2 || got.Captures[0].PredictedMediaBytes != 98_690_824 {
		t.Fatalf("inventory = %+v", got)
	}
	if got.Cases[0].CaseID != "usgs.gov/media/videos/november-2021-yellowstone-volcano" || got.Cases[1].CaseID != "usgs.gov/media/videos/usgs-gas-hydrates-lab" {
		t.Fatalf("case IDs = %q, %q", got.Cases[0].CaseID, got.Cases[1].CaseID)
	}
}

func TestValidateInventoryRejectsUSGSAuthorityEscapes(t *testing.T) {
	for name, mutate := range map[string]func(*InventoryCase){
		"lookalike page host": func(item *InventoryCase) {
			item.ItemURL = "https://www.usgs.gov.example/media/videos/" + item.ItemID
		},
		"arbitrary USGS path": func(item *InventoryCase) {
			item.ItemURL = "https://www.usgs.gov/programs/" + item.ItemID
		},
		"page query": func(item *InventoryCase) {
			item.ItemURL += "?download=1"
		},
		"credentialed page": func(item *InventoryCase) {
			item.ItemURL = "https://user@www.usgs.gov/media/videos/" + item.ItemID
		},
		"HTTP page": func(item *InventoryCase) {
			item.ItemURL = strings.Replace(item.ItemURL, "https://", "http://", 1)
		},
		"lookalike media host": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, usgsMediaHost, usgsMediaHost+".example", 1)
		},
		"sibling output bucket": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, usgsMediaHost, "usgs-ocapsv2-public-output-media.s3.us-east-1.amazonaws.com", 1)
		},
		"input media bucket": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, usgsMediaHost, "usgs-ocapsv2-public-input-media.s3.us-west-2.amazonaws.com", 1)
		},
		"wildcard media rule": func(item *InventoryCase) {
			item.AllowedMediaHosts = []string{".amazonaws.com"}
		},
		"HTTP media": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "https://", "http://", 1)
		},
		"credentialed media": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "https://", "https://user@", 1)
		},
		"media query": func(item *InventoryCase) {
			item.Representation.URL += "?download=1"
		},
		"media port": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, usgsMediaHost, usgsMediaHost+":443", 1)
		},
		"encoded traversal": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "/atoms/video/", "/atoms/%2e%2e/", 1)
		},
		"outside output namespace": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, usgsMediaPathRoot, "/private/", 1)
		},
		"non-MP4 derivative": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "/MP4/", "/original/", 1)
		},
		"representation name mismatch": func(item *InventoryCase) {
			item.Representation.Name = "other.mp4"
		},
	} {
		t.Run(name, func(t *testing.T) {
			lane, snapshot := frozenUSGSLane()
			inventory, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "fixture", AllowedMediaHosts: []string{usgsMediaHost}})
			if err != nil {
				t.Fatal(err)
			}
			mutate(&inventory.Cases[0])
			if failures := ValidateInventory(inventory); !slices.ContainsFunc(failures, func(failure string) bool { return strings.Contains(failure, "USGS video authority") }) {
				t.Fatalf("failures = %v", failures)
			}
		})
	}
}

func frozenUSGSLane() (Lane, time.Time) {
	snapshot := time.Date(2026, 9, 5, 19, 56, 0, 0, time.UTC)
	first := Candidate{
		ItemID: "november-2021-yellowstone-volcano", Title: "November (2021) Yellowstone Volcano", RoleHints: []string{"programme_parent"},
		ItemURL: "https://www.usgs.gov/media/videos/november-2021-yellowstone-volcano", MetadataURL: "https://www.usgs.gov/media/videos/november-2021-yellowstone-volcano",
		MetadataRetrievedAt: time.Date(2026, 9, 5, 19, 55, 0, 832_179_000, time.UTC), MetadataSHA256: "7fbd7e3a7c5033260132dcbefd268ba543894eaac657b3e3b42ef640b9ff540e",
		RightsAssertions: []string{"Exact USGS item page states Sources/Usage: Public Domain.", "Exact USGS item page credits video editing to Liz Westby."},
		Representation:   Representation{Name: "2021_Nov_1_YVO_Monthly_Update.mp4", URL: "https://" + usgsMediaHost + usgsMediaPathRoot + "atoms/video/2021_Nov_1_YVO_Monthly_Update/MP4/2021_Nov_1_YVO_Monthly_Update.mp4", MIMEType: "video/mp4", Bytes: 33_805_660},
	}
	first.Representation.Soundtrack = BindRepresentationSoundtrack(first.Representation, SoundtrackUnknown, SoundtrackEvidenceFirstPartyMetadata, first.MetadataSHA256, "frozen page capture did not establish audio")
	second := Candidate{
		ItemID: "usgs-gas-hydrates-lab", Title: "USGS Gas Hydrates Lab", RoleHints: []string{"programme_parent"},
		ItemURL: "https://www.usgs.gov/media/videos/usgs-gas-hydrates-lab", MetadataURL: "https://www.usgs.gov/media/videos/usgs-gas-hydrates-lab",
		MetadataRetrievedAt: time.Date(2026, 9, 5, 19, 55, 3, 116_095_000, time.UTC), MetadataSHA256: "d902343c3b70479cd91db966a20160f2a9620d04e70cfa3b5eb71942e1cf8ea2",
		RightsAssertions: []string{"Exact USGS item page states Sources/Usage: Public Domain.", "Exact USGS item page identifies Stephen M. Wessells of the U.S. Geological Survey as contact."},
		Representation:   Representation{Name: "Gas_Hydrates_2012.mp4", URL: "https://" + usgsMediaHost + usgsMediaPathRoot + "Gas_Hydrates_2012/MP4/Gas_Hydrates_2012.mp4", MIMEType: "video/mp4", Bytes: 64_885_164},
	}
	second.Representation.Soundtrack = BindRepresentationSoundtrack(second.Representation, SoundtrackUnknown, SoundtrackEvidenceFirstPartyMetadata, second.MetadataSHA256, "frozen page capture did not establish audio")
	return Lane{Authority: usgsVideoAuthority, MaxRequests: 4, RequestsUsed: 4, MaxResponseBytes: 2_000_000, ResponseBytes: 166_726, MaxPredictedMediaBytes: 100_000_000, PredictedMediaBytes: 98_690_824, MaxWallTimeMS: 120_000, WallTimeMS: 3_429, Cases: []Candidate{first, second}}, snapshot
}

func TestInventoryFromFrozenBlenderLaneUsesExactAuthority(t *testing.T) {
	lane, snapshot := frozenBlenderLane()
	got, err := InventoryFromLane(lane, LaneInventoryOptions{
		SnapshotAt: snapshot, Collection: "blender-trailer-seed.json", AllowedMediaHosts: []string{blenderSintelMediaHost},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != InventorySchemaVersion || len(got.Cases) != 1 || got.Captures[0].PredictedMediaBytes != 7_608_204 || got.Cases[0].CaseID != "blender.org/open-movies/sintel-trailer-720p" {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestValidateInventoryRejectsBlenderAuthorityEscapes(t *testing.T) {
	for name, mutate := range map[string]func(*InventoryCase){
		"arbitrary Blender subdomain": func(item *InventoryCase) {
			item.ItemURL = "https://studio.blender.org/download/"
		},
		"generic licence page": func(item *InventoryCase) {
			item.MetadataURL = "https://www.blender.org/about/license/"
		},
		"HTTP page": func(item *InventoryCase) {
			item.ItemURL = strings.Replace(item.ItemURL, "https://", "http://", 1)
		},
		"credentialed page": func(item *InventoryCase) {
			item.ItemURL = strings.Replace(item.ItemURL, "https://", "https://user@", 1)
		},
		"mirror": func(item *InventoryCase) {
			item.Representation.URL = "https://mirror.example/sintel_trailer-720p.mp4"
		},
		"other Blender object": func(item *InventoryCase) {
			item.Representation.URL = "https://download.blender.org/durian/trailer/other.mp4"
		},
		"directory listing": func(item *InventoryCase) {
			item.Representation.URL = "https://download.blender.org/durian/trailer/"
		},
		"wildcard host": func(item *InventoryCase) {
			item.AllowedMediaHosts = []string{".blender.org"}
		},
		"HTTP media": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "https://", "http://", 1)
		},
		"credentialed media": func(item *InventoryCase) {
			item.Representation.URL = strings.Replace(item.Representation.URL, "https://", "https://user@", 1)
		},
		"unsupported MIME type": func(item *InventoryCase) {
			item.Representation.MIMEType = "application/octet-stream"
		},
		"representation name drift": func(item *InventoryCase) {
			item.Representation.Name = "other.mp4"
		},
	} {
		t.Run(name, func(t *testing.T) {
			lane, snapshot := frozenBlenderLane()
			inventory, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "fixture", AllowedMediaHosts: []string{blenderSintelMediaHost}})
			if err != nil {
				t.Fatal(err)
			}
			mutate(&inventory.Cases[0])
			if failures := ValidateInventory(inventory); !slices.ContainsFunc(failures, func(failure string) bool { return strings.Contains(failure, "blender Open Movie authority") }) {
				t.Fatalf("failures = %v", failures)
			}
		})
	}
}

func TestValidateInventoryRejectsUnreviewedBlenderAuthority(t *testing.T) {
	lane, snapshot := frozenBlenderLane()
	inventory, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "fixture", AllowedMediaHosts: []string{blenderSintelMediaHost}})
	if err != nil {
		t.Fatal(err)
	}
	inventory.Captures[0].Authority = "blender.org"
	inventory.Captures[0].CaptureID = NewCaptureID("blender.org", "fixture", "trailer")
	inventory.Cases[0].Authority = "blender.org"
	inventory.Cases[0].CaseID = CaseID("blender.org", blenderSintelItemID)
	inventory.Cases[0].CaptureIDs = []string{inventory.Captures[0].CaptureID}
	if failures := ValidateInventory(inventory); !slices.ContainsFunc(failures, func(failure string) bool { return strings.Contains(failure, "no supported authority host policy") }) {
		t.Fatalf("failures = %v", failures)
	}
}

func frozenBlenderLane() (Lane, time.Time) {
	snapshot := time.Date(2026, 9, 5, 20, 10, 0, 0, time.UTC)
	item := Candidate{
		ItemID: blenderSintelItemID, Title: "Sintel Trailer", RoleHints: []string{"trailer"}, ItemURL: blenderSintelPageURL, MetadataURL: blenderSintelPageURL,
		MetadataRetrievedAt: time.Date(2026, 9, 5, 20, 9, 49, 147_481_000, time.UTC), MetadataSHA256: "ae7f8471316a597fbffa2bc39f3560790aa0f968f4788d7b1bbe64d020b04b0f",
		RightsAssertions: []string{"Exact official Blender project download page classifies the representation under Trailer and links the Sintel trailer MP4 path.", "Official Sintel project pages state that the film and project-created work are released under Creative Commons Attribution 3.0; exact trailer/music attribution and any third-party elements remain for rights review."},
		Representation:   Representation{Name: blenderSintelMediaName, URL: blenderSintelMediaURL, MIMEType: "video/mp4", Bytes: 7_608_204},
	}
	item.Representation.Soundtrack = BindRepresentationSoundtrack(item.Representation, SoundtrackUnknown, SoundtrackEvidenceFirstPartyMetadata, item.MetadataSHA256, "frozen project download page did not establish audio")
	return Lane{Authority: blenderOpenMovieAuthority, MaxRequests: 2, RequestsUsed: 2, MaxResponseBytes: 1_000_000, ResponseBytes: 27_556, MaxPredictedMediaBytes: 10_000_000, PredictedMediaBytes: 7_608_204, MaxWallTimeMS: 60_000, WallTimeMS: 978, Cases: []Candidate{item}}, snapshot
}

func TestHistoricalPilotLanesCannotPromoteWithoutSoundtrackAuthority(t *testing.T) {
	snapshot := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	lanes := map[string]string{"prelinger": "archive.org", "loc": "tile.loc.gov", "nasa": "images-assets.nasa.gov", "cdc": "www.cdc.gov", "commons": "upload.wikimedia.org"}
	for name, host := range lanes {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("corpus/pilot/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var lane Lane
			if err := json.Unmarshal(raw, &lane); err != nil {
				t.Fatal(err)
			}
			if _, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "pilot/" + name, AllowedMediaHosts: []string{host}}); err == nil || !strings.Contains(err.Error(), "soundtrack expectation") {
				t.Fatalf("historical lane promotion error = %v", err)
			}
		})
	}
}

func testInventoryRepresentation(representation InventoryRepresentation, evidenceSHA256 string) InventoryRepresentation {
	representation.Soundtrack = BindInventorySoundtrack(representation, SoundtrackPresentExpected, SoundtrackEvidenceFirstPartyMetadata, evidenceSHA256, "fixture metadata")
	return representation
}
