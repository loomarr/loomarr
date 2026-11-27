package upkeep

import (
	"context"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/identity"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// Standing is what the channels say about the clips on air, in the order they are let go
// (pod.Standing).
type Standing struct {
	// Idle is the clips that fit no channel.
	Idle []string
	// Surplus is the clips some channel could air but none needs for its evening target, most
	// aired first.
	Surplus []string
}

// Room is what a pass reads to make room when a fetch was refused for library_limit (decision 13
// of #3838). All three are required for a pass to evict a clip on air; without Standing and InBreak
// it still lets go of rejects early.
type Room struct {
	// Shortfall is how many bytes the library budget is short of the room a refused reservation
	// asked for; 0 when none was refused or the budget has since made room.
	Shortfall func(ctx context.Context) int64
	// Standing reads how the channels stand toward the clips on air.
	Standing func(ctx context.Context) (Standing, error)
	// InBreak is the clips in a break inside the guide window: assembled for a channel's coming
	// breaks, or airing now. They are never evicted.
	InBreak func(ctx context.Context) (map[string]bool, error)
}

// held is how many bytes wait in quarantine, 0 when they cannot be measured.
func (j *Job) held() int64 {
	n, err := j.bin.Held()
	if err != nil {
		j.warn("upkeep: the quarantine could not be measured", "err", err)
		return 0
	}
	return n
}

// finishEvictions sets aside the files of clips on air that an earlier pass evicted and a crash
// left in place. The evict commits before any file moves, so what is left is only bytes.
func (j *Job) finishEvictions(ctx context.Context, res *Result) error {
	rows, err := j.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionReady},
	})
	if err != nil {
		return err
	}
	var referenced *identity.AssetRefs
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.FilesDeletedAt.IsZero() || j.disposed[row.ClipHash] {
			continue
		}
		if _, err := j.setAside(ctx, row.ClipHash, &referenced); err != nil {
			return err
		}
		if j.disposed[row.ClipHash] {
			res.Quarantined++
		}
	}
	return nil
}

// makeRoom frees the room a refused fetch asked for, in this order, and stops at the first that is
// enough:
//
//  1. leftover download staging, which every pass has already cleaned before this runs;
//  2. fetched rejects, oldest first, without waiting out their week;
//  3. clips that fit no channel, without waiting out their week;
//  4. surplus beyond what each channel needs for its evening target, most aired first.
//
// A clip a channel needs for its target, a clip in a break inside the guide window, a hand-added
// clip and a pinned clip are never evicted. What a pass sets aside stays in quarantine for a day and
// counts against the budget until then, so the room it asked for is the shortfall less what
// quarantine already holds or was emptied of this pass: asking again every hour would otherwise
// evict a second time for the same refusal.
func (j *Job) makeRoom(ctx context.Context, res *Result, emptied int64) error {
	if j.room.Shortfall == nil {
		return nil
	}
	short := j.room.Shortfall(ctx)
	if short <= 0 {
		return nil
	}
	held, err := j.bin.Held()
	if err != nil {
		j.warn("upkeep: the quarantine could not be measured, so nothing is evicted for room; the next pass tries again", "err", err)
		return err
	}
	need := short - held - emptied
	if need <= 0 {
		return nil
	}
	var referenced *identity.AssetRefs
	evicted := func(moved int64) {
		need -= moved
		res.Evicted++
		res.EvictedBytes += moved
	}

	rows, err := j.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRejected},
	})
	if err != nil {
		return err
	}
	slices.SortFunc(rows, func(a, b pipelinestate.ClipPipeline) int {
		if c := a.UpdatedAt.Compare(b.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ClipHash, b.ClipHash)
	})
	now := j.now().UTC()
	for _, row := range rows {
		if need <= 0 {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.AcquisitionID == "" || !row.FilesDeletedAt.IsZero() || j.disposed[row.ClipHash] {
			continue
		}
		retired, moved, err := j.letGo(ctx, row, now, &referenced)
		if err != nil {
			return err
		}
		if !retired {
			continue
		}
		res.LetGo++
		if j.disposed[row.ClipHash] {
			res.Quarantined++
		}
		evicted(moved)
	}
	if need <= 0 || j.room.Standing == nil || j.room.InBreak == nil {
		return nil
	}

	standing, err := j.room.Standing(ctx)
	if err != nil {
		j.warn("upkeep: how the channels stand could not be read, so no clip on air is evicted; the next pass tries again", "err", err)
		return err
	}
	inBreak, err := j.room.InBreak(ctx)
	if err != nil {
		j.warn("upkeep: the clips in coming breaks could not be read, so no clip on air is evicted; the next pass tries again", "err", err)
		return err
	}
	for _, hash := range slices.Concat(standing.Idle, standing.Surplus) {
		if need <= 0 {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if inBreak[hash] {
			continue
		}
		retired, err := j.store.RetireReadyClip(ctx, hash, now)
		if err != nil {
			return err
		}
		if !retired {
			continue
		}
		moved, err := j.setAside(ctx, hash, &referenced)
		if err != nil {
			return err
		}
		if j.disposed[hash] {
			res.Quarantined++
		}
		evicted(moved)
	}
	return nil
}
