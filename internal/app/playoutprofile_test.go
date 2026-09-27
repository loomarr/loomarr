package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/mediameasure"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type staticChannelReader struct{ channel store.Channel }

func (s staticChannelReader) GetChannel(context.Context, string) (store.Channel, error) {
	return s.channel, nil
}

func TestBuild_WiresMeasuredCapacityToAdmissionAndQuality(t *testing.T) {
	t.Setenv("API_TOKEN", "capacity-test-token")

	st := testkit.MigratedSQLiteStore(t)
	for key, value := range map[string]string{
		"playout.backend":      "internal",
		"playout.encoder":      "libx264",
		"playout.max_channels": "9",
	} {
		if err := st.SetSetting(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	h := application.Handler()
	r := application.playoutResolver
	if r == nil {
		t.Fatal("Build wired no playout resolver")
	}
	// Detection is lazy; install the result a real encoder trial would publish without running
	// ffmpeg in a unit test. The configured 9 is deliberately above the measured 3.
	r.maxChannels.Store(3)

	req := httptest.NewRequest(http.MethodGet, "/v1/playout/sessions", nil)
	req.Header.Set("Authorization", "Bearer capacity-test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/playout/sessions = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var telemetry api.PlayoutTelemetry
	if err := json.NewDecoder(rec.Body).Decode(&telemetry); err != nil {
		t.Fatal(err)
	}
	if telemetry.Capacity != 3 {
		t.Errorf("admission capacity = %d, want measured capacity 3", telemetry.Capacity)
	}
}

func TestPreparedEncodePoolUsesEffectiveCapacity(t *testing.T) {
	measuredCalls := 0
	pool := newPreparedEncodePool(
		func() playout.Encoder { return playout.EncoderVAAPI },
		func() int {
			measuredCalls++
			return 12
		},
		func(measured int) int {
			if measured != 12 {
				t.Fatalf("effective capacity received measured %d, want 12", measured)
			}
			return 4
		},
	)
	var releases []func()
	for i := range 3 {
		_, release, ok := pool.AcquireBackground(t.Context(), time.Unix(int64(i), 0))
		if !ok {
			t.Fatalf("background lease %d refused below effective capacity reserve", i+1)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	if _, _, ok := pool.AcquireBackground(t.Context(), time.Unix(3, 0)); ok {
		t.Fatal("prepared pool bypassed operator cap: fourth background lease admitted at effective capacity four")
	}
	if measuredCalls != 4 {
		t.Fatalf("measured capacity calls = %d, want one refresh for every admission", measuredCalls)
	}
}

func TestPreparedEncodePoolDisablesHardwarePreparationForSoftwareOverride(t *testing.T) {
	pool := newPreparedEncodePool(
		func() playout.Encoder { return playout.EncoderSoftware },
		func() int { t.Fatal("software override invoked the hardware capacity probe"); return 0 },
		func(int) int { t.Fatal("software override applied a hardware capacity"); return 0 },
	)
	if _, _, ok := pool.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("software override admitted hardware preparation")
	}
}

func TestPreparedEncodePoolRefreshesMeasuredAndEffectiveCapacity(t *testing.T) {
	measured, effectiveLimit := 1, 4
	pool := newPreparedEncodePool(
		func() playout.Encoder { return playout.EncoderVAAPI },
		func() int { return measured },
		func(value int) int { return min(value, effectiveLimit) },
	)
	if _, _, ok := pool.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("background work admitted before multi-slot measurement completed")
	}

	measured = 12
	releases := make([]func(), 0, 3)
	for i := range 3 {
		_, release, ok := pool.AcquireBackground(t.Context(), time.Unix(int64(i), 0))
		if !ok {
			t.Fatalf("background lease %d did not observe the completed measurement", i+1)
		}
		releases = append(releases, release)
	}
	effectiveLimit = 2
	if _, _, ok := pool.AcquireBackground(t.Context(), time.Time{}); ok {
		t.Fatal("background work ignored a lowered effective capacity")
	}
	for _, release := range releases {
		release()
	}
}

func TestPlayoutResolver_ProfileUsesMatchingEvidenceBeforeAsyncValidation(t *testing.T) {
	loadCalls := 0
	validationStarted := make(chan struct{})
	validationRelease := make(chan struct{})
	r := &playoutResolver{
		tier: func() string { return "balanced" }, encoder: func() string { return "" },
		loadCapabilityEvidence: func(context.Context) (playout.Capacity, bool) {
			loadCalls++
			return playout.Capacity{Chosen: playout.EncoderNVENC, MaxChannels: 4}, true
		},
		ffmpegPath: func() string {
			close(validationStarted)
			<-validationRelease
			return "/nonexistent/ffmpeg"
		},
	}

	before := time.Now()
	profile := r.Profile(t.Context(), 0)
	if elapsed := time.Since(before); elapsed > 100*time.Millisecond {
		t.Fatalf("Profile waited %s for asynchronous capability validation", elapsed)
	}
	if profile.Encoder != playout.EncoderNVENC || loadCalls != 1 ||
		r.maxChannels.Load() != 4 || !r.detectReady.Load() {
		t.Fatalf("first Profile = %+v, load calls=%d max=%d ready=%v; want matching NVENC evidence",
			profile, loadCalls, r.maxChannels.Load(), r.detectReady.Load())
	}
	select {
	case <-validationStarted:
	case <-time.After(time.Second):
		t.Fatal("Profile did not start evidence validation in the background")
	}
	close(validationRelease)
	_ = r.detectedEncoder(t.Context())
}

func TestPlayoutResolver_AudioTrackHonoursChannelOverride(t *testing.T) {
	r := &playoutResolver{
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			return playout.SourceObservation{Streams: []playout.ObservedStream{
				{Index: 1, Kind: "audio", Language: "eng"}, {Index: 2, Kind: "audio", Language: "jpn"},
			}}, nil
		},
		channels: staticChannelReader{channel: store.Channel{Policy: schedule.ChannelPolicy{
			OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "jpn"}},
		}}},
	}

	if got := r.AudioTrackFor(context.Background(), "channel-1", "", "movie.mkv"); got != 1 {
		t.Fatalf("AudioTrackFor = %d, want channel override track 1", got)
	}
}

func TestPlayoutResolver_AudioTrackWarmsInventoryThenPerformsNoIO(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(`{
		"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
		"MediaSources":[{"Id":"source-1","ETag":"rev-1","MediaStreams":[
			{"Index":0,"Type":"Video","Codec":"h264"},
			{"Index":2,"Type":"Audio","Language":"jpn"},
			{"Index":4,"Type":"Audio","Language":"eng"}
		]}]
	}`)}
	probeCalls := 0
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 1 {
		t.Fatalf("cold metadata-first AudioTrackFor = %d, want audio ordinal 1", got)
	}
	firstRequests := len(server.Requests())
	if firstRequests != 1 || probeCalls != 0 {
		t.Fatalf("cold metadata-first calls = library %d, ffprobe %d; want 1, 0", firstRequests, probeCalls)
	}
	server.InventoryItems = nil
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 1 {
		t.Fatalf("warm inventory AudioTrackFor = %d, want audio ordinal 1", got)
	}
	if got := len(server.Requests()); got != firstRequests || probeCalls != 0 {
		t.Fatalf("warm inventory added calls = library %d→%d, ffprobe %d; want none", firstRequests, got, probeCalls)
	}
}

func TestPlayoutResolver_AudioTrackPersistsProbeFallback(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	server.InventoryItems = map[string]json.RawMessage{"item-1": json.RawMessage(`{
		"Id":"item-1","Type":"Movie","DateLastSaved":"2026-09-04T12:00:00Z",
		"MediaSources":[{"Id":"source-1","ETag":"rev-1","MediaStreams":[
			{"Index":2,"Type":"Audio","Language":"jpn"},
			{"Index":2,"Type":"Audio","Language":"eng"}
		]}]
	}`)}
	probeCalls := 0
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{Container: "matroska", Streams: []playout.ObservedStream{
				{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
				{Index: 1, Kind: "audio", Codec: "aac", Language: "jpn", Channels: 2},
				{Index: 3, Kind: "audio", Codec: "aac", Language: "eng", Channels: 2},
			}}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 1 {
		t.Fatalf("probe fallback AudioTrackFor = %d, want ordinal 1", got)
	}
	firstRequests := len(server.Requests())
	server.InventoryItems = nil
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 1 {
		t.Fatalf("measured inventory AudioTrackFor = %d, want ordinal 1", got)
	}
	if len(server.Requests()) != firstRequests || probeCalls != 1 {
		t.Fatalf("measured warm calls = library %d→%d, ffprobe %d; want no second I/O", firstRequests, len(server.Requests()), probeCalls)
	}
}

func TestPlayoutResolver_AudioTrackPersistsFullProbeWhenLibraryUnavailable(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	server := testkit.NewMediaServer(t)
	probeCalls := 0
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	r := &playoutResolver{
		lib: newTestLibraryClient(server), inventory: inventory.New(st), now: func() time.Time { return now },
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{Container: "matroska", DurationMillis: 90_000, Bitrate: 4_000_000,
				UnsafePreroll: true,
				Streams: []playout.ObservedStream{
					{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
					{Index: 1, Kind: "audio", Codec: "aac", Language: "jpn", Channels: 2},
					{Index: 2, Kind: "audio", Codec: "aac", Language: "eng", Channels: 2},
				}}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", server.URL+"/video"); got != 1 {
		t.Fatalf("cold fallback AudioTrackFor = %d, want ordinal 1", got)
	}
	firstRequests := len(server.Requests())
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", server.URL+"/video"); got != 1 {
		t.Fatalf("warm fallback AudioTrackFor = %d, want ordinal 1", got)
	}
	if len(server.Requests()) != firstRequests || probeCalls != 1 {
		t.Fatalf("warm fallback added I/O: library %d→%d, probes %d", firstRequests, len(server.Requests()), probeCalls)
	}
	origin, err := r.lib.InventoryOrigin("item-1")
	if err != nil {
		t.Fatal(err)
	}
	item, ok, err := r.inventory.Item(context.Background(), inventory.ItemRef{Origin: &origin})
	if err != nil || !ok || len(item.Sources) != 1 || item.Sources[0].Measurement == nil {
		t.Fatalf("persisted fallback = (%+v, %v, %v)", item, ok, err)
	}
	facts := item.Sources[0].Measurement.Observation.Facts
	if facts.Container != "matroska" || facts.DurationMillis != 90_000 || !facts.UnsafePreroll || len(facts.Streams) != 3 ||
		facts.Streams[0].Width != 1920 {
		t.Fatalf("persisted probe lost superset facts: %+v", facts)
	}
}

func TestPlayoutResolver_LocalFileRevisionInvalidatesMeasuredAudio(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/movie.mkv"
	if err := os.WriteFile(path, []byte("first revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	probeCalls := 0
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	r := &playoutResolver{
		inventory: inventory.New(st), now: func() time.Time { return now },
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{Streams: []playout.ObservedStream{
				{Index: 1, Kind: "audio", Language: "jpn"}, {Index: 2, Kind: "audio", Language: "eng"},
			}}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", path); got != 1 {
		t.Fatalf("cold local AudioTrackFor = %d, want ordinal 1", got)
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", path); got != 1 || probeCalls != 1 {
		t.Fatalf("warm local = %d, probes %d; want 1, 1", got, probeCalls)
	}
	changed := now.Add(time.Hour)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", path); got != 1 || probeCalls != 2 {
		t.Fatalf("changed local = %d, probes %d; want revision invalidation and second probe", got, probeCalls)
	}
}

func TestPlayoutResolver_LocalAudioProbeFeedsCopyPlanWithoutSecondProbe(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/movie.mkv"
	if err := os.WriteFile(path, []byte("one stable local revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceProbes, formatProbes := 0, 0
	r := &playoutResolver{
		inventory: inventory.New(st), now: time.Now,
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			sourceProbes++
			return playout.SourceObservation{
				Container: "matroska", DurationMillis: 90_000, Bitrate: 4_000_000,
				Streams: []playout.ObservedStream{
					{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080,
						FrameRate: "25/1", PixelFormat: "yuv420p"},
					{Index: 1, Kind: "audio", Codec: "aac", Language: "jpn", Channels: 2, SampleRate: 48_000},
					{Index: 2, Kind: "audio", Codec: "aac", Language: "eng", Channels: 2, SampleRate: 48_000},
				},
			}, nil
		},
		probeFormat: func(context.Context, string) (playout.MediaFormat, error) {
			formatProbes++
			return playout.MediaFormat{}, nil
		},
	}

	if got := r.AudioTrackFor(t.Context(), "channel-1", "item-1", path); got != 1 {
		t.Fatalf("AudioTrackFor = %d, want English audio ordinal 1", got)
	}
	plan, format := r.PlanFor(t.Context(), path, playout.PlanFull)
	if !plan.DirectPlay() || format.VideoCodec != "h264" || format.AudioCodec != "aac" ||
		format.Width != 1920 || format.Height != 1080 || format.FrameRate != 25 {
		t.Fatalf("inventory-backed PlanFor = (%+v, %+v), want direct-play 1080p25 h264/aac", plan, format)
	}
	if sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("probe calls = source %d, format %d; want one shared source probe", sourceProbes, formatProbes)
	}
}

func TestPlayoutResolver_EmptyAudioPreferenceSkipsInventoryLibraryAndProbe(t *testing.T) {
	server := testkit.NewMediaServer(t)
	probeCalls := 0
	r := &playoutResolver{
		lib: newTestLibraryClient(server), audioLanguage: func() string { return "  " },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 0 {
		t.Fatalf("AudioTrackFor = %d, want first track", got)
	}
	if len(server.Requests()) != 0 || probeCalls != 0 {
		t.Fatalf("empty preference performed I/O: requests=%d probes=%d", len(server.Requests()), probeCalls)
	}
}

func newTestLibraryClient(server *testkit.MediaServer) *library.Client {
	return library.New(library.Emby, strings.TrimRight(server.URL, "/"), server.AdminToken, "inventory-test-device")
}

// ⚠ **THE QUALITY LADDER'S DEPENDENCIES ARE CALLED UNGUARDED.**
//
// `Profile` reaches `r.tier()` and `r.encoder()` with no nil checks, so either one missing is
// a panic on the LIVE playout path — when a viewer tunes in, which is the worst place to find
// out. (The rung is the session's ResourceBudget lease, passed in; it has no resolver input.)
func TestPlayoutResolver_ProfileNeedsEveryLadderInput(t *testing.T) {
	// A resolver wired the way Build wires it — every ladder input present.
	full := func() *playoutResolver {
		return &playoutResolver{
			tier:    func() string { return "720p" },
			encoder: func() string { return "libx264" },
		}
	}

	// The positive case first, so the negatives below are proven to be panics rather than a
	// resolver that never works.
	if got := full().Profile(context.Background(), 0); got.Encoder == "" {
		t.Fatalf("Profile with every input wired returned %+v, want a usable profile", got)
	}

	// ⚠ Each input removed IN TURN must panic rather than silently degrade. A zero value
	// here would be worse than a crash: the ladder would quietly pick the wrong quality and
	// nobody would know which input was missing.
	for _, tc := range []struct {
		name string
		bust func(*playoutResolver)
	}{
		{"tier", func(r *playoutResolver) { r.tier = nil }},
		{"encoder", func(r *playoutResolver) { r.encoder = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := full()
			tc.bust(r)
			defer func() {
				if recover() == nil {
					t.Errorf("Profile with %s unset did NOT panic — it is called unguarded, "+
						"so an unset input must fail loudly rather than resolve a wrong quality",
						tc.name)
				}
			}()
			_ = r.Profile(context.Background(), 0)
		})
	}
}

// ⚠ **THE ASSERTION THAT WAS MISSING.** The test above pins the invariant (an unset ladder
// input panics); this pins that Build actually satisfies it.
//
// Both are needed, and the gap between them is exactly where the original defect lived:
// `activeChannels` was back-patched after construction, and deleting the assignment left
// every test green while a viewer tuning in would panic.
//
// Reads the resolver Build really constructed rather than one assembled here, since
// a test-built resolver only proves the test knows how to fill a struct.
func TestBuild_WiresEveryLadderInput(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	// Playout is only wired on the internal backend; without this the resolver is nil and
	// the test would pass vacuously.
	if err := st.SetSetting(context.Background(), "playout.backend", "internal"); err != nil {
		t.Fatal(err)
	}

	application, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	r := application.playoutResolver
	if r == nil {
		t.Fatal("Build wired no playout resolver on the internal backend — " +
			"this test can no longer see what it is meant to guard")
	}

	for _, tc := range []struct {
		name string
		set  bool
	}{
		{"tier", r.tier != nil},
		{"encoder", r.encoder != nil},
	} {
		if !tc.set {
			t.Errorf("Build left %s unset — Profile calls it unguarded, so a viewer "+
				"tuning in would panic", tc.name)
		}
	}
}

// A source that has never been measured is probed ONCE at first play, its facts are recorded, and
// the builder then receives them: the minimal probe flags appear only because facts exist. The
// second tune reads the stored facts and asks nothing of ffprobe.
func TestPlayoutResolver_FirstPlayMeasuresFactsOnceAndActivatesMinimalProbe(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/movie.mkv"
	if err := os.WriteFile(path, []byte("one stable local revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceProbes, formatProbes := 0, 0
	r := &playoutResolver{
		inventory: inventory.New(st), now: time.Now,
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			sourceProbes++
			return playout.SourceObservation{
				Container: "matroska,webm", DurationMillis: 90_000,
				Streams: []playout.ObservedStream{
					{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: "25/1", PixelFormat: "yuv420p"},
					{Index: 1, Kind: "audio", Codec: "aac", Channels: 2, SampleRate: 48_000},
				},
			}, nil
		},
		probeFormat: func(context.Context, string) (playout.MediaFormat, error) {
			formatProbes++
			return playout.MediaFormat{}, nil
		},
	}
	r.measurer = r.newMeasurer(mediameasure.Tools{}, st)

	_, format := r.PlanFor(t.Context(), path, playout.PlanFull)
	pipe, err := playout.Build(playout.HostFor(playout.EncoderSoftware, false, playout.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
	if err != nil || len(pipe.MissingFacts) != 0 || !slices.Contains(pipe.PreInput, "-fpsprobesize") {
		t.Fatalf("first-play pipeline = %+v, err %v; want measured facts to activate the minimal probe", pipe, err)
	}
	if sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("first play probes = source %d, format %d; want exactly one source probe", sourceProbes, formatProbes)
	}
	if _, format2 := r.PlanFor(t.Context(), path, playout.PlanFull); format2.VideoCodec != "h264" || sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("second tune: format %+v, probes source %d format %d; want stored facts and no probe", format2, sourceProbes, formatProbes)
	}
}

func TestPlayoutResolver_UnmeasurableSourceKeepsFullProbe(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/movie.ts"
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &playoutResolver{
		inventory: inventory.New(st), now: time.Now,
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			return playout.SourceObservation{}, errors.New("unreadable")
		},
		probeFormat: func(context.Context, string) (playout.MediaFormat, error) { return playout.MediaFormat{}, nil },
	}
	r.measurer = r.newMeasurer(mediameasure.Tools{}, st)
	_, format := r.PlanFor(t.Context(), path, playout.PlanFull)
	pipe, err := playout.Build(playout.HostFor(playout.EncoderSoftware, false, playout.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
	if err != nil || len(pipe.MissingFacts) == 0 || slices.Contains(pipe.PreInput, "-fpsprobesize") {
		t.Fatalf("pipeline without facts = %+v, err %v; want no minimal probe", pipe, err)
	}
}

// Real ffprobe, real store: a Matroska file's measured facts must be complete enough that the
// builder activates minimal probing, and the background analysis must then land in the store.
func TestPlayoutResolver_FirstPlayWithRealFFprobeActivatesMinimalProbeAndStoresAnalysis(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "clip.mkv")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=4", "-f", "lavfi", "-i", "sine=f=440:r=48000:d=4",
		"-c:v", "mpeg4", "-g", "25", "-c:a", "aac", path).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	st := testkit.MigratedSQLiteStore(t)
	r := &playoutResolver{inventory: inventory.New(st), now: time.Now,
		probeSource: playout.FFprobeSourceNextTo(ffmpeg)}
	m := r.newMeasurer(mediameasure.DefaultTools(ffmpeg, playout.FFprobeBeside(ffmpeg)), st)
	r.measurer = m
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go m.Run(ctx)

	_, format := r.PlanFor(ctx, path, playout.PlanFull)
	pipe, err := playout.Build(playout.HostFor(playout.EncoderSoftware, false, playout.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
	if err != nil || len(pipe.MissingFacts) != 0 || !slices.Contains(pipe.PreInput, "-fpsprobesize") {
		t.Fatalf("real-ffprobe facts %+v left MissingFacts %v (err %v)", format, pipe.MissingFacts, err)
	}
	m.Drain()
	origin, _ := r.ensureLocalInventorySource(ctx, path)
	item, ok, err := st.InventoryItem(ctx, inventory.ItemRef{Origin: &origin})
	if err != nil || !ok {
		t.Fatalf("item: ok %v err %v", ok, err)
	}
	a, ok, err := st.InventoryAnalysis(ctx, item.Sources[0].ID)
	if err != nil || !ok || len(a.Keyframes) != 4 || a.IntegratedLUFS == nil {
		t.Fatalf("stored analysis = %+v ok %v err %v; want 4 keyframes and loudness", a, ok, err)
	}
}

// The host fingerprint behind the persisted encoder evidence cost a cold first tune 0.2 s live
// (#1512 G2). Warmed at boot, the first Profile reads the loaded evidence and loads nothing.
func TestPlayoutResolver_WarmProfileTakesTheEvidenceOffTheFirstTune(t *testing.T) {
	var loadCalls atomic.Int32
	r := &playoutResolver{
		tier: func() string { return "balanced" }, encoder: func() string { return "" },
		detectContext: t.Context(),
		loadCapabilityEvidence: func(context.Context) (playout.Capacity, bool) {
			loadCalls.Add(1)
			time.Sleep(200 * time.Millisecond)
			return playout.Capacity{Chosen: playout.EncoderNVENC, MaxChannels: 4}, true
		},
		ffmpegPath: func() string { return "/nonexistent/ffmpeg" },
	}
	r.WarmProfile(t.Context())

	before := time.Now()
	profile := r.Profile(t.Context(), 0)
	if elapsed := time.Since(before); elapsed > 50*time.Millisecond || loadCalls.Load() != 1 {
		t.Fatalf("first Profile after WarmProfile took %s with %d evidence loads; want no load on the tune",
			elapsed, loadCalls.Load())
	}
	if profile.Encoder != playout.EncoderNVENC {
		t.Fatalf("first Profile encoder %q, want the warmed NVENC evidence", profile.Encoder)
	}
	_ = r.detectedEncoder(t.Context())
}
