//go:build ffmpeg

package testkit

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// FillerDefectFixtures are one invented four-second clip, clean and damaged in each way the
// defect floor measures (#2034). Generated from ffmpeg's lavfi sources only, so they are
// redistributable, and each is labelled by how it was made.
type FillerDefectFixtures struct {
	Clean       string
	Blocky      string // starved MPEG-2, which has no deblocking filter: visible compression blocks
	Blurred     string // a 2.5 px gaussian blur over the whole picture
	Upscaled    string // shot at 160x120, enlarged to 640x480
	Windowboxed string // a 480x360 picture inside a black 640x480 frame
	Combed      string // fields from two moments woven into each frame
	Dropouts    string // twelve one-line white specks on every frame, as a worn tape drops out
	Clipped     string // the tone driven 30x past full scale
	Rotated     string // the clean clip flagged to display turned 90 degrees: still clean
}

const (
	defectPicture = "testsrc2=size=640x480:rate=25:duration=4"
	defectTone    = "sine=frequency=440:sample_rate=48000:duration=4"
)

// FillerDefectMedia generates the defect fixtures into dir.
func FillerDefectMedia(t testing.TB, dir string) FillerDefectFixtures {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag fixture requires ffmpeg")
	}
	f := FillerDefectFixtures{
		Clean:       filepath.Join(dir, "defect-clean.mp4"),
		Blocky:      filepath.Join(dir, "defect-blocky.mp4"),
		Blurred:     filepath.Join(dir, "defect-blurred.mp4"),
		Upscaled:    filepath.Join(dir, "defect-upscaled.mp4"),
		Windowboxed: filepath.Join(dir, "defect-windowboxed.mp4"),
		Combed:      filepath.Join(dir, "defect-combed.mp4"),
		Dropouts:    filepath.Join(dir, "defect-dropouts.mp4"),
		Clipped:     filepath.Join(dir, "defect-clipped.mp4"),
		Rotated:     filepath.Join(dir, "defect-rotated.mp4"),
	}
	encode := func(out, picture, videoFilter, audioFilter string, codec ...string) {
		args := []string{"-nostdin", "-v", "error", "-y",
			"-f", "lavfi", "-i", picture, "-f", "lavfi", "-i", defectTone}
		if videoFilter != "" {
			args = append(args, "-vf", videoFilter)
		}
		if audioFilter != "" {
			args = append(args, "-af", audioFilter)
		}
		if len(codec) == 0 {
			codec = []string{"-c:v", "libx264", "-crf", "18"}
		}
		args = append(args, codec...)
		args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", out)
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("generate filler defect fixture: %v: %s", err, out)
		}
	}
	encode(f.Clean, defectPicture, "", "")
	encode(f.Blocky, defectPicture, "", "", "-c:v", "mpeg2video", "-q:v", "28")
	encode(f.Blurred, defectPicture, "gblur=sigma=2.5", "")
	encode(f.Upscaled, "testsrc2=size=160x120:rate=25:duration=4", "scale=640:480:flags=bicubic", "")
	encode(f.Windowboxed, "testsrc2=size=480x360:rate=25:duration=4", "pad=640:480:80:60", "")
	encode(f.Combed, "testsrc2=size=640x480:rate=50:duration=4", "tinterlace=mode=interleave_top", "")
	encode(f.Dropouts, defectPicture, dropoutSpecks(), "")
	encode(f.Clipped, defectPicture, "", "volume=30,aformat=sample_fmts=s16")
	// A phone clip: the coded frame is 640x480 and a display matrix turns it upright.
	if out, err := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y", "-display_rotation", "90",
		"-i", f.Clean, "-c", "copy", f.Rotated).CombinedOutput(); err != nil {
		t.Fatalf("generate filler defect fixture: %v: %s", err, out)
	}
	return f
}

// dropoutSpecks draws twelve one-line white specks that move every frame. One line tall matters:
// a dropout is unlike its neighbours in the same field, which a thicker streak is not.
func dropoutSpecks() string {
	var boxes []string
	for k := 1; k <= 12; k++ {
		boxes = append(boxes, fmt.Sprintf("drawbox=x='mod(t*%d,560)':y='mod(t*%d+%d,478)':w=%d:h=1:color=white@1:t=fill",
			k*733, k*911, k*37, 40+k*7))
	}
	return strings.Join(boxes, ",")
}
