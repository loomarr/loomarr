//go:build ffmpeg

package testkit

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// FillerConditioningFixtures are synthetic, redistributable media generated solely from ffmpeg's
// lavfi sources. They contain no downloaded or third-party media.
type FillerConditioningFixtures struct {
	Compilation    string
	OffsetLoudness string
	Silent         string
	WhiteFrozen    string
	ShortVideo     string
	ShortAudio     string
}

// FillerConditioningMedia generates the bounded media used to verify filler conditioning
// measurements. The compilation is 37 seconds at 320x180 and 30000/1001 fps: three twelve-second
// color/tone identities separated by half-second black/silent spans. Its third identity is a static
// field, which gives freezedetect a deterministic interval. The second fixture carries deliberately
// quiet audio starting 120ms after video. The third fixture is fully content-silent audio on black;
// the fourth ends frozen on white so detector termination cannot depend on the final pixel value.
// The final pair deliberately give video and audio different EOFs.
func FillerConditioningMedia(t testing.TB, dir string) FillerConditioningFixtures {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag fixture requires ffmpeg")
	}
	fixtures := FillerConditioningFixtures{
		Compilation:    filepath.Join(dir, "conditioning-compilation.mp4"),
		OffsetLoudness: filepath.Join(dir, "conditioning-offset-loudness.mp4"),
		Silent:         filepath.Join(dir, "conditioning-silent.mp4"),
		WhiteFrozen:    filepath.Join(dir, "conditioning-white-frozen.mp4"),
		ShortVideo:     filepath.Join(dir, "conditioning-short-video.mp4"),
		ShortAudio:     filepath.Join(dir, "conditioning-short-audio.mp4"),
	}

	compilationArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=330:sample_rate=48000:duration=12",
		"-f", "lavfi", "-i", "color=c=black:size=320x180:rate=30000/1001:duration=0.5",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000:duration=0.5",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=30000/1001:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=660:sample_rate=48000:duration=12",
		"-f", "lavfi", "-i", "color=c=black:size=320x180:rate=30000/1001:duration=0.5",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000:duration=0.5",
		"-f", "lavfi", "-i", "color=c=green:size=320x180:rate=30000/1001:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=990:sample_rate=48000:duration=12",
		"-filter_complex",
		"[1:a]volume=1.0,aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo[a0];" +
			"[5:a]volume=0.8,aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo[a1];" +
			"[9:a]volume=0.6,aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo[a2];" +
			"[0:v][a0][2:v][3:a][4:v][a1][6:v][7:a][8:v][a2]concat=n=5:v=1:a=1[v][a]",
		"-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.Compilation,
	}
	runFixtureCommand(t, ffmpeg, compilationArgs)

	offsetArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001:duration=6",
		// AAC priming advances the first presented packet by about 21.3ms. Offset the encoded input by
		// 141.3ms so the muxed fixture exposes the intended 120ms presented-stream delay.
		"-itsoffset", "0.141333", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=5.858667",
		"-map", "0:v:0", "-map", "1:a:0", "-af", "volume=0.02",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.OffsetLoudness,
	}
	runFixtureCommand(t, ffmpeg, offsetArgs)

	silentArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=black:size=320x180:rate=30000/1001:duration=3",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000:duration=3",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.Silent,
	}
	runFixtureCommand(t, ffmpeg, silentArgs)

	whiteFrozenArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=white:size=320x180:rate=30000/1001:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.WhiteFrozen,
	}
	runFixtureCommand(t, ffmpeg, whiteFrozenArgs)

	shortVideoArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001:duration=1",
		"-f", "lavfi", "-i", "color=c=white:size=320x180:rate=30000/1001:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=5",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]",
		"-map", "[v]", "-map", "2:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.ShortVideo,
	}
	runFixtureCommand(t, ffmpeg, shortVideoArgs)

	shortAudioArgs := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001:duration=5",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000:duration=3",
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "300", "-keyint_min", "300", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-y", fixtures.ShortAudio,
	}
	runFixtureCommand(t, ffmpeg, shortAudioArgs)
	return fixtures
}

func runFixtureCommand(t testing.TB, executable string, args []string) {
	t.Helper()
	if out, err := exec.Command(executable, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate filler conditioning fixture: %v: %s", err, out)
	}
}
