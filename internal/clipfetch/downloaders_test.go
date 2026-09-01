package clipfetch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestYtDlpDownloaderCancellationStopsCompleteProcessTree(t *testing.T) {
	dir := t.TempDir()
	executable, parentPIDFile, childPIDFile := blockingYtDlp(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	result := downloadAsync(ctx, executable, dir, "")

	parentPID := waitForHelperPID(t, parentPIDFile)
	childPID := waitForHelperPID(t, childPIDFile)
	t.Cleanup(func() { killTestProcess(childPID) })
	cancel()
	_ = waitForDownload(t, result)

	assertProcessGone(t, parentPID)
	assertProcessGone(t, childPID)
}

func TestYtDlpDownloaderCancellationIsNotSuccessfulAcquisition(t *testing.T) {
	dir := t.TempDir()
	sidecar := writeTestSidecar(t, dir)
	executable, _, childPIDFile := blockingYtDlp(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	result := downloadAsync(ctx, executable, dir, "source-1")

	childPID := waitForHelperPID(t, childPIDFile)
	t.Cleanup(func() { killTestProcess(childPID) })
	cancel()
	err := waitForDownload(t, result)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download error = %v, want context.Canceled", err)
	}
	if sidecarStamped(t, sidecar) {
		t.Fatal("cancelled download stamped sidecar as a successful acquisition")
	}
}

func TestYtDlpDownloaderDeadlineIsNotSuccessfulAcquisition(t *testing.T) {
	dir := t.TempDir()
	sidecar := writeTestSidecar(t, dir)
	executable, _, childPIDFile := blockingYtDlp(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	result := downloadAsync(ctx, executable, dir, "source-1")

	childPID := waitForHelperPID(t, childPIDFile)
	t.Cleanup(func() { killTestProcess(childPID) })
	err := waitForDownload(t, result)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Download error = %v, want context.DeadlineExceeded", err)
	}
	if sidecarStamped(t, sidecar) {
		t.Fatal("timed-out download stamped sidecar as a successful acquisition")
	}
}

func TestYtDlpDownloaderFailureKeepsBoundedActionableDiagnostics(t *testing.T) {
	dir := t.TempDir()
	executable := testkit.DescendantProcessExecutable(t, "fake-yt-dlp")
	t.Setenv(testkit.ProcessTreeModeEnv, "fail-large")

	_, _, err := NewYtDlpDownloader(executable, "ffmpeg").Download(context.Background(), Source{
		Kind: YouTube,
		URL:  "https://example.invalid/playlist",
	}, dir)
	if err == nil {
		t.Fatal("Download succeeded after yt-dlp exited non-zero")
	}
	diagnostic := err.Error()
	if len(diagnostic) > 70<<10 {
		t.Fatalf("diagnostic retained %d bytes, want at most 70 KiB", len(diagnostic))
	}
	if !strings.Contains(diagnostic, "TAIL-MARKER") {
		t.Fatalf("diagnostic lost actionable tail: %q", diagnostic)
	}
	if strings.Contains(diagnostic, "HEAD-MARKER") {
		t.Fatal("diagnostic retained stale head after output exceeded its bound")
	}
}

func TestYtDlpDownloaderNaturalSuccessStampsSidecar(t *testing.T) {
	dir := t.TempDir()
	sidecar := writeTestSidecar(t, dir)
	executable := testkit.DescendantProcessExecutable(t, "fake-yt-dlp")
	t.Setenv(testkit.ProcessTreeModeEnv, "success")

	_, _, err := NewYtDlpDownloader(executable, "ffmpeg").Download(context.Background(), Source{
		ID:   "source-1",
		Kind: YouTube,
		URL:  "https://example.invalid/playlist",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sidecarStamped(t, sidecar) {
		t.Fatal("successful download did not stamp its sidecar")
	}
}

func TestYtDlpDownloaderNaturalFailureDoesNotStampSidecar(t *testing.T) {
	dir := t.TempDir()
	sidecar := writeTestSidecar(t, dir)
	executable := testkit.DescendantProcessExecutable(t, "fake-yt-dlp")
	t.Setenv(testkit.ProcessTreeModeEnv, "fail-large")

	_, _, err := NewYtDlpDownloader(executable, "ffmpeg").Download(context.Background(), Source{
		ID:   "source-1",
		Kind: YouTube,
		URL:  "https://example.invalid/playlist",
	}, dir)
	if err == nil {
		t.Fatal("Download succeeded after yt-dlp exited non-zero")
	}
	if sidecarStamped(t, sidecar) {
		t.Fatal("failed download stamped sidecar as a successful acquisition")
	}
}

func blockingYtDlp(t *testing.T, dir string) (executable, parentPIDFile, childPIDFile string) {
	t.Helper()
	executable = testkit.DescendantProcessExecutable(t, "fake-yt-dlp")
	parentPIDFile = dir + "/parent.pid"
	childPIDFile = dir + "/child.pid"
	t.Setenv(testkit.ProcessTreeModeEnv, "block")
	t.Setenv(testkit.ProcessTreeParentPIDFileEnv, parentPIDFile)
	t.Setenv(testkit.ProcessTreeChildPIDFileEnv, childPIDFile)
	return executable, parentPIDFile, childPIDFile
}

func downloadAsync(ctx context.Context, executable, dir, sourceID string) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, _, err := NewYtDlpDownloader(executable, "ffmpeg").Download(ctx, Source{
			ID:   sourceID,
			Kind: YouTube,
			URL:  "https://example.invalid/playlist",
		}, dir)
		result <- err
	}()
	return result
}

func waitForDownload(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Download did not return after cancellation")
		return nil
	}
}

func writeTestSidecar(t *testing.T, dir string) string {
	t.Helper()
	path := dir + "/clip.info.json"
	if err := os.WriteFile(path, []byte(`{"title":"clip"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sidecarStamped(t *testing.T, path string) bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	_, stamped := doc[filler.SidecarLoomarrKey()]
	return stamped
}

func waitForHelperPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr != nil {
				t.Fatalf("parse helper pid: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read helper pid: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not write %s", path)
	return 0
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if testProcessGone(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d survived yt-dlp cancellation", pid)
}
