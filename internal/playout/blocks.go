package playout

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// AiringIdentity is the scheduler-owned boundary metadata carried beside a finite transport block.
// It never enters the MPEG-TS payload; the supervisor uses it to identify real transitions without
// guessing from process exits or request timing.
type AiringIdentity struct {
	StartedAt time.Time
	Kind      schedule.SlotKind
	ContentID string
}

type Block struct {
	Content  io.ReadCloser
	Identity AiringIdentity
}

// BlockSource opens the finite MPEG-TS block that belongs on a Channel now. EOF is an Airing
// boundary: the supervisor closes that body, resolves again from the authoritative wall clock, and
// opens the next programme, Clip or fallback card.
type BlockSource func(context.Context, string, EncodePlan) (Block, error)

// BlockSpawner builds the production session spawner around finite, explicit blocks. One long-lived
// copy mux keeps output timestamps monotonic; Go owns the EOF-and-advance loop so an Airing boundary
// is no longer hidden inside ffconcat.
func BlockSpawner(ffmpeg string, source BlockSource, log *slog.Logger) Spawner {
	return func(ctx context.Context, channelID string, plan EncodePlan) (*Process, error) {
		proc, err := StartPiped(ctx, ffmpeg, BlockMuxArgs(), log, nil)
		if err != nil {
			return nil, err
		}
		go pumpBlocks(ctx, proc.Stdin, source, channelID, plan, log)
		return proc, nil
	}
}

// BlockMuxArgs builds the one continuous transport mux fed by the block supervisor. Children have
// already copied or encoded into the session's stable broadcast format; this process only rebases
// their finite timestamp domains onto one monotonic MPEG-TS timeline.
func BlockMuxArgs() []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-progress", progressPipeArg(), "-nostats",
		"-f", "mpegts", "-i", "pipe:0",
		"-c", "copy",
		"-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", "pipe:1",
	}
}

func pumpBlocks(
	ctx context.Context, dst io.WriteCloser, source BlockSource,
	channelID string, plan EncodePlan, log *slog.Logger,
) {
	defer func() { _ = dst.Close() }()
	var previous AiringIdentity
	for ctx.Err() == nil {
		block, err := source(ctx, channelID, plan)
		if err != nil {
			if log != nil && ctx.Err() == nil {
				log.Warn("playout: block open failed; retrying", "channel", channelID, "plan", plan.String(), "err", err)
			}
			if !waitForBlockRetry(ctx) {
				return
			}
			continue
		}
		if previous != (AiringIdentity{}) && block.Identity != previous && log != nil {
			log.Info("playout: block transition",
				"channel", channelID,
				"from_kind", previous.Kind, "from_content", previous.ContentID,
				"to_kind", block.Identity.Kind, "to_content", block.Identity.ContentID,
				"started_at", block.Identity.StartedAt)
		}
		previous = block.Identity
		n, copyErr := io.Copy(dst, block.Content)
		closeErr := block.Content.Close()
		if copyErr != nil || closeErr != nil {
			if log != nil && ctx.Err() == nil {
				log.Warn("playout: block ended with an error; resolving current Airing",
					"channel", channelID, "plan", plan.String(), "copy_err", copyErr, "close_err", closeErr)
			}
		}
		// An empty success would otherwise hot-loop. Real programme and fallback responses always
		// carry MPEG-TS, so bounded retry is the honest failure posture.
		if n == 0 && !waitForBlockRetry(ctx) {
			return
		}
	}
}

func waitForBlockRetry(ctx context.Context) bool {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
