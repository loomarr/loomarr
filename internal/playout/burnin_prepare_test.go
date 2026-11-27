package playout_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

// burnFFmpeg is an ffmpeg double that logs each run and writes a small track for the extraction;
// the conversion fails, so a test that reaches it shows in the log.
func burnFFmpeg(t *testing.T) (bin, log string) {
	t.Helper()
	log = filepath.Join(t.TempDir(), "runs")
	bin = execfixture.POSIX(t, "ffmpeg", `echo "$@" >> `+log+`
case "$*" in
*"-f sup"*) printf 'PGtrack' ;;
*) exit 3 ;;
esac`)
	return bin, log
}

// The preparer hands the decoder the track Go copied out (its file, its SHA-256 and the source
// stream's codec), and refuses a result past the decoder's ceilings before converting anything.
func TestFFmpegBurnInWith_HandsTheExtractedTrackAndRefusesPastTheCeilings(t *testing.T) {
	cues := []testkit.SubtitleCue{{StartMs: 1000, EndMs: 2000}}
	many := make([]testkit.SubtitleCue, 50_001)
	for i := range many {
		many[i] = testkit.SubtitleCue{StartMs: int64(i), Clear: true}
	}
	for name, dec := range map[string]*testkit.SubtitleDecoder{
		"odd canvas":       {Width: 1919, Height: 1080, Cues: cues},
		"zero canvas":      {Width: 0, Height: 1080, Cues: cues},
		"oversized canvas": {Width: 7680, Height: 4320, Cues: cues},
		"too many states":  {Width: 64, Height: 36, Cues: many},
		"still outside":    {Width: 64, Height: 36, Cues: cues, StillDir: t.TempDir()},
	} {
		bin, log := burnFFmpeg(t)
		dir := filepath.Join(t.TempDir(), "track")
		if _, err := playout.FFmpegBurnInWith(bin, nil, dec)(t.Context(), "/media/film.mkv", 3, "hdmv_pgs_subtitle", dir); err == nil {
			t.Errorf("%s: prepared", name)
			continue
		}
		calls := dec.Calls()
		if len(calls) != 1 {
			t.Fatalf("%s: decoder asked %d times", name, len(calls))
		}
		sum := sha256.Sum256([]byte("PGtrack"))
		if c := calls[0]; c.Codec != "hdmv_pgs_subtitle" || c.SHA256 != hex.EncodeToString(sum[:]) ||
			!filepath.IsAbs(c.Sup) || !filepath.IsAbs(c.Dir) {
			t.Errorf("%s: decoder got %+v, want the extracted track, its digest and codec", name, c)
		}
		runs, _ := os.ReadFile(log)
		if n := strings.Count(string(runs), "\n"); n != 1 || !strings.Contains(string(runs), "-map 0:3 ") {
			t.Errorf("%s: ffmpeg ran %q, want only the extraction of stream 3", name, runs)
		}
		if left, _ := filepath.Glob(dir + "*"); len(left) != 0 {
			t.Errorf("%s: left %v on disk", name, left)
		}
	}
}
