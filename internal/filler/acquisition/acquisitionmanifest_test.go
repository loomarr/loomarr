package acquisition_test

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

func TestAcquisitionArtifactValidate_BindsExactWatchRelativeBytes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	valid := acquisition.Artifact{
		ID: "artifact-1", AcquisitionID: "acq-1", SourceID: "archive:classic",
		Provider: "archive", SourceURL: "https://archive.org/details/one",
		StagingPath: ".loomarr-acquisitions/acq-1/one.mp4", MediaPath: "one.mp4",
		SidecarPath: "one.info.json", MediaSHA256: strings.Repeat("a", 64), MediaBytes: 42,
		State: acquisition.ArtifactStaged, CompletedAt: now, UpdatedAt: now,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid manifest: %v", err)
	}

	for name, mutate := range map[string]func(*acquisition.Artifact){
		"absolute media":   func(a *acquisition.Artifact) { a.MediaPath = "/tmp/one.mp4" },
		"escaping staging": func(a *acquisition.Artifact) { a.StagingPath = "../one.mp4" },
		"missing digest":   func(a *acquisition.Artifact) { a.MediaSHA256 = "" },
		"zero bytes":       func(a *acquisition.Artifact) { a.MediaBytes = 0 },
		"archive newline": func(a *acquisition.Artifact) {
			a.ProviderArchiveEntry = "youtube one\nyoutube two"
		},
		"archive additional identity": func(a *acquisition.Artifact) {
			a.ProviderArchiveEntry = "youtube one two"
		},
		"committed empty archive": func(a *acquisition.Artifact) {
			a.ProviderArchiveCommitted = true
		},
		"repair without reason": func(a *acquisition.Artifact) {
			a.State, a.RepairReason = acquisition.ArtifactRepair, ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid acquisition artifact passed validation")
			}
		})
	}
}

func TestAcquisitionArtifactOutcomeFrom_ExposesBoundedRepairReason(t *testing.T) {
	artifacts := []acquisition.Artifact{
		{State: acquisition.ArtifactRepair, RepairReason: "newest actionable reason"},
		{State: acquisition.ArtifactConsumed},
		{State: acquisition.ArtifactRepair, RepairReason: "older reason"},
	}
	got := acquisition.ArtifactOutcomeFrom(artifacts)
	if got.Repair != 2 || got.Consumed != 1 || got.RepairReason != "newest actionable reason" {
		t.Fatalf("outcome = %+v", got)
	}
}
