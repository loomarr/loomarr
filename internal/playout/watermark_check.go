package playout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The watermark self-check (#1512 phase 1d): does THIS host's GPU overlay draw the bug AND keep
// the programme?
//
// A speed or exit-code gate is worthless here. On the image's ffmpeg n8.1.2, overlay_cuda after
// NVDEC airs an all-green picture with only the bug drawn, at full speed, exit 0, with a correct SPS
// (spike #1532, finding 4) — the same class as tonemap_vaapi's black picture (#1516). So the check
// runs the production graph (BuildItem, the family's real decode, filters and encoder) on a
// hardware-decodable clip and asserts the PICTURE of the encoded output:
//
//   - the programme survives: outside the bug, the bug-on frame matches the bug-off frame;
//   - the bug is present, where placement says, at the expected blend: 65% white over the measured
//     background, which also proves the family's alpha convention (straight vs premultiplied);
//   - the bug-on SPS and PPS are byte-identical to the bug-off ones, because breaks air bug-off in
//     the same channel stream and a parameter-set change there resets decoders.
//
// Any failure disables the watermark on the host (HostProfile.Overlay false). It is never moved to
// the CPU.

// WatermarkCheckResult is the self-check's verdict and what it measured.
type WatermarkCheckResult struct {
	Works bool
	// Detail is why the check failed, or the measurements when it passed.
	Detail string
}

const (
	checkWidth, checkHeight, checkFPS = 1920, 1080, 25
	checkFrames                       = 25
	checkBug                          = 64 // the test bug: a white square
	checkAlpha                        = 166.0 / 255
	// checkFrame is the decoded frame compared; past the first so the overlay has settled.
	checkFrame = 12
	// pictureTolerance is the mean absolute luma difference allowed outside the bug: two encodes
	// of the same picture differ by rate control, not by picture.
	pictureTolerance = 3.0
	// bugTolerance is the allowed distance from the expected blend. A wrong alpha convention misses
	// by 50+ luma levels; chroma subsampling and coding cost a few.
	bugTolerance = 14.0
)

// WatermarkCheck runs the self-check for host's family in dir (scratch space it may fill).
func WatermarkCheck(ctx context.Context, ffmpeg string, host HostProfile, dir string) WatermarkCheckResult {
	if host.Family != FamilyVAAPI && host.Family != FamilyNVENC {
		return WatermarkCheckResult{Detail: "no GPU overlay in the " + string(host.Family) + " family"}
	}
	fail := func(format string, a ...any) WatermarkCheckResult {
		return WatermarkCheckResult{Detail: fmt.Sprintf(format, a...)}
	}
	wm, err := writeCheckBug(dir)
	if err != nil {
		return fail("write test bug: %v", err)
	}
	host.Overlay = true
	var details []string
	for _, class := range checkClasses(host) {
		src, err := class.make(ctx, ffmpeg, dir)
		if err != nil {
			if class.optional {
				details = append(details, class.name+": not checked ("+firstLine(err.Error())+")")
				continue
			}
			return fail("%s source: %v", class.name, err)
		}
		d, err := checkClass(ctx, ffmpeg, host, class.facts, src, wm, filepath.Join(dir, class.name))
		if err != nil {
			return fail("%s: %v", class.name, err)
		}
		details = append(details, class.name+": "+d)
	}
	return WatermarkCheckResult{Works: true, Detail: strings.Join(details, "; ")}
}

type checkSource struct {
	name     string
	facts    MediaFormat
	optional bool
	make     func(ctx context.Context, ffmpeg, dir string) (string, error)
}

// checkClasses are the programme classes the host airs: SDR H.264 always; HDR10 HEVC when the host
// tone-maps (its graph differs: 10-bit decode, tone-map, then the overlay). The HDR fixture needs
// libx265; a build without it checks SDR only and says so.
func checkClasses(host HostProfile) []checkSource {
	classes := []checkSource{{
		name: "sdr",
		facts: MediaFormat{VideoCodec: "h264", Width: checkWidth, Height: checkHeight, FrameRate: checkFPS,
			PixelFormat: "yuv420p", Container: "matroska,webm"},
		make: func(ctx context.Context, ffmpeg, dir string) (string, error) {
			return synth(ctx, ffmpeg, filepath.Join(dir, "sdr.mkv"),
				"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d:duration=1.2", checkWidth, checkHeight, checkFPS),
				"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25")
		},
	}}
	if host.TonemapOpenCL || host.Libplacebo || host.CPUTonemap {
		classes = append(classes, checkSource{
			name: "hdr", optional: true,
			facts: MediaFormat{VideoCodec: "hevc", Width: checkWidth, Height: checkHeight, FrameRate: checkFPS,
				PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084", Container: "matroska,webm"},
			make: func(ctx context.Context, ffmpeg, dir string) (string, error) {
				return synth(ctx, ffmpeg, filepath.Join(dir, "hdr.mkv"),
					"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d:duration=1.2", checkWidth, checkHeight, checkFPS),
					"-vf", "zscale=tin=bt709:min=bt709:pin=bt709:rin=tv:t=smpte2084:p=bt2020:m=bt2020nc:r=tv:npl=203,format=yuv420p10le",
					"-c:v", "libx265", "-preset", "ultrafast",
					"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
					"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
			},
		})
	}
	return classes
}

func synth(ctx context.Context, ffmpeg, out string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	full := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, args...)
	if b, err := exec.CommandContext(ctx, ffmpeg, append(full, out)...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: %s", err, bytes.TrimSpace(b))
	}
	return out, nil
}

// checkClass encodes src bug-off and bug-on through the production graph and compares them.
func checkClass(ctx context.Context, ffmpeg string, host HostProfile, facts MediaFormat, src string, wm *Watermark, prefix string) (string, error) {
	out := OutputProfile{Width: checkWidth, Height: checkHeight, FPS: checkFPS, Quality: outputQuality,
		TargetKbps: outputTargetKbps, MaxKbps: outputMaxKbps, GOPSeconds: outputGOPSeconds, AudioKbps: 128}
	encode := func(bug *Watermark, path string) error {
		p, err := BuildItem(host, facts, out, bug)
		if err != nil {
			return err
		}
		if bug != nil && !p.Watermark {
			return fmt.Errorf("the graph has no overlay: %s", strings.Join(p.Fallbacks, "; "))
		}
		args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, p.PreInput...)
		args = append(args, "-i", src, "-map", "0:v:0", "-frames:v", strconv.Itoa(checkFrames), "-vf", p.VideoFilter)
		args = append(append(args, p.VideoEncode...), "-f", "h264", path)
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if b, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("encode: %w: %s", err, firstLine(string(bytes.TrimSpace(b))))
		}
		return nil
	}
	off, on := prefix+"-off.h264", prefix+"-on.h264"
	if err := encode(nil, off); err != nil {
		return "", fmt.Errorf("bug off: %w", err)
	}
	if err := encode(wm, on); err != nil {
		return "", fmt.Errorf("bug on: %w", err)
	}
	offRaw, err := os.ReadFile(off)
	if err != nil {
		return "", err
	}
	onRaw, err := os.ReadFile(on)
	if err != nil {
		return "", err
	}
	if err := sameParameterSets(offRaw, onRaw); err != nil {
		return "", err
	}
	offY, err := decodeLuma(ctx, ffmpeg, off, checkFrame)
	if err != nil {
		return "", err
	}
	onY, err := decodeLuma(ctx, ffmpeg, on, checkFrame)
	if err != nil {
		return "", err
	}
	x, y := wm.position(facts, out)
	m := compareBug(offY, onY, checkWidth, Rect{X: x, Y: y, W: wm.Width, H: wm.Height})
	switch {
	case m.pictureMean < 16.5:
		return "", fmt.Errorf("the bug-off picture is black (YAVG %.1f): the fixture or the graph is broken", m.pictureMean)
	case m.pictureDiff > pictureTolerance:
		return "", fmt.Errorf("the overlay lost the programme picture: outside the bug, mean |ΔY| %.1f vs bug-off (YAVG on %.1f, off %.1f)",
			m.pictureDiff, m.onMean, m.pictureMean)
	case math.Abs(m.bugLuma-m.bugWant) > bugTolerance:
		return "", fmt.Errorf("the bug is wrong: luma %.1f where a 65%% white blend is %.1f (background %.1f)", m.bugLuma, m.bugWant, m.bugBackground)
	}
	return fmt.Sprintf("picture ΔY %.2f, bug luma %.1f (want %.1f over %.1f), SPS/PPS identical",
		m.pictureDiff, m.bugLuma, m.bugWant, m.bugBackground), nil
}

// writeCheckBug writes the test bug, a white square at the check alpha, in both conventions.
func writeCheckBug(dir string) (*Watermark, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	a := uint8(math.Round(checkAlpha * 255))
	straight := image.NewNRGBA(image.Rect(0, 0, checkBug, checkBug))
	pm := image.NewNRGBA(image.Rect(0, 0, checkBug, checkBug))
	for y := 0; y < checkBug; y++ {
		for x := 0; x < checkBug; x++ {
			straight.SetNRGBA(x, y, color.NRGBA{255, 255, 255, a})
			pm.SetNRGBA(x, y, color.NRGBA{a, a, a, a}) // premultiplied values, stored as-is
		}
	}
	wm := &Watermark{Straight: filepath.Join(dir, "bug.png"), Premultiplied: filepath.Join(dir, "bug.pm.png"),
		Width: checkBug, Height: checkBug, Corner: CornerTopRight, MarginX: 96, MarginY: 54}
	for path, img := range map[string]image.Image{wm.Straight: straight, wm.Premultiplied: pm} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
	}
	if !safeGraphPath.MatchString(wm.Straight) {
		return nil, errors.New("scratch directory path needs escaping: " + dir)
	}
	return wm, nil
}

// decodeLuma decodes frame n of an H.264 elementary stream to 8-bit luma.
func decodeLuma(ctx context.Context, ffmpeg, path string, n int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-i", path,
		"-vf", "select=eq(n\\,"+strconv.Itoa(n)+")", "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	cmd.Stderr = &stderr
	y, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w: %s", filepath.Base(path), err, firstLine(stderr.String()))
	}
	if len(y) != checkWidth*checkHeight {
		return nil, fmt.Errorf("decode %s: %d luma bytes, want %dx%d", filepath.Base(path), len(y), checkWidth, checkHeight)
	}
	return y, nil
}

type bugMeasure struct {
	pictureMean, onMean, pictureDiff float64
	bugLuma, bugWant, bugBackground  float64
}

// compareBug measures the programme outside the bug (with a guard band for chroma and coding
// bleed) and the bug's interior against the expected straight-alpha blend of limited-range white.
func compareBug(off, on []byte, width int, bug Rect) bugMeasure {
	const guard, inset = 8, 6
	var m bugMeasure
	var nOut, nIn float64
	for i := range off {
		x, y := i%width, i/width
		o, b := float64(off[i]), float64(on[i])
		m.pictureMean += o
		m.onMean += b
		inGuard := x >= bug.X-guard && x < bug.X+bug.W+guard && y >= bug.Y-guard && y < bug.Y+bug.H+guard
		inBug := x >= bug.X+inset && x < bug.X+bug.W-inset && y >= bug.Y+inset && y < bug.Y+bug.H-inset
		switch {
		case inBug:
			m.bugLuma += b
			m.bugBackground += o
			m.bugWant += checkAlpha*235 + (1-checkAlpha)*o
			nIn++
		case !inGuard:
			m.pictureDiff += math.Abs(o - b)
			nOut++
		}
	}
	total := float64(len(off))
	m.pictureMean /= total
	m.onMean /= total
	m.pictureDiff /= nOut
	m.bugLuma /= nIn
	m.bugWant /= nIn
	m.bugBackground /= nIn
	return m
}

// sameParameterSets compares the first SPS and PPS of two H.264 elementary streams.
func sameParameterSets(off, on []byte) error {
	for _, kind := range []struct {
		name string
		typ  byte
	}{{"SPS", 7}, {"PPS", 8}} {
		a, b := firstNAL(off, kind.typ), firstNAL(on, kind.typ)
		if a == nil || b == nil {
			return fmt.Errorf("no %s in the encoded stream", kind.name)
		}
		if !bytes.Equal(a, b) {
			return fmt.Errorf("bug-on %s differs from bug-off (%x vs %x): a programme and a break would reset the decoder", kind.name, b, a)
		}
	}
	return nil
}

// firstNAL returns the first Annex B NAL unit of the given type, start code excluded.
func firstNAL(stream []byte, typ byte) []byte {
	for i := 0; i+3 < len(stream); i++ {
		if stream[i] != 0 || stream[i+1] != 0 || stream[i+2] != 1 {
			continue
		}
		start := i + 3
		if start < len(stream) && stream[start]&0x1f == typ {
			end := bytes.Index(stream[start:], []byte{0, 0, 1})
			if end < 0 {
				return stream[start:]
			}
			nal := stream[start : start+end]
			return bytes.TrimRight(nal, "\x00") // a 4-byte start code's leading zero
		}
	}
	return nil
}
