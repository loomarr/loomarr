package people

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// redemptionFake answers each redemption step with a fixed outcome.
type redemptionFake struct {
	kind       access.Kind
	previewErr error
	redeemErr  error
}

func (f redemptionFake) Preview(context.Context, string) (access.Invitation, error) {
	return access.Invitation{Kind: f.kind}, f.previewErr
}

func (f redemptionFake) RedeemLocal(context.Context, string, string) (string, time.Time, identitystore.User, error) {
	return "token", time.Time{}, identitystore.User{ID: "u"}, f.redeemErr
}

func (f redemptionFake) RedeemLibrary(context.Context, string, string, string) (string, time.Time, identitystore.User, error) {
	return "token", time.Time{}, identitystore.User{ID: "u"}, f.redeemErr
}

func redemptionHandlers(f redemptionFake) *Handlers {
	return &Handlers{invitationRedemption: f, invitationRedeemLimiter: newInvitationRedeemLimiter()}
}

func redeemInput(username string) *redeemInvitationInput {
	in := &redeemInvitationInput{}
	in.Body.Grant, in.Body.Username, in.Body.Password = "grant", username, "correct horse"
	return in
}

// guessesThatFit is how many attempts the account bucket takes before refusing, one source per
// attempt, each failing the Library's credential check.
func guessesThatFit(t *testing.T, s *Handlers, from func(i int) string, account string) int {
	t.Helper()
	failing := *s
	failing.invitationRedemption = redemptionFake{kind: access.KindLibrary, redeemErr: auth.ErrInvalidCredentials}
	n := 0
	for i := 1; i <= 100; i++ {
		_, _, _, err := failing.redeemThrottled(t.Context(), from(i), redeemInput(account))
		if errors.Is(err, auth.ErrInvalidCredentials) || err != nil && !isTooMany(err) {
			n++
		}
	}
	return n
}

func isTooMany(err error) bool {
	return err != nil && err.Error() == tooManyInvitationAttempts().Error()
}

// The redeem gate checked the account bucket on the caller's username before the grant, so anyone
// could spend an invitee's allowance with garbage grants. Only a Library credential the Library
// refuses is a guess; everything else leaves the bucket whole, whichever address family sent it.
func TestRedeemSpendsTheAccountBucketOnlyOnAFailedCredential(t *testing.T) {
	ipv4 := func(i int) string { return "192.0.2." + strconv.Itoa(i) }
	ipv6 := func(i int) string { return "2001:db8:7:" + strconv.Itoa(i) + "::1" }
	notGuesses := map[string]redemptionFake{
		"an invalid grant":           {kind: access.KindLibrary, previewErr: auth.ErrInvalidInvitation},
		"an expired or used grant":   {kind: access.KindLocal, previewErr: auth.ErrInvalidInvitation},
		"a weak password":            {kind: access.KindLocal, redeemErr: auth.ErrWeakPassword},
		"the Library unreachable":    {kind: access.KindLibrary, redeemErr: auth.ErrInvitationProviderUnavailable},
		"a store failure":            {kind: access.KindLibrary, redeemErr: errors.New("store down")},
		"a redemption that succeeds": {kind: access.KindLocal},
	}
	for name, fake := range notGuesses {
		for family, from := range map[string]func(int) string{"IPv4": ipv4, "IPv6": ipv6} {
			t.Run(name+" "+family, func(t *testing.T) {
				s := redemptionHandlers(fake)
				for i := 1; i <= 100; i++ {
					_, _, _, err := s.redeemThrottled(t.Context(), from(i), redeemInput("Ada"))
					if isTooMany(err) {
						t.Fatalf("attempt %d from a fresh source was refused: %q spent the account bucket", i, name)
					}
				}
				// The invitee's own failed guesses still have the whole bucket.
				if n := guessesThatFit(t, s, func(i int) string { return "203.0.113." + strconv.Itoa(i) }, "Ada"); n != 10 {
					t.Fatalf("%d refused-credential attempts fit after %q, want the full burst of 10", n, name)
				}
			})
		}
	}

	t.Run("a refused Library credential is charged, from either family", func(t *testing.T) {
		for family, from := range map[string]func(int) string{"IPv4": ipv4, "IPv6": ipv6} {
			s := redemptionHandlers(redemptionFake{})
			if n := guessesThatFit(t, s, from, "Ada"); n != 10 {
				t.Errorf("%s: %d guesses fit across 100 sources, want 10", family, n)
			}
		}
	})

	t.Run("the account is the case-folded username, and another account has its own bucket", func(t *testing.T) {
		s := redemptionHandlers(redemptionFake{})
		guessesThatFit(t, s, ipv4, "Ada")
		if _, _, _, err := s.redeemThrottled(t.Context(), "198.51.100.1", redeemInput(" ADA ")); !isTooMany(err) {
			t.Errorf("a respelled username reached a drained bucket: %v", err)
		}
		if _, _, _, err := s.redeemThrottled(t.Context(), "198.51.100.1", redeemInput("bob")); isTooMany(err) {
			t.Errorf("draining one account throttled another: %v", err)
		}
	})
}
