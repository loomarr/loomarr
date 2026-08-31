package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestRunLocksNonAuthorizingPilotReview(t *testing.T) {
	pilotRaw, err := os.ReadFile("../../internal/fillercorpus/corpus/pilot/locked.json")
	if err != nil {
		t.Fatal(err)
	}
	preparedAt := time.Date(2026, 8, 26, 14, 6, 0, 0, time.UTC)
	sheet, err := fillercorpus.PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		t.Fatal(err)
	}
	worksheetRaw, _ := json.Marshal(sheet)
	csvRaw, err := fillercorpus.PilotReviewCSV(sheet)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvRaw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records[1:] {
		record[19] = "independent-reviewer"
		record[20] = "2026-08-26T14:07:00Z"
		record[21] = "true"
		record[22] = "held"
		record[23] = "false"
		record[24] = "insufficient rights or product relevance"
		record[25] = "false"
		record[27] = "[]"
	}
	var completed bytes.Buffer
	writer := csv.NewWriter(&completed)
	if err := writer.WriteAll(records); err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	pilotPath := filepath.Join(temp, "pilot.json")
	worksheetPath := filepath.Join(temp, "worksheet.json")
	csvPath := filepath.Join(temp, "completed.csv")
	resultPath := filepath.Join(temp, "result.json")
	for path, data := range map[string][]byte{pilotPath: pilotRaw, worksheetPath: worksheetRaw, csvPath: completed.Bytes()} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--pilot", pilotPath, "--worksheet", worksheetPath, "--completed-csv", csvPath,
		"--out", resultPath, "--locked-at", "2026-08-26T14:08:00Z",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	resultRaw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result fillercorpus.PilotReviewResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatal(err)
	}
	if result.DownloadAuthority || len(result.Decisions) != 50 {
		t.Fatalf("result = %+v", result)
	}
	for _, lane := range result.Lanes {
		if lane.QualifiedForAdapter {
			t.Fatalf("held lane qualified: %+v", lane)
		}
	}
}
