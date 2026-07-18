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
	huma.Register(api, huma.Operation{
		OperationID: "bootstrap", Method: http.MethodPost, Path: "/v1/setup/bootstrap",
		Summary: "Create the first admin", Description: "First-run only: creates the owning local admin. Succeeds exactly once (409 once an admin exists). Unauthenticated by design — it is functional only while no admin exists (§11).",
		Tags: []string{"setup"},
	}, s.bootstrap)

	huma.Register(api, huma.Operation{
		OperationID: "import-users", Method: http.MethodPost, Path: "/v1/users/import",
		Summary: "Import media-server users", Description: "Admin only. Allowlists the named media-server user ids — the only way a media-server user gains access (§11).",
		Tags: []string{"users"},
	}, s.importUsers)

	huma.Register(api, huma.Operation{
		OperationID: "import-candidates", Method: http.MethodGet, Path: "/v1/users/candidates",
		Summary: "List importable media-server accounts", Description: "Admin only. The media-server accounts available to import, each flagged whether it is already allowlisted (§11). Read-only — listing never provisions.",
		Tags: []string{"users"},
	}, s.importCandidates)
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
		return nil, huma.Error501NotImplemented("provisioning not configured")
	}
	found, err := s.provision.Candidates(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("could not list media-server users", err)
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
		return nil, huma.Error409Conflict("an admin already exists — bootstrap is closed")
	}
	if errors.Is(err, auth.ErrInvalidBootstrap) {
		return nil, huma.Error422UnprocessableEntity("username and password are required")
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
		return nil, huma.Error502BadGateway("import failed", err)
	}
	out := &importUsersOutput{}
	out.Body.Imported = n
	return out, nil
}
