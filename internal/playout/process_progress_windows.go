//go:build windows

package playout

import "os/exec"

func progressPipeArg() string { return "pipe:2" }

func wireProgress(cmd *exec.Cmd) (progressWiring, error) {
	stderr, writer, err := newProcessOutputPipe()
	if err != nil {
		return progressWiring{}, err
	}
	cmd.Stderr = writer
	return progressWiring{
		reader:   stderr,
		combined: true,
		afterStart: func() {
			_ = writer.Close()
		},
		closeFailure: func() {
			_ = stderr.Close()
			_ = writer.Close()
		},
	}, nil
}
