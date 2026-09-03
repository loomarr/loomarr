package fillersafetyreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/fillersafety"
	"github.com/loomarr/loomarr/internal/mediatools"
)

const (
	maximumToolBytes       = int64(512 << 20)
	maximumToolOutputBytes = 64 << 10
)

type audioExtractor interface {
	Extract(context.Context, string, *fillersafety.CompleteMediaPlan, int64) ([]byte, error)
}

type ffmpegAudioExtractor struct{}

func (ffmpegAudioExtractor) Extract(
	ctx context.Context,
	ffmpegPath string,
	plan *fillersafety.CompleteMediaPlan,
	maximumBytes int64,
) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || plan == nil || maximumBytes < 12 {
		return nil, fmt.Errorf("model review audio extraction input is invalid")
	}
	dir, err := os.MkdirTemp("", "loomarr-spoken-review-audio-")
	if err != nil {
		return nil, fmt.Errorf("create private model review audio workspace")
	}
	defer func() { _ = os.RemoveAll(dir) }()
	destination := filepath.Join(dir, "complete.wav")
	if err := mediatools.ExtractSpanWAV(ctx, ffmpegPath, plan.SourcePath, 0, plan.Audio.EndMS, destination); err != nil {
		return nil, fmt.Errorf("extract complete model review audio")
	}
	info, err := os.Lstat(destination)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 12 || info.Size() > maximumBytes {
		return nil, fmt.Errorf("model review audio exceeds its byte ceiling")
	}
	file, err := os.Open(destination)
	if err != nil {
		return nil, fmt.Errorf("open model review audio")
	}
	defer func() { _ = file.Close() }()
	wav, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil || len(wav) < 12 || int64(len(wav)) > maximumBytes ||
		string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, fmt.Errorf("model review audio is invalid")
	}
	return wav, nil
}

func identifyFFmpeg(ctx context.Context, value string) (fillersafety.ToolIdentity, string, error) {
	path, err := exec.LookPath(value)
	if err != nil {
		return fillersafety.ToolIdentity{}, "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fillersafety.ToolIdentity{}, "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return fillersafety.ToolIdentity{}, "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumToolBytes {
		return fillersafety.ToolIdentity{}, "", fmt.Errorf("model review ffmpeg must resolve to a bounded regular file")
	}
	digest, err := hashTool(path, info)
	if err != nil {
		return fillersafety.ToolIdentity{}, "", fmt.Errorf("read model review ffmpeg identity")
	}
	command := exec.CommandContext(ctx, path, "-version") //nolint:gosec // operator-selected resolved regular executable
	var stdout, stderr boundedToolBuffer
	stdout.remaining, stderr.remaining = maximumToolOutputBytes, maximumToolOutputBytes
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil || stdout.overflow || stderr.overflow {
		return fillersafety.ToolIdentity{}, "", fmt.Errorf("model review ffmpeg version probe failed")
	}
	fields := strings.Fields(strings.SplitN(stdout.String(), "\n", 2)[0])
	if len(fields) < 3 {
		return fillersafety.ToolIdentity{}, "", fmt.Errorf("model review ffmpeg version is malformed")
	}
	version := strings.Join(fields[:3], " ")
	if len(version) > 128 {
		return fillersafety.ToolIdentity{}, "", fmt.Errorf("model review ffmpeg version is too long")
	}
	return fillersafety.ToolIdentity{Version: version, BinarySHA256: digest}, path, nil
}

func hashTool(path string, expected os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return "", fmt.Errorf("model review ffmpeg identity changed while opening")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maximumToolBytes+1))
	if err != nil || written != expected.Size() {
		return "", fmt.Errorf("hash model review ffmpeg")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type boundedToolBuffer struct {
	buffer    bytes.Buffer
	remaining int
	overflow  bool
}

func (buffer *boundedToolBuffer) Write(value []byte) (int, error) {
	written := len(value)
	if len(value) > buffer.remaining {
		value = value[:buffer.remaining]
		buffer.overflow = true
	}
	if len(value) != 0 {
		_, _ = buffer.buffer.Write(value)
		buffer.remaining -= len(value)
	}
	return written, nil
}

func (buffer *boundedToolBuffer) String() string { return buffer.buffer.String() }
