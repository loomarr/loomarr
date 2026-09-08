package playout

import "time"

// ProgramClock places a finite source on the shared Channel media timeline.
// Its origin is stable for the session; StartedAt is the source's Airing start.
// The zero value is a standalone finite programme without a shared parent.
type ProgramClock struct {
	Origin    time.Time
	StartedAt time.Time
}

func (c ProgramClock) active() bool { return !c.Origin.IsZero() && !c.StartedAt.IsZero() }

func (c ProgramClock) apply(args []string, outputSeek time.Duration) []string {
	if !c.active() {
		return args
	}
	output := args[len(args)-1]
	aligned := make([]string, 0, len(args)+10)
	aligned = append(aligned, "-copyts", "-start_at_zero")
	aligned = append(aligned, args[:len(args)-1]...)
	// FFmpeg subtracts an output-side seek even with copyts. Restore it in the common shift
	// so discarded preroll cannot move either stream earlier on the Channel timeline.
	aligned = append(aligned, "-output_ts_offset", seconds(c.StartedAt.Sub(c.Origin)+outputSeek), "-muxdelay", "0", "-muxpreload", "0", output)
	return aligned
}
