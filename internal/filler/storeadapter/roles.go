package storeadapter

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The store roles below are what the adapters read and write. Each names only the methods its
// adapter calls or promotes: an adapter embeds the roles a seam takes exactly as the store has
// them, and declares only the methods that differ.

type clipLister interface {
	ListClips(ctx context.Context, filter clipstore.ClipFilter) ([]clipstore.Clip, error)
}

type syncRows interface {
	UpsertClip(ctx context.Context, c clipstore.Clip) error
	LiveClipPaths(ctx context.Context) (map[string]string, error)
	DeleteClipsNotIn(ctx context.Context, keep []string) (int, error)
}

type pipelineClipRows interface {
	UpsertClip(ctx context.Context, c clipstore.Clip) error
	ReplaceClipIdentity(ctx context.Context, oldHash string, c clipstore.Clip) error
	ReplaceAiringClip(ctx context.Context, oldHash string, c clipstore.Clip, rec defects.Record) error
	CommitConditioningPublication(ctx context.Context, publication asset.ConditioningPublication, c clipstore.Clip) error
	SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
	HoldClips(ctx context.Context, paths []string, at time.Time) (int, error)
	SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error
	ClipsWithoutAirWindow(ctx context.Context) ([]clipstore.Clip, error)
	SetClipAirWindow(ctx context.Context, hash string, startMs, endMs int64, at time.Time) error
}

type languageRows interface {
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
}

type languageStore interface {
	clipLister
	languageRows
}

type transcribeRows interface {
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
}

type transcribeStore interface {
	clipLister
	transcribeRows
}

type visionRows interface {
	ApplyClipVision(ctx context.Context, hash, path, brand, visibleText string, era int, leaves []string, at time.Time) error
	RecordFillerEraObservation(ctx context.Context, clipHash string, era int, source research.ObservationSource, at time.Time) error
	// ListTaxa is the vocabulary the vision tier grounds its category against (§10 V45a).
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

type fingerprintRows interface {
	ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error)
	UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error
}

type remoteStateReader interface {
	ListAcquisitionRemoteStates(ctx context.Context) (map[string]acquisition.ExistingRemoteState, error)
}

type taxaLister interface {
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

type rewindRows interface {
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
	ClearClipVisibleText(ctx context.Context, path string, at time.Time) error
}

type sweepRows interface {
	MarkClipReaped(ctx context.Context, hash string, at time.Time) error
	MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error
}

type upkeepRows interface {
	ListClipPipelines(ctx context.Context, f pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error)
	GetClipPipeline(ctx context.Context, hash string) (pipelinestate.ClipPipeline, bool, error)
	RetireRejectedClip(ctx context.Context, hash string, rejectedBy, at time.Time) (bool, error)
	RetireReadyClip(ctx context.Context, hash string, at time.Time) (bool, error)
	ListSettledReels(ctx context.Context) ([]string, error)
	RetireSettledReel(ctx context.Context, hash string, at time.Time) (bool, error)
	AcquisitionArtifactForClip(ctx context.Context, mediaPath, clipHash string) (acquisition.Artifact, bool, error)
	AcquisitionHasArtifacts(ctx context.Context, acquisitionID string) (bool, error)
}

type catalogStore interface {
	clipLister
	ListClipVariants(ctx context.Context) (map[string][]string, error)
}

type clipPathReader interface {
	GetClipByPath(ctx context.Context, path string) (clipstore.Clip, error)
}

type imageRefDeleter interface {
	DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error
}
