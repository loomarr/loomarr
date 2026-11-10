package app

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFetchStoreAdapter_ListAcquisitionRemoteStatesMapsArtifactLifecycle(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	testkit.SaveFillerSources(t, st, "archive:classic")
	run := acquisition.AcquisitionRun{ID: "identity-roundtrip", Trigger: acquisition.TriggerSource, SourceID: "archive:classic", Status: acquisition.RunRunning, Requested: 1, StartedAt: now, UpdatedAt: now}
	if err := st.UpsertAcquisitionRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	artifacts := []acquisition.Artifact{
		{ID: "staged-artifact", AcquisitionID: run.ID, SourceID: run.SourceID, Provider: "archive", RemoteID: "remote-Staged", SourceURL: "https://archive.org/details/remote-Staged", StagingPath: "stage/staged.mp4", MediaPath: "staged.mp4", MediaSHA256: strings.Repeat("a", 64), MediaBytes: 1, State: acquisition.ArtifactStaged, CompletedAt: now, UpdatedAt: now},
		{ID: "consumed-artifact", AcquisitionID: run.ID, SourceID: run.SourceID, Provider: "archive", RemoteID: "remote-Consumed", SourceURL: "https://archive.org/details/remote-Consumed", StagingPath: "stage/consumed.mp4", MediaPath: "consumed.mp4", MediaSHA256: strings.Repeat("b", 64), MediaBytes: 1, State: acquisition.ArtifactConsumed, CompletedAt: now, UpdatedAt: now},
	}
	if err := st.UpsertAcquisitionArtifacts(t.Context(), artifacts); err != nil {
		t.Fatal(err)
	}
	states, err := (storeadapter.NewFetch(st, nil, nil)).ListAcquisitionRemoteStates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		want acquisition.ExistingRemoteState
	}{{"remote-Staged", acquisition.RemoteQueued}, {"remote-Consumed", acquisition.RemoteCatalogued}} {
		key := (acquisition.RemoteIdentity{Provider: "archive", SourceID: run.SourceID, RemoteID: tc.id}).Key()
		if states[key] != tc.want {
			t.Fatalf("state[%q] = %q, want %q", key, states[key], tc.want)
		}
	}
}
