package filler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/mediatools"
)

type mediaDerivativeRequest struct {
	ClipDir     string
	Source      MediaAssetIdentity
	Input       Probed
	Recipe      mediatools.DerivativeRecipe
	Tool        mediatools.MediaToolIdentity
	FFmpegPath  string
	Probe       Prober
	Diagnostics *diagnostics.ProcessManager
	Transcode   func(context.Context, mediatools.TranscodeRequest, func(int)) (MediaQuality, error)
	OnProgress  func(int)
}

func buildMediaDerivative(ctx context.Context, request mediaDerivativeRequest) (MediaDerivativeLineage, error) {
	if err := validateMediaAssetIdentity(request.Source, MediaAssetSourceMaster, mediaMasterDirName); err != nil {
		return MediaDerivativeLineage{}, err
	}
	if err := request.Recipe.Validate(); err != nil {
		return MediaDerivativeLineage{}, err
	}
	if request.Recipe.Role != mediatools.DerivativeEvidence {
		return MediaDerivativeLineage{}, fmt.Errorf("build media derivative: role %q is not evidence", request.Recipe.Role)
	}
	if err := request.Tool.Validate(); err != nil {
		return MediaDerivativeLineage{}, err
	}
	if request.Transcode == nil || request.Probe == nil || request.Input.DurationMs <= 0 || request.Input.Height <= 0 {
		return MediaDerivativeLineage{}, fmt.Errorf("build media derivative: transcode, probe, and measured input are required")
	}
	recipeDigest, err := request.Recipe.Digest()
	if err != nil {
		return MediaDerivativeLineage{}, err
	}
	sourcePath := filepath.Join(request.ClipDir, filepath.FromSlash(request.Source.Path))
	stageDir := filepath.Join(request.ClipDir, MediaAssetRootName, ".staging")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return MediaDerivativeLineage{}, fmt.Errorf("build media derivative: create staging directory: %w", err)
	}
	stage, err := os.CreateTemp(stageDir, "evidence-*.mp4")
	if err != nil {
		return MediaDerivativeLineage{}, fmt.Errorf("build media derivative: create staging file: %w", err)
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return MediaDerivativeLineage{}, err
	}
	_ = os.Remove(stagePath)
	defer func() {
		_ = os.Remove(stagePath)
		_ = os.Remove(sidecarPathFor(stagePath))
	}()
	quality, err := request.Transcode(ctx, mediatools.TranscodeRequest{
		In: sourcePath, Out: stagePath, DurationMs: request.Input.DurationMs, HadAudio: !request.Input.Silent,
		TargetLUFS: request.Recipe.TargetLUFS, Profile: request.Recipe.Profile(), FFmpegPath: request.FFmpegPath,
		Probe: request.Probe, Diagnostics: request.Diagnostics,
	}, request.OnProgress)
	if err != nil {
		return MediaDerivativeLineage{}, fmt.Errorf("build evidence derivative: %w", err)
	}
	output, err := request.Probe(ctx, stagePath)
	if err != nil || output.DurationMs <= 0 || output.Height <= 0 || (!request.Input.Silent && output.Silent) {
		return MediaDerivativeLineage{}, fmt.Errorf("build evidence derivative: output streams are incomplete")
	}
	digest, size, err := FileSHA256(stagePath)
	if err != nil {
		return MediaDerivativeLineage{}, err
	}
	clipHash, err := ClipID(stagePath)
	if err != nil {
		return MediaDerivativeLineage{}, err
	}
	rel := filepath.ToSlash(filepath.Join(MediaAssetRootName, mediaEvidenceDirName,
		request.Source.SHA256[:2], request.Source.SHA256[2:4], recipeDigest[:16], digest+".mp4"))
	target := filepath.Join(request.ClipDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return MediaDerivativeLineage{}, err
	}
	if err := publishRetainedMaster(ctx, stagePath, target, digest, size); err != nil {
		return MediaDerivativeLineage{}, fmt.Errorf("publish evidence derivative: %w", err)
	}
	lineage := MediaDerivativeLineage{
		Asset:       MediaAssetIdentity{Role: MediaAssetEvidence, SHA256: digest, Bytes: size, ClipHash: clipHash, Path: rel},
		InputSHA256: request.Source.SHA256, RecipeID: request.Recipe.ID, RecipeSHA256: recipeDigest,
		Tool: request.Tool, DurationMs: output.DurationMs, Quality: quality,
	}
	if err := validateMediaDerivative(lineage, MediaAssetEvidence, mediaEvidenceDirName, request.Source.SHA256); err != nil {
		return MediaDerivativeLineage{}, err
	}
	return lineage, nil
}
