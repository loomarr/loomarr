//go:build ffmpeg

package testkit

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// FillerIdentityFixtures are synthetic, redistributable adverts generated solely from ffmpeg's lavfi
// sources, for creative identity (#2033). Master is one 30 s advert; the rest are what a catalog
// meets: a degraded copy, an edge-trimmed copy, a cut-down built from its shots, and a stranger.
type FillerIdentityFixtures struct {
	Master    string
	Reencode  string
	Trimmed   string
	CutDown   string
	Different string
}

// FillerIdentityMedia generates the identity corpus.
//   - Master: twelve 2.5 s shots of seeded gradients and Life at 320x180, 30000/1001 fps, over pink
//     noise and a melody that steps every half second, so both fingerprints carry real structure.
//   - Reencode: Master at 160x90, CRF 38, 25 fps and −6 dB.
//   - Trimmed: Master from 0.9 s for 28 s.
//   - CutDown: Master's 0–8 s and 20–27 s joined, 15 s.
//   - Different: another advert built the same way from other seeds.
func FillerIdentityMedia(t testing.TB, dir string) FillerIdentityFixtures {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag fixture requires ffmpeg")
	}
	f := FillerIdentityFixtures{
		Master:    filepath.Join(dir, "identity-master.mp4"),
		Reencode:  filepath.Join(dir, "identity-reencode.mp4"),
		Trimmed:   filepath.Join(dir, "identity-trimmed.mp4"),
		CutDown:   filepath.Join(dir, "identity-cutdown.mp4"),
		Different: filepath.Join(dir, "identity-different.mp4"),
	}
	runFixtureCommand(t, ffmpeg, identityAdvertArgs(f.Master, 1))
	runFixtureCommand(t, ffmpeg, identityAdvertArgs(f.Different, 7))
	runFixtureCommand(t, ffmpeg, []string{
		"-nostdin", "-v", "error", "-y", "-i", f.Master,
		"-vf", "scale=160:90,fps=25", "-af", "volume=-6dB",
		"-c:v", "libx264", "-crf", "38", "-c:a", "aac", f.Reencode,
	})
	runFixtureCommand(t, ffmpeg, []string{
		"-nostdin", "-v", "error", "-y", "-ss", "0.9", "-t", "28", "-i", f.Master,
		"-c:v", "libx264", "-crf", "20", "-c:a", "aac", f.Trimmed,
	})
	runFixtureCommand(t, ffmpeg, []string{
		"-nostdin", "-v", "error", "-y", "-i", f.Master, "-filter_complex",
		"[0:v]trim=0:8,setpts=PTS-STARTPTS[v0];[0:a]atrim=0:8,asetpts=PTS-STARTPTS[a0];" +
			"[0:v]trim=20:27,setpts=PTS-STARTPTS[v1];[0:a]atrim=20:27,asetpts=PTS-STARTPTS[a1];" +
			"[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]",
		"-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-crf", "20", "-c:a", "aac", f.CutDown,
	})
	return f
}

// identityAdvertArgs renders one advert whose every shot and note derive from seed.
func identityAdvertArgs(out string, seed int) []string {
	const shots, shotSeconds = 12, 2.5
	args := []string{"-nostdin", "-v", "error", "-y"}
	var chain strings.Builder
	for i := range shots {
		s := seed*100 + i
		source := fmt.Sprintf("gradients=s=320x180:r=30000/1001:d=%g:seed=%d:speed=0.01:nb_colors=4", shotSeconds, s)
		if i%2 == 1 {
			source = fmt.Sprintf("life=s=320x180:r=2:seed=%d:ratio=0.3:mold=10:life_color=#%06x,fps=30000/1001,trim=duration=%g",
				s, 0x305080+s*7919%0xa0a0a0, shotSeconds)
		}
		args = append(args, "-f", "lavfi", "-i", source)
		fmt.Fprintf(&chain, "[%d:v]format=yuv420p,setsar=1[s%d];", i, i)
	}
	for i := range shots {
		fmt.Fprintf(&chain, "[s%d]", i)
	}
	fmt.Fprintf(&chain, "concat=n=%d:v=1:a=0[v];", shots)
	melody := fmt.Sprintf("0.25*sin(2*PI*(196+%d*mod(floor(t*2)*%d,9))*t)", 37+seed, 3+seed)
	args = append(args,
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:amplitude=0.15:sample_rate=48000:duration=30", seed),
		"-f", "lavfi", "-i", fmt.Sprintf("aevalsrc=exprs='%s':sample_rate=48000:duration=30", melody),
	)
	fmt.Fprintf(&chain, "[%d:a][%d:a]amix=inputs=2:normalize=0,aformat=channel_layouts=stereo[a]", shots, shots+1)
	return append(args, "-filter_complex", chain.String(), "-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-crf", "18", "-c:a", "aac", "-b:a", "128k", out)
}
