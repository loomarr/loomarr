package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// TextSelection is the text model the enrichment pass would ask now: the install's main selection,
// built into a provider by build. A selection without a URL or model is none.
func TextSelection(selection llm.Selection, build func(llm.Selection) llm.Provider) enrichment.TextSelection {
	if selection.URL == "" || selection.Model == "" {
		return enrichment.TextSelection{}
	}
	return enrichment.TextSelection{Provider: build(selection),
		ProviderName: selection.Provider, Model: selection.Model}
}

func capabilityVersion(version string, identity ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return version + ":" + hex.EncodeToString(sum[:8])
}

// TranscriptCapability is the speech-to-text capability the install offers now, with the version
// its choice stamps on a transcript. selection is the install's main language-model selection,
// which a hosted speech service with no URL of its own reuses.
func TranscriptCapability(set settings.Reader, selection llm.Selection) enrichment.CapabilitySelection {
	if !settings.FillerTranscribeEnabled.Get(set) {
		return enrichment.CapabilitySelection{}
	}
	if settings.ASRProvider.Get(set) == "hosted" {
		model := strings.TrimSpace(settings.ASRModel.Get(set))
		url := strings.TrimSpace(settings.ASRURL.Get(set))
		identity := "speech-service"
		if url == "" {
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

// VisionCapability is the frame-reading capability the install offers now, with the version its
// endpoint choice stamps on an observation.
func VisionCapability(set settings.Reader, selection llm.Selection) enrichment.CapabilitySelection {
	if !settings.FillerVisionEnabled.Get(set) {
		return enrichment.CapabilitySelection{}
	}
	wiring := visionEndpoint(set, selection)
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

// TranscriptObserver is the transcribe rung's read-only side.
type TranscriptObserver interface {
	Observe(context.Context, clipstore.Clip) (filler.StageResult, string, error)
}

// VisionObserver is the vision rung's read-only side.
type VisionObserver interface {
	Observe(context.Context, clipstore.Clip) (filler.StageResult, filler.VisionObservation, error)
}

// MediaStore is the slice of the filler store a media executor reads.
type MediaStore interface {
	GetClip(ctx context.Context, hash string) (clipstore.Clip, error)
	ListFillerEnrichment(ctx context.Context, clipHash string) ([]enrichment.State, error)
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

// MediaExecutor is intentionally narrower than filler.Stage: the enrichment coordinator has
// already decided that useful work remains, and this adapter can load media and produce observations
// but has no readiness, persistence, or conveyor mutation in its interface.
type MediaExecutor struct {
	store      MediaStore
	transcript TranscriptObserver
	vision     VisionObserver
	kind       enrichment.Capability
}

// NewTranscriptExecutor returns the executor for the transcript capability.
func NewTranscriptExecutor(store MediaStore, observer TranscriptObserver) MediaExecutor {
	return MediaExecutor{store: store, transcript: observer, kind: enrichment.CapabilityTranscript}
}

// NewVisionExecutor returns the executor for the vision capability.
func NewVisionExecutor(store MediaStore, observer VisionObserver) MediaExecutor {
	return MediaExecutor{store: store, vision: observer, kind: enrichment.CapabilityVision}
}

func (e MediaExecutor) Run(ctx context.Context, candidate enrichment.Candidate) (enrichment.CapabilityResult, error) {
	if e.store == nil {
		return enrichment.CapabilityResult{}, nil
	}
	clip, err := e.store.GetClip(ctx, candidate.ClipHash)
	if err != nil {
		return enrichment.CapabilityResult{}, fmt.Errorf("load filler clip: %w", err)
	}
	if e.kind == enrichment.CapabilityTranscript {
		if e.transcript == nil {
			return enrichment.CapabilityResult{}, nil
		}
		_, transcript, err := e.transcript.Observe(ctx, clip)
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
	_, observation, err := e.vision.Observe(ctx, clip)
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

// ResearchSignals loads the cited-research input for a clip from its source metadata file.
type ResearchSignals struct{ files fs.FS }

// NewResearchSignals returns the research input loader over the clip library's files.
func NewResearchSignals(files fs.FS) ResearchSignals { return ResearchSignals{files: files} }

func (l ResearchSignals) Load(_ context.Context, candidate research.Candidate) (research.Input, error) {
	input := research.Input{Title: candidate.Name}
	if metadata, ok := asset.ReadSourceMetadataFS(l.files, candidate.Path); ok {
		input.Title = metadata.Title
		input.Description = metadata.Description
		input.SourceURL = metadata.WebpageURL
	}
	input.SourceKind, _, _ = strings.Cut(candidate.SourceID, ":")
	return input, nil
}

// EnrichmentSignals loads the evidence the enrichment pass reads for a clip: its source metadata
// file, the station its text names, and what its source declares.
type EnrichmentSignals struct {
	store    records.FillerSourceStore
	files    fs.FS
	stations *station.Lazy // nil: no station is ever named
}

// NewEnrichmentSignals returns the enrichment signal loader. stations may be nil, in which case no
// station is ever named.
func NewEnrichmentSignals(store records.FillerSourceStore, files fs.FS, stations *station.Lazy) EnrichmentSignals {
	return EnrichmentSignals{store: store, files: files, stations: stations}
}

func (l EnrichmentSignals) Load(ctx context.Context, candidate enrichment.Candidate, observedAt time.Time) (enrichment.Signals, error) {
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
		directory, err := l.stations.Get(ctx)
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
