package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPublishesOneImmutableProviderFreeManifest(t *testing.T) {
	root := t.TempDir()
	scorecardPath, capturePath, evidenceDir := writeFixture(t, root)
	outPath := filepath.Join(root, "out", "manifest.json")
	args := []string{
		"--scorecard", scorecardPath, "--capture", capturePath, "--evidence-dir", evidenceDir,
		"--generated-at", "2026-10-15T15:01:00Z", "--out", outPath,
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run code = %d, stderr = %s", code, stderr.String())
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"contract": "planner-reference-host-v1"`)) ||
		!strings.Contains(stdout.String(), "sha256=") {
		t.Fatalf("output = %s, stdout = %s", raw, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "file exists") {
		t.Fatalf("second run code = %d, stderr = %s", code, stderr.String())
	}
	again, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatal("immutable publication changed after a second run")
	}
}

func TestReadEvidenceDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "ollama-show.json")); err != nil {
		t.Fatal(err)
	}
	_, err := readEvidenceDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("error = %v, want symlink refusal", err)
	}
}

func writeFixture(t *testing.T, root string) (string, string, string) {
	t.Helper()
	const model = "hf.co/loomarr/gemma:Q4_K_M"
	card := []byte(`{"schemaVersion":10,"corpusVersion":"planner-certification-v3","profile":"m5-pro-gemma","generator":{"provider":"ollama","model":"` + model + `"},"contract":{"corpusVersion":"planner-certification-v3","catalogFixtureSha256":"` + strings.Repeat("1", 64) + `","promptVersion":"planner-prompt-v1","toolSchemaVersion":"planner-tools-v1","scorerVersion":"planner-scorer-v3"},"assessment":{"performance":{"resourceStatus":"measured","resourceSource":"ollama:/api/ps","peakRamBytes":2147483648,"peakVramBytes":10737418240}},"cases":[{"case":"one","trials":3}]}`)
	scorecardPath := filepath.Join(root, "scorecard.json")
	if err := os.WriteFile(scorecardPath, card, 0o600); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, "evidence")
	if err := os.Mkdir(evidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	kinds := []string{
		"ollama-list.json", "ollama-ps-after.json", "ollama-ps-cold-before.json",
		"ollama-ps-warm-before.json", "ollama-show.json", "ollama-version.txt",
		"sw-vers.txt", "system-profiler.json", "uname.txt",
	}
	declared := make([]map[string]any, 0, len(kinds))
	for _, kind := range kinds {
		raw := []byte("bounded raw capture for " + kind)
		if err := os.WriteFile(filepath.Join(evidenceDir, kind), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		declared = append(declared, map[string]any{"kind": kind, "sha256": hex.EncodeToString(sum[:]), "bytes": len(raw)})
	}
	cardSum := sha256.Sum256(card)
	capture := map[string]any{
		"schemaVersion": 1, "contract": "planner-reference-host-v1", "runId": "m5-pro-gemma-q4",
		"startedAt": "2026-10-15T14:00:00Z", "completedAt": "2026-10-15T15:00:00Z",
		"scorecardSha256": hex.EncodeToString(cardSum[:]), "scorecardBytes": len(card),
		"model": map[string]any{
			"tag": model, "ollamaDigest": strings.Repeat("a", 64),
			"sourceRepository": "loomarr/gemma-gguf", "sourceRevision": strings.Repeat("b", 40),
			"ggufFile": "gemma-Q4_K_M.gguf", "ggufSha256": strings.Repeat("c", 64),
			"quantization": "Q4_K_M", "contextLength": 8192,
			"templateSha256": strings.Repeat("d", 64), "modelfileSha256": strings.Repeat("e", 64),
			"licenseId": "Gemma", "licenseSha256": strings.Repeat("f", 64),
		},
		"runtime": map[string]any{
			"ollamaVersion": "0.15.1", "macosVersion": "27.0", "macosBuild": "26A123",
			"architecture": "arm64", "hardwareModel": "Macmini11,1", "chip": "Apple M5 Pro",
			"physicalUnifiedMemoryBytes": int64(64 << 30),
		},
		"protocol": map[string]any{
			"profile": "m5-pro-gemma", "contextLength": 8192, "maxOutputTokens": 2048,
			"temperature": 0, "seed": 42, "coldRuns": 1, "unreportedWarmups": 1, "measuredWarmTrials": 3,
		},
		"residency": map[string]any{
			"coldBefore": map[string]any{"selectedModelResident": false},
			"warmBefore": map[string]any{
				"selectedModelResident": true, "model": model, "ollamaDigest": strings.Repeat("a", 64),
				"ramBytes": int64(2 << 30), "vramBytes": int64(10 << 30),
			},
			"after": map[string]any{
				"selectedModelResident": true, "model": model, "ollamaDigest": strings.Repeat("a", 64),
				"ramBytes": int64(2 << 30), "vramBytes": int64(10 << 30),
			},
		},
		"evidence": declared,
	}
	captureRaw, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "capture.json")
	if err := os.WriteFile(capturePath, captureRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	return scorecardPath, capturePath, evidenceDir
}
