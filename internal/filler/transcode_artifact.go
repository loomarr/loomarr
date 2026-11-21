package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/tools"
)

type conditioningSnapshots struct {
	Source string
	Parent string
}

// snapshotConditioningArtifacts binds the child and retained parent to private complete-byte
// snapshots before any measurement or rewrite. The caller sees one operation and cannot forget
// either identity or exact-byte check.
func snapshotConditioningArtifacts(ctx context.Context, stageDir, source, parent, sourceHash, parentHash string) (conditioningSnapshots, error) {
	out := conditioningSnapshots{
		Source: filepath.Join(stageDir, "source"+filepath.Ext(source)),
		Parent: filepath.Join(stageDir, "parent"+filepath.Ext(parent)),
	}
	cleanup := func() { _ = os.Remove(out.Source); _ = os.Remove(out.Parent) }
	if err := asset.SnapshotOwnedFile(ctx, source, out.Source); err != nil {
		cleanup()
		return conditioningSnapshots{}, errors.New("conditioning source could not be snapshotted")
	}
	if err := asset.SnapshotOwnedFile(ctx, parent, out.Parent); err != nil {
		cleanup()
		return conditioningSnapshots{}, errors.New("conditioning parent could not be snapshotted")
	}
	sourceID, sourceErr := asset.ClipID(out.Source)
	parentID, parentErr := asset.ClipID(out.Parent)
	if sourceErr != nil || parentErr != nil || sourceID != sourceHash || parentID != parentHash {
		cleanup()
		return conditioningSnapshots{}, errors.New("conditioning source or parent bytes do not match catalog identity")
	}
	sourceEqual, sourceErr := asset.ExactFileBytesEqual(ctx, source, out.Source, tools.ConditioningMaxSnapshotBytes)
	parentEqual, parentErr := asset.ExactFileBytesEqual(ctx, parent, out.Parent, tools.ConditioningMaxSnapshotBytes)
	if sourceErr != nil || parentErr != nil || !sourceEqual || !parentEqual {
		cleanup()
		return conditioningSnapshots{}, errors.New("conditioning source or parent snapshot is not exact")
	}
	return out, nil
}

// validateRecoveredTranscode proves an existing canonical artifact is the exact output and restart
// record from this attempt. Sparse catalog identity alone is never recovery authority.
func validateRecoveredTranscode(ctx context.Context, staged, published, expectedHash, profileID string, lineage *asset.ConditioningLineage, evidence *asset.ConditioningEvidence, quality tools.MediaQuality, assets *asset.Manifest) error {
	publishedID, err := asset.ClipID(published)
	if err != nil || publishedID != expectedHash {
		return errors.New("existing transformed artifact bytes do not match their identity")
	}
	equal, err := asset.ExactFileBytesEqual(ctx, staged, published, tools.ConditioningMaxSnapshotBytes)
	if err != nil || !equal {
		return errors.New("existing transformed artifact bytes do not exactly match the staged output")
	}
	tags, ok := asset.ReadSidecarTags(published)
	if !ok || tags.Mezzanine != profileID {
		return errors.New("existing transformed artifact has missing or corrupt evidence")
	}
	if assets != nil && !reflect.DeepEqual(tags.MediaAssets, assets) {
		return fmt.Errorf("existing transformed artifact is not bound to this media asset manifest (differs in %s)",
			manifestDifference(tags.MediaAssets, assets))
	}
	if evidence != nil && (!reflect.DeepEqual(tags.ConditioningLineage, lineage) ||
		tags.Conditioning == nil || !reflect.DeepEqual(*tags.Conditioning, *evidence) ||
		tags.MediaQuality == nil || !reflect.DeepEqual(*tags.MediaQuality, quality)) {
		return errors.New("existing transformed artifact is not bound to this child evidence")
	}
	return nil
}

// manifestDifference names the parts of two media asset manifests that differ, so a refused
// recovery says what changed between attempts. Field names only: no paths or hashes.
func manifestDifference(published, current *asset.Manifest) string {
	if published == nil || current == nil {
		return "presence"
	}
	var parts []string
	if published.Version != current.Version {
		parts = append(parts, "version")
	}
	if published.SourceMaster != current.SourceMaster {
		parts = append(parts, "sourceMaster")
	}
	for _, d := range []struct {
		name string
		a, b *asset.DerivativeLineage
	}{{"evidence", published.Evidence, current.Evidence}, {"playback", published.Playback, current.Playback}} {
		switch {
		case (d.a == nil) != (d.b == nil):
			parts = append(parts, d.name)
		case d.a != nil:
			parts = append(parts, lineageDifference(d.name, *d.a, *d.b)...)
		}
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ", ")
}

func lineageDifference(name string, a, b asset.DerivativeLineage) []string {
	var parts []string
	for _, f := range []struct {
		field string
		same  bool
	}{
		{"asset", a.Asset == b.Asset}, {"inputSha256", a.InputSHA256 == b.InputSHA256},
		{"recipe", a.Recipe == b.Recipe && a.RecipeSHA256 == b.RecipeSHA256}, {"tool", a.Tool == b.Tool},
		{"durationMs", a.DurationMs == b.DurationMs}, {"quality", reflect.DeepEqual(a.Quality, b.Quality)},
		{"qc", a.QC == b.QC}, {"inputProbe", a.InputProbe == b.InputProbe}, {"outputProbe", a.OutputProbe == b.OutputProbe},
	} {
		if !f.same {
			parts = append(parts, name+"."+f.field)
		}
	}
	return parts
}

// publishTranscodeReplacement owns the final sidecar/media visibility ordering for one transformed
// artifact. The staged pair remains private until this operation succeeds.
func publishTranscodeReplacement(staged, published, original string, tags asset.SidecarTags, alreadyPublished bool) error {
	if err := os.MkdirAll(filepath.Dir(published), 0o755); err != nil {
		return fmt.Errorf("create content shard: %w", err)
	}
	if published == original {
		return asset.WriteSidecarTags(original, tags, false)
	}
	if alreadyPublished {
		return nil
	}
	return publishHiddenMediaPair(staged, published)
}
