package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillerquarantine"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestRunLocksCompleteSpreadsheetReviewToDownloaderJSONL(t *testing.T) {
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "inventory.json")
	worksheetPath := filepath.Join(dir, "worksheet.json")
	csvPath := filepath.Join(dir, "completed.csv")
	approvalsPath := filepath.Join(dir, "approvals.jsonl")
	inspectionPath := filepath.Join(dir, "inspection.json")
	heldInspectionPath := filepath.Join(dir, "held-inspection.json")
	metadataDigest := strings.Repeat("a", 64)
	retrievedAt := "2026-08-25T08:00:00Z"
	reviewedAt := "2026-08-25T09:00:00Z"
	retrievedTime, _ := time.Parse(time.RFC3339, retrievedAt)
	authority := "archive.org/prelinger"
	captureID := fillercorpus.NewCaptureID(authority, "prelinger", "commercial")
	item := fillercorpus.InventoryCase{CaseID: fillercorpus.CaseID(authority, "soda-ad"), CaptureIDs: []string{captureID}, Authority: authority, ItemID: "soda-ad", Title: "Mountain Dew", RoleHints: []string{"commercial"}, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/", RightsAssertions: []string{"CC0"}, ItemURL: "https://archive.org/details/soda-ad", MetadataURL: "https://archive.org/metadata/soda-ad", MetadataRetrievedAt: retrievedTime, MetadataSHA256: metadataDigest, AllowedMediaHosts: []string{"archive.org", ".archive.org"}, Representation: fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportHTTPS, Name: "soda.mp4", URL: "https://archive.org/download/soda-ad/soda.mp4", MIMEType: "video/mp4", Origin: "original", Bytes: 1024}}
	item.Representation = lockTestSoundtrack(item.Representation, fillercorpus.SoundtrackPresentExpected, metadataDigest)
	inventory := fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: retrievedTime, Captures: []fillercorpus.Capture{{CaptureID: captureID, Transport: fillercorpus.TransportHTTPS, Authority: authority, Collection: "prelinger", RoleHint: "commercial", SnapshotAt: retrievedTime, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 2048, ResponseBytes: 100, MaxPredictedMediaBytes: 2048, PredictedMediaBytes: 1024, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []fillercorpus.InventoryCase{item}}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	if err := os.WriteFile(inventoryPath, inventoryRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	reportRaw := testkit.FillerQuarantineReport(t, inventoryRaw, map[string]string{item.CaseID: fillerquarantine.DispositionEligibleForRightsReview}, nil)
	if err := os.WriteFile(inspectionPath, reportRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	heldReportRaw := testkit.FillerQuarantineReport(t, inventoryRaw, map[string]string{item.CaseID: fillerquarantine.DispositionHold}, nil)
	if err := os.WriteFile(heldInspectionPath, heldReportRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	authorityView, err := fillerquarantine.OpenRightsEligibility(inventoryRaw, reportRaw)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := authorityView.Selected(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	rowValue := fillercorpus.RightsReviewRowFromCase(item)
	rowValue.Rank, rowValue.InventorySHA256 = 1, digest
	rowValue.QuarantineInspection = selection.Cases[0].QuarantineInspection
	worksheet := fillercorpus.RightsWorksheet{SchemaVersion: fillercorpus.RightsWorksheetSchemaVersion, Profile: fillercorpus.RightsProfileDevelopment, InventorySHA256: digest, SnapshotAt: retrievedTime, PreparedAt: retrievedTime.Add(30 * time.Minute), MinItems: 1, MaxItems: 1, QuarantineInspection: selection.QuarantineInspection, Cases: []fillercorpus.RightsReviewRow{rowValue}}
	raw, err := json.Marshal(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worksheetPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	completed := append(fillercorpus.ImmutableRightsReviewRecordForProfile(rowValue, fillercorpus.RightsProfileDevelopment), "rights-reviewer", reviewedAt, "approved", "CC0 dedication and item inspection permit redistribution.", "true", "", "[]")
	if err := writer.WriteAll([][]string{fillercorpus.RightsReviewCSVHeader(), completed}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for name, reportPath := range map[string]string{
		"held":  heldInspectionPath,
		"drift": writeReportFixture(t, dir, "drifted-inspection.json", append(append([]byte(nil), reportRaw...), '\n')),
	} {
		t.Run(name+" report fails closed", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			output := filepath.Join(dir, name+"-approvals.jsonl")
			code := run([]string{
				"--inventory", inventoryPath, "--quarantine-inspection", reportPath,
				"--worksheet", worksheetPath, "--completed-csv", csvPath, "--approvals-out", output,
				"--locked-at", "2026-08-25T10:00:00Z", "--profile", "development",
			}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failed lock published authority: %v", err)
			}
		})
	}
	for name, mutate := range map[string]func(*fillercorpus.RightsWorksheet){
		"missing global binding": func(value *fillercorpus.RightsWorksheet) { value.QuarantineInspection = nil },
		"changed case binding": func(value *fillercorpus.RightsWorksheet) {
			value.Cases[0].QuarantineInspection.ContentSHA256 = strings.Repeat("f", 64)
		},
	} {
		t.Run(name+" fails closed", func(t *testing.T) {
			worksheetCopyRaw, err := json.Marshal(worksheet)
			if err != nil {
				t.Fatal(err)
			}
			var changed fillercorpus.RightsWorksheet
			if err := json.Unmarshal(worksheetCopyRaw, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			changedRaw, _ := json.Marshal(changed)
			changedPath := writeReportFixture(t, dir, strings.ReplaceAll(name, " ", "-")+".json", changedRaw)
			output := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".jsonl")
			var stdout, stderr bytes.Buffer
			code := run([]string{
				"--inventory", inventoryPath, "--quarantine-inspection", inspectionPath,
				"--worksheet", changedPath, "--completed-csv", csvPath, "--approvals-out", output,
				"--locked-at", "2026-08-25T10:00:00Z", "--profile", "development",
			}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--inventory", inventoryPath,
		"--quarantine-inspection", inspectionPath,
		"--worksheet", worksheetPath,
		"--completed-csv", csvPath,
		"--approvals-out", approvalsPath,
		"--locked-at", "2026-08-25T10:00:00Z",
		"--profile", "development",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}
	approvalRaw, err := os.ReadFile(approvalsPath)
	if err != nil {
		t.Fatal(err)
	}
	var approval struct {
		WorksheetSchemaVersion int                                           `json:"worksheetSchemaVersion"`
		InventorySHA256        string                                        `json:"inventorySha256"`
		CaseID                 string                                        `json:"caseId"`
		MetadataSHA256         string                                        `json:"metadataSha256"`
		ReviewerID             string                                        `json:"reviewerId"`
		ReviewedAt             time.Time                                     `json:"reviewedAt"`
		Decision               string                                        `json:"decision"`
		Redistributable        bool                                          `json:"redistributable"`
		QuarantineInspection   *fillercorpus.QuarantineInspectionCaseBinding `json:"quarantineInspection"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(approvalRaw), &approval); err != nil {
		t.Fatal(err)
	}
	if approval.WorksheetSchemaVersion != fillercorpus.RightsWorksheetSchemaVersion || approval.InventorySHA256 != digest || approval.CaseID != item.CaseID || approval.MetadataSHA256 != metadataDigest || approval.ReviewerID != "rights-reviewer" || approval.Decision != "approved" || !approval.Redistributable || approval.QuarantineInspection == nil || approval.QuarantineInspection.Report.ReportSHA256 != fillercorpus.InventorySHA256(reportRaw) {
		t.Fatalf("unexpected locked approval: %+v", approval)
	}
}

func TestTemporalReplacementRightsLockPreservesSoundtrackHold(t *testing.T) {
	retrieved := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	representation := lockTestSoundtrack(fillercorpus.InventoryRepresentation{
		Transport: fillercorpus.TransportHTTPS, Name: "silent.mp4", URL: "https://tile.loc.gov/silent.mp4",
		MIMEType: "video/mp4", Bytes: 1024,
	}, fillercorpus.SoundtrackIntentionallySilent, strings.Repeat("a", 64))
	row := fillercorpus.RightsReviewRow{
		InventorySHA256: strings.Repeat("f", 64), CaseID: "loc.gov/national-screening-room/00694220",
		CaptureIDs: []string{"capture"}, Authority: "loc.gov/national-screening-room", ItemID: "00694220",
		MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: retrieved, Representation: representation,
	}
	heldFields := []string{
		"rights-reviewer", retrieved.Add(time.Minute).Format(time.RFC3339), "held", "full-source decode establishes intentional silence",
		"false", "false", "false", "false", "false", "false", "false", "false", "false", "", "[]",
	}
	decision, err := parseQuarantineDecision(row, heldFields, retrieved.Add(2*time.Minute), fillercorpus.QuarantinePurposeTemporalStructureReplacement)
	if err != nil {
		t.Fatal(err)
	}
	if decision.QuarantineContract == nil || !slices.Contains(decision.QuarantineContract.HoldReasons, fillercorpus.HoldReasonSoundtrackIntentionallySilent) {
		t.Fatalf("locked decision = %+v", decision)
	}

	approvedFields := append([]string(nil), heldFields...)
	approvedFields[2] = "approved"
	approvedFields[4], approvedFields[5] = "true", "true"
	if _, err := parseQuarantineDecision(row, approvedFields, retrieved.Add(2*time.Minute), fillercorpus.QuarantinePurposeTemporalStructureReplacement); err == nil || !strings.Contains(err.Error(), fillercorpus.HoldReasonSoundtrackIntentionallySilent) {
		t.Fatalf("approved silent soundtrack error = %v", err)
	}
}

func TestMetBatchCompletionStillPassesThroughOrdinaryItemLevelLocker(t *testing.T) {
	dir := t.TempDir()
	snapshot := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	collection := "selection-sha256:" + strings.Repeat("1", 64)
	role := "policy-positive-nomination"
	captureID := fillercorpus.NewCaptureID(fillercorpus.MetAuthority, collection, role)
	item := fillercorpus.InventoryCase{
		CaseID: fillercorpus.CaseID(fillercorpus.MetAuthority, "195733"), CaptureIDs: []string{captureID},
		Authority: fillercorpus.MetAuthority, ItemID: "195733", Title: "Venus", RoleHints: []string{role},
		Collection: []string{"Metropolitan Museum of Art", "search-term:venus"}, Creator: []string{"Artist"},
		SubjectTerms: []string{"Female Nudes"}, SourceFamily: "met-object:195733", Date: "1900",
		RightsAssertions: []string{"Met object record isPublicDomain=true."},
		ItemURL:          "https://www.metmuseum.org/art/collection/search/195733", MetadataURL: "https://collectionapi.metmuseum.org/public/collection/v1/objects/195733",
		MetadataCache: "objects/example.json", MetadataRetrievedAt: snapshot.Add(-time.Minute), MetadataSHA256: strings.Repeat("a", 64),
		AllowedMediaHosts: []string{"images.metmuseum.org"},
		Representation:    fillercorpus.InventoryRepresentation{Transport: fillercorpus.TransportHTTPS, Name: "image.jpg", URL: "https://images.metmuseum.org/image.jpg", MIMEType: "image/jpeg", Bytes: 100},
	}
	item.Representation = lockTestSoundtrack(item.Representation, fillercorpus.SoundtrackIntentionallySilent, item.MetadataSHA256)
	inventory := fillercorpus.Inventory{
		SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: snapshot,
		Captures: []fillercorpus.Capture{{
			CaptureID: captureID, Transport: fillercorpus.TransportHTTPS, Authority: fillercorpus.MetAuthority, Collection: collection, RoleHint: role,
			SnapshotAt: snapshot, MaxRequests: 3, RequestsUsed: 3, MaxResponseBytes: 1024, ResponseBytes: 100,
			MaxPredictedMediaBytes: 100, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10,
		}},
		Cases: []fillercorpus.InventoryCase{item},
	}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	inventoryDigest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	inspectionRaw := testkit.FillerQuarantineReport(t, inventoryRaw, map[string]string{
		item.CaseID: fillerquarantine.DispositionEligibleForRightsReview,
	}, nil)
	authority, err := fillerquarantine.OpenRightsEligibility(inventoryRaw, inspectionRaw)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := authority.Selected(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	row := fillercorpus.RightsReviewRowFromCase(item)
	row.Rank, row.InventorySHA256 = 1, inventoryDigest
	row.QuarantineInspection = selection.Cases[0].QuarantineInspection
	worksheet := fillercorpus.RightsWorksheet{
		SchemaVersion: fillercorpus.RightsWorksheetSchemaVersion, Profile: fillercorpus.RightsProfileDevelopment,
		InventorySHA256: inventoryDigest, SnapshotAt: snapshot, PreparedAt: snapshot.Add(time.Minute), MinItems: 1, MaxItems: 1,
		Instructions: []string{"Review the exact item."}, QuarantineInspection: selection.QuarantineInspection, Cases: []fillercorpus.RightsReviewRow{row},
	}
	worksheetRaw, err := json.Marshal(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	prescreen := fillercorpus.MetRightsPrescreen{
		SchemaVersion: fillercorpus.MetRightsPrescreenSchemaVersion, InventorySHA256: inventoryDigest,
		PolicyEvidenceID: "met-open-access-metadata-prescreen-v1", PolicyEvidenceSHA256: strings.Repeat("b", 64),
		PolicySources: []fillercorpus.MetOpenAccessPolicySource{
			{Kind: "api_documentation", URL: "https://metmuseum.github.io/", SHA256: "037f875cd22180ecb31a67cb38707ce2ea88eb7087c2f81edd27a0a1aa56dd6a"},
			{Kind: "openaccess_license", URL: "https://raw.githubusercontent.com/metmuseum/openaccess/6fa206f0df6cf349d4fe558028d4c08e95f44eb6/LICENSE", SHA256: "36ffd9dc085d529a7e60e1276d73ae5a030b020313e6c5408593a6ae2af39673", Commit: "6fa206f0df6cf349d4fe558028d4c08e95f44eb6"},
			{Kind: "openaccess_readme", URL: "https://raw.githubusercontent.com/metmuseum/openaccess/6fa206f0df6cf349d4fe558028d4c08e95f44eb6/README.md", SHA256: "26f24c669b3eb888a02498113dc94feb2674ee9d007a1d470c13be36413a29c2", Commit: "6fa206f0df6cf349d4fe558028d4c08e95f44eb6"},
		},
		Limitations: []string{"cc0_does_not_resolve_non_copyright_rights", "dataset_cc0_does_not_license_images", "metadata_prescreen_is_not_rights_approval", "source_policy_pages_require_independent_review"},
		PreparedAt:  snapshot.Add(2 * time.Minute), MinItems: 1, MaxItems: 1, TotalCases: 1, PassedCases: 1, CompleteCoverage: true,
		Instructions: []string{
			"This is a mechanical metadata pre-screen, not a rights approval or legal conclusion.",
			"Inspect every held row; passing rows still require the existing independent item-level rights decision.",
			"No result grants download, truth, training, production, scheduling, or broadcast authority.",
		},
		Cases: []fillercorpus.MetRightsPrescreenCase{{CaseID: item.CaseID, MetadataSHA256: item.MetadataSHA256, Status: "met_metadata_prescreen_pass"}},
	}
	prescreenRaw, err := json.Marshal(prescreen)
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := fillercorpus.PrepareMetRightsBatchAttestation(inventoryRaw, worksheetRaw, prescreenRaw)
	if err != nil {
		t.Fatal(err)
	}
	attestation.ReviewerID = "maintainer"
	attestation.ReviewedAt = snapshot.Add(3 * time.Minute).Format(time.RFC3339)
	attestation.Acceptance = fillercorpus.MetRightsBatchAcceptanceAccepted
	attestation.Basis = "met_cc0_open_access_object_reviewed_v1: exact evidence and limitations reviewed for private development use."
	attestationRaw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := fillercorpus.CompleteMetRightsBatchReview(inventoryRaw, worksheetRaw, prescreenRaw, attestationRaw)
	if err != nil {
		t.Fatal(err)
	}
	inventoryPath := filepath.Join(dir, "inventory.json")
	worksheetPath := filepath.Join(dir, "worksheet.json")
	csvPath := filepath.Join(dir, "completed.csv")
	inspectionPath := filepath.Join(dir, "quarantine-inspection.json")
	for path, raw := range map[string][]byte{inventoryPath: inventoryRaw, worksheetPath: worksheetRaw, csvPath: completion.CompletedCSV, inspectionPath: inspectionRaw} {
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lockDecisionsForProfile(inventoryPath, worksheetPath, csvPath, snapshot.Add(4*time.Minute), fillercorpus.RightsProfileDevelopment); err == nil {
		t.Fatal("ordinary locker accepted the remote Met batch without its quarantine inspection")
	}
	driftedInspectionPath := writeReportFixture(t, dir, "drifted-quarantine-inspection.json", append(append([]byte(nil), inspectionRaw...), '\n'))
	if _, err := lockDecisionsForProfile(inventoryPath, worksheetPath, csvPath, snapshot.Add(4*time.Minute), fillercorpus.RightsProfileDevelopment, driftedInspectionPath); err == nil {
		t.Fatal("ordinary locker accepted a drifted quarantine inspection")
	}
	changedWorksheet := worksheet
	changedWorksheet.Cases = append([]fillercorpus.RightsReviewRow(nil), worksheet.Cases...)
	changedCaseBinding := *changedWorksheet.Cases[0].QuarantineInspection
	changedCaseBinding.ContentSHA256 = strings.Repeat("f", 64)
	changedWorksheet.Cases[0].QuarantineInspection = &changedCaseBinding
	changedWorksheetRaw, _ := json.Marshal(changedWorksheet)
	changedWorksheetPath := writeReportFixture(t, dir, "changed-binding-worksheet.json", changedWorksheetRaw)
	if _, err := lockDecisionsForProfile(inventoryPath, changedWorksheetPath, csvPath, snapshot.Add(4*time.Minute), fillercorpus.RightsProfileDevelopment, inspectionPath); err == nil {
		t.Fatal("ordinary locker accepted a changed quarantine case binding")
	}
	if _, err := fillercorpus.CompleteMetRightsBatchReview(inventoryRaw, changedWorksheetRaw, prescreenRaw, attestationRaw); err == nil {
		t.Fatal("batch completion accepted a changed report binding after attestation")
	}
	decisions, err := lockDecisionsForProfile(inventoryPath, worksheetPath, csvPath, snapshot.Add(4*time.Minute), fillercorpus.RightsProfileDevelopment, inspectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].CaseID != item.CaseID || decisions[0].Decision != "approved" || !decisions[0].Redistributable ||
		!strings.Contains(decisions[0].Basis, completion.AttestationSHA256) {
		t.Fatalf("decisions = %+v", decisions)
	}
	if err := os.WriteFile(inventoryPath, append(inventoryRaw, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lockDecisionsForProfile(inventoryPath, worksheetPath, csvPath, snapshot.Add(4*time.Minute), fillercorpus.RightsProfileDevelopment, inspectionPath); err == nil {
		t.Fatal("ordinary locker accepted a batch review after inventory drift")
	}
}

func writeReportFixture(t *testing.T, dir, name string, raw []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunRefusesExistingApprovalsBeforeReadingInputs(t *testing.T) {
	dir := t.TempDir()
	approvalsPath := filepath.Join(dir, "approvals.jsonl")
	original := []byte("existing locked authority\n")
	if err := os.WriteFile(approvalsPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--inventory", filepath.Join(dir, "missing-inventory.json"),
		"--worksheet", filepath.Join(dir, "missing-worksheet.json"),
		"--completed-csv", filepath.Join(dir, "missing-review.csv"),
		"--approvals-out", approvalsPath,
		"--locked-at", "2026-09-05T12:00:00Z",
		"--profile", "quarantine",
	}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "approvals output already exists") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if got, err := os.ReadFile(approvalsPath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("existing approvals changed: got=%q err=%v", got, err)
	}
}

func TestDecodeWorksheetRejectsUnknownAndTrailingFields(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown":  `{"schemaVersion":3,"unknown":true}`,
		"trailing": `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeWorksheet([]byte(raw)); err == nil {
				t.Fatal("non-strict worksheet was accepted")
			}
		})
	}
}

func TestWriteJSONLCannotReplacePublishedApprovals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.jsonl")
	first := []fillercorpus.RightsDecision{{CaseID: "first"}}
	if err := writeJSONL(path, first); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONL(path, []fillercorpus.RightsDecision{{CaseID: "replacement"}}); err == nil {
		t.Fatal("immutable rights decisions were replaced")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("published approvals changed: got=%q err=%v", got, err)
	}
}

func TestParseDecisionRejectsIncompleteOrInconsistentAuthority(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	lockedAt := retrievedAt.Add(2 * time.Hour)
	row := fillercorpus.RightsReviewRow{MetadataRetrievedAt: retrievedAt, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/"}
	valid := []string{"rights-reviewer", retrievedAt.Add(time.Hour).Format(time.RFC3339), "approved", "Exact item inspection confirms CC0 redistribution.", "true", "", "[]"}
	tests := map[string]func([]string){
		"missing reviewer":           func(fields []string) { fields[0] = "" },
		"review before metadata":     func(fields []string) { fields[1] = retrievedAt.Add(-time.Minute).Format(time.RFC3339) },
		"review after lock":          func(fields []string) { fields[1] = lockedAt.Add(time.Minute).Format(time.RFC3339) },
		"unknown decision":           func(fields []string) { fields[2] = "maybe" },
		"missing basis":              func(fields []string) { fields[3] = "" },
		"invalid redistribution":     func(fields []string) { fields[4] = "yes" },
		"approval without authority": func(fields []string) { fields[4] = "false" },
		"malformed restrictions":     func(fields []string) { fields[6] = "none" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fields := append([]string(nil), valid...)
			mutate(fields)
			if _, err := parseDecision(row, fields, lockedAt); err == nil {
				t.Fatal("invalid rights authority was accepted")
			}
		})
	}
	t.Run("held row grants redistribution", func(t *testing.T) {
		fields := append([]string(nil), valid...)
		fields[2] = "held"
		if _, err := parseDecision(row, fields, lockedAt); err == nil {
			t.Fatal("held row granted redistribution authority")
		}
	})
	t.Run("attribution license lacks credit", func(t *testing.T) {
		byRow := row
		byRow.LicenseURL = "https://creativecommons.org/licenses/by/4.0/"
		if _, err := parseDecision(byRow, valid, lockedAt); err == nil {
			t.Fatal("attribution-bearing approval without credit was accepted")
		}
	})
}

func TestParseQuarantineDecisionAllowsOnlyLocalCopyAndInspection(t *testing.T) {
	retrievedAt := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	lockedAt := retrievedAt.Add(2 * time.Hour)
	row := fillercorpus.RightsReviewRow{CaseID: "loc/item", MetadataRetrievedAt: retrievedAt}
	valid := []string{
		"rights-reviewer", retrievedAt.Add(time.Hour).Format(time.RFC3339), "approved", "Exact source terms permit a local quarantine copy and inspection.",
		"true", "true", "false", "false", "false", "false", "false", "false", "false", "", "[]",
	}
	decision, err := parseQuarantineDecision(row, valid, lockedAt, fillercorpus.QuarantinePurposeLocalInspection)
	if err != nil || decision.QuarantineContract == nil || decision.Redistributable || len(decision.QuarantineContract.HoldReasons) != 0 {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	for index, name := range []string{"provider transfer", "redistribution", "corpus preparation", "training", "catalog ingestion", "scheduling", "production admission"} {
		t.Run(name, func(t *testing.T) {
			fields := append([]string(nil), valid...)
			fields[6+index] = "true"
			if _, err := parseQuarantineDecision(row, fields, lockedAt, fillercorpus.QuarantinePurposeLocalInspection); err == nil {
				t.Fatal("downstream authority was accepted")
			}
		})
	}
	for _, index := range []int{4, 5} {
		fields := append([]string(nil), valid...)
		fields[index] = "false"
		if _, err := parseQuarantineDecision(row, fields, lockedAt, fillercorpus.QuarantinePurposeLocalInspection); err == nil {
			t.Fatalf("required local authority field %d was omitted", index)
		}
	}
	t.Run("held row remains inert", func(t *testing.T) {
		fields := append([]string(nil), valid...)
		fields[2] = "held"
		for index := 4; index <= 12; index++ {
			fields[index] = "false"
		}
		decision, err := parseQuarantineDecision(row, fields, lockedAt, fillercorpus.QuarantinePurposeLocalInspection)
		if err != nil || decision.QuarantineContract == nil || len(decision.QuarantineContract.HoldReasons) == 0 {
			t.Fatalf("held decision = %+v, %v", decision, err)
		}
	})
}

func TestParseHoldoutDecisionRequiresEveryIndependentAuthorityAxis(t *testing.T) {
	retrievedAt := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	lockedAt := retrievedAt.Add(2 * time.Hour)
	row := fillercorpus.RightsReviewRow{CaseID: "direct/one", MetadataRetrievedAt: retrievedAt}
	template := &fillercorpus.HoldoutRightsTemplate{AgreementID: "agreement-v1", AgreementSHA256: strings.Repeat("a", 64), ProcessorID: "openrouter/vertex", ProcessorTermsSHA256: strings.Repeat("b", 64)}
	valid := []string{
		"rights-reviewer", retrievedAt.Add(time.Hour).Format(time.RFC3339), "approved", "Executed schedule and every evidence bundle inspected.",
		"schedule-one", strings.Repeat("c", 64), fillercorpus.RightsStatusCleared, strings.Repeat("d", 64),
		"true", "true", "true", "true", "true",
		fillercorpus.RightsStatusNotPresent, fillercorpus.RightsStatusCleared, fillercorpus.RightsStatusNotPresent, fillercorpus.RightsStatusCleared, fillercorpus.RightsStatusCleared, fillercorpus.RightsStatusNotPresent, strings.Repeat("e", 64),
		fillercorpus.RedistributionExternalOnly, fillercorpus.RightsTerritoryWorldwide, fillercorpus.RightsTermPerpetualIrrevocable, "", fillercorpus.RightsWithdrawalDefectRetirement,
		"", "", "", "", "[]",
	}
	decision, err := parseHoldoutDecision(row, template, valid, lockedAt)
	if err != nil || decision.HoldoutContract == nil || len(decision.HoldoutContract.HoldReasons) != 0 || decision.Redistributable {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
	tests := map[string]func([]string){
		"schedule digest mismatch": func(fields []string) { fields[5] = "bad" },
		"unknown signer authority": func(fields []string) { fields[6] = fillercorpus.RightsStatusUnknown },
		"provider grant missing":   func(fields []string) { fields[12] = "false" },
		"embedded rights conflict": func(fields []string) { fields[13] = fillercorpus.RightsStatusConflicting },
		"expired term": func(fields []string) {
			fields[22] = fillercorpus.RightsTermExpires
			fields[23] = lockedAt.Add(-time.Minute).Format(time.RFC3339)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fields := append([]string(nil), valid...)
			mutate(fields)
			if _, err := parseHoldoutDecision(row, template, fields, lockedAt); err == nil {
				t.Fatal("incomplete certification authority was approved")
			}
		})
	}
	t.Run("blank held schedule emits reasons", func(t *testing.T) {
		fields := append([]string(nil), valid...)
		fields[2] = "held"
		for index := 4; index < len(fields); index++ {
			fields[index] = ""
		}
		decision, err := parseHoldoutDecision(row, template, fields, lockedAt)
		if err != nil || decision.HoldoutContract == nil || len(decision.HoldoutContract.HoldReasons) == 0 {
			t.Fatalf("held decision = %+v, %v", decision, err)
		}
	})
}

func lockTestSoundtrack(representation fillercorpus.InventoryRepresentation, status, evidenceSHA256 string) fillercorpus.InventoryRepresentation {
	representation.Soundtrack = fillercorpus.BindInventorySoundtrack(representation, status, fillercorpus.SoundtrackEvidenceFirstPartyMetadata, evidenceSHA256, "fixture metadata")
	return representation
}
