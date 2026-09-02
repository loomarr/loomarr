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
	const (
		model       = "hf.co/loomarr/gemma:Q4_K_M"
		sourceRepo  = "loomarr/gemma-gguf"
		ggufFile    = "gemma-Q4_K_M.gguf"
		quant       = "Q4_K_M"
		ollama      = "0.15.1"
		macOS       = "27.0"
		macOSBuild  = "26A123"
		hardware    = "Macmini11,1"
		chip        = "Apple M5 Pro"
		licenseID   = "Gemma"
		template    = "template"
		licenseText = "Gemma"
	)
	modelDigest := strings.Repeat("a", 64)
	sourceRevision := strings.Repeat("b", 40)
	ggufDigest := strings.Repeat("c", 64)
	modelfile := "FROM /Users/test/sha256-" + ggufDigest + "\nPARAMETER num_ctx 8192\n"
	card := []byte(`{"schemaVersion":10,"corpusVersion":"planner-certification-v3","profile":"m5-pro-gemma","generator":{"provider":"ollama","model":"` + model + `"},"contract":{"corpusVersion":"planner-certification-v3","catalogFixtureSha256":"` + strings.Repeat("1", 64) + `","promptVersion":"planner-prompt-v1","toolSchemaVersion":"planner-tools-v1","scorerVersion":"planner-scorer-v3"},"assessment":{"performance":{"resourceStatus":"measured","resourceSource":"ollama:/api/ps","peakRamBytes":2147483648,"peakVramBytes":10737418240}},"cases":[{"case":"one","trials":3}]}`)
	scorecardPath := filepath.Join(root, "scorecard.json")
	if err := os.WriteFile(scorecardPath, card, 0o600); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, "evidence")
	if err := os.Mkdir(evidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	evidence := map[string][]byte{
		"huggingface-model.json":     []byte(`{"id":"` + sourceRepo + `","sha":"` + sourceRevision + `","cardData":{"license":"` + licenseID + `"},"siblings":[{"rfilename":"` + ggufFile + `","lfs":{"sha256":"` + ggufDigest + `"}}]}`),
		"gguf-sha256.txt":            []byte(ggufDigest + "  /Users/test/" + ggufFile + "\n"),
		"ollama-version.json":        []byte(`{"version":"` + ollama + `"}`),
		"ollama-list.json":           []byte(`{"models":[{"name":"` + model + `","model":"` + model + `","digest":"` + modelDigest + `","details":{"quantization_level":"` + quant + `"}}]}`),
		"ollama-load-request.json":   []byte(`{"model":"` + model + `","prompt":"","stream":false,"keep_alive":"30m","options":{"num_ctx":8192}}`),
		"ollama-show-request.json":   []byte(`{"model":"` + model + `"}`),
		"ollama-show.json":           []byte(`{"license":"` + licenseText + `","modelfile":"FROM /Users/test/sha256-` + ggufDigest + `\nPARAMETER num_ctx 8192\n","template":"` + template + `","details":{"quantization_level":"` + quant + `"}}`),
		"ollama-ps-cold-before.json": []byte(`{"models":[]}`),
		"ollama-ps-warm-before.json": []byte(`{"models":[{"name":"` + model + `","model":"` + model + `","digest":"` + modelDigest + `","size":12884901888,"size_vram":10737418240,"context_length":8192}]}`),
		"ollama-ps-after.json":       []byte(`{"models":[{"name":"` + model + `","model":"` + model + `","digest":"` + modelDigest + `","size":12884901888,"size_vram":10737418240,"context_length":8192}]}`),
		"sw-vers.txt":                []byte("ProductName:\t\tmacOS\nProductVersion:\t\t" + macOS + "\nBuildVersion:\t\t" + macOSBuild + "\n"),
		"system-profiler.json":       []byte(`{"SPHardwareDataType":[{"machine_model":"` + hardware + `","chip_type":"` + chip + `"}]}`),
		"sysctl-hw-memsize.txt":      []byte("68719476736\n"),
		"uname.txt":                  []byte("arm64\n"),
	}
	declared := make([]map[string]any, 0, len(evidence))
	for kind, raw := range evidence {
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
			"tag": model, "ollamaDigest": modelDigest,
			"sourceRepository": sourceRepo, "sourceRevision": sourceRevision,
			"ggufFile": ggufFile, "ggufSha256": ggufDigest,
			"quantization": quant, "contextLength": 8192,
			"templateSha256": hashText(template), "modelfileSha256": hashText(modelfile),
			"licenseId": licenseID, "licenseSha256": hashText(licenseText),
		},
		"runtime": map[string]any{
			"ollamaVersion": ollama, "macosVersion": macOS, "macosBuild": macOSBuild,
			"architecture": "arm64", "hardwareModel": hardware, "chip": chip,
			"physicalUnifiedMemoryBytes": int64(64 << 30),
		},
		"protocol": map[string]any{
			"profile": "m5-pro-gemma", "contextLength": 8192, "maxOutputTokens": 2048,
			"temperature": 0.2, "seed": nil, "coldStarts": 1, "warmupLoads": 1, "measuredWarmTrials": 3,
		},
		"residency": map[string]any{
			"coldBefore": map[string]any{"selectedModelResident": false},
			"warmBefore": map[string]any{
				"selectedModelResident": true, "model": model, "ollamaDigest": modelDigest,
				"ramBytes": int64(2 << 30), "vramBytes": int64(10 << 30),
			},
			"after": map[string]any{
				"selectedModelResident": true, "model": model, "ollamaDigest": modelDigest,
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

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
