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
	cropFile := filepath.Join(dir, "crop.txt")
	metadataFile := filepath.Join(dir, "frames.txt")
	if strings.ContainsAny(dir, `'\`) {
		return defects.Readings{}, fmt.Errorf("measure defects: temporary path %q cannot be quoted in a filter graph", dir)
	}

	// ⚠ -noautorotate: ffprobe reports the coded frame size and the decoder would otherwise turn a
	// rotated clip upright, so the upscale branch scaled back to the probed size and the windowbox
	// share measured against it would both be the wrong shape. A defect does not depend on which
	// way up the picture is.
	//
	// ⚠ Two passes. The first finds the active picture; the second measures only inside it.
	// Black borders aligned to the coding blocks read as block edges to blockdetect (a 5% windowbox
	// scored 36 against a clean 13, a 25% one 218), and letterboxed widescreen is common in SD-era
	// ads, so measuring the whole frame failed good clips for their bars.
	if _, err := runDefectsPass(ctx, ffmpegPath, file,
		fmt.Sprintf("[0:v:0]fps=2,cropdetect=round=2:reset=0,metadata=print:file='%s'[crop]", cropFile), "[crop]"); err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	cropMeta, err := os.ReadFile(cropFile)
	if err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	active := defects.ParseActiveArea(string(cropMeta), width, height)
	log, err := runDefectsPass(ctx, ffmpegPath, file, defectsFilterGraph(metadataFile, active, hasAudio), "[defects]")
	if err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	frames, err := os.ReadFile(metadataFile)
	if err != nil {
		return defects.Readings{}, fmt.Errorf("measure defects %s: %w", filepath.Base(file), err)
	}
	return defects.Parse(log, string(frames), width, height, active, hasAudio), nil
}

// runDefectsPass decodes file through graph, discarding the output, and returns ffmpeg's log.
func runDefectsPass(ctx context.Context, ffmpegPath, file, graph, output string) (string, error) {
	args := []string{"-nostdin", "-threads", strconv.Itoa(BackgroundThreads), "-hide_banner", "-nostats", "-v", "info",
		"-noautorotate", "-i", file, "-filter_threads", strconv.Itoa(BackgroundThreads),
		"-filter_complex", graph, "-map", output, "-f", "null", "-"}
	var stderr bytes.Buffer
	cmd := bgexec.FFmpeg(ctx, FFmpegOr(ffmpegPath), args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, tail(stderr.String(), 2048))
	}
	return stderr.String(), nil
}

// defectsFilterGraph crops to the active picture, samples it at defects.SampleFPS for blocks, blur,
// tape outliers and two upscale scores, reads every frame for interlace, and measures the sound as
// 16-bit samples so a clipped track flattens at full scale.
func defectsFilterGraph(metadataFile string, active defects.ActiveArea, hasAudio bool) string {
	graph := fmt.Sprintf("[0:v:0]crop=%d:%d:%d:%d,format=yuv420p,split=2[sampled][fields];"+
		"[sampled]fps=%d,split=2[detect][detail];"+
		"[detect]blockdetect,blurdetect,signalstats=stat=tout+vrep,metadata=print:file='%s'[defects];"+
		// Shrink the picture and enlarge it back to its own size: what survives is the detail it
		// actually has. By half for the HD era's upscale check; by a quarter for older eras, where
		// a good tape copy already has no detail at half size but a tiny source enlarged has none
		// at a quarter. ⚠ The 2x branch is first: Parse tells the two ssim summaries apart by order.
		"[detail]split=4[shrink2][reference2][shrink4][reference4];"+
		"[shrink2]scale=iw/2:ih/2:flags=area,scale=%[1]d:%[2]d:flags=bicubic[restored2];"+
		"[restored2][reference2]ssim,nullsink;"+
		"[shrink4]scale=iw/4:ih/4:flags=area,scale=%[1]d:%[2]d:flags=bicubic[restored4];"+
		"[restored4][reference4]ssim,nullsink;"+
		"[fields]idet,nullsink",
		active.Width, active.Height, active.X, active.Y, defects.SampleFPS, metadataFile)
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
