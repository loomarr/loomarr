package playoutcert

import "context"

// copyWorkload measures the declared lane through ordinary playback. A label
// cannot substitute for a cold source, decoded media, or admission accounting.
func copyWorkload(ctx context.Context, endpoint *endpoint, config Config, indexes []int, catalog []observation, baseline ResourceSample) (Phase, []ResourceSample) {
	if len(indexes) == 0 {
		return phaseFrom("copy_raw", []observation{{class: "cohort_missing"}}), nil
	}
	var results []observation
	var continuity []HeldContinuityObservation
	var samples []ResourceSample
	for batch := 0; batch < len(indexes); batch += config.Concurrency {
		converged, sample := waitForConvergence(ctx, endpoint, config, baseline, config.CleanupTimeout)
		samples = append(samples, sample)
		if converged.class != "ok" {
			results = append(results, observation{class: "baseline_not_converged"})
			break
		}
		var selected []int
		for _, index := range indexes[batch:min(batch+config.Concurrency, len(indexes))] {
			switch {
			case catalog[index].hit:
				results = append(results, observation{class: "copy_source_prepared"})
			case catalog[index].class != "prepared_miss":
				results = append(results, observation{class: "dependency_failed"})
			default:
				selected = append(selected, index)
			}
		}
		if len(selected) == 0 {
			continue
		}
		held := startHeldBurst(ctx, endpoint, config, selected)
		samples = append(samples, held.sample)
		switch {
		case held.sample.Capacity == 0:
			results = append(results, observation{class: "metric_failed"})
		case held.sample.TranscodeCost != baseline.TranscodeCost:
			results = append(results, observation{class: "copy_cost_mismatch"})
		case held.sample.SessionsActive != baseline.SessionsActive+len(selected) || held.sample.ViewerActive < len(selected):
			results = append(results, observation{class: "copy_session_missing"})
		}
		// Startup deliberately reserves conservatively before the child's copy
		// proof. Keep that overall peak, but sample the complete held interval
		// separately so its copy claim cannot hide a later video-cost spike.
		playbackSampler := newPhaseSampler(ctx, endpoint, config.CleanupPoll)
		playbackSampler.begin("copy_raw")
		held.verify(ctx)
		playback := playbackSampler.end("copy_raw")
		playbackSampler.close()
		held.release()
		if playback.SampleFailures > 0 || playback.Samples == 0 {
			results = append(results, observation{class: "metric_failed"})
		}
		if playback.Maximum.TranscodeCost != baseline.TranscodeCost {
			results = append(results, observation{class: "copy_cost_mismatch"})
		}
		if playback.Samples > 0 {
			playback.Maximum.Point = "copy_raw_held"
			samples = append(samples, playback.Maximum)
		}
		results = append(results, held.results...)
		continuity = append(continuity, held.continuity...)
	}
	converged, sample := waitForConvergence(ctx, endpoint, config, baseline, config.CleanupTimeout)
	samples = append(samples, sample)
	if converged.class != "ok" {
		results = append(results, converged)
	}
	phase := phaseFrom("copy_raw", results)
	phase.HeldContinuity = continuity
	return phase, samples
}
