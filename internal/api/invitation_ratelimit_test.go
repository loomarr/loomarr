package api_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/api/people"
	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// countingRedemption is an InvitationRedemptionService that records how often each credential
// check was reached. A library redemption asks the media server to authenticate a username and
// password the caller chose, so every call that gets through is a password guess against it.
type countingRedemption struct {
	kind                                 access.Kind
	previews, localRedeems, libraryTries atomic.Int64
}

func (c *countingRedemption) Preview(context.Context, string) (access.Invitation, error) {
	c.previews.Add(1)
	return access.Invitation{Kind: c.kind, Username: "Ada", Role: access.RoleMember}, nil
}

func (c *countingRedemption) RedeemLocal(context.Context, string, string) (string, time.Time, identitystore.User, error) {
	c.localRedeems.Add(1)
	return "", time.Time{}, identitystore.User{}, auth.ErrInvalidInvitation
}

func (c *countingRedemption) RedeemLibrary(context.Context, string, string, string) (string, time.Time, identitystore.User, error) {
	c.libraryTries.Add(1)
	return "", time.Time{}, identitystore.User{}, auth.ErrInvalidCredentials
}

func newRedemptionHarness(t *testing.T, service people.InvitationRedemptionService) *apiHarness {
	t.Helper()
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log,
			People: api.PeopleOptions{InvitationRedemption: service},
		})
	})
}

const rateLimitBearer = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// Redeeming a library invitation authenticates a caller-chosen username and password against the
// media server, with no credential of the caller's own. It was the one public credential route with
// no limiter at all.
func TestInvitationRedeemLibraryIsThrottled(t *testing.T) {
	t.Run("one account from one source", func(t *testing.T) {
		service := &countingRedemption{kind: access.KindLibrary}
		server := newRedemptionHarness(t, service).Server
		var limited int
		for range 50 {
			response := do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
				`{"grant":"`+rateLimitBearer+`","username":"ada","password":"a-guess-of-the-password"}`)
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode == http.StatusTooManyRequests {
				limited++
				if strings.Contains(string(body), rateLimitBearer) || strings.Contains(string(body), "a-guess") {
					t.Fatalf("429 body echoes a secret: %s", body)
				}
			}
		}
		if got := service.libraryTries.Load(); got > 5 {
			t.Fatalf("%d guesses at one account from one source reached the media server, want at most 5", got)
		}
		if limited < 45 {
			t.Fatalf("only %d of 50 attempts were refused with 429", limited)
		}
	})

	t.Run("one source rotating usernames", func(t *testing.T) {
		service := &countingRedemption{kind: access.KindLibrary}
		server := newRedemptionHarness(t, service).Server
		for i := range 100 {
			do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
				fmt.Sprintf(`{"grant":%q,"username":"user%d","password":"a-guess-of-the-password"}`, rateLimitBearer, i))
		}
		if got := service.libraryTries.Load(); got > 10 {
			t.Fatalf("%d guesses across 100 usernames reached the media server, want at most 10", got)
		}
	})

	t.Run("a refused attempt never reaches the invitation store", func(t *testing.T) {
		service := &countingRedemption{kind: access.KindLibrary}
		server := newRedemptionHarness(t, service).Server
		for range 100 {
			do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
				`{"grant":"`+rateLimitBearer+`","username":"ada","password":"a-guess-of-the-password"}`)
		}
		if got := service.previews.Load(); got > 5 {
			t.Fatalf("%d refused redemptions still looked up the grant, want at most 5", got)
		}
	})
}

// A local invitation has no username, so only the source can hold it: the same bound, from the
// same source, however the grant is varied.
func TestInvitationRedeemLocalIsThrottled(t *testing.T) {
	service := &countingRedemption{kind: access.KindLocal}
	server := newRedemptionHarness(t, service).Server
	for i := range 100 {
		grant := fmt.Sprintf("%064x", i)
		do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
			`{"grant":"`+grant+`","password":"correct horse battery staple"}`)
	}
	if got := service.localRedeems.Load(); got > 10 {
		t.Fatalf("%d redemptions from one source went through, want at most 10", got)
	}
}

func TestInvitationPreviewIsThrottled(t *testing.T) {
	service := &countingRedemption{kind: access.KindLocal}
	server := newRedemptionHarness(t, service).Server
	ok, limited := 0, 0
	for i := range 100 {
		response := do(t, server, http.MethodPost, "/v1/invitations/preview", "",
			fmt.Sprintf(`{"grant":"%064x"}`, i))
		switch response.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("preview status = %d", response.StatusCode)
		}
	}
	if ok == 0 || ok > 10 || limited < 90 {
		t.Fatalf("100 previews from one source: %d allowed and %d refused, want 1 to 10 allowed", ok, limited)
	}
	if got := service.previews.Load(); int(got) != ok {
		t.Fatalf("service saw %d previews but %d were answered: a refused one reached it", got, ok)
	}
}

// Previewing a link before redeeming it is the normal flow, so the two share nothing: a person who
// opens the invitation page and then submits is never refused for having looked first, and spending
// one budget leaves the other.
func TestInvitationPreviewAndRedeemHaveSeparateBudgets(t *testing.T) {
	service := &countingRedemption{kind: access.KindLocal}
	server := newRedemptionHarness(t, service).Server
	for range 50 {
		do(t, server, http.MethodPost, "/v1/invitations/preview", "", `{"grant":"`+rateLimitBearer+`"}`)
	}
	redeem := do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
		`{"grant":"`+rateLimitBearer+`","password":"correct horse battery staple"}`)
	if redeem.StatusCode == http.StatusTooManyRequests {
		t.Fatal("redeeming was refused because previews had used the budget")
	}

	service = &countingRedemption{kind: access.KindLocal}
	server = newRedemptionHarness(t, service).Server
	for range 50 {
		do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
			`{"grant":"`+rateLimitBearer+`","password":"correct horse battery staple"}`)
	}
	preview := do(t, server, http.MethodPost, "/v1/invitations/preview", "", `{"grant":"`+rateLimitBearer+`"}`)
	if preview.StatusCode == http.StatusTooManyRequests {
		t.Fatal("previewing was refused because redemptions had used the budget")
	}
}
