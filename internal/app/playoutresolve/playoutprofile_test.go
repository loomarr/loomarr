package playoutresolve

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

type staticChannelReader struct{ channel channelstore.Channel }

func (s staticChannelReader) GetChannel(context.Context, string) (channelstore.Channel, error) {
	return s.channel, nil
}

func TestPlayoutResolver_ProfileUsesMatchingEvidenceBeforeAsyncValidation(t *testing.T) {
	loadCalls := 0
	validationStarted := make(chan struct{})
	validationRelease := make(chan struct{})
	r := &Resolver{
		tier: func() string { return "balanced" }, encoder: func() string { return "" },
		loadCapabilityEvidence: func(context.Context) (playout.Capacity, bool) {
			loadCalls++
			return playout.Capacity{Chosen: encode.EncoderNVENC, MaxChannels: 4}, true
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
	if profile.Encoder != encode.EncoderNVENC || loadCalls != 1 ||
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
	r := &Resolver{
		audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			return playout.SourceObservation{Streams: []playout.ObservedStream{
				{Index: 1, Kind: "audio", Language: "eng"}, {Index: 2, Kind: "audio", Language: "jpn"},
			}}, nil
		},
		channels: staticChannelReader{channel: channelstore.Channel{Policy: schedule.ChannelPolicy{
			OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "jpn"}},
		}}},
	}

	if got := r.AudioTrackFor(context.Background(), "channel-1", "", "movie.mkv"); got != 1 {
		t.Fatalf("AudioTrackFor = %d, want channel override track 1", got)
	}
}

// A file the scanner read takes its tracks from the Library, whose "en" matches the preferred
// "eng", with no probe; once the file changes on disk the Library's read of it is stale, so
// playout probes it as before.
func TestPlayoutResolver_AudioTrackReadsTheLibraryForAScannedFile(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, []byte("matroska"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if err := st.SaveLibraryItems(t.Context(), []store.LibraryItem{{ID: "m1", Kind: "movie", Name: "Film",
		Sources: []store.LibrarySource{{Path: path, Plays: true, Streams: []store.LibraryStream{
			{Index: 0, Kind: "video"}, {Index: 1, Kind: "audio", Language: "ru"}, {Index: 2, Kind: "audio", Language: "en"},
		}}}}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(t.Context(), []store.LibraryFile{{Path: path, Root: filepath.Dir(path),
		Size: info.Size(), ModifiedNs: info.ModTime().UnixNano(), Identity: "digest", Kind: "movie",
		Header: store.LibraryHeaderRead, SeenAt: at, ItemID: "m1"}}); err != nil {
		t.Fatal(err)
	}
	probeCalls := 0
	r := &Resolver{
		libraryAudio: st, audioLanguage: func() string { return "eng" },
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{Streams: []playout.ObservedStream{
				{Index: 1, Kind: "audio", Language: "eng"}, {Index: 2, Kind: "audio", Language: "rus"},
			}}, nil
		},
	}
	if got := r.AudioTrackFor(t.Context(), "channel-1", "", path); got != 1 || probeCalls != 0 {
		t.Fatalf("AudioTrackFor = %d with %d probes; want the Library's en track (ordinal 1) and no probe", got, probeCalls)
	}
	if err := os.WriteFile(path, []byte("matroska, remuxed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.AudioTrackFor(t.Context(), "channel-1", "", path); got != 0 || probeCalls != 1 {
		t.Fatalf("after the file changed, AudioTrackFor = %d with %d probes; want the probe's eng track (ordinal 0)", got, probeCalls)
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
	r := &Resolver{
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
	r := &Resolver{
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
	format := r.FormatOf(t.Context(), "", path)
	if format.VideoCodec != "h264" || format.AudioCodec != "aac" ||
		format.Width != 1920 || format.Height != 1080 || format.FrameRate != 25 {
		t.Fatalf("inventory-backed FormatOf = %+v, want 1080p25 h264/aac", format)
	}
	if sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("probe calls = source %d, format %d; want one shared source probe", sourceProbes, formatProbes)
	}
}

func TestPlayoutResolver_EmptyAudioPreferenceSkipsTheLibraryAndProbe(t *testing.T) {
	probeCalls := 0
	r := &Resolver{
		audioLanguage: func() string { return "  " },
		libraryAudio:  libraryAudioPanics{t},
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			probeCalls++
			return playout.SourceObservation{}, nil
		},
	}
	if got := r.AudioTrackFor(context.Background(), "channel-1", "item-1", "movie.mkv"); got != 0 {
		t.Fatalf("AudioTrackFor = %d, want first track", got)
	}
	if probeCalls != 0 {
		t.Fatalf("empty preference probed %d times", probeCalls)
	}
}

type libraryAudioPanics struct{ t *testing.T }

func (f libraryAudioPanics) LibraryAudioTracks(context.Context, string, int64, int64) ([]store.LibraryStream, bool, error) {
	f.t.Fatal("an empty preference read the Library")
	return nil, false, nil
}

// ⚠ **THE QUALITY LADDER'S DEPENDENCIES ARE CALLED UNGUARDED.**
//
// `Profile` reaches `r.tier()` and `r.encoder()` with no nil checks, so either one missing is
// a panic on the LIVE playout path — when a viewer tunes in, which is the worst place to find
// out. (The rung is the session's ResourceBudget lease, passed in; it has no resolver input.)
func TestPlayoutResolver_ProfileNeedsEveryLadderInput(t *testing.T) {
	// A resolver wired the way Build wires it — every ladder input present.
	full := func() *Resolver {
		return &Resolver{
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
		bust func(*Resolver)
	}{
		{"tier", func(r *Resolver) { r.tier = nil }},
		{"encoder", func(r *Resolver) { r.encoder = nil }},
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
	r := &Resolver{
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
	r.measurer = r.NewMeasurer(measure.Tools{}, st)

	format := r.FormatOf(t.Context(), "", path)
	pipe, err := playout.Build(encode.HostFor(encode.EncoderSoftware, false, encode.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
	if err != nil || len(pipe.MissingFacts) != 0 || !slices.Contains(pipe.PreInput, "-fpsprobesize") {
		t.Fatalf("first-play pipeline = %+v, err %v; want measured facts to activate the minimal probe", pipe, err)
	}
	if sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("first play probes = source %d, format %d; want exactly one source probe", sourceProbes, formatProbes)
	}
	if format2 := r.FormatOf(t.Context(), "", path); format2.VideoCodec != "h264" || sourceProbes != 1 || formatProbes != 0 {
		t.Fatalf("second tune: format %+v, probes source %d format %d; want stored facts and no probe", format2, sourceProbes, formatProbes)
	}
}

// The channel watermark anchors to the measured picture (#1512 1d): once the source's analysis
// holds its active picture, the airtime format carries it, so a letterboxed film's bug sits on
// the picture's corner, not in the black bar.
func TestPlayoutResolver_AirtimeFormatCarriesTheMeasuredActivePicture(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/film.mkv"
	if err := os.WriteFile(path, []byte("one stable local revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{
		inventory: inventory.New(st), analyses: st, now: time.Now,
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			return playout.SourceObservation{
				Container: "matroska,webm", DurationMillis: 90_000,
				Streams: []playout.ObservedStream{
					{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: "25/1", PixelFormat: "yuv420p"},
				},
			}, nil
		},
	}
	r.measurer = r.NewMeasurer(measure.Tools{}, st)
	if format := r.FormatOf(t.Context(), "", path); format.Active != (playout.Rect{}) {
		t.Fatalf("unmeasured source: active %+v, want unknown", format.Active)
	}
	origin, ok := r.ensureLocalInventorySource(t.Context(), path)
	if !ok {
		t.Fatal("no local inventory source")
	}
	source, found, err := r.inventory.ResolveSource(t.Context(), inventory.SourceRequest{
		Item: inventory.ItemRef{Origin: &origin}, Now: time.Now(), Kinds: []inventory.SourceKind{inventory.SourceLocalFile}})
	if err != nil || !found {
		t.Fatalf("resolve source: found %v err %v", found, err)
	}
	if err := st.RecordInventoryAnalysis(t.Context(), inventory.Analysis{SourceID: source.ID, Revision: source.Revision,
		AnalyzedAt: time.Now(), ActivePicture: &inventory.PictureArea{Y: 140, W: 1920, H: 800}}); err != nil {
		t.Fatal(err)
	}
	if format := r.FormatOf(t.Context(), "", path); format.Active != (playout.Rect{Y: 140, W: 1920, H: 800}) {
		t.Fatalf("measured source: active %+v, want the stored letterbox picture", format.Active)
	}
}

func TestPlayoutResolver_UnmeasurableSourceKeepsFullProbe(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := t.TempDir() + "/movie.ts"
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{
		inventory: inventory.New(st), now: time.Now,
		probeSource: func(context.Context, string) (playout.SourceObservation, error) {
			return playout.SourceObservation{}, errors.New("unreadable")
		},
		probeFormat: func(context.Context, string) (playout.MediaFormat, error) { return playout.MediaFormat{}, nil },
	}
	r.measurer = r.NewMeasurer(measure.Tools{}, st)
	format := r.FormatOf(t.Context(), "", path)
	pipe, err := playout.Build(encode.HostFor(encode.EncoderSoftware, false, encode.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
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
	r := &Resolver{inventory: inventory.New(st), now: time.Now,
		probeSource: playout.FFprobeSourceNextTo(ffmpeg)}
	m := r.NewMeasurer(measure.DefaultTools(ffmpeg, playout.FFprobeBeside(ffmpeg)), st)
	r.measurer = m
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go m.Run(ctx)

	format := r.FormatOf(ctx, "", path)
	pipe, err := playout.Build(encode.HostFor(encode.EncoderSoftware, false, encode.GPUFilters{}), format, playout.ChannelOutput(playout.DefaultProfile()))
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
	r := &Resolver{
		tier: func() string { return "balanced" }, encoder: func() string { return "" },
		detectContext: t.Context(),
		loadCapabilityEvidence: func(context.Context) (playout.Capacity, bool) {
			loadCalls.Add(1)
			time.Sleep(200 * time.Millisecond)
			return playout.Capacity{Chosen: encode.EncoderNVENC, MaxChannels: 4}, true
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
	if profile.Encoder != encode.EncoderNVENC {
		t.Fatalf("first Profile encoder %q, want the warmed NVENC evidence", profile.Encoder)
	}
	_ = r.detectedEncoder(t.Context())
}
