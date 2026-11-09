package filler

import (
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The defect floor (internal/filler/defectfloor) drops an admitted clip's evidence copy by rewriting
// its sidecar; a rewind reads that same sidecar to decide the clip must rebuild its evidence from
// the transcode. This pins the reading side of that contract.
func TestEvidenceReleasedReadsTheSidecarTheDefectFloorLeaves(t *testing.T) {
	dir := t.TempDir()
	manifest := screeningSubjectManifest(t)
	clip := func(body string, m asset.Manifest) clipstore.Clip {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		rel := cliplibrary.ClipRelPath(hash, ".mp4")
		if err := asset.WriteSidecarTags(filepath.Join(dir, rel), asset.SidecarTags{MediaAssets: &m}, false); err != nil {
			t.Fatal(err)
		}
		return clipstore.Clip{Clip: clips.Clip{Hash: hash, Path: rel}}
	}
	released := manifest
	released.Evidence = nil
	p := &Pipeline{clipDir: dir}
	if !p.evidenceReleased(clip("an admitted ad", released)) {
		t.Error("a clip that keeps its master but no evidence copy was not read as released")
	}
	if p.evidenceReleased(clip("an ad still holding its evidence", manifest)) {
		t.Error("a clip still naming its evidence copy was read as released")
	}
	if p.evidenceReleased(clipstore.Clip{Clip: clips.Clip{Hash: "no-sidecar", Path: "00/00/" + "0000000000000000000000000000000000000000000000000000000000000000.mp4"}}) {
		t.Error("a clip with no sidecar was read as released")
	}
}
