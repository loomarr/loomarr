package playoutcert

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

type programmeSignalEvent struct {
	video   *DecodedVideoSignal
	audio   *DecodedAudioSignal
	clock   ProgrammeMediaClock
	preroll bool
}

// programmeSignalEpoch owns one decoder lifecycle. Its clock is established
// before admitting bytes and independently checked for every asset in that epoch.
type programmeSignalEpoch struct {
	input      *observedReadCloser
	cancel     context.CancelFunc
	done       chan struct{}
	mu         sync.Mutex
	clock      ProgrammeMediaClock
	err        error
	clockErr   bool
	validate   func() error
	aacPreroll bool
}

func startProgrammeSignalEpoch(ctx context.Context, reader *preparedHLSReader, evidence ProgrammeEvidence, decoder SignalDecoder, capture int, events chan<- programmeSignalEvent) *programmeSignalEpoch {
	ctx, cancel := context.WithCancel(ctx)
	epoch := &programmeSignalEpoch{cancel: cancel, done: make(chan struct{})}
	epoch.input = &observedReadCloser{source: reader.epoch(), limit: capture, notify: make(chan struct{}, 1)}
	reader.onAssetMismatch = func() {
		epoch.mu.Lock()
		epoch.clockErr = true
		epoch.mu.Unlock()
	}
	reader.onSegment = func(segment preparedHLSSegment) (ProgrammeAssetEvidence, error) {
		reference, _, _ := strings.Cut(segment.uri, "?")
		initReference, _, _ := strings.Cut(segment.init, "?")
		proof, err := evidence.ResolveAsset(ctx, ProgrammeAsset{Reference: reference, InitReference: initReference, Epoch: segment.boundary, StartedAt: segment.startedAt, Duration: segment.duration})
		clock := proof.Clock
		epoch.mu.Lock()
		defer epoch.mu.Unlock()
		if err != nil || clock.Origin.IsZero() || clock.Generation == "" || len(clock.Generation) > 256 ||
			len(proof.Media) == 0 || len(proof.Media) > 32<<20 || len(proof.Init) > 32<<20 ||
			(initReference == "" && len(proof.Init) != 0) ||
			(initReference != "" && proof.AACPreroll) ||
			(!epoch.clock.Origin.IsZero() && (!epoch.clock.Origin.Equal(clock.Origin) || epoch.clock.Generation != clock.Generation || epoch.aacPreroll != proof.AACPreroll)) {
			epoch.clockErr = true
			return ProgrammeAssetEvidence{}, errors.New("asset_clock_mismatch")
		}
		epoch.clock = clock
		epoch.aacPreroll = proof.AACPreroll
		epoch.validate = proof.Validate
		proof.Media, proof.Init = slices.Clone(proof.Media), slices.Clone(proof.Init)
		return proof, nil
	}
	send := func(event programmeSignalEvent) {
		epoch.mu.Lock()
		event.clock = epoch.clock
		epoch.mu.Unlock()
		select {
		case events <- event:
		case <-ctx.Done():
		}
	}
	go func() {
		firstAudio := true
		err := decoder.DecodeSignals(ctx, epoch.input, func(v DecodedVideoSignal) { send(programmeSignalEvent{video: &v}) }, func(a DecodedAudioSignal) {
			epoch.mu.Lock()
			preroll := firstAudio && epoch.aacPreroll
			epoch.mu.Unlock()
			firstAudio = false
			send(programmeSignalEvent{audio: &a, preroll: preroll})
		})
		epoch.mu.Lock()
		epoch.err = err
		epoch.mu.Unlock()
		close(epoch.done)
	}()
	return epoch
}

func (e *programmeSignalEpoch) stop() { e.cancel(); _ = e.input.Close(); <-e.done }

func (e *programmeSignalEpoch) transport() (reads, bytes int, last time.Time) {
	e.input.mu.Lock()
	defer e.input.mu.Unlock()
	return e.input.reads, e.input.bytes, e.input.lastRead
}

func (e *programmeSignalEpoch) capture() []byte {
	e.input.mu.Lock()
	defer e.input.mu.Unlock()
	return append([]byte(nil), e.input.capture...)
}

func observeProgrammeSignals(ctx context.Context, endpoint *endpoint, config Config, lane boundaryLane) (result boundaryLaneResult) {
	result.evidence.Lane = lane.name
	fail := func(class string) boundaryLaneResult {
		result.observation.class = class
		result.evidence.Outcome = class
		return result
	}
	if lane.channelIndex < 0 {
		return fail("cohort_missing")
	}
	armedAt := time.Now()
	until := armedAt.Add(config.ProgrammeBoundaryTimeout)
	evidence, err := freezeProgrammeEvidence(config.ProgrammeEvidence, config.Channels[lane.channelIndex].ID, armedAt, until)
	if err != nil {
		return fail("evidence_unavailable")
	}
	decoder := config.SignalDecoder
	if decoder == nil {
		decoder = FFmpegSignalDecoder{}
	}
	signed, _, class := endpoint.mint(ctx, config.Channels[lane.channelIndex].ID)
	if class != "ok" {
		return fail(class)
	}
	// Both cohorts exercise the ordinary signed viewer route. A prepared cohort
	// additionally requires its advertised fMP4 initialization map.
	reader := newLiveHLSReader(ctx, endpoint, signed)
	reader.requireMap = lane.name == "prepared"
	events := make(chan programmeSignalEvent, 256)
	epoch := startProgrammeSignalEpoch(ctx, reader, evidence, decoder, config.RawCaptureBytes, events)
	defer func() {
		closeErr := reader.Close()
		epoch.stop()
		if closeErr != nil && (result.observation.class == "ok" || result.observation.class == "decode_failed") {
			result.observation.class, result.evidence.Outcome = "close_failed", "close_failed"
		}
	}()
	check := newProgrammeSignalCheck(evidence.Programmes, armedAt)
	var transitionAt, lastVideoArrival, lastAudioArrival time.Time
	var boundaryFrames, boundarySamples int64
	var completedReads, completedBytes, boundaryReads, boundaryBytes int
	validated := false
	var epochVideo, epochAudio int64
	consume := func(event programmeSignalEvent) string {
		beforeVideo, beforeAudio := check.videoFrames, check.audioSamples
		var err error
		if event.video != nil {
			err = check.videoSignal(event.clock, *event.video)
		} else if event.audio != nil {
			err = check.checkAudioSignal(event.clock, *event.audio, event.preroll)
		}
		if err != nil {
			return err.Error()
		}
		now := time.Now()
		if check.videoFrames > beforeVideo {
			epochVideo += check.videoFrames - beforeVideo
			lastVideoArrival = now
		}
		if check.audioSamples > beforeAudio {
			epochAudio += check.audioSamples - beforeAudio
			lastAudioArrival = now
		}
		if !validated && epochVideo > 0 && epochAudio > 0 {
			shape, err := config.Validator.Validate(ctx, epoch.capture())
			if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 {
				return "invalid_media"
			}
			result.evidence.Media, result.observation.media = shape, shape
			validated = true
		}
		if check.transition >= 0 && transitionAt.IsZero() {
			transitionAt = now
			boundaryFrames, boundarySamples = check.videoFrames, check.audioSamples
			reads, bytes, _ := epoch.transport()
			boundaryReads, boundaryBytes = completedReads+reads, completedBytes+bytes
		}
		return ""
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return fail("programme_observation_timeout")
		case event := <-events:
			if class := consume(event); class != "" {
				return fail(class)
			}
		case <-epoch.done:
			epoch.mu.Lock()
			decodeErr, clockErr := epoch.err, epoch.clockErr
			epoch.mu.Unlock()
			if clockErr {
				return fail("asset_clock_mismatch")
			}
			if decodeErr != nil {
				return fail("decode_failed")
			}
			// Wait joined the producer callbacks. Drain their queued observations
			// before assigning a clock to the next decoder epoch.
			for len(events) > 0 {
				if class := consume(<-events); class != "" {
					return fail(class)
				}
			}
			if !validated {
				return fail("invalid_media")
			}
			select {
			case <-reader.transition:
			default:
				return fail("unexpected_media_eof")
			}
			reads, bytes, _ := epoch.transport()
			completedReads += reads
			completedBytes += bytes
			epoch.stop()
			check.beginEpoch()
			epoch = startProgrammeSignalEpoch(ctx, reader, evidence, decoder, config.RawCaptureBytes, events)
			validated = false
			epochVideo, epochAudio = 0, 0
		case <-tick.C:
		}
		if transitionAt.IsZero() || !validated || epochVideo < 2 || epochAudio < 2048 || time.Since(transitionAt) < config.ProgrammeBoundaryLateObservation {
			continue
		}
		lateWall := transitionAt.Add(3 * config.ProgrammeBoundaryLateObservation / 4)
		lateMedia := check.programmes[check.transition].StartsAt.Add(3 * config.ProgrammeBoundaryLateObservation / 4)
		// Correctly signed future bytes still cannot prove a transition that has
		// not aired. Require both the arrival interval and the scheduled interval.
		if time.Now().Before(lateMedia) || len(events) > 0 {
			continue
		}
		reads, bytes, lastRead := epoch.transport()
		if lastVideoArrival.Before(lateWall) || lastAudioArrival.Before(lateWall) || lastRead.Before(lateWall) ||
			check.lastMatchedVideo.Before(lateMedia) || check.lastMatchedAudio.Before(lateMedia) ||
			completedReads+reads <= boundaryReads || completedBytes+bytes <= boundaryBytes {
			continue
		}
		epoch.input.mu.Lock()
		readErr := epoch.input.readErr
		epoch.input.mu.Unlock()
		if readErr != nil {
			continue
		}
		// A terminal decoder cannot leave a retained success merely because its
		// final burst happened to satisfy the counters.
		select {
		case <-epoch.done:
			continue
		default:
		}
		if ctx.Err() != nil {
			return fail("programme_observation_timeout")
		}
		epoch.mu.Lock()
		validate, clockErr := epoch.validate, epoch.clockErr
		epoch.mu.Unlock()
		if clockErr || (validate != nil && validate() != nil) {
			return fail("asset_clock_mismatch")
		}
		transitions := 0
		for index := check.initial + 1; index < len(check.programmes); index++ {
			if check.video[index] < 2 || check.audio[index] < 2048 {
				break
			}
			transitions++
		}
		result.evidence.Outcome = "ok"
		result.evidence.Transitions = transitions
		result.evidence.ObservationMS = float64(time.Since(transitionAt).Microseconds()) / 1000
		result.evidence.DecodedFrameDelta = check.videoFrames - boundaryFrames
		result.evidence.DecodedAudioSamplesDelta = check.audioSamples - boundarySamples
		result.evidence.ReadDelta = completedReads + reads - boundaryReads
		result.evidence.BytesDelta = completedBytes + bytes - boundaryBytes
		result.observation.class = "ok"
		result.observation.duration = time.Since(armedAt)
		return result
	}
}
