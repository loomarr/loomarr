package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

type retentionStore struct {
	diagnosticBefore   time.Time
	diagnosticMax      int64
	diagnosticErr      error
	notificationBefore time.Time
	notificationErr    error
	sessionsCalled     bool
}

type retentionDiagnostics struct {
	before time.Time
	max    int64
}

func (d *retentionDiagnostics) Purge(_ context.Context, before time.Time, max int64) (diagnostics.PurgeResult, error) {
	d.before, d.max = before, max
	return diagnostics.PurgeResult{ProcessRuns: 2, RetainedBytes: 77}, nil
}

func (*retentionStore) PurgeDeniedProposals(context.Context, time.Time) (int, error) { return 0, nil }
func (*retentionStore) PurgeFinishedJobs(context.Context, time.Time) (int, error)    { return 0, nil }
func (*retentionStore) PurgeActivity(context.Context, time.Time) (int, error)        { return 0, nil }
func (s *retentionStore) PurgeTerminalNotifications(_ context.Context, before time.Time) (int, error) {
	s.notificationBefore = before
	return 2, s.notificationErr
}
func (s *retentionStore) PurgeDiagnostics(_ context.Context, before time.Time, maxBytes int64) (diagnostics.PurgeResult, error) {
	s.diagnosticBefore = before
	s.diagnosticMax = maxBytes
	return diagnostics.PurgeResult{Events: 2, ProcessRuns: 1, RetainedBytes: 99}, s.diagnosticErr
}
func (s *retentionStore) PurgeExpiredSessions(context.Context, time.Time) (int, error) {
	s.sessionsCalled = true
	return 0, nil
}

func TestHousekeepingAppliesDiagnosticAgeAndStoragePolicy(t *testing.T) {
	now := time.Date(2026, 8, 23, 17, 0, 0, 0, time.UTC)
	store := &retentionStore{}
	service := New(store, Windows{
		Proposals: func() time.Duration { return 90 * 24 * time.Hour },
		Jobs:      func() time.Duration { return 30 * 24 * time.Hour },
		Activity:  func() time.Duration { return 30 * 24 * time.Hour },
		Diagnostics: func() time.Duration {
			return 7 * 24 * time.Hour
		},
		DiagnosticsMaxBytes: func() int64 { return 512 * 1024 * 1024 },
	}, func() time.Time { return now }, nil)

	if err := service.Housekeeping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(-7 * 24 * time.Hour); !store.diagnosticBefore.Equal(want) {
		t.Fatalf("diagnostic horizon = %v, want %v", store.diagnosticBefore, want)
	}
	if store.diagnosticMax != 512*1024*1024 {
		t.Fatalf("diagnostic budget = %d, want 512 MiB", store.diagnosticMax)
	}
	if want := now.Add(-30 * 24 * time.Hour); !store.notificationBefore.Equal(want) {
		t.Fatalf("notification horizon = %v, want %v", store.notificationBefore, want)
	}
}

func TestHousekeepingAttemptsLaterStagesAfterDiagnosticFailure(t *testing.T) {
	want := errors.New("diagnostic store unavailable")
	store := &retentionStore{diagnosticErr: want}
	service := New(store, Windows{
		Proposals: func() time.Duration { return time.Hour },
		Jobs:      func() time.Duration { return time.Hour },
		Activity:  func() time.Duration { return time.Hour },
		Diagnostics: func() time.Duration {
			return time.Hour
		},
		DiagnosticsMaxBytes: func() int64 { return 1 },
	}, time.Now, nil)

	err := service.Housekeeping(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Housekeeping error = %v, want diagnostics error", err)
	}
	if !store.sessionsCalled {
		t.Fatal("diagnostic failure stopped later session cleanup")
	}
}

func TestHousekeepingAttemptsLaterStagesAfterNotificationFailure(t *testing.T) {
	want := errors.New("notification store unavailable")
	store := &retentionStore{notificationErr: want}
	service := New(store, Windows{
		Proposals: func() time.Duration { return time.Hour },
		Jobs:      func() time.Duration { return time.Hour },
		Activity:  func() time.Duration { return time.Hour },
		Diagnostics: func() time.Duration {
			return time.Hour
		},
		DiagnosticsMaxBytes: func() int64 { return 1 },
	}, time.Now, nil)

	err := service.Housekeeping(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Housekeeping error = %v, want notification error", err)
	}
	if store.diagnosticBefore.IsZero() || !store.sessionsCalled {
		t.Fatal("notification failure stopped later housekeeping stages")
	}
}

func TestHousekeepingUsesFilesystemAwareDiagnosticsCoordinator(t *testing.T) {
	now := time.Date(2026, 8, 23, 17, 0, 0, 0, time.UTC)
	store := &retentionStore{}
	coordinator := &retentionDiagnostics{}
	service := New(store, Windows{
		Proposals: func() time.Duration { return time.Hour }, Jobs: func() time.Duration { return time.Hour },
		Activity: func() time.Duration { return time.Hour }, Diagnostics: func() time.Duration { return 7 * 24 * time.Hour },
		DiagnosticsMaxBytes: func() int64 { return 1234 },
	}, func() time.Time { return now }, nil).WithDiagnostics(coordinator)
	if err := service.PurgeDiagnostics(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !coordinator.before.Equal(now.Add(-7*24*time.Hour)) || coordinator.max != 1234 {
		t.Fatalf("coordinator got before=%v max=%d", coordinator.before, coordinator.max)
	}
	if !store.diagnosticBefore.IsZero() {
		t.Fatal("production coordinator fell through to store-only purge")
	}
}
