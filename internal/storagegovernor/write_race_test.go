package storagegovernor_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// These tests swap package-level stat seams, so they must not run in parallel.

func TestMonitorPathIgnoresAFileRemovedBetweenListingAndStat(t *testing.T) {
	root := t.TempDir()
	vanishing := filepath.Join(root, "segment-000034.m4s")
	if err := os.WriteFile(vanishing, make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.m4s"), make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, _ := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 100,
	})
	defer lease.Release()
	restore := storagegovernor.SetWalkStatHooks(nil, func(path string) (fs.FileInfo, error) {
		if path == vanishing {
			_ = os.Remove(path)
		}
		return os.Lstat(path)
	})
	defer restore()

	_, finish := storagegovernor.MonitorPath(t.Context(), lease, root, 0)
	if err := finish(); err != nil {
		t.Fatalf("a file that vanished mid-walk failed the write monitor: %v", err)
	}
}

func TestFilesystemMeterIgnoresAnEntryRemovedByConcurrentStagingCleanup(t *testing.T) {
	root := t.TempDir()
	vanishing := filepath.Join(root, ".staging-x", "segment-000093.m4s")
	if err := os.MkdirAll(filepath.Dir(vanishing), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vanishing, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	governor := newFilesystemGovernor(t,
		[]storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainPrepared}},
		func(storagegovernor.Domain) storagegovernor.Policy {
			return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
		})
	restore := storagegovernor.SetWalkStatHooks(func(entry fs.DirEntry) (fs.FileInfo, error) {
		if entry.Name() == "segment-000093.m4s" {
			_ = os.Remove(vanishing)
		}
		return entry.Info()
	}, nil)
	defer restore()

	lease, decision := governor.Reserve(context.Background(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10, Mode: storagegovernor.Automatic,
	})
	if lease != nil {
		defer lease.Release()
	}
	if !decision.Allowed {
		t.Fatalf("a staging file removed by another worker's cleanup made capacity unavailable: %+v err=%v",
			decision.Snapshot.Reason, decision.Err)
	}
}

func TestMonitorGrowingPathExtendsTheReservationInsteadOfDiscardingNearlyFinishedOutput(t *testing.T) {
	root := t.TempDir()
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, _ := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10,
	})
	defer lease.Release()
	if err := os.WriteFile(filepath.Join(root, "segment.m4s"), make([]byte, 11), 0o600); err != nil {
		t.Fatal(err)
	}
	_, finish := storagegovernor.MonitorGrowingPath(t.Context(), lease, root, 0)
	if err := finish(); err != nil {
		t.Fatalf("output past its estimate was discarded instead of re-estimated: %v", err)
	}
	if got := lease.Estimated(); got <= 11 {
		t.Fatalf("lease estimate = %d, want extended past the observed 11 bytes", got)
	}
}

// shrinkingMeter reports free space that the test lowers after the reservation was granted.
type shrinkingMeter struct {
	preparedCapacityMeterForMonitor
	free *int64
}

func (m shrinkingMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return storagegovernor.Measurement{ID: "shrinking", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: *m.free}, nil
}

func TestMonitorGrowingPathStillStopsWhenTheHostCannotTakeTheExtension(t *testing.T) {
	root := t.TempDir()
	free := 100 * storagegovernor.GiB
	governor := storagegovernor.New(shrinkingMeter{free: &free}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, decision := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10,
	})
	if lease == nil {
		t.Fatalf("reservation refused: %+v", decision)
	}
	defer lease.Release()
	if err := os.WriteFile(filepath.Join(root, "segment.m4s"), make([]byte, 11), 0o600); err != nil {
		t.Fatal(err)
	}
	free = 0 // the disk filled while the encode ran
	_, finish := storagegovernor.MonitorGrowingPath(t.Context(), lease, root, 0)
	if err := finish(); err == nil || !strings.Contains(err.Error(), "storage write paused") {
		t.Fatalf("a refused extension must still stop the writer, got %v", err)
	}
}
