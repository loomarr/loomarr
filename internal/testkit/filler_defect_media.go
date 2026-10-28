//go:build ffmpeg

package testkit

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// DefectDamage names one way the defect floor's synthetic clips are damaged (#2034). Each is made
// from ffmpeg's lavfi sources only, so the clips are redistributable, and each is labelled by how it
// was made rather than by what the detectors read.
type DefectDamage string

const (
	DamageNone      DefectDamage = "none"
	DamageBlocks    DefectDamage = "blocks"    // MPEG-2 at quantiser = severity; MPEG-2 has no deblocking filter
	DamageBlur      DefectDamage = "blur"      // a gaussian blur of sigma = severity px over the whole picture
	DamageUpscale   DefectDamage = "upscale"   // shot at 640/severity wide, enlarged to 640x480
	DamageWindowbox DefectDamage = "windowbox" // the picture shrunk to severity of each side, centred in black
	DamageCombing   DefectDamage = "combing"   // fields from two moments woven into each frame
	DamageDropouts  DefectDamage = "dropouts"  // severity one-line white specks on every frame, as worn tape drops out
	DamageClipping  DefectDamage = "clipping"  // the tone driven severity times louder, then cut to 16 bits
	DamageRotated   DefectDamage = "rotated"   // the clean clip flagged to display turned 90 degrees: still clean
)

// FillerDefectFixtures are the heavy end of each damage, one clip apiece.
type FillerDefectFixtures struct {
	Clean, Blocky, Blurred, Upscaled, Windowboxed, Combed, Dropouts, Clipped, Rotated string
}

const defectTone = "sine=frequency=440:sample_rate=48000:duration=4"

// defectPicture is four seconds panning across one detailed mandelbrot frame at rate fps.
//
// ⚠ Not testsrc2: blockdetect looks for edges repeating at any period from 3 to 24 pixels, and
// testsrc2's bars and checkerboards are exactly that. A clean testsrc2 encode read 13 and one
// rendered at 480x360 read 138; fractal detail beside smooth gradients reads about 1, as footage
// does. One frame rendered and panned, because rendering mandelbrot every frame took minutes.
func defectPicture(rate int) string {
	return fmt.Sprintf("mandelbrot=size=1280x960:rate=%[1]d:maxiter=512:start_scale=2.5,trim=end_frame=1,"+
		"loop=loop=-1:size=1,setpts=N/%[1]d/TB,crop=640:480:x='t*60':y='t*40'", rate)
}

// FillerDefectMedia generates the heavy fixtures into dir.
func FillerDefectMedia(t testing.TB, dir string) FillerDefectFixtures {
	t.Helper()
	return FillerDefectFixtures{
		Clean:       FillerDefectClip(t, dir, DamageNone, 0),
		Blocky:      FillerDefectClip(t, dir, DamageBlocks, 28),
		Blurred:     FillerDefectClip(t, dir, DamageBlur, 2.5),
		Upscaled:    FillerDefectClip(t, dir, DamageUpscale, 4),
		Windowboxed: FillerDefectClip(t, dir, DamageWindowbox, 0.75),
		Combed:      FillerDefectClip(t, dir, DamageCombing, 1),
		Dropouts:    FillerDefectClip(t, dir, DamageDropouts, 12),
		Clipped:     FillerDefectClip(t, dir, DamageClipping, 30),
		Rotated:     FillerDefectClip(t, dir, DamageRotated, 90),
	}
}

// FillerDefectClip generates one invented four-second clip damaged by damage at severity, and
// returns its path in dir.
func FillerDefectClip(t testing.TB, dir string, damage DefectDamage, severity float64) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag fixture requires ffmpeg")
	}
	sev := strconv.FormatFloat(severity, 'f', -1, 64)
	out := filepath.Join(dir, fmt.Sprintf("defect-%s-%s.mp4", damage, sev))
	picture, videoFilter, audioFilter := defectPicture(25), "", ""
	codec := []string{"-c:v", "libx264", "-crf", "18"}
	switch damage {
	case DamageNone:
	case DamageBlocks:
		codec = []string{"-c:v", "mpeg2video", "-q:v", sev}
	case DamageBlur:
		videoFilter = "gblur=sigma=" + sev
	// Upscale and windowbox shrink the 640x480 picture rather than render it smaller, so each
	// fixture carries one damage only.
	case DamageUpscale:
		w := int(640/severity) / 2 * 2
		videoFilter = fmt.Sprintf("scale=%d:%d:flags=area,scale=640:480:flags=bicubic", w, w*3/4/2*2)
	case DamageWindowbox:
		w := int(640*severity) / 2 * 2
		h := int(480*severity) / 2 * 2
		videoFilter = fmt.Sprintf("scale=%d:%d:flags=area,pad=640:480:%d:%d", w, h, (640-w)/2, (480-h)/2)
	case DamageCombing:
		picture = defectPicture(50)
		videoFilter = "tinterlace=mode=interleave_top"
	case DamageDropouts:
		videoFilter = dropoutSpecks(int(severity))
	case DamageClipping:
		audioFilter = "volume=" + sev + ",aformat=sample_fmts=s16"
	case DamageRotated:
		clean := FillerDefectClip(t, dir, DamageNone, 0)
		// A phone clip: the coded frame is 640x480 and a display matrix turns it upright.
		runDefectFFmpeg(t, ffmpeg, "-nostdin", "-v", "error", "-y", "-display_rotation", sev, "-i", clean, "-c", "copy", out)
		return out
	default:
		t.Fatalf("unknown defect damage %q", damage)
	}
	args := []string{"-nostdin", "-v", "error", "-y", "-f", "lavfi", "-i", picture, "-f", "lavfi", "-i", defectTone}
	if videoFilter != "" {
		args = append(args, "-vf", videoFilter)
	}
	if audioFilter != "" {
		args = append(args, "-af", audioFilter)
	}
	args = append(args, codec...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "4", out)
	runDefectFFmpeg(t, ffmpeg, args...)
	return out
}

func runDefectFFmpeg(t testing.TB, ffmpeg string, args ...string) {
	t.Helper()
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate filler defect fixture: %v: %s", err, out)
	}
}

// dropoutSpecks draws n one-line white specks that move every frame. One line tall matters: a
// dropout is unlike its neighbours in the same field, which a thicker streak is not.
func dropoutSpecks(n int) string {
	var boxes []string
	for k := 1; k <= n; k++ {
		boxes = append(boxes, fmt.Sprintf("drawbox=x='mod(t*%d,560)':y='mod(t*%d+%d,478)':w=%d:h=1:color=white@1:t=fill",
			k*733, k*911, k*37, 40+k*7))
	}
	return strings.Join(boxes, ",")
}
