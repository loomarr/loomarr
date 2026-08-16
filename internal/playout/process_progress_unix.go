//go:build !windows

package playout

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const progressFD = 3

func progressPipeArg() string { return "pipe:" + strconv.Itoa(progressFD) }

func wireProgress(cmd *exec.Cmd) (progressWiring, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return progressWiring{}, fmt.Errorf("progress pipe: %w", err)
	}
	cmd.ExtraFiles = []*os.File{pw} // becomes fd 3 in the child
	return progressWiring{
		reader: pr,
		afterStart: func() {
			_ = pw.Close()
		},
		closeFailure: func() {
			_ = pr.Close()
			_ = pw.Close()
		},
	}, nil
}
