package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestRunRequiresLockedInputsAndCredential(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--manifest") {
		t.Fatalf("missing inputs: code=%d stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	code := run([]string{"--manifest", "m", "--packets", "p", "--config", "c", "--snapshot", "s", "--corpus-root", "r", "--predictions", "o"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") {
		t.Fatalf("missing key: code=%d stderr=%s", code, stderr.String())
	}
}

func TestReadConfigRejectsUnknownAndTrailingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"run":{},"policy":{},"routes":[],"legacy":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fillerbakeoffio.ReadStrictJSON[bakeoffConfigFile](path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"run":{},"policy":{},"routes":[]} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fillerbakeoffio.ReadStrictJSON[bakeoffConfigFile](path); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing error = %v", err)
	}
}

func TestReadPacketsRejectsDuplicateCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packets.jsonl")
	packet := fillerbakeoff.Packet{SchemaVersion: fillerbakeoff.PacketSchemaVersion, CaseID: "case-1"}
	var line bytes.Buffer
	if err := writeJSONLine(&line, packet); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), line.Bytes()...)
	line.Write(first)
	if err := os.WriteFile(path, line.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fillerbakeoffio.ReadPackets(path); err == nil || !strings.Contains(err.Error(), "duplicate case") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadTranscriptsRejectsDuplicateCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcripts.jsonl")
	transcript := fillerbakeoff.TranscriptArtifact{SchemaVersion: fillerbakeoff.TranscriptSchemaVersion, CaseID: "case-1"}
	var lines bytes.Buffer
	if err := writeJSONLine(&lines, transcript); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), lines.Bytes()...)
	lines.Write(first)
	if err := os.WriteFile(path, lines.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fillerbakeoffio.ReadTranscripts(path); err == nil || !strings.Contains(err.Error(), "duplicate case") {
		t.Fatalf("error = %v", err)
	}
}

func TestWritePredictionsIsPrivateJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "predictions.jsonl")
	want := []fillereval.Prediction{{CaseID: "case-1", Verdict: fillereval.VerdictReview}}
	if err := fillerbakeoffio.WritePredictions(path, ".filler-bakeoff-test-*", want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if data, _ := os.ReadFile(path); !bytes.Contains(data, []byte(`"caseId":"case-1"`)) || data[len(data)-1] != '\n' {
		t.Fatalf("output = %q", data)
	}
	if err := fillerbakeoffio.WritePredictions(path, ".filler-bakeoff-test-*", []fillereval.Prediction{{CaseID: "replacement"}}); err == nil {
		t.Fatal("immutable ledger was overwritten")
	}
	if data, _ := os.ReadFile(path); !bytes.Contains(data, []byte(`"caseId":"case-1"`)) {
		t.Fatalf("original ledger changed: %q", data)
	}
}

func writeJSONLine(buffer *bytes.Buffer, value any) error {
	return json.NewEncoder(buffer).Encode(value)
}
