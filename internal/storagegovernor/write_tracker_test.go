package storagegovernor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

func TestWriteTrackerChargesEachWriterAgainstOneCeiling(t *testing.T) {
	root := t.TempDir()
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, decision := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 10,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	defer lease.Release()

	tracker := storagegovernor.NewWriteTracker(lease)
	first := filepath.Join(root, "first.bin")
	if err := os.WriteFile(first, make([]byte, 6), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tracker.RecordFile(t.Context(), first); err != nil {
		t.Fatalf("first writer within the ceiling: %v", err)
	}
	// 6 + 5 crosses the 10-byte reservation even though each writer alone fits.
	if err := tracker.Record(t.Context(), 5); err == nil || !strings.Contains(err.Error(), string(storagegovernor.ReasonEstimateUnknown)) {
		t.Fatalf("second writer error = %v, want estimate ceiling", err)
	}
	if err := tracker.Record(t.Context(), 4); err != nil {
		t.Fatalf("refused Record must not consume capacity: %v", err)
	}
}

func TestWriteTrackerRejectsNonRegularOutput(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.bin")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, decision := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 10,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	defer lease.Release()

	if err := storagegovernor.NewWriteTracker(lease).RecordFile(t.Context(), link); err == nil {
		t.Fatal("a symlinked output was accepted")
	}
}

func TestWriteTrackerWithoutLeaseImposesNoCeiling(t *testing.T) {
	var nilTracker *storagegovernor.WriteTracker
	for name, tracker := range map[string]*storagegovernor.WriteTracker{
		"nil tracker": nilTracker,
		"nil lease":   storagegovernor.NewWriteTracker(nil),
	} {
		ctx, finish := tracker.Monitor(t.Context(), t.TempDir())
		if ctx == nil {
			t.Fatalf("%s: Monitor returned a nil context", name)
		}
		if err := finish(); err != nil {
			t.Fatalf("%s: finish: %v", name, err)
		}
		if err := tracker.Record(t.Context(), 1<<40); err != nil {
			t.Fatalf("%s: Record: %v", name, err)
		}
		if err := tracker.RecordFile(t.Context(), "/does/not/exist"); err != nil {
			t.Fatalf("%s: RecordFile: %v", name, err)
		}
	}
}
