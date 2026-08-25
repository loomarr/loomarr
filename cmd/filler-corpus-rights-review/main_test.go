package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPrepareWorksheetIsDeterministicAndInert(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{SchemaVersion: 1, Source: "archive.org", Collection: "classic_tv_commercials", SnapshotAt: snapshot}
	for _, id := range []string{"third", "first", "second"} {
		inv.Cases = append(inv.Cases, candidate{
			Identifier: id, Title: id, MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: snapshot,
			File: sourceFile{Name: id + ".mp4", Bytes: 1024},
		})
	}
	first, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical input produced different worksheets")
	}
	if len(first.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(first.Cases))
	}
	for _, row := range first.Cases {
		if row.Decision != "" || row.ReviewerID != "" || row.Redistributable {
			t.Fatalf("row %s unexpectedly grants authority: %+v", row.Identifier, row)
		}
	}
}

func TestPrepareWorksheetFailsBelowMinimum(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{SchemaVersion: 1, Source: "archive.org", Collection: "small", SnapshotAt: snapshot, Cases: []candidate{{
		Identifier: "one", MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: snapshot,
		File: sourceFile{Name: "one.mp4", Bytes: 1},
	}}}
	if _, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot, 2, 10); err == nil {
		t.Fatal("undersized inventory was accepted")
	}
}
