package playoutcert

import (
	"bytes"
	"context"
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
	if err := config.Validate(); err != nil {
		return Report{}, err
	}
	config = config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		return Report{}, err
	}
	report := Report{SchemaVersion: SchemaVersion, StartedAt: config.Now(), Phases: []Phase{}, Resources: []ResourceSample{}, Failures: []string{}}
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
	preparedIndexes := preparedChannelIndexes(config.Channels)
	transcodeIndexes := transcodeChannelIndexes(config.Channels)

	signed := make([]*url.URL, len(config.Channels))
	mintObs := parallel(config.Concurrency, len(config.Channels), func(index int) observation {
		urlValue, elapsed, class := endpoint.mint(ctx, config.Channels[index].ID)
		if class == "ok" {
			signed[index] = urlValue
		}
		return observation{duration: elapsed, class: class}
	})
	configuredObs := parallel(config.Concurrency, len(preparedIndexes), func(iteration int) observation {
		index := preparedIndexes[iteration]
		if signed[index] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[index])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	configured := phaseFrom("configured", append(mintObs, configuredObs...))
	configured.Attempts = len(preparedIndexes)
	configured.Successes = configured.PreparedHits
	configured.Failures = configured.Attempts - configured.Successes
	report.Phases = append(report.Phases, phaseFrom("mint", mintObs), configured)

	surfCount := len(preparedIndexes) * config.SurfRounds
	surfObs := parallel(config.Concurrency, surfCount, func(iteration int) observation {
		index := preparedIndexes[iteration%len(preparedIndexes)]
		if signed[index] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[index])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	report.Phases = append(report.Phases, phaseFrom("surf", surfObs))

	fanObs := parallel(config.FanInViewers, config.FanInViewers, func(int) observation {
		if len(preparedIndexes) == 0 || signed[preparedIndexes[0]] == nil {
			return observation{class: "mint_failed"}
		}
		elapsed, hit, class := endpoint.prepared(ctx, signed[preparedIndexes[0]])
		return observation{duration: elapsed, hit: hit, class: class}
	})
	report.Phases = append(report.Phases, phaseFrom("fan_in", fanObs))
	preparedRaw, preparedRawSample := rawBurst(ctx, endpoint, config, selectBurstIndexes(preparedIndexes, target.Capacity))
	preparedRawPhase := phaseFrom("prepared_raw", preparedRaw)
	report.Phases = append(report.Phases, preparedRawPhase)
	if preparedRawSample.Capacity > 0 {
		report.Resources = append(report.Resources, preparedRawSample)
	}

	raw, rawSample := rawBurst(ctx, endpoint, config, selectBurstIndexes(transcodeIndexes, target.Capacity))
	report.Phases = append(report.Phases, phaseFrom("raw_capacity", raw))
	if rawSample.Capacity > 0 {
		report.Resources = append(report.Resources, rawSample)
	}
	recoveryObs, recoverySample := waitForConvergence(ctx, endpoint, config, baseline, min(config.CleanupTimeout, config.WarmGrace+10*time.Second))
	report.Phases = append(report.Phases, phaseFrom("capacity_recovery", []observation{recoveryObs}))
	if recoverySample.Capacity > 0 {
		report.Resources = append(report.Resources, recoverySample)
	}
	overload, overloadSample := rawBurst(ctx, endpoint, config, selectBurstIndexes(transcodeIndexes, target.Capacity+1))
	overloadPhase := phaseFrom("overload", overload)
	if rejected := overloadPhase.HTTPClasses["http_503"]; rejected > 0 {
		overloadPhase.Failures -= rejected
		overloadPhase.Successes += rejected
	}
	report.Phases = append(report.Phases, overloadPhase)
	if overloadSample.Capacity > 0 {
		report.Resources = append(report.Resources, overloadSample)
	}

	cleanupObs, finalSample := waitForConvergence(ctx, endpoint, config, baseline, config.CleanupTimeout)
	report.Phases = append(report.Phases, phaseFrom("cleanup", []observation{cleanupObs}))
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
			started := time.Now()
			path := "/v1/playout/stream/" + url.PathEscape(config.Channels[channelIndex].ID) + "?token=" + url.QueryEscape(endpoint.device)
			resp, err := endpoint.request(ctx, http.MethodGet, path, nil, false)
			if err != nil {
				results[position] = observation{class: "request_failed"}
				ready <- struct{}{}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				results[position] = observation{class: httpClass(resp.StatusCode)}
				ready <- struct{}{}
				return
			}
			var first [1]byte
			if _, readErr := io.ReadFull(resp.Body, first[:]); readErr != nil {
				results[position] = observation{class: "body_failed"}
				ready <- struct{}{}
				return
			}
			firstByte := time.Since(started)
			capture, decodeErr := config.Decoder.FirstFrame(ctx, io.MultiReader(bytes.NewReader(first[:]), resp.Body), config.RawCaptureBytes)
			decoded := time.Since(started)
			shape := MediaShape{}
			class := "ok"
			if decodeErr != nil {
				class = "decode_failed"
			}
			if class == "ok" {
				capture, err = completeValidationCapture(resp.Body, capture, config.RawCaptureBytes)
				if err != nil {
					class = "body_failed"
				}
			}
			if class == "ok" && config.Validator != nil {
				shape, err = config.Validator.Validate(ctx, capture)
				if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 {
					class = "invalid_media"
				}
			}
			results[position] = observation{duration: decoded, firstByte: firstByte, class: class, media: shape}
			ready <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
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

func completeValidationCapture(body io.Reader, capture []byte, maxBytes int) ([]byte, error) {
	target := min(maxBytes, 256<<10)
	if len(capture) >= target {
		return append([]byte(nil), capture[:target]...), nil
	}
	out := make([]byte, len(capture), target)
	copy(out, capture)
	additional := make([]byte, target-len(out))
	if _, err := io.ReadFull(body, additional); err != nil {
		return out, err
	}
	return append(out, additional...), nil
}
