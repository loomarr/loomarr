package playoutcert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type observation struct {
	duration  time.Duration
	firstByte time.Duration
	hit       bool
	class     string
	media     MediaShape
}

func Run(ctx context.Context, config Config) (Report, error) {
	config = config.normalized()
	if err := config.Validate(); err != nil {
		return Report{}, err
	}
	endpoint, err := newEndpoint(config)
	if err != nil {
		return Report{}, err
	}
	report := Report{SchemaVersion: SchemaVersion, StartedAt: config.Now(), Phases: []Phase{}, Resources: []ResourceSample{}, Failures: []string{}}
	report.FaultProfiles, report.Failures = faultQualifications(config.FaultProfiles, config.FaultController)
	target, _, err := endpoint.target(ctx, len(config.Channels), manifestDigest(config.Channels))
	if err != nil {
		return Report{}, fmt.Errorf("target preflight failed")
	}
	report.Target = target
	baseline, err := endpoint.sample(ctx, "baseline")
	if err != nil {
		return Report{}, fmt.Errorf("resource preflight failed")
	}
	report.Resources = append(report.Resources, baseline)
	samper := newPhaseSampler(ctx, endpoint, config.CleanupPoll)
	defer samper.close()
	// Mint, lookup and surf exercise the whole configured catalog. Explicit
	// prepared roles select the readiness cohort; absent roles mean all channels.
	configuredIndexes := make([]int, len(config.Channels))
	for index := range config.Channels {
		configuredIndexes[index] = index
	}
	preparedIndexes := preparedChannelIndexes(config.Channels)
	transcodeIndexes := strictTranscodeChannelIndexes(config.Channels)

	signed := make([]*url.URL, len(config.Channels))
	samper.begin("mint")
	mintObs := parallel(config.Concurrency, len(config.Channels), func(index int) observation {
		urlValue, elapsed, class := endpoint.mint(ctx, config.Channels[index].ID)
		if class == "ok" {
			signed[index] = urlValue
		}
		return observation{duration: elapsed, class: class}
	})
	appendSampledPhase(&report, samper, phaseFrom("mint", mintObs))
	samper.begin("configured")
	configuredObs := parallel(config.Concurrency, len(configuredIndexes), func(iteration int) observation {
		index := configuredIndexes[iteration]
		if signed[index] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[index])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	configured := phaseFrom("configured", configuredObs)
	// A 204 prepared miss is expected only for an explicitly cold transcode
	// channel. Preserve its class while counting the catalog lookup as complete.
	for index, obs := range configuredObs {
		if obs.class == "prepared_miss" && containsIndex(transcodeIndexes, index) {
			configured.Failures--
			configured.Successes++
		}
	}
	configured.PreparedHits = 0
	coldIndexes := []int{}
	for _, index := range preparedIndexes {
		if configuredObs[index].hit {
			configured.PreparedHits++
		}
	}
	for _, index := range transcodeIndexes {
		if configuredObs[index].class == "prepared_miss" {
			coldIndexes = append(coldIndexes, index)
		}
	}
	appendSampledPhase(&report, samper, configured)

	surfCount := len(configuredIndexes) * config.SurfRounds
	samper.begin("surf")
	surfObs := parallel(config.Concurrency, surfCount, func(iteration int) observation {
		index := configuredIndexes[iteration%len(configuredIndexes)]
		if signed[index] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[index])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	surf := phaseFrom("surf", surfObs)
	for iteration, obs := range surfObs {
		index := configuredIndexes[iteration%len(configuredIndexes)]
		if obs.class == "prepared_miss" && containsIndex(transcodeIndexes, index) {
			surf.Failures--
			surf.Successes++
		}
	}
	appendSampledPhase(&report, samper, surf)

	samper.begin("programme_boundary")
	appendSampledPhase(&report, samper, programmeBoundarySoak(ctx, endpoint, config, preparedIndexes, coldIndexes))

	samper.begin("prepared_fan_in")
	preparedFanObs := parallel(config.FanInViewers, config.FanInViewers, func(int) observation {
		if len(preparedIndexes) == 0 || signed[preparedIndexes[0]] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[preparedIndexes[0]])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	appendSampledPhase(&report, samper, phaseFrom("prepared_fan_in", preparedFanObs))
	fanIndexes := []int{}
	if len(transcodeIndexes) > 0 {
		fanIndexes = make([]int, config.FanInViewers)
		for index := range fanIndexes {
			fanIndexes[index] = transcodeIndexes[0]
		}
	}
	samper.begin("fan_in")
	fanObs, fanSample := rawBurst(ctx, endpoint, config, fanIndexes)
	if fanSample.SessionsActive > baseline.SessionsActive+1 || fanSample.TranscodeCost > baseline.TranscodeCost+1 {
		fanObs = append(fanObs, observation{class: "fanout_split"})
	}
	appendSampledPhase(&report, samper, phaseFrom("fan_in", fanObs))
	recordBurstSample(&report, "fan_in", fanSample)
	samper.begin("prepared_raw")
	preparedRaw, preparedRawSample := rawBurst(ctx, endpoint, config, selectBurstIndexes(preparedIndexes, target.Capacity))
	preparedRawPhase := phaseFrom("prepared_raw", preparedRaw)
	appendSampledPhase(&report, samper, preparedRawPhase)
	recordBurstSample(&report, "prepared_raw", preparedRawSample)

	if len(transcodeIndexes) < target.Capacity+1 {
		samper.begin("raw_capacity")
		appendSampledPhase(&report, samper, phaseFrom("raw_capacity", []observation{{class: "transcode_cohort_insufficient"}}))
		samper.begin("overload")
		appendSampledPhase(&report, samper, phaseFrom("overload", []observation{{class: "transcode_cohort_insufficient"}}))
	} else {
		samper.begin("raw_capacity")
		held := startHeldBurst(ctx, endpoint, config, transcodeIndexes[:target.Capacity])
		rawPhase := phaseFrom("raw_capacity", held.results)
		appendSampledPhase(&report, samper, rawPhase)
		recordBurstSample(&report, "raw_capacity", held.sample)

		samper.begin("overload")
		overload, overloadSample := rawBurst(ctx, endpoint, config, []int{transcodeIndexes[target.Capacity]})
		// The held readers begin their bounded observation only after the excess
		// admission decision, so bytes buffered before that event cannot by
		// themselves establish continued production.
		held.verify(ctx)
		held.release()
		overloadPhase := phaseFrom("overload", overload)
		overloadPhase.HeldContinuity = append([]HeldContinuityObservation(nil), held.continuity...)
		// A capacity probe has exactly one acceptable result: its one excess
		// request is rejected. Keep HTTP-class evidence intact, but account that
		// expected rejection as a successful assertion rather than a failed
		// media request.
		if overloadPhase.Attempts != 1 || overloadPhase.HTTPClasses["http_503"] != 1 || overloadPhase.Failures != 1 {
			overloadPhase.HTTPClasses["admission_outcome_missing"]++
		} else {
			overloadPhase.Successes++
			overloadPhase.Failures--
		}
		if held.failed() {
			overloadPhase.HTTPClasses["held_viewer_interrupted"]++
		}
		if overloadPhase.HTTPClasses["admission_outcome_missing"] > 0 || overloadPhase.HTTPClasses["held_viewer_interrupted"] > 0 {
			overloadPhase.Successes = 0
			overloadPhase.Failures = overloadPhase.Attempts
		}
		appendSampledPhase(&report, samper, overloadPhase)
		recordBurstSample(&report, "overload", overloadSample)
	}
	samper.begin("capacity_recovery")
	recoveryObs, recoverySample := waitForConvergence(ctx, endpoint, config, baseline, min(config.CleanupTimeout, config.WarmGrace+10*time.Second))
	appendSampledPhase(&report, samper, phaseFrom("capacity_recovery", []observation{recoveryObs}))
	recordConvergenceSample(&report, "capacity_recovery", recoverySample)
	if len(transcodeIndexes) > 0 {
		// The overload probe intentionally leaves warm parents behind.  Lifecycle
		// evidence starts only from the recorded baseline, otherwise its first raw
		// request can correctly reuse an overload parent and look like a failed start.
		lifecycleBaseline, _ := waitForConvergence(ctx, endpoint, config, baseline, min(config.CleanupTimeout, config.WarmGrace+10*time.Second))
		report.Phases = append(report.Phases, lifecycleDrill(ctx, endpoint, config, transcodeIndexes[0], baseline, lifecycleBaseline.class, samper)...)
	} else {
		for _, name := range []string{"cancellation", "warm_reuse", "grace_expiry"} {
			samper.begin(name)
			appendSampledPhase(&report, samper, phaseFrom(name, []observation{{class: "cohort_missing"}}))
		}
	}
	samper.begin("cleanup")
	cleanupObs, finalSample := waitForConvergence(ctx, endpoint, config, baseline, config.CleanupTimeout)
	appendSampledPhase(&report, samper, phaseFrom("cleanup", []observation{cleanupObs}))
	if finalSample.Capacity == 0 {
		sample, sampleErr := endpoint.sample(ctx, "final")
		if sampleErr == nil {
			finalSample = sample
		}
	}
	if finalSample.Capacity == 0 {
		report.Failures = append(report.Failures, "final_resource_sample_failed")
	} else {
		report.Resources = append(report.Resources, finalSample)
	}
	for _, phase := range report.Phases {
		if phase.Resources.SampleFailures > 0 || phase.Resources.Samples == 0 {
			report.Failures = append(report.Failures, phase.Name+"_resource_sample_failed")
		}
	}

	for _, phase := range report.Phases {
		if phase.Failures > 0 {
			report.Failures = append(report.Failures, phase.Name+"_failed")
		}
	}
	if configured.PreparedHits != len(preparedIndexes) {
		report.Failures = append(report.Failures, "prepared_coverage_incomplete")
	}
	if time.Duration(configured.P95MS*float64(time.Millisecond)) > config.PreparedP95 {
		report.Failures = append(report.Failures, "prepared_p95_exceeded")
	}
	if time.Duration(preparedRawPhase.P95MS*float64(time.Millisecond)) > config.PreparedRawP95 {
		report.Failures = append(report.Failures, "prepared_raw_p95_exceeded")
	}
	for _, sample := range report.Resources {
		if sample.Capacity > 0 && sample.TranscodeCost > sample.Capacity {
			report.Failures = append(report.Failures, "capacity_oversubscribed")
			break
		}
	}
	if len(report.Resources) >= 2 {
		final := report.Resources[len(report.Resources)-1]
		if final.SessionsActive > baseline.SessionsActive || final.FFmpegRunning > baseline.FFmpegRunning || final.OpenFDs > baseline.OpenFDs+4 || final.Goroutines > baseline.Goroutines+8 {
			report.Failures = append(report.Failures, "cleanup_residual")
		}
	}
	report.CompletedAt = config.Now()
	report.Certified = config.Certify && len(report.Failures) == 0
	return report, nil
}

func containsIndex(indexes []int, want int) bool {
	for _, index := range indexes {
		if index == want {
			return true
		}
	}
	return false
}

func appendSampledPhase(report *Report, sampler *phaseSampler, phase Phase) {
	phase.Resources = sampler.end(phase.Name)
	if phase.Resources.SampleFailures > 0 || phase.Resources.Samples == 0 {
		report.Failures = append(report.Failures, phase.Name+"_resource_sample_failed")
	}
	report.Phases = append(report.Phases, phase)
}

type rawConnection struct {
	body      io.ReadCloser
	startedAt time.Time
	firstByte time.Duration
	first     [1]byte
}

func openRaw(ctx context.Context, endpoint *endpoint, config Config, channelIndex int) (*rawConnection, string) {
	return openRawFor(ctx, endpoint, config, channelIndex, endpoint.timeout)
}

func openRawFor(ctx context.Context, endpoint *endpoint, config Config, channelIndex int, timeout time.Duration) (*rawConnection, string) {
	started := time.Now()
	path := "/v1/playout/stream/" + url.PathEscape(config.Channels[channelIndex].ID) + "?token=" + url.QueryEscape(endpoint.device)
	resp, err := endpoint.requestFor(ctx, http.MethodGet, path, nil, false, timeout)
	if err != nil {
		return nil, "request_failed"
	}
	if resp.StatusCode != http.StatusOK {
		class := httpClass(resp.StatusCode)
		_ = resp.Body.Close()
		return nil, class
	}
	var first [1]byte
	if _, err := io.ReadFull(resp.Body, first[:]); err != nil {
		_ = resp.Body.Close()
		return nil, "body_failed"
	}
	return &rawConnection{body: resp.Body, startedAt: started, firstByte: time.Since(started), first: first}, "ok"
}

type boundaryLane struct {
	name         string
	channelIndex int
}

type boundaryLaneResult struct {
	observation observation
	evidence    ProgrammeBoundaryObservation
}

func programmeBoundarySoak(ctx context.Context, endpoint *endpoint, config Config, preparedIndexes, coldIndexes []int) Phase {
	lanes := []boundaryLane{{name: "prepared", channelIndex: -1}, {name: "transcode", channelIndex: -1}}
	if len(preparedIndexes) > 0 {
		lanes[0].channelIndex = preparedIndexes[0]
	}
	if len(coldIndexes) > 0 {
		lanes[1].channelIndex = coldIndexes[0]
	}
	results := make([]boundaryLaneResult, len(lanes))
	phaseCtx, cancel := context.WithTimeout(ctx, config.ProgrammeBoundaryTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for index, lane := range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = observeProgrammeBoundary(phaseCtx, endpoint, config, lane)
		}()
	}
	wg.Wait()
	observations := make([]observation, len(results))
	evidence := make([]ProgrammeBoundaryObservation, len(results))
	for index, result := range results {
		observations[index] = result.observation
		evidence[index] = result.evidence
	}
	phase := phaseFrom("programme_boundary", observations)
	phase.ProgrammeBoundaries = evidence
	return phase
}

func observeProgrammeBoundary(ctx context.Context, endpoint *endpoint, config Config, lane boundaryLane) (result boundaryLaneResult) {
	result = boundaryLaneResult{evidence: ProgrammeBoundaryObservation{Lane: lane.name}}
	fail := func(class string) boundaryLaneResult {
		result.observation.class = class
		result.evidence.Outcome = class
		return result
	}
	if lane.channelIndex < 0 {
		return fail("cohort_missing")
	}
	if config.ProgrammeBoundaryWitness == nil {
		return fail("evidence_unavailable")
	}
	subscription, err := config.ProgrammeBoundaryWitness.Subscribe(config.Channels[lane.channelIndex].ID)
	if err != nil || subscription == nil {
		return fail("witness_failed")
	}
	defer subscription.Close()
	connection, class := openRawFor(ctx, endpoint, config, lane.channelIndex, 0)
	if class != "ok" {
		return fail(class)
	}
	observer, shape, _, class := startValidatedObserver(ctx, config, connection)
	if class != "ok" {
		return fail(class)
	}
	defer func() {
		if closeErr := observer.close(); closeErr != nil && result.observation.class == "ok" {
			result.observation.class = "close_failed"
			result.evidence.Outcome = "close_failed"
		}
	}()
	result.evidence.Media = shape
	result.observation.media = shape
	if err := subscription.WaitInitial(ctx); err != nil {
		return fail(boundaryWaitClass("initial_witness", err))
	}
	if err := subscription.WaitTransition(ctx); err != nil {
		return fail(boundaryWaitClass("transition", err))
	}
	transitionAt := time.Now()
	atBoundary := observer.snapshot()
	// A post-transition burst is not continuity. The same admitted reader and
	// decoder must still advance during the late part of the observation.
	late := transitionAt.Add(3 * config.ProgrammeBoundaryLateObservation / 4)
	timer := time.NewTimer(config.ProgrammeBoundaryLateObservation)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fail("late_observation_timeout")
	case <-timer.C:
	}
	after := observer.snapshot()
	if after.decoderDone || after.decoderErr != nil || after.readErr != nil {
		return fail("post_boundary_decode_failed")
	}
	if !postBoundaryProgressed(atBoundary, after, late) {
		return fail("post_boundary_stalled")
	}
	result.evidence.Outcome = "ok"
	result.evidence.Transitions = 1
	result.evidence.ObservationMS = float64(time.Since(transitionAt).Microseconds()) / 1000
	result.evidence.DecodedFrameDelta = after.frames - atBoundary.frames
	result.evidence.ReadDelta = after.reads - atBoundary.reads
	result.evidence.BytesDelta = after.bytes - atBoundary.bytes
	result.observation.class = "ok"
	result.observation.duration = time.Since(connection.startedAt)
	result.observation.firstByte = connection.firstByte
	return result
}

func postBoundaryProgressed(atBoundary, after decoderSnapshot, late time.Time) bool {
	return after.frames > atBoundary.frames && after.reads > atBoundary.reads && after.bytes > atBoundary.bytes && !after.lastRead.Before(late) && !after.lastFrame.Before(late)
}

func boundaryWaitClass(stage string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return stage + "_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return stage + "_cancelled"
	}
	return stage + "_failed"
}

func startValidatedObserver(ctx context.Context, config Config, connection *rawConnection) (*decoderObserver, MediaShape, time.Duration, string) {
	input := &joinedReadCloser{
		Reader: io.MultiReader(bytes.NewReader(connection.first[:]), connection.body),
		Closer: connection.body,
	}
	observer := startDecoderObserver(ctx, config.Decoder, input, config.RawCaptureBytes)
	deadline := connection.startedAt.Add(config.RequestTimeout)
	initialCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	snapshot, err := observer.wait(initialCtx, func(current decoderSnapshot) bool { return current.frames > 0 })
	decoded := time.Since(connection.startedAt)
	if !snapshot.firstFrame.IsZero() {
		decoded = snapshot.firstFrame.Sub(connection.startedAt)
	}
	if err != nil {
		_ = observer.close()
		return nil, MediaShape{}, decoded, "decode_failed"
	}
	target := min(config.RawCaptureBytes, 256<<10)
	snapshot, err = observer.wait(initialCtx, func(current decoderSnapshot) bool { return len(current.capture) >= target })
	if err != nil {
		_ = observer.close()
		return nil, MediaShape{}, decoded, "body_failed"
	}
	shape, err := config.Validator.Validate(initialCtx, snapshot.capture)
	if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 {
		_ = observer.close()
		return nil, MediaShape{}, decoded, "invalid_media"
	}
	return observer, shape, decoded, "ok"
}

func lifecycleDrill(ctx context.Context, endpoint *endpoint, config Config, channelIndex int, baseline ResourceSample, precondition string, sampler *phaseSampler) []Phase {
	const startsMetric = "loomarr_playout_session_starts_total"
	sampler.begin("cancellation")
	if precondition != "ok" {
		failure := phaseFrom("cancellation", []observation{{class: "baseline_not_converged"}})
		failure.Resources = sampler.end("cancellation")
		return lifecycleMissingSamples(sampler, failure)
	}
	channelID, target := config.Channels[channelIndex].ID, "full"
	before, err := endpoint.metricTotal(ctx, startsMetric)
	if err != nil {
		failure := phaseFrom("cancellation", []observation{{class: "metric_failed"}})
		failure.Resources = sampler.end("cancellation")
		return lifecycleMissingSamples(sampler, failure)
	}
	first, class := openRaw(ctx, endpoint, config, channelIndex)
	if class != "ok" {
		failure := phaseFrom("cancellation", []observation{{class: class}})
		failure.Resources = sampler.end("cancellation")
		return lifecycleMissingSamples(sampler, failure)
	}
	afterFirst, metricErr := endpoint.metricTotal(ctx, startsMetric)
	firstState := waitForSessionState(ctx, endpoint, config, config.RequestTimeout, func(snapshot sessionSnapshot) bool {
		session, found := snapshot.session(channelID, target)
		return found && session.Viewers > 0
	})
	_ = first.body.Close()
	cancelStarted := time.Now()
	cancelClass := waitForSessionState(ctx, endpoint, config, config.RequestTimeout, func(snapshot sessionSnapshot) bool {
		session, found := snapshot.session(channelID, target)
		return found && session.Viewers == 0
	})
	if firstState != "ok" {
		cancelClass = "session_identity_failed"
	}
	cancellation := phaseFrom("cancellation", []observation{{duration: time.Since(cancelStarted), firstByte: first.firstByte, class: cancelClass}})
	cancellation.Resources = sampler.end("cancellation")

	sampler.begin("warm_reuse")
	second, warmClass := openRaw(ctx, endpoint, config, channelIndex)
	if warmClass == "ok" {
		afterSecond, totalErr := endpoint.metricTotal(ctx, startsMetric)
		warmState := waitForSessionState(ctx, endpoint, config, config.RequestTimeout, func(snapshot sessionSnapshot) bool {
			session, found := snapshot.session(channelID, target)
			return found && session.Viewers > 0
		})
		if metricErr != nil || totalErr != nil || afterFirst <= before || afterSecond != afterFirst || warmState != "ok" {
			warmClass = "parent_not_reused"
		}
	}
	warmDuration, warmFirstByte := time.Duration(0), time.Duration(0)
	if second != nil {
		warmDuration, warmFirstByte = time.Since(second.startedAt), second.firstByte
		_ = second.body.Close()
	}
	warm := phaseFrom("warm_reuse", []observation{{duration: warmDuration, firstByte: warmFirstByte, class: warmClass}})
	warm.Resources = sampler.end("warm_reuse")

	sampler.begin("grace_expiry")
	expiryStarted := time.Now()
	expiryClass := waitForSessionState(ctx, endpoint, config, min(config.CleanupTimeout, config.WarmGrace+10*time.Second), func(snapshot sessionSnapshot) bool {
		_, found := snapshot.session(channelID, target)
		return !found
	})
	beforeRestart, totalErr := endpoint.metricTotal(ctx, startsMetric)
	third, restartClass := openRaw(ctx, endpoint, config, channelIndex)
	if expiryClass == "ok" && restartClass == "ok" {
		afterRestart, restartErr := endpoint.metricTotal(ctx, startsMetric)
		restartState := waitForSessionState(ctx, endpoint, config, config.RequestTimeout, func(snapshot sessionSnapshot) bool {
			session, found := snapshot.session(channelID, target)
			return found && session.Viewers > 0
		})
		if totalErr != nil || restartErr != nil || afterRestart <= beforeRestart || restartState != "ok" {
			expiryClass = "parent_not_restarted"
		}
	} else if expiryClass == "ok" {
		expiryClass = restartClass
	}
	restartFirstByte := time.Duration(0)
	if third != nil {
		restartFirstByte = third.firstByte
		_ = third.body.Close()
	}
	expiry := phaseFrom("grace_expiry", []observation{{duration: time.Since(expiryStarted), firstByte: restartFirstByte, class: expiryClass}})
	expiry.Resources = sampler.end("grace_expiry")
	return []Phase{cancellation, warm, expiry}
}

func lifecycleMissingSamples(sampler *phaseSampler, cancellation Phase) []Phase {
	sampler.begin("warm_reuse")
	warm := phaseFrom("warm_reuse", []observation{{class: "dependency_failed"}})
	warm.Resources = sampler.end("warm_reuse")
	sampler.begin("grace_expiry")
	expiry := phaseFrom("grace_expiry", []observation{{class: "dependency_failed"}})
	expiry.Resources = sampler.end("grace_expiry")
	return []Phase{cancellation, warm, expiry}
}

func waitForSessionState(ctx context.Context, endpoint *endpoint, config Config, timeout time.Duration, accept func(sessionSnapshot) bool) string {
	started := time.Now()
	for time.Since(started) <= timeout {
		snapshot, err := endpoint.sessions(ctx)
		if err == nil && accept(snapshot) {
			return "ok"
		}
		select {
		case <-ctx.Done():
			return "cancelled"
		case <-time.After(config.CleanupPoll):
		}
	}
	return "state_timeout"
}

func waitForConvergence(ctx context.Context, endpoint *endpoint, config Config, baseline ResourceSample, timeout time.Duration) (observation, ResourceSample) {
	started := time.Now()
	class := "cleanup_timeout"
	var converged ResourceSample
	for time.Since(started) <= timeout {
		// The harness itself fans out HTTP connections during the bursts. Retire
		// idle client sockets before judging the server's post-load fd/goroutine
		// baseline, otherwise the measurement counts its own keep-alive pool as a
		// Loomarr leak.
		config.Client.CloseIdleConnections()
		sample, sampleErr := endpoint.sample(ctx, "converged")
		if sampleErr == nil && resourceConverged(sample, baseline) {
			sample.Point = "converged"
			converged = sample
			class = "ok"
			break
		}
		select {
		case <-ctx.Done():
			class = "cancelled"
		case <-time.After(config.CleanupPoll):
		}
		if class == "cancelled" {
			break
		}
	}
	return observation{duration: time.Since(started), class: class}, converged
}

func resourceConverged(current, baseline ResourceSample) bool {
	return current.SessionsActive <= baseline.SessionsActive &&
		current.TranscodeCost <= baseline.TranscodeCost &&
		current.FFmpegRunning <= baseline.FFmpegRunning &&
		current.OpenFDs <= baseline.OpenFDs+4 &&
		current.Goroutines <= baseline.Goroutines+8
}

func parallel(workers, count int, fn func(int) observation) []observation {
	if count <= 0 {
		return nil
	}
	workers = min(max(workers, 1), count)
	results := make([]observation, count)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = fn(index)
			}
		}()
	}
	for index := range count {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func phaseFrom(name string, observations []observation) Phase {
	latencies := make([]time.Duration, 0, len(observations))
	firstBytes := make([]time.Duration, 0, len(observations))
	failures, hits := 0, 0
	classes := map[string]int{}
	media := []MediaShape{}
	for _, item := range observations {
		classes[item.class]++
		if item.class == "ok" {
			latencies = append(latencies, item.duration)
			if item.firstByte > 0 {
				firstBytes = append(firstBytes, item.firstByte)
			}
		} else {
			failures++
		}
		if item.hit {
			hits++
		}
		if item.media.VideoStreams != 0 || item.media.AudioStreams != 0 {
			media = append(media, item.media)
		}
	}
	phase := Phase{Name: name, LatencySummary: summarize(latencies, failures), PreparedHits: hits, HTTPClasses: classes, Media: media}
	phase.FirstByte = summarize(firstBytes, failures)
	return phase
}

func selectBurstIndexes(available []int, count int) []int {
	if count > len(available) {
		count = len(available)
	}
	if count < 0 {
		count = 0
	}
	return append([]int(nil), available[:count]...)
}

func recordBurstSample(report *Report, phase string, sample ResourceSample) {
	if sample.Capacity == 0 {
		report.Failures = append(report.Failures, phase+"_resource_sample_failed")
		return
	}
	// rawBurst/startHeldBurst take this sample before releasing their request
	// cohort, so it is an in-phase observation of the active load.
	sample.Point = phase + "_in_phase"
	report.Resources = append(report.Resources, sample)
}

func recordConvergenceSample(report *Report, phase string, sample ResourceSample) {
	if sample.Capacity == 0 {
		report.Failures = append(report.Failures, phase+"_resource_sample_failed")
		return
	}
	sample.Point = phase + "_converged"
	report.Resources = append(report.Resources, sample)
}

type heldBurst struct {
	results    []observation
	continuity []HeldContinuityObservation
	sample     ResourceSample
	verifyC    chan struct{}
	verifyAck  chan struct{}
	releaseC   chan struct{}
	wg         sync.WaitGroup
}

func (b *heldBurst) verify(ctx context.Context) {
	close(b.verifyC)
	for range b.results {
		<-b.verifyAck
	}
}

func (b *heldBurst) release() {
	close(b.releaseC)
	b.wg.Wait()
}

func (b *heldBurst) failed() bool {
	for _, result := range b.results {
		if result.class != "ok" {
			return true
		}
	}
	return false
}

func startHeldBurst(ctx context.Context, endpoint *endpoint, config Config, indexes []int) *heldBurst {
	b := &heldBurst{results: make([]observation, len(indexes)), continuity: make([]HeldContinuityObservation, len(indexes)), verifyC: make(chan struct{}), verifyAck: make(chan struct{}, len(indexes)), releaseC: make(chan struct{})}
	if len(indexes) == 0 {
		b.results = []observation{{class: "no_capacity"}}
		return b
	}
	start, ready := make(chan struct{}), make(chan struct{}, len(indexes))
	for position, channelIndex := range indexes {
		b.wg.Add(1)
		go func(position, channelIndex int) {
			defer b.wg.Done()
			<-start
			connection, class := openRaw(ctx, endpoint, config, channelIndex)
			if class != "ok" {
				b.results[position] = observation{class: class}
				b.continuity[position] = HeldContinuityObservation{Outcome: "admission_failed"}
				ready <- struct{}{}
				select {
				case <-b.verifyC:
				case <-b.releaseC:
				case <-ctx.Done():
				}
				b.verifyAck <- struct{}{}
				return
			}
			observer, shape, decoded, class := startValidatedObserver(ctx, config, connection)
			if class != "ok" {
				b.results[position] = observation{duration: decoded, firstByte: connection.firstByte, class: class}
				b.continuity[position] = HeldContinuityObservation{Outcome: class}
				ready <- struct{}{}
				select {
				case <-b.verifyC:
				case <-b.releaseC:
				case <-ctx.Done():
				}
				b.verifyAck <- struct{}{}
				return
			}
			b.results[position] = observation{duration: decoded, firstByte: connection.firstByte, class: "ok", media: shape}
			ready <- struct{}{}
			select {
			case <-b.verifyC:
				b.observeHeldViewer(ctx, config, position, connection, observer, shape)
				b.verifyAck <- struct{}{}
				return
			case <-ctx.Done():
				_ = observer.close()
				b.results[position] = observation{duration: time.Since(connection.startedAt), firstByte: connection.firstByte, class: "held_stream_interrupted"}
				b.continuity[position] = HeldContinuityObservation{Outcome: "cancelled"}
				b.verifyAck <- struct{}{}
			case <-b.releaseC:
				_ = observer.close()
				b.results[position] = observation{duration: time.Since(connection.startedAt), firstByte: connection.firstByte, class: "held_stream_interrupted"}
				b.continuity[position] = HeldContinuityObservation{Outcome: "released"}
				b.verifyAck <- struct{}{}
			}
		}(position, channelIndex)
	}
	close(start)
	for range indexes {
		<-ready
	}
	sample, err := endpoint.sample(ctx, "raw_capacity")
	if err == nil {
		b.sample = sample
	}
	return b
}

func (b *heldBurst) observeHeldViewer(ctx context.Context, config Config, position int, connection *rawConnection, observer *decoderObserver, shape MediaShape) {
	remaining := time.Until(connection.startedAt.Add(config.RequestTimeout)) - 50*time.Millisecond
	horizon := min(remaining, 1500*time.Millisecond)
	if horizon <= 0 {
		horizon = time.Millisecond
	}
	started := time.Now()
	late := started.Add(3 * horizon / 4)
	baseline := observer.snapshot()
	timer := time.NewTimer(horizon)
	outcome := "observed"
	select {
	case <-timer.C:
	case <-ctx.Done():
		outcome = "cancelled"
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	preClose := observer.snapshot()
	_ = observer.close()
	reads, total := preClose.reads-baseline.reads, preClose.bytes-baseline.bytes
	evidence := HeldContinuityObservation{Outcome: outcome, ObservationMS: float64(time.Since(started)) / float64(time.Millisecond), AdvancingReads: max(reads, 0), BytesObserved: max(total, 0), Media: shape}
	if outcome == "observed" && preClose.readErr == io.EOF {
		outcome = "ended"
	} else if outcome == "observed" && preClose.decoderErr != nil {
		outcome = "decode_failed"
	} else if outcome == "observed" && preClose.readErr != nil {
		outcome = "body_failed"
	} else if outcome == "observed" && (reads <= 0 || total <= 0 || preClose.lastRead.Before(late)) {
		outcome = "stalled"
	} else if outcome == "observed" && (preClose.frames <= baseline.frames || preClose.lastFrame.Before(late)) {
		outcome = "decode_failed"
	}
	evidence.DecodedFrame = preClose.frames > baseline.frames && !preClose.lastFrame.Before(late)
	evidence.Outcome = outcome
	b.continuity[position] = evidence
	class := "ok"
	if outcome != "observed" {
		class = "held_stream_interrupted"
	}
	b.results[position] = observation{duration: time.Since(connection.startedAt), firstByte: connection.firstByte, class: class, media: shape}
}

func rawBurst(ctx context.Context, endpoint *endpoint, config Config, indexes []int) ([]observation, ResourceSample) {
	count := len(indexes)
	if count <= 0 {
		return []observation{{class: "no_capacity"}}, ResourceSample{}
	}
	start := make(chan struct{})
	release := make(chan struct{})
	ready := make(chan struct{}, count)
	results := make([]observation, count)
	var wg sync.WaitGroup
	for position := range count {
		channelIndex := indexes[position]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			connection, class := openRaw(ctx, endpoint, config, channelIndex)
			if class != "ok" {
				results[position] = observation{class: class}
				ready <- struct{}{}
				return
			}
			observer, shape, decoded, class := startValidatedObserver(ctx, config, connection)
			results[position] = observation{duration: decoded, firstByte: connection.firstByte, class: class, media: shape}
			ready <- struct{}{}
			if observer == nil {
				return
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			_ = observer.close()
		}()
	}
	close(start)
	for range count {
		<-ready
	}
	sample, err := endpoint.sample(ctx, "burst")
	if err != nil {
		sample = ResourceSample{}
	}
	close(release)
	wg.Wait()
	return results, sample
}
