package filler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

type mediaDerivativeRequest struct {
	ClipDir string
	Source  asset.Identity
	Input   tools.Probed
	Recipe  tools.DerivativeRecipe
	// Host is the hardware profile a hardware recipe opens (its VAAPI render node).
	Host        encode.HostProfile
	Tool        tools.MediaToolIdentity
	FFmpegPath  string
	Probe       tools.Prober
	Diagnostics *diagnostics.ProcessManager
	Transcode   func(context.Context, tools.TranscodeRequest, func(int)) (tools.MediaQuality, error)
	Verify      func(context.Context, string, string, int64, int, bool, float64) (tools.DerivativeQC, error)
	OnProgress  func(int)
	Storage     *storagegovernor.WriteTracker
}

func (s *TranscodeStage) prepareEvidenceDerivative(ctx context.Context, source asset.Identity, input tools.Probed, ffmpeg string, enc encodeTarget, storage *storagegovernor.WriteTracker) (*asset.DerivativeLineage, tools.MediaToolIdentity, error) {
	if s.evidenceTranscode == nil {
		return nil, tools.MediaToolIdentity{}, nil
	}
	if s.identifyFFmpeg == nil {
		return nil, tools.MediaToolIdentity{}, fmt.Errorf("media tool identity is unavailable")
	}
	tool, err := s.identifyFFmpeg(ctx, ffmpeg)
	if err != nil {
		return nil, tools.MediaToolIdentity{}, err
	}
	built, err := buildMediaDerivative(ctx, mediaDerivativeRequest{
		ClipDir: s.clipDir, Source: source, Input: input,
		Recipe: tools.EvidenceDerivativeRecipe(enc.profile), Host: enc.host, Tool: tool,
		FFmpegPath: ffmpeg, Probe: s.probe, Diagnostics: s.diagnostics,
		Transcode: s.evidenceTranscode, Verify: s.verifyDerivative,
		OnProgress: func(percent int) { reportProgress(ctx, pipelinestate.StageTranscode, percent*40/100) },
		Storage:    storage,
	})
	if err != nil {
		return nil, tools.MediaToolIdentity{}, err
	}
	return &built, tool, nil
}

func buildMediaDerivative(ctx context.Context, request mediaDerivativeRequest) (asset.DerivativeLineage, error) {
	if err := asset.ValidateIdentity(request.Source, asset.RoleSourceMaster, asset.MasterDirName); err != nil {
		return asset.DerivativeLineage{}, err
	}
	if err := request.Recipe.Validate(); err != nil {
		return asset.DerivativeLineage{}, err
	}
	if request.Recipe.Role != tools.DerivativeEvidence {
		return asset.DerivativeLineage{}, fmt.Errorf("build media derivative: role %q is not evidence", request.Recipe.Role)
	}
	if err := request.Tool.Validate(); err != nil {
		return asset.DerivativeLineage{}, err
	}
	if request.Transcode == nil || request.Verify == nil || request.Probe == nil || request.Input.DurationMs <= 0 || request.Input.Height <= 0 {
		return asset.DerivativeLineage{}, fmt.Errorf("build media derivative: transcode, verification, probe, and measured input are required")
	}
	recipeDigest, err := request.Recipe.Digest()
	if err != nil {
		return asset.DerivativeLineage{}, err
	}
	sourcePath := filepath.Join(request.ClipDir, filepath.FromSlash(request.Source.Path))
	stageDir := filepath.Join(request.ClipDir, split.MediaAssetRootName, ".staging")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return asset.DerivativeLineage{}, fmt.Errorf("build media derivative: create staging directory: %w", err)
	}
	stage, err := os.CreateTemp(stageDir, "evidence-*.mp4")
	if err != nil {
		return asset.DerivativeLineage{}, fmt.Errorf("build media derivative: create staging file: %w", err)
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return asset.DerivativeLineage{}, err
	}
	_ = os.Remove(stagePath)
	defer func() {
		_ = os.Remove(stagePath)
		_ = os.Remove(tools.SidecarPathFor(stagePath))
	}()
	writeCtx, finishStorage := request.Storage.Monitor(ctx, stagePath)
	quality, err := request.Transcode(writeCtx, tools.TranscodeRequest{
		In: sourcePath, Out: stagePath, DurationMs: request.Input.DurationMs, HadAudio: !request.Input.Silent,
		InputProbe: &request.Input,
		TargetLUFS: request.Recipe.TargetLUFS, Profile: request.Recipe.Profile(), Host: request.Host, FFmpegPath: request.FFmpegPath,
		Probe: request.Probe, Diagnostics: request.Diagnostics,
	}, request.OnProgress)
	if err != nil {
		if storageErr := finishStorage(); storageErr != nil {
			return asset.DerivativeLineage{}, storageErr
		}
		return asset.DerivativeLineage{}, fmt.Errorf("build evidence derivative: %w", err)
	}
	if err := finishStorage(); err != nil {
		return asset.DerivativeLineage{}, err
	}
	output, err := request.Probe(ctx, stagePath)
	if err != nil || output.DurationMs <= 0 || output.Height <= 0 || (!request.Input.Silent && output.Silent) {
		return asset.DerivativeLineage{}, fmt.Errorf("build evidence derivative: output streams are incomplete")
	}
	qc, err := request.Verify(ctx, request.FFmpegPath, stagePath, output.DurationMs,
		request.Recipe.KeyframeSeconds, !output.Silent, request.Recipe.TargetLUFS)
	if err != nil {
		return asset.DerivativeLineage{}, fmt.Errorf("build evidence derivative: %w", err)
	}
	digest, size, err := asset.FileSHA256(stagePath)
	if err != nil {
		return asset.DerivativeLineage{}, err
	}
	if err := request.Storage.Record(ctx, size); err != nil {
		return asset.DerivativeLineage{}, err
	}
	clipHash, err := asset.ClipID(stagePath)
	if err != nil {
		return asset.DerivativeLineage{}, err
	}
	rel := filepath.ToSlash(filepath.Join(split.MediaAssetRootName, split.MediaEvidenceDirName,
		request.Source.SHA256[:2], request.Source.SHA256[2:4], recipeDigest[:16], digest+".mp4"))
	target := filepath.Join(request.ClipDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return asset.DerivativeLineage{}, err
	}
	if err := asset.PublishRetainedMaster(ctx, stagePath, target, digest, size); err != nil {
		return asset.DerivativeLineage{}, fmt.Errorf("publish evidence derivative: %w", err)
	}
	lineage := asset.DerivativeLineage{
		Asset:       asset.Identity{Role: asset.RoleEvidence, SHA256: digest, Bytes: size, ClipHash: clipHash, Path: rel},
		InputSHA256: request.Source.SHA256, Recipe: request.Recipe, RecipeSHA256: recipeDigest,
		Tool: request.Tool, DurationMs: output.DurationMs, Quality: quality, QC: qc,
		InputProbe: request.Input, OutputProbe: output,
	}
	if err := asset.ValidateDerivative(lineage, asset.RoleEvidence, split.MediaEvidenceDirName, request.Source.SHA256); err != nil {
		return asset.DerivativeLineage{}, err
	}
	return lineage, nil
}
