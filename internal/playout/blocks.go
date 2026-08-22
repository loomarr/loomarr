package playout

import (
	"context"
	"io"
	"log/slog"
	"time"
)

// BlockSource opens the finite MPEG-TS block that belongs on a Channel now. EOF is an Airing
// boundary: the supervisor closes that body, resolves again from the authoritative wall clock, and
// opens the next programme, Clip or fallback card.
type BlockSource func(context.Context, string, EncodePlan) (io.ReadCloser, error)

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
		n, copyErr := io.Copy(dst, block)
		closeErr := block.Close()
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
