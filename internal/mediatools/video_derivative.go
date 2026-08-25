package mediatools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
)

const (
	HostedVideoMaxDurationMS      = int64(60_000)
	HostedVideoMaxBytes           = 12 << 20
	HostedVideoMaxDiagnosticBytes = 64 << 10
)

// HostedVideoDeriver is the narrow extraction seam used by certification and,
// later, the evidence router. It does not widen MediaTools for callers that only
// need ordinary ingest operations.
type HostedVideoDeriver interface {
	HostedVideoIn(ctx context.Context, file string, startMS, endMS int64) (HostedVideoDerivative, error)
}

// HostedVideoDerivative is the bounded, provenance-bearing MP4 supplied to a
// direct-video evidence provider (§10 V61).
type HostedVideoDerivative struct {
	MP4      []byte
	MIMEType string
	SHA256   string
	StartMS  int64
	EndMS    int64
}

// HostedVideoIn creates one metadata-free derivative of an already measured
// span. A longer span must be narrowed deliberately by the evidence router; this
// method never silently chooses which evidence to omit.
func (t *FFmpegTools) HostedVideoIn(ctx context.Context, file string, startMS, endMS int64) (HostedVideoDerivative, error) {
	if startMS < 0 || endMS <= startMS || endMS-startMS > HostedVideoMaxDurationMS {
		return HostedVideoDerivative{}, fmt.Errorf("hosted video requires a measured span of at most %dms, got %d..%d for %s", HostedVideoMaxDurationMS, startMS, endMS, file)
	}

	var stdout bytes.Buffer
	stderr := cappedDiagnostic{remaining: HostedVideoMaxDiagnosticBytes}
	cmd := exec.CommandContext(ctx, FFmpegOr(t.FFmpegPath),
		"-nostdin", "-hide_banner", "-nostats", "-v", "error",
		"-ss", msToSeconds(startMS), "-t", msToSeconds(endMS-startMS),
		"-i", file,
		"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn",
		"-map_metadata", "-1", "-map_chapters", "-1",
		"-vf", "scale='min(1280,iw)':'min(720,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k", "-threads", "1",
		"-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-")
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return HostedVideoDerivative{}, fmt.Errorf("open hosted video output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return HostedVideoDerivative{}, fmt.Errorf("start hosted video encoder: %w", err)
	}
	_, copyErr := io.Copy(&stdout, io.LimitReader(pipe, HostedVideoMaxBytes+1))
	if stdout.Len() > HostedVideoMaxBytes {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return HostedVideoDerivative{}, fmt.Errorf("hosted video exceeded 12 MiB (%d-byte) ceiling", HostedVideoMaxBytes)
	}
	if copyErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return HostedVideoDerivative{}, fmt.Errorf("read hosted video output: %w", copyErr)
	}
	if err := cmd.Wait(); err != nil {
		return HostedVideoDerivative{}, fmt.Errorf("ffmpeg hosted video %s at %d..%d: %w: %s", file, startMS, endMS, err, stderr.String())
	}
	data := append([]byte(nil), stdout.Bytes()...)
	if len(data) == 0 {
		return HostedVideoDerivative{}, fmt.Errorf("ffmpeg hosted video %s produced no media", file)
	}
	digest := sha256.Sum256(data)
	return HostedVideoDerivative{
		MP4: data, MIMEType: "video/mp4", SHA256: fmt.Sprintf("%x", digest),
		StartMS: startMS, EndMS: endMS,
	}, nil
}

var _ HostedVideoDeriver = (*FFmpegTools)(nil)

type cappedDiagnostic struct {
	buf       bytes.Buffer
	remaining int
}

func (b *cappedDiagnostic) Write(p []byte) (int, error) {
	written := len(p)
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	if len(p) > 0 {
		_, _ = b.buf.Write(p)
		b.remaining -= len(p)
	}
	return written, nil
}

func (b *cappedDiagnostic) String() string { return b.buf.String() }
