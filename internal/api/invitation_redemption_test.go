package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestInvitationRedemption_PublicPreviewRequiresExplicitSubmitAndIssuesSession(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/redemption.db")
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	at := time.Date(2030, 3, 17, 12, 0, 0, 0, time.UTC)
	const bearer = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	admin := invitation.NewService(st, testkit.LibraryAccountResolver{}, func() string { return "invitation-1" },
		func() (string, error) { return bearer, nil }, func() time.Time { return at })
	created, err := admin.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLocal, Username: "Ada", Role: invitation.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Regenerate(ctx, created.ID, invitation.ConveyanceQR); err != nil {
		t.Fatal(err)
	}
	mgr := auth.NewManager(st, time.Hour, func() time.Time { return at.Add(time.Minute) })
	redemption := auth.NewInvitationRedemptionService(st, nil, mgr, func() string { return "user-ada" },
		func() time.Time { return at.Add(time.Minute) })
	handler := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: api.NewSessionAuthorizer(mgr, "break-glass"),
		Login:    auth.NewLoginService(nil, st, mgr, nil, func() time.Time { return at.Add(time.Minute) }),
		Sessions: mgr, InvitationRedemption: redemption, CookieSecure: "false",
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	preview := do(t, server, http.MethodPost, "/v1/invitations/preview", "",
		`{"grant":"`+bearer+`"}`)
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
	if _, err := st.GetUser(ctx, "user-ada"); err == nil {
		t.Fatal("preview created the invited user")
	}

	weak := do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
		`{"grant":"`+bearer+`","password":"short"}`)
	weakBody, _ := io.ReadAll(weak.Body)
	if weak.StatusCode != http.StatusUnprocessableEntity || strings.Contains(string(weakBody), bearer) {
		t.Fatalf("weak password = %d %s; want safe 422 without bearer", weak.StatusCode, weakBody)
	}

	redeemed := do(t, server, http.MethodPost, "/v1/invitations/redeem", "",
		`{"grant":"`+bearer+`","password":"correct horse battery staple"}`)
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
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/me", strings.NewReader(""))
	request.AddCookie(sessionCookie)
	me, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if me.StatusCode != http.StatusOK {
		contents, _ := io.ReadAll(me.Body)
		t.Fatalf("redeemed session /me = %d %s", me.StatusCode, contents)
	}

	reused := do(t, server, http.MethodPost, "/v1/invitations/preview", "",
		`{"grant":"`+bearer+`"}`)
	reusedBody, _ := io.ReadAll(reused.Body)
	if reused.StatusCode != http.StatusGone || strings.Contains(string(reusedBody), bearer) {
		t.Fatalf("reused bearer = %d %s; want safe 410 without bearer", reused.StatusCode, reusedBody)
	}
}
