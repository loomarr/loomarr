package bgexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCapThreadsBoundsDecoderFiltersAndEncoder(t *testing.T) {
	got := CapThreads([]string{"-i", "in.mkv", "-vf", "scale=1:1", "out.mp4"})
	want := []string{"-threads", "1", "-filter_threads", "1", "-filter_complex_threads", "1",
		"-i", "in.mkv", "-vf", "scale=1:1", "-threads", "1", "out.mp4"}
	if !slices.Equal(got, want) {
		t.Fatalf("CapThreads = %v\nwant       %v", got, want)
	}
}

func TestCapThreadsRespectsAnExplicitChoice(t *testing.T) {
	in := []string{"-threads", "4", "-i", "in.mkv", "out.mp4"}
	if got := CapThreads(in); !slices.Equal(got, in) {
		t.Fatalf("an explicit -threads was rewritten: %v", got)
	}
}

// fakeTool writes its own nice value and argv to files, then exits with the given code.
func fakeTool(t *testing.T, exit int) (tool, report string) {
	t.Helper()
	dir := t.TempDir()
	report = filepath.Join(dir, "report")
	tool = filepath.Join(dir, "tool")
	script := "#!/bin/sh\nps -o ni= -p $$ > " + report + "\necho \"$@\" >> " + report + "\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return tool, report
}

func requireNice0(t *testing.T) {
	t.Helper()
	out, err := exec.Command("ps", "-o", "ni=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Skip("needs ps and a test process at nice 0")
	}
}

func TestEveryEntryPointRunsAtBackgroundPriorityAndFFmpegIsCapped(t *testing.T) {
	requireNice0(t)
	ctx := context.Background()
	for name, run := range map[string]func(tool string) error{
		"Run":            func(tool string) error { return FFmpeg(ctx, tool, "-i", "a", "b").Run() },
		"Output":         func(tool string) error { _, err := FFmpeg(ctx, tool, "-i", "a", "b").Output(); return err },
		"CombinedOutput": func(tool string) error { _, err := FFmpeg(ctx, tool, "-i", "a", "b").CombinedOutput(); return err },
	} {
		tool, report := fakeTool(t, 0)
		if err := run(tool); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, _ := os.ReadFile(report)
		lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
		if strings.TrimSpace(lines[0]) != "10" {
			t.Errorf("%s ran at nice %q, want 10", name, lines[0])
		}
		if len(lines) < 2 || !strings.Contains(lines[1], "-threads 1") {
			t.Errorf("%s: ffmpeg args were not thread-capped: %q", name, raw)
		}
	}
}

func TestToolIsNicedButNotThreadCapped(t *testing.T) {
	requireNice0(t)
	tool, report := fakeTool(t, 0)
	if err := Tool(context.Background(), tool, "-version").Run(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(report)
	if strings.Contains(string(raw), "-threads") || !strings.HasPrefix(strings.TrimSpace(string(raw)), "10") {
		t.Fatalf("Tool report = %q, want nice 10 and untouched args", raw)
	}
}

func TestOutputKeepsExitErrorStderr(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Tool(context.Background(), tool).Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || !strings.Contains(string(exit.Stderr), "boom") {
		t.Fatalf("err = %v (stderr %q), want ExitError code 3 carrying stderr", err, func() []byte {
			if exit != nil {
				return exit.Stderr
			}
			return nil
		}())
	}
}

// ⚠ proctree reports a stop it caused as success. A killed tool's truncated output must not pass
// for a result.
func TestCancelledContextIsAnErrorNotAPartialSuccess(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := Tool(ctx, tool).Run(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline exceeded", err)
	}
}

func TestCapThreadsLeavesStreamSpecificThreadChoicesAlone(t *testing.T) {
	in := []string{"-threads:v", "1", "-i", "in.mkv", "-f", "null", "-"}
	if got := CapThreads(in); !slices.Equal(got, in) {
		t.Fatalf("a caller's -threads:v choice was rewritten: %v", got)
	}
}
