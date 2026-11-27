// Package mediatool locates the media binaries real-tool tests run, without importing application
// packages: it is a cycle-free leaf beneath testkit, so a package testkit itself imports can use it.
package mediatool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// FFmpeg returns the ffmpeg a test should run, in this order:
//
//  1. FFMPEG_PATH, an explicit choice;
//  2. the repo-local .tools/ffmpeg/bin/ffmpeg, the pinned build `make bootstrap` installs on Apple
//     Silicon, which wins over any ffmpeg on PATH;
//  3. ffmpeg on PATH, which is the normal case on Linux, where .tools is never created.
//
// The error means none exists.
func FFmpeg(t testing.TB) (string, error) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, pinned, err := locate(dir, os.Getenv("FFMPEG_PATH"), exec.LookPath)
	if err == nil && !pinned && runtime.GOOS == "darwin" {
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
			local := filepath.Join(dir, ".tools", "ffmpeg", "bin", "ffmpeg")
			if info, err := os.Stat(local); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
				return local, true, nil
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
