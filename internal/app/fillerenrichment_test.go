package app

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/fillerstore"
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
		"asr.url":                   "http://ai-server:8083/v1",
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

func TestDeterministicFillerEnrichmentDoesNotInventClipGeographyFromInstallationLocation(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_300, 0).UTC()
	source := fillerstore.NewFillerSource("youtube:inherited", "youtube", "https://youtube.example/channel",
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
	source := fillerstore.NewFillerSource("youtube:regional", "youtube", "https://youtube.example/channel",
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

// #2061, the reported case end to end: Cleveland station news that research had already called
// "national US" (so a US household aired it anywhere) becomes local to its station on the next
// deterministic pass, and the clip row, which airing reads, follows the axis.
func TestDeterministicFillerEnrichmentLocatesStationNewsOverAnEarlierNationalAnswer(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{
		Hash: "cleveland-news", Path: "cleveland-news.mp4", Name: "WEWS NewsChannel 5 News Update 03/10/1991",
		Kind: filler.Interstitial, Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	research := fillerenrichment.State{ClipHash: clip.Hash, Axis: fillerenrichment.AxisGeography, Status: fillerenrichment.StatusComplete,
		Value: fillerenrichment.Value{Geography: fillerenrichment.Geography{Scope: "national", Country: "US"}},
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceInference, Reference: "context.report:context-stack:citations:1",
			Confidence: 80, Producer: "context-model:test", ProducerVersion: "filler-context-v2", ObservedAt: at}}
	if _, changed, err := st.ApplyFillerEnrichment(t.Context(), research, at); err != nil || !changed {
		t.Fatalf("apply research = changed %v, err %v", changed, err)
	}
	if got, _ := st.GetClip(t.Context(), clip.Hash); got.GeographicScope != filler.GeographicNational {
		t.Fatalf("precondition: research projected %+v, want national", got.Clip)
	}

	runner := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st},
		fillerEnrichmentSignals{store: st, files: fstest.MapFS{}, stations: &stationDirectory{list: st.ListBroadcastStations}}.Load,
		func() int { return 1 }, func() time.Time { return at.Add(time.Minute) },
	)
	if result, err := runner.Run(t.Context()); err != nil || result.Considered != 1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.GeographicScope != filler.GeographicLocal || got.Country != "US" || got.Station != "fcc:59441" || got.AirDate != "1991-03-10" {
		t.Fatalf("after the station pass the clip row = %+v, want local to WEWS (fcc:59441) on 1991-03-10", got.Clip)
	}
	if filler.GeographicallyEligible(got.Clip, filler.Geography{Country: "US"}) {
		t.Fatal("Cleveland station news is still eligible for a US household with no local area")
	}
}

// #1412 end to end through the scheduler driver: production has no text model configured and a
// preparation backlog that does not drain. The deterministic pass must still be recorded for an
// existing clip that has never had one.
func TestFillerPipelineDriverRecordsPassForExistingClipUnderProductionShape(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{Hash: "existing", Path: "existing.mp4", Name: "Brambly Sauce Advert",
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

// geographyFixture registers one archive source and one break-body clip fetched from it.
func geographyFixture(t *testing.T, st fillerstore.Store, at time.Time, country, title string) (store.Clip, fstest.MapFS) {
	t.Helper()
	source := fillerstore.NewFillerSource("archive:geo", "archive", "archive.org/details/commercials", "Archive source", at)
	source.Geography = filler.Geography{Country: country}
	if err := st.UpsertFillerSource(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	clip := store.Clip{Clip: filler.Clip{
		Hash: "geo-clip", Path: "geo-clip.mp4", Name: title,
		Kind: filler.Commercial, Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"geo-clip.info.json": {Data: []byte(`{"title":"` + title + `","loomarr":{"sourceId":"archive:geo"}}`)},
	}
	return clip, files
}

func runDeterministic(t *testing.T, st fillerstore.Store, files fstest.MapFS, at time.Time) {
	t.Helper()
	runner := fillerenrichment.NewRunner(fillerEnrichmentRepository{st: st},
		fillerEnrichmentSignals{store: st, files: files}.Load,
		func() int { return 5 }, func() time.Time { return at })
	if result, err := runner.Run(t.Context()); err != nil || result.Considered != 1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	if again, err := runner.Run(t.Context()); err != nil || again.Considered != 0 {
		t.Fatalf("second Run() = %+v, err %v; the pass must be recorded so the clip is not re-selected", again, err)
	}
}

func usPod(c filler.Clip) filler.Pod {
	c.Placement = filler.PlacementBreakBody
	return filler.Assemble([]filler.Clip{c}, filler.Window{ChannelID: "ch1", Seed: 1, GapMs: 60000, PodMax: 2},
		filler.Policy{Geography: filler.Geography{Country: "US"}}, nil)
}

// #1670: British archive ads aired on a US-only household labelled country=US, because a source
// that declares no country stamped the HOUSEHOLD's home country onto every clip it fetched.
func TestFillerEnrichmentUndeclaredSourceLeavesClipGeographyUnknown(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip, files := geographyFixture(t, st, at, "", "Westbridge bank commercial")
	runDeterministic(t, st, files, at.Add(time.Minute))
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Country != "" || got.GeographicScope == filler.GeographicNational {
		t.Fatalf("undeclared source labelled the clip %+v; want unknown geography", got.Clip)
	}
	if filler.GeographicallyEligible(got.Clip, filler.Geography{Country: "US"}) {
		t.Fatalf("clip from an undeclared source passed the US-only filter: %+v", got.Clip)
	}
	if pod := usPod(got.Clip); pod.MatchLevel != filler.MatchBumperCard {
		t.Fatalf("US pod aired an unknown-geography clip: %+v", pod)
	}
}

func TestFillerEnrichmentDeclaredSourceStillLabelsClips(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip, files := geographyFixture(t, st, at, "US", "Twirlpop commercial")
	runDeterministic(t, st, files, at.Add(time.Minute))
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.GeographicScope != filler.GeographicNational || got.Country != "US" ||
		got.GeoEvidence != "registered_source:archive:geo" {
		t.Fatalf("declared US source geography = %+v, want national US from the source", got.Clip)
	}
	if pod := usPod(got.Clip); pod.MatchLevel == filler.MatchBumperCard {
		t.Fatalf("US pod over a declared-US clip fell back to the bumper card: %+v", pod)
	}
}

func TestFillerEnrichmentBritishAdvertWordingIsNotUSEvidence(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip, files := geographyFixture(t, st, at, "US", "Lynxa Advert 1994")
	runDeterministic(t, st, files, at.Add(time.Minute))
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Country != "" {
		t.Fatalf("\"Advert\" title kept the US source default: %+v", got.Clip)
	}
}

// The prod clips were stamped by producer version 3. The version bump must re-run them and retract
// the stamp; leaving it would keep every British ad that already aired eligible forever.
func TestFillerEnrichmentRetractsEarlierHomeCountryStamp(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip, files := geographyFixture(t, st, at, "", "Hollowbrook IPA commercial")
	stampedAt := at.Add(time.Minute)
	if _, err := st.ApplyFillerEnrichmentPass(t.Context(), fillerenrichment.Pass{
		ClipHash: clip.Hash, Producer: fillerenrichment.DeterministicProducer, ProducerVersion: "3",
		TaxonomyVersion: fillerenrichment.ControlledTaxonomyVersion, CompletedAt: stampedAt,
		States: []fillerenrichment.State{{ClipHash: clip.Hash, Axis: fillerenrichment.AxisGeography,
			Status: fillerenrichment.StatusComplete,
			Value:  fillerenrichment.Value{Geography: fillerenrichment.Geography{Scope: "national", Country: "US"}},
			Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceSourceDefault,
				Reference: "registered_source:archive:geo", Confidence: 100,
				Producer: fillerenrichment.DeterministicProducer, ProducerVersion: "3", ObservedAt: stampedAt}}},
	}); err != nil {
		t.Fatal(err)
	}
	if stamped, err := st.GetClip(t.Context(), clip.Hash); err != nil || stamped.Country != "US" {
		t.Fatalf("fixture did not reproduce the v3 stamp: %+v, err %v", stamped.Clip, err)
	}
	runDeterministic(t, st, files, at.Add(time.Hour))
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Country != "" || got.GeoEvidence != "" || filler.GeographicallyEligible(got.Clip, filler.Geography{Country: "US"}) {
		t.Fatalf("re-enrichment kept the inherited US stamp: %+v", got.Clip)
	}
	states, err := st.ListFillerEnrichment(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Axis == fillerenrichment.AxisGeography && state.Status != fillerenrichment.StatusUnsupported {
			t.Fatalf("geography state after retraction = %+v, want unsupported", state)
		}
	}
}

// A withdrawal retracts only its own producer's answer: a cited or operator location stays.
func TestFillerEnrichmentWithdrawalKeepsOtherEvidence(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_500, 0).UTC()
	clip, files := geographyFixture(t, st, at, "", "Tumbleworths commercial")
	if _, err := st.ApplyFillerEnrichmentPass(t.Context(), fillerenrichment.Pass{
		ClipHash: clip.Hash, Producer: "vision", ProducerVersion: "1", CompletedAt: at,
		States: []fillerenrichment.State{{ClipHash: clip.Hash, Axis: fillerenrichment.AxisGeography,
			Status: fillerenrichment.StatusComplete,
			Value:  fillerenrichment.Value{Geography: fillerenrichment.Geography{Scope: "national", Country: "GB"}},
			Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceContent, Reference: "frame.visible_text",
				Confidence: 100, Producer: "vision", ProducerVersion: "1", ObservedAt: at}}},
	}); err != nil {
		t.Fatal(err)
	}
	runDeterministic(t, st, files, at.Add(time.Hour))
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Country != "GB" {
		t.Fatalf("withdrawal cleared a content-observed location: %+v", got.Clip)
	}
}
