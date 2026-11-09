package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

type fillerEnrichmentRepository struct{ st records.Store }

type fillerResearchRepository struct {
	st records.FillerResearchStore
}

func activeFillerTextSelection(set resolved, recorder *metrics.Recorder) enrichment.TextSelection {
	selection := resolveSelection(set)
	if selection.URL == "" || selection.Model == "" {
		return enrichment.TextSelection{}
	}
	return enrichment.TextSelection{Provider: buildProviderFor(selection, recorder),
		ProviderName: selection.Provider, Model: selection.Model}
}

func capabilityVersion(version string, identity ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return version + ":" + hex.EncodeToString(sum[:8])
}

func activeFillerTranscriptCapability(set resolved) enrichment.CapabilitySelection {
	if !settings.FillerTranscribeEnabled.Get(set) {
		return enrichment.CapabilitySelection{}
	}
	if settings.ASRProvider.Get(set) == "hosted" {
		model := strings.TrimSpace(settings.ASRModel.Get(set))
		url := strings.TrimSpace(settings.ASRURL.Get(set))
		identity := "speech-service"
		if url == "" {
			selection := resolveSelection(set)
			url, identity = selection.URL, selection.Provider
		}
		if url == "" || model == "" || identity == "" {
			return enrichment.CapabilitySelection{}
		}
		return enrichment.CapabilitySelection{Available: true,
			Producer: "transcript:" + identity,
			ProducerVersion: capabilityVersion(filler.TranscriptionPassVersion,
				identity, url, model)}
	}
	model := strings.TrimSpace(settings.IngestWhisperModel.Get(set))
	if model == "" {
		return enrichment.CapabilitySelection{}
	}
	return enrichment.CapabilitySelection{Available: true, Producer: "transcript:whisper",
		ProducerVersion: capabilityVersion(filler.TranscriptionPassVersion,
			settings.IngestWhisperPath.Get(set), model)}
}

func activeFillerVisionCapability(set resolved) enrichment.CapabilitySelection {
	if !settings.FillerVisionEnabled.Get(set) {
		return enrichment.CapabilitySelection{}
	}
	wiring := visionEndpoint(set)
	if wiring.url == "" || strings.TrimSpace(wiring.model) == "" ||
		(wiring.provider != "openai" && wiring.provider != "ollama") {
		return enrichment.CapabilitySelection{}
	}
	identity := wiring.identity
	if identity == "" {
		identity = wiring.provider
	}
	return enrichment.CapabilitySelection{Available: true, Producer: "vision:" + identity,
		ProducerVersion: capabilityVersion(filler.VisionPromptVersion,
			identity, wiring.url, wiring.model)}
}

type fillerTranscriptObserver interface {
	Observe(context.Context, filler.StoreClip) (filler.StageResult, string, error)
}

type fillerVisionObserver interface {
	Observe(context.Context, filler.StoreClip) (filler.StageResult, filler.VisionObservation, error)
}

// fillerMediaExecutor is intentionally narrower than filler.Stage: the enrichment coordinator has
// already decided that useful work remains, and this adapter can load media and produce observations
// but has no readiness, persistence, or conveyor mutation in its interface.
type fillerMediaExecutor struct {
	store      records.Store
	transcript fillerTranscriptObserver
	vision     fillerVisionObserver
	kind       enrichment.Capability
}

func (e fillerMediaExecutor) Run(ctx context.Context, candidate enrichment.Candidate) (enrichment.CapabilityResult, error) {
	if e.store == nil {
		return enrichment.CapabilityResult{}, nil
	}
	clip, err := e.store.GetClip(ctx, candidate.ClipHash)
	if err != nil {
		return enrichment.CapabilityResult{}, fmt.Errorf("load filler clip: %w", err)
	}
	storeClip := fillerClip(clip)
	if e.kind == enrichment.CapabilityTranscript {
		if e.transcript == nil {
			return enrichment.CapabilityResult{}, nil
		}
		_, transcript, err := e.transcript.Observe(ctx, storeClip)
		if err != nil {
			return enrichment.CapabilityResult{}, err
		}
		return enrichment.CapabilityResult{Observation: &enrichment.MediaObservation{
			Transcript: &transcript,
		}}, nil
	}
	if e.kind != enrichment.CapabilityVision || e.vision == nil {
		return enrichment.CapabilityResult{}, nil
	}
	currentStates, err := e.store.ListFillerEnrichment(ctx, candidate.ClipHash)
	if err != nil {
		return enrichment.CapabilityResult{}, fmt.Errorf("load filler enrichment before vision: %w", err)
	}
	current := make(map[enrichment.Axis]enrichment.State, len(currentStates))
	for _, state := range currentStates {
		current[state.Axis] = state
	}
	_, observation, err := e.vision.Observe(ctx, storeClip)
	if err != nil {
		return enrichment.CapabilityResult{}, err
	}
	var states []enrichment.State
	evidence := func(reference string) enrichment.Evidence {
		return enrichment.Evidence{Kind: enrichment.EvidenceContent,
			Reference: reference, Confidence: 100}
	}
	if observation.Brand != "" {
		states = append(states, enrichment.State{Axis: enrichment.AxisBrand,
			Status: enrichment.StatusComplete, Value: enrichment.Value{Text: observation.Brand},
			Evidence: evidence("frame.visible_text")})
	}
	if observation.Era > 0 {
		states = append(states, enrichment.State{Axis: enrichment.AxisEra,
			Status: enrichment.StatusComplete, Value: enrichment.Value{Year: observation.Era},
			Evidence: evidence("frame.visible_text")})
	}
	taxa, err := e.store.ListTaxa(ctx)
	if err != nil {
		return enrichment.CapabilityResult{}, fmt.Errorf("load filler taxonomy after vision: %w", err)
	}
	tagAxes := make(map[string]enrichment.Axis, len(taxa))
	for _, taxon := range taxa {
		tagAxes[taxon.Slug] = enrichment.Axis(taxon.Axis)
	}
	byAxis := map[enrichment.Axis][]string{}
	for _, tag := range observation.Tags {
		axis := tagAxes[tag]
		if axis.Valid() {
			byAxis[axis] = append(byAxis[axis], tag)
		}
	}
	for axis, tags := range byAxis {
		if accepted, ok := current[axis]; ok {
			tags = append(append([]string(nil), accepted.Value.Tags...), tags...)
		}
		states = append(states, enrichment.State{Axis: axis, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Tags: tags}, Evidence: evidence("frame.taxonomy_grounded")})
	}
	return enrichment.CapabilityResult{
		States: states,
		Observation: &enrichment.MediaObservation{Vision: &enrichment.VisionObservation{
			VisibleText: observation.VisibleText, SuggestedEra: observation.SuggestedEra,
		}},
	}, nil
}

func (r fillerEnrichmentRepository) ListCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]enrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return fillerEnrichmentCandidates(clips, err)
}

func (r fillerEnrichmentRepository) ListCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]enrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCapabilityCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return fillerEnrichmentCandidates(clips, err)
}

func fillerEnrichmentCandidates(clips []clipstore.Clip, err error) ([]enrichment.Candidate, error) {
	if err != nil {
		return nil, err
	}
	out := make([]enrichment.Candidate, len(clips))
	for i, clip := range clips {
		out[i] = enrichment.Candidate{ClipHash: clip.Hash, Path: clip.Path, Name: clip.Name,
			Kind: string(clip.Role), Transcript: clip.Transcript, VisibleText: clip.VisibleText}
	}
	return out, nil
}

func (r fillerEnrichmentRepository) ListStates(ctx context.Context, clipHash string) ([]enrichment.State, error) {
	return r.st.ListFillerEnrichment(ctx, clipHash)
}

func (r fillerEnrichmentRepository) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return r.st.ListTaxa(ctx)
}

func (r fillerEnrichmentRepository) ApplyPass(ctx context.Context, pass enrichment.Pass) (int, error) {
	return r.st.ApplyFillerEnrichmentPass(ctx, pass)
}

func (r fillerResearchRepository) ListCandidates(ctx context.Context, producer, producerVersion,
	adapter, adapterVersion string, limit int) ([]research.Candidate, error) {
	return r.st.ListFillerResearchCandidates(ctx, producer, producerVersion, adapter, adapterVersion, limit)
}

func (r fillerResearchRepository) SaveReport(ctx context.Context, report research.Report) error {
	return r.st.SaveFillerResearchReport(ctx, report)
}

func (r fillerResearchRepository) PromoteStoredCountries(ctx context.Context, limit int) (int, error) {
	return r.st.PromoteStoredFillerResearchCountries(ctx, limit)
}

type fillerResearchSignals struct{ files fs.FS }

func (l fillerResearchSignals) Load(_ context.Context, candidate research.Candidate) (research.Input, error) {
	input := research.Input{Title: candidate.Name}
	if metadata, ok := asset.ReadSourceMetadataFS(l.files, candidate.Path); ok {
		input.Title = metadata.Title
		input.Description = metadata.Description
		input.SourceURL = metadata.WebpageURL
	}
	input.SourceKind, _, _ = strings.Cut(candidate.SourceID, ":")
	return input, nil
}

// fillerDetailRunner keeps one scheduler row while the modules retain different authority:
// enrichment writes verified axes; research writes only cited suggestions.
type fillerDetailRunner struct {
	enrichment fillerEnrichmentRunner
	research   *research.Runner
	retire     *fillerGeographyRetirement // runs last, once geography has had every pass
}

func (r fillerDetailRunner) Run(ctx context.Context) (enrichment.RunResult, error) {
	var out enrichment.RunResult
	var enrichmentErr, researchErr, retireErr error
	if r.enrichment != nil {
		out, enrichmentErr = r.enrichment.Run(ctx)
	}
	if r.research != nil {
		var researched research.RunResult
		researched, researchErr = r.research.Run(ctx)
		out.Considered += researched.Considered
		out.Updated += researched.Updated
		out.Failed += researched.Failed
	}
	if r.retire != nil {
		_, retireErr = r.retire.Run(ctx)
	}
	return out, errors.Join(enrichmentErr, researchErr, retireErr)
}

type fillerEnrichmentSignals struct {
	store    records.FillerSourceStore
	files    fs.FS
	stations *stationDirectory // nil: no station is ever named
}

// stationDirectory loads the broadcast station directory on first use. Boot seeds it and nothing
// rewrites it while the process runs, so one load serves every pass; a failed load is retried.
type stationDirectory struct {
	list func(context.Context) ([]station.Station, error)
	mu   sync.Mutex
	dir  *station.Directory
}

func (s *stationDirectory) get(ctx context.Context) (*station.Directory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		stations, err := s.list(ctx)
		if err != nil {
			return nil, fmt.Errorf("load broadcast stations: %w", err)
		}
		s.dir = station.NewDirectory(stations)
	}
	return s.dir, nil
}

// resolveDuring adapts the directory for research, which names a call sign and a span. An
// unreadable directory resolves nothing, so the named station answers no geography.
func (s *stationDirectory) resolveDuring(log *slog.Logger) research.StationResolver {
	return func(callSign, from, to string) (string, string, bool) {
		dir, err := s.get(context.Background())
		if err != nil {
			log.Warn("filler research: broadcast stations unavailable; the named station is unresolved", "err", err)
			return "", "", false
		}
		station, ok := dir.ResolveDuring(callSign, from, to)
		return station.ID, station.Country, ok
	}
}

func (l fillerEnrichmentSignals) Load(ctx context.Context, candidate enrichment.Candidate, observedAt time.Time) (enrichment.Signals, error) {
	signals := enrichment.Signals{ClipHash: candidate.ClipHash, Kind: candidate.Kind,
		Title: candidate.Name, OriginalName: candidate.Name, Transcript: candidate.Transcript,
		VisibleText: candidate.VisibleText, ObservedAt: observedAt}
	metadata, ok := asset.ReadSourceMetadataFS(l.files, candidate.Path)
	if ok {
		signals.Title = metadata.Title
		signals.Description = metadata.Description
		signals.OriginalName = metadata.OriginalName
		signals.UploadDate = metadata.UploadDate
		signals.SourceID = metadata.SourceID
	}
	if l.stations != nil {
		directory, err := l.stations.get(ctx)
		if err != nil {
			return enrichment.Signals{}, err
		}
		// The transcript is speech, where a spoken "WJW" is rarer than a misheard word; the item and
		// on-screen text are where archives and stations write call signs.
		if match, found := directory.Identify(signals.Title, signals.OriginalName, signals.Description, signals.VisibleText); found {
			signals.Station = &match
		}
	}
	if signals.SourceID == "" || l.store == nil {
		return signals, nil
	}
	sources, err := l.store.ListFillerSources(ctx)
	if err != nil {
		return enrichment.Signals{}, err
	}
	for _, source := range sources {
		if source.ID != signals.SourceID {
			continue
		}
		// ⚠ Only what the source itself DECLARES is evidence about where its clips aired. Fetch
		// eligibility inherits the household's home country (EffectiveGeography), but copying
		// that inheritance into a clip labelled British archive ads "US" and let them past the
		// US-only filter (#1670). An undeclared source contributes nothing; the clip stays
		// unknown until item, content, or cited evidence says otherwise.
		// A source declares only a country (#2062), so its default is national; only a broadcast
		// station makes a clip local.
		geography := source.Geography.Normalize()
		if geography.Country != "" {
			signals.Source = enrichment.Geography{Scope: string(pod.GeographicNational), Country: geography.Country}
		}
		break
	}
	return signals, nil
}
