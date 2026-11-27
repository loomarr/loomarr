// Package mediatool locates the media binaries real-tool tests run, without importing application
// packages: it is a cycle-free leaf beneath testkit, so a package testkit itself imports can use it.
package mediatool

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// errUnusable marks a repo-local build that must not run and must not be passed over for PATH.
var errUnusable = errors.New("the pinned ffmpeg is unusable; run make bootstrap")

// FFmpeg returns the ffmpeg a test should run, in this order:
//
//  1. FFMPEG_PATH, an explicit choice;
//  2. the repo-local .tools/ffmpeg/bin/ffmpeg, the pinned build `make bootstrap` installs on Apple
//     Silicon, which wins over any ffmpeg on PATH;
//  3. ffmpeg on PATH, the normal case on Linux, where .tools is never created.
//
// A repo-local build whose recorded pin differs from scripts/macos-ffmpeg.pins fails the test, as
// does an Apple Silicon host with no ffmpeg at all. Elsewhere the error means none exists.
func FFmpeg(t testing.TB) (string, error) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, pinned, err := locate(dir, os.Getenv("FFMPEG_PATH"), exec.LookPath)
	appleSilicon := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	switch {
	case errors.Is(err, errUnusable):
		t.Fatal(err)
	case err != nil && appleSilicon:
		t.Fatalf("no ffmpeg; run make bootstrap: %v", err)
	case err == nil && !pinned && appleSilicon:
		// Shown when the test fails: a PATH build may lack filters the pinned one has.
		t.Logf("using %s, not the pinned ffmpeg; run make bootstrap", path)
	}
	return path, err
}

// locate reports pinned for a choice that needs no warning: FFMPEG_PATH or the repo-local build.
func locate(dir, override string, lookPath func(string) (string, error)) (path string, pinned bool, err error) {
	if override != "" {
		return override, true, nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			local, err := repoLocal(dir)
			if err != nil || local != "" {
				return local, true, err
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	path, err = lookPath("ffmpeg")
	return path, false, err
}

// repoLocal returns the installed build under root, or "" when nothing was installed there.
func repoLocal(root string) (string, error) {
	bin := filepath.Join(root, ".tools", "ffmpeg", "bin")
	local := filepath.Join(bin, "ffmpeg")
	info, statErr := os.Stat(local)
	recorded, recordErr := os.ReadFile(filepath.Join(bin, ".pins"))
	if errors.Is(statErr, fs.ErrNotExist) && errors.Is(recordErr, fs.ErrNotExist) {
		return "", nil
	}
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%w: %s is not an executable", errUnusable, local)
	}
	want, err := os.ReadFile(filepath.Join(root, "scripts", "macos-ffmpeg.pins"))
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUnusable, err)
	}
	if recordErr != nil || strings.TrimSpace(string(recorded)) != pinLine(string(want)) {
		return "", fmt.Errorf("%w: %s was not installed from the current pins", errUnusable, bin)
	}
	return local, nil
}

// pinLine is the pins file without its comment lines.
func pinLine(file string) string {
	var lines []string
	for line := range strings.Lines(file) {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, ""))
}
