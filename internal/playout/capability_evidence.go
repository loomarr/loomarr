package playout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

const (
	capabilityEvidenceVersion = 1
	// capabilityEvidenceName and capabilityEvidenceTempPrefix keep the file names the evidence has
	// always had, so an install that already measured its host reads the same file.
	capabilityEvidenceName       = ".host-capability-v1.json"
	capabilityEvidenceTempPrefix = ".host-capability-"
	capabilityEvidenceMaxAge     = 7 * 24 * time.Hour
	capabilityValidationSecs     = 1
	capabilityIdentityTimeout    = 250 * time.Millisecond
)

type capabilityEvidence struct {
	Version     int            `json:"version"`
	Fingerprint string         `json:"fingerprint"`
	Encoder     encode.Encoder `json:"encoder"`
	MaxChannels int            `json:"maxChannels"`
	ObservedAt  time.Time      `json:"observedAt"`
}

type capabilityEvidenceDependencies struct {
	now         func() time.Time
	fingerprint func(context.Context, string, string, Profile) (string, error)
	detect      func(context.Context, string, Profile, string) Capacity
	validate    func(context.Context, string, encode.Encoder, Profile) Capability
	// log reports what stops the evidence from being kept or reused; nil is silent.
	log *slog.Logger
}

// warn logs a failure that sends the host back to a full measurement at every start.
func (d capabilityEvidenceDependencies) warn(msg string, args ...any) {
	if d.log != nil {
		d.log.Warn(msg, args...)
	}
}

// DetectObservedWithEvidence reuses a persisted hardware measurement only after a short real
// encoder trial proves it still emits keyframe-bearing MPEG-TS on the current FFmpeg/GPU host.
// reused distinguishes that bounded restart validation from a full candidate/capacity benchmark.
func DetectObservedWithEvidence(
	ctx context.Context, ffmpegPath string, profile Profile, gpu, root string,
	manager *diagnostics.ProcessManager, log *slog.Logger,
) (capacity Capacity, reused bool) {
	return detectObservedWithEvidence(ctx, ffmpegPath, profile, gpu, root, capabilityEvidenceDependencies{
		now:         time.Now,
		log:         log,
		fingerprint: hostCapabilityFingerprint,
		detect: func(ctx context.Context, ffmpeg string, profile Profile, gpu string) Capacity {
			return DetectObserved(ctx, ffmpeg, profile, gpu, manager)
		},
		validate: func(ctx context.Context, ffmpeg string, encoder encode.Encoder, profile Profile) Capability {
			return trialEncodeObserved(ctx, ffmpeg, encoder, profile, capabilityValidationSecs, manager)
		},
	})
}

// LoadMatchingObservedCapabilityEvidence performs only the cheap restart identity check. It never
// launches an encoder trial or the full candidate/capacity benchmark. Callers may therefore use a
// still-fresh measurement immediately when the FFmpeg/GPU/profile fingerprint is unchanged, while
// DetectObservedWithEvidence revalidates that measurement asynchronously.
func LoadMatchingObservedCapabilityEvidence(
	ctx context.Context, ffmpegPath string, profile Profile, gpu, root string, log *slog.Logger,
) (Capacity, bool) {
	capacity, _, ok := matchingCapabilityEvidence(ctx, ffmpegPath, profile, gpu, root, capabilityEvidenceDependencies{
		now:         time.Now,
		log:         log,
		fingerprint: hostCapabilityFingerprint,
	})
	return capacity, ok
}

func detectObservedWithEvidence(
	ctx context.Context, ffmpegPath string, profile Profile, gpu, root string,
	deps capabilityEvidenceDependencies,
) (Capacity, bool) {
	if deps.now == nil {
		deps.now = time.Now
	}
	validated, fingerprint, ok := validatedCapabilityEvidence(ctx, ffmpegPath, profile, gpu, root, deps)
	if ok {
		return validated, true
	}

	capacity := deps.detect(ctx, ffmpegPath, profile, gpu)
	// A cancelled run measured under a dying context; storing it would make the next start trust it.
	// No root is an install that keeps no evidence, not a failure to keep it.
	if ctx.Err() == nil && fingerprint != "" && strings.TrimSpace(root) != "" &&
		!encode.IsSoftware(capacity.Chosen) && capacity.MaxChannels > 0 {
		if err := storeCapabilityEvidence(root, capabilityEvidence{
			Version: capabilityEvidenceVersion, Fingerprint: fingerprint,
			Encoder: capacity.Chosen, MaxChannels: capacity.MaxChannels, ObservedAt: deps.now().UTC(),
		}); err != nil {
			deps.warn("playout: could not store the host's encoder measurement; it will be measured again at every start",
				"root", root, "err", err)
		}
	}
	return capacity, false
}

func validatedCapabilityEvidence(
	ctx context.Context, ffmpegPath string, profile Profile, gpu, root string,
	deps capabilityEvidenceDependencies,
) (Capacity, string, bool) {
	capacity, fingerprint, ok := matchingCapabilityEvidence(ctx, ffmpegPath, profile, gpu, root, deps)
	if !ok || deps.validate == nil {
		return Capacity{}, fingerprint, false
	}
	validated := deps.validate(ctx, ffmpegPath, capacity.Chosen, profile)
	if !validated.Works {
		return Capacity{}, fingerprint, false
	}
	// The validation trial is a real encode of the chosen encoder, so its peak RSS is a current
	// per-encode host-memory measurement; nothing about memory is persisted with the evidence.
	capacity.EncodeHostBytes = validated.PeakRSSBytes
	return capacity, fingerprint, true
}

func matchingCapabilityEvidence(
	ctx context.Context, ffmpegPath string, profile Profile, gpu, root string,
	deps capabilityEvidenceDependencies,
) (Capacity, string, bool) {
	if deps.now == nil {
		deps.now = time.Now
	}
	fingerprint, err := deps.fingerprint(ctx, ffmpegPath, gpu, profile)
	if err != nil {
		deps.warn("playout: could not identify the host to reuse its encoder measurement; it will be measured again at every start",
			"err", err)
		return Capacity{}, "", false
	}
	evidence, ok := loadCapabilityEvidence(root, fingerprint, deps.now(), deps.warn)
	if !ok {
		return Capacity{}, fingerprint, false
	}
	return Capacity{Chosen: evidence.Encoder, MaxChannels: evidence.MaxChannels}, fingerprint, true
}

// loadCapabilityEvidence reads the stored measurement and reports whether it is one this host may
// reuse. No file, a file from another host, an old one or one an older build wrote are the ordinary
// reasons to measure afresh and are silent; a file that cannot be read or parsed is not, and goes
// to warn.
func loadCapabilityEvidence(
	root, fingerprint string, now time.Time, warn func(msg string, args ...any),
) (capabilityEvidence, bool) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(fingerprint) == "" {
		return capabilityEvidence{}, false
	}
	path := filepath.Join(root, capabilityEvidenceName)
	body, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			warn("playout: could not read the host's stored encoder measurement; it will be measured again",
				"path", path, "err", err)
		}
		return capabilityEvidence{}, false
	}
	var evidence capabilityEvidence
	if err := json.Unmarshal(body, &evidence); err != nil {
		warn("playout: could not read the host's stored encoder measurement; it will be measured again",
			"path", path, "err", err)
		return capabilityEvidence{}, false
	}
	if evidence.Version != capabilityEvidenceVersion ||
		evidence.Fingerprint != fingerprint || evidence.ObservedAt.IsZero() || now.Before(evidence.ObservedAt) ||
		now.Sub(evidence.ObservedAt) > capabilityEvidenceMaxAge || evidence.MaxChannels <= 0 ||
		encode.IsSoftware(evidence.Encoder) || !supportedHardwareEncoder(evidence.Encoder) {
		return capabilityEvidence{}, false
	}
	return evidence, true
}

func storeCapabilityEvidence(root string, evidence capabilityEvidence) error {
	return writeEvidence(root, capabilityEvidenceName, evidence)
}

// writeEvidence publishes one host-measurement file atomically: temp file, fsync, rename, dir fsync.
func writeEvidence(root, name string, v any) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("playout: empty capability evidence root")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return fmt.Errorf("playout: create capability evidence root: %w", err)
	}
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("playout: encode capability evidence: %w", err)
	}
	tmp, err := os.CreateTemp(root, capabilityEvidenceTempPrefix+"*")
	if err != nil {
		return fmt.Errorf("playout: create capability evidence: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(root, name)); err != nil {
		return fmt.Errorf("playout: publish capability evidence: %w", err)
	}
	committed = true
	directory, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("playout: open capability evidence root: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("playout: sync capability evidence root: %w", err)
	}
	return nil
}

func supportedHardwareEncoder(encoder encode.Encoder) bool {
	for _, candidate := range encode.H264Engines {
		if candidate != encode.EncoderSoftware && candidate == encoder {
			return true
		}
	}
	return false
}

func hostCapabilityFingerprint(
	ctx context.Context, ffmpegPath, gpu string, profile Profile,
) (string, error) {
	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	resolved, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	// This command runs on the first live fallback after restart. A wedged executable must turn
	// evidence reuse into a quick miss, never become the cold-start latency itself.
	versionCtx, cancel := context.WithTimeout(ctx, capabilityIdentityTimeout)
	defer cancel()
	version, err := exec.CommandContext(versionCtx, resolved, "-version").Output()
	if err != nil {
		return "", err
	}
	if len(version) > 64<<10 {
		version = version[:64<<10]
	}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00",
		runtime.GOOS, runtime.GOARCH, resolved, info.Size(), info.ModTime().UnixNano(), info.Mode(),
		strings.TrimSpace(gpu), profile.Width, profile.Height, profile.Framerate,
		profile.VideoBitrate, profile.AudioBitrate)
	_, _ = digest.Write(version)
	return hex.EncodeToString(digest.Sum(nil)), nil
}
