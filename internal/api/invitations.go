package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/store"
)

func (s *Server) registerInvitations(api huma.API) {
	if s.invitations == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "create-invitation", Method: http.MethodPost, Path: "/v1/invitations",
		Summary: "Invite a person (admin)", Tags: []string{"invitations"},
		DefaultStatus: http.StatusCreated,
	}, RoleAdmin), s.createInvitation)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-invitations", Method: http.MethodGet, Path: "/v1/invitations",
		Summary: "List person invitations (admin)", Tags: []string{"invitations"},
	}, RoleAdmin), s.listInvitations)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-invitation", Method: http.MethodGet, Path: "/v1/invitations/{id}",
		Summary: "Inspect a person invitation (admin)", Tags: []string{"invitations"},
	}, RoleAdmin), s.getInvitation)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "issue-invitation-grant", Method: http.MethodPost, Path: "/v1/invitations/{id}/grant",
		Summary: "Regenerate a copied or QR invitation link (admin)", Tags: []string{"invitations"},
	}, RoleAdmin), s.issueInvitationGrant)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "revoke-invitation", Method: http.MethodDelete, Path: "/v1/invitations/{id}",
		Summary: "Revoke a person invitation (admin)", Tags: []string{"invitations"},
		DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.revokeInvitation)
}

type invitationBody struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind" enum:"local,library"`
	Username      string              `json:"username,omitempty"`
	LibraryUserID string              `json:"libraryUserId,omitempty"`
	DisplayName   string              `json:"displayName,omitempty"`
	Role          string              `json:"role" enum:"member,admin"`
	Status        string              `json:"status" enum:"pending,redeemed,expired,revoked"`
	CreatedAt     int64               `json:"createdAt" doc:"Unix ms"`
	ExpiresAt     int64               `json:"expiresAt" doc:"Unix ms"`
	TerminalAt    int64               `json:"terminalAt,omitempty" doc:"Unix ms"`
	RedeemedBy    string              `json:"redeemedBy,omitempty"`
	Contact       *contactAddressBody `json:"contactAddress,omitempty"`
}

type createInvitationInput struct {
	Body struct {
		Kind          string `json:"kind" enum:"local,library"`
		Username      string `json:"username,omitempty" maxLength:"200"`
		LibraryUserID string `json:"libraryUserId,omitempty" maxLength:"200"`
		ContactEmail  string `json:"contactEmail,omitempty" maxLength:"320"`
		Role          string `json:"role,omitempty" enum:"member,admin" doc:"Defaults to member."`
	}
}
type createInvitationOutput struct{ Body invitationBody }

func (s *Server) createInvitation(ctx context.Context, in *createInvitationInput) (*createInvitationOutput, error) {
	value, err := s.invitations.Create(ctx, invitation.CreateCommand{
		Kind: invitation.Kind(in.Body.Kind), Username: in.Body.Username,
		LibraryUserID: in.Body.LibraryUserID, ContactEmail: in.Body.ContactEmail,
		Role: invitation.Role(in.Body.Role),
	})
	if err != nil {
		return nil, invitationMutationError(err)
	}
	body, err := s.invitationResponse(ctx, value)
	if err != nil {
		return nil, err
	}
	return &createInvitationOutput{Body: body}, nil
}

type listInvitationsOutput struct {
	Body struct {
		Invitations []invitationBody `json:"invitations"`
	}
}

func (s *Server) listInvitations(ctx context.Context, _ *struct{}) (*listInvitationsOutput, error) {
	values, err := s.invitations.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &listInvitationsOutput{}
	out.Body.Invitations = make([]invitationBody, 0, len(values))
	for _, value := range values {
		body, err := s.invitationResponse(ctx, value)
		if err != nil {
			return nil, err
		}
		out.Body.Invitations = append(out.Body.Invitations, body)
	}
	return out, nil
}

type invitationIDInput struct {
	ID string `path:"id"`
}
type getInvitationOutput struct{ Body invitationBody }

func (s *Server) getInvitation(ctx context.Context, in *invitationIDInput) (*getInvitationOutput, error) {
	value, err := s.invitations.Get(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Invitation not found", "That invitation doesn't exist or is no longer retained.")
	}
	if err != nil {
		return nil, err
	}
	body, err := s.invitationResponse(ctx, value)
	if err != nil {
		return nil, err
	}
	return &getInvitationOutput{Body: body}, nil
}

type issueInvitationGrantInput struct {
	ID   string `path:"id"`
	Body struct {
		Conveyance string `json:"conveyance" enum:"copy,qr"`
	}
}
type issueInvitationGrantOutput struct {
	Body struct {
		URL       string `json:"url" doc:"One-time bearer link. It is returned only by this mutation and is never persisted."`
		ExpiresAt int64  `json:"expiresAt" doc:"Unix ms"`
	}
}

func (s *Server) issueInvitationGrant(ctx context.Context, in *issueInvitationGrantInput) (*issueInvitationGrantOutput, error) {
	base, err := recipientOrigin(s.accessPublicURL)
	if err != nil {
		return nil, errUnprocessable("Recipient address required",
			"Set the recipient-reachable address in Settings → Notifications before creating a link.")
	}
	issued, err := s.invitations.Regenerate(ctx, in.ID, invitation.Conveyance(in.Body.Conveyance))
	if err != nil {
		return nil, invitationMutationError(err)
	}
	out := &issueInvitationGrantOutput{}
	out.Body.URL = base + "/join#grant=" + url.QueryEscape(issued.Plaintext)
	out.Body.ExpiresAt = unixMillis(issued.ExpiresAt)
	return out, nil
}

func (s *Server) revokeInvitation(ctx context.Context, in *invitationIDInput) (*struct{}, error) {
	if err := s.invitations.Revoke(ctx, in.ID); err != nil {
		return nil, invitationMutationError(err)
	}
	return &struct{}{}, nil
}

func (s *Server) invitationResponse(ctx context.Context, value invitation.Invitation) (invitationBody, error) {
	body := invitationBody{
		ID: value.ID, Kind: string(value.Kind), Username: value.Username,
		LibraryUserID: value.LibraryUserID, DisplayName: value.DisplayName,
		Role: string(value.Role), Status: string(value.Status), CreatedAt: unixMillis(value.CreatedAt),
		ExpiresAt: unixMillis(value.ExpiresAt), RedeemedBy: value.RedeemedBy,
	}
	if !value.TerminalAt.IsZero() {
		body.TerminalAt = unixMillis(value.TerminalAt)
	}
	address, err := s.invitations.Contact(ctx, value.ID)
	if err == nil {
		body.Contact = toContactAddressBody(address)
	} else if !errors.Is(err, store.ErrNotFound) {
		return invitationBody{}, err
	}
	return body, nil
}

func invitationMutationError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound("Invitation not found", "That invitation doesn't exist or can no longer be changed.")
	case errors.Is(err, store.ErrInvitationIdentityConflict):
		return errConflict("Person already reserved", "That person already has an account or a pending invitation.")
	case errors.Is(err, store.ErrContactAddressConflict):
		return errConflict("Email already in use", "That contact address is already attached to another person or invitation.")
	default:
		return errUnprocessable("Invitation couldn't be saved", "Check the selected person and invitation details, then try again.")
	}
}

func recipientOrigin(read func() string) (string, error) {
	if read == nil {
		return "", errors.New("access public URL is unavailable")
	}
	raw := strings.TrimSpace(read())
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("access public URL is not an absolute HTTP origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("access public URL must be an origin without a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func unixMillis(value time.Time) int64 { return value.UnixMilli() }
