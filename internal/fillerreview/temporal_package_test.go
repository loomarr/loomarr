package fillerreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadTemporalReviewPackageVerifiesFilesAndExposesSignals(t *testing.T) {
	packagePath := writeTemporalTestPackage(t)
	pack, cases, packageSHA256, err := LoadTemporalReviewPackage(packagePath, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pack.BatchID != "temporal-test" || len(cases) != 1 || len(cases[0].Signals) != 3 || !reviewSHA256(packageSHA256) {
		t.Fatalf("loaded package = %+v cases=%+v sha=%q", pack, cases, packageSHA256)
	}
	if cases[0].Signals[0].Kind != "frame" || cases[0].Signals[1].Kind != "ocr" || cases[0].Signals[2].Kind != "transcript" {
		t.Fatalf("signals = %+v", cases[0].Signals)
	}

	framePath := filepath.Join(filepath.Dir(packagePath), pack.Cases[0].Frames[0].Path)
	if err := os.WriteFile(framePath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := LoadTemporalReviewPackage(packagePath, 1); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("tampered frame error = %v", err)
	}
}

func writeTemporalTestPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cases", "opaque"), 0o750); err != nil {
		t.Fatal(err)
	}
	instructions := []byte("instructions")
	template := []byte("template")
	frame := []byte("synthetic frame bytes")
	for path, data := range map[string][]byte{
		"INSTRUCTIONS.md":           instructions,
		"labels.template.jsonl":     template,
		"cases/opaque/frame-01.jpg": frame,
	} {
		if err := os.WriteFile(filepath.Join(root, path), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pack := TemporalReviewPackage{
		SchemaVersion: TemporalReviewPackageSchemaVersion, BatchID: "temporal-test", GeneratedAt: time.Unix(1, 0).UTC(), EvidenceVersion: "evidence-v1",
		TemporalManifestSHA256: strings.Repeat("a", 64), OCRManifestSHA256: strings.Repeat("b", 64),
		OCRPacketsSHA256: strings.Repeat("c", 64), SelectionSHA256: strings.Repeat("d", 64),
		InstructionsPath: "INSTRUCTIONS.md", InstructionsSHA256: hashBytes(instructions),
		TemplatePath: "labels.template.jsonl", TemplateSHA256: hashBytes(template),
		Cases: []TemporalReviewCase{{
			Alias: "opaque", DurationMS: 1_000,
			Frames: []TemporalReviewFrame{{
				ID: "frame-01", Path: "cases/opaque/frame-01.jpg", SHA256: hashBytes(frame), Bytes: int64(len(frame)), Width: 16, Height: 9,
				AtMS: 100, OCRSignalID: "ocr-frame-01", OCR: []TemporalReviewOCRObservation{{Text: "offer", Confidence: 0.9, X: 0.1, Y: 0.1, Width: 0.5, Height: 0.2}},
			}},
			TranscriptSegments: []TemporalReviewTranscript{{ID: "transcript-01", StartMS: 200, EndMS: 400, Text: "Buy now"}},
		}},
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(root, "package.json")
	if err := os.WriteFile(packagePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return packagePath
}
