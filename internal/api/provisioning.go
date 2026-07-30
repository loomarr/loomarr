package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
)

// registerProvisioning mounts the identity-provisioning routes (§11): first-run
// bootstrap (unauthenticated, self-gating) and explicit media-server import
// (admin-only). These are the ONLY routes that create users.
func (s *Server) registerProvisioning(api huma.API) {
	if s.provision == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "bootstrap", Method: http.MethodPost, Path: "/v1/setup/bootstrap",
		Summary: "Create the first admin", Description: "First-run only: creates the owning local admin. Succeeds exactly once (409 once an admin exists). Unauthenticated by design — it is functional only while no admin exists (§11).",
		Tags: []string{"setup"},
	}, RolePublic), s.bootstrap)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "setup-state", Method: http.MethodGet, Path: "/v1/setup/state",
		Summary: "Whether the install has an owning admin", Description: "Unauthenticated. Reports only whether bootstrap has happened, so the frontend can route a first-run visitor to the wizard instead of a login they cannot pass (§7/§13). Exposes nothing that POST /v1/setup/bootstrap doesn't already reveal by answering 409-vs-created.",
		Tags: []string{"setup"},
	}, RolePublic), s.setupState)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "import-users", Method: http.MethodPost, Path: "/v1/users/import",
		Summary: "Import media-server users", Description: "Admin only. Allowlists the named media-server user ids — the only way a media-server user gains access (§11).",
		Tags: []string{"users"},
	}, RoleAdmin), s.importUsers)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "import-candidates", Method: http.MethodGet, Path: "/v1/users/candidates",
		Summary: "List importable media-server accounts", Description: "Admin only. The media-server accounts available to import, each flagged whether it is already allowlisted (§11). Read-only — listing never provisions.",
		Tags: []string{"users"},
	}, RoleAdmin), s.importCandidates)
}

// ImportCandidate is one media-server account an admin may allowlist (§11).
type ImportCandidate struct {
	ID       string `json:"id" doc:"Media-server user id — the value POST /v1/users/import takes."`
	Name     string `json:"name"`
	IsAdmin  bool   `json:"isAdmin" doc:"Admin on the media server; import may map this to a Loomarr admin."`
	Disabled bool   `json:"disabled" doc:"Disabled on the media server."`
	Imported bool   `json:"imported" doc:"Already allowlisted in Loomarr."`
}

type importCandidatesOutput struct {
	Body struct {
		Candidates []ImportCandidate `json:"candidates"`
	}
}

// importCandidates gives the UI something real to pick from. Without it, importing
// would require knowing raw media-server user ids (§11 read side).
func (s *Server) importCandidates(ctx context.Context, _ *struct{}) (*importCandidatesOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.provision == nil {
		return nil, errNotImplemented("Media server not connected", "Connect a media server in Settings to import its users.")
	}
	found, err := s.provision.Candidates(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't reach the media server",
			"Loomarr couldn't list users from your media server. Check its connection in Settings and try again.", err)
	}
	out := &importCandidatesOutput{}
	out.Body.Candidates = make([]ImportCandidate, 0, len(found))
	for _, c := range found {
		out.Body.Candidates = append(out.Body.Candidates, ImportCandidate{
			ID: c.ID, Name: c.Name, IsAdmin: c.IsAdmin, Disabled: c.Disabled, Imported: c.Imported,
		})
	}
	return out, nil
}

type setupStateOutput struct {
	Body struct {
		Bootstrapped bool `json:"bootstrapped" doc:"Whether an owning admin exists. False ⇒ the install is unclaimed and the wizard's bootstrap step is the only way in (§11)."`
		DevLogin     bool `json:"devLogin" doc:"Whether this server was started with LOOMARR_DEV_LOGIN=1, so the login screen may offer the credential-free dev sign-in (§11). False on every shipped install — the route does not exist there. Reporting it leaks nothing: an operator can already discover it by calling the endpoint."`
		// SSO says whether the login screen should offer a sign-in-with button. Answered by
		// the SERVER, on the same request the bootstrap guard already makes, so the button
		// appears only when the route it points at actually exists — the same posture as
		// devLogin above. Reporting it leaks nothing an unauthenticated caller could not
		// learn by following the redirect.
		SSO bool `json:"sso" doc:"Whether single sign-on is configured, so the login screen may offer it (§11, V8)."`
	}
}

// setupState answers the one question a first-run visitor's browser must be able to
// ask before it holds any session (§7). No requireAdmin — that is the point: gating
// it would reproduce the dead end it exists to remove.
//
// A nil provisioner reports bootstrapped=true. That reads backwards, but it is the
// safe direction: provisioning is unwired only in schema-only/partial servers, and
// claiming to be UNclaimed there would route every visitor into a wizard whose
// bootstrap step cannot work.
func (s *Server) setupState(ctx context.Context, _ *struct{}) (*setupStateOutput, error) {
	out := &setupStateOutput{}
	out.Body.SSO = s.sso != nil && s.sso.Available()
	// Mirrors exactly what registerAuth mounted, so the login screen can never offer a
	// dev-login link that 404s (§11).
	out.Body.DevLogin = s.devLogin
	if s.provision == nil {
		out.Body.Bootstrapped = true
		return out, nil
	}
	done, err := s.provision.Bootstrapped(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't check setup state",
			"Loomarr couldn't tell whether first-run setup is complete. Try again in a moment.", err)
	}
	out.Body.Bootstrapped = done
	return out, nil
}

type bootstrapInput struct {
	Body struct {
		Username string `json:"username" doc:"The owning admin's local username."`
		Password string `json:"password" doc:"The owning admin's password (bcrypt-hashed at rest)."`
	}
}

type bootstrapOutput struct {
	Body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
}

// bootstrap creates the first local admin (§11). No requireAdmin: it is gated
// internally on "no admin exists yet" — a second call 409s.
func (s *Server) bootstrap(ctx context.Context, in *bootstrapInput) (*bootstrapOutput, error) {
	u, err := s.provision.Bootstrap(ctx, in.Body.Username, in.Body.Password)
	if errors.Is(err, auth.ErrBootstrapClosed) {
		return nil, errConflict("Already set up", "An admin account already exists, so first-run setup is closed.")
	}
	if errors.Is(err, auth.ErrInvalidBootstrap) {
		return nil, errUnprocessable("Details required", "Enter both a username and a password to create the admin account.")
	}
	if err != nil {
		return nil, err
	}
	out := &bootstrapOutput{}
	out.Body.ID, out.Body.Name, out.Body.Role = u.ID, u.Name, string(u.Role)
	return out, nil
}

type importUsersInput struct {
	Body struct {
		IDs       []string `json:"ids" doc:"Media-server user ids to allowlist."`
		MakeAdmin bool     `json:"makeAdmin,omitempty" doc:"Grant admin to imported users that are media-server admins (default member)."`
	}
}

type importUsersOutput struct {
	Body struct {
		Imported int `json:"imported" doc:"How many rows were created/updated."`
	}
}

// importUsers allowlists media-server users (§11). Admin-only.
func (s *Server) importUsers(ctx context.Context, in *importUsersInput) (*importUsersOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	n, err := s.provision.Import(ctx, in.Body.IDs, in.Body.MakeAdmin)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Import failed",
			"Loomarr couldn't import the selected users. Check the media-server connection and try again.", err)
	}
	out := &importUsersOutput{}
	out.Body.Imported = n
	return out, nil
}
