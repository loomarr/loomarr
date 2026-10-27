//go:build ffmpeg

package mediatools_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/creative"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The identity corpus resolves through real ffmpeg decoding (#2033): a degraded re-encode and an
// edge-trimmed copy are the master's copies, a cut-down from its shots is a linked variant, and
// another advert is a stranger.
func TestFingerprintResolvesTheIdentityCorpus(t *testing.T) {
	tools := conditioningRealTools(t)
	corpus := testkit.FillerIdentityMedia(t, t.TempDir())
	ctx := context.Background()
	fingerprint := func(path string, durationMs int64) creative.Fingerprint {
		t.Helper()
		fp, err := tools.Fingerprint(ctx, path, 0, durationMs)
		if err != nil {
			t.Fatal(err)
		}
		if len(fp.Video) == 0 || len(fp.Audio) == 0 {
			t.Fatalf("%s fingerprinted %d frames and %d audio hops", path, len(fp.Video), len(fp.Audio))
		}
		return fp
	}
	master := fingerprint(corpus.Master, 30_000)
	for _, tc := range []struct {
		name string
		fp   creative.Fingerprint
		want creative.Relation
	}{
		{"re-encode", fingerprint(corpus.Reencode, 30_000), creative.Same},
		{"edge-trimmed copy", fingerprint(corpus.Trimmed, 28_000), creative.Same},
		{"cut-down", fingerprint(corpus.CutDown, 15_000), creative.Variant},
		{"different advert", fingerprint(corpus.Different, 30_000), creative.Different},
	} {
		if got := creative.Compare(master, tc.fp); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}
}
