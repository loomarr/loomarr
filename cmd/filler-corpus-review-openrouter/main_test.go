package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func TestRunRecoversCrashStaleLockOnlyWithExactDigest(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	out := filepath.Join(t.TempDir(), "completed-review")
	checkpointDir := out + ".private"
	if err := os.Mkdir(checkpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := []byte("crash-stale-active-run\n")
	if err := os.WriteFile(filepath.Join(checkpointDir, "active-run.lock"), stale, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(stale)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--out", out, "--recover-lock-sha256", hex.EncodeToString(digest[:])}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "recovered crash-stale lock") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(checkpointDir, "active-run.lock")); !os.IsNotExist(err) {
		t.Fatalf("active lock remains: %v", err)
	}
}

func TestRunRequiresPaidReviewContract(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") || !strings.Contains(stderr.String(), "--snapshot") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunOfflineInspectionNeverRequiresProviderCredentials(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"--inspect-checkpoint", filepath.Join(t.TempDir(), "private-review-state")}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || strings.Contains(stderr.String(), "OPENROUTER_API_KEY") || !strings.Contains(stderr.String(), "offline inspection requires --package") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunOfflineInspectionSucceedsWithoutCredentialsClientNetworkOrProviderUse(t *testing.T) {
	args, checkpointPath := offlineInspectionCLIFixture(t)
	t.Setenv("OPENROUTER_API_KEY", "must-not-be-read-or-emitted")
	before, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(args, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var attestation fillerreview.OpenRouterReviewAttestation
	if err := json.Unmarshal(stdout.Bytes(), &attestation); err != nil {
		t.Fatalf("decode stdout: %v; stdout=%q", err, stdout.String())
	}
	if attestation.Status != fillerreview.OpenRouterReviewInspectionAwaitingExplicitMaintainerApproval || attestation.ProviderExecutionAuthorized || strings.Contains(stdout.String(), "must-not-be-read-or-emitted") {
		t.Fatalf("attestation=%+v stdout=%q", attestation, stdout.String())
	}
	after, err := os.ReadFile(checkpointPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("checkpoint changed: err=%v", err)
	}
}

func TestRunOfflineInspectionEmitsZeroStdoutOnValidationFailure(t *testing.T) {
	args, checkpointPath := offlineInspectionCLIFixture(t)
	if err := os.Chmod(checkpointPath, 0o400); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(args, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "mode 0600") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunOfflineInspectionEmitsNothingWithoutPrivateCheckpoint(t *testing.T) {
	args, checkpointPath := offlineInspectionCLIFixture(t)
	if err := os.Remove(checkpointPath); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(args, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "checkpoint") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func offlineInspectionCLIFixture(t *testing.T) ([]string, string) {
	t.Helper()
	root := t.TempDir()
	packageDir := filepath.Join(root, "review-package")
	checkpointDir := filepath.Join(root, "private-review-state")
	if err := os.Mkdir(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(checkpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	instructions := []byte("blind instructions\n")
	template := []byte("empty template\n")
	if err := os.WriteFile(filepath.Join(packageDir, "INSTRUCTIONS.md"), instructions, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "labels.template.jsonl"), template, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := make([]fillerreview.Case, 300)
	for index := range cases {
		cases[index] = fillerreview.Case{
			Alias: fmt.Sprintf("review-%03d", index+1), ContentSHA256: strings.Repeat("d", 64),
			EvidenceSHA256: strings.Repeat("e", 64), SegmentDurationMS: 5_000,
			DecoderFacts: []fillerreview.DecoderFact{{Claim: "media_usability", Value: "unusable", Kind: "decoder"}},
		}
	}
	manifest := fillerreview.Package{
		SchemaVersion: fillerreview.SchemaVersion, BatchID: "blind-b", DraftSHA256: strings.Repeat("b", 64),
		ReviewPacketSHA256: strings.Repeat("c", 64), EvidenceVersion: "evidence-v1",
		InstructionsPath: "INSTRUCTIONS.md", InstructionsSHA256: testSHA256(instructions),
		LabelTemplatePath: "labels.template.jsonl", LabelTemplateSHA256: testSHA256(template), Cases: cases,
	}
	manifestRaw := testJSON(t, manifest)
	manifestPath := filepath.Join(packageDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptsPath := filepath.Join(root, "transcripts.jsonl")
	if err := os.WriteFile(transcriptsPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	retrievedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snapshot := fillerbakeoff.OpenRouterSnapshot{
		SchemaVersion: fillerbakeoff.OpenRouterSnapshotSchemaVersion, SourceBaseURL: fillerbakeoff.OpenRouterBaseURL,
		RetrievedAt: retrievedAt, Requests: 3, ResponseBytes: 100,
		Models: []fillerbakeoff.OpenRouterModelSnapshot{{
			ID: "review/vendor-model", CanonicalSlug: "review/vendor-model", Name: "Reviewer", Created: 1,
			InputModalities: []string{"image", "text"}, OutputModalities: []string{"text"},
			Endpoints: []fillerbakeoff.OpenRouterEndpointSnapshot{{
				Name: "Provider Route", ModelID: "review/vendor-model", ProviderName: "Provider Route", ProviderSlug: "provider/route",
				ContextLength: 16_384, MaxCompletionTokens: 4_096, MaxPromptTokens: 16_384,
				SupportedParameters: []string{"response_format", "structured_outputs"}, Pricing: map[string]string{"prompt": "0.000001"}, ZDR: true,
			}},
		}},
	}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := os.WriteFile(snapshotPath, testJSON(t, snapshot), 0o600); err != nil {
		t.Fatal(err)
	}
	identity := map[string]any{
		"schemaVersion": 1, "packageManifestSha256": testSHA256(manifestRaw),
		"capabilitySnapshotSha256": fillerbakeoff.OpenRouterSnapshotSHA256(snapshot),
		"baseUrl":                  fillerbakeoff.OpenRouterBaseURL, "model": "review/vendor-model", "resolvedModel": "review/vendor-model",
		"upstreamProvider": "Provider Route", "upstreamProviderSlug": "provider/route",
		"promptVersion": "filler-blind-review-openrouter-v7", "promptSha256": "18b813ae107b57782145c9174afee002f03fbd55272da8369b84709a8d194185",
		"reviewerId": fillerreview.OpenRouterReviewerBID, "batchId": "blind-b", "expectedCases": 300,
		"maxRequests": 301, "maxSpendNanoUsd": 4_000_000, "maxChargeNanoUsd": 2_000_000,
	}
	checkpoint := map[string]any{
		"identity": identity,
		"attempts": []any{map[string]any{
			"alias": "review-001", "attempt": 1, "requestedAt": retrievedAt,
			"requestSha256": strings.Repeat("1", 64), "state": "failed", "latencyMs": 0,
			"chargedAmountUsd": "0.001", "chargedNanoUsd": 1_000_000,
		}},
		"submissions": []any{}, "calls": []any{},
	}
	checkpointPath := filepath.Join(checkpointDir, "checkpoint.json")
	if err := os.WriteFile(checkpointPath, testJSON(t, checkpoint), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--package", packageDir, "--transcripts", transcriptsPath, "--snapshot", snapshotPath,
		"--model", "review/vendor-model", "--provider", "Provider Route", "--provider-slug", "provider/route",
		"--reviewer-id", fillerreview.OpenRouterReviewerBID, "--inspect-checkpoint", checkpointDir,
		"--max-spend-nanousd", "4000000", "--max-charge-nanousd", "2000000",
	}
	return args, checkpointPath
}

func testJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func testSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
