//go:build ffmpeg

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerquarantine"
)

func TestQuarantineInspectorWithRealMedia(t *testing.T) {
	fixture := newQuarantineInspectionFixture(t)
	report, err := fillerquarantine.Inspect(t.Context(), fixture.config(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Cases != 1 || report.Summary.PriorSources != 1 || report.Summary.EligibleForRightsReview != 1 || report.Summary.Held != 0 {
		t.Fatalf("summary=%+v", report.Summary)
	}
	if len(report.Cases) != 1 || report.Cases[0].Disposition != fillerquarantine.DispositionEligibleForRightsReview || len(report.Cases[0].HoldReasons) != 0 || len(report.Comparisons) != 1 || report.Comparisons[0].Related {
		t.Fatalf("case=%+v comparisons=%+v", report.Cases, report.Comparisons)
	}
	if err := fillerquarantine.Validate(report); err != nil {
		t.Fatalf("report did not revalidate: %v", err)
	}

	if err := os.Remove(filepath.Join(fixture.mediaRoot, fixture.priorSourceName)); err != nil {
		t.Fatal(err)
	}
	incomplete, err := fillerquarantine.Inspect(t.Context(), fixture.config(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if incomplete.Summary.Held != 1 || incomplete.Summary.UnavailablePriorSources != 1 || incomplete.Cases[0].Disposition != fillerquarantine.DispositionHold || !slices.Contains(incomplete.Cases[0].HoldReasons, "prior_perceptual_exposure_incomplete") {
		t.Fatalf("incomplete summary=%+v case=%+v", incomplete.Summary, incomplete.Cases[0])
	}

	timedOut, err := fillerquarantine.Inspect(t.Context(), fixture.config(time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
	if !reflect.DeepEqual(timedOut, fillerquarantine.Report{}) {
		t.Fatalf("timeout returned partial report: %+v", timedOut)
	}
}
