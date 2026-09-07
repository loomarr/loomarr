package playoutcert

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type FFprobeValidator struct {
	Path string
}

type FFmpegDecoder struct {
	Path    string
	command func(context.Context, string, ...string) *exec.Cmd
}

func (d FFmpegDecoder) Decode(ctx context.Context, input io.ReadCloser, reportFrames func(int64)) error {
	path := strings.TrimSpace(d.Path)
	if path == "" {
		path = "ffmpeg"
	}
	command := d.command
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(ctx, path,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-stats_period", "0.05",
		"-probesize", "256k", "-analyzeduration", "500000",
		"-i", "pipe:0", "-map", "0:v:0", "-f", "null", "-",
		"-progress", "pipe:1", "-nostats")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = input.Close()
		return errors.New("ffmpeg decoder stdin unavailable")
	}
	progress, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return errors.New("ffmpeg decoder progress unavailable")
	}
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		return errors.New("ffmpeg decoder did not start")
	}
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdin, input)
		if closeErr := stdin.Close(); copyErr == nil {
			copyErr = closeErr
		}
		copyDone <- copyErr
	}()
	progressDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(progress)
		var previous int64
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "frame=") {
				continue
			}
			frames, parseErr := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "frame=")), 10, 64)
			if parseErr == nil && frames > previous {
				previous = frames
				reportFrames(frames)
			}
		}
		progressDone <- scanner.Err()
	}()
	// StdoutPipe requires its reader to reach EOF before Wait closes the pipe.
	// The child closing progress at exit supplies that EOF; only then may Wait
	// reap the process.
	progressErr := <-progressDone
	waitErr := cmd.Wait()
	_ = input.Close()
	copyErr := <-copyDone
	if waitErr != nil || copyErr != nil || progressErr != nil {
		return errors.New("ffmpeg decoder failed")
	}
	return nil
}

func (v FFprobeValidator) Validate(ctx context.Context, body []byte) (MediaShape, error) {
	if len(body) == 0 {
		return MediaShape{}, errors.New("media capture is empty")
	}
	path := strings.TrimSpace(v.Path)
	if path == "" {
		path = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, path,
		"-v", "error", "-show_entries", "stream=codec_type,codec_name", "-of", "json", "pipe:0")
	cmd.Stdin = bytes.NewReader(body)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return MediaShape{}, errors.New("ffprobe rejected the bounded media capture")
	}
	return parseProbe(output.Bytes())
}

func parseProbe(raw []byte) (MediaShape, error) {
	var document struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return MediaShape{}, errors.New("ffprobe output is not JSON")
	}
	shape := MediaShape{}
	for _, stream := range document.Streams {
		switch stream.CodecType {
		case "video":
			shape.VideoStreams++
			if shape.VideoCodec == "" {
				shape.VideoCodec = stream.CodecName
			}
		case "audio":
			shape.AudioStreams++
			if shape.AudioCodec == "" {
				shape.AudioCodec = stream.CodecName
			}
		}
	}
	if shape.VideoStreams != 1 || shape.AudioStreams != 1 {
		return shape, fmt.Errorf("media stream shape is video=%d audio=%d", shape.VideoStreams, shape.AudioStreams)
	}
	if !safeIdentity(shape.VideoCodec, 32, false) || !safeIdentity(shape.AudioCodec, 32, false) {
		return MediaShape{}, errors.New("ffprobe returned an invalid codec identity")
	}
	return shape, nil
}
