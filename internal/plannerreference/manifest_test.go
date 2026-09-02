package plannerreference

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildManifestCanonicalizesAndBindsReferenceHostEvidence(t *testing.T) {
	card, captured, evidence, generatedAt := validFixture(t)
	artifact, err := BuildManifest(rawInputs(t, card, captured, evidence, generatedAt))
	if err != nil {
		t.Fatal(err)
	}
	again, err := BuildManifest(rawInputs(t, card, captured, evidence, generatedAt))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(artifact.JSON, again.JSON) || artifact.SHA256 != again.SHA256 {
		t.Fatal("identical raw inputs did not produce an identical manifest")
	}
	sum := sha256.Sum256(artifact.JSON)
	if artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("artifact digest = %q, want SHA-256 of canonical JSON", artifact.SHA256)
	}
	if !bytes.HasSuffix(artifact.JSON, []byte("\n")) {
		t.Fatal("canonical manifest lacks terminal newline")
	}
	text := string(artifact.JSON)
	for _, want := range []string{
		`"contract": "planner-reference-host-v1"`,
		`"generatorProvider": "ollama"`,
		`"generatorModel": "hf.co/loomarr/gemma:Q4_K_M"`,
		`"physicalUnifiedMemoryBytes": 68719476736`,
		`"kind": "ollama-list.json"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("manifest missing %s", want)
		}
	}
	if strings.Contains(text, "/Users/") || strings.Contains(text, "unrelated-resident-model") ||
		strings.Contains(text, "raw capture") {
		t.Fatal("manifest leaked raw capture content or local paths")
	}
}

func TestBuildManifestRejectsArtifactScorecardAndEvidenceDrift(t *testing.T) {
	card, captured, evidence, generatedAt := validFixture(t)

	t.Run("scorecard bytes", func(t *testing.T) {
		mutated := append(bytes.Clone(card), ' ')
		_, err := BuildManifest(RawInputs{
			Scorecard: mutated, Capture: encodeCapture(t, card, captured),
			Evidence: evidence, GeneratedAt: generatedAt,
		})
		assertErrorContains(t, err, "scorecard digest")
	})
	t.Run("scorecard model", func(t *testing.T) {
		mutated := bytes.Replace(card, []byte(`"hf.co/loomarr/gemma:Q4_K_M"`), []byte(`"other:model"`), 1)
		_, err := BuildManifest(rawInputs(t, mutated, captured, evidence, generatedAt))
		assertErrorContains(t, err, "scorecard profile or generator")
	})
	t.Run("scorecard profile", func(t *testing.T) {
		mutated := bytes.Replace(card, []byte(`"m5-pro-gemma"`), []byte(`"another-profile"`), 1)
		_, err := BuildManifest(rawInputs(t, mutated, captured, evidence, generatedAt))
		assertErrorContains(t, err, "scorecard profile or generator")
	})
	t.Run("raw evidence", func(t *testing.T) {
		mutated := cloneEvidence(evidence)
		mutated["ollama-show.json"] = []byte("changed")
		_, err := BuildManifest(rawInputs(t, card, captured, mutated, generatedAt))
		assertErrorContains(t, err, "digest or byte count")
	})
	t.Run("missing evidence", func(t *testing.T) {
		mutated := cloneEvidence(evidence)
		delete(mutated, "uname.txt")
		_, err := BuildManifest(rawInputs(t, card, captured, mutated, generatedAt))
		assertErrorContains(t, err, "incomplete")
	})
}

func TestBuildManifestRejectsMutableOrInconsistentModelAndHostEvidence(t *testing.T) {
	card, captured, evidence, generatedAt := validFixture(t)
	tests := map[string]struct {
		mutate func(*capture)
		want   string
	}{
		"mutable tag":       {func(c *capture) { c.Model.Tag = "gemma-latest" }, "explicit immutable tag"},
		"source revision":   {func(c *capture) { c.Model.SourceRevision = "main" }, "sourceRevision"},
		"gguf path":         {func(c *capture) { c.Model.GGUFFile = "../model.gguf" }, "bounded basename"},
		"quantization":      {func(c *capture) { c.Model.Quantization = "q4 maybe" }, "quantization"},
		"context mismatch":  {func(c *capture) { c.Protocol.ContextLength = 16384 }, "context/output"},
		"host architecture": {func(c *capture) { c.Runtime.Architecture = "amd64" }, "arm64"},
		"host memory":       {func(c *capture) { c.Runtime.PhysicalUnifiedMemoryBytes = 32 << 30 }, "64..512 GiB"},
		"cold resident":     {func(c *capture) { c.Residency.ColdBefore.SelectedModelResident = true }, "selected model was absent"},
		"warm absent":       {func(c *capture) { c.Residency.WarmBefore.SelectedModelResident = false }, "warmBefore"},
		"warm mismatch":     {func(c *capture) { c.Residency.WarmBefore.VRAMBytes++ }, "residency disagree"},
		"cold runs":         {func(c *capture) { c.Protocol.ColdRuns = 0 }, "cold/warm run counts"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mutated := captured
			test.mutate(&mutated)
			_, err := BuildManifest(rawInputs(t, card, mutated, evidence, generatedAt))
			assertErrorContains(t, err, test.want)
		})
	}
}

func TestBuildManifestRejectsMalformedAdversarialAndOverBoundInputs(t *testing.T) {
	card, captured, evidence, generatedAt := validFixture(t)
	validCapture := encodeCapture(t, card, captured)

	tests := map[string]struct {
		card    []byte
		capture []byte
		want    string
	}{
		"unknown capture field": {
			card: card, capture: bytes.Replace(validCapture, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"unknown":true`), 1), want: "unknown field",
		},
		"duplicate capture field": {
			card: card, capture: bytes.Replace(validCapture, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1), want: "duplicate object key",
		},
		"trailing capture": {card: card, capture: append(bytes.Clone(validCapture), []byte(`{}`)...), want: "trailing JSON value"},
		"duplicate scorecard field": {
			card:    bytes.Replace(card, []byte(`"schemaVersion":10`), []byte(`"schemaVersion":10,"schemaVersion":10`), 1),
			capture: validCapture, want: "duplicate object key",
		},
		"trailing scorecard": {card: append(bytes.Clone(card), []byte(`{}`)...), capture: validCapture, want: "trailing JSON value"},
		"over-bound capture": {card: card, capture: bytes.Repeat([]byte("x"), maxCaptureBytes+1), want: "capture exceeds"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := BuildManifest(RawInputs{Scorecard: test.card, Capture: test.capture, Evidence: evidence, GeneratedAt: generatedAt})
			assertErrorContains(t, err, test.want)
		})
	}
}

func validFixture(t *testing.T) ([]byte, capture, map[string][]byte, time.Time) {
	t.Helper()
	const (
		modelTag = "hf.co/loomarr/gemma:Q4_K_M"
		ram      = int64(2 << 30)
		vram     = int64(10 << 30)
	)
	card := []byte(`{"schemaVersion":10,"corpusVersion":"planner-certification-v3","profile":"m5-pro-gemma","generator":{"provider":"ollama","model":"` + modelTag + `"},"contract":{"corpusVersion":"planner-certification-v3","catalogFixtureSha256":"` + strings.Repeat("1", 64) + `","promptVersion":"planner-prompt-v1","toolSchemaVersion":"planner-tools-v1","scorerVersion":"planner-scorer-v3"},"assessment":{"performance":{"resourceStatus":"measured","resourceSource":"ollama:/api/ps","peakRamBytes":2147483648,"peakVramBytes":10737418240}},"cases":[{"case":"one","trials":3},{"case":"two","trials":3}],"certified":false}`)
	evidence := make(map[string][]byte, len(requiredEvidenceKinds))
	declared := make([]evidenceReference, 0, len(requiredEvidenceKinds))
	for i := len(requiredEvidenceKinds) - 1; i >= 0; i-- {
		kind := requiredEvidenceKinds[i]
		raw := []byte("bounded raw capture for " + kind)
		evidence[kind] = raw
		sum := sha256.Sum256(raw)
		declared = append(declared, evidenceReference{Kind: kind, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))})
	}
	captured := capture{
		SchemaVersion: 1, Contract: contractVersion, RunID: "m5-pro-gemma-q4",
		StartedAt:   time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC),
		Model: modelCapture{
			Tag: modelTag, OllamaDigest: strings.Repeat("a", 64),
			SourceRepository: "loomarr/gemma-gguf", SourceRevision: strings.Repeat("b", 40),
			GGUFFile: "gemma-Q4_K_M.gguf", GGUFSHA256: strings.Repeat("c", 64),
			Quantization: "Q4_K_M", ContextLength: 8192, TemplateSHA256: strings.Repeat("d", 64),
			ModelfileSHA256: strings.Repeat("e", 64), LicenseID: "Gemma", LicenseSHA256: strings.Repeat("f", 64),
		},
		Runtime: runtimeCapture{
			OllamaVersion: "0.15.1", MacOSVersion: "27.0", MacOSBuild: "26A123",
			Architecture: "arm64", HardwareModel: "Macmini11,1", Chip: "Apple M5 Pro",
			PhysicalUnifiedMemoryBytes: 64 << 30,
		},
		Protocol: protocolCapture{
			Profile: "m5-pro-gemma", ContextLength: 8192, MaxOutputTokens: 2048,
			Temperature: 0, Seed: 42, ColdRuns: 1, UnreportedWarmups: 1, MeasuredWarmTrials: 3,
		},
		Residency: residencyCapture{
			ColdBefore: selectedResidency{},
			WarmBefore: selectedResidency{SelectedModelResident: true, Model: modelTag, OllamaDigest: strings.Repeat("a", 64), RAMBytes: ram, VRAMBytes: vram},
			After:      selectedResidency{SelectedModelResident: true, Model: modelTag, OllamaDigest: strings.Repeat("a", 64), RAMBytes: ram, VRAMBytes: vram},
		},
		Evidence: declared,
	}
	return card, captured, evidence, time.Date(2026, 10, 15, 15, 1, 0, 0, time.UTC)
}

func rawInputs(t *testing.T, card []byte, captured capture, evidence map[string][]byte, generatedAt time.Time) RawInputs {
	t.Helper()
	return RawInputs{Scorecard: card, Capture: encodeCapture(t, card, captured), Evidence: evidence, GeneratedAt: generatedAt}
}

func encodeCapture(t *testing.T, card []byte, captured capture) []byte {
	t.Helper()
	sum := sha256.Sum256(card)
	captured.ScorecardSHA256 = hex.EncodeToString(sum[:])
	captured.ScorecardBytes = int64(len(card))
	raw, err := json.Marshal(captured)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneEvidence(source map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(source))
	for key, value := range source {
		out[key] = bytes.Clone(value)
	}
	return out
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}
