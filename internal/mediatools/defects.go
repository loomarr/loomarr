package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/bgexec"
	"github.com/loomarr/loomarr/internal/defects"
)

// MeasureDefects reads a clip's picture and sound defects in one decode (#2034). width and height
// are the probed frame size; hasAudio says whether the file has a sound stream.
//
// ⚠ **A pass of its own rather than a branch of the playback transcode**, for two reasons. Re-checks
// (existing clips, a new MeasurementVersion) have no transcode to ride. And cropdetect logs every
// sampled frame, which in the transcode's bounded log would push out the black and freeze evidence
// the publish gate reads. Decoding a 30 s advert again costs about a second.
func MeasureDefects(ctx context.Context, ffmpegPath, file string, width, height int, hasAudio bool) (defects.Readings, error) {
	if width <= 0 || height <= 0 {
		return defects.Readings{}, fmt.Errorf("measure defects %s: no video frame size", filepath.Base(file))
	}
	dir, err := os.MkdirTemp("", "loomarr-defects-")
	if err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	metadataFile := filepath.Join(dir, "frames.txt")
	if strings.ContainsAny(metadataFile, `'\`) {
		return defects.Readings{}, fmt.Errorf("measure defects: temporary path %q cannot be quoted in a filter graph", metadataFile)
	}

	args := []string{"-nostdin", "-threads", strconv.Itoa(BackgroundThreads), "-hide_banner", "-nostats", "-v", "info",
		"-i", file, "-filter_threads", strconv.Itoa(BackgroundThreads),
		"-filter_complex", defectsFilterGraph(metadataFile, width, height, hasAudio), "-map", "[defects]", "-f", "null", "-"}
	var stderr bytes.Buffer
	cmd := bgexec.FFmpeg(ctx, FFmpegOr(ffmpegPath), args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w: %s", filepath.Base(file), err, tail(stderr.String(), 2048))
	}
	frames, err := os.ReadFile(metadataFile)
	if err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	return defects.Parse(stderr.String(), string(frames), width, height, hasAudio), nil
}

// defectsFilterGraph samples the picture at defects.SampleFPS for blocks, blur, tape outliers,
// the active area and the upscale score, reads every frame for interlace, and measures the sound
// as 16-bit samples so a clipped track flattens at full scale.
func defectsFilterGraph(metadataFile string, width, height int, hasAudio bool) string {
	graph := fmt.Sprintf("[0:v:0]format=yuv420p,split=2[sampled][fields];"+
		"[sampled]fps=%d,split=2[detect][detail];"+
		"[detect]blockdetect,blurdetect,signalstats=stat=tout+vrep,cropdetect=round=2:reset=0,"+
		"metadata=print:file='%s'[defects];"+
		// Halve the frame and enlarge it back to its own size: what survives is the detail the
		// frame actually has.
		"[detail]split=2[shrink][reference];"+
		"[shrink]scale=iw/2:ih/2:flags=area,scale=%d:%d:flags=bicubic[restored];"+
		"[restored][reference]ssim,nullsink;"+
		"[fields]idet,nullsink",
		defects.SampleFPS, metadataFile, width, height)
	if hasAudio {
		graph += ";[0:a:0]aformat=sample_fmts=s16,astats=measure_overall=Peak_level+Peak_count+Flat_factor+Noise_floor:measure_perchannel=none,anullsink"
	}
	return graph
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
