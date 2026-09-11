package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercandidatepool"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerquarantine"
	"github.com/loomarr/loomarr/internal/fillerreview"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFrozenSourceFixturesProduceClosedCandidateDispositions(t *testing.T) {
	root := t.TempDir()
	snapshot := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	metadata := strings.Repeat("a", 64)
	lanes := []struct {
		name       string
		lane       fillercorpus.Lane
		collection string
		hosts      []string
		want       string
		wantHold   string
	}{
		{
			name: "CDC", collection: "cdc-audible-seed-v1.json", hosts: []string{"www.cdc.gov"},
			lane: frozenCandidateLane(snapshot, "cdc.gov", "charge-your-phone", "PSA", "https://www.cdc.gov/natural-disasters/psa-toolkit/charge-your-phone.html", "https://www.cdc.gov/wcms/video/low-res/disasters/2023/378637863-ATSDR-PSAs-Charge-your-Phone-1200-by-645.mp4", "378637863-ATSDR-PSAs-Charge-your-Phone-1200-by-645.mp4", 352_384, fillercorpus.SoundtrackPresentExpected, metadata),
			want: fillercandidatepool.DispositionEligible,
		},
		{
			name: "Blender", collection: "blender-trailer-seed.json", hosts: []string{"download.blender.org"},
			lane: frozenCandidateLane(snapshot, "blender.org/open-movies", "sintel-trailer-720p", "trailer", "https://durian.blender.org/download/", "https://download.blender.org/durian/trailer/sintel_trailer-720p.mp4", "sintel_trailer-720p.mp4", 7_608_204, fillercorpus.SoundtrackUnknown, "ae7f8471316a597fbffa2bc39f3560790aa0f968f4788d7b1bbe64d020b04b0f"),
			want: fillercandidatepool.DispositionHeld, wantHold: fillercandidatepool.HoldSoundtrackUnknown,
		},
		{
			name: "USGS", collection: "usgs-audible-seed.json", hosts: []string{"usgs-ocapsv2-public-output-media.s3.us-west-2.amazonaws.com"},
			lane: frozenCandidateLane(snapshot, "usgs.gov/media/videos", "november-2021-yellowstone-volcano", "programme_parent", "https://www.usgs.gov/media/videos/november-2021-yellowstone-volcano", "https://usgs-ocapsv2-public-output-media.s3.us-west-2.amazonaws.com/assets/palladium/production/s3fs-public/atoms/video/2021_Nov_1_YVO_Monthly_Update/MP4/2021_Nov_1_YVO_Monthly_Update.mp4", "2021_Nov_1_YVO_Monthly_Update.mp4", 33_805_660, fillercorpus.SoundtrackUnknown, "7fbd7e3a7c5033260132dcbefd268ba543894eaac657b3e3b42ef640b9ff540e"),
			want: fillercandidatepool.DispositionHeld, wantHold: fillercandidatepool.HoldSoundtrackUnknown,
		},
		{
			name: "LOC", collection: "loc-historical-v4.json", hosts: []string{"tile.loc.gov"},
			lane: frozenCandidateLane(snapshot, "loc.gov/national-screening-room", "97516784", "commercial", "https://www.loc.gov/item/97516784/", "https://tile.loc.gov/storage-services/service/mbrs/ntscrm/00007341/00007341.mp4", "00007341.mp4", 16_028_110, fillercorpus.SoundtrackUnknown, "b34773cb4f00ce9735c48ac76204c25aa951867c41cf6ef13e847f5d92e862df"),
			want: fillercandidatepool.DispositionHeld, wantHold: fillercandidatepool.HoldSoundtrackUnknown,
		},
	}

	inventories := make([]fillercorpus.Inventory, 0, len(lanes))
	fixtureByCase := make(map[string]int, len(lanes))
	for index, fixture := range lanes {
		inventory, err := fillercorpus.InventoryFromLane(fixture.lane, fillercorpus.LaneInventoryOptions{SnapshotAt: snapshot, Collection: fixture.collection, AllowedMediaHosts: fixture.hosts})
		if err != nil {
			t.Fatalf("%s frozen inventory: %v", fixture.name, err)
		}
		inventories = append(inventories, inventory)
		fixtureByCase[inventory.Cases[0].CaseID] = index
	}
	inventory, err := fillercorpus.MergeInventories(inventories...)
	if err != nil {
		t.Fatal(err)
	}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	inventorySHA := fillercorpus.InventorySHA256(inventoryRaw)
	dispositions := make(map[string]string, len(inventory.Cases))
	contentByCase := make(map[string]string, len(inventory.Cases))
	for index, item := range inventory.Cases {
		media := []byte("frozen inspected media for " + item.CaseID)
		localFile := "source-" + string(rune('a'+index)) + ".mp4"
		if err := os.WriteFile(filepath.Join(root, localFile), media, 0o600); err != nil {
			t.Fatal(err)
		}
		dispositions[item.CaseID] = fillerquarantine.DispositionEligibleForRightsReview
		contentByCase[item.CaseID] = fillercandidatepool.Digest(media)
	}
	quarantineRaw := testkit.FillerQuarantineReport(t, inventoryRaw, dispositions, contentByCase)
	quarantine, err := fillerquarantine.OpenRightsEligibility(inventoryRaw, quarantineRaw)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := quarantine.Selected(len(inventory.Cases), len(inventory.Cases))
	if err != nil {
		t.Fatal(err)
	}
	bindingByCase := make(map[string]*fillercorpus.QuarantineInspectionCaseBinding, len(selection.Cases))
	for _, selected := range selection.Cases {
		bindingByCase[selected.Inventory.CaseID] = selected.QuarantineInspection
	}

	prior := fillercandidatepool.Exposure{SourceSHA256: []string{}, FamilyIDs: []string{}, ProgrammeProvenance: []fillercandidatepool.ProgrammeProvenance{}}
	for index, item := range inventory.Cases {
		fixture := lanes[fixtureByCase[item.CaseID]]
		t.Run(fixture.name, func(t *testing.T) {
			media := []byte("frozen inspected media for " + item.CaseID)
			localFile := "source-" + string(rune('a'+index)) + ".mp4"
			contentSHA := fillercandidatepool.Digest(media)
			decision := fillercorpus.RightsDecision{
				InventorySHA256: inventorySHA, CaseID: item.CaseID, CaptureIDs: item.CaptureIDs,
				Authority: item.Authority, ItemID: item.ItemID, MetadataSHA256: item.MetadataSHA256,
				ReviewerID: "frozen-fixture-reviewer", ReviewedAt: snapshot.Add(time.Hour), Decision: "approved", Basis: "frozen fixture rights", Redistributable: true,
				QuarantineInspection: bindingByCase[item.CaseID],
			}
			materialized := fillercorpus.MaterializedCase{CaseID: item.CaseID, LocalFile: localFile, ContentSHA256: contentSHA}
			role := fillereval.TemporalRoleCommercial
			unit := fillereval.UnitStandalone
			switch fixture.name {
			case "CDC":
				role = fillereval.TemporalRolePSA
			case "Blender":
				role = fillereval.TemporalRoleTrailer
			case "USGS":
				unit = fillereval.UnitProgrammeExcerpt
			}
			review := fillerreview.ReplacementCandidateReviewCase{
				CaseID: item.CaseID, EvidenceAlias: "evidence-" + string(rune('a'+index)), InspectedSourceFile: localFile, InspectedSourceSHA: contentSHA,
				ReviewedMediaPath: localFile, ReviewedMediaSHA: contentSHA, ReviewedMediaBytes: int64(len(media)), DurationMS: 180_000,
				Unit: unit, TechnicalVerdict: "continue", HadAudio: true, FullDecodeMeasured: true,
				Suitability: "candidate_no_signal_observed", FamilyID: "singleton-" + string(rune('a'+index)),
			}
			if unit == fillereval.UnitStandalone {
				review.Role = &role
				review.Transition = fillerreview.TemporalTransitionAuthorityCase{
					EvidenceAlias: review.EvidenceAlias, CaseID: item.CaseID, SourceSHA256: contentSHA, DurationMS: review.DurationMS,
					Head: fillerreview.TemporalTransitionEdge{StartMS: 0, EndMS: 1_000}, Tail: fillerreview.TemporalTransitionEdge{StartMS: 179_000, EndMS: 180_000},
				}
			}
			candidate, err := buildCandidate(Config{SourceRoot: root}, item, decision, materialized, review, quarantine, prior)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := buildCandidate(Config{SourceRoot: root}, item, decision, materialized, review, quarantine, prior)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(candidate, repeated) {
				t.Fatalf("candidate did not reproduce for identical frozen authorities:\nfirst: %+v\nsecond: %+v", candidate, repeated)
			}
			if candidate.Disposition != fixture.want {
				t.Fatalf("disposition = %q, holds=%v; want %q", candidate.Disposition, candidate.HoldReasons, fixture.want)
			}
			if fixture.wantHold != "" && !reflect.DeepEqual(candidate.HoldReasons, []string{fixture.wantHold}) {
				t.Fatalf("holds = %v; want only %q", candidate.HoldReasons, fixture.wantHold)
			}
		})
	}
}

func frozenCandidateLane(snapshot time.Time, authority, itemID, role, itemURL, mediaURL, mediaName string, mediaBytes int64, soundtrack, metadataSHA string) fillercorpus.Lane {
	representation := fillercorpus.Representation{Name: mediaName, URL: mediaURL, MIMEType: "video/mp4", Bytes: mediaBytes}
	representation.Soundtrack = fillercorpus.BindRepresentationSoundtrack(representation, soundtrack, fillercorpus.SoundtrackEvidenceFirstPartyMetadata, metadataSHA, "frozen source metadata")
	return fillercorpus.Lane{
		Authority: authority, MaxRequests: 2, RequestsUsed: 2, MaxResponseBytes: 2_000_000, ResponseBytes: 1,
		MaxPredictedMediaBytes: mediaBytes, PredictedMediaBytes: mediaBytes, MaxWallTimeMS: 60_000, WallTimeMS: 1,
		Cases: []fillercorpus.Candidate{{
			ItemID: itemID, Title: itemID, RoleHints: []string{role}, ItemURL: itemURL, MetadataURL: itemURL,
			MetadataRetrievedAt: snapshot, MetadataSHA256: metadataSHA, RightsAssertions: []string{"frozen source rights assertion"}, Representation: representation,
		}},
	}
}

func TestBuildCandidateRequiresEveryLocalDownstreamAuthority(t *testing.T) {
	root := t.TempDir()
	item, decision, review, quarantine := localCandidateFixture(t, root)
	config := Config{SourceRoot: root}
	prior := fillercandidatepool.Exposure{SourceSHA256: []string{}, FamilyIDs: []string{}, ProgrammeProvenance: []fillercandidatepool.ProgrammeProvenance{}}
	candidate, err := buildCandidate(config, item, decision, fillercorpus.MaterializedCase{}, review, quarantine, prior)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Disposition != fillercandidatepool.DispositionEligible || len(candidate.HoldReasons) != 0 {
		t.Fatalf("eligible candidate = %+v", candidate)
	}
	pool := fillercandidatepool.Pool{
		SchemaVersion: fillercandidatepool.SchemaVersion, ContractVersion: fillercandidatepool.ContractVersion,
		GeneratedAt:   time.Date(2026, 9, 11, 5, 0, 0, 0, time.UTC),
		Inputs:        []fillercandidatepool.Input{{Name: "fixture", SHA256: strings.Repeat("a", 64)}},
		PriorExposure: prior, Candidates: []fillercandidatepool.Candidate{candidate},
	}
	if err := fillercandidatepool.Validate(pool); err != nil {
		t.Fatalf("candidate does not satisfy pool contract: %v", err)
	}

	for name, test := range map[string]struct {
		mutate func(*fillercorpus.InventoryCase, *fillercorpus.RightsDecision, *fillerreview.ReplacementCandidateReviewCase, *fillercandidatepool.Exposure)
		want   string
	}{
		"held rights": {func(_ *fillercorpus.InventoryCase, decision *fillercorpus.RightsDecision, _ *fillerreview.ReplacementCandidateReviewCase, _ *fillercandidatepool.Exposure) {
			decision.Decision = "held"
		}, fillercandidatepool.HoldRights},
		"missing decode": {func(_ *fillercorpus.InventoryCase, _ *fillercorpus.RightsDecision, review *fillerreview.ReplacementCandidateReviewCase, _ *fillercandidatepool.Exposure) {
			review.FullDecodeMeasured = false
		}, fillercandidatepool.HoldTechnical},
		"missing audio": {func(_ *fillercorpus.InventoryCase, _ *fillercorpus.RightsDecision, review *fillerreview.ReplacementCandidateReviewCase, _ *fillercandidatepool.Exposure) {
			review.HadAudio = false
		}, fillercandidatepool.HoldSoundtrackDecodedAudio},
		"audience hold": {func(_ *fillercorpus.InventoryCase, _ *fillercorpus.RightsDecision, review *fillerreview.ReplacementCandidateReviewCase, _ *fillercandidatepool.Exposure) {
			review.Suitability = "coverage_hold"
		}, fillercandidatepool.HoldSuitability},
		"hand role absent": {func(_ *fillercorpus.InventoryCase, _ *fillercorpus.RightsDecision, review *fillerreview.ReplacementCandidateReviewCase, _ *fillercandidatepool.Exposure) {
			review.Role = nil
		}, fillercandidatepool.HoldSemanticRole},
		"prior family": {func(_ *fillercorpus.InventoryCase, _ *fillercorpus.RightsDecision, review *fillerreview.ReplacementCandidateReviewCase, prior *fillercandidatepool.Exposure) {
			prior.FamilyIDs = []string{review.FamilyID}
		}, fillercandidatepool.HoldPriorFamily},
	} {
		t.Run(name, func(t *testing.T) {
			changedItem, changedDecision, changedReview := item, decision, review
			changedPrior := fillercandidatepool.Exposure{SourceSHA256: []string{}, FamilyIDs: []string{}, ProgrammeProvenance: []fillercandidatepool.ProgrammeProvenance{}}
			test.mutate(&changedItem, &changedDecision, &changedReview, &changedPrior)
			got, err := buildCandidate(config, changedItem, changedDecision, fillercorpus.MaterializedCase{}, changedReview, quarantine, changedPrior)
			if err != nil {
				t.Fatal(err)
			}
			if got.Disposition != fillercandidatepool.DispositionHeld || !contains(got.HoldReasons, test.want) {
				t.Fatalf("held candidate = %+v, want %q", got, test.want)
			}
		})
	}
}

func TestBuildRejectsLegacyInventoryBeforePublishing(t *testing.T) {
	root := t.TempDir()
	inventoryPath := filepath.Join(root, "inventory.json")
	if err := os.WriteFile(inventoryPath, []byte(`{"schemaVersion":4}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Build(Config{
		InventoryPath: inventoryPath, RightsDecisionsPath: filepath.Join(root, "decisions.jsonl"),
		RightsProfile: fillercorpus.RightsProfileDevelopment, SourceRoot: root,
		PriorAdjudicationPaths: []string{filepath.Join(root, "prior.json")},
		GeneratedAt:            time.Date(2026, 9, 11, 5, 0, 0, 0, time.UTC), OutputPath: filepath.Join(root, "pool.json"),
	})
	if err == nil || !strings.Contains(err.Error(), "schemaVersion") {
		t.Fatalf("legacy inventory error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "pool.json")); !os.IsNotExist(statErr) {
		t.Fatalf("legacy inventory published output: %v", statErr)
	}
}

func TestBuildRejectsNonLocalInventoryWithoutExactAcquisitionAuthorities(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	authority, itemID, role := "images.nasa.gov", "fixture-video", "trailer"
	captureID := fillercorpus.NewCaptureID(authority, "", role)
	metadataSHA := strings.Repeat("a", 64)
	representation := fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportHTTPS, Name: "clip.mp4", URL: "https://images-assets.nasa.gov/clip.mp4", MIMEType: "video/mp4", Bytes: 100}
	representation.Soundtrack = fillercorpus.BindInventorySoundtrack(representation, fillercorpus.SoundtrackUnknown, fillercorpus.SoundtrackEvidenceFirstPartyMetadata, metadataSHA, "frozen metadata did not establish audio")
	item := fillercorpus.InventoryCase{
		CaseID: fillercorpus.CaseID(authority, itemID), CaptureIDs: []string{captureID}, Authority: authority, ItemID: itemID,
		Title: "Fixture", RoleHints: []string{role}, RightsAssertions: []string{"NASA"},
		ItemURL: "https://images.nasa.gov/details/fixture-video", MetadataURL: "https://images-assets.nasa.gov/metadata.json",
		MetadataRetrievedAt: at, MetadataSHA256: metadataSHA, AllowedMediaHosts: []string{"images-assets.nasa.gov"}, Representation: representation,
	}
	inventory := fillercorpus.Inventory{
		SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: at,
		Captures: []fillercorpus.Capture{{CaptureID: captureID, Transport: fillercorpus.TransportHTTPS, Authority: authority, RoleHint: role, SnapshotAt: at, MaxRequests: 1, RequestsUsed: 1, MaxResponseBytes: 1, ResponseBytes: 1, MaxPredictedMediaBytes: 100, PredictedMediaBytes: 100, MaxWallTimeMS: 1, WallTimeMS: 1}},
		Cases:    []fillercorpus.InventoryCase{item},
	}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	inventoryPath := filepath.Join(root, "inventory.json")
	if err := os.WriteFile(inventoryPath, inventoryRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	decision := fillercorpus.RightsDecision{
		InventorySHA256: fillercorpus.InventorySHA256(inventoryRaw), CaseID: item.CaseID, CaptureIDs: item.CaptureIDs,
		Authority: authority, ItemID: itemID, MetadataSHA256: metadataSHA, ReviewerID: "reviewer",
		ReviewedAt: at.Add(time.Minute), Decision: "held", Basis: "held fixture rights",
	}
	decisionRaw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	decisionsPath := filepath.Join(root, "decisions.jsonl")
	if err := os.WriteFile(decisionsPath, append(decisionRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Build(Config{
		InventoryPath: inventoryPath, RightsDecisionsPath: decisionsPath, RightsProfile: fillercorpus.RightsProfileDevelopment,
		SourceRoot: root, PriorAdjudicationPaths: []string{filepath.Join(root, "prior.json")}, GeneratedAt: at.Add(time.Hour),
	})
	if err == nil || !strings.Contains(err.Error(), "materialization, quarantine download, and inspection") {
		t.Fatalf("missing acquisition authority error = %v", err)
	}
}

func localCandidateFixture(t *testing.T, root string) (fillercorpus.InventoryCase, fillercorpus.RightsDecision, fillerreview.ReplacementCandidateReviewCase, fillerquarantine.RightsEligibility) {
	t.Helper()
	at := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	media := []byte("complete local audiovisual fixture")
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), media, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rights.txt"), []byte("rights"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "provenance.txt"), []byte("provenance"), 0o600); err != nil {
		t.Fatal(err)
	}
	mediaSHA := fillercandidatepool.Digest(media)
	metadataSHA := strings.Repeat("a", 64)
	representation := fillercorpus.InventoryRepresentation{
		Transport: fillercorpus.TransportLocal, Name: "clip.mp4", Path: "clip.mp4", MIMEType: "video/mp4",
		Bytes: int64(len(media)), SHA256: mediaSHA, DurationMS: 30_000,
	}
	representation.Soundtrack = fillercorpus.BindInventorySoundtrack(representation, fillercorpus.SoundtrackPresentExpected, fillercorpus.SoundtrackEvidenceFirstPartyMetadata, metadataSHA, "fixture metadata")
	authority, itemID, role := "fixture.local", "clip", "commercial"
	captureID := fillercorpus.NewCaptureID(authority, "", role)
	item := fillercorpus.InventoryCase{
		CaseID: fillercorpus.CaseID(authority, itemID), CaptureIDs: []string{captureID}, Authority: authority, ItemID: itemID,
		Title: "Clip", RoleHints: []string{role}, RightsAssertions: []string{"fixture"},
		ItemURL: "https://fixture.local/items/clip", MetadataURL: "https://fixture.local/metadata/clip",
		MetadataRetrievedAt: at, MetadataSHA256: metadataSHA, Representation: representation,
		Evidence: []fillercorpus.InventoryEvidence{
			{Kind: "provenance", Path: "provenance.txt", Bytes: 10, SHA256: fillercandidatepool.Digest([]byte("provenance"))},
			{Kind: "rights", Path: "rights.txt", Bytes: 6, SHA256: fillercandidatepool.Digest([]byte("rights"))},
		},
	}
	inventory := fillercorpus.Inventory{
		SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: at,
		Captures: []fillercorpus.Capture{{CaptureID: captureID, Transport: fillercorpus.TransportLocal, Authority: authority, RoleHint: role, SnapshotAt: at, MaxPredictedMediaBytes: int64(len(media)), PredictedMediaBytes: int64(len(media)), MaxWallTimeMS: 1}},
		Cases:    []fillercorpus.InventoryCase{item},
	}
	raw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := fillerquarantine.OpenRightsEligibility(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	decision := fillercorpus.RightsDecision{
		InventorySHA256: fillercorpus.InventorySHA256(raw), CaseID: item.CaseID, CaptureIDs: item.CaptureIDs,
		Authority: authority, ItemID: itemID, MetadataSHA256: metadataSHA, ReviewerID: "reviewer",
		ReviewedAt: at.Add(time.Minute), Decision: "approved", Basis: "reviewed fixture rights", Redistributable: true,
	}
	roleValue := fillereval.TemporalRoleCommercial
	review := fillerreview.ReplacementCandidateReviewCase{
		CaseID: item.CaseID, EvidenceAlias: "evidence-clip", InspectedSourceFile: "clip.mp4", InspectedSourceSHA: mediaSHA,
		ReviewedMediaPath: "clip.mp4", ReviewedMediaSHA: mediaSHA, ReviewedMediaBytes: int64(len(media)), DurationMS: 30_000,
		Unit: fillereval.UnitStandalone, Role: &roleValue, TechnicalVerdict: "continue", HadAudio: true, FullDecodeMeasured: true,
		Suitability: "candidate_no_signal_observed", FamilyID: "singleton-fixture",
		Transition: fillerreview.TemporalTransitionAuthorityCase{
			EvidenceAlias: "evidence-clip", CaseID: item.CaseID, SourceSHA256: mediaSHA, DurationMS: 30_000,
			Head: fillerreview.TemporalTransitionEdge{StartMS: 0, EndMS: 1_000}, Tail: fillerreview.TemporalTransitionEdge{StartMS: 29_000, EndMS: 30_000},
		},
	}
	return item, decision, review, quarantine
}
