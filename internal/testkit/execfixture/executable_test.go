//go:build !windows

package execfixture

import (
	"context"
	"os/exec"
	"sync"
	"testing"
)

// An executable written while other goroutines fork must run at once: a child forked while the
// file was open for writing inherits that descriptor until it execs, and exec of the file then
// fails with ETXTBSY ("text file busy", seen in CI on clipfetch's fake yt-dlp).
func TestExecutableRunsWhileOtherGoroutinesFork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_ = exec.Command("true").Run()
			}
		}()
	}
	defer func() { cancel(); wg.Wait() }()
	for i := range 300 {
		path := Executable(t, "fake", "#!/bin/sh\nexit 0\n")
		if err := exec.Command(path).Run(); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}
