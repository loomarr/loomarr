package auth

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// attemptFrom is one credential attempt on account from clientIP: admitted or not, and the
// admission to resolve when it was.
func attemptFrom(l *SourceLimiter, clientIP, account string) (*Admission, bool) {
	return l.Admit(LoginRateKey(clientIP, account))
}

// The probe of the 2026-11-13 re-score. The account-wide bucket was spent on every attempt before
// the password was looked at, so an attacker who knew the username and spent one attempt per refill
// held the owner at 2 of 60 attempts. Only a failed credential may spend it, and an address that has
// signed in as the account is not held by it.
func TestAccountBucketCannotBeHeldEmptyAgainstTheOwner(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	limiter := NewLoginLimiter()
	sourceLimiterClock(limiter, &now)

	// The owner has signed in from the household before.
	adm, ok := attemptFrom(limiter, "192.168.1.20", "admin")
	if !ok {
		t.Fatal("the owner's first sign-in was refused")
	}
	adm.Succeeded()

	// The attacker spends the whole account bucket on failures from rotating sources.
	for i := 1; i <= loginAccountBurst; i++ {
		a, ok := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "admin")
		if !ok {
			t.Fatalf("attacker attempt %d refused inside the burst", i)
		}
		a.Failed()
	}
	if _, ok := attemptFrom(limiter, "198.51.100.1", "admin"); ok {
		t.Fatal("a fresh source reached an account whose bucket the guesses had spent")
	}

	ownerIn := 0
	for round := range 60 {
		now = now.Add(loginAccountRefill)
		if a, ok := attemptFrom(limiter, "198.51.100."+strconv.Itoa(round+10), "admin"); ok { // the refilled token
			a.Failed()
		}
		now = now.Add(time.Second)
		if a, ok := attemptFrom(limiter, "192.168.1.20", "admin"); ok {
			ownerIn++
			a.Succeeded()
		}
	}
	if ownerIn != 60 {
		t.Fatalf("the owner was admitted %d of 60 times while the account was under attack, want 60", ownerIn)
	}
}

// The account bucket counts guesses. A success is not one, and neither is an attempt that never
// reached a credential check, so neither may spend it.
func TestAccountBucketIsSpentOnlyByFailedCredentials(t *testing.T) {
	spend := map[string]func(*Admission){
		"succeeded": func(a *Admission) { a.Succeeded() },
		"abandoned": func(a *Admission) { a.Abandon() },
	}
	for name, resolve := range spend {
		t.Run(name+" attempts from 100 sources leave the whole bucket", func(t *testing.T) {
			limiter := NewLoginLimiter()
			for i := 1; i <= 100; i++ {
				a, ok := attemptFrom(limiter, "192.0.2."+strconv.Itoa(i), "admin")
				if !ok {
					t.Fatalf("attempt %d refused though none before it was a failed guess", i)
				}
				resolve(a)
			}
			failures := 0
			for i := 101; i <= 200; i++ {
				a, ok := attemptFrom(limiter, "192.0.2."+strconv.Itoa(i), "admin")
				if !ok {
					break
				}
				a.Failed()
				failures++
			}
			if failures != loginAccountBurst {
				t.Fatalf("%d failed guesses fit after %s attempts, want the full burst of %d", failures, name, loginAccountBurst)
			}
		})
	}
}

// Rotating sources buys no guesses, on either address family: the cost of a guess is the same.
func TestFailedGuessesAreCappedAcrossRotatingSources(t *testing.T) {
	sources := map[string]func(i int) string{
		"IPv4 pool":                func(i int) string { return "192.0.2." + strconv.Itoa(i) },
		"IPv6 /64s inside one /56": func(i int) string { return "2001:db8:1:" + strconv.Itoa(i) + "::1" },
		"IPv4 and IPv6 mixed": func(i int) string {
			if i%2 == 0 {
				return "198.51.100." + strconv.Itoa(i)
			}
			return "2001:db8:2:" + strconv.Itoa(i) + "::1"
		},
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			limiter := NewLoginLimiter()
			guesses := 0
			for i := 1; i <= 80; i++ {
				if a, ok := attemptFrom(limiter, source(i), "admin"); ok {
					a.Failed()
					guesses++
				}
			}
			if guesses != loginAccountBurst {
				t.Fatalf("%d failed guesses from 80 sources, want %d", guesses, loginAccountBurst)
			}
		})
	}
}

// Attempts in flight count against the bucket before they resolve, so a burst of concurrent
// attempts cannot all see the one token that is left and all be let through.
func TestConcurrentAttemptsCannotOverrunTheAccountBucket(t *testing.T) {
	limiter := NewLoginLimiter()
	var mu sync.Mutex
	admitted := 0
	var wg sync.WaitGroup
	release := make(chan struct{})
	for i := 1; i <= 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, ok := attemptFrom(limiter, "2001:db8:3:"+strconv.Itoa(i)+"::1", "admin")
			if !ok {
				return
			}
			mu.Lock()
			admitted++
			mu.Unlock()
			<-release // every admitted attempt is still checking its password
			a.Failed()
		}()
	}
	// Let the attempts reach the gate before any resolves.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if admitted > loginAccountBurst {
		t.Fatalf("%d concurrent attempts admitted, want at most %d", admitted, loginAccountBurst)
	}
}

// What the exemption is and is not. It is for an address that has signed in as this account, and
// only for the account bucket: the address and the account-from-this-address buckets still hold it.
func TestSignedInSourceExemptionIsNarrow(t *testing.T) {
	setup := func() (*SourceLimiter, *time.Time) {
		now := time.Unix(1_900_000_000, 0)
		limiter := NewLoginLimiter()
		sourceLimiterClock(limiter, &now)
		for i := 1; i <= loginAccountBurst; i++ {
			a, _ := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "admin")
			a.Failed()
		}
		return limiter, &now
	}
	signIn := func(limiter *SourceLimiter, ip, account string) {
		t.Helper()
		a, ok := attemptFrom(limiter, ip, account)
		if !ok {
			t.Fatalf("setup sign-in from %s refused", ip)
		}
		a.Succeeded()
	}

	t.Run("a never-seen source is still held while the account is drained", func(t *testing.T) {
		limiter, _ := setup()
		if _, ok := attemptFrom(limiter, "192.168.1.20", "admin"); ok {
			t.Fatal("a source that never signed in got past a drained account bucket")
		}
	})

	t.Run("it is per account", func(t *testing.T) {
		limiter := NewLoginLimiter()
		signIn(limiter, "192.168.1.20", "admin")
		for i := 1; i <= loginAccountBurst; i++ {
			a, _ := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "ada")
			a.Failed()
		}
		if _, ok := attemptFrom(limiter, "192.168.1.20", "ada"); ok {
			t.Fatal("signing in as admin exempted the source from ada's account bucket")
		}
	})

	t.Run("it follows the throttle source, so an IPv6 neighbour in the same /64 shares it", func(t *testing.T) {
		limiter := NewLoginLimiter()
		signIn(limiter, "2001:db8:9:1::5", "admin")
		for i := 1; i <= loginAccountBurst; i++ {
			a, _ := attemptFrom(limiter, "2001:db8:9:"+strconv.Itoa(100+i)+"::1", "admin")
			a.Failed()
		}
		if a, ok := attemptFrom(limiter, "2001:db8:9:1:aaaa::7", "admin"); !ok {
			t.Fatal("another address in the signed-in /64 lost the exemption")
		} else {
			a.Succeeded()
		}
		if _, ok := attemptFrom(limiter, "2001:db8:9:2::5", "admin"); ok {
			t.Fatal("a neighbouring /64 shared the exemption")
		}
	})

	t.Run("case and padding of the username do not matter", func(t *testing.T) {
		limiter := NewLoginLimiter()
		signIn(limiter, "192.168.1.20", " Admin ")
		for i := 1; i <= loginAccountBurst; i++ {
			a, _ := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "admin")
			a.Failed()
		}
		if a, ok := attemptFrom(limiter, "192.168.1.20", "ADMIN"); !ok {
			t.Fatal("a respelled username lost the exemption")
		} else {
			a.Succeeded()
		}
	})

	t.Run("the source and pair buckets still hold an exempt source", func(t *testing.T) {
		limiter := NewLoginLimiter()
		signIn(limiter, "192.168.1.20", "admin")
		admitted := 0
		for range 50 {
			if a, ok := attemptFrom(limiter, "192.168.1.20", "admin"); ok {
				a.Failed()
				admitted++
			}
		}
		if admitted > 5 {
			t.Fatalf("a signed-in source made %d guesses in a burst, want the pair burst of 5 to hold it", admitted)
		}
	})

	t.Run("a failure from an exempt source still counts as a guess", func(t *testing.T) {
		limiter := NewLoginLimiter()
		signIn(limiter, "192.168.1.20", "admin")
		a, _ := attemptFrom(limiter, "192.168.1.20", "admin")
		a.Failed()
		guesses := 0
		for i := 1; i <= 50; i++ {
			if b, ok := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "admin"); ok {
				b.Failed()
				guesses++
			}
		}
		if guesses != loginAccountBurst-1 {
			t.Fatalf("%d further guesses fit after one exempt failure, want %d", guesses, loginAccountBurst-1)
		}
	})

	t.Run("it lapses", func(t *testing.T) {
		now := time.Unix(1_900_000_000, 0)
		limiter := NewLoginLimiter()
		sourceLimiterClock(limiter, &now)
		signIn(limiter, "192.168.1.20", "admin")
		now = now.Add(signedInSourceTTL + time.Hour)
		for i := 1; i <= loginAccountBurst; i++ {
			a, _ := attemptFrom(limiter, "203.0.113."+strconv.Itoa(i), "admin")
			a.Failed()
		}
		if _, ok := attemptFrom(limiter, "192.168.1.20", "admin"); ok {
			t.Fatal("a sign-in older than the lifetime still exempted the source")
		}
	})

	t.Run("the memory of sign-ins is bounded", func(t *testing.T) {
		limiter := NewLoginLimiter()
		for i := range 3 * maxSignedInSources {
			limiter.rememberSignedIn("10."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256)+".1", "admin")
		}
		if n := limiter.signedInCount(); n > maxSignedInSources {
			t.Fatalf("%d signed-in sources remembered, want at most %d", n, maxSignedInSources)
		}
	})
}

// An admission is resolved once: a handler that resolves it on two paths cannot refund or charge
// twice.
func TestAdmissionResolvesOnce(t *testing.T) {
	limiter := NewLoginLimiter()
	a, ok := attemptFrom(limiter, "192.0.2.1", "admin")
	if !ok {
		t.Fatal("refused")
	}
	a.Failed()
	a.Failed()
	a.Succeeded()
	a.Abandon()
	guesses := 1
	for i := 2; i <= 60; i++ {
		if b, ok := attemptFrom(limiter, "192.0.2."+strconv.Itoa(i), "admin"); ok {
			b.Failed()
			guesses++
		}
	}
	if guesses != loginAccountBurst {
		t.Fatalf("%d guesses fit, want %d: a resolved admission was charged again", guesses, loginAccountBurst)
	}
	// And the nil admission a no-account key returns is safe to resolve.
	none, ok := limiter.Admit(LoginRateKey("192.0.2.200", ""))
	if !ok {
		t.Fatal("an empty username was refused")
	}
	none.Failed()
	none.Succeeded()
	none.Abandon()
}

// An attempt the source or pair buckets refuse is never admitted, and so never reaches the account
// bucket: one address cannot lock an account out of everyone else's reach.
func TestRefusedAttemptsSpendNothingFromTheAccount(t *testing.T) {
	limiter := NewLoginLimiter()
	for range 1000 {
		if a, ok := attemptFrom(limiter, "203.0.113.9", "admin"); ok {
			a.Failed()
		}
	}
	guesses := 0
	for i := 1; i <= 50; i++ {
		if a, ok := attemptFrom(limiter, "198.51.100."+strconv.Itoa(i), "admin"); ok {
			a.Failed()
			guesses++
		}
	}
	if guesses < loginAccountBurst-5 {
		t.Fatalf("after one source hammered the account, others got %d guesses, want at least %d", guesses, loginAccountBurst-5)
	}
}
