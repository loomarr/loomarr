package mediameasure

import "os/exec"

func lowPriority(cmd *exec.Cmd) error { return cmd.Start() }
