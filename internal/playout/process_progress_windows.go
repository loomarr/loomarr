//go:build windows

package playout

import "os/exec"

func progressPipeArg() string { return "pipe:2" }

func wireProgress(cmd *exec.Cmd) (progressWiring, error) {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return progressWiring{}, err
	}
	return progressWiring{
		reader:   stderr,
		combined: true,
		closeFailure: func() {
			_ = stderr.Close()
		},
	}, nil
}
