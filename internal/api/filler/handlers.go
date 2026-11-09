// Package filler serves the filler routes under /v1/filler: the pull approval queue, the header's
// watch status, clip-detail research status and the defect floor report.
package filler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// ErrIngestUnavailable reports that this install cannot run the ingest tooling. It is NOT
// a configuration error — no setting can assert that a binary executes — which is why the
// API renders it as a distinct problem type rather than feature_not_configured.
//
// ⚠ This used to mean "you are on loomarr:latest, run loomarr:filler instead". The single
// image always ships the tooling (§16), so it now means a DEGRADED install: a custom build
// without the vendored binaries, or a path that is missing or not executable.
var ErrIngestUnavailable = errors.New("ingest tooling not present in this image")

// Store is the slice of the aggregate store these routes read and write.
type Store interface {
	httpcore.UserLister
	records.FillerPullStore
	records.FillerAcquisitionStore
	records.FillerSplitProposalStore
	records.FillerPipelineStore
	records.FillerSourceStore
	CountClips(ctx context.Context, filter clipstore.ClipFilter) (int, error)
}

// Ingester starts the download of an approved pull.
type Ingester interface {
	// IngestPull validates readiness, constructs the queued run, and calls commit exactly once
	// inside the same transaction that queues it; it returns the job id.
	IngestPull(ctx context.Context, pullID string, targets []acquisition.AcquisitionTarget, commit func(context.Context, acquisition.AcquisitionRun) error) (jobID string, err error)
}

// ResearchService owns the operator-facing status and pre-save provider check for optional
// clip-detail research. Search results remain internal pipeline evidence; this seam intentionally
// exposes no query or per-clip command.
type ResearchService interface {
	Status(context.Context) (research.WebStatus, error)
	Test(context.Context, research.WebConfig) (research.WebStatus, bool, string, error)
}

// Deps is what the filler routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// LiveSettings reads the home location the pull gate and the watch verdict judge sources by.
	LiveSettings settings.Reader
	// Filler starts approved pulls; its FetchStatus, when it has one, feeds the watch header.
	Filler Ingester
	// FillerLayout is the clip folder topology this server generation runs on.
	FillerLayout cliplibrary.Layout
	// The Filler prefix matches the api.Options fields these are copied from.
	FillerResearch ResearchService
	FillerDefects  *defectfloor.DefectReports
}

// Handlers serves the filler routes.
type Handlers struct {
	log            *slog.Logger
	store          Store
	liveSettings   settings.Reader
	filler         Ingester
	fillerLayout   cliplibrary.Layout
	fillerResearch ResearchService
	fillerDefects  *defectfloor.DefectReports
}

// New returns the filler handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings, filler: d.Filler,
		fillerLayout: d.FillerLayout, fillerResearch: d.FillerResearch, fillerDefects: d.FillerDefects,
	}
}

// Register mounts every route this package serves.
func (s *Handlers) Register(api huma.API) {
	s.registerFillerPulls(api)
	s.registerFillerWatch(api)
	s.registerFillerResearch(api)
	s.registerFillerDefects(api)
}

// HomeGeography is the installation's home location as the filler sources and pulls judge it, or
// the zero value without settings. A source serves home or it does not, so the sources routes ask
// the same question.
func HomeGeography(live settings.Reader) pod.Geography {
	if live == nil {
		return pod.Geography{}
	}
	return pod.Geography{
		Country:  settings.FillerHomeCountry.Get(live),
		Place:    settings.FillerHomePlace.Get(live),
		Fallback: settings.FillerRegionFallback.Joined(live),
	}.Normalize()
}

func (s *Handlers) fillerHomeGeography() pod.Geography { return HomeGeography(s.liveSettings) }

// IncomingPreparingFilter is the one predicate for "getting ready": Incoming's Preparing group and
// the watch header's preparing count both read it (#2733).
func IncomingPreparingFilter() pipelinestate.PipelineFilter {
	return pipelinestate.PipelineFilter{Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRunning}}
}

// OrPlaceholder is v, or placeholder when v is empty.
func OrPlaceholder(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}
