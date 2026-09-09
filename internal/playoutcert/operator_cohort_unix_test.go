//go:build !windows

package playoutcert

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorCohortRefusesFIFOsWithoutWaitingForWriter(t *testing.T) {
	for _, mode := range []string{"manifest", "source", "replacement after load"} {
		t.Run(mode, func(t *testing.T) {
			f := newOperatorCohortFixture(t)
			path := filepath.Join(f.root, f.manifest.Channels[0].Programmes[1].File)
			var cohort *OperatorCohort
			if mode == "replacement after load" {
				var err error
				cohort, err = LoadOperatorCohort(t.Context(), f.path, f.channels)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = cohort.Close() }()
			}
			if mode == "manifest" {
				path = f.path
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			if cohort != nil {
				parent := t.TempDir()
				if _, err := cohort.Stage(t.Context(), parent); err == nil {
					t.Fatal("FIFO staged as media")
				}
				entries, err := os.ReadDir(parent)
				if err != nil || len(entries) != 0 {
					t.Fatal("FIFO failure retained staged files")
				}
			} else {
				opened, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
				if err == nil {
					_ = opened.Close()
					t.Fatal("FIFO accepted as regular input")
				}
			}
		})
	}
}
