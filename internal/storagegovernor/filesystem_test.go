package storagegovernor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

func newFilesystemGovernor(t *testing.T, roots []storagegovernor.ManagedRoot, policy func(storagegovernor.Domain) storagegovernor.Policy) *storagegovernor.Governor {
	t.Helper()
	governor, err := storagegovernor.NewFilesystem(roots, policy)
	if err != nil {
		t.Fatal(err)
	}
	return governor
}

func TestFilesystemGovernorCountsManagedRootsWithoutDoubleCountingNestedRoots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	watch := filepath.Join(root, "_watch")
	if err := os.MkdirAll(watch, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(watch, "partial.mp4"), make([]byte, 50), 0o600); err != nil {
		t.Fatal(err)
	}
	governor := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{
		{Path: root, Domain: storagegovernor.DomainFiller},
		{Path: watch, Domain: storagegovernor.DomainFiller},
		{Path: root, Domain: storagegovernor.DomainFiller},
	}, func(domain storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB, AutomaticBudget: domain == storagegovernor.DomainFiller}
	})
	decision := governor.Snapshot(context.Background(), root)
	if !decision.Allowed {
		t.Fatalf("snapshot = %+v", decision)
	}
	if decision.Snapshot.ManagedBytes != 150 {
		t.Fatalf("managed bytes = %d, want 150", decision.Snapshot.ManagedBytes)
	}
}

func TestFilesystemGovernorDoesNotFollowSymlinkedMedia(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	outsideMedia := filepath.Join(outside, "outside.mp4")
	if err := os.WriteFile(outsideMedia, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideMedia, filepath.Join(root, "linked.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	governor := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}, nil)
	decision := governor.Snapshot(context.Background(), root)
	if !decision.Allowed {
		t.Fatalf("snapshot = %+v", decision)
	}
	if decision.Snapshot.ManagedBytes != 0 {
		t.Fatalf("managed bytes = %d, want symlink excluded", decision.Snapshot.ManagedBytes)
	}
}

func TestFilesystemGovernorHonorsCancellationDuringUsageWalk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}, nil).Snapshot(ctx, root)
	if decision.Allowed || decision.Snapshot.Reason != storagegovernor.ReasonCapacityUnavailable || decision.Err == nil {
		t.Fatalf("cancelled snapshot = %+v", decision)
	}
}

func TestFilesystemGovernorKeepsDomainUsageSeparateOnOneFilesystem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fillerRoot := filepath.Join(root, "filler")
	diagnosticsRoot := filepath.Join(root, "diagnostics")
	if err := os.MkdirAll(fillerRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(diagnosticsRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fillerRoot, "clip.mp4"), make([]byte, 256), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(diagnosticsRoot, "run.log"), make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	governor := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{
		{Path: fillerRoot, Domain: storagegovernor.DomainFiller},
		{Path: diagnosticsRoot, Domain: storagegovernor.DomainDiagnostics},
	}, nil)
	filler := governor.DomainSnapshot(context.Background(), fillerRoot, storagegovernor.DomainFiller)
	if !filler.Allowed || filler.Snapshot.ManagedBytes != 256 {
		t.Fatalf("filler snapshot = %+v, want only filler root counted", filler)
	}
	diagnostics := governor.DomainSnapshot(context.Background(), diagnosticsRoot, storagegovernor.DomainDiagnostics)
	if !diagnostics.Allowed || diagnostics.Snapshot.ManagedBytes != 512 {
		t.Fatalf("diagnostics snapshot = %+v, want only diagnostics root counted", diagnostics)
	}
}

func TestFilesystemGovernorRejectsOverlappingDomains(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := storagegovernor.NewFilesystem([]storagegovernor.ManagedRoot{
		{Path: root, Domain: storagegovernor.DomainFiller},
		{Path: filepath.Join(root, "diagnostics"), Domain: storagegovernor.DomainDiagnostics},
	}, nil)
	if err == nil {
		t.Fatal("overlapping roots with different ownership were accepted")
	}
}

func TestFilesystemGovernorRestartReplacesLostReservationsWithCrashLeftUsage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	policy := func(domain storagegovernor.Domain) storagegovernor.Policy {
		if domain == storagegovernor.DomainFiller {
			return storagegovernor.Policy{SoftBudgetBytes: 100}
		}
		return storagegovernor.Policy{}
	}
	before := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}, policy)
	lease, decision := before.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 30,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("pre-crash reserve = %+v", decision)
	}
	// Simulate a process dying without Release: only private bytes survive, never the in-memory
	// lease. A fresh governor must count those bytes instead of treating the restart as free space.
	staging := filepath.Join(root, ".loomarr-acquisitions", "run", "001")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "partial.mp4"), make([]byte, 80), 0o600); err != nil {
		t.Fatal(err)
	}
	after := newFilesystemGovernor(t, []storagegovernor.ManagedRoot{{Path: root, Domain: storagegovernor.DomainFiller}}, policy)
	if next, restarted := after.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 30,
	}); next != nil || restarted.Snapshot.Reason != storagegovernor.ReasonLibraryLimit || restarted.Snapshot.ManagedBytes != 80 {
		t.Fatalf("post-restart reserve = lease %v decision %+v", next, restarted)
	}
}
