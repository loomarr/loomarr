package filler

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/split"
)

// Fixtures for the root tests that build a split.Assessment by hand. They mirror the ones in
// filler/split's own tests, which are not importable from here.

func structureSource(durationMs int64) split.SourceAsset {
	return split.SourceAsset{
		Role: split.SourceLegacyPlayback, SHA256: strings.Repeat("a", 64), Bytes: 1024,
		ClipHash: strings.Repeat("b", 64), Path: "aa/bb/source.mp4", DurationMs: durationMs,
	}
}

func structureObservation(id string, kind split.ObservationKind, effect split.ObservationEffect, startMs, endMs int64) split.Observation {
	return split.Observation{
		ID: id, Kind: kind, Effect: effect, StartMs: startMs, EndMs: endMs,
		Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("c", 64),
	}
}

func structureClaim(startMs, endMs int64, role split.Role, evidenceID string) split.RoleClaim {
	return split.RoleClaim{
		StartMs: startMs, EndMs: endMs, Role: role, EvidenceIDs: []string{evidenceID},
		Reason: "fixture evidence establishes the interval role",
	}
}

func structureRoleObservation(t *testing.T, durationMs, startMs, endMs int64, role split.Role, id string) split.Observation {
	t.Helper()
	evidence, err := split.NewRoleEvidence(split.RoleEvidenceInput{
		Source: structureSource(durationMs), StartMs: startMs, EndMs: endMs, Role: role,
		Reason: "fixture evidence establishes the interval role", Frames: [][]byte{[]byte(id)},
		PromptVersion: "fixture-role-v1", Prompt: "classify this exact segment", Response: `{"role":"fixture"}`,
		RequestedProvider: "fixture", ResolvedProvider: "fixture", RequestedModel: "fixture", ResolvedModel: "fixture",
		Modalities: []string{"image", "text"}, Attempts: 1,
		AssessedAt: time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := split.NewRoleObservation(id, evidence)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func assessStructure(t *testing.T, durationMs int64, observations []split.Observation, claims []split.RoleClaim) split.Assessment {
	t.Helper()
	assessment, err := split.Assess(split.Input{
		Source: structureSource(durationMs), Observations: observations, RoleClaims: claims,
		AssessedAt: time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return assessment
}
