package playoutbench

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/playout"
)

// The MPEG-TS clock is 90 kHz; one AAC-LC frame is 1024 samples at 48 kHz.
const (
	tsClock         = 90000
	aacFrameTicks   = 1024 * tsClock / 48000 // 1920
	gridToleranceTk = 1
)

// analysis is what one clip's encoded output must satisfy at a commercial-break boundary.
type analysis struct {
	clip           Clip
	firstPacketMs  float64
	ptsGaps        int
	audioOffGrid   int
	spsHex         string
	loudnessDevLU  float64
	avEndOffsetMs  float64
	videoFrameRate int
}

// offGrid counts consecutive deltas that are not exactly one step (± the tick tolerance): a gap, a
// duplicate or a torn timestamp. pts must be sorted.
func offGrid(pts []int64, step int64) int {
	n := 0
	for i := 1; i < len(pts); i++ {
		if d := pts[i] - pts[i-1]; d < step-gridToleranceTk || d > step+gridToleranceTk {
			n++
		}
	}
	return n
}

// spsNAL returns the first H.264 sequence parameter set (NAL type 7) in an Annex-B stream, without
// the start code, or nil.
func spsNAL(stream []byte) []byte {
	for i := 0; i+3 < len(stream); i++ {
		if stream[i] != 0 || stream[i+1] != 0 || stream[i+2] != 1 {
			continue
		}
		if stream[i+3]&0x1f != 7 {
			continue
		}
		rest := stream[i+3:]
		end := len(rest)
		for j := 0; j+2 < len(rest); j++ {
			if rest[j] == 0 && rest[j+1] == 0 && (rest[j+2] == 1 || (rest[j+2] == 0 && j+3 < len(rest) && rest[j+3] == 1)) {
				end = j
				break
			}
		}
		return rest[:end]
	}
	return nil
}

var integratedLoudness = regexp.MustCompile(`(?s)Integrated loudness:\s*I:\s*(-?[0-9.]+|-inf) LUFS`)

// parseIntegratedLoudness reads the last ebur128 summary from ffmpeg's log.
func parseIntegratedLoudness(log string) (float64, error) {
	all := integratedLoudness.FindAllStringSubmatch(log, -1)
	if len(all) == 0 {
		return 0, fmt.Errorf("no ebur128 summary in ffmpeg output")
	}
	v, err := strconv.ParseFloat(all[len(all)-1][1], 64)
	if err != nil || math.IsInf(v, 0) {
		return 0, fmt.Errorf("integrated loudness %q is not a number (silent output?)", all[len(all)-1][1])
	}
	return v, nil
}

// packetPTS returns the sorted PTS values of one stream type of a TS file.
func (r *run) packetPTS(ctx context.Context, path, stream string) ([]int64, error) {
	out, err := execOutput(ctx, r.FFprobe, "-v", "error", "-select_streams", stream, "-show_entries", "packet=pts", "-of", "csv=p=0", path)
	if err != nil {
		return nil, err
	}
	var pts []int64
	for _, line := range strings.Fields(out) {
		v, err := strconv.ParseInt(strings.TrimSuffix(line, ","), 10, 64)
		if err != nil {
			continue // "N/A"
		}
		pts = append(pts, v)
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i] < pts[j] })
	return pts, nil
}

// analyse encodes the clip through the real pipeline and checks the output the way a break boundary
// would: timestamps on the frame grid, audio on the AAC frame grid, SPS bytes, loudness.
func (r *run) analyse(ctx context.Context, c Clip, pipe playout.Pipeline) (analysis, error) {
	ts := filepath.Join(r.Dir, c.Name+".out.ts")
	defer func() { _ = os.Remove(ts) }()
	t, err := r.encodeTo(ctx, c, pipe, ts)
	if err != nil {
		return analysis{}, err
	}
	a := analysis{clip: c, firstPacketMs: float64(t.firstByte.Microseconds()) / 1000, videoFrameRate: r.out.FPS}

	vpts, err := r.packetPTS(ctx, ts, "v:0")
	if err != nil || len(vpts) < 2 {
		return a, fmt.Errorf("video packets: %v (%d found)", err, len(vpts))
	}
	apts, err := r.packetPTS(ctx, ts, "a:0")
	if err != nil || len(apts) < 2 {
		return a, fmt.Errorf("audio packets: %v (%d found)", err, len(apts))
	}
	frame := int64(tsClock / r.out.FPS)
	a.ptsGaps = offGrid(vpts, frame)
	a.audioOffGrid = offGrid(apts, aacFrameTicks)
	vEnd, aEnd := vpts[len(vpts)-1]+frame, apts[len(apts)-1]+aacFrameTicks
	a.avEndOffsetMs = math.Abs(float64(vEnd-aEnd)) / tsClock * 1000

	annexB, err := execOutput(ctx, r.FFmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-i", ts, "-map", "0:v:0", "-c:v", "copy", "-frames:v", "1", "-f", "h264", "pipe:1")
	if err != nil {
		return a, fmt.Errorf("sps: %w", err)
	}
	sps := spsNAL([]byte(annexB))
	if sps == nil {
		return a, fmt.Errorf("no SPS in the encoded output")
	}
	a.spsHex = fmt.Sprintf("%x", sps)

	if c.File != "" {
		// An open film has its own mastered loudness; only the generated clips are built at the target.
		return a, nil
	}
	log := r.capture(ctx, r.FFmpeg, "-hide_banner", "-nostats", "-i", ts, "-map", "0:a:0", "-af", "ebur128=peak=none", "-f", "null", "-")
	lufs, err := parseIntegratedLoudness(log)
	if err != nil {
		return a, err
	}
	a.loudnessDevLU = math.Abs(lufs - TargetLUFS)
	return a, nil
}

// reportAnalysis folds every clip's checks into the break-sequence metrics.
func (r *run) reportAnalysis(all []analysis) {
	if len(all) == 0 {
		for _, n := range []string{"break/pts_gaps", "break/audio_off_grid", "break/sps_variants", "break/loudness_dev_lu", "break/first_packet_ms"} {
			r.rep.Skipped[n] = "no clip encoded"
		}
		return
	}
	var gaps, offAudio int
	var loud, drift, first float64
	sps := map[string]bool{}
	for _, a := range all {
		gaps += a.ptsGaps
		offAudio += a.audioOffGrid
		sps[a.spsHex] = true
		loud = math.Max(loud, a.loudnessDevLU)
		drift = math.Max(drift, a.avEndOffsetMs)
		if a.clip.Break {
			first = math.Max(first, a.firstPacketMs)
		}
	}
	r.rep.Set("break/pts_gaps", float64(gaps), "count", Exact)
	r.rep.Set("break/audio_off_grid", float64(offAudio), "count", Exact)
	r.rep.Set("break/sps_variants", float64(len(sps)), "count", Exact)
	r.rep.Set("break/loudness_dev_lu", loud, "LU", Lower)
	r.rep.Set("break/av_end_offset_ms", drift, "ms", Lower)
	r.rep.Set("break/first_packet_ms", first, "ms", Lower)
}

func execOutput(ctx context.Context, bin string, args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w: %s", filepath.Base(bin), err, tail(errb.String()))
	}
	return out.String(), nil
}
