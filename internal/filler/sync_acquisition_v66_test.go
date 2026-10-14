package filler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type consumedAcquisitionAuthority struct {
	artifact AcquisitionArtifact
}

func (a *consumedAcquisitionAuthority) AcquisitionArtifactForClip(
	_ context.Context,
	_, clipHash string,
) (AcquisitionArtifact, bool, error) {
	return a.artifact, clipHash == a.artifact.ClipHash, nil
}

func (a *consumedAcquisitionAuthority) UpsertAcquisitionArtifacts(
	_ context.Context,
	artifacts []AcquisitionArtifact,
) error {
	if len(artifacts) == 1 {
		a.artifact = artifacts[0]
	}
	return nil
}

type consumedV66Fixture struct {
	syncer       *Syncer
	authority    *consumedAcquisitionAuthority
	clip         RawClip
	masterPath   string
	playbackPath string
}

func newConsumedV66Fixture(t *testing.T) consumedV66Fixture {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mp4")
	if err := os.WriteFile(sourcePath, []byte("exact acquired source bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceSHA, sourceBytes, err := FileSHA256(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sourceHash, err := ClipID(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	masterRel := filepath.ToSlash(filepath.Join(
		MediaAssetRootName, mediaMasterDirName, sourceSHA[:2], sourceSHA[2:4], sourceSHA+".mp4",
	))
	masterPath := filepath.Join(root, filepath.FromSlash(masterRel))
	if err := os.MkdirAll(filepath.Dir(masterPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sourcePath, masterPath); err != nil {
		t.Fatal(err)
	}

	playbackHash := writeContentAddressedClip(t, root, []byte("conditioned playback derivative"), ".mp4")
	playbackRel := filepath.ToSlash(ClipRelPath(playbackHash, ".mp4"))
	playbackPath := filepath.Join(root, filepath.FromSlash(playbackRel))
	playbackSHA, playbackBytes, err := FileSHA256(playbackPath)
	if err != nil {
		t.Fatal(err)
	}

	manifest := screeningSubjectManifest(t)
	manifest.SourceMaster = MediaAssetIdentity{
		Role: MediaAssetSourceMaster, SHA256: sourceSHA, Bytes: sourceBytes,
		ClipHash: sourceHash, Path: masterRel,
	}
	manifest.Evidence = nil
	manifest.Playback.Asset = MediaAssetIdentity{
		Role: MediaAssetPlayback, SHA256: playbackSHA, Bytes: playbackBytes,
		ClipHash: playbackHash, Path: playbackRel,
	}
	manifest.Playback.InputSHA256 = sourceSHA
	if err := manifest.validate(); err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarTags(playbackPath, SidecarTags{
		SourceID: "archive:classic", AcquisitionID: "acq-1", MediaAssets: &manifest,
	}, true); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_800_000_000, 0).UTC()
	authority := &consumedAcquisitionAuthority{artifact: AcquisitionArtifact{
		ID: "artifact-1", AcquisitionID: "acq-1", SourceID: "archive:classic",
		Provider: "archive", SourceURL: "https://archive.example/details/one",
		MediaPath: "retired-intake-name.mp4", MediaSHA256: sourceSHA, MediaBytes: sourceBytes,
		ClipHash: playbackHash, State: ArtifactConsumed, CompletedAt: now, UpdatedAt: now,
	}}
	syncer := &Syncer{dir: root, acquisitions: authority, now: func() time.Time { return now }}
	return consumedV66Fixture{
		syncer: syncer, authority: authority,
		clip:       RawClip{ID: playbackHash, Path: playbackRel},
		masterPath: masterPath, playbackPath: playbackPath,
	}
}

func TestAuthorizeAcquisitionAcceptsConsumedPlaybackBoundToExactSourceMaster(t *testing.T) {
	fixture := newConsumedV66Fixture(t)

	artifact, found, err := fixture.syncer.authorizeAcquisition(t.Context(), fixture.clip)
	if err != nil || !found || artifact.State != ArtifactConsumed {
		t.Fatalf("consumed V66 acquisition = %+v, found=%v, error=%v", artifact, found, err)
	}
	if fixture.authority.artifact.State != ArtifactConsumed || fixture.authority.artifact.RepairReason != "" {
		t.Fatalf("consumed V66 acquisition was moved to repair: %+v", fixture.authority.artifact)
	}
}

func TestAuthorizeAcquisitionDoesNotRecoverDriftedConsumedV66Assets(t *testing.T) {
	tests := []struct {
		name string
		path func(consumedV66Fixture) string
	}{
		{name: "source master", path: func(f consumedV66Fixture) string { return f.masterPath }},
		{name: "playback derivative", path: func(f consumedV66Fixture) string { return f.playbackPath }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newConsumedV66Fixture(t)
			path := test.path(fixture)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			body[0] ^= 0xff
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}

			_, found, err := fixture.syncer.authorizeAcquisition(t.Context(), fixture.clip)
			if err == nil || !found {
				t.Fatalf("drifted V66 acquisition found=%v, error=%v", found, err)
			}
			if fixture.authority.artifact.State != ArtifactRepair || fixture.authority.artifact.RepairReason == "" {
				t.Fatalf("drifted V66 acquisition escaped repair: %+v", fixture.authority.artifact)
			}
		})
	}
}
