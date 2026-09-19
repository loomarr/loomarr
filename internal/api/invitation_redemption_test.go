package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/testkit"
)

type invitationRedemptionHarness struct {
	*apiHarness
	Grant string
}

func newInvitationRedemptionHarness(t *testing.T) *invitationRedemptionHarness {
	t.Helper()
	at := time.Date(2030, 3, 17, 12, 0, 0, 0, time.UTC)
	const grant = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	base := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		admin := invitation.NewService(defaults.Store, testkit.LibraryAccountResolver{}, func() string { return "invitation-1" },
			func() (string, error) { return grant, nil }, func() time.Time { return at })
		created, err := admin.Create(t.Context(), invitation.CreateCommand{
			Kind: invitation.KindLocal, Username: "Ada", Role: invitation.RoleAdmin,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Regenerate(t.Context(), created.ID, invitation.ConveyanceQR); err != nil {
			t.Fatal(err)
		}
		manager := auth.NewManager(defaults.Store, time.Hour, func() time.Time { return at.Add(time.Minute) })
		redemption := auth.NewInvitationRedemptionService(defaults.Store, nil, manager, func() string { return "user-ada" },
			func() time.Time { return at.Add(time.Minute) })
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: api.NewSessionAuthorizer(manager, "break-glass"),
			Login:    auth.NewLoginService(nil, defaults.Store, manager, nil, func() time.Time { return at.Add(time.Minute) }),
			Sessions: manager, InvitationRedemption: redemption, CookieSecure: "false",
		})
	})
	return &invitationRedemptionHarness{apiHarness: base, Grant: grant}
}

func TestInvitationRedemption_PublicPreviewRequiresExplicitSubmitAndIssuesSession(t *testing.T) {
	harness := newInvitationRedemptionHarness(t)
	ctx := context.Background()

	preview := harness.Do(http.MethodPost, "/v1/invitations/preview", "",
		`{"grant":"`+harness.Grant+`"}`)
	previewBytes, _ := io.ReadAll(preview.Body)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.StatusCode, previewBytes)
	}
	var previewBody struct {
		Kind           string `json:"kind"`
		Username       string `json:"username"`
		Role           string `json:"role"`
		CredentialPath string `json:"credentialPath"`
	}
	if err := json.Unmarshal(previewBytes, &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.Kind != "local" || previewBody.Username != "Ada" || previewBody.Role != "admin" ||
		previewBody.CredentialPath != "local_password" {
		t.Fatalf("preview body = %+v", previewBody)
	}
	if _, err := harness.Store.GetUser(ctx, "user-ada"); err == nil {
		t.Fatal("preview created the invited user")
	}

	weak := harness.Do(http.MethodPost, "/v1/invitations/redeem", "",
		`{"grant":"`+harness.Grant+`","password":"short"}`)
	weakBody, _ := io.ReadAll(weak.Body)
	if weak.StatusCode != http.StatusUnprocessableEntity || strings.Contains(string(weakBody), harness.Grant) {
		t.Fatalf("weak password = %d %s; want safe 422 without bearer", weak.StatusCode, weakBody)
	}

	redeemed := harness.Do(http.MethodPost, "/v1/invitations/redeem", "",
		`{"grant":"`+harness.Grant+`","password":"correct horse battery staple"}`)
	body, _ := io.ReadAll(redeemed.Body)
	if redeemed.StatusCode != http.StatusOK {
		t.Fatalf("redeem = %d %s", redeemed.StatusCode, body)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range redeemed.Cookies() {
		if cookie.Name == auth.CookieName && cookie.Value != "" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %+v", sessionCookie)
	}
	request, _ := http.NewRequest(http.MethodGet, harness.Server.URL+"/v1/auth/me", strings.NewReader(""))
	request.AddCookie(sessionCookie)
	me, err := harness.Server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if me.StatusCode != http.StatusOK {
		contents, _ := io.ReadAll(me.Body)
		t.Fatalf("redeemed session /me = %d %s", me.StatusCode, contents)
	}

	reused := harness.Do(http.MethodPost, "/v1/invitations/preview", "",
		`{"grant":"`+harness.Grant+`"}`)
	reusedBody, _ := io.ReadAll(reused.Body)
	if reused.StatusCode != http.StatusGone || strings.Contains(string(reusedBody), harness.Grant) {
		t.Fatalf("reused bearer = %d %s; want safe 410 without bearer", reused.StatusCode, reusedBody)
	}
}
