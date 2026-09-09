package playoutcert

import "strings"

// provenance describes where an emitted value came from, rather than what it
// happens to equal. A private probe can collide with independent report data;
// that is not evidence that the renderer disclosed the probe's source.
type provenance uint8

const (
	provenanceUnknown provenance = iota
	provenanceFixed
	provenanceDynamic
	provenanceSensitive
)

func fixedReportString(path, value string) bool {
	switch path {
	case "auditStatus":
		return oneOf(value, "missing", "passed", "failed", "unavailable")
	case "auditReason":
		return oneOf(value, "publication_not_finalized", "audit_capsule_missing", "run_proof_missing", "report_mutated", "sensitive_value_exposed", "dynamic_value_collision", "matcher_limit_exceeded", "output_limit_exceeded", "finalization_deadline_exceeded", "encoding_unsupported", "provenance_missing")
	case "phases.*.name":
		return fixedPhaseName(value)
	case "phases.*.programmeBoundaries.*.lane":
		return oneOf(value, "prepared", "transcode")
	case "phases.*.programmeBoundaries.*.outcome", "phases.*.heldContinuity.*.outcome":
		return fixedObservationClass(value)
	case "phases.*.resources.maximum.point", "resources.*.point", "faultProfiles.*.baseline.point", "faultProfiles.*.phasePeak.point", "faultProfiles.*.final.point":
		return fixedSamplePoint(value)
	case "faultProfiles.*.profile":
		return oneOf(value, string(FaultChildFailure), string(FaultParentFailure), string(FaultShutdown))
	case "faultProfiles.*.status":
		return oneOf(value, "qualified", "unqualified", "unavailable")
	case "faultProfiles.*.outcome":
		return oneOf(value, "complete", "not_selected", "controller_not_exercised", "controller_unavailable", "run_failed", "cleanup_failed")
	case "faultProfiles.*.receiptOutcome":
		return oneOf(value, "exited", "not_observed", "fault_budget_expired")
	case "faultProfiles.*.selectedContinuity", "faultProfiles.*.peerContinuity", "faultProfiles.*.recovery":
		return oneOf(value, "interrupted", "continued", "recovered", "not_applicable", "not_observed")
	}
	if path == "failures.*" {
		return fixedFailure(value)
	}
	return false
}

func fixedReportScalar(path, value string) bool {
	switch path {
	case "schemaVersion":
		return value == "3"
	case "certified":
		return value == "true" || value == "false"
	}
	return false
}

func fixedSummaryValue(kind, value string) bool {
	switch kind {
	case "phase":
		return fixedPhaseName(value)
	case "faultProfile":
		return oneOf(value, string(FaultChildFailure), string(FaultParentFailure), string(FaultShutdown))
	case "faultStatus":
		return oneOf(value, "qualified", "unqualified", "unavailable")
	case "faultOutcome":
		return oneOf(value, "complete", "not_selected", "controller_not_exercised", "controller_unavailable", "run_failed", "cleanup_failed")
	case "failure":
		return fixedFailure(value)
	case "httpClass":
		return fixedObservationClass(value)
	}
	return false
}

func fixedObservationClass(value string) bool {
	return oneOf(value,
		"ok", "request_failed", "invalid_response", "invalid_signed_url", "audit_unavailable", "prepared_miss", "body_failed", "invalid_hls", "empty_asset",
		"mint_failed", "fanout_split", "transcode_cohort_insufficient", "admission_outcome_missing", "held_viewer_interrupted", "cohort_missing", "controller_unavailable", "initial_media_missing", "shutdown_failed", "generation_unavailable", "fault_failed",
		"copy_cost_mismatch", "copy_session_missing", "copy_source_prepared",
		"asset_clock_mismatch", "programme_observation_timeout", "unexpected_media_eof",
		"invalid_media_clock", "media_outside_truth", "invalid_video_signal", "invalid_audio_signal",
		"video_time_regressed", "audio_time_regressed", "programme_video_mismatch", "programme_audio_mismatch",
		"evidence_unavailable", "witness_failed", "close_failed", "initial_witness_timeout", "initial_witness_cancelled", "initial_witness_failed", "transition_timeout", "transition_cancelled", "transition_failed", "late_observation_timeout", "post_boundary_decode_failed", "post_boundary_stalled", "decode_failed", "invalid_media",
		"baseline_not_converged", "metric_failed", "dependency_failed", "cancelled", "state_timeout", "no_capacity", "held_stream_interrupted", "admission_failed", "released", "http_503",
	)
}

func fixedPhaseName(value string) bool {
	return oneOf(value, "mint", "configured", "surf", "copy_raw", "prepared_fan_in", "fan_in", "prepared_raw", "raw_capacity", "overload", "capacity_recovery", "cleanup", "shutdown", "child_failure", "parent_failure", "programme_boundary", "cancellation", "warm_reuse", "grace_expiry")
}

func fixedSamplePoint(value string) bool {
	if oneOf(value, "baseline", "final", "converged", "raw_capacity", "burst", "copy_raw_held") {
		return true
	}
	for _, suffix := range []string{"_sample", "_in_phase", "_converged"} {
		if name, ok := strings.CutSuffix(value, suffix); ok && fixedPhaseName(name) {
			return true
		}
	}
	return false
}

func fixedFailure(value string) bool {
	if oneOf(value, "prepared_coverage_incomplete", "prepared_p95_exceeded", "prepared_raw_p95_exceeded", "capacity_oversubscribed", "cleanup_residual", string(PublicationDowngradeCleanupFailed)) {
		return true
	}
	for _, suffix := range []string{"_resource_sample_failed", "_failed", "_unavailable"} {
		if name, ok := strings.CutSuffix(value, suffix); ok && (fixedPhaseName(name) || oneOf(name, string(FaultChildFailure), string(FaultParentFailure), string(FaultShutdown))) {
			return true
		}
	}
	return false
}

func provenanceForJSONKey(path, key string) provenance {
	if path == "phases.*.httpClasses" && fixedObservationClass(key) {
		return provenanceFixed
	}
	if oneOf(key, jsonObjectFields(path)...) {
		return provenanceFixed
	}
	return provenanceDynamic
}

func jsonObjectFields(path string) []string {
	switch path {
	case "":
		return []string{"schemaVersion", "startedAt", "completedAt", "target", "phases", "resources", "failures", "faultProfiles", "certified", "auditStatus", "auditReason"}
	case "target":
		return []string{"version", "revision", "manifestSha256", "cohortManifestSha256", "configuredChannels", "capacity"}
	case "phases.*":
		return []string{"name", "attempts", "successes", "failures", "p50Ms", "p95Ms", "p99Ms", "firstByte", "preparedHits", "httpClasses", "media", "heldContinuity", "programmeBoundaries", "resources"}
	case "phases.*.firstByte":
		return []string{"attempts", "successes", "failures", "p50Ms", "p95Ms", "p99Ms"}
	case "phases.*.media.*", "phases.*.programmeBoundaries.*.media", "phases.*.heldContinuity.*.media":
		return []string{"videoStreams", "audioStreams", "videoCodec", "audioCodec"}
	case "phases.*.programmeBoundaries.*":
		return []string{"lane", "outcome", "transitions", "observationMs", "decodedFrameDelta", "decodedAudioSamplesDelta", "readDelta", "bytesDelta", "media"}
	case "phases.*.heldContinuity.*":
		return []string{"outcome", "observationMs", "advancingReads", "bytesObserved", "decodedFrame", "media"}
	case "phases.*.resources":
		return []string{"samples", "sampleFailures", "intervalMs", "cpuSecondsDelta", "maximum"}
	case "phases.*.resources.maximum", "resources.*", "faultProfiles.*.baseline", "faultProfiles.*.phasePeak", "faultProfiles.*.final":
		return []string{"point", "rssBytes", "cpuSeconds", "openFds", "goroutines", "httpInFlight", "sessionsActive", "viewerActive", "graceIdle", "transcodeCost", "capacity", "ffmpegRunning", "preparedChannels", "readyChannels", "channelHealth", "stalledChannels", "gpuVramGiB", "llmVramGiB", "gpuContended"}
	case "faultProfiles.*":
		return []string{"profile", "status", "outcome", "baseline", "phasePeak", "final", "receiptOutcome", "selectedContinuity", "peerContinuity", "recovery"}
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
