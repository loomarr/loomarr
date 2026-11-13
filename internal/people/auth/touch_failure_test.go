package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// touchFailingStore fails the use-recording writes, as a locked database does.
type touchFailingStore struct {
	store.Store
}

func (touchFailingStore) TouchSession(context.Context, string, identitystore.SessionSeen) error {
	return errors.New("database is locked")
}

func (touchFailingStore) TouchDeviceToken(context.Context, string, time.Time) error {
	return errors.New("database is locked")
}

// captureWarnings routes the default logger into a buffer for the test. It is not safe beside
// parallel tests that log, so the tests using it do not call t.Parallel.
func captureWarnings(t *testing.T) func() int {
	t.Helper()
	var mu sync.Mutex
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}), nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(buf.String(), "auth.touch_failed")
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// A session whose use cannot be recorded still resolves, and the failure is logged once per
// touchGranularity rather than once per request.
func TestResolveReportsAFailedSessionTouch(t *testing.T) {
	warnings := captureWarnings(t)
	ctx := t.Context()
	base := newStore(t)
	if err := base.UpsertUser(ctx, identitystore.User{ID: "u1", Name: "A", Role: identitystore.RoleMember}); err != nil {
		t.Fatal(err)
	}
	clock := now
	mgr := NewManager(touchFailingStore{base}, time.Hour, func() time.Time { return clock })
	token, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	for range 20 {
		if _, err := mgr.Resolve(ctx, token, "Firefox"); err != nil {
			t.Fatalf("Resolve failed because the touch did: %v", err)
		}
	}
	if got := warnings(); got != 1 {
		t.Fatalf("20 requests with a failing touch logged %d warnings, want 1", got)
	}
	clock = clock.Add(touchGranularity)
	if _, err := mgr.Resolve(ctx, token, "Firefox"); err != nil {
		t.Fatal(err)
	}
	if got := warnings(); got != 2 {
		t.Errorf("a request a granularity later logged %d warnings in all, want 2", got)
	}
}

// A paired device is held to the same rule as a session.
func TestResolveDeviceReportsAFailedTouch(t *testing.T) {
	warnings := captureWarnings(t)
	ctx := t.Context()
	base := newStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	working := NewDeviceManager(base, func() time.Time { return at })
	if err := base.UpsertUser(ctx, identitystore.User{ID: "u-member", Name: "member", Role: identitystore.RoleMember}); err != nil {
		t.Fatal(err)
	}
	deviceCode, userCode, _, err := working.StartPairing(ctx, "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := working.Approve(ctx, userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := working.Redeem(ctx, deviceCode)
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewDeviceManager(touchFailingStore{base}, func() time.Time { return at })
	at = at.Add(2 * time.Minute)
	for range 20 {
		if _, err := mgr.ResolveDevice(ctx, token); err != nil {
			t.Fatalf("ResolveDevice failed because the touch did: %v", err)
		}
	}
	if got := warnings(); got != 1 {
		t.Fatalf("20 requests with a failing touch logged %d warnings, want 1", got)
	}
}
