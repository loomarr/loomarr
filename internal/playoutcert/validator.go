package playoutcert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type FFprobeValidator struct {
	Path string
}

type FFmpegDecoder struct {
	Path string
}

func (d FFmpegDecoder) FirstFrame(ctx context.Context, input io.Reader, maxBytes int) ([]byte, error) {
	path := strings.TrimSpace(d.Path)
	if path == "" {
		path = "ffmpeg"
	}
	var capture bytes.Buffer
	limited := io.LimitReader(input, int64(maxBytes))
	cmd := exec.CommandContext(ctx, path,
		"-hide_banner", "-loglevel", "error",
		"-probesize", "256k", "-analyzeduration", "500000",
		"-i", "pipe:0", "-map", "0:v:0", "-frames:v", "1", "-f", "null", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("ffmpeg decoder stdin unavailable")
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.New("ffmpeg decoder did not start")
	}
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(stdin, io.TeeReader(limited, &capture))
		_ = stdin.Close()
		close(copyDone)
	}()
	waitErr := cmd.Wait()
	<-copyDone
	if waitErr != nil {
		return capture.Bytes(), errors.New("ffmpeg did not decode a first video frame")
	}
	if capture.Len() == 0 {
		return nil, errors.New("decoder consumed no transport bytes")
	}
	return append([]byte(nil), capture.Bytes()...), nil
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
