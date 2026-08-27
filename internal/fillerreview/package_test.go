package fillerreview

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestBuildMaterializesBlindVerifiedReviewPackage(t *testing.T) {
	fixture := reviewFixture(t)
	output := filepath.Join(t.TempDir(), "review-package")
	result, err := Build(fixture.config(output, MaterializeHardlink))
	if err != nil {
		t.Fatal(err)
	}
	if result.Cases != fillereval.CertificationMinDevelopment || result.Files != fillereval.CertificationMinDevelopment || result.Bytes != int64(len(fixture.media))*fillereval.CertificationMinDevelopment || result.ManifestSHA256 == "" || result.LabelTemplateSHA256 == "" {
		t.Fatalf("unexpected result: %+v", result)
	}

	raw, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-case-", "secret-title-", "secret-creator", "secret-source-path", "secret-signal-", "caseId", "filename", "source_policy", "rights:"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("review manifest leaks %q", forbidden)
		}
	}
	var manifest Package
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Cases) != fillereval.CertificationMinDevelopment || manifest.EvidenceVersion != "review-evidence-v1" || manifest.Cases[0].Alias != fixture.review.Cases[0].Alias || len(manifest.Cases[0].Signals) != 1 || manifest.Cases[0].Signals[0].ID != "frame-01" || len(manifest.Cases[0].DecoderFacts) != 1 {
		t.Fatalf("unexpected package manifest: %+v", manifest.Cases[0])
	}
	sourceInfo, err := os.Stat(filepath.Join(fixture.corpusRoot, "frame.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(filepath.Join(output, filepath.FromSlash(manifest.Cases[0].Signals[0].Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, targetInfo) {
		t.Fatal("hardlink mode copied the derivative")
	}

	template, err := os.Open(filepath.Join(output, manifest.LabelTemplatePath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = template.Close() }()
	scanner := bufio.NewScanner(template)
	lines := 0
	for scanner.Scan() {
		lines++
		var submission fillereval.LabelSubmission
		if err := json.Unmarshal(scanner.Bytes(), &submission); err != nil {
			t.Fatal(err)
		}
		if submission.Alias != fixture.review.Cases[lines-1].Alias || submission.BatchID != fixture.review.BatchID || submission.ReviewerID != "" || !submission.ReviewedAt.IsZero() || submission.Labels.ContentRole != "" {
			t.Fatalf("template line %d is not an invalid alias-bound placeholder: %+v", lines, submission)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != fillereval.CertificationMinDevelopment {
		t.Fatalf("template has %d lines", lines)
	}
}

func TestBuildCopyModeCreatesIndependentVerifiedFiles(t *testing.T) {
	fixture := reviewFixture(t)
	output := filepath.Join(t.TempDir(), "review-package")
	if _, err := Build(fixture.config(output, MaterializeCopy)); err != nil {
		t.Fatal(err)
	}
	var manifest Package
	raw, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(filepath.Join(fixture.corpusRoot, "frame.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(filepath.Join(output, filepath.FromSlash(manifest.Cases[0].Signals[0].Path)))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(sourceInfo, targetInfo) {
		t.Fatal("copy mode retained the source inode")
	}
}

func TestBuildRejectsChangedDerivativeWithoutPublishing(t *testing.T) {
	fixture := reviewFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.corpusRoot, "frame.jpg"), bytes.Repeat([]byte("x"), len(fixture.media)), 0o640); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "review-package")
	if _, err := Build(fixture.config(output, MaterializeHardlink)); err == nil {
		t.Fatalf("changed derivative error = %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("failed build published output: %v", err)
	}
}

type fixturePaths struct {
	draftPath, reviewPath, mapPath, packetsPath, corpusRoot string
	review                                                  fillereval.BlindReviewPacket
	media                                                   []byte
}

func (f fixturePaths) config(output string, mode MaterializationMode) Config {
	return Config{
		DraftPath: f.draftPath, ReviewPacketPath: f.reviewPath, AliasMapPath: f.mapPath,
		EvidencePacketsPath: f.packetsPath, CorpusRoot: f.corpusRoot, OutputDir: output, Mode: mode,
	}
}

func reviewFixture(t *testing.T) fixturePaths {
	t.Helper()
	root := t.TempDir()
	corpusRoot := filepath.Join(root, "corpus")
	if err := os.MkdirAll(corpusRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	media := []byte("bounded fake jpeg bytes")
	if err := os.WriteFile(filepath.Join(corpusRoot, "frame.jpg"), media, 0o640); err != nil {
		t.Fatal(err)
	}
	mediaSHA := hashBytes(media)
	draft := fillereval.Manifest{
		SchemaVersion: fillereval.SchemaVersion, Kind: fillereval.CorpusDevelopmentSeed,
		CorpusVersion: "review-package-test", SliceGates: []fillereval.SliceGate{{Slice: "contract", MinCases: fillereval.CertificationMinDevelopment, MinAccuracy: .99}},
	}
	packets := make([]fillerbakeoff.Packet, 0, fillereval.CertificationMinDevelopment)
	for i := range fillereval.CertificationMinDevelopment {
		id := "secret-case-" + leftPad(i)
		packet := fillerbakeoff.Packet{
			SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: id, EvidenceVersion: "review-evidence-v1", ContentSHA256: mediaSHA,
			Facts: []filleradmission.Evidence{
				{ID: "media-usability", Claim: filleradmission.ClaimMediaUsability, Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder, Source: "decoder:" + id, Location: "black_percent=0;source_path=secret-source-path;silence_percent=0"},
				{ID: "source-license", Claim: filleradmission.ClaimSourceLicense, Value: filleradmission.EligibilityEligible, Kind: filleradmission.KindSourcePolicy, Source: "rights:" + id},
			},
			Signals: []fillerbakeoff.Signal{
				{ID: "filename", Kind: string(filleradmission.KindFilename), Text: "secret-title-" + leftPad(i) + ".mp4"},
				{ID: "source-metadata", Kind: string(filleradmission.KindUploaderMetadata), Text: `{"title":"secret-title-` + leftPad(i) + `","creator":"secret-creator"}`},
				{ID: "secret-signal-" + leftPad(i), Kind: string(filleradmission.KindFrame), Path: "frame.jpg", SHA256: mediaSHA, Bytes: int64(len(media)), Width: 320, Height: 180, AtMS: 500, ContentTypes: []string{"image/jpeg"}},
			},
		}
		packets = append(packets, packet)
		draft.Cases = append(draft.Cases, fillereval.Case{
			ID: id, Split: fillereval.SplitDevelopment, Cluster: "cluster-" + leftPad(i), ContentSHA256: mediaSHA,
			EvidenceSHA256: fillerbakeoff.PacketSHA256(packet), Source: "archive.org", License: "CC0-1.0",
			Provenance: fillereval.MediaProvenance{
				Authority: "archive.org", ItemID: id, ItemRef: "https://example.invalid/" + id,
				MetadataRetrievedAt: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), MetadataSHA256: strings.Repeat("a", 64),
				EvidenceRef: "private:" + id, RightsStatement: "secret", RightsDecision: "approved", RightsReviewerID: "rights-reviewer",
				RightsReviewedAt: time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC), SourceFilename: "secret-title-" + leftPad(i) + ".mp4",
				SourceRef: "https://example.invalid/media/" + id, SourceBytes: int64(len(media)), SegmentDurationMS: 1_000,
				Creator: "secret-creator", Campaign: "secret-campaign", SourceFamily: "secret-family",
			},
		})
	}
	review, mapping, failures := fillereval.PrepareBlindReview(draft, "review-package-test-a", rand.New(rand.NewSource(1))) //nolint:gosec // deterministic test entropy
	if len(failures) > 0 {
		t.Fatalf("prepare review: %v", failures)
	}
	draftPath := filepath.Join(root, "draft.json")
	reviewPath := filepath.Join(root, "review.json")
	mapPath := filepath.Join(root, "map.json")
	packetsPath := filepath.Join(root, "packets.jsonl")
	writeTestJSON(t, draftPath, draft)
	writeTestJSON(t, reviewPath, review)
	writeTestJSON(t, mapPath, mapping)
	file, err := os.OpenFile(packetsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, packet := range packets {
		if err := encoder.Encode(packet); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return fixturePaths{draftPath: draftPath, reviewPath: reviewPath, mapPath: mapPath, packetsPath: packetsPath, corpusRoot: corpusRoot, review: review, media: media}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func leftPad(value int) string {
	return fmt.Sprintf("%03d", value)
}
