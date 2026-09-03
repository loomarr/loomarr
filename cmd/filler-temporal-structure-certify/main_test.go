package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func TestRunPublishesStructureCertificationSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	args := []string{
		"--holdout-authoring", "authoring.json", "--holdout-receipt", "receipt.json",
		"--public", "manifest.json", "--authority", "authority.json",
		"--assessment", "first.json", "--assessment", "second.json",
		"--compared-at", "2026-09-03T12:00:00Z", "--certified-at", "2026-09-03T13:00:00Z",
		"--out", "certification.json",
	}
	code := run(args, &stdout, &stderr, capabilities{publish: func(config fillerreview.TemporalStructureCertificationConfig) (fillerreview.TemporalStructureCertificationReport, string, error) {
		called = true
		if config.HoldoutAuthoringPath != "authoring.json" || config.HoldoutReceiptPath != "receipt.json" || len(config.AssessmentPaths) != 2 || config.AssessmentPaths[1] != "second.json" || config.OutputPath != "certification.json" {
			t.Fatalf("config = %+v", config)
		}
		return fillerreview.TemporalStructureCertificationReport{
			CertificationStatus: fillerreview.TemporalStructureCertificationPassed, Cases: 60,
			AssessorIDs: []string{"first", "second"}, CertifiedSlices: []string{"one", "two"},
			Slices: []fillerreview.TemporalStructureSliceCertification{{Slice: "one"}, {Slice: "two"}},
		}, strings.Repeat("a", 64), nil
	}})
	if code != 0 || !called || stderr.Len() != 0 || !strings.Contains(stdout.String(), "60 cases across 2 assessors") || !strings.Contains(stdout.String(), "2/2 difficult slices certified") || !strings.Contains(stdout.String(), "productionAdmissionAllowed=false") {
		t.Fatalf("code=%d called=%v stdout=%q stderr=%q", code, called, stdout.String(), stderr.String())
	}
}

func TestRunRequiresCompleteAuthorityAndFixedTimes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr, capabilities{}); code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "at least two --assessment") {
		t.Fatalf("missing code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	args := []string{
		"--holdout-authoring", "authoring.json", "--holdout-receipt", "receipt.json",
		"--public", "manifest.json", "--authority", "authority.json",
		"--assessment", "first.json", "--assessment", "second.json",
		"--compared-at", "now", "--certified-at", "2026-09-03T13:00:00Z", "--out", "certification.json",
	}
	if code := run(args, &stdout, &stderr, capabilities{}); code != 2 || !strings.Contains(stderr.String(), "--compared-at must be RFC3339") {
		t.Fatalf("time code=%d stderr=%q", code, stderr.String())
	}
}

func TestAssessmentPathsRejectsEmptyAndPreservesOrder(t *testing.T) {
	var paths assessmentPaths
	if err := paths.Set(""); err == nil {
		t.Fatal("empty assessment path was accepted")
	}
	if err := paths.Set("second.json"); err != nil {
		t.Fatal(err)
	}
	if err := paths.Set("first.json"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "second.json" || paths[1] != "first.json" {
		t.Fatalf("paths = %v", paths)
	}
}
