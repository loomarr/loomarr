package storeadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Defects bridges the store to the defect floor (#2034) and its enforcement
// (#2036). Held clips are listed: admission waits at the defects rung for their verdict.
type Defects struct{ st records.Store }

// NewDefects returns the defect floor's store over st.
func NewDefects(st records.Store) Defects { return Defects{st: st} }

func (a Defects) RetireDefectiveCopy(ctx context.Context, hash string, at time.Time) error {
	err := a.st.RetireDefectiveCopy(ctx, hash, at)
	if errors.Is(err, store.ErrNotFound) {
		return defectfloor.ErrCopyGone
	}
	return err
}

func (a Defects) GetClip(ctx context.Context, hash string) (cliplibrary.StoreClip, bool, error) {
	return Sync(a).GetClip(ctx, hash)
}

func (a Defects) ListClipPaths(ctx context.Context) ([]string, error) {
	clips, err := a.st.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: true, IncludeComposites: true, IncludeRemoved: true})
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(clips))
	for i, c := range clips {
		paths[i] = c.Path
	}
	return paths, nil
}

func (a Defects) ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error) {
	return a.st.ListClipFingerprints(ctx, algorithm)
}

func (a Defects) UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error {
	return a.st.UpsertClipFingerprint(ctx, clipHash, algorithm, fp)
}

func (a Defects) ListDefectClips(ctx context.Context) ([]defectfloor.DefectClip, error) {
	clips, err := a.st.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: true})
	if err != nil {
		return nil, err
	}
	rows, err := a.st.ListClipPipelines(ctx, pipelinestate.PipelineFilter{Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRunning}})
	if err != nil {
		return nil, err
	}
	stage := make(map[string]pipelinestate.StageID, len(rows))
	for _, r := range rows {
		stage[r.ClipHash] = r.Stage
	}
	out := make([]defectfloor.DefectClip, len(clips))
	for i, c := range clips {
		out[i] = defectfloor.DefectClip{StoreClip: StoreClip(c), CreatedAt: c.CreatedAt,
			Stage: stage[c.Hash]}
	}
	return out, nil
}

func (a Defects) ListClipDefects(ctx context.Context) (map[string]defectfloor.DefectRecord, error) {
	rows, err := a.st.ListClipDefects(ctx)
	if errors.Is(err, clipstore.ErrCorruptClipDefects) {
		err = fmt.Errorf("%w: %w", defectfloor.ErrDefectRecordsCorrupt, err)
	} else if err != nil {
		return nil, err
	}
	out := make(map[string]defectfloor.DefectRecord, len(rows))
	for hash, r := range rows {
		out[hash] = defectfloor.DefectRecord(r)
	}
	return out, err
}

func (a Defects) GetClipDefects(ctx context.Context, clipHash string) (defectfloor.DefectRecord, bool, error) {
	r, ok, err := a.st.GetClipDefects(ctx, clipHash)
	return defectfloor.DefectRecord(r), ok, err
}

func (a Defects) UpsertClipDefects(ctx context.Context, r defectfloor.DefectRecord) error {
	return a.st.UpsertClipDefects(ctx, clipstore.ClipDefectRecord(r))
}

func (a Defects) ListClipDefectLabels(ctx context.Context) (map[string]defectfloor.DefectLabel, error) {
	rows, err := a.st.ListClipDefectLabels(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]defectfloor.DefectLabel, len(rows))
	for hash, l := range rows {
		out[hash] = defectfloor.DefectLabel(l)
	}
	return out, nil
}

func (a Defects) SetClipDefectLabel(ctx context.Context, clipHash string, label defects.Label, at time.Time) error {
	err := a.st.SetClipDefectLabel(ctx, clipHash, label, at)
	if errors.Is(err, store.ErrNotFound) {
		return defectfloor.ErrDefectNotMeasured
	}
	return err
}
