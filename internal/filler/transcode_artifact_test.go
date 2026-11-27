package filler

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// A refused transcode recovery names what changed between attempts (#4429), so the next one
// on prod says which measurement or copy moved instead of only that the manifests differ.
func TestManifestDifferenceNamesWhatMoved(t *testing.T) {
	base := func() *asset.Manifest {
		return &asset.Manifest{Version: 1,
			SourceMaster: asset.Identity{Role: asset.RoleSourceMaster, SHA256: "m"},
			Evidence:     &asset.DerivativeLineage{Asset: asset.Identity{SHA256: "e"}},
			Playback: &asset.DerivativeLineage{Asset: asset.Identity{SHA256: "p"},
				QC: tools.DerivativeQC{Loudness: tools.ConditioningLoudness{IntegratedLUFS: -24}}},
		}
	}
	for name, tc := range map[string]struct {
		change func(*asset.Manifest)
		want   string
	}{
		"rebuilt evidence":     {func(m *asset.Manifest) { m.Evidence.Asset.SHA256 = "e2" }, "evidence.asset"},
		"re-measured loudness": {func(m *asset.Manifest) { m.Playback.QC.Loudness.IntegratedLUFS = -23.9 }, "playback.qc"},
		"evidence dropped":     {func(m *asset.Manifest) { m.Evidence = nil }, "evidence"},
		"two parts": {func(m *asset.Manifest) {
			m.SourceMaster.SHA256 = "m2"
			m.Playback.Quality.DurationMs = 5
		}, "sourceMaster, playback.quality"},
	} {
		t.Run(name, func(t *testing.T) {
			current := base()
			tc.change(current)
			if got := manifestDifference(base(), current); got != tc.want {
				t.Fatalf("manifestDifference = %q, want %q", got, tc.want)
			}
		})
	}
	if got := manifestDifference(nil, base()); got != "presence" {
		t.Fatalf("manifestDifference(nil, m) = %q, want presence", got)
	}
}
