package app

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type recordingTranscriptObserver struct{ clip filler.StoreClip }

func (s *recordingTranscriptObserver) Observe(_ context.Context, clip filler.StoreClip) (filler.StageResult, string, error) {
	s.clip = clip
	return filler.StageResult{Clip: clip, Verdict: filler.VerdictContinue}, "spoken words", nil
}

type recordingVisionObserver struct {
	clip        filler.StoreClip
	observation filler.VisionObservation
}

func (s *recordingVisionObserver) Observe(_ context.Context, clip filler.StoreClip) (filler.StageResult, filler.VisionObservation, error) {
	s.clip = clip
	return filler.StageResult{Clip: clip, Verdict: filler.VerdictContinue}, s.observation, nil
}

func TestFillerMediaCapabilitySelectionsTrackLiveProviderIdentity(t *testing.T) {
	off := visionSet(t, map[string]string{})
	if activeFillerTranscriptCapability(off).Available || activeFillerVisionCapability(off).Available {
		t.Fatal("optional media capability became available without household opt-in")
	}

	local := visionSet(t, map[string]string{
		"filler.transcribe.enabled": "true",
		"asr.provider":              "whisper",
		"ingest.whisper_model":      "/models/ggml-small.en.bin",
	})
	transcript := activeFillerTranscriptCapability(local)
	if !transcript.Available || transcript.Producer != "transcript:whisper" || transcript.ProducerVersion == "" {
		t.Fatalf("local transcript capability = %+v", transcript)
	}

	hostedA := visionSet(t, map[string]string{
		"filler.transcribe.enabled": "true",
		"asr.provider":              "hosted",
		"asr.model":                 "openai/whisper-large-v3",
		"llm.provider":              "openai",
		"llm.hosted_provider":       "openrouter",
		"llm.url":                   "https://openrouter.ai/api/v1",
		"llm.model":                 "openai/gpt-4o-mini",
	})
	hostedB := visionSet(t, map[string]string{
		"filler.transcribe.enabled": "true",
		"asr.provider":              "hosted",
		"asr.model":                 "google/gemini-2.5-flash",
		"llm.provider":              "openai",
		"llm.hosted_provider":       "openrouter",
		"llm.url":                   "https://openrouter.ai/api/v1",
		"llm.model":                 "openai/gpt-4o-mini",
	})
	a, b := activeFillerTranscriptCapability(hostedA), activeFillerTranscriptCapability(hostedB)
	if !a.Available || a.Producer != "transcript:openrouter" || a.ProducerVersion == b.ProducerVersion {
		t.Fatalf("hosted transcript identities = %+v / %+v", a, b)
	}
	dedicated := activeFillerTranscriptCapability(visionSet(t, map[string]string{
		"filler.transcribe.enabled": "true",
		"asr.provider":              "hosted",
		"asr.url":                   "http://fictional-ai-server:8083/v1",
		"asr.model":                 "whisper-large-v3-turbo-q5_0",
	}))
	if !dedicated.Available || dedicated.Producer != "transcript:speech-service" {
		t.Fatalf("dedicated speech capability = %+v", dedicated)
	}

	visionA := activeFillerVisionCapability(visionSet(t, map[string]string{
		"filler.vision.enabled": "true", "llm.provider": "openai",
		"llm.hosted_provider": "openrouter", "llm.url": "https://openrouter.ai/api/v1",
		"llm.model": "openai/gpt-4o-mini", "filler.vision.model": "google/gemini-2.5-flash",
	}))
	visionB := activeFillerVisionCapability(visionSet(t, map[string]string{
		"filler.vision.enabled": "true", "llm.provider": "openai",
		"llm.hosted_provider": "openrouter", "llm.url": "https://openrouter.ai/api/v1",
		"llm.model": "openai/gpt-4o-mini", "filler.vision.model": "qwen/qwen2.5-vl-72b",
	}))
	if !visionA.Available || visionA.Producer != "vision:openrouter" || visionA.ProducerVersion == visionB.ProducerVersion {
		t.Fatalf("vision identities = %+v / %+v", visionA, visionB)
	}
}

func TestFillerMediaExecutorLoadsAReadyClipWithoutOwningReadiness(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_100, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{
		Hash: "ready", Path: "ready.mp4", Name: "Ready advert", Kind: filler.Commercial,
		Placement: filler.PlacementBreakBody, Held: false, DurationMs: 30_000,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	observer := &recordingTranscriptObserver{}
	executor := fillerMediaExecutor{store: st, transcript: observer, kind: fillerenrichment.CapabilityTranscript}
	result, err := executor.Run(t.Context(), fillerenrichment.Candidate{ClipHash: clip.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if observer.clip.Hash != clip.Hash || observer.clip.Held || observer.clip.Placement != filler.PlacementBreakBody {
		t.Fatalf("stage clip = %+v", observer.clip)
	}
	if result.Observation == nil || result.Observation.Transcript == nil ||
		*result.Observation.Transcript != "spoken words" {
		t.Fatalf("capability result = %+v", result)
	}
	stored, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil || stored.Held || stored.Placement != filler.PlacementBreakBody {
		t.Fatalf("stored clip = %+v, err %v", stored, err)
	}
}

func TestFillerEnrichmentVisionPassCommitsObservationWithoutReplacingOperatorAnswers(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_200, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{
		Hash: "operator", Path: "operator.mp4", Name: "Operator advert", Kind: filler.Commercial,
		Brand: "Operator Brand", Era: 1987, Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	for _, state := range []fillerenrichment.State{
		{ClipHash: clip.Hash, Axis: fillerenrichment.AxisBrand, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Text: "Operator Brand"}},
		{ClipHash: clip.Hash, Axis: fillerenrichment.AxisEra, Status: fillerenrichment.StatusComplete,
			Value: fillerenrichment.Value{Year: 1987}},
		{ClipHash: clip.Hash, Axis: fillerenrichment.AxisProduct, Status: fillerenrichment.StatusComplete},
	} {
		state.Evidence = fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceOperator,
			Reference: "operator", Confidence: 100, Producer: "operator", ProducerVersion: "1", ObservedAt: at}
		if _, changed, err := st.ApplyFillerEnrichment(t.Context(), state, at); err != nil || !changed {
			t.Fatalf("apply operator state = changed %v, err %v", changed, err)
		}
	}
	observer := &recordingVisionObserver{observation: filler.VisionObservation{
		Brand: "Model Brand", VisibleText: "MODEL BRAND 1999 CANDY", Era: 1999, Tags: []string{"candy"},
	}}
	runner := fillerenrichment.NewCapabilityRunner(
		fillerenrichment.CapabilityVision,
		fillerEnrichmentRepository{st: st},
		func() fillerenrichment.CapabilitySelection {
			return fillerenrichment.CapabilitySelection{
				Available: true, Producer: "vision:test", ProducerVersion: "vision-v1:model",
			}
		},
		fillerMediaExecutor{store: st, vision: observer, kind: fillerenrichment.CapabilityVision}.Run,
		func() int { return 1 }, func() time.Time { return at.Add(time.Minute) },
	)
	result, err := runner.Run(t.Context())
	if err != nil || result.Considered != 1 || result.Failed != 0 {
		t.Fatalf("vision catch-up = %+v, err %v", result, err)
	}
	if observer.clip.Hash != clip.Hash {
		t.Fatalf("observed clip = %+v", observer.clip)
	}
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Brand != "Operator Brand" || got.Era != 1987 || len(got.AssertedTags) != 0 ||
		!got.VisionTagged || got.VisibleText != "MODEL BRAND 1999 CANDY" {
		t.Fatalf("vision catch-up projection = %+v", got)
	}
	second, err := runner.Run(t.Context())
	if err != nil || second.Considered != 0 {
		t.Fatalf("completed frame observation woke its own pass: %+v, err %v", second, err)
	}
}

func TestDeterministicFillerEnrichmentProjectsPinnedExamplesWithoutAProvider(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	for _, clip := range []store.Clip{
		{Clip: filler.Clip{Hash: "hp", Path: "hp.mp4", Name: "HP Sauce Advert", Kind: filler.Commercial}, UpdatedAt: at, CreatedAt: at},
		{Clip: filler.Clip{Hash: "tootsie", Path: "tootsie.mp4", Name: "Tootsie Pop Classic Commercial", Kind: filler.Commercial}, UpdatedAt: at, CreatedAt: at.Add(time.Second)},
	} {
		if err := st.UpsertClip(t.Context(), clip); err != nil {
			t.Fatal(err)
		}
	}
	files := fstest.MapFS{
		"hp.info.json":      {Data: []byte(`{"title":"HP Sauce Advert","description":"An advert for HP Sauce broadcast 1999 on Five.","upload_date":"20250107","loomarr":{"originalName":"hp-sauce-advert.mp4","sourceId":"classic"}}`)},
		"tootsie.info.json": {Data: []byte(`{"title":"Tootsie Pop Classic Commercial","description":"The classic Tootsie Pop commercial. There are 3 versions.","upload_date":"20240215","loomarr":{"originalName":"tootsie-pop.mp4","sourceId":"classic"}}`)},
	}
	runner := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st}, fillerEnrichmentSignals{store: st, files: files}.Load,
		func() int { return 10 }, func() time.Time { return at.Add(time.Minute) },
	)
	result, err := runner.Run(t.Context())
	if err != nil || result.Considered != 2 || result.Updated != 2*len(fillerenrichment.DeterministicAxes)-1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	hp, err := st.GetClip(t.Context(), "hp")
	if err != nil {
		t.Fatal(err)
	}
	if hp.Era != 1999 || hp.Brand != "HP Sauce" || hp.Category != "condiments" || hp.Country != "GB" || hp.Network != "Five" {
		t.Fatalf("HP projection = %+v", hp)
	}
	tootsie, err := st.GetClip(t.Context(), "tootsie")
	if err != nil {
		t.Fatal(err)
	}
	if tootsie.Era != 0 || tootsie.Brand != "Tootsie Pop" || tootsie.Category != "candy" {
		t.Fatalf("Tootsie projection = %+v", tootsie)
	}
	candidates, err := st.ListFillerEnrichmentCandidates(t.Context(), fillerenrichment.DeterministicProducer,
		fillerenrichment.DeterministicProducerVersion, fillerenrichment.ControlledTaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("completed clips remained candidates: %+v, err %v", candidates, err)
	}
}

func TestDeterministicFillerEnrichmentDoesNotInventClipGeographyFromInstallationLocation(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_300, 0).UTC()
	source := store.NewFillerSource("youtube:inherited", "youtube", "https://youtube.example/channel",
		"Inherited source", at)
	if err := st.UpsertFillerSource(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	clip := store.Clip{Clip: filler.Clip{
		Hash: "inherited-geography", Path: "inherited-geography.mp4", Name: "Local advert",
		Kind: filler.Commercial, Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"inherited-geography.info.json": {Data: []byte(`{"title":"Local advert","loomarr":{"sourceId":"youtube:inherited"}}`)},
	}
	runner := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st},
		fillerEnrichmentSignals{store: st, files: files}.Load,
		func() int { return 1 }, func() time.Time { return at.Add(time.Minute) },
	)
	result, err := runner.Run(t.Context())
	if err != nil || result.Considered != 1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.GeographicScope != filler.GeographicUnknown || got.Country != "" || got.Market != "" {
		t.Fatalf("inherited source invented clip geography = %+v", got.Clip)
	}
}

func TestDeterministicFillerEnrichmentKeepsAnExplicitSourceGeography(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_400, 0).UTC()
	source := store.NewFillerSource("youtube:regional", "youtube", "https://youtube.example/channel",
		"Regional source", at)
	source.Geography = filler.Geography{Country: "GB", Market: "London"}
	if err := st.UpsertFillerSource(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	clip := store.Clip{Clip: filler.Clip{
		Hash: "explicit-geography", Path: "explicit-geography.mp4", Name: "Regional advert",
		Kind: filler.Commercial, Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"explicit-geography.info.json": {Data: []byte(`{"title":"Regional advert","loomarr":{"sourceId":"youtube:regional"}}`)},
	}
	runner := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st},
		fillerEnrichmentSignals{store: st, files: files}.Load,
		func() int { return 1 }, func() time.Time { return at.Add(time.Minute) },
	)
	result, err := runner.Run(t.Context())
	if err != nil || result.Considered != 1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.GeographicScope != filler.GeographicLocal || got.Country != "GB" || got.Market != "London" {
		t.Fatalf("explicit source geography = %+v, want local GB / London", got.Clip)
	}
}

// #1412 end to end through the scheduler driver: production has no text model configured and a
// preparation backlog that does not drain. The deterministic pass must still be recorded for an
// existing clip that has never had one.
func TestFillerPipelineDriverRecordsPassForExistingClipUnderProductionShape(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{Hash: "existing", Path: "existing.mp4", Name: "HP Sauce Advert",
		Kind: filler.Commercial}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	signals := fillerEnrichmentSignals{store: st, files: fstest.MapFS{}}
	runner := fillerenrichment.NewRunner(fillerEnrichmentRepository{st: st}, signals.Load,
		func() int { return 25 }, func() time.Time { return at.Add(time.Minute) })
	coordinator := fillerenrichment.NewCoordinator(runner, fillerEnrichmentRepository{st: st},
		func() fillerenrichment.TextSelection { return fillerenrichment.TextSelection{} },
		signals.Load, func() int { return 25 }, func() time.Time { return at.Add(time.Minute) })
	driver := newFillerPipelineDriver(nil, coordinator, nil)
	driver.prepare = func(context.Context) (filler.PipelineResult, error) {
		return filler.PipelineResult{Overview: filler.PipelineOverview{Runnable: 168, InProgress: 2}}, nil
	}
	if err := driver.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	candidates, err := st.ListFillerEnrichmentCandidates(t.Context(), fillerenrichment.DeterministicProducer,
		fillerenrichment.DeterministicProducerVersion, fillerenrichment.ControlledTaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("clip still lacks a deterministic pass: %+v, err %v", candidates, err)
	}
}
