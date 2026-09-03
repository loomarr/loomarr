package fillerreview

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

const temporalStructureWindowMotionPrefix = "lavfi.signalstats.YAVG="

type FFmpegTemporalStructureWindowMotionMeasurer struct {
	identity TemporalTruthToolIdentity
}

func NewFFmpegTemporalStructureWindowMotionMeasurer(ctx context.Context, ffmpegName string) (*FFmpegTemporalStructureWindowMotionMeasurer, error) {
	identity, err := temporalTruthExecutableIdentity(ctx, ffmpegName)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg motion identity: %w", err)
	}
	return &FFmpegTemporalStructureWindowMotionMeasurer{identity: identity}, nil
}

func (m *FFmpegTemporalStructureWindowMotionMeasurer) Identity() TemporalTruthToolIdentity {
	if m == nil {
		return TemporalTruthToolIdentity{}
	}
	return m.identity
}

// Measure computes the mean absolute luma delta between every pair of adjacent decoded frames.
// It streams metadata rather than buffering one line per frame in memory.
func (m *FFmpegTemporalStructureWindowMotionMeasurer) Measure(ctx context.Context, path string) (TemporalStructureWindowMotionSample, error) {
	if m == nil || strings.TrimSpace(m.identity.Path) == "" || strings.TrimSpace(path) == "" {
		return TemporalStructureWindowMotionSample{}, errors.New("window motion measurer is unavailable")
	}
	command := exec.CommandContext(ctx, m.identity.Path,
		"-nostdin", "-hide_banner", "-v", "error", "-i", path,
		"-map", "0:v:0", "-vf", "tblend=all_mode=difference,signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-",
		"-an", "-threads", "1", "-filter_threads", "1", "-filter_complex_threads", "1", "-f", "null", "-",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return TemporalStructureWindowMotionSample{}, err
	}
	commandOutput := &boundedTemporalStructureMediaOutput{}
	command.Stderr = commandOutput
	if err := command.Start(); err != nil {
		return TemporalStructureWindowMotionSample{}, err
	}
	values := make([]int64, 0, 4_500)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "frame:"):
			continue
		case strings.HasPrefix(line, temporalStructureWindowMotionPrefix):
			value, parseErr := parseTemporalStructureWindowMicroluma(strings.TrimPrefix(line, temporalStructureWindowMotionPrefix))
			if parseErr != nil {
				_ = command.Process.Kill()
				_ = command.Wait()
				return TemporalStructureWindowMotionSample{}, parseErr
			}
			values = append(values, value)
		case line != "":
			_ = command.Process.Kill()
			_ = command.Wait()
			return TemporalStructureWindowMotionSample{}, fmt.Errorf("ffmpeg motion emitted unexpected metadata %q", line)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return TemporalStructureWindowMotionSample{}, err
	}
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return TemporalStructureWindowMotionSample{}, ctxErr
	}
	if commandOutput.overflowed() {
		return TemporalStructureWindowMotionSample{}, errors.New("ffmpeg motion output exceeded 256 KiB")
	}
	if waitErr != nil {
		return TemporalStructureWindowMotionSample{}, fmt.Errorf("ffmpeg motion decode: %w: %s", waitErr, commandOutput.message())
	}
	if commandOutput.message() != "" {
		return TemporalStructureWindowMotionSample{}, fmt.Errorf("ffmpeg motion reported errors despite a successful exit: %s", commandOutput.message())
	}
	if len(values) == 0 {
		return TemporalStructureWindowMotionSample{}, errors.New("ffmpeg motion produced no adjacent-frame measurements")
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	var sum int64
	for _, value := range values {
		if sum > int64(^uint64(0)>>1)-value {
			return TemporalStructureWindowMotionSample{}, errors.New("ffmpeg motion measurement overflow")
		}
		sum += value
	}
	p95Index := (95*len(sorted)+99)/100 - 1
	return TemporalStructureWindowMotionSample{
		Frames: int64(len(values)), SumMicroluma: sum,
		P95Microluma: sorted[p95Index], MaximumMicroluma: sorted[len(sorted)-1],
	}, nil
}

func parseTemporalStructureWindowMicroluma(value string) (int64, error) {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return 0, errors.New("ffmpeg motion emitted invalid luma delta")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("ffmpeg motion emitted invalid luma delta")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > 255 {
		return 0, errors.New("ffmpeg motion emitted invalid luma delta")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	for _, char := range fraction {
		if char < '0' || char > '9' {
			return 0, errors.New("ffmpeg motion emitted invalid luma delta")
		}
	}
	padded := fraction + "0000000"
	micro, err := strconv.ParseInt(padded[:6], 10, 64)
	if err != nil {
		return 0, errors.New("ffmpeg motion emitted invalid luma delta")
	}
	if len(fraction) > 6 && padded[6] >= '5' {
		micro++
	}
	result := whole*TemporalStructureWindowMotionScale + micro
	if result > 255*TemporalStructureWindowMotionScale {
		return 0, errors.New("ffmpeg motion emitted invalid luma delta")
	}
	return result, nil
}

var _ TemporalStructureWindowMotionMeasurer = (*FFmpegTemporalStructureWindowMotionMeasurer)(nil)
