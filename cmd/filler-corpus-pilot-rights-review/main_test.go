package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPreparesLockedPilotPacket(t *testing.T) {
	temp := t.TempDir()
	jsonPath := filepath.Join(temp, "worksheet.json")
	csvPath := filepath.Join(temp, "worksheet.csv")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--pilot", "../../internal/fillercorpus/corpus/pilot/locked.json",
		"--out", jsonPath,
		"--csv-out", csvPath,
		"--prepared-at", "2026-08-26T14:06:00Z",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var sheet struct {
		PilotSHA256 string            `json:"pilotSha256"`
		Cases       []json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(raw, &sheet); err != nil {
		t.Fatal(err)
	}
	if len(sheet.PilotSHA256) != 64 || len(sheet.Cases) != 50 {
		t.Fatalf("sheet = %+v", sheet)
	}
	if info, err := os.Stat(csvPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("CSV mode: info=%v err=%v", info, err)
	}
}
