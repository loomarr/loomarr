package fillercorpus

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPilotReviewRoundTripQualifiesOnlyReviewedYield(t *testing.T) {
	pilotRaw, preparedAt := pilotReviewFixture(t)
	sheet, err := PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Cases) != 50 || sheet.Cases[0].Rank != 1 || sheet.Cases[49].Rank != 50 {
		t.Fatalf("worksheet cases = %d", len(sheet.Cases))
	}
	worksheetRaw, err := json.Marshal(sheet)
	if err != nil {
		t.Fatal(err)
	}
	completed := completedPilotReviewCSV(t, sheet, func(index int, record []string) {
		record[19] = "reviewer-independent"
		record[20] = preparedAt.Add(time.Hour).Format(time.RFC3339)
		record[21] = "true"
		if index%10 < 5 {
			record[22] = "approved"
			record[23] = "true"
			record[25] = "true"
			record[26] = "Source creator and collection attribution"
		} else {
			record[22] = "held"
			record[23] = "false"
			record[25] = "false"
		}
		record[24] = "reviewed exact source rights and product role"
		record[27] = "[]"
	})
	result, err := LockPilotReview(pilotRaw, worksheetRaw, completed, preparedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.DownloadAuthority || !result.IndependentAttestation || result.ReviewerID != "reviewer-independent" || len(result.Decisions) != 50 {
		t.Fatalf("result identity = %+v", result)
	}
	for _, lane := range result.Lanes {
		if lane.CasesReviewed != 10 || lane.ApprovedAndRelevant != 5 || !lane.QualifiedForAdapter {
			t.Fatalf("lane = %+v", lane)
		}
	}
	for _, decision := range result.Decisions {
		if decision.DownloadAuthority {
			t.Fatalf("pilot decision granted download authority: %+v", decision)
		}
	}
}

func TestPilotReviewRejectsChangedEvidenceAndFalseIndependence(t *testing.T) {
	pilotRaw, preparedAt := pilotReviewFixture(t)
	sheet, err := PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		t.Fatal(err)
	}
	worksheetRaw, _ := json.Marshal(sheet)
	completed := completedPilotReviewCSV(t, sheet, func(_ int, record []string) {
		record[19] = "reviewer"
		record[20] = preparedAt.Add(time.Hour).Format(time.RFC3339)
		record[21] = "true"
		record[22] = "held"
		record[23] = "false"
		record[24] = "not sufficiently clear"
		record[25] = "false"
		record[27] = "[]"
	})
	records := readCSV(t, completed)
	records[1][5] = "changed title"
	if _, err := LockPilotReview(pilotRaw, worksheetRaw, writeCSV(t, records), preparedAt.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("changed evidence error = %v", err)
	}
	records = readCSV(t, completed)
	records[1][21] = "false"
	if _, err := LockPilotReview(pilotRaw, worksheetRaw, writeCSV(t, records), preparedAt.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "independent") {
		t.Fatalf("false independence error = %v", err)
	}
}

func TestPilotReviewDoesNotQualifyWeakLane(t *testing.T) {
	pilotRaw, preparedAt := pilotReviewFixture(t)
	sheet, err := PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		t.Fatal(err)
	}
	worksheetRaw, _ := json.Marshal(sheet)
	completed := completedPilotReviewCSV(t, sheet, func(index int, record []string) {
		record[19] = "reviewer"
		record[20] = preparedAt.Add(time.Hour).Format(time.RFC3339)
		record[21] = "true"
		record[22] = "approved"
		record[23] = "true"
		record[24] = "rights clear but role reviewed separately"
		record[25] = "true"
		record[26] = "Source creator and collection attribution"
		record[27] = "[]"
		if index < 10 && index%10 >= 4 {
			record[23] = "false"
		}
	})
	result, err := LockPilotReview(pilotRaw, worksheetRaw, completed, preparedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Lanes[0].QualifiedForAdapter || result.Lanes[0].ApprovedAndRelevant != 4 {
		t.Fatalf("weak lane = %+v", result.Lanes[0])
	}
	for _, lane := range result.Lanes[1:] {
		if !lane.QualifiedForAdapter {
			t.Fatalf("strong lane = %+v", lane)
		}
	}
}

func TestPilotReviewCSVNeutralizesFormulaLeadingText(t *testing.T) {
	pilotRaw, preparedAt := pilotReviewFixture(t)
	var pilot Pilot
	if err := json.Unmarshal(pilotRaw, &pilot); err != nil {
		t.Fatal(err)
	}
	pilot.Lanes[0].Cases[0].Title = "=IMPORTDATA(\"https://example.invalid\")"
	pilotRaw, _ = json.Marshal(pilot)
	sheet, err := PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := PilotReviewCSV(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if got := readCSV(t, data)[1][5]; !strings.HasPrefix(got, "'=") {
		t.Fatalf("title = %q", got)
	}
}

func pilotReviewFixture(t *testing.T) ([]byte, time.Time) {
	t.Helper()
	snapshot := time.Date(2026, 8, 26, 13, 31, 0, 0, time.UTC)
	pilot := Pilot{SchemaVersion: PilotSchemaVersion, SnapshotAt: snapshot, LockedAt: snapshot.Add(time.Minute)}
	for laneIndex, authority := range PilotAuthorities {
		lane := Lane{Authority: authority, MaxRequests: 20, RequestsUsed: 10, MaxResponseBytes: 1_000_000, ResponseBytes: 1000, MaxPredictedMediaBytes: 1_000_000, MaxWallTimeMS: 60_000, WallTimeMS: 1000}
		for caseIndex := range 10 {
			id := strings.ReplaceAll(authority, "/", "-") + "-" + string(rune('a'+caseIndex))
			candidate := Candidate{
				ItemID: id, Title: "Candidate " + id, RoleHints: []string{"commercial"},
				ItemURL: "https://example.com/item/" + id, MetadataURL: "https://example.com/meta/" + id,
				MetadataRetrievedAt: snapshot.Add(-time.Duration(laneIndex+caseIndex+1) * time.Minute),
				MetadataSHA256:      strings.Repeat(string(rune('a'+laneIndex)), 64), RightsAssertions: []string{"source assertion"},
				Representation: Representation{Name: id + ".mp4", URL: "https://example.com/media/" + id, MIMEType: "video/mp4", Bytes: 1000},
			}
			if authority == "commons.wikimedia.org" {
				candidate.DiscoveryPath = []string{"Category:Advertising videos"}
			}
			lane.Cases = append(lane.Cases, candidate)
			lane.PredictedMediaBytes += candidate.Representation.Bytes
		}
		pilot.Lanes = append(pilot.Lanes, lane)
	}
	raw, err := json.Marshal(pilot)
	if err != nil {
		t.Fatal(err)
	}
	return raw, pilot.LockedAt.Add(time.Minute)
}

func completedPilotReviewCSV(t *testing.T, sheet PilotReviewWorksheet, fill func(int, []string)) []byte {
	t.Helper()
	data, err := PilotReviewCSV(sheet)
	if err != nil {
		t.Fatal(err)
	}
	records := readCSV(t, data)
	for index := range records[1:] {
		fill(index, records[index+1])
	}
	return writeCSV(t, records)
}

func readCSV(t *testing.T, data []byte) [][]string {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func writeCSV(t *testing.T, records [][]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.WriteAll(records); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
