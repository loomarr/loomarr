package connectiontest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// values is a fixed settings view keyed by setting key.
type values map[string]any

func (v values) Value(key string) any { return v[key] }

func TestProbeWritableDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := probeWritableDirectory(dir); err != nil {
		t.Fatalf("writable temp directory failed probe: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe left artifacts behind: %v", entries)
	}

	missing := filepath.Join(dir, "missing")
	if err := probeWritableDirectory(missing); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing directory probe = %v, want actionable missing-path error", err)
	}

	file := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probeWritableDirectory(file); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("regular file probe = %v, want not-a-folder error", err)
	}
}

func TestFillerRejectsWatchContainingClipLibrary(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	for name, watch := range map[string]string{
		"same":     filepath.Join(base, "clips"),
		"ancestor": base,
	} {
		t.Run(name, func(t *testing.T) {
			probes := New(Config{Settings: values{
				"filler.dir":       filepath.Join(base, "clips"),
				"filler.watch_dir": watch,
			}})
			ok, detail := probes["filler"](context.Background())
			if ok || !strings.Contains(detail, "unsafe") {
				t.Fatalf("filler Test = %v, %q; want unsafe-layout failure", ok, detail)
			}
		})
	}
}
