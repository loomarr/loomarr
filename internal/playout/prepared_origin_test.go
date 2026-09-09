package playout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/testkit/playoutprocessfixture"
)

func TestPreparedBlockContentReportsNaturalExit(t *testing.T) {
	for _, mode := range []string{"prepared-success", "prepared-failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			content := startPreparedBlockHelper(t, ctx, mode)
			got, err := io.ReadAll(content)
			if string(got) != playoutprocessfixture.PreparedPrefix {
				t.Fatalf("forwarded output = %q", got)
			}
			if mode == "prepared-failure" {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
					t.Fatalf("read error = %v; want child exit status 7", err)
				}
			} else if err != nil {
				t.Fatalf("successful child: %v", err)
			}
			if err := content.Close(); err != nil {
				t.Fatalf("close after natural exit: %v", err)
			}
		})
	}
}

func TestPreparedBlockFailureResolvesBeforeScheduledEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	content := startPreparedBlockHelper(t, ctx, "prepared-failure")
	calls := 0
	source := BlockSource(func(_ context.Context, blockRequest BlockRequest) (Block, error) {
		calls++
		if calls > 1 {
			cancel()
			return Block{}, context.Canceled
		}
		return Block{Content: content, Identity: AiringIdentity{
			ContentID: "failed-programme", EndsAt: time.Now().Add(time.Hour),
		}}, nil
	})
	var output writeCloser
	pumpBlocks(ctx, &output, source, "channel", PlanBaseline, nil)
	if calls != 2 {
		t.Fatalf("source calls = %d; partial child failure waited for scheduled end instead of resolving again", calls)
	}
	if output.String() != playoutprocessfixture.PreparedPrefix {
		t.Fatalf("forwarded prefix = %q", output.String())
	}
}

func TestPreparedBlockContentStopsAfterStdoutCloses(t *testing.T) {
	for _, stop := range []string{"cancel", "close"} {
		t.Run(stop, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			content := startPreparedBlockHelper(t, ctx, "prepared-stalled")
			prefix := make([]byte, len(playoutprocessfixture.PreparedPrefix))
			if _, err := io.ReadFull(content, prefix); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(io.Discard, content)
				close(done)
			}()
			if stop == "cancel" {
				cancel()
			} else {
				_ = content.Close()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("read did not finish after stopping the child")
			}
			if err := content.process.Wait(); err != nil {
				t.Fatalf("cancelled child was not reaped cleanly: %v", err)
			}
		})
	}
}

func startPreparedBlockHelper(t *testing.T, ctx context.Context, mode string) *processBlockContent {
	t.Helper()
	proc, err := Start(ctx, os.Args[0], []string{"-test.run=^TestProcessTreeHelper$", "--", mode}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := &processBlockContent{reader: proc.Stdout, process: proc}
	t.Cleanup(func() { _ = content.Close() })
	return content
}

type fixedPreparedResolver struct {
	window PreparedWindow
	ok     bool
}

func (r fixedPreparedResolver) ResolvePrepared(_ context.Context, _ TuneRequest, at time.Time) (PreparedWindow, bool, error) {
	return r.window, r.ok, nil
}

func preparedSpec(source string) prepared.Specification {
	return prepared.Specification{
		SourceFingerprint: source,
		Rendition: prepared.RenditionContract{
			VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080,
			FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160,
			SegmentDurationMS: 2000, PackagingVersion: prepared.CurrentPackagingVersion,
		},
	}
}

func publishHLS(t *testing.T, lib *prepared.Library, spec prepared.Specification) prepared.Publication {
	return publishHLSWithSegments(t, lib, spec, 4)
}

func publishHLSWithSegments(
	t *testing.T, lib *prepared.Library, spec prepared.Specification, segmentCount int,
) prepared.Publication {
	t.Helper()
	pub, err := lib.Publish(t.Context(), spec, func(_ context.Context, workspace string) (prepared.Output, error) {
		var manifest strings.Builder
		manifest.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:2\n")
		manifest.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")
		files := []string{prepared.MediaManifestName, "init.mp4"}
		for i := range segmentCount {
			name := fmt.Sprintf("seg-%d.m4s", i)
			files = append(files, name)
			fmt.Fprintf(&manifest, "#EXTINF:2.000,\n%s\n", name)
		}
		manifest.WriteString("#EXT-X-ENDLIST\n")
		for _, file := range files {
			body := []byte(file)
			if file == prepared.MediaManifestName {
				body = []byte(manifest.String())
			}
			if err := os.WriteFile(filepath.Join(workspace, file), body, 0o600); err != nil {
				return prepared.Output{}, err
			}
		}
		return prepared.Output{Files: files}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestPreparedOriginExposesTheSharedDVRHorizonAcrossAirings(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	oldestSpec := preparedSpec("episode-a")
	previousSpec := preparedSpec("episode-b")
	currentSpec := preparedSpec("episode-c")
	oldest := publishHLSWithSegments(t, lib, oldestSpec, 240)
	previous := publishHLSWithSegments(t, lib, previousSpec, 240)
	current := publishHLSWithSegments(t, lib, currentSpec, 240)
	started := time.Unix(10_000, 0).UTC()
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Previous: []PreparedAiring{
			{Specification: oldestSpec, StartedAt: started.Add(-16 * time.Minute)},
			{Specification: previousSpec, StartedAt: started.Add(-8 * time.Minute)},
		},
		Current: PreparedAiring{Specification: currentSpec, StartedAt: started, Offset: time.Minute},
	}})

	presentation, hit, err := origin.Tune(t.Context(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	manifest := string(presentation.Manifest)
	firstInWindow := preparedAssetToken(oldest.Key, "seg-60.m4s")
	justExpired := preparedAssetToken(oldest.Key, "seg-59.m4s")
	for _, want := range []string{
		"#EXT-X-PROGRAM-DATE-TIME:" + started.Add(-14*time.Minute).Format(time.RFC3339Nano),
		firstInWindow,
		preparedAssetToken(previous.Key, "seg-0.m4s"),
		preparedAssetToken(current.Key, "seg-30.m4s"),
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
	if strings.Contains(manifest, justExpired) {
		t.Errorf("manifest retained expired segment %q", justExpired)
	}
	if got := strings.Count(manifest, "#EXT-X-DISCONTINUITY"); got != 2 {
		t.Errorf("discontinuities = %d, want two Airing boundaries", got)
	}
}

func TestPreparedOriginRendersAKeyedWallClockManifest(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("source-a")
	pub := publishHLS(t, lib, spec)
	started := time.Unix(1_000, 0).UTC()
	preparedOrigin := newPreparedOrigin(lib, fixedPreparedResolver{
		window: PreparedWindow{Current: PreparedAiring{
			Specification: spec, StartedAt: started, Offset: 5 * time.Second,
		}}, ok: true,
	})

	presentation, hit, err := preparedOrigin.Tune(t.Context(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	manifest := string(presentation.Manifest)
	initAsset := preparedAssetToken(pub.Key, "init.mp4")
	seg0 := preparedAssetToken(pub.Key, "seg-0.m4s")
	seg1 := preparedAssetToken(pub.Key, "seg-1.m4s")
	seg2 := preparedAssetToken(pub.Key, "seg-2.m4s")
	for _, token := range []string{initAsset, seg0, seg1, seg2} {
		if strings.Contains(token, "/") {
			t.Fatalf("prepared asset token %q spans multiple route segments", token)
		}
	}
	for _, want := range []string{
		"#EXT-X-MEDIA-SEQUENCE:500", "#EXT-X-PROGRAM-DATE-TIME:1970-01-01T00:16:40Z",
		fmt.Sprintf(`#EXT-X-MAP:URI="%s"`, initAsset), seg0, seg1, seg2,
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, unwanted := range []string{preparedAssetToken(pub.Key, "seg-3.m4s"), "#EXT-X-ENDLIST"} {
		if strings.Contains(manifest, unwanted) {
			t.Errorf("manifest contains %q:\n%s", unwanted, manifest)
		}
	}

	asset, ok, err := preparedOrigin.OpenAsset("ch-one", PlanBaseline, seg2)
	if err != nil || !ok {
		t.Fatalf("OpenAsset = (_, %v, %v), want hit", ok, err)
	}
	defer func() { _ = asset.Content.Close() }()
	body, err := io.ReadAll(asset.Content)
	if err != nil || string(body) != "seg-2.m4s" {
		t.Fatalf("asset body = %q, err=%v", body, err)
	}
	if !asset.Immutable {
		t.Fatal("prepared asset is not marked immutable")
	}
	if _, ok, err := preparedOrigin.OpenAsset("ch-one", PlanBaseline, seg2+".ts"); err != nil || ok {
		t.Fatalf("asset with a forged content-type suffix = (_, %v, %v), want miss", ok, err)
	}
}

func TestPreparedMPEGTSBlockCopiesVideoAndDecodesPublicationAudioAtAiringOffset(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("source-a")
	pub := publishHLS(t, lib, spec)
	started := time.Unix(1_000, 0).UTC()
	identity := AiringIdentity{
		StartedAt: started, EndsAt: started.Add(8 * time.Minute), Kind: "program",
		ContentID: "movie:tmdb:1", ScheduleBlockID: "block-one",
	}
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Current: PreparedAiring{
			Specification: spec, StartedAt: started, Offset: 75 * time.Second, Identity: identity,
		},
	}})
	var gotArgs []string
	var gotSpec diagnostics.ProcessSpec
	source := newPreparedMPEGTSBlockSource(origin, func(
		_ context.Context, args []string, processSpec diagnostics.ProcessSpec,
	) (*Process, error) {
		gotArgs = append([]string(nil), args...)
		gotSpec = processSpec
		return &Process{Stdout: io.NopCloser(strings.NewReader("prepared-ts"))}, nil
	})

	block, err := source(t.Context(), BlockRequest{ChannelID: "ch-one", Plan: PlanFull, TimelineOrigin: started.Add(75 * time.Second), AudioBitrate: 128})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = block.Content.Close() }()
	body, err := io.ReadAll(block.Content)
	if err != nil || string(body) != "prepared-ts" {
		t.Fatalf("block body = %q, err=%v", body, err)
	}
	if block.Identity != identity {
		t.Fatalf("block identity = %+v, want %+v", block.Identity, identity)
	}
	wantFormat := BroadcastFormat{
		VideoCodec: "h264", Width: 1920, Height: 1080, Framerate: 25,
		VideoBitrate: 5000, AudioBitrate: 128,
	}
	if block.Format != wantFormat {
		t.Fatalf("block format = %+v, want %+v", block.Format, wantFormat)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{
		"-ss 75.000", "-to 480.000", "-c:v copy", "-c:a s302m", "-af atrim=start=75.000:end=480.000", "-f mpegts",
		filepath.Join(pub.Directory, prepared.MediaManifestName),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("prepared remux args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "-readrate") {
		t.Fatalf("prepared copy remux must leave pacing to the Channel mux: %s", joined)
	}
	if gotSpec.Purpose != "playout_prepared_remux" || gotSpec.ChannelID != "ch-one" ||
		gotSpec.ScheduleBlockID != "block-one" || strings.Contains(strings.Join(gotSpec.Args, " "), pub.Directory) {
		t.Fatalf("diagnostic process spec = %+v, want correlated and path-redacted", gotSpec)
	}
}

func TestPreparedMPEGTSRejectsUnsupportedPublicationFormat(t *testing.T) {
	contract := preparedSpec("source-a").Rendition
	contract.VideoCodec = "vp9"
	if _, ok := preparedBroadcastFormat(contract); ok {
		t.Fatal("raw prepared delivery accepted an unsupported video codec")
	}
	contract.VideoCodec = "h264"
	contract.AudioLayout = "5.1"
	if _, ok := preparedBroadcastFormat(contract); ok {
		t.Fatal("raw prepared delivery accepted audio that violates the stable stereo session shape")
	}
}

func TestOriginPreparedHitBypassesLiveAndMissFallsBack(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("source-a")
	publishHLS(t, lib, spec)
	hls := &tuneHLS{path: filepath.Join(t.TempDir(), "live.m3u8")}
	if err := os.WriteFile(hls.path, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}

	hitOrigin := newOrigin(newPreparedOrigin(lib, fixedPreparedResolver{
		window: PreparedWindow{Current: PreparedAiring{
			Specification: spec, StartedAt: time.Unix(1_000, 0), Offset: 0,
		}}, ok: true,
	}), nil, hls)
	got, err := hitOrigin.Tune(t.Context(), TuneRequest{ChannelID: "prepared", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || string(got.Manifest) == "live" || hls.channel != "" {
		t.Fatalf("prepared Tune = (%q, %v), live channel=%q", got.Manifest, err, hls.channel)
	}
	gotAgain, err := hitOrigin.Tune(t.Context(), TuneRequest{
		ChannelID: "another-channel", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || string(gotAgain.Manifest) != string(got.Manifest) || hls.channel != "" {
		t.Fatalf("shared prepared Tune = (%q, %v), live channel=%q", gotAgain.Manifest, err, hls.channel)
	}

	recorder := metrics.New(metrics.Options{})
	missOrigin := newOrigin(newPreparedOrigin(lib, fixedPreparedResolver{ok: false}), nil, hls)
	missOrigin.observer = recorder
	got, err = missOrigin.Tune(t.Context(), TuneRequest{ChannelID: "fallback", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || string(got.Manifest) != "live" || hls.channel != "fallback" {
		t.Fatalf("fallback Tune = (%q, %v), live channel=%q", got.Manifest, err, hls.channel)
	}
	assertMetricsContain(t, recorder, `loomarr_playout_fallbacks_total{reason="prepared_to_live"} 1`)

	hls.channel = ""
	_, err = missOrigin.Tune(t.Context(), TuneRequest{
		ChannelID: "warm-only", Plan: PlanBaseline, Delivery: DeliveryHLS, PreparedOnly: true,
	})
	if !errors.Is(err, ErrPreparedUnavailable) || hls.channel != "" {
		t.Fatalf("prepared-only miss = %v, live channel=%q; want clean miss without live fallback", err, hls.channel)
	}
}

func TestPreparedManifestCannotReferenceOutsideItsPublication(t *testing.T) {
	t.Parallel()
	spec := preparedSpec("source-a")
	window := PreparedAiring{Specification: spec, StartedAt: time.Unix(1_000, 0)}
	manifest := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\n../outside.m4s\n")
	if _, err := parsePreparedManifest(manifest, strings.Repeat("a", 64), []string{"media.m3u8"}, window); err == nil {
		t.Fatal("renderPreparedManifest accepted an asset outside the publication")
	}
}

func TestPreparedOriginCarriesThePreviousAiringAcrossADiscontinuity(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previousSpec := preparedSpec("episode-a")
	currentSpec := preparedSpec("episode-b")
	previous := publishHLS(t, lib, previousSpec)
	current := publishHLS(t, lib, currentSpec)
	started := time.Unix(1_004, 0).UTC()
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Previous: []PreparedAiring{{Specification: previousSpec, StartedAt: started.Add(-8 * time.Second)}},
		Current:  PreparedAiring{Specification: currentSpec, StartedAt: started, Offset: 500 * time.Millisecond},
	}})

	presentation, hit, err := origin.Tune(t.Context(), TuneRequest{ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	manifest := string(presentation.Manifest)
	wantOrder := []string{
		preparedAssetToken(previous.Key, "seg-2.m4s"), preparedAssetToken(previous.Key, "seg-3.m4s"),
		"#EXT-X-DISCONTINUITY", fmt.Sprintf(`#EXT-X-MAP:URI="%s"`, preparedAssetToken(current.Key, "init.mp4")),
		preparedAssetToken(current.Key, "seg-0.m4s"),
	}
	position := 0
	for _, want := range wantOrder {
		next := strings.Index(manifest[position:], want)
		if next < 0 {
			t.Fatalf("manifest missing %q after byte %d:\n%s", want, position, manifest)
		}
		position += next + len(want)
	}
}

// The rendered window must carry BOTH tags a native player needs across a programme boundary. The
// pre-existing discontinuity test asserts an ordered subsequence, which is structurally blind to a
// tag that is simply absent — so these assert presence and value instead.
func TestPreparedManifestCarriesDiscontinuitySequenceAndPerBoundaryDateTime(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	previousSpec := preparedSpec("episode-a")
	currentSpec := preparedSpec("episode-b")
	publishHLS(t, lib, previousSpec)
	publishHLS(t, lib, currentSpec)
	started := time.Unix(1_004, 0).UTC()
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Previous: []PreparedAiring{{
			Specification: previousSpec,
			StartedAt:     started.Add(-8 * time.Second),
			// Three programmes already scrolled out of this Channel's window.
			DiscontinuitySequence: 3,
		}},
		Current: PreparedAiring{Specification: currentSpec, StartedAt: started, Offset: 500 * time.Millisecond},
	}})

	presentation, hit, err := origin.Tune(t.Context(), TuneRequest{ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	manifest := string(presentation.Manifest)

	// The ordinal comes from the airing at the HEAD of the window, which is what a reload must be
	// able to correlate against.
	if want := "#EXT-X-DISCONTINUITY-SEQUENCE:3"; !strings.Contains(manifest, want) {
		t.Errorf("manifest missing %q:\n%s", want, manifest)
	}

	// One PDT per boundary: the window spans two airings, so a single head PDT is the bug.
	if got, want := strings.Count(manifest, "#EXT-X-PROGRAM-DATE-TIME:"), 2; got != want {
		t.Errorf("PROGRAM-DATE-TIME count = %d, want %d (one per boundary):\n%s", got, want, manifest)
	}

	// The second PDT must describe the segment it precedes, not repeat the window's head.
	discontinuity := strings.Index(manifest, "#EXT-X-DISCONTINUITY\n")
	if discontinuity < 0 {
		t.Fatalf("manifest has no discontinuity:\n%s", manifest)
	}
	if !strings.Contains(manifest[discontinuity:], "#EXT-X-PROGRAM-DATE-TIME:"+started.Format(time.RFC3339Nano)) {
		t.Errorf("no PDT for the current airing's start after the boundary:\n%s", manifest)
	}
}

// A Channel with no boundary history omits the tag rather than asserting a false zero point.
func TestPreparedManifestOmitsDiscontinuitySequenceAtTheStartOfAChannel(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("episode-a")
	publishHLS(t, lib, spec)
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Current: PreparedAiring{Specification: spec, StartedAt: time.Unix(1_000, 0).UTC(), Offset: time.Second},
	}})

	presentation, hit, err := origin.Tune(t.Context(), TuneRequest{ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	if manifest := string(presentation.Manifest); strings.Contains(manifest, "#EXT-X-DISCONTINUITY-SEQUENCE") {
		t.Errorf("unstarted Channel should omit the tag:\n%s", manifest)
	}
}

// A completed predecessor can be observed after the next programme has already
// started. Exercise the actual block loop and prepared adapter together: clean
// EOF alone must not replace the resolver's current position with offset zero.
func TestPumpBlocksLatePreparedHandoffRetainsCurrentPosition(t *testing.T) {
	for _, tc := range []struct {
		name           string
		predecessorGap time.Duration
		body           string
		slowLookup     bool
	}{
		{name: "clean_adjacent", body: "previous-tail"},
		{name: "clean_early_slow_lookup", body: "previous-tail", slowLookup: true},
		{name: "clean_after_gap", predecessorGap: time.Second, body: "previous-tail"},
		{name: "clean_after_missed_airing", predecessorGap: 8 * time.Second, body: "previous-tail"},
		{name: "empty_adjacent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lib, err := prepared.NewLibrary(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				spec := preparedSpec("late-handoff")
				publishHLS(t, lib, spec)
				started := time.Now().UTC().Add(-5 * time.Second)
				if tc.slowLookup {
					started = time.Now().UTC().Add(10 * time.Millisecond)
				}
				current := AiringIdentity{StartedAt: started, EndsAt: started.Add(8 * time.Second),
					Kind: "program", ContentID: "current", ScheduleBlockID: "current-block"}
				previousEnd := started.Add(-tc.predecessorGap)
				previous := AiringIdentity{StartedAt: previousEnd.Add(-8 * time.Second), EndsAt: previousEnd,
					Kind: "program", ContentID: "previous", ScheduleBlockID: "previous-block"}
				origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
					Current: PreparedAiring{Specification: spec, StartedAt: started,
						Offset: 5 * time.Second, Identity: current},
				}})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var gotArgs []string
				preparedSource := newPreparedMPEGTSBlockSource(origin, func(
					_ context.Context, args []string, _ diagnostics.ProcessSpec,
				) (*Process, error) {
					gotArgs = append([]string(nil), args...)
					cancel()
					return nil, errors.New("captured current-airing process request")
				})
				calls := 0
				source := BlockSource(func(ctx context.Context, blockRequest BlockRequest) (Block, error) {
					calls++
					if calls == 1 {
						return Block{Content: io.NopCloser(strings.NewReader(tc.body)), Identity: previous}, nil
					}
					if tc.slowLookup {
						timer := time.NewTimer(time.Until(started.Add(5 * time.Second)))
						defer timer.Stop()
						select {
						case <-ctx.Done():
							return Block{}, ctx.Err()
						case <-timer.C:
						}
					}
					return preparedSource(ctx, blockRequest)
				})
				var output writeCloser
				pumpBlocks(ctx, &output, source, "channel", PlanBaseline, nil)
				wantCalls := 2
				if tc.slowLookup {
					wantCalls = 3
				}
				if calls != wantCalls {
					t.Fatalf("source calls = %d, want %d", calls, wantCalls)
				}
				if output.String() != tc.body {
					t.Fatalf("predecessor output = %q", output.String())
				}
				joined := strings.Join(gotArgs, " ")
				if !strings.Contains(joined, "-ss 5.000 ") {
					t.Fatalf("resolved five-second offset discarded after %s: %s", tc.name, joined)
				}
			})
		})
	}
}
