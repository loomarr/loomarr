package auth

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// touchCounter counts the use-recording writes that reach the real store.
type touchCounter struct {
	store.Store
	sessions, devices int
}

func (c *touchCounter) TouchSession(ctx context.Context, tokenHash string, seen identitystore.SessionSeen) error {
	c.sessions++
	return c.Store.TouchSession(ctx, tokenHash, seen)
}

func (c *touchCounter) TouchDeviceToken(ctx context.Context, tokenHash string, at time.Time) error {
	c.devices++
	return c.Store.TouchDeviceToken(ctx, tokenHash, at)
}

// A burst of requests on one session records its use once, not once per request (#2446). A new
// minute, or a different client, records it again.
func TestResolveRecordsASessionUseAtMostOncePerMinute(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := &touchCounter{Store: newStore(t)}
	if err := st.UpsertUser(ctx, identitystore.User{ID: "u1", Name: "A", Role: identitystore.RoleMember}); err != nil {
		t.Fatal(err)
	}
	clock := now
	mgr := NewManager(st, time.Hour, func() time.Time { return clock })
	token, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}

	for range 20 {
		if _, err := mgr.Resolve(ctx, token, "Firefox on macOS"); err != nil {
			t.Fatal(err)
		}
	}
	if st.sessions != 1 {
		t.Fatalf("20 requests in the same instant wrote the session %d times, want 1", st.sessions)
	}

	clock = clock.Add(30 * time.Second)
	if _, err := mgr.Resolve(ctx, token, "Safari on iPhone"); err != nil {
		t.Fatal(err)
	}
	if st.sessions != 2 {
		t.Fatalf("a different client did not record its use: %d writes, want 2", st.sessions)
	}

	clock = clock.Add(59 * time.Second)
	if _, err := mgr.Resolve(ctx, token, "Safari on iPhone"); err != nil {
		t.Fatal(err)
	}
	if st.sessions != 2 {
		t.Fatalf("a request 59s after the last recorded use wrote: %d writes in all, want 2", st.sessions)
	}
	clock = clock.Add(time.Second)
	if _, err := mgr.Resolve(ctx, token, "Safari on iPhone"); err != nil {
		t.Fatal(err)
	}
	if st.sessions != 3 {
		t.Fatalf("a request a minute after the last recorded use wrote %d times in all, want 3", st.sessions)
	}
	sess, err := st.GetSession(ctx, hashToken(token), clock)
	if err != nil {
		t.Fatal(err)
	}
	if !sess.LastSeenAt.Equal(clock) || !sess.ExpiresAt.Equal(clock.Add(time.Hour)) || sess.ClientLabel != "Safari on iPhone" {
		t.Errorf("session = last seen %s, expires %s, client %q; want %s, %s, Safari on iPhone",
			sess.LastSeenAt, sess.ExpiresAt, sess.ClientLabel, clock, clock.Add(time.Hour))
	}
}

// A paired device's use is recorded (it never was before #2446), throttled the same way.
func TestResolveDeviceRecordsItsUseAtMostOncePerMinute(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	base := newStore(t)
	st := &touchCounter{Store: base}
	at := time.Unix(1_700_000_000, 0).UTC()
	mgr := NewDeviceManager(st, func() time.Time { return at })
	if err := st.UpsertUser(ctx, identitystore.User{ID: "u-member", Name: "member", Role: identitystore.RoleMember}); err != nil {
		t.Fatal(err)
	}
	deviceCode, userCode, _, err := mgr.StartPairing(ctx, "Living Room Shield")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(ctx, userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := mgr.Redeem(ctx, deviceCode)
	if err != nil {
		t.Fatal(err)
	}

	// Redeeming stamps the first use, so requests in the same minute record nothing more.
	if _, err := mgr.ResolveDevice(ctx, token); err != nil {
		t.Fatal(err)
	}
	if st.devices != 0 {
		t.Fatalf("a request in the minute the device paired wrote %d times, want 0", st.devices)
	}
	at = at.Add(time.Minute)
	for range 20 {
		if _, err := mgr.ResolveDevice(ctx, token); err != nil {
			t.Fatal(err)
		}
	}
	if st.devices != 1 {
		t.Fatalf("20 device requests a minute later wrote %d times, want 1", st.devices)
	}
	dt, err := base.GetDeviceToken(ctx, hashToken(token))
	if err != nil {
		t.Fatal(err)
	}
	if !dt.LastSeenAt.Equal(at) {
		t.Errorf("device last seen = %s, want %s", dt.LastSeenAt, at)
	}
	seen, err := base.UserLastSeen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !seen["u-member"].Equal(at) {
		t.Errorf("owner last seen = %s, want the device's use at %s", seen["u-member"], at)
	}
}
