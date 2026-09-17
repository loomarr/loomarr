package storagegovernor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

func TestMonitorPathStopsOutputThatCrossesItsLease(t *testing.T) {
	root := t.TempDir()
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, decision := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	defer lease.Release()

	guarded, finish := storagegovernor.MonitorPath(t.Context(), lease, root, 0)
	if err := os.WriteFile(filepath.Join(root, "segment.m4s"), make([]byte, 11), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := finish(); err == nil || !strings.Contains(err.Error(), string(storagegovernor.ReasonEstimateUnknown)) {
		t.Fatalf("finish error = %v", err)
	}
	if context.Cause(guarded) == nil {
		t.Fatal("guarded writer context was not cancelled")
	}
}

func TestMonitorPathAcceptsBoundedRegularOutputAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, _ := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10,
	})
	if lease == nil {
		t.Fatal("reservation was refused")
	}
	defer lease.Release()
	if err := os.WriteFile(filepath.Join(root, "segment.m4s"), make([]byte, 9), 0o600); err != nil {
		t.Fatal(err)
	}
	_, finish := storagegovernor.MonitorPath(t.Context(), lease, root, 0)
	if err := finish(); err != nil {
		t.Fatalf("bounded output: %v", err)
	}

	linkRoot := t.TempDir()
	linkLease, _ := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: linkRoot, Domain: storagegovernor.DomainPrepared, EstimatedBytes: 10,
	})
	if linkLease == nil {
		t.Fatal("symlink test reservation was refused")
	}
	defer linkLease.Release()
	if err := os.Symlink(filepath.Join(root, "segment.m4s"), filepath.Join(linkRoot, "linked.m4s")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, finish = storagegovernor.MonitorPath(t.Context(), linkLease, linkRoot, 0)
	if err := finish(); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink finish error = %v", err)
	}
}

func TestMonitorPathsAppliesOneCeilingAcrossSeveralOutputs(t *testing.T) {
	root := t.TempDir()
	governor := storagegovernor.New(preparedCapacityMeterForMonitor{}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, _ := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 10,
	})
	if lease == nil {
		t.Fatal("reservation was refused")
	}
	defer lease.Release()
	first := filepath.Join(root, "still.jpg")
	second := filepath.Join(root, "preview.webp")
	if err := os.WriteFile(first, make([]byte, 6), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}
	_, finish := storagegovernor.MonitorPaths(t.Context(), lease, []string{first, second}, 0)
	if err := finish(); err == nil || !strings.Contains(err.Error(), string(storagegovernor.ReasonEstimateUnknown)) {
		t.Fatalf("finish error = %v", err)
	}
}

type preparedCapacityMeterForMonitor struct{}

func (preparedCapacityMeterForMonitor) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return storagegovernor.Measurement{
		ID: "monitor", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 100 * storagegovernor.GiB,
	}, nil
}

func (preparedCapacityMeterForMonitor) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	return 0, nil
}
