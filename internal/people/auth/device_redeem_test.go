package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// pairingBarrier holds every GetDevicePairing caller until n of them have read, so n overlapping
// redeems all see an approved, unspent pairing before any of them writes.
type pairingBarrier struct {
	store.Store
	n       int
	mu      sync.Mutex
	reads   int
	release chan struct{}
}

func newPairingBarrier(st store.Store, n int) *pairingBarrier {
	return &pairingBarrier{Store: st, n: n, release: make(chan struct{})}
}

func (b *pairingBarrier) GetDevicePairing(ctx context.Context, codeHash string, now time.Time) (identitystore.DevicePairing, error) {
	p, err := b.Store.GetDevicePairing(ctx, codeHash, now)
	b.mu.Lock()
	b.reads++
	if b.reads == b.n {
		close(b.release)
	}
	b.mu.Unlock()
	select {
	case <-b.release:
	case <-time.After(5 * time.Second):
	}
	return p, err
}

// Two or more overlapping redeems of one approved code produce exactly one credential. Every
// caller reads the pairing as approved before any writes, so only an atomic claim can pick a
// single winner; the losers are told the code is gone (store.ErrNotFound), the answer a consumed
// code gives.
func TestConcurrentRedeemMintsOneCredential(t *testing.T) {
	t.Parallel()
	const racers = 8
	_, st, _ := deviceFixture(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	mgr := NewDeviceManager(newPairingBarrier(st, racers), func() time.Time { return now })

	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "Living Room Shield")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}

	type result struct {
		token string
		err   error
	}
	results := make([]result, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, _, err := mgr.Redeem(t.Context(), deviceCode)
			results[i] = result{token, err}
		}()
	}
	wg.Wait()

	wins := 0
	for i, r := range results {
		switch {
		case r.err == nil && r.token != "":
			wins++
		case errors.Is(r.err, store.ErrNotFound):
		default:
			t.Errorf("racer %d: token %q, err %v; want a token or ErrNotFound", i, r.token, r.err)
		}
	}
	if wins != 1 {
		t.Errorf("%d redeems succeeded, want exactly 1", wins)
	}
	devices, err := mgr.ListFor(t.Context(), "u-member")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Errorf("%d device tokens exist, want 1", len(devices))
	}
}

// failingClaim makes the atomic claim fail, as a database outage would.
type failingClaim struct {
	store.Store
	err error
}

func (f failingClaim) RedeemDevicePairing(context.Context, string, string, time.Time) (identitystore.DevicePairing, error) {
	return identitystore.DevicePairing{}, f.err
}

// A claim the store could not make is reported, and the device is not handed a credential it
// cannot use. The pairing stays redeemable, so the device's next poll can succeed.
func TestRedeemReportsAFailedClaim(t *testing.T) {
	t.Parallel()
	_, st, _ := deviceFixture(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	boom := errors.New("database is locked")
	broken := NewDeviceManager(failingClaim{Store: st, err: boom}, func() time.Time { return now })
	working := NewDeviceManager(st, func() time.Time { return now })

	deviceCode, userCode, _, err := working.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := working.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := broken.Redeem(t.Context(), deviceCode)
	if !errors.Is(err, boom) || token != "" {
		t.Fatalf("Redeem = (%q, %v), want the store's error and no token", token, err)
	}
	if devices, _ := working.ListFor(t.Context(), "u-member"); len(devices) != 0 {
		t.Fatalf("%d device tokens exist after a failed claim, want 0", len(devices))
	}
	if _, _, err := working.Redeem(t.Context(), deviceCode); err != nil {
		t.Errorf("Redeem after a failed claim = %v, want the pairing still redeemable", err)
	}
}
