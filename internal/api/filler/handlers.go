// Package filler serves the filler routes under /v1/filler: the clip catalog and its edits, the
// Incoming queue, readiness and pool health, the pull approval queue, the header's watch status,
// clip-detail research status and the defect floor report.
package filler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// ErrIngestUnavailable reports that this install cannot run the ingest tooling. It is NOT
// a configuration error — no setting can assert that a binary executes — which is why the
// API renders it as a distinct problem type rather than feature_not_configured.
//
// ⚠ This used to mean "you are on loomarr:latest, run loomarr:filler instead". The single
// image always ships the tooling (§16), so it now means a DEGRADED install: a custom build
// without the vendored binaries, or a path that is missing or not executable.
var ErrIngestUnavailable = errors.New("ingest tooling not present in this image")

// Store is the slice of the filler store these routes read and write: the clip catalog, the
// channel list a catalog change reconciles, and the filler persistence roles.
type Store interface {
	httpcore.UserLister
	store.ChannelStore
	store.ClipStore
	records.FillerPullStore
	records.FillerAcquisitionStore
	records.FillerSplitProposalStore
	records.FillerPipelineStore
	records.FillerSourceStore
	records.FillerEnrichmentStore
	records.FillerResearchStore
}

// ResearchService owns the operator-facing status and pre-save provider check for optional
// clip-detail research. Search results remain internal pipeline evidence; this seam intentionally
// exposes no query or per-clip command.
type ResearchService interface {
	Status(context.Context) (research.WebStatus, error)
	Test(context.Context, research.WebConfig) (research.WebStatus, bool, string, error)
}

// ChannelReconciler converges a channel after the filler catalog changed under it. When the
// implementation also offers ReconcileFillerChange, only the affected channels are rebuilt.
type ChannelReconciler interface {
	Reconcile(ctx context.Context, channelID string) error
}

// PodService answers the pool and per-clip channel-fit questions the Filler page asks of the pod
// assembler (§12).
type PodService interface {
	// ClipFit reports how ONE clip relates to EVERY channel's selection (§10 V35 item 1.7).
	ClipFit(ctx context.Context, clipPath string) (map[string]pod.Fit, error)
	// Pool is catalog-wide filler health, the same per-channel numbers the channel route reports.
	Pool(ctx context.Context) (pod.PoolReport, error)
}

// Deps is what the filler routes need from the rest of the server. Every field is optional: a
// nil service leaves its routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// LiveSettings reads the home location the pull gate and the watch verdict judge sources by.
	LiveSettings settings.Reader
	// Settings gates the clip routes on the filler feature; nil reads as enabled.
	Settings httpcore.FeatureGate
	// Filler is the acquisition and readiness service. Its optional Rewinder and fetch-status
	// capabilities are found by type assertion.
	Filler Service
	// FillerLayout is the clip folder topology this server generation runs on.
	FillerLayout cliplibrary.Layout
	// The Filler prefix matches the api.Options fields these are copied from.
	FillerResearch ResearchService
	FillerDefects  *defectfloor.DefectReports
	// Channels reconciles the channels a catalog change touches.
	Channels ChannelReconciler
	// Pods answers pool health and per-clip channel fit.
	Pods PodService
	// ImageRecords resolves image hashes to their records in one deduplicated pass; nil leaves
	// every artwork URL as it is.
	ImageRecords func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	// IntakeLanguages reads the language the intake pipeline works in; nil reports none.
	IntakeLanguages func(context.Context) (string, error)
	// SourceProviders names every provider a filler source can use.
	SourceProviders *source.Registry
	// Taxonomy previews and applies clip taxonomy edits and reconciles the channels they touch;
	// nil keeps store-only wiring.
	Taxonomy TaxonomyEditor
	// Origin says whether the request address may be read from proxy headers (the location
	// suggestion works from the caller's address).
	Origin httpcore.Origin
	// PatchSettings saves settings through the audited Settings path and returns each edit's
	// status ("saved", "pinned", ...); nil answers 501 where a route needs it.
	PatchSettings func(ctx context.Context, edits map[string]string, updatedBy string) []string
	// PlaysCounted is whether this install can observe a clip airing (internal playout).
	PlaysCounted bool
	// Now is the request clock; nil is time.Now.
	Now func() time.Time
}

// Handlers serves the filler routes.
type Handlers struct {
	log             *slog.Logger
	store           Store
	liveSettings    settings.Reader
	settings        httpcore.FeatureGate
	filler          Service
	fillerLayout    cliplibrary.Layout
	fillerResearch  ResearchService
	fillerDefects   *defectfloor.DefectReports
	channels        ChannelReconciler
	pods            PodService
	imageRecords    func(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO
	intakeLanguages func(context.Context) (string, error)
	sourceProviders *source.Registry
	taxonomy        TaxonomyEditor
	origin          httpcore.Origin
	patchSettings   func(ctx context.Context, edits map[string]string, updatedBy string) []string
	playsCountedOn  bool
	now             func() time.Time

	// stations is the broadcast station directory, read once from the store (it changes only
	// with a seed revision at boot).
	stations stationCache
}

// New returns the filler handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings, settings: d.Settings, filler: d.Filler,
		fillerLayout: d.FillerLayout, fillerResearch: d.FillerResearch, fillerDefects: d.FillerDefects,
		channels: d.Channels, pods: d.Pods, imageRecords: d.ImageRecords, intakeLanguages: d.IntakeLanguages,
		sourceProviders: d.SourceProviders, taxonomy: d.Taxonomy, origin: d.Origin, patchSettings: d.PatchSettings, playsCountedOn: d.PlaysCounted, now: d.Now,
	}
}

// Register mounts every route this package serves.
func (s *Handlers) Register(api huma.API) {
	s.registerFiller(api)
	s.registerFillerSources(api)
	s.registerFillerIncoming(api)
	s.registerFillerBulk(api)
	s.registerFillerPulls(api)
	s.registerFillerWatch(api)
	s.registerFillerResearch(api)
	s.registerFillerDefects(api)
	s.registerTaxonomy(api)
	s.registerLocations(api)
}

// featureOff reports whether a named feature (config-design §7) is configured-off right now. Safe
// when settings is unset (unit tests): reports false so a wired dependency still serves.
func (s *Handlers) featureOff(ctx context.Context, feature string) bool {
	return s.settings != nil && !s.settings.Features(ctx)[feature]
}

// imageDTOsByHash resolves image hashes in one pass, or nothing without an image service.
func (s *Handlers) imageDTOsByHash(ctx context.Context, hashes []string) map[string]*httpcore.ImageDTO {
	if s.imageRecords == nil || len(hashes) == 0 {
		return nil
	}
	return s.imageRecords(ctx, hashes)
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

// SourceDisabledError is the shared answer for "you switched this off".
//
// 409 rather than 502 or 501: the request is well-formed and the feature is present — the
// install is simply in a state that refuses it, and the operator can change that state. The
// detail names the exact switch, because "disabled" without a location is a dead end.
func SourceDisabledError() huma.StatusError {
	return httpcore.Conflict("The drop-folder is switched off",
		"Loomarr isn't scanning the drop-folder because that source is turned off. Switch it back on under Filler → Sources, then sync again.")
}

// OrPlaceholder is v, or placeholder when v is empty.
func OrPlaceholder(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}
