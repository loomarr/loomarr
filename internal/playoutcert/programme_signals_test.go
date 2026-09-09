package playoutcert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

type programmeSignalFixture struct {
	config          Config
	truth           *playoutcertfixture.ProgrammeEvidence[ProgrammeEvidence]
	decoder         *playoutcertfixture.SignalDecoder[DecodedVideoSignal, DecodedAudioSignal]
	bodies          []*playoutcertfixture.PacedBody
	requests        atomic.Int32
	clocks          atomic.Int32
	origin          time.Time
	discontinuity   bool
	thirdEpoch      bool
	raw             bool
	blockRefresh    bool
	refreshes       atomic.Int32
	blocked         chan struct{}
	refreshCanceled chan struct{}
}

func newProgrammeSignalFixture(t *testing.T) *programmeSignalFixture {
	t.Helper()
	f := &programmeSignalFixture{origin: time.Now().Add(-time.Second), blocked: make(chan struct{}), refreshCanceled: make(chan struct{})}
	f.decoder = &playoutcertfixture.SignalDecoder[DecodedVideoSignal, DecodedAudioSignal]{}
	for i := range 40 {
		pts, luma, rate := int64(1_550_000+(i-2)*25_000), float64(235), 0.037
		if i < 2 {
			pts, luma, rate = int64(1_150_000+i*25_000), 16, 0.018
		}
		v := DecodedVideoSignal{PTSUS: pts, Luma: luma}
		a := DecodedAudioSignal{PTSUS: pts, Samples: 1024, ZeroCrossingRate: rate, RMSDB: -24}
		f.decoder.Signals = append(f.decoder.Signals, playoutcertfixture.SignalPair[DecodedVideoSignal, DecodedAudioSignal]{Video: &v, Audio: &a})
	}
	first, next := []byte{0, 1}, make([]byte, 38)
	for i := range next {
		next[i] = byte(i + 2)
	}
	f.bodies = []*playoutcertfixture.PacedBody{playoutcertfixture.NewPacedBody(first, 25*time.Millisecond, false), playoutcertfixture.NewPacedBody(next, 25*time.Millisecond, true)}
	f.truth = &playoutcertfixture.ProgrammeEvidence[ProgrammeEvidence]{Value: ProgrammeEvidence{Programmes: []ExpectedProgramme{
		{StartsAt: f.origin, EndsAt: f.origin.Add(1400 * time.Millisecond), Luma: SignalRange{0, 25}, ZeroCrossingRate: SignalRange{0.012, 0.026}, RMSDB: SignalRange{-80, -1}},
		{StartsAt: f.origin.Add(1400 * time.Millisecond), EndsAt: f.origin.Add(10 * time.Second), Luma: SignalRange{225, 255}, ZeroCrossingRate: SignalRange{0.027, 0.05}, RMSDB: SignalRange{-80, -1}},
	}}}
	f.truth.Value.ResolveAsset = func(_ context.Context, asset ProgrammeAsset) (ProgrammeAssetEvidence, error) {
		f.clocks.Add(1)
		index := strings.Index("abc", asset.Reference)
		if len(asset.Reference) != 1 || index < 0 || index >= len(f.bodies) {
			return ProgrammeAssetEvidence{}, errors.New("unowned fixture asset")
		}
		media := append([]byte(nil), f.bodies[index].Bytes...)
		if len(media) == 0 {
			media = []byte{0}
		}
		return ProgrammeAssetEvidence{Clock: ProgrammeMediaClock{Origin: f.origin, Generation: "private-source"}, Media: media}, nil
	}
	f.config = Config{BaseURL: "http://127.0.0.1:18080", AdminBearer: "private-admin", Channels: []Channel{{ID: "channel", Roles: []string{"prepared"}}}, RequestTimeout: time.Second, ProgrammeBoundaryTimeout: 2 * time.Second, ProgrammeBoundaryLateObservation: 150 * time.Millisecond, RawCaptureBytes: 188, ProgrammeEvidence: f.truth, SignalDecoder: f.decoder, Validator: &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}}}
	f.config.Client = &http.Client{Transport: httpfixture.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		f.requests.Add(1)
		response := func(body io.ReadCloser) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: r}
		}
		if r.URL.Path == "/v1/channels/channel/play-url" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer private-admin" {
				return nil, errors.New("unexpected mint request")
			}
			return response(io.NopCloser(strings.NewReader(`{"relativeUrl":"/hls?sig=private-signature"}`))), nil
		}
		if r.URL.Query().Get("mode") != "" || r.URL.Query().Get("sig") != "private-signature" {
			return nil, errors.New("ordinary signed HLS route required")
		}
		switch r.URL.Path {
		case "/hls":
			if f.blockRefresh && f.refreshes.Add(1) > 1 {
				close(f.blocked)
				<-r.Context().Done()
				close(f.refreshCanceled)
				return nil, r.Context().Err()
			}
			manifest := fmt.Sprintf("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init?sig=private-signature\"\n#EXT-X-PROGRAM-DATE-TIME:%s\n#EXTINF:0.1,\na?sig=private-signature\n#EXTINF:5,\nb?sig=private-signature\n", f.origin.Format(time.RFC3339Nano))
			if f.raw {
				manifest = strings.Replace(manifest, "#EXT-X-MAP:URI=\"init?sig=private-signature\"\n", "", 1)
			}
			if f.blockRefresh {
				manifest = strings.Split(manifest, "#EXTINF:5,")[0]
			}
			if f.discontinuity {
				manifest = strings.Replace(manifest, "#EXTINF:5,", "#EXT-X-DISCONTINUITY\n#EXT-X-PROGRAM-DATE-TIME:"+f.origin.Add(100*time.Millisecond).Format(time.RFC3339Nano)+"\n#EXTINF:5,", 1)
			}
			if f.thirdEpoch {
				manifest += "#EXT-X-DISCONTINUITY\n#EXT-X-PROGRAM-DATE-TIME:" + f.origin.Add(5100*time.Millisecond).Format(time.RFC3339Nano) + "\n#EXTINF:5,\nc?sig=private-signature\n"
			}
			return response(io.NopCloser(strings.NewReader(manifest))), nil
		case "/init":
			return response(io.NopCloser(strings.NewReader(""))), nil
		case "/a":
			return response(f.bodies[0]), nil
		case "/b":
			return response(f.bodies[1]), nil
		case "/c":
			return response(f.bodies[2]), nil
		default:
			return nil, errors.New("unexpected request")
		}
	})}
	t.Cleanup(func() {
		for _, body := range f.bodies {
			_ = body.Close()
		}
	})
	return f
}

func runProgrammeSignalFixture(t *testing.T, f *programmeSignalFixture) boundaryLaneResult {
	t.Helper()
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), config.ProgrammeBoundaryTimeout)
	defer cancel()
	lane := "prepared"
	if f.raw {
		lane = "transcode"
	}
	result := observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: lane, channelIndex: 0})
	if f.decoder.Started.Load() != f.decoder.Stopped.Load() {
		t.Fatalf("decoder lifecycle started=%d stopped=%d", f.decoder.Started.Load(), f.decoder.Stopped.Load())
	}
	return result
}

func TestProgrammeSignalsBoundAACPrerollToOneDeclaredFrame(t *testing.T) {
	for _, mode := range []string{"declared", "undeclared", "extra priming", "malformed", "declaration changed"} {
		t.Run(mode, func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.raw = true
			priming := DecodedAudioSignal{PTSUS: 1_128_667, Samples: 1024, ZeroCrossingRate: 0.053711, RMSDB: -37.044586}
			f.decoder.Signals = append([]playoutcertfixture.SignalPair[DecodedVideoSignal, DecodedAudioSignal]{{Audio: &priming}}, f.decoder.Signals...)
			first, next := []byte{0, 1, 2}, make([]byte, 38)
			for i := range next {
				next[i] = byte(i + 3)
			}
			for _, body := range f.bodies {
				_ = body.Close()
			}
			f.bodies = []*playoutcertfixture.PacedBody{playoutcertfixture.NewPacedBody(first, 25*time.Millisecond, false), playoutcertfixture.NewPacedBody(next, 25*time.Millisecond, true)}
			resolve := f.truth.Value.ResolveAsset
			f.truth.Value.ResolveAsset = func(ctx context.Context, asset ProgrammeAsset) (ProgrammeAssetEvidence, error) {
				proof, err := resolve(ctx, asset)
				proof.AACPreroll = mode != "undeclared"
				proof.Media = first
				if asset.Reference == "b" {
					proof.Media = next
					if mode == "declaration changed" {
						proof.AACPreroll = false
					}
				}
				return proof, err
			}
			want := "ok"
			switch mode {
			case "undeclared":
				want = "programme_audio_mismatch"
			case "extra priming":
				f.decoder.Signals[1].Audio.ZeroCrossingRate = priming.ZeroCrossingRate
				want = "programme_audio_mismatch"
			case "malformed":
				priming.Samples = 1025
				want = "invalid_audio_signal"
			case "declaration changed":
				want = "asset_clock_mismatch"
			}
			if got := runProgrammeSignalFixture(t, f); got.observation.class != want {
				t.Fatalf("outcome=%s want=%s", got.observation.class, want)
			}
		})
	}
}

func TestProgrammeSignalsQualifyExpectedAVAndOrdinarySignedRoute(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	got := runProgrammeSignalFixture(t, f)
	if got.observation.class != "ok" || got.evidence.Transitions != 1 || got.evidence.DecodedFrameDelta < 2 || got.evidence.DecodedAudioSamplesDelta < 2048 || got.evidence.ReadDelta <= 0 || got.evidence.BytesDelta <= 0 {
		t.Fatalf("qualification=%+v", got)
	}
	if f.truth.Calls.Load() != 1 || f.clocks.Load() != 2 || f.bodies[0].Closed.Load() != 1 || f.bodies[1].Closed.Load() != 1 {
		t.Fatalf("freeze=%d clocks=%d body closes=%d/%d", f.truth.Calls.Load(), f.clocks.Load(), f.bodies[0].Closed.Load(), f.bodies[1].Closed.Load())
	}
}

func TestProgrammeSignalsRejectMissingTruthBeforeNetwork(t *testing.T) {
	for _, mode := range []string{"missing", "source error", "missing clock", "gap", "ambiguous"} {
		t.Run(mode, func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			switch mode {
			case "missing":
				f.config.ProgrammeEvidence = nil
			case "source error":
				f.truth.Error = errors.New("private media path must not escape")
			case "missing clock":
				f.truth.Value.ResolveAsset = nil
			case "gap":
				f.truth.Value.Programmes[1].StartsAt = f.truth.Value.Programmes[1].StartsAt.Add(time.Second)
			case "ambiguous":
				p := &f.truth.Value.Programmes[1]
				p.Luma = f.truth.Value.Programmes[0].Luma
				p.ZeroCrossingRate = f.truth.Value.Programmes[0].ZeroCrossingRate
			}
			got := runProgrammeSignalFixture(t, f)
			if got.observation.class != "evidence_unavailable" || f.requests.Load() != 0 || f.decoder.Started.Load() != 0 {
				t.Fatalf("result=%+v requests=%d decoders=%d", got, f.requests.Load(), f.decoder.Started.Load())
			}
		})
	}
}

func TestProgrammeSignalsRejectWrongSignalsAndSourceReplacement(t *testing.T) {
	for _, mode := range []string{"video", "audio", "silence", "generation", "origin", "missing origin", "decoder failure", "priming declaration"} {
		t.Run(mode, func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			want := "programme_" + mode + "_mismatch"
			switch mode {
			case "video":
				f.decoder.Signals[2].Video.Luma = 16
			case "audio":
				f.decoder.Signals[2].Audio.ZeroCrossingRate = 0.018
			case "silence":
				f.decoder.Signals[2].Audio.Silence = true
				want = "programme_audio_mismatch"
			case "decoder failure":
				f.decoder.Failure = errors.New("private decoder stderr")
				want = "decode_failed"
			default:
				want = "asset_clock_mismatch"
				resolve := f.truth.Value.ResolveAsset
				f.truth.Value.ResolveAsset = func(ctx context.Context, a ProgrammeAsset) (ProgrammeAssetEvidence, error) {
					c := ProgrammeMediaClock{Origin: f.origin, Generation: "private-source"}
					if mode == "missing origin" {
						c.Origin = time.Time{}
					}
					if a.Reference == "b" {
						if mode == "origin" {
							c.Origin = c.Origin.Add(time.Second)
						}
						if mode == "generation" {
							c.Generation = "replacement"
						}
					}
					proof, err := resolve(ctx, a)
					proof.Clock = c
					if mode == "priming declaration" && a.Reference == "b" {
						proof.AACPreroll = true
					}
					return proof, err
				}
			}
			got := runProgrammeSignalFixture(t, f)
			if got.observation.class != want {
				t.Fatalf("result=%+v want=%s", got, want)
			}
		})
	}
}

func TestProgrammeSignalsRequireLateAudioVideoAndTransport(t *testing.T) {
	for _, mode := range []string{"healthy", "early burst", "audio stops", "video stops", "transport error", "cancel blocked body", "no transition", "no initial audio"} {
		t.Run(mode, func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.config.ProgrammeBoundaryTimeout = time.Second
			switch mode {
			case "no initial audio":
				f.decoder.Signals[0].Audio = nil
				f.decoder.Signals[1].Audio = nil
			case "early burst":
				f.bodies[1].Bytes = f.bodies[1].Bytes[:3]
			case "audio stops":
				for i := 5; i < len(f.decoder.Signals); i++ {
					f.decoder.Signals[i].Audio = nil
				}
			case "video stops":
				for i := 5; i < len(f.decoder.Signals); i++ {
					f.decoder.Signals[i].Video = nil
				}
			case "transport error":
				f.bodies[1].Bytes = f.bodies[1].Bytes[:3]
				f.bodies[1].Hold = false
				f.bodies[1].Terminal = errors.New("private transport error")
			case "no transition":
				f.bodies[1].Bytes = nil
			case "cancel blocked body":
				f.bodies[0].Bytes = nil
				f.bodies[0].Hold = true
			}
			got := runProgrammeSignalFixture(t, f)
			want := "programme_observation_timeout"
			if mode == "healthy" {
				want = "ok"
			}
			if mode == "transport error" {
				want = "decode_failed"
			}
			if got.observation.class != want {
				t.Fatalf("result=%+v want=%s", got, want)
			}
			if mode == "cancel blocked body" && f.bodies[0].Closed.Load() != 1 {
				t.Fatal("blocked input not closed")
			}
		})
	}
}

func TestProgrammeSignalsCannotQualifyBeforeScheduledTransition(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.config.ProgrammeBoundaryTimeout = 400 * time.Millisecond
	boundary := f.origin.Add(2 * time.Second)
	f.truth.Value.Programmes[0].EndsAt = boundary
	f.truth.Value.Programmes[1].StartsAt = boundary
	for i := 2; i < len(f.decoder.Signals); i++ {
		f.decoder.Signals[i].Video.PTSUS += 600_000
		f.decoder.Signals[i].Audio.PTSUS += 600_000
	}
	got := runProgrammeSignalFixture(t, f)
	if got.observation.class != "programme_observation_timeout" {
		t.Fatalf("future transition qualified before scheduled boundary: %+v", got)
	}
}

func TestProgrammeSignalsBufferedOutputCannotReplaceLateTransport(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		t.Run(fmt.Sprintf("buffered=%t", buffered), func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.config.ProgrammeBoundaryTimeout = time.Second
			for _, body := range f.bodies {
				body.Interval = 0
			}
			f.decoder.EmitInterval = 25 * time.Millisecond
			if buffered {
				// The first five emissions establish the A/V transition before
				// the second batch is read. Later callbacks stay fresh and both
				// read counters advance, but all reads precede the late interval.
				f.decoder.BatchSizes = []int{5, 35}
			}
			got := runProgrammeSignalFixture(t, f)
			want := "ok"
			if buffered {
				want = "programme_observation_timeout"
			}
			if got.observation.class != want {
				t.Fatalf("buffered=%t result=%+v want=%s", buffered, got, want)
			}
			for _, body := range f.bodies {
				if body.Closed.Load() != 1 {
					t.Fatal("input not closed after delayed decoder output")
				}
			}
		})
	}
}

func TestProgrammeSignalsFreezeOwnsExpectedSchedule(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	resolve := f.truth.Value.ResolveAsset
	f.truth.Value.ResolveAsset = func(ctx context.Context, a ProgrammeAsset) (ProgrammeAssetEvidence, error) {
		if a.Reference == "a" {
			f.truth.Value.Programmes[1].Luma = SignalRange{0, 25}
		}
		return resolve(ctx, a)
	}
	got := runProgrammeSignalFixture(t, f)
	if got.observation.class != "ok" || f.truth.Calls.Load() != 1 {
		t.Fatalf("frozen truth was replaced: %+v freeze calls=%d", got, f.truth.Calls.Load())
	}
}

func TestProgrammeBoundaryPhaseRequiresPrivateTruthForBothLanes(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.config.ProgrammeEvidence = nil
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	phase := programmeBoundarySoak(t.Context(), endpoint, config, []int{0}, []int{0})
	if phase.Attempts != 2 || phase.Failures != 2 || phase.HTTPClasses["evidence_unavailable"] != 2 || f.requests.Load() != 0 || f.decoder.Started.Load() != 0 {
		t.Fatalf("phase=%+v requests=%d decoder starts=%d", phase, f.requests.Load(), f.decoder.Started.Load())
	}
}

func TestProgrammeSignalsValidateEachDecoderEpoch(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.discontinuity = true
			validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}, {VideoStreams: 1, AudioStreams: 1}}}
			if missing {
				validator.Shapes[1].AudioStreams = 0
			}
			f.config.Validator = validator
			got := runProgrammeSignalFixture(t, f)
			want := "ok"
			if missing {
				want = "invalid_media"
			}
			if got.observation.class != want || validator.Calls() != 2 || f.decoder.Started.Load() != 2 || f.decoder.Stopped.Load() != 2 {
				t.Fatalf("result=%+v validator=%d decoder=%d/%d", got, validator.Calls(), f.decoder.Started.Load(), f.decoder.Stopped.Load())
			}
		})
	}
}

func TestProgrammeSignalsCloseFailureCannotQualify(t *testing.T) {
	for _, index := range []int{0, 1} {
		t.Run(fmt.Sprintf("asset=%d", index), func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.bodies[index].CloseError = errors.New("private close failure")
			got := runProgrammeSignalFixture(t, f)
			if got.observation.class != "close_failed" {
				t.Fatalf("result=%+v", got)
			}
			if f.bodies[index].Closed.Load() != 1 {
				t.Fatal("failed asset close was not owned exactly once")
			}
		})
	}
}

func TestProgrammeSignalsCancellationAfterValidationJoinsEpochs(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.discontinuity = true
	calls := make(chan int, 3)
	f.config.Validator = &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, CallStarted: calls}
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result := make(chan boundaryLaneResult, 1)
	go func() {
		result <- observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case call := <-calls:
			if call != want {
				t.Fatalf("validation=%d want=%d", call, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	cancel()
	select {
	case got := <-result:
		if got.observation.class == "ok" || f.decoder.Started.Load() != 2 || f.decoder.Stopped.Load() != 2 || f.bodies[0].Closed.Load() != 1 || f.bodies[1].Closed.Load() != 1 {
			t.Fatalf("result=%+v decoder=%d/%d closes=%d/%d", got, f.decoder.Started.Load(), f.decoder.Stopped.Load(), f.bodies[0].Closed.Load(), f.bodies[1].Closed.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not join decoder and input")
	}
}

func TestProgrammeSignalsQueuedEpochAfterLateWindowStillRequiresValidation(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.discontinuity = true
	f.thirdEpoch = true
	f.bodies[1].Bytes = f.bodies[1].Bytes[:5]
	f.bodies[1].Hold = false
	f.bodies = append(f.bodies, playoutcertfixture.NewPacedBody([]byte{10, 11, 12}, 25*time.Millisecond, true))
	gate := make(chan struct{})
	calls := make(chan int, 3)
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}, {VideoStreams: 1, AudioStreams: 1}, {VideoStreams: 1}}, WaitFor: gate, WaitForCall: 2, CallStarted: calls}
	f.config.Validator = validator
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result := make(chan boundaryLaneResult, 1)
	go func() {
		result <- observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case call := <-calls:
			if call != want {
				t.Fatalf("validation=%d want=%d", call, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// The observer is held inside second-epoch validation while its decoder
	// finishes and the next discontinuity queues. Release beyond both late clocks.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for f.decoder.Stopped.Load() < 2 || time.Now().Before(f.origin.Add(1700*time.Millisecond)) {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(gate)
	select {
	case got := <-result:
		if got.observation.class != "invalid_media" || validator.Calls() != 3 || f.decoder.Started.Load() != 3 || f.decoder.Stopped.Load() != 3 {
			t.Fatalf("result=%+v validations=%d decoder=%d/%d", got, validator.Calls(), f.decoder.Started.Load(), f.decoder.Stopped.Load())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestProgrammeSignalsBlockedRefreshCancelsBeforeJoin(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.blockRefresh = true
	f.config.Validator = &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1}}, WaitFor: f.blocked}
	got := runProgrammeSignalFixture(t, f)
	if got.observation.class != "invalid_media" {
		t.Fatalf("result=%+v", got)
	}
	select {
	case <-f.refreshCanceled:
	case <-time.After(time.Second):
		t.Fatal("blocked refresh was not cancelled")
	}
	if f.decoder.Started.Load() != 1 || f.decoder.Stopped.Load() != 1 || f.bodies[0].Closed.Load() != 1 {
		t.Fatal("reader/decoder did not join")
	}
}

func TestProgrammeSignalsCleanDecoderEOFWithoutDiscontinuityRejects(t *testing.T) {
	for _, call := range []int{1, 2} {
		t.Run(fmt.Sprint(call), func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.discontinuity = call == 2
			f.decoder.ReturnAfter = 2
			f.decoder.ReturnOnCall = call
			got := runProgrammeSignalFixture(t, f)
			if got.observation.class != "unexpected_media_eof" || got.evidence.Transitions != 0 || f.decoder.Started.Load() != int32(call) || f.decoder.Stopped.Load() != int32(call) {
				t.Fatalf("result=%+v lifecycle=%d/%d", got, f.decoder.Started.Load(), f.decoder.Stopped.Load())
			}
		})
	}
}

func TestProgrammeSignalsQueuedEpochAfterLateWindowCanContinue(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	f.discontinuity = true
	f.thirdEpoch = true
	f.truth.Value.Programmes[1].EndsAt = f.origin.Add(1900 * time.Millisecond)
	p := f.truth.Value.Programmes[0]
	p.StartsAt = f.truth.Value.Programmes[1].EndsAt
	p.EndsAt = f.origin.Add(10 * time.Second)
	f.truth.Value.Programmes = append(f.truth.Value.Programmes, p)
	f.bodies[1].Bytes = f.bodies[1].Bytes[:5]
	f.bodies[1].Hold = false
	data := make([]byte, 24)
	for i := range data {
		data[i] = byte(len(f.decoder.Signals))
		pts := int64(2_050_000 + i*25_000)
		v := DecodedVideoSignal{PTSUS: pts, Luma: 16}
		a := DecodedAudioSignal{PTSUS: pts, Samples: 1024, ZeroCrossingRate: 0.018, RMSDB: -24}
		f.decoder.Signals = append(f.decoder.Signals, playoutcertfixture.SignalPair[DecodedVideoSignal, DecodedAudioSignal]{Video: &v, Audio: &a})
	}
	f.bodies = append(f.bodies, playoutcertfixture.NewPacedBody(data, 25*time.Millisecond, true))
	gate := make(chan struct{})
	calls := make(chan int, 3)
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, WaitFor: gate, WaitForCall: 2, CallStarted: calls}
	f.config.Validator = validator
	f.config.ProgrammeBoundaryTimeout = 3 * time.Second
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result := make(chan boundaryLaneResult, 1)
	go func() {
		result <- observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	}()
	for want := 1; want <= 2; want++ {
		select {
		case call := <-calls:
			if call != want {
				t.Fatalf("validation=%d want=%d", call, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for f.decoder.Stopped.Load() < 2 || time.Now().Before(f.origin.Add(2200*time.Millisecond)) {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(gate)
	select {
	case got := <-result:
		if got.observation.class != "ok" || got.evidence.Transitions != 2 || got.evidence.DecodedFrameDelta < 2 || got.evidence.DecodedAudioSamplesDelta < 2048 || got.evidence.ReadDelta <= 0 || got.evidence.BytesDelta <= 0 || validator.Calls() != 3 || f.decoder.Started.Load() != 3 || f.decoder.Stopped.Load() != 3 {
			t.Fatalf("result=%+v validations=%d decoder=%d/%d", got, validator.Calls(), f.decoder.Started.Load(), f.decoder.Stopped.Load())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestProgrammeSignalsCancellationAfterInitialValidation(t *testing.T) {
	f := newProgrammeSignalFixture(t)
	calls := make(chan int, 1)
	f.config.Validator = &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, CallStarted: calls}
	config := f.config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result := make(chan boundaryLaneResult, 1)
	go func() {
		result <- observeProgrammeSignals(ctx, endpoint, config, boundaryLane{name: "prepared", channelIndex: 0})
	}()
	select {
	case call := <-calls:
		if call != 1 {
			t.Fatalf("validation=%d", call)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case got := <-result:
		if got.observation.class == "ok" || f.decoder.Started.Load() != 1 || f.decoder.Stopped.Load() != 1 || f.bodies[0].Closed.Load() != 1 {
			t.Fatalf("result=%+v decoder=%d/%d first body closed=%d", got, f.decoder.Started.Load(), f.decoder.Stopped.Load(), f.bodies[0].Closed.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation after initial validation did not join")
	}
}

func TestProgrammeSignalsRejectUnboundResponseBytes(t *testing.T) {
	for _, mode := range []string{"changed media", "short media", "extra media", "changed init", "retired source"} {
		t.Run(mode, func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			// A replacement byte has the exact same decoded signal, so signal checks
			// alone cannot detect the substitution.
			f.decoder.Signals = append(f.decoder.Signals, f.decoder.Signals[2])
			resolve := f.truth.Value.ResolveAsset
			f.truth.Value.ResolveAsset = func(ctx context.Context, asset ProgrammeAsset) (ProgrammeAssetEvidence, error) {
				proof, err := resolve(ctx, asset)
				if asset.Reference == "a" && mode == "changed init" {
					proof.Init = []byte{0}
				}
				if asset.Reference == "b" {
					switch mode {
					case "changed media":
						f.bodies[1].Bytes[0] = 40
					case "short media":
						f.bodies[1].Bytes = f.bodies[1].Bytes[:3]
						f.bodies[1].Hold = false
					case "extra media":
						proof.Media = proof.Media[:3]
					case "retired source":
						proof.Validate = func() error { return errors.New("private source retired") }
					}
				}
				return proof, err
			}
			got := runProgrammeSignalFixture(t, f)
			if got.observation.class != "asset_clock_mismatch" {
				t.Fatalf("unbound response=%+v", got)
			}
			for _, body := range f.bodies {
				if body.Closed.Load() > 1 {
					t.Fatal("response closed more than once")
				}
			}
		})
	}
}

func TestProgrammeSignalsRecheckSourceAfterBufferedCallbacks(t *testing.T) {
	for _, retire := range []bool{false, true} {
		t.Run(fmt.Sprintf("retire=%t", retire), func(t *testing.T) {
			f := newProgrammeSignalFixture(t)
			f.config.ProgrammeBoundaryTimeout = time.Second
			for _, body := range f.bodies {
				body.Interval = 0
			}
			// The second read occurs after the late-read threshold; its callbacks
			// continue after source retirement without another network read.
			f.decoder.BatchSizes = []int{10, 30}
			f.decoder.EmitInterval = 25 * time.Millisecond
			var retired atomic.Bool
			f.decoder.BeforeEmit = func(index int) {
				if retire && index >= 18 {
					retired.Store(true)
				}
			}
			resolve := f.truth.Value.ResolveAsset
			f.truth.Value.ResolveAsset = func(ctx context.Context, asset ProgrammeAsset) (ProgrammeAssetEvidence, error) {
				proof, err := resolve(ctx, asset)
				proof.Validate = func() error {
					if retired.Load() {
						return errors.New("retired after last read")
					}
					return nil
				}
				return proof, err
			}
			got := runProgrammeSignalFixture(t, f)
			want := "ok"
			if retire {
				want = "asset_clock_mismatch"
			}
			if got.observation.class != want {
				t.Fatalf("result=%+v want=%s", got, want)
			}
		})
	}
}
