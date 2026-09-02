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

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerreference"
)

func TestRunRequiresEveryBoundInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "fixed generated-at") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestValidateAcquiredSourcesBindsExactOpenedBytes(t *testing.T) {
	root := t.TempDir()
	data := []byte("exact acquired bytes")
	if err := os.WriteFile(filepath.Join(root, "source.mp4"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	ledger := fillerreference.DownloadLedger{Cases: []fillerreference.DownloadCase{{
		CaseID: "case-a", LocalFile: "source.mp4", ContentSHA256: hex.EncodeToString(digest[:]),
		Representation: fillercorpus.InventoryRepresentation{Bytes: int64(len(data))},
	}}}
	if err := validateAcquiredSources(root, ledger); err != nil {
		t.Fatal(err)
	}
	ledger.Cases[0].ContentSHA256 = strings.Repeat("0", 64)
	if err := validateAcquiredSources(root, ledger); err == nil {
		t.Fatal("source hash mismatch accepted")
	}
}

func TestValidateAcquiredSourcesRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "source.mp4")); err != nil {
		t.Fatal(err)
	}
	ledger := fillerreference.DownloadLedger{Cases: []fillerreference.DownloadCase{{
		CaseID: "case-a", LocalFile: "source.mp4", ContentSHA256: strings.Repeat("0", 64),
		Representation: fillercorpus.InventoryRepresentation{Bytes: 7},
	}}}
	if err := validateAcquiredSources(root, ledger); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestPublishIsImmutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := publish(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := publish(path, []byte("second")); err == nil {
		t.Fatal("existing audit overwritten")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("published data=%q err=%v", data, err)
	}
}

func TestRunBindsRawContentReviewBytesAndReproducesExactOutput(t *testing.T) {
	fixture := writeRunFixture(t)
	first := filepath.Join(fixture.root, "audit-first.json")
	second := filepath.Join(fixture.root, "audit-second.json")

	for _, output := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if code := run(fixture.args(output), &stdout, &stderr); code != 0 {
			t.Fatalf("run(%s) code=%d stdout=%q stderr=%q", filepath.Base(output), code, stdout.String(), stderr.String())
		}
	}
	firstRaw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("same locked inputs and generated-at produced different output bytes")
	}
	var audit fillerreference.Audit
	if err := json.Unmarshal(firstRaw, &audit); err != nil {
		t.Fatal(err)
	}
	reviewRaw, err := os.ReadFile(fixture.contentReview)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Inputs.ContentReviewSHA256 != fillerreference.SHA256(reviewRaw) || audit.Summary.Cases != 300 {
		t.Fatalf("audit binding=%+v summary=%+v", audit.Inputs, audit.Summary)
	}

	// A byte-only, semantically neutral mutation must produce a different bound
	// input identity even though the decoded authority is unchanged.
	if err := os.WriteFile(fixture.contentReview, append(reviewRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	third := filepath.Join(fixture.root, "audit-third.json")
	var stdout, stderr bytes.Buffer
	if code := run(fixture.args(third), &stdout, &stderr); code != 0 {
		t.Fatalf("raw-mutated run code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	thirdRaw, err := os.ReadFile(third)
	if err != nil {
		t.Fatal(err)
	}
	var changed fillerreference.Audit
	if err := json.Unmarshal(thirdRaw, &changed); err != nil {
		t.Fatal(err)
	}
	if changed.Inputs.ContentReviewSHA256 == audit.Inputs.ContentReviewSHA256 || bytes.Equal(thirdRaw, firstRaw) {
		t.Fatal("raw content-review byte mutation did not change the bound audit identity")
	}
}

func TestRunRejectsNonStrictContentReviewJSON(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"unknown field", func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"findings"`), []byte(`"unexpectedAuthority":true,"findings"`), 1)
		}},
		{"trailing value", func(raw []byte) []byte { return append(raw, []byte("\n{}\n")...) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeRunFixture(t)
			raw, err := os.ReadFile(fixture.contentReview)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture.contentReview, test.mutate(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(fixture.root, "must-not-publish.json")
			var stdout, stderr bytes.Buffer
			if code := run(fixture.args(output), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "content review") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid input published output: %v", err)
			}
		})
	}
}

func TestRunRejectsDuplicateJSONObjectKeysAcrossBoundInputs(t *testing.T) {
	tests := []struct {
		name string
		path func(runFixture) string
	}{
		{"manifest", func(f runFixture) string { return f.manifest }},
		{"packet JSONL", func(f runFixture) string { return f.packets }},
		{"mapping", func(f runFixture) string { return f.mapping }},
		{"download ledger", func(f runFixture) string { return f.downloads }},
		{"content review", func(f runFixture) string { return f.contentReview }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeRunFixture(t)
			path := test.path(fixture)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mutated := bytes.Replace(raw, []byte(`"schemaVersion":`), []byte(`"schemaVersion":1,"schemaVersion":`), 1)
			if bytes.Equal(mutated, raw) {
				t.Fatal("fixture has no schemaVersion key")
			}
			if err := os.WriteFile(path, mutated, 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(fixture.root, "must-not-publish.json")
			var stdout, stderr bytes.Buffer
			if code := run(fixture.args(output), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "duplicate JSON object key") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("duplicate-key input published output: %v", err)
			}
		})
	}
}

func TestRunNeverOverwritesPublishedAudit(t *testing.T) {
	fixture := writeRunFixture(t)
	output := filepath.Join(fixture.root, "immutable.json")
	var stdout, stderr bytes.Buffer
	if code := run(fixture.args(output), &stdout, &stderr); code != 0 {
		t.Fatalf("first run code=%d stderr=%q", code, stderr.String())
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(fixture.args(output), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "publish immutable audit") {
		t.Fatalf("second run code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("second run changed immutable output")
	}
}

type runFixture struct {
	root, manifest, packets, mapping, downloads, contentReview, sources string
}

func (f runFixture) args(output string) []string {
	return []string{
		"-manifest", f.manifest,
		"-packets", f.packets,
		"-mapping", f.mapping,
		"-download-ledger", f.downloads,
		"-content-review", f.contentReview,
		"-source-root", f.sources,
		"-output", output,
		"-generated-at", "2026-08-31T20:00:00Z",
	}
}

func writeRunFixture(t *testing.T) runFixture {
	t.Helper()
	root := t.TempDir()
	sources := filepath.Join(root, "sources")
	if err := os.Mkdir(sources, 0o750); err != nil {
		t.Fatal(err)
	}
	lockedAt := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	manifest := fillereval.Manifest{
		SchemaVersion: fillereval.SchemaVersion, Kind: fillereval.CorpusDevelopmentSeed,
		CorpusVersion: "run-fixture", LockedAt: lockedAt,
		SliceGates: []fillereval.SliceGate{{Slice: "contract", MinCases: 300, MinAccuracy: .99}},
	}
	ledger := fillerreference.DownloadLedger{
		SchemaVersion: 1, InventorySHA256: strings.Repeat("d", 64), GeneratedAt: lockedAt,
		MaxRequests: 1, MaxItems: 300, MaxBytes: 1_000_000,
	}
	var packetLines bytes.Buffer
	packetEncoder := json.NewEncoder(&packetLines)
	for index := 0; index < 300; index++ {
		id := fmt.Sprintf("fixture/candidate-%03d", index)
		if index == 0 {
			id = "archive.org/classic_tv_commercials/VID20181114WA0037"
		}
		data := []byte(fmt.Sprintf("exact source bytes %03d", index))
		digest := sha256.Sum256(data)
		contentSHA := hex.EncodeToString(digest[:])
		localFile := fmt.Sprintf("%03d.mp4", index)
		if err := os.WriteFile(filepath.Join(sources, localFile), data, 0o600); err != nil {
			t.Fatal(err)
		}
		evidence := []fillereval.Evidence{{ID: "role", Kind: "review", Claim: "content_role", Value: filleradmission.RoleInterstitial, Provenance: "blind review"}}
		if index == 0 {
			evidence = []fillereval.Evidence{
				{ID: "frame-a", Kind: string(filleradmission.KindFrame), Claim: "casual candid activity", Value: "person outdoors", Provenance: "bound/frame-a.jpg"},
				{ID: "transcript-a", Kind: string(filleradmission.KindTranscript), Claim: "casual speech", Value: "see you soon", Provenance: "bound/audio.wav#transcript"},
			}
		}
		packet := fillerbakeoff.Packet{
			SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: id,
			EvidenceVersion: fillerreference.PacketEvidenceVersion, ContentSHA256: contentSHA,
			Facts: []filleradmission.Evidence{
				{ID: "media", Claim: filleradmission.ClaimMediaUsability, Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder, Source: "decoder", Location: "source_duration_ms=30500;segment_start_ms=0;segment_duration_ms=30000;no_video=false;no_audio=false;black_percent=0;silence_percent=0"},
				{ID: "license", Claim: filleradmission.ClaimSourceLicense, Value: filleradmission.EligibilityEligible, Kind: filleradmission.KindSourcePolicy, Source: "rights"},
			},
			Signals: []fillerbakeoff.Signal{{ID: "video", Kind: "video", DurationMS: 30_000, Width: 1280, Height: 720}},
		}
		item := fillereval.Case{
			ID: id, Split: fillereval.SplitDevelopment, Cluster: id, ContentSHA256: contentSHA,
			Source: "fixture/source", License: "CC0-1.0", Truth: fillereval.TruthEligible,
			ContentRole: filleradmission.RoleInterstitial, Taxonomy: map[string][]string{}, Slices: []string{"contract"}, Evidence: evidence,
			Provenance: fillereval.MediaProvenance{
				Authority: "archive.org/fixture", ItemID: id, ItemRef: "https://example.invalid/items/" + id,
				MetadataRetrievedAt: lockedAt, MetadataSHA256: strings.Repeat("e", 64), EvidenceRef: "https://example.invalid/evidence/" + id,
				LicenseURL: "https://example.invalid/license/" + id, RightsStatement: "fixture rights", RightsDecision: "approved",
				RightsReviewerID: "rights", RightsReviewedAt: lockedAt, Redistributable: true,
				SourceFilename: localFile, SourceRef: "https://example.invalid/media/" + id, SourceBytes: int64(len(data)), SegmentDurationMS: 30_000,
			},
			EvidenceSHA256: fillerbakeoff.PacketSHA256(packet),
		}
		labelSHA := fillereval.LabelSHA256(item)
		item.LabelReviews = []fillereval.LabelReview{
			{ReviewerID: "reviewer-a", BatchID: "batch-a", ReviewedAt: lockedAt, Independent: true, SubmissionSHA256: labelSHA},
			{ReviewerID: "reviewer-b", BatchID: "batch-b", ReviewedAt: lockedAt, Independent: true, SubmissionSHA256: labelSHA},
		}
		manifest.Cases = append(manifest.Cases, item)
		ledger.Cases = append(ledger.Cases, fillerreference.DownloadCase{
			CaseID: id, Authority: item.Provenance.Authority, ItemID: item.Provenance.ItemID,
			LicenseURL: item.Provenance.LicenseURL, ItemURL: item.Provenance.ItemRef, MetadataURL: item.Provenance.EvidenceRef,
			MetadataRetrievedAt: lockedAt, MetadataSHA256: item.Provenance.MetadataSHA256,
			Representation: fillercorpus.InventoryRepresentation{Name: localFile, URL: item.Provenance.SourceRef, Bytes: int64(len(data))},
			LocalFile:      localFile, ContentSHA256: contentSHA,
			Approval: fillercorpus.RightsDecision{
				InventorySHA256: ledger.InventorySHA256, CaseID: id, CaptureIDs: []string{"fixture/capture"},
				Authority: item.Provenance.Authority, ItemID: item.Provenance.ItemID, MetadataSHA256: item.Provenance.MetadataSHA256,
				ReviewerID: "rights", ReviewedAt: lockedAt, Decision: "approved", Basis: "fixture", Redistributable: true,
			},
			VerifiedAt: lockedAt,
		})
		ledger.Bytes += int64(len(data))
		if err := packetEncoder.Encode(packet); err != nil {
			t.Fatal(err)
		}
	}

	fixture := runFixture{
		root: root, manifest: filepath.Join(root, "manifest.json"), packets: filepath.Join(root, "packets.jsonl"),
		mapping: filepath.Join(root, "mapping.json"), downloads: filepath.Join(root, "downloads.json"),
		contentReview: filepath.Join(root, "content-review.json"), sources: sources,
	}
	manifestRaw := mustJSON(t, manifest)
	mapping := fillerreference.MappingArtifact{SchemaVersion: 1, MappingVersion: "mapping-v1", SourceManifestSHA256: fillerreference.SHA256(manifestRaw)}
	review := fillerreference.ContentReviewArtifact{
		SchemaVersion: 1, Kind: fillerreference.ContentReviewKind, ContractVersion: fillerreference.ContractVersion,
		ReviewerID: "contract-reviewer", ReviewedAt: lockedAt, SourceManifestSHA256: fillerreference.SHA256(manifestRaw),
		Findings: []fillerreference.ContentFinding{{
			ContentSHA256: manifest.Cases[0].ContentSHA256, Disposition: string(fillerreference.DispositionExclude),
			ReasonCode: fillerreference.ReasonNonBroadcastMaterial, Detail: "bound content is not broadcast material",
			EvidenceRefs: []string{"frame-a", "transcript-a"},
		}},
	}
	writeFixtureFile(t, fixture.manifest, manifestRaw)
	writeFixtureFile(t, fixture.packets, packetLines.Bytes())
	writeFixtureFile(t, fixture.mapping, mustJSON(t, mapping))
	writeFixtureFile(t, fixture.downloads, mustJSON(t, ledger))
	writeFixtureFile(t, fixture.contentReview, mustJSON(t, review))
	return fixture
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeFixtureFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
