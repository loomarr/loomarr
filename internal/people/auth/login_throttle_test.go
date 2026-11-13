package auth

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// loginThrottleFixture is a local account "admin" (password "owner-pass"), a disabled one
// ("gone", same password), and a LoginService behind a login limiter on a fake clock.
func loginThrottleFixture(t *testing.T) (*LoginService, *SourceLimiter, *time.Time) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	hash, err := hashPassword("owner-pass")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []identitystore.User{
		{ID: "u-admin", Name: "admin", Role: identitystore.RoleAdmin, PasswordHash: hash, CreatedAt: now, UpdatedAt: now},
		{ID: "u-gone", Name: "gone", Role: identitystore.RoleMember, PasswordHash: hash, Disabled: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.UpsertUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	clock := now
	limiter := NewLoginLimiter()
	sourceLimiterClock(limiter, &clock)
	mgr := NewManager(st, time.Hour, func() time.Time { return clock })
	return NewLoginService(nil, st, mgr, limiter, func() time.Time { return clock }), limiter, &clock
}

func loginFrom(svc *LoginService, ip, username, password string) error {
	_, _, _, err := svc.Login(context.Background(), username, password, LoginRateKey(ip, username))
	return err
}

// The re-score probe through the service: guesses at "admin" from rotating addresses drain the
// account bucket, and the owner, from an address that has signed in, still gets in every time.
func TestLoginOwnerSignsInWhileTheAccountIsUnderGuessing(t *testing.T) {
	svc, _, clock := loginThrottleFixture(t)
	if err := loginFrom(svc, "192.168.1.20", "admin", "owner-pass"); err != nil {
		t.Fatalf("owner's first sign-in: %v", err)
	}
	for i := 1; i <= loginAccountBurst; i++ {
		if err := loginFrom(svc, "203.0.113."+strconv.Itoa(i), "admin", "guess"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("guess %d = %v, want a refused credential", i, err)
		}
	}
	if err := loginFrom(svc, "198.51.100.1", "admin", "guess"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("a further guess from a fresh address = %v, want it throttled", err)
	}
	ownerIn := 0
	for round := range 60 {
		*clock = clock.Add(loginAccountRefill + time.Second) // a whole token has come back
		// The attacker spends everything the bucket has, each guess from a fresh address.
		for k := range 5 {
			if err := loginFrom(svc, "198.51."+strconv.Itoa(round)+"."+strconv.Itoa(k+1), "admin", "guess"); errors.Is(err, ErrRateLimited) {
				break
			}
		}
		*clock = clock.Add(time.Second)
		if loginFrom(svc, "192.168.1.20", "admin", "owner-pass") == nil {
			ownerIn++
		}
	}
	if ownerIn != 60 {
		t.Fatalf("owner signed in %d of 60 times during the attack, want 60", ownerIn)
	}
	// A new address during the attack waits for a refill: the exemption is earned by a sign-in.
	if err := loginFrom(svc, "2001:db8:55:1::9", "admin", "owner-pass"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("a never-seen address's sign-in during an attack = %v, want it throttled until a refill", err)
	}
}

// Which outcomes of a sign-in are a guess. A wrong password, an unknown user and a disabled
// account's correct password are all the one refused credential and spend the account bucket; a
// success spends nothing, however many addresses it comes from.
func TestLoginChargesTheAccountBucketOnlyForRefusedCredentials(t *testing.T) {
	type attempt struct{ username, password string }
	for name, tc := range map[string]struct {
		attempt attempt
		account string
		charged bool
	}{
		"wrong password":                {attempt{"admin", "nope"}, "admin", true},
		"wrong password, respelled":     {attempt{" Admin ", "nope"}, "admin", true},
		"unknown user":                  {attempt{"nobody", "x"}, "nobody", true},
		"disabled account, right pass":  {attempt{"gone", "owner-pass"}, "gone", true},
		"right password":                {attempt{"admin", "owner-pass"}, "admin", false},
	} {
		for family, source := range map[string]func(i int) string{
			"IPv4": func(i int) string { return "192.0.2." + strconv.Itoa(i) },
			"IPv6": func(i int) string { return "2001:db8:8:" + strconv.Itoa(i) + "::1" },
		} {
			t.Run(name+" "+family, func(t *testing.T) {
				svc, limiter, clock := loginThrottleFixture(t)
				for i := 1; i <= 40; i++ {
					*clock = clock.Add(5 * time.Second)
					err := loginFrom(svc, source(i), tc.attempt.username, tc.attempt.password)
					if errors.Is(err, ErrRateLimited) && !tc.charged {
						t.Fatalf("attempt %d throttled: a %s spent the account bucket", i, name)
					}
				}
				fits := 0
				for i := 100; i < 160; i++ {
					if a, ok := limiter.Admit(LoginRateKey(source(i), tc.account)); ok {
						a.Failed()
						fits++
					}
				}
				// 40 refused credentials 5 s apart drained the burst, less what 200 s refilled.
				if tc.charged && fits > 8 {
					t.Fatalf("%d further guesses fit after 40 refused credentials, want the bucket drained", fits)
				}
				if !tc.charged && fits != loginAccountBurst {
					t.Fatalf("%d guesses fit after %s, want the whole burst of %d", fits, name, loginAccountBurst)
				}
			})
		}
	}
}

// A limiter that cannot hold an attempt open (a plain RateLimiter) keeps its old meaning: every
// attempt spends. An attempt that fails for a reason that is not a credential (the store is down)
// is not charged on the limiter that can tell.
func TestLoginAbandonsAnAttemptThatNeverReachedACredentialCheck(t *testing.T) {
	svc, limiter, _ := loginThrottleFixture(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 1; i <= 40; i++ {
		_, _, _, err := svc.Login(cancelled, "admin", "owner-pass", LoginRateKey("192.0.2."+strconv.Itoa(i), "admin"))
		if errors.Is(err, ErrRateLimited) {
			t.Fatalf("attempt %d throttled: a failure that is not a refused credential spent the account bucket", i)
		}
	}
	fits := 0
	for i := 100; i < 160; i++ {
		if a, ok := limiter.Admit(LoginRateKey("192.0.2."+strconv.Itoa(i), "admin")); ok {
			a.Failed()
			fits++
		}
	}
	if fits != loginAccountBurst {
		t.Fatalf("%d guesses fit, want the whole burst of %d", fits, loginAccountBurst)
	}
}
