package proctree

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

const helperModeEnv = "LOOMARR_PROCTREE_HELPER_MODE"

func TestSupervisorHelperProcess(t *testing.T) {
	switch os.Getenv(helperModeEnv) {
	case "sleep":
		time.Sleep(time.Hour)
	case "fail":
		os.Exit(23)
	}
}

func helperCommand(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorHelperProcess$")
	cmd.Env = append(os.Environ(), helperModeEnv+"="+mode)
	return cmd
}

func TestSupervisorContextCancellationStopsSynchronously(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Start(ctx, helperCommand("sleep"))
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := s.Wait(); err != nil {
		t.Fatalf("requested stop error = %v, want nil", err)
	}
	if !s.Stopped() {
		t.Fatal("context cancellation did not record a requested stop")
	}
}

func TestSupervisorAlreadyCancelledDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := helperCommand("sleep")

	if _, err := Start(ctx, cmd); err == nil {
		t.Fatal("Start with an already-cancelled context succeeded")
	}
	if cmd.Process != nil {
		t.Fatalf("cancelled command started with pid %d", cmd.Process.Pid)
	}
}

func TestSupervisorNaturalFailureRemainsVisible(t *testing.T) {
	s, err := Start(context.Background(), helperCommand("fail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err == nil {
		t.Fatal("natural non-zero exit was suppressed")
	}
	if s.Stopped() {
		t.Fatal("natural exit was recorded as a requested stop")
	}
}

func TestSupervisorConcurrentStopAndWaitAreIdempotent(t *testing.T) {
	s, err := Start(context.Background(), helperCommand("sleep"))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Stop()
			if err := s.Wait(); err != nil {
				t.Errorf("Wait after Stop = %v", err)
			}
		}()
	}
	wg.Wait()
}
