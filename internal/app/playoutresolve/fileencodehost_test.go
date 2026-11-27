package playoutresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
)

// Filler derivatives encode on the channels' hardware (#2121): an operator's pinned encoder wins,
// the detected one applies only once detection has settled, and until then (or with no playout at
// all) the encode is software rather than a guess.
func TestFileEncodeHost_PinnedThenDetectedThenSoftware(t *testing.T) {
	var none *Resolver
	if got := none.FileEncodeHost(""); got.Family != encode.FamilySoftware {
		t.Errorf("no playout: family %s, want software", got.Family)
	}
	r := &Resolver{detected: encode.EncoderNVENC}
	if got := r.FileEncodeHost(""); got.Family != encode.FamilySoftware {
		t.Errorf("before detection settles: family %s, want software", got.Family)
	}
	r.detectReady.Store(true)
	if got := r.FileEncodeHost(""); got.Family != encode.FamilyNVENC {
		t.Errorf("detected NVENC: family %s", got.Family)
	}
	if got := r.FileEncodeHost(encode.EncoderVAAPI); got.Family != encode.FamilyVAAPI || got.RenderNode == "" {
		t.Errorf("pinned VAAPI: %+v, want the VAAPI family with its render node", got)
	}
}

// A boot that settles on software after this host measured NVENC within the week is missing its
// GPU (#3855), and filler upkeep keeps originals while it is. A host that never measured hardware,
// or one that found it again, is not.
func TestHardwareMissing_SoftwareAfterAHardwareWeek(t *testing.T) {
	root := t.TempDir()
	r := &Resolver{capabilityRoot: func() string { return root }, detected: encode.EncoderSoftware}
	r.detectReady.Store(true)
	if r.HardwareMissing() {
		t.Fatal("a host that never measured hardware is missing it")
	}
	evidence := fmt.Sprintf(`{"version":1,"fingerprint":"with-gpu","encoder":%q,"maxChannels":6,"observedAt":%q}`,
		encode.EncoderNVENC, time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(root, ".host-capability-v1.json"), []byte(evidence), 0o600); err != nil {
		t.Fatal(err)
	}
	if !r.HardwareMissing() {
		t.Fatal("software a day after NVENC worked is not reported missing")
	}
	r.detected = encode.EncoderNVENC
	if r.HardwareMissing() {
		t.Fatal("the GPU came back, yet hardware is reported missing")
	}
}
