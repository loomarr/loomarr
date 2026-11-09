package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

// fillerSourceAdapter bridges the optional Tunarr annotation slice used by
// cliplibrary.DirSource (§10): it registers FILLER_DIR and joins Tunarr program uuids
// onto Loomarr's locally-scanned, content-hash-identified clips.
type fillerSourceAdapter struct {
	prog       tunarrFillerClient
	configured func() bool
}

// tunarrFillerClient is the exact tunarr slice needed to annotate Loomarr's local
// filler catalog with optional Tunarr program ids. Keeping the seam narrow lets the adapter's
// live-enable behavior be tested without starting a network service.
type tunarrFillerClient interface {
	EnsureLocalFillerSource(ctx context.Context, dir string) (tunarr.EnsureLocalSourceResult, error)
	ListLocalFillerClipsAll(ctx context.Context) ([]tunarr.LocalClip, error)
}

// available is resolved per call so an internal-only process can gain Tunarr filler
// annotation after the connection is saved, without turning an empty Tunarr URL into a
// warning on every ordinary local scan. A nil resolver is always on, for direct construction
// in tests.
func (a fillerSourceAdapter) available() bool {
	return a.configured == nil || a.configured()
}

func (a fillerSourceAdapter) EnsureLocalSource(ctx context.Context, dir string) error {
	if !a.available() {
		return nil
	}
	_, err := a.prog.EnsureLocalFillerSource(ctx, dir)
	return err
}

// LocalClipIDsByName maps a clip's file name to the Tunarr program uuid Tunarr assigned it.
//
// Name is the only join key Tunarr's scan reports back, so it is what we have. Imperfect: two
// clips sharing a basename in different subfolders collide and one may take the other's uuid.
// That is tolerable ONLY because the uuid stopped being identity (§9.1) — a wrong uuid degrades
// a Tunarr filler-list, it cannot corrupt the catalog or misdirect internal playout, both of
// which key on the path.
func (a fillerSourceAdapter) LocalClipIDsByName(ctx context.Context) (map[string]string, error) {
	if !a.available() {
		return map[string]string{}, nil
	}
	clips, err := a.prog.ListLocalFillerClipsAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(clips))
	for _, c := range clips {
		// Tunarr reports the display name; the scanner derives its Name the same way (base
		// filename without extension), so strip an extension here if Tunarr kept one.
		name := strings.TrimSuffix(c.Name, filepath.Ext(c.Name))
		out[name] = c.ProgramID
	}
	return out, nil
}

// clipExposureAdapter is deliberately separate from the catalog reader: pod planning can read
// actual-airing history but cannot write it. Playout owns the sole write boundary.
type clipExposureAdapter struct{ st store.Store }

// airingSettle is how long before a break an airing must have been recorded to count for it.
// Playout resolves a break's first clip no earlier than its run-ahead before the break and records
// each clip as it resolves it, so every airing the break counts was already stored when its first
// clip was assembled. The margin covers clock skew between Postgres replicas.
const airingSettle = playout.OnAirLookahead + 3*time.Second

func (a clipExposureAdapter) FillerAirings(ctx context.Context) (pod.AiringHistory, error) {
	airings, err := a.st.FillerAirings(ctx)
	return pod.AiringHistory{Airings: airings, Settle: airingSettle}, err
}
