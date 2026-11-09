package asset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

const (
	MasterDirName   = "masters"
	ManifestVersion = 1
)

// Role says which retained rendition an Identity describes.
type Role string

const (
	RoleSourceMaster Role = "source_master"
	RoleEvidence     Role = "evidence"
	RolePlayback     Role = "playback"
)

// Identity identifies bytes independently of their catalog path. SHA256 is always the
// complete-file digest; ClipHash retains the bounded sparse identity only to bind the intake row.
type Identity struct {
	Role     Role   `json:"role"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
	ClipHash string `json:"clipHash"`
	Path     string `json:"path"`
}

// DerivativePath is the catalog-relative path of a rendition, or "" when there is none.
func DerivativePath(d *DerivativeLineage) string {
	if d == nil {
		return ""
	}
	return d.Asset.Path
}

// Manifest is the closed portable relationship between one source and its renditions.
// Derivative execution details are added by the recipe layer; source retention can ship first
// without inventing an evidence/playback identity that does not exist yet.
type Manifest struct {
	Version      int                `json:"version"`
	SourceMaster Identity           `json:"sourceMaster"`
	Evidence     *DerivativeLineage `json:"evidence,omitempty"`
	Playback     *DerivativeLineage `json:"playback,omitempty"`
}

// DerivativeLineage proves which source, recipe and executable produced one rendition.
type DerivativeLineage struct {
	Asset        Identity                `json:"asset"`
	InputSHA256  string                  `json:"inputSha256"`
	Recipe       tools.DerivativeRecipe  `json:"recipe"`
	RecipeSHA256 string                  `json:"recipeSha256"`
	Tool         tools.MediaToolIdentity `json:"tool"`
	DurationMs   int64                   `json:"durationMs"`
	Quality      tools.MediaQuality      `json:"quality"`
	QC           tools.DerivativeQC      `json:"qc"`
	InputProbe   tools.Probed            `json:"inputProbe"`
	OutputProbe  tools.Probed            `json:"outputProbe"`
}

// Validate reports whether the manifest is internally consistent and uses the supported version.
func (m Manifest) Validate() error {
	if m.Version != ManifestVersion {
		return fmt.Errorf("media asset manifest version %d is unsupported", m.Version)
	}
	if err := ValidateIdentity(m.SourceMaster, RoleSourceMaster, MasterDirName); err != nil {
		return err
	}
	if m.Evidence != nil {
		if err := ValidateDerivative(*m.Evidence, RoleEvidence, split.MediaEvidenceDirName, m.SourceMaster.SHA256); err != nil {
			return err
		}
	}
	if m.Playback != nil {
		if err := ValidateDerivative(*m.Playback, RolePlayback, "", m.SourceMaster.SHA256); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDerivative checks one rendition's lineage against its role, hidden tree and the
// digest of the input it was built from.
func ValidateDerivative(derivative DerivativeLineage, role Role, tree, inputSHA256 string) error {
	if err := ValidateIdentity(derivative.Asset, role, tree); err != nil {
		return err
	}
	if derivative.InputSHA256 != inputSHA256 || !split.IsContentHash(derivative.RecipeSHA256) || derivative.DurationMs <= 0 {
		return fmt.Errorf("%s derivative lineage is incomplete", role)
	}
	wantRecipeRole := tools.DerivativeEvidence
	if role == RolePlayback {
		wantRecipeRole = tools.DerivativePlayback
	}
	computedRecipeDigest, err := derivative.Recipe.Digest()
	if err != nil || derivative.Recipe.Role != wantRecipeRole || computedRecipeDigest != derivative.RecipeSHA256 {
		return fmt.Errorf("%s derivative recipe identity is invalid", role)
	}
	if derivative.InputProbe.DurationMs <= 0 || derivative.OutputProbe.DurationMs != derivative.DurationMs ||
		derivative.InputProbe.Height <= 0 || derivative.OutputProbe.Height <= 0 {
		return fmt.Errorf("%s derivative probe evidence is incomplete", role)
	}
	if err := derivative.Tool.Validate(); err != nil {
		return fmt.Errorf("%s derivative tool identity: %w", role, err)
	}
	if err := tools.ValidateDerivativeQC(derivative.QC, derivative.DurationMs,
		derivative.Recipe.KeyframeSeconds, !derivative.OutputProbe.Silent, derivative.Recipe.TargetLUFS); err != nil {
		return fmt.Errorf("%s derivative QC: %w", role, err)
	}
	return nil
}

// ValidateReuseFiles reopens every recorded rendition before completed work may be reused.
// Manifest lineage and cached QC describe the bytes that passed verification; neither proves that
// the same regular file still occupies the recorded path.
func (m Manifest) ValidateReuseFiles(ctx context.Context, clipDir, playbackPath, clipHash string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := ValidateFile(ctx, clipDir, m.SourceMaster, RoleSourceMaster, MasterDirName); err != nil {
		return err
	}
	if m.Evidence != nil {
		if err := ValidateFile(ctx, clipDir, m.Evidence.Asset, RoleEvidence, split.MediaEvidenceDirName); err != nil {
			return err
		}
	}
	if m.Playback != nil {
		if filepath.Clean(filepath.FromSlash(m.Playback.Asset.Path)) != filepath.Clean(filepath.FromSlash(playbackPath)) ||
			m.Playback.Asset.ClipHash != clipHash {
			return errors.New("playback derivative identity does not match the catalog clip")
		}
		if err := ValidateFile(ctx, clipDir, m.Playback.Asset, RolePlayback, ""); err != nil {
			return err
		}
	} else {
		// A legacy mezzanine has no honestly reconstructable playback recipe/tool lineage. Its
		// retained master is nevertheless the exact byte authority for the still-visible file.
		legacyPlayback := m.SourceMaster
		legacyPlayback.Role = RolePlayback
		legacyPlayback.Path = playbackPath
		if legacyPlayback.ClipHash != clipHash {
			return errors.New("legacy playback identity does not match the catalog clip")
		}
		if err := ValidateFile(ctx, clipDir, legacyPlayback, RolePlayback, ""); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFile reopens the regular file identity names under clipDir and requires its
// size, digest and sparse identity to match.
func ValidateFile(ctx context.Context, clipDir string, identity Identity, role Role, tree string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(clipDir) {
		return fmt.Errorf("%s derivative root is not absolute", role)
	}
	if err := ValidateIdentity(identity, role, tree); err != nil {
		return err
	}
	full := filepath.Join(clipDir, filepath.FromSlash(identity.Path))
	info, err := os.Lstat(full)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != identity.Bytes {
		return fmt.Errorf("%s derivative bytes do not match the manifest", role)
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(clipDir)
	resolvedFull, fileErr := filepath.EvalSymlinks(full)
	resolvedInfo, statErr := os.Stat(resolvedFull)
	if rootErr != nil || fileErr != nil || statErr != nil || !os.SameFile(info, resolvedInfo) ||
		resolvedFull == resolvedRoot || !split.PathContains(resolvedRoot, resolvedFull) {
		return fmt.Errorf("%s derivative path escapes the filler root", role)
	}
	digest, size, err := FileSHA256(resolvedFull)
	if err != nil || digest != identity.SHA256 || size != identity.Bytes {
		return fmt.Errorf("%s derivative bytes do not match the manifest", role)
	}
	sparse, err := ClipID(resolvedFull)
	if err != nil || sparse != identity.ClipHash {
		return fmt.Errorf("%s derivative sparse identity does not match the manifest", role)
	}
	return ctx.Err()
}

// ValidateIdentity checks an identity's role, hashes, size and that its path lies in
// the hidden tree the role belongs to (tree is empty for playable files).
func ValidateIdentity(asset Identity, role Role, tree string) error {
	if asset.Role != role || !split.IsContentHash(asset.SHA256) || !split.IsContentHash(asset.ClipHash) || asset.Bytes <= 0 {
		return fmt.Errorf("%s media asset identity is invalid", role)
	}
	clean := filepath.Clean(filepath.FromSlash(asset.Path))
	if filepath.IsAbs(clean) || !split.IsRelativePath(clean) {
		return fmt.Errorf("%s media asset path is outside the filler root", role)
	}
	if tree == "" {
		if split.PathContains(split.MediaAssetRootName, clean) {
			return fmt.Errorf("%s media asset path is hidden rather than playable", role)
		}
		return nil
	}
	wantPrefix := filepath.Join(split.MediaAssetRootName, tree)
	if clean == wantPrefix || !split.PathContains(wantPrefix, clean) {
		return fmt.Errorf("%s media asset path is outside its hidden tree", role)
	}
	return nil
}

// PreserveSourceMasterWithStorage snapshots the exact transcode input before any derivative is
// built. The returned hidden pathname is the stable input for all later recipes, so a concurrent
// mutation of the visible name cannot mix bytes between evidence and playback.
func PreserveSourceMasterWithStorage(ctx context.Context, clipDir, sourcePath, clipHash string, tags SidecarTags, storage *storagegovernor.WriteTracker) (Identity, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	if clipDir == "" || !filepath.IsAbs(clipDir) || !filepath.IsAbs(sourcePath) || !split.IsContentHash(clipHash) {
		return Identity{}, errors.New("preserve source master: absolute roots and a content hash are required")
	}
	rel, err := filepath.Rel(clipDir, sourcePath)
	if err != nil || !split.IsRelativePath(rel) {
		return Identity{}, errors.New("preserve source master: source is outside the filler root")
	}
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return Identity{}, fmt.Errorf("preserve source master: inspect source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return Identity{}, errors.New("preserve source master: source is not a non-empty regular file")
	}

	stageDir := filepath.Join(clipDir, split.MediaAssetRootName, ".staging")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return Identity{}, fmt.Errorf("preserve source master: create staging directory: %w", err)
	}
	stage, err := os.CreateTemp(stageDir, "master-*")
	if err != nil {
		return Identity{}, fmt.Errorf("preserve source master: create staging file: %w", err)
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return Identity{}, fmt.Errorf("preserve source master: close staging file: %w", err)
	}
	_ = os.Remove(stagePath)
	defer func() { _ = os.Remove(stagePath) }()
	writeCtx, finishStorage := storage.Monitor(ctx, stagePath)
	if err := SnapshotOwnedFile(writeCtx, sourcePath, stagePath); err != nil {
		if storageErr := finishStorage(); storageErr != nil {
			return Identity{}, storageErr
		}
		return Identity{}, fmt.Errorf("preserve source master: snapshot source: %w", err)
	}
	if err := finishStorage(); err != nil {
		return Identity{}, err
	}
	digest, size, err := FileSHA256(stagePath)
	if err != nil {
		return Identity{}, fmt.Errorf("preserve source master: digest snapshot: %w", err)
	}
	if err := storage.Record(ctx, size); err != nil {
		return Identity{}, err
	}
	snapshotClipHash, err := ClipID(stagePath)
	if err != nil || snapshotClipHash != clipHash {
		return Identity{}, errors.New("preserve source master: snapshot does not match catalog identity")
	}
	ext := retainedMediaExtension(sourcePath)
	relMaster := filepath.ToSlash(filepath.Join(split.MediaAssetRootName, MasterDirName, digest[:2], digest[2:4], digest+ext))
	masterPath := filepath.Join(clipDir, filepath.FromSlash(relMaster))
	if err := os.MkdirAll(filepath.Dir(masterPath), 0o755); err != nil {
		return Identity{}, fmt.Errorf("preserve source master: create master directory: %w", err)
	}
	if err := PublishRetainedMaster(ctx, stagePath, masterPath, digest, size); err != nil {
		return Identity{}, err
	}
	asset := Identity{
		Role: RoleSourceMaster, SHA256: digest, Bytes: size,
		ClipHash: clipHash, Path: relMaster,
	}
	manifest := Manifest{Version: ManifestVersion, SourceMaster: asset}
	if err := manifest.Validate(); err != nil {
		return Identity{}, err
	}
	if sourceSidecar := tools.SidecarPathFor(sourcePath); sourceSidecar != tools.SidecarPathFor(masterPath) {
		if _, err := os.Stat(sourceSidecar); err == nil {
			if err := CopyFile(sourceSidecar, tools.SidecarPathFor(masterPath)); err != nil {
				return Identity{}, fmt.Errorf("preserve source master: copy portable provenance: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Identity{}, fmt.Errorf("preserve source master: inspect portable provenance: %w", err)
		}
	}
	tags.MediaAssets = &manifest
	if err := WriteSidecarTags(masterPath, tags, false); err != nil {
		return Identity{}, fmt.Errorf("preserve source master: write manifest: %w", err)
	}
	return asset, nil
}

// RetainedSourceMaster returns the source master tags already record, after proving its bytes
// still match, or preserves the source as a new master when none is recorded.
func RetainedSourceMaster(ctx context.Context, clipDir, sourcePath, clipHash string, tags SidecarTags, storage *storagegovernor.WriteTracker) (Identity, error) {
	if tags.MediaAssets == nil {
		return PreserveSourceMasterWithStorage(ctx, clipDir, sourcePath, clipHash, tags, storage)
	}
	if err := tags.MediaAssets.Validate(); err != nil {
		return Identity{}, fmt.Errorf("resolve source master: %w", err)
	}
	master := tags.MediaAssets.SourceMaster
	masterPath := filepath.Join(clipDir, filepath.FromSlash(master.Path))
	digest, size, err := FileSHA256(masterPath)
	if err != nil || digest != master.SHA256 || size != master.Bytes {
		return Identity{}, errors.New("resolve source master: retained bytes do not match the manifest")
	}
	masterClipHash, err := ClipID(masterPath)
	if err != nil || masterClipHash != master.ClipHash {
		return Identity{}, errors.New("resolve source master: retained sparse identity does not match the manifest")
	}
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	return master, nil
}

// PublishRetainedMaster links staged into the content-addressed master name, accepting an
// existing file there only when its digest, size and exact bytes match.
func PublishRetainedMaster(ctx context.Context, staged, target, digest string, size int64) error {
	if err := os.Link(staged, target); err == nil {
		_ = os.Remove(staged)
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("preserve source master: publish: %w", err)
	}
	existingDigest, existingSize, digestErr := FileSHA256(target)
	if digestErr != nil || existingDigest != digest || existingSize != size {
		return errors.New("preserve source master: existing content-addressed master does not match")
	}
	equal, err := ExactFileBytesEqual(ctx, staged, target, tools.ConditioningMaxSnapshotBytes)
	if err != nil || !equal {
		return errors.New("preserve source master: existing content-addressed master does not match exact bytes")
	}
	_ = os.Remove(staged)
	return nil
}

func retainedMediaExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) < 2 || len(ext) > 10 {
		return ".bin"
	}
	for _, char := range ext[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return ".bin"
		}
	}
	return ext
}
