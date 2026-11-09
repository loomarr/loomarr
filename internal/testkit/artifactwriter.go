package testkit

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// ArtifactWriter is an in-memory fetch.ArtifactWriter: it keeps the latest manifest it was given
// for each artifact, so a test that does not care about acquisition records can still hand the
// Ingestor the writer production requires, and one that does can read them back.
type ArtifactWriter struct {
	mu        sync.Mutex
	artifacts []acquisition.Artifact
	// Err, when set, is returned by every write.
	Err error
}

// UpsertAcquisitionArtifacts records the manifests, replacing an earlier one with the same ID.
func (w *ArtifactWriter) UpsertAcquisitionArtifacts(_ context.Context, artifacts []acquisition.Artifact) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Err != nil {
		return w.Err
	}
	for _, artifact := range artifacts {
		replaced := false
		for i := range w.artifacts {
			if w.artifacts[i].ID == artifact.ID {
				w.artifacts[i], replaced = artifact, true
				break
			}
		}
		if !replaced {
			w.artifacts = append(w.artifacts, artifact)
		}
	}
	return nil
}

// Artifacts returns the manifests recorded so far.
func (w *ArtifactWriter) Artifacts() []acquisition.Artifact {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]acquisition.Artifact(nil), w.artifacts...)
}
