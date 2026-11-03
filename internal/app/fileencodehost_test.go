package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
)

// Filler derivatives encode on the channels' hardware (#2121): an operator's pinned encoder wins,
// the detected one applies only once detection has settled, and until then (or with no playout at
// all) the encode is software rather than a guess.
func TestFileEncodeHost_PinnedThenDetectedThenSoftware(t *testing.T) {
	var none *playoutResolver
	if got := none.fileEncodeHost(""); got.Family != encode.FamilySoftware {
		t.Errorf("no playout: family %s, want software", got.Family)
	}
	r := &playoutResolver{detected: encode.EncoderNVENC}
	if got := r.fileEncodeHost(""); got.Family != encode.FamilySoftware {
		t.Errorf("before detection settles: family %s, want software", got.Family)
	}
	r.detectReady.Store(true)
	if got := r.fileEncodeHost(""); got.Family != encode.FamilyNVENC {
		t.Errorf("detected NVENC: family %s", got.Family)
	}
	if got := r.fileEncodeHost(encode.EncoderVAAPI); got.Family != encode.FamilyVAAPI || got.RenderNode == "" {
		t.Errorf("pinned VAAPI: %+v, want the VAAPI family with its render node", got)
	}
}
