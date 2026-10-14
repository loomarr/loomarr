package releaseverify

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestEnsureContainerImageUsesCachedImage(t *testing.T) {
	result := runEnsureContainerImage(t, testkit.FakeDockerConfig{InspectStatus: 0})
	if result.Err != nil {
		t.Fatalf("cached image: %v", result.Err)
	}
	if result.PullAttempts != 0 {
		t.Fatalf("cached image made %d pull attempts, want 0", result.PullAttempts)
	}
	if len(result.SleepCalls) != 0 {
		t.Fatalf("cached image slept: %v", result.SleepCalls)
	}
	if want := []string{"example.invalid/image:pinned"}; !slices.Equal(result.InspectImages, want) {
		t.Fatalf("cached inspect images = %v, want %v", result.InspectImages, want)
	}
	if len(result.PullImages) != 0 {
		t.Fatalf("cached image pulled unexpected images: %v", result.PullImages)
	}
}

func TestEnsureContainerImageRetriesTransientPullFailure(t *testing.T) {
	result := runEnsureContainerImage(t, testkit.FakeDockerConfig{
		InspectStatus: 1,
		PullStatuses:  []int{35, 35, 35, 35, 0},
	})
	if result.Err != nil {
		t.Fatalf("transient pull: %v", result.Err)
	}
	if result.PullAttempts != 5 {
		t.Fatalf("transient pull made %d attempts, want 5", result.PullAttempts)
	}
	if want := []int{2, 2, 2, 2}; !slices.Equal(result.SleepCalls, want) {
		t.Fatalf("transient pull sleeps = %v, want %v", result.SleepCalls, want)
	}
	if want := []string{
		"example.invalid/image:pinned",
		"example.invalid/image:pinned",
		"example.invalid/image:pinned",
		"example.invalid/image:pinned",
		"example.invalid/image:pinned",
	}; !slices.Equal(result.PullImages, want) {
		t.Fatalf("transient pull images = %v, want %v", result.PullImages, want)
	}
}

func TestEnsureContainerImageStopsAfterFiveFailures(t *testing.T) {
	result := runEnsureContainerImage(t, testkit.FakeDockerConfig{
		InspectStatus: 1,
		PullStatuses:  []int{21, 22, 23, 24, 25},
	})
	var exitError *exec.ExitError
	if !errors.As(result.Err, &exitError) {
		t.Fatalf("permanent pull error = %v, want process exit", result.Err)
	}
	if exitError.ExitCode() != 25 {
		t.Fatalf("permanent pull status = %d, want fifth docker status 25", exitError.ExitCode())
	}
	if result.PullAttempts != 5 {
		t.Fatalf("permanent pull made %d attempts, want 5", result.PullAttempts)
	}
	if want := []int{2, 2, 2, 2}; !slices.Equal(result.SleepCalls, want) {
		t.Fatalf("permanent pull sleeps = %v, want %v", result.SleepCalls, want)
	}
}

func TestEnsureContainerImageFailsClosedWhenSleepFails(t *testing.T) {
	result := runEnsureContainerImage(t, testkit.FakeDockerConfig{
		InspectStatus: 1,
		PullStatuses:  []int{35, 0},
		SleepStatuses: []int{41},
	})
	var exitError *exec.ExitError
	if !errors.As(result.Err, &exitError) {
		t.Fatalf("sleep failure = %v, want process exit", result.Err)
	}
	if exitError.ExitCode() != 41 {
		t.Fatalf("sleep failure status = %d, want 41", exitError.ExitCode())
	}
	if result.PullAttempts != 1 {
		t.Fatalf("sleep failure made %d pulls, want 1", result.PullAttempts)
	}
	if want := []int{2}; !slices.Equal(result.SleepCalls, want) {
		t.Fatalf("sleep failure waits = %v, want %v", result.SleepCalls, want)
	}
}

func runEnsureContainerImage(t *testing.T, config testkit.FakeDockerConfig) testkit.FakeDockerResult {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	return testkit.RunWithFakeDocker(t, filepath.Join(root, "scripts", "ensure-container-image.sh"),
		[]string{"example.invalid/image:pinned"}, config)
}
