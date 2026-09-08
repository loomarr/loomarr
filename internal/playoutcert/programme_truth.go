package playoutcert

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"
)

// ProgrammeEvidenceSource supplies independently qualified private expectations.
// Freeze runs before observation; decoded signals are never inputs to this port.
type ProgrammeEvidenceSource interface {
	Freeze(channelID string, from, until time.Time) (ProgrammeEvidence, error)
	PrivateInputs() []string
	CohortManifestSHA256() string
}

type ProgrammeEvidence struct {
	Programmes   []ExpectedProgramme
	ResolveAsset func(context.Context, ProgrammeAsset) (ProgrammeAssetEvidence, error)
}

// ProgrammeAsset is private transport provenance, never part of a public report.
type ProgrammeAsset struct {
	Reference     string
	InitReference string
	Epoch         string
	StartedAt     time.Time
	Duration      time.Duration
}

// ProgrammeAssetEvidence binds the media clock to independently supplied bytes.
// Each reference is bounded to 32 MiB and copied before transport observation.
type ProgrammeAssetEvidence struct {
	Clock ProgrammeMediaClock
	Media []byte
	Init  []byte
	// Validate rejects a retired live source. Immutable publications need no
	// liveness callback. It is checked before admitting each read and success.
	Validate func() error
}

type ProgrammeMediaClock struct {
	Origin     time.Time
	Generation string
}

type SignalRange struct{ Min, Max float64 }

// ProgrammeSignature is predeclared media truth independent of an airing time.
type ProgrammeSignature struct {
	Luma, ZeroCrossingRate, RMSDB SignalRange
	Silence                       bool
}

func (s ProgrammeSignature) Programme(start, end time.Time) ExpectedProgramme {
	return ExpectedProgramme{StartsAt: start, EndsAt: end, Luma: s.Luma, ZeroCrossingRate: s.ZeroCrossingRate, RMSDB: s.RMSDB, Silence: s.Silence}
}

type ExpectedProgramme struct {
	StartsAt, EndsAt              time.Time
	Luma, ZeroCrossingRate, RMSDB SignalRange
	Silence                       bool
}

func (r SignalRange) contains(value float64) bool {
	return finite(value) && value >= r.Min && value <= r.Max
}

func (r SignalRange) valid(minimum, maximum float64) bool {
	return finite(r.Min) && finite(r.Max) && r.Min >= minimum && r.Max <= maximum && r.Min <= r.Max
}

func (r SignalRange) overlaps(other SignalRange) bool {
	return r.Min <= other.Max && other.Min <= r.Max
}

func freezeProgrammeEvidence(source ProgrammeEvidenceSource, channelID string, from, until time.Time) (ProgrammeEvidence, error) {
	if source == nil || !until.After(from) {
		return ProgrammeEvidence{}, errors.New("evidence_unavailable")
	}
	evidence, err := source.Freeze(channelID, from, until)
	if err != nil {
		return ProgrammeEvidence{}, errors.New("evidence_unavailable")
	}
	if evidence.ResolveAsset == nil {
		return ProgrammeEvidence{}, errors.New("evidence_unavailable")
	}
	if err := validateProgrammeSequence(evidence.Programmes, from, until); err != nil {
		return ProgrammeEvidence{}, err
	}
	evidence.Programmes = slices.Clone(evidence.Programmes)
	return evidence, nil
}

func validateProgrammeSequence(programmes []ExpectedProgramme, from, until time.Time) error {
	if len(programmes) < 2 || len(programmes) > 2048 || !until.After(from) {
		return errors.New("invalid_programme_truth")
	}
	for index, programme := range programmes {
		if programme.StartsAt.IsZero() || !programme.EndsAt.After(programme.StartsAt) || !programme.Luma.valid(0, 255) ||
			!programme.ZeroCrossingRate.valid(0, 1) || !programme.RMSDB.valid(-200, 0) {
			return errors.New("invalid_programme_truth")
		}
		if index == 0 {
			continue
		}
		previous := programmes[index-1]
		if !previous.EndsAt.Equal(programme.StartsAt) {
			return errors.New("invalid_programme_truth")
		}
		if previous.Luma.overlaps(programme.Luma) && previous.Silence == programme.Silence &&
			(previous.Silence || (previous.ZeroCrossingRate.overlaps(programme.ZeroCrossingRate) && previous.RMSDB.overlaps(programme.RMSDB))) {
			return errors.New("ambiguous_programme_truth")
		}
	}
	if programmes[0].StartsAt.After(from) || programmes[len(programmes)-1].EndsAt.Before(until) {
		return errors.New("incomplete_programme_truth")
	}
	return nil
}

// programmeSignalCheck is confined to the observation goroutine. Audio and video
// callbacks may arrive in either order; each stream advances independently.
type programmeSignalCheck struct {
	programmes                         []ExpectedProgramme
	armedAt                            time.Time
	guard                              time.Duration
	video, audio                       []int64
	lastVideo, lastAudio               time.Time
	lastMatchedVideo, lastMatchedAudio time.Time
	videoFrames, audioSamples          int64
	initial, transition                int
}

func newProgrammeSignalCheck(programmes []ExpectedProgramme, armedAt time.Time) *programmeSignalCheck {
	c := &programmeSignalCheck{programmes: slices.Clone(programmes), armedAt: armedAt, guard: 100 * time.Millisecond,
		video: make([]int64, len(programmes)), audio: make([]int64, len(programmes)), initial: -1, transition: -1}
	// Select the succession from the frozen schedule and arm time, before any
	// signal arrives. A nearly finished programme cannot provide two guarded windows.
	c.initial = slices.IndexFunc(c.programmes, func(p ExpectedProgramme) bool { return p.EndsAt.After(armedAt.Add(2 * c.guard)) })
	return c
}

// A declared discontinuity starts a separate decoder timeline. Its own strict
// ordering begins afresh; frozen expectations, arm time and matched evidence remain.
func (c *programmeSignalCheck) beginEpoch() { c.lastVideo = time.Time{}; c.lastAudio = time.Time{} }

func signalTime(clock ProgrammeMediaClock, ptsUS int64) (time.Time, error) {
	if clock.Origin.IsZero() || clock.Generation == "" || ptsUS > math.MaxInt64/int64(time.Microsecond) || ptsUS < math.MinInt64/int64(time.Microsecond) {
		return time.Time{}, errors.New("invalid_media_clock")
	}
	return clock.Origin.Add(time.Duration(ptsUS) * time.Microsecond), nil
}

func (c *programmeSignalCheck) programme(at time.Time, duration time.Duration) (int, error) {
	if at.Before(c.armedAt) {
		return -1, nil
	}
	index := slices.IndexFunc(c.programmes, func(p ExpectedProgramme) bool { return !at.Before(p.StartsAt) && at.Before(p.EndsAt) })
	if index < 0 {
		return -1, errors.New("media_outside_truth")
	}
	p := c.programmes[index]
	if at.Before(p.StartsAt.Add(c.guard)) || at.Add(duration).After(p.EndsAt.Add(-c.guard)) {
		return -1, nil
	}
	return index, nil
}

func (c *programmeSignalCheck) videoSignal(clock ProgrammeMediaClock, signal DecodedVideoSignal) error {
	if !finite(signal.Luma) || signal.Luma < 0 || signal.Luma > 255 {
		return errors.New("invalid_video_signal")
	}
	at, err := signalTime(clock, signal.PTSUS)
	if err != nil {
		return err
	}
	if !c.lastVideo.IsZero() && !at.After(c.lastVideo) {
		return errors.New("video_time_regressed")
	}
	c.lastVideo = at
	index, err := c.programme(at, 0)
	if err != nil || index < 0 {
		return err
	}
	if !c.programmes[index].Luma.contains(signal.Luma) {
		return errors.New("programme_video_mismatch")
	}
	c.video[index]++
	c.videoFrames++
	c.lastMatchedVideo = at
	c.advance()
	return nil
}

func (c *programmeSignalCheck) audioSignal(clock ProgrammeMediaClock, signal DecodedAudioSignal) error {
	at, err := signalTime(clock, signal.PTSUS)
	if err != nil {
		return err
	}
	if signal.Samples <= 0 || signal.Samples > 1048576 || !finite(signal.ZeroCrossingRate) || signal.ZeroCrossingRate < 0 || signal.ZeroCrossingRate > 1 ||
		(!signal.Silence && (!finite(signal.RMSDB) || signal.RMSDB > 0)) {
		return errors.New("invalid_audio_signal")
	}
	if !c.lastAudio.IsZero() && !at.After(c.lastAudio) {
		return errors.New("audio_time_regressed")
	}
	c.lastAudio = at
	duration := time.Duration(signal.Samples) * time.Second / 48000
	index, err := c.programme(at, duration)
	if err != nil || index < 0 {
		return err
	}
	p := c.programmes[index]
	if signal.Silence != p.Silence || !p.ZeroCrossingRate.contains(signal.ZeroCrossingRate) || (!signal.Silence && !p.RMSDB.contains(signal.RMSDB)) {
		return errors.New("programme_audio_mismatch")
	}
	c.audio[index] += signal.Samples
	c.audioSamples += signal.Samples
	c.lastMatchedAudio = at.Add(duration)
	c.advance()
	return nil
}

func (c *programmeSignalCheck) advance() {
	if c.initial < 0 || c.initial+1 >= len(c.programmes) {
		return
	}
	next := c.initial + 1
	if c.video[c.initial] >= 2 && c.audio[c.initial] >= 2048 && c.video[next] >= 2 && c.audio[next] >= 2048 {
		c.transition = next
	}
}
