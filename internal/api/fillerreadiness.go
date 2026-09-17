package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

type AcquisitionOutcomeDTO struct {
	Enrolled      int `json:"enrolled"`
	Preparing     int `json:"preparing"`
	NeedsDecision int `json:"needsDecision"`
	Ready         int `json:"ready"`
	Complete      int `json:"complete"`
	Rejected      int `json:"rejected"`
	Dismissed     int `json:"dismissed"`
}

type AcquisitionArtifactOutcomeDTO struct {
	Staged       int    `json:"staged"`
	Published    int    `json:"published"`
	Consumed     int    `json:"consumed"`
	Repair       int    `json:"repair"`
	RepairReason string `json:"repairReason,omitempty"`
}

type AcquisitionRepairSummaryDTO struct {
	Count        int    `json:"count"`
	LatestReason string `json:"latestReason,omitempty"`
}

type PipelineOverviewDTO struct {
	Runnable      int `json:"runnable"`
	InProgress    int `json:"inProgress"`
	Scheduled     int `json:"scheduled"`
	NeedsDecision int `json:"needsDecision"`
	Ready         int `json:"ready"`
	Complete      int `json:"complete"`
	Rejected      int `json:"rejected"`
	Dismissed     int `json:"dismissed"`
	Recoverable   int `json:"recoverable" doc:"Terminal failures with an explicit retry or restore action"`
}

type FillerAcquisitionRunDTO struct {
	ID       string `json:"id"`
	Trigger  string `json:"trigger" enum:"scheduled,source,pull,manual"`
	SourceID string `json:"sourceId,omitempty"`
	PullID   string `json:"pullId,omitempty"`
	Status   string `json:"status" enum:"queued,running,success,error"`

	Requested int    `json:"requested"`
	Fetched   int    `json:"fetched"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	Empty     int    `json:"empty"`
	Error     string `json:"error,omitempty"`

	StartedAt   string                        `json:"startedAt" format:"date-time"`
	CompletedAt string                        `json:"completedAt,omitempty" format:"date-time"`
	UpdatedAt   string                        `json:"updatedAt" format:"date-time"`
	Outcome     AcquisitionOutcomeDTO         `json:"outcome"`
	Artifacts   AcquisitionArtifactOutcomeDTO `json:"artifacts"`
}

type FillerReadinessDTO struct {
	Ready       bool   `json:"ready"`
	NextAction  string `json:"nextAction" enum:"none,enable_fetch,free_catalog_capacity,free_disposable_space,choose_another_folder,change_storage_limit,retry_acquisition,retry_failed_work,review_incoming,add_filler,improve_channel_coverage"`
	ChannelID   string `json:"channelId,omitempty"`
	ActionCount int    `json:"actionCount,omitempty"`

	Fetch        FillerFetchStatusDTO        `json:"fetch"`
	Storage      FillerStorageStatusDTO      `json:"storage"`
	Pipeline     PipelineOverviewDTO         `json:"pipeline"`
	Pool         PoolDTO                     `json:"pool"`
	Acquisitions []FillerAcquisitionRunDTO   `json:"acquisitions"`
	Repairs      AcquisitionRepairSummaryDTO `json:"repairs"`
}

type FillerStorageStatusDTO struct {
	TotalBytes              int64  `json:"totalBytes"`
	FreeBytes               int64  `json:"freeBytes"`
	ManagedBytes            int64  `json:"managedBytes"`
	ReservedBytes           int64  `json:"reservedBytes"`
	FilesystemReservedBytes int64  `json:"filesystemReservedBytes"`
	SoftBudgetBytes         int64  `json:"softBudgetBytes"`
	HardReserveBytes        int64  `json:"hardReserveBytes"`
	AvailableBytes          int64  `json:"availableBytes"`
	Automatic               bool   `json:"automatic"`
	State                   string `json:"state" enum:"healthy,approaching,paused,unknown"`
	PausedBy                string `json:"pausedBy,omitempty" enum:"library_limit,host_reserve,estimate_unknown,capacity_unavailable"`
}

type fillerReadinessOutput struct {
	Body FillerReadinessDTO
}

type FillerStorageCleanupPreviewDTO struct {
	Items int   `json:"items"`
	Bytes int64 `json:"bytes"`
}

type fillerStorageCleanupPreviewOutput struct {
	Body FillerStorageCleanupPreviewDTO
}

type FillerStorageCleanupResultDTO struct {
	RemovedItems int                            `json:"removedItems"`
	RemovedBytes int64                          `json:"removedBytes"`
	FailedItems  int                            `json:"failedItems"`
	Remaining    FillerStorageCleanupPreviewDTO `json:"remaining"`
}

type fillerStorageCleanupResultOutput struct {
	Body FillerStorageCleanupResultDTO
}

type getFillerAcquisitionInput struct {
	JobID string `path:"jobId" minLength:"1" maxLength:"256"`
}

type getFillerAcquisitionOutput struct {
	Body FillerAcquisitionRunDTO
}

func (s *Server) getFillerAcquisition(ctx context.Context, in *getFillerAcquisitionInput) (*getFillerAcquisitionOutput, error) {
	run, err := s.store.GetAcquisitionRun(ctx, in.JobID, time.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("Filler acquisition not found")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("read filler acquisition", err)
	}
	return &getFillerAcquisitionOutput{Body: acquisitionRunDTO(run)}, nil
}

func (s *Server) fillerReadiness(ctx context.Context, _ *struct{}) (*fillerReadinessOutput, error) {
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before checking readiness.")
	}
	readiness, err := s.filler.Readiness(ctx)
	if err != nil {
		return nil, err
	}
	return &fillerReadinessOutput{Body: fillerReadinessDTO(readiness)}, nil
}

func (s *Server) previewFillerStorageCleanup(ctx context.Context, _ *struct{}) (*fillerStorageCleanupPreviewOutput, error) {
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before checking storage.")
	}
	preview, err := s.filler.PreviewStorageCleanup(ctx)
	if err != nil {
		return nil, err
	}
	return &fillerStorageCleanupPreviewOutput{Body: fillerStorageCleanupPreviewDTO(preview)}, nil
}

func (s *Server) cleanupFillerStorage(ctx context.Context, _ *struct{}) (*fillerStorageCleanupResultOutput, error) {
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before cleaning storage.")
	}
	result, err := s.filler.CleanupStorage(ctx)
	if err != nil {
		return nil, err
	}
	return &fillerStorageCleanupResultOutput{Body: FillerStorageCleanupResultDTO{
		RemovedItems: result.RemovedItems,
		RemovedBytes: result.RemovedBytes,
		FailedItems:  result.FailedItems,
		Remaining:    fillerStorageCleanupPreviewDTO(result.Remaining),
	}}, nil
}

func fillerStorageCleanupPreviewDTO(preview filler.StorageCleanupPreview) FillerStorageCleanupPreviewDTO {
	return FillerStorageCleanupPreviewDTO{Items: preview.Items, Bytes: preview.Bytes}
}

func fillerReadinessDTO(readiness filler.Readiness) FillerReadinessDTO {
	runs := make([]FillerAcquisitionRunDTO, 0, len(readiness.Runs))
	for _, run := range readiness.Runs {
		runs = append(runs, acquisitionRunDTO(run))
	}
	return FillerReadinessDTO{
		Ready: readiness.Ready, NextAction: string(readiness.Next),
		ChannelID: readiness.ChannelID, ActionCount: readiness.Count,
		Fetch: FillerFetchStatusDTO{
			Enabled: readiness.Fetch.Enabled, StoppedBy: readiness.Fetch.StoppedBy,
			CatalogClips: readiness.Fetch.CatalogClips, MaxCatalog: readiness.Fetch.MaxCatalog,
		},
		Storage: FillerStorageStatusDTO{
			TotalBytes: readiness.Storage.TotalBytes, FreeBytes: readiness.Storage.FreeBytes,
			ManagedBytes: readiness.Storage.ManagedBytes, ReservedBytes: readiness.Storage.ReservedBytes,
			FilesystemReservedBytes: readiness.Storage.FilesystemReservedBytes,
			SoftBudgetBytes:         readiness.Storage.SoftBudgetBytes, HardReserveBytes: readiness.Storage.HardReserveBytes,
			AvailableBytes: readiness.Storage.AvailableBytes, Automatic: readiness.Storage.Automatic,
			State:    readiness.Storage.State,
			PausedBy: readiness.Storage.PausedBy,
		},
		Pipeline: pipelineOverviewDTO(readiness.Pipeline), Pool: poolDTO(readiness.Pool),
		Acquisitions: runs,
		Repairs:      AcquisitionRepairSummaryDTO{Count: readiness.Repairs.Count, LatestReason: readiness.Repairs.LatestReason},
	}
}

func pipelineOverviewDTO(overview filler.PipelineOverview) PipelineOverviewDTO {
	return PipelineOverviewDTO{
		Runnable: overview.Runnable, InProgress: overview.InProgress, Scheduled: overview.Scheduled,
		NeedsDecision: overview.NeedsDecision, Ready: overview.Ready, Complete: overview.Complete,
		Rejected: overview.Rejected, Dismissed: overview.Dismissed, Recoverable: overview.Recoverable,
	}
}

func acquisitionRunDTO(run filler.AcquisitionRun) FillerAcquisitionRunDTO {
	return FillerAcquisitionRunDTO{
		ID: run.ID, Trigger: string(run.Trigger), SourceID: run.SourceID, PullID: run.PullID,
		Status: string(run.Status), Requested: run.Requested, Fetched: run.Fetched,
		Skipped: run.Skipped, Failed: run.Failed, Empty: run.Empty, Error: run.Error,
		StartedAt: formatAcquisitionTime(run.StartedAt), CompletedAt: formatAcquisitionTime(run.CompletedAt),
		UpdatedAt: formatAcquisitionTime(run.UpdatedAt),
		Outcome: AcquisitionOutcomeDTO{
			Enrolled: run.Outcome.Enrolled, Preparing: run.Outcome.Preparing,
			NeedsDecision: run.Outcome.NeedsDecision, Ready: run.Outcome.Ready,
			Complete: run.Outcome.Complete,
			Rejected: run.Outcome.Rejected, Dismissed: run.Outcome.Dismissed,
		},
		Artifacts: AcquisitionArtifactOutcomeDTO{
			Staged: run.Artifacts.Staged, Published: run.Artifacts.Published,
			Consumed: run.Artifacts.Consumed, Repair: run.Artifacts.Repair,
			RepairReason: run.Artifacts.RepairReason,
		},
	}
}

func formatAcquisitionTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}
