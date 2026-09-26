package mediatools

import (
	"context"
	"os/exec"

	"github.com/loomarr/loomarr/internal/proctree"
)

// runBackground runs a filler-pipeline media tool to completion at background scheduling
// priority (#1512 G5). It is the one place the batch tools (quality inspection, conditioning,
// derivative QC) share, so none of them can compete with live playout or the app for CPU.
//
// The cmd must be built with exec.Command, not CommandContext: proctree binds the process tree
// to ctx itself. As with cmd.Run, a context cancellation surfaces to the caller through ctx.Err.
func runBackground(ctx context.Context, cmd *exec.Cmd) error {
	supervised, err := proctree.Start(ctx, cmd, proctree.LowPriority())
	if err != nil {
		return err
	}
	return supervised.Wait()
}
