package fillerreview

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestBuildTemporalModelReviewPackageCreatesIndependentReproducibleBatches(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	first, firstResult := buildTemporalModelFixture(t, fixture, "model-batch-a", "panel-a", "seed-a")
	repeat, repeatResult := buildTemporalModelFixture(t, fixture, "model-batch-a", "panel-a", "seed-a")
	second, _ := buildTemporalModelFixture(t, fixture, "model-batch-b", "panel-b", "seed-b")

	firstPack, firstSignals, firstSHA, err := LoadTemporalModelReviewPackage(filepath.Join(first, "public", "manifest.json"), fillereval.TemporalTruthSelectionCases)
	if err != nil {
		t.Fatal(err)
	}
	repeatPack, _, repeatSHA, err := LoadTemporalModelReviewPackage(filepath.Join(repeat, "public", "manifest.json"), fillereval.TemporalTruthSelectionCases)
	if err != nil {
		t.Fatal(err)
	}
	secondPack, _, secondSHA, err := LoadTemporalModelReviewPackage(filepath.Join(second, "public", "manifest.json"), fillereval.TemporalTruthSelectionCases)
	if err != nil {
		t.Fatal(err)
	}
	if firstSHA != repeatSHA || firstResult.PackageSHA256 != repeatResult.PackageSHA256 || firstResult.MapSHA256 != repeatResult.MapSHA256 || !slices.Equal(temporalModelAliases(firstPack), temporalModelAliases(repeatPack)) {
		t.Fatal("the same declared model batch did not reproduce identical authority files")
	}
	if firstSHA == secondSHA || firstPack.Cases[0].Alias == secondPack.Cases[0].Alias || slices.Equal(temporalModelAliases(firstPack), temporalModelAliases(secondPack)) {
		t.Fatal("independent panel batches did not receive distinct identities and order")
	}
	if firstResult.Cases != fillereval.TemporalTruthSelectionCases || firstResult.Files != fillereval.TemporalTruthSelectionCases || firstResult.Bytes <= 0 || len(firstSignals) != fillereval.TemporalTruthSelectionCases {
		t.Fatalf("package result=%+v signals=%d", firstResult, len(firstSignals))
	}
	if firstPack.PanelSlot != "panel-a" || firstPack.EvidenceViewVersion != TemporalModelReviewEvidenceViewVersion || firstPack.QuestionVersion != TemporalHumanReviewQuestionVersion {
		t.Fatalf("model package identity = %+v", firstPack)
	}
	for index, item := range firstPack.Cases {
		if len(item.Frames) != 1 || len(firstSignals[index].Signals) != 1 || firstSignals[index].Alias != item.Alias || firstSignals[index].Signals[0].ID != "frame-01" {
			t.Fatalf("case/signals %d = %+v / %+v", index, item, firstSignals[index])
		}
	}

	sourceInfo, err := os.Stat(filepath.Join(fixture.root, "public", "frame.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	linkedInfo, err := os.Stat(filepath.Join(first, "public", filepath.FromSlash(firstPack.Cases[0].Frames[0].Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, linkedInfo) {
		t.Fatal("default model evidence is not a hard link to the sealed frame")
	}
	if _, err := os.Stat(filepath.Join(first, "public", "cases", firstPack.Cases[0].Alias, "review.mp4")); !os.IsNotExist(err) {
		t.Fatalf("model frame/OCR/transcript view unexpectedly contains review video: %v", err)
	}
	publicRaw := readTree(t, filepath.Join(first, "public"))
	for _, secret := range fixture.secrets {
		if bytes.Contains(publicRaw, []byte(secret)) {
			t.Fatalf("public model batch leaks coordinator-only value %q", secret)
		}
	}

	mapping, err := readStrictJSON[TemporalModelReviewMap](filepath.Join(first, "private", "map.json"))
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := validateTemporalModelReviewMap(firstPack, firstSHA, mapping)
	if err != nil || len(aliases) != fillereval.TemporalTruthSelectionCases {
		t.Fatalf("private model map = %d aliases, %v", len(aliases), err)
	}
}

func TestTemporalModelReviewPackageFailsClosedOnTamperAndDrift(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	root, _ := buildTemporalModelFixture(t, fixture, "model-batch-fail", "panel-a", "seed-fail")
	packagePath := filepath.Join(root, "public", "manifest.json")
	pack, _, packageSHA, err := LoadTemporalModelReviewPackage(packagePath, fillereval.TemporalTruthSelectionCases)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := readStrictJSON[TemporalModelReviewMap](filepath.Join(root, "private", "map.json"))
	if err != nil {
		t.Fatal(err)
	}

	wrongPackage := mapping
	wrongPackage.PackageSHA256 = strings.Repeat("f", 64)
	if _, err := validateTemporalModelReviewMap(pack, packageSHA, wrongPackage); err == nil {
		t.Fatal("private map with wrong package digest was accepted")
	}
	duplicate := mapping
	duplicate.Entries = slices.Clone(mapping.Entries)
	duplicate.Entries[1] = duplicate.Entries[0]
	if _, err := validateTemporalModelReviewMap(pack, packageSHA, duplicate); err == nil {
		t.Fatal("private map with duplicate aliases was accepted")
	}

	framePath := filepath.Join(root, "public", filepath.FromSlash(pack.Cases[0].Frames[0].Path))
	if err := os.WriteFile(framePath, []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := LoadTemporalModelReviewPackage(packagePath, fillereval.TemporalTruthSelectionCases); err == nil {
		t.Fatal("tampered frame was accepted")
	}
}

func TestTemporalModelReviewPackageRejectsUnknownManifestFields(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	root, _ := buildTemporalModelFixture(t, fixture, "model-batch-unknown", "panel-a", "seed-unknown")
	packagePath := filepath.Join(root, "public", "manifest.json")
	raw, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["historicalLabel"] = json.RawMessage(`"commercial"`)
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, raw, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := LoadTemporalModelReviewPackage(packagePath, fillereval.TemporalTruthSelectionCases); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
}

func buildTemporalModelFixture(t *testing.T, fixture temporalHumanReviewFixture, batch, panel, seed string) (string, TemporalModelReviewPackageResult) {
	t.Helper()
	output := filepath.Join(t.TempDir(), batch)
	result, err := BuildTemporalModelReviewPackage(TemporalModelReviewPackageConfig{
		EvidenceManifestPath: fixture.manifest, EvidencePrivateMapPath: fixture.privateMap, SelectionPath: fixture.selection,
		PanelSlot: panel, BatchID: batch, Seed: seed, PreparedAt: fixture.preparedAt, OutputDir: output,
		Materialization: TemporalHumanReviewHardlink,
	})
	if err != nil {
		t.Fatal(err)
	}
	return output, result
}

func temporalModelAliases(pack TemporalModelReviewPackage) []string {
	aliases := make([]string, 0, len(pack.Cases))
	for _, item := range pack.Cases {
		aliases = append(aliases, item.Alias)
	}
	return aliases
}
