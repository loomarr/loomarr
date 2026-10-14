package mediatools

import (
	"reflect"
	"testing"
)

func TestParseShowinfoSceneCutsOffsetsSortsAndDeduplicates(t *testing.T) {
	stderr := "pts_time:2.500 other\npts_time:0.100\npts_time:2.500\n"
	got, err := parseShowinfoSceneCuts(stderr, 1_000, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1_100, 3_500}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cuts = %v, want %v", got, want)
	}
}

func TestParseShowinfoSceneCutsRejectsTimestampOutsideSpan(t *testing.T) {
	if _, err := parseShowinfoSceneCuts("pts_time:5.000", 0, 5_000); err == nil {
		t.Fatal("out-of-span scene timestamp was accepted")
	}
}
