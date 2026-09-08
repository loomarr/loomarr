package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/prepared"
)

type syntheticAssetTruth struct {
	initDigest         [32]byte
	offset, firstVideo time.Duration
}

type syntheticLiveClock struct {
	clock      playoutcert.ProgrammeMediaClock
	source     *playout.Process
	ctx        context.Context
	generation uint64
}

type syntheticProgrammeEvidence struct {
	privateInputs        []string
	cohortManifestSHA256 string
	schedule             syntheticProgrammeSchedule
	signatures           map[string][2]playoutcert.ProgrammeSignature
	prepared             map[string]bool
	assets               map[string][2]map[string]syntheticAssetTruth
	origin               *playout.PreparedOrigin
	live                 *playout.HLSManager
	mu                   sync.Mutex
	clocks               map[string]syntheticLiveClock
}

func (s *syntheticProgrammeEvidence) PrivateInputs() []string {
	return append([]string(nil), s.privateInputs...)
}

func (s *syntheticProgrammeEvidence) CohortManifestSHA256() string {
	return s.cohortManifestSHA256
}

func (t *PlayoutCertificationTarget) ProgrammeEvidence() playoutcert.ProgrammeEvidenceSource {
	return t.programmeEvidence
}

func (s *syntheticProgrammeEvidence) recordClock(ctx context.Context, channel string, process *playout.Process, generation uint64, origin time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.clocks[channel].generation > generation {
		return
	}
	s.clocks[channel] = syntheticLiveClock{source: process, ctx: ctx, generation: generation, clock: playoutcert.ProgrammeMediaClock{Origin: origin, Generation: strconv.FormatUint(generation, 10)}}
}

func (s *syntheticProgrammeEvidence) Freeze(channel string, from, until time.Time) (playoutcert.ProgrammeEvidence, error) {
	preparedChannel, known := s.prepared[channel]
	if !known || !until.After(from) {
		return playoutcert.ProgrammeEvidence{}, errors.New("unknown programme cohort")
	}
	signatures, ok := s.signatures[channel]
	if !ok {
		return playoutcert.ProgrammeEvidence{}, errors.New("channel signal truth unavailable")
	}
	first := s.schedule.ordinal(from)
	last := s.schedule.ordinal(until) + 1
	if last-first > 2048 {
		return playoutcert.ProgrammeEvidence{}, errors.New("programme truth exceeds bound")
	}
	evidence := playoutcert.ProgrammeEvidence{}
	for index := first; index <= last; index++ {
		starts := s.schedule.epoch.Add(time.Duration(index) * s.schedule.duration)
		p := signatures[index%2].Programme(starts, starts.Add(s.schedule.duration))
		evidence.Programmes = append(evidence.Programmes, p)
	}
	var boundLiveClock playoutcert.ProgrammeMediaClock
	evidence.ResolveAsset = func(ctx context.Context, asset playoutcert.ProgrammeAsset) (playoutcert.ProgrammeAssetEvidence, error) {
		if !preparedChannel {
			proof, err := s.liveAsset(ctx, channel, asset)
			if err != nil {
				return playoutcert.ProgrammeAssetEvidence{}, err
			}
			clock := proof.Clock
			if boundLiveClock.Origin.IsZero() {
				boundLiveClock = clock
			}
			if !boundLiveClock.Origin.Equal(clock.Origin) || boundLiveClock.Generation != clock.Generation {
				return playoutcert.ProgrammeAssetEvidence{}, errors.New("source generation changed")
			}
			return proof, nil
		}
		// The independent truth was derived from the immutable publication before
		// observation. Reopening through the origin binds an opaque advertised asset
		// without teaching the certification adapter its token encoding.
		opened, ok, err := s.origin.OpenAsset(channel, playout.PlanBaseline, path.Base(asset.Reference))
		if err != nil || !ok {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared truth asset unavailable")
		}
		defer func() { _ = opened.Content.Close() }()
		data, err := io.ReadAll(io.LimitReader(opened.Content, (32<<20)+1))
		if err != nil || len(data) > 32<<20 || ctx.Err() != nil {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared truth asset invalid")
		}
		digest := sha256.Sum256(data)
		ordinal := s.schedule.ordinal(asset.StartedAt)
		truth, ok := s.assets[channel][ordinal%2][hex.EncodeToString(digest[:])]
		if !ok {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared truth asset changed")
		}
		starts := s.schedule.epoch.Add(time.Duration(ordinal) * s.schedule.duration)
		if !asset.StartedAt.Equal(starts.Add(truth.offset)) {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared asset schedule changed")
		}
		init, ok, err := s.origin.OpenAsset(channel, playout.PlanBaseline, path.Base(asset.InitReference))
		if err != nil || !ok {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared initialization unavailable")
		}
		initData, err := readSyntheticReference(ctx, init.Content)
		if err != nil || sha256.Sum256(initData) != truth.initDigest {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("prepared initialization changed")
		}
		return playoutcert.ProgrammeAssetEvidence{Clock: playoutcert.ProgrammeMediaClock{Origin: asset.StartedAt.Add(-truth.firstVideo), Generation: strconv.FormatInt(ordinal, 10)}, Media: data, Init: initData}, nil
	}
	return evidence, nil
}

// liveAsset opens the exact remux's file and source together, then freezes its
// bytes independently of the HTTP consumer. A later response must match them.
func (s *syntheticProgrammeEvidence) liveAsset(ctx context.Context, channel string, asset playoutcert.ProgrammeAsset) (playoutcert.ProgrammeAssetEvidence, error) {
	if s.live == nil {
		return playoutcert.ProgrammeAssetEvidence{}, errors.New("live asset source unavailable")
	}
	file, process, err := s.live.OpenAssetSource(channel, playout.PlanBaseline, path.Base(asset.Reference))
	if err != nil {
		return playoutcert.ProgrammeAssetEvidence{}, errors.New("live asset unavailable")
	}
	data, err := readSyntheticReference(ctx, file)
	if err != nil {
		return playoutcert.ProgrammeAssetEvidence{}, err
	}
	s.mu.Lock()
	state, ok := s.clocks[channel]
	s.mu.Unlock()
	if !ok || state.source != process || state.clock.Origin.IsZero() {
		return playoutcert.ProgrammeAssetEvidence{}, errors.New("live asset source changed")
	}
	proof := playoutcert.ProgrammeAssetEvidence{Clock: state.clock, Media: data}
	proof.Validate = func() error {
		s.mu.Lock()
		current, ok := s.clocks[channel]
		s.mu.Unlock()
		if ctx.Err() != nil || state.ctx.Err() != nil || !ok || current.source != process || current.clock.Generation != state.clock.Generation || !current.clock.Origin.Equal(state.clock.Origin) {
			return errors.New("live asset source retired")
		}
		return nil
	}
	if asset.InitReference != "" {
		init, initProcess, err := s.live.OpenAssetSource(channel, playout.PlanBaseline, path.Base(asset.InitReference))
		if err != nil {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("live initialization unavailable")
		}
		proof.Init, err = readSyntheticReference(ctx, init)
		if err != nil || initProcess != process {
			return playoutcert.ProgrammeAssetEvidence{}, errors.New("live initialization source changed")
		}
	}
	if err := proof.Validate(); err != nil {
		return playoutcert.ProgrammeAssetEvidence{}, err
	}
	return proof, nil
}

func readSyntheticReference(ctx context.Context, content io.ReadCloser) ([]byte, error) {
	defer func() { _ = content.Close() }()
	data, err := io.ReadAll(io.LimitReader(content, (32<<20)+1))
	if err != nil || ctx.Err() != nil || len(data) > 32<<20 {
		return nil, errors.New("invalid private asset reference")
	}
	return data, nil
}

// readCertificationPublicationTruth qualifies each immutable synthetic asset's own media clock.
// This runs during fixture preparation, before the target admits observations.
func readCertificationPublicationTruth(ctx context.Context, ffmpeg string, publication prepared.Publication) (map[string]syntheticAssetTruth, error) {
	assets := make(map[string]syntheticAssetTruth)
	manifest, err := os.ReadFile(filepath.Join(publication.Directory, prepared.MediaManifestName))
	if err != nil {
		return nil, err
	}
	init, err := os.ReadFile(filepath.Join(publication.Directory, "init.mp4"))
	if err != nil {
		return nil, err
	}
	var offset, duration time.Duration
	for _, line := range strings.Split(string(manifest), "\n") {
		if strings.HasPrefix(line, "#EXTINF:") {
			raw, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			duration, err = time.ParseDuration(raw + "s")
			if err != nil || duration <= 0 {
				return nil, errors.New("invalid synthetic asset duration")
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if filepath.Base(line) != line || duration <= 0 {
			return nil, errors.New("invalid synthetic asset reference")
		}
		data, err := os.ReadFile(filepath.Join(publication.Directory, line))
		if err != nil {
			return nil, err
		}
		probe := exec.CommandContext(ctx, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1", "-show_entries", "packet=pts_time", "-of", "json", "pipe:0")
		probe.Stdin = io.MultiReader(bytes.NewReader(init), bytes.NewReader(data))
		output, err := probe.Output()
		if err != nil {
			return nil, errors.New("synthetic asset clock probe failed")
		}
		var result struct {
			Packets []struct {
				PTS string `json:"pts_time"`
			} `json:"packets"`
		}
		if json.Unmarshal(output, &result) != nil || len(result.Packets) != 1 {
			return nil, errors.New("synthetic asset clock missing")
		}
		first, err := time.ParseDuration(result.Packets[0].PTS + "s")
		if err != nil {
			return nil, errors.New("synthetic asset clock invalid")
		}
		digest := sha256.Sum256(data)
		key := hex.EncodeToString(digest[:])
		truth := syntheticAssetTruth{offset: offset, firstVideo: first, initDigest: sha256.Sum256(init)}
		if previous, ok := assets[key]; ok && previous != truth {
			return nil, errors.New("ambiguous synthetic asset clock")
		}
		assets[key] = truth
		offset += duration
		duration = 0
	}
	return assets, nil
}
