package clipfetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	media := strings.TrimSuffix(sidecar, ".info.json") + ".mp4"
	executable := testkit.Executable(t, "fake-yt-dlp", fmt.Sprintf(`#!/bin/sh
while test "$1" != "--print-to-file"; do shift; done
printf '%%s\n' %q >> "$3"
`, strconv.Quote(media)))

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
	_, stamped := readSidecar(t, path)[filler.SidecarLoomarrKey()]
	return stamped
}

func readSidecar(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
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

func TestYtDlpDownloader_CreatesMissingDropDirectoryForPrivateResultFile(t *testing.T) {
	drop := filepath.Join(t.TempDir(), "incoming")
	argsFile := filepath.Join(t.TempDir(), "args")
	ytdlp := testkit.Executable(t, "yt-dlp", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
`, argsFile))

	d := NewYtDlpDownloader(ytdlp, "ffmpeg")
	src := Source{Kind: YouTube, URL: "https://youtube.com/watch?v=current-id"}
	if _, _, err := d.Download(context.Background(), src, drop); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(drop)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("drop path %q is not a directory", drop)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--print-to-file") {
		t.Fatalf("yt-dlp args = %q, want private result-file argument", args)
	}
}

func TestYtDlpDownloader_StampsOnlyCurrentInvocationSidecars(t *testing.T) {
	drop := t.TempDir()
	unrelated := filepath.Join(drop, "operator-drop.info.json")
	if err := os.WriteFile(unrelated, []byte(`{"title":"operator drop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(drop, "Current title [current-id].mp4")
	sidecar := filepath.Join(drop, "Current title [current-id].info.json")
	playlistMedia := filepath.Join(drop, "Second title [second-id].mp4")
	playlistSidecar := filepath.Join(drop, "Second title [second-id].info.json")
	marker := filepath.Join(drop, ".first-run")
	ytdlp := testkit.Executable(t, "yt-dlp", fmt.Sprintf(`#!/bin/sh
case "$*" in
	*--print-to-file\ after_move:%%\(filepath\)j*) ;;
	*) exit 9 ;;
esac
while test "$1" != "--print-to-file"; do shift; done
result="$3"
if test -e %q; then
  exit 0
fi
printf '%%s\n' '{"title":"current download"}' > %q
touch %q
printf '%%s\n' '{"title":"second download"}' > %q
touch %q
printf '%%s\n' %q >> "$result"
printf '%%s\n' %q >> "$result"
printf 'download warning: %%s\n' %q >&2
`, marker, sidecar, marker, playlistSidecar, playlistMedia, strconv.Quote(filepath.Base(media)), strconv.Quote(filepath.Base(playlistMedia)), media))

	d := NewYtDlpDownloader(ytdlp, "ffmpeg")
	src := Source{ID: "youtube:current", AcquisitionID: "acq-current", Kind: YouTube, URL: "https://youtube.com/watch?v=current-id"}
	if _, _, err := d.Download(context.Background(), src, drop); err != nil {
		t.Fatal(err)
	}
	if _, stamped := readSidecar(t, unrelated)[filler.SidecarLoomarrKey()]; stamped {
		t.Fatal("stderr diagnostic stamped an unrelated pre-existing sidecar")
	}
	for _, current := range []string{sidecar, playlistSidecar} {
		mark, ok := readSidecar(t, current)[filler.SidecarLoomarrKey()].(map[string]any)
		if !ok || mark["sourceId"] != "youtube:current" || mark["acquisitionId"] != "acq-current" {
			t.Fatalf("current sidecar %q mark = %#v, want exact source and acquisition attribution", current, mark)
		}
	}

	before, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Download(context.Background(), src, drop); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("all-skipped retry changed a prior sidecar")
	}
	if _, stamped := readSidecar(t, unrelated)[filler.SidecarLoomarrKey()]; stamped {
		t.Fatal("all-skipped retry stamped an unrelated sidecar")
	}
}

func TestYtDlpDownloader_RejectsEscapingAndSymlinkedSidecars(t *testing.T) {
	drop := t.TempDir()
	outside := t.TempDir()
	outsideMedia := filepath.Join(outside, "outside.mp4")
	outsideSidecar := filepath.Join(outside, "outside.info.json")
	if err := os.WriteFile(outsideSidecar, []byte(`{"title":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkMedia := filepath.Join(drop, "symlinked.mp4")
	symlinkSidecar := filepath.Join(drop, "symlinked.info.json")
	if err := os.Symlink(outsideSidecar, symlinkSidecar); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(drop, "Current title [current-id].mp4")
	sidecar := filepath.Join(drop, "Current title [current-id].info.json")
	ytdlp := testkit.Executable(t, "yt-dlp", fmt.Sprintf(`#!/bin/sh
while test "$1" != "--print-to-file"; do shift; done
result="$3"
printf '%%s\n' '{"title":"current download"}' > %q
touch %q
printf '%%s\n' %q >> "$result"
printf '%%s\n' %q >> "$result"
printf '%%s\n' %q >> "$result"
printf '%%s\n' 'not-json' >> "$result"
printf '%%s\n' '""' >> "$result"
printf '%%s\n' %q >> "$result"
`, sidecar, media, strconv.Quote(media), strconv.Quote(symlinkMedia), strconv.Quote(outsideMedia), strconv.Quote(filepath.Join("..", filepath.Base(outsideMedia)))))

	d := NewYtDlpDownloader(ytdlp, "ffmpeg")
	src := Source{ID: "youtube:current", AcquisitionID: "acq-current", Kind: YouTube, URL: "https://youtube.com/watch?v=current-id"}
	if _, _, err := d.Download(context.Background(), src, drop); err != nil {
		t.Fatal(err)
	}
	if _, stamped := readSidecar(t, outsideSidecar)[filler.SidecarLoomarrKey()]; stamped {
		t.Fatal("escaping or symlinked result stamped a sidecar outside the drop directory")
	}
	if _, stamped := readSidecar(t, sidecar)[filler.SidecarLoomarrKey()]; !stamped {
		t.Fatal("in-directory sidecar was not stamped")
	}
}

func TestStampFetchedRejectsFinalSymlinkAndOpenedFilePathMismatch(t *testing.T) {
	drop := t.TempDir()
	root, err := os.OpenRoot(drop)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	normal := filepath.Join(drop, "normal.info.json")
	if err := os.WriteFile(normal, []byte(`{"title":"normal"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(drop, "result.jsonl")
	if err := os.WriteFile(result, []byte("\"normal.mp4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	normalSidecars := ytDlpSidecars(root, result)
	if len(normalSidecars) != 1 {
		t.Fatalf("opened normal sidecars = %d, want 1", len(normalSidecars))
	}
	stampFetched(root, normalSidecars, "youtube:current", "acq-current")
	if _, stamped := readSidecar(t, normal)[filler.SidecarLoomarrKey()]; !stamped {
		t.Fatal("regular sidecar was not stamped")
	}

	unrelated := filepath.Join(drop, "unrelated.info.json")
	if err := os.WriteFile(unrelated, []byte(`{"title":"unrelated"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(drop, "linked.info.json")
	if err := os.Symlink("unrelated.info.json", linked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result, []byte("\"linked.mp4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stampFetched(root, ytDlpSidecars(root, result), "youtube:current", "acq-current")
	if _, stamped := readSidecar(t, unrelated)[filler.SidecarLoomarrKey()]; stamped {
		t.Fatal("final symlink stamped its unrelated target")
	}

	claimed := filepath.Join(drop, "claimed.info.json")
	if err := os.WriteFile(claimed, []byte(`{"title":"claimed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result, []byte("\"claimed.mp4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sidecars := ytDlpSidecars(root, result)
	if len(sidecars) != 1 {
		t.Fatalf("opened sidecars = %d, want 1", len(sidecars))
	}
	original := filepath.Join(drop, "original.info.json")
	if err := os.Rename(claimed, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claimed, []byte(`{"title":"replacement"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stampFetched(root, sidecars, "youtube:current", "acq-current")
	for _, path := range []string{original, claimed} {
		if _, stamped := readSidecar(t, path)[filler.SidecarLoomarrKey()]; stamped {
			t.Fatalf("path replacement stamped %q", path)
		}
	}
}
