package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/library/scan"
)

// Sonarr's and Radarr's webhooks (#2151 decisions 9 and 10). Loomarr registers itself in each arr
// with a generated secret; the arr posts file events here, and the scanner passes over the folders
// they name. The delivery route is public because an arr holds no session: the secret is the
// credential, sent as the basic-auth password.

// LibraryHookService is scan.ArrHooks.
type LibraryHookService interface {
	Authorized(username, password string) bool
	Receive(ctx context.Context, app string, body []byte) error
	Statuses() []scan.HookStatus
	Test(ctx context.Context, app string) error
}

func (s *Server) registerLibraryHooks(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "receiveLibraryHook", Method: http.MethodPost, Path: "/v1/library/hooks/{app}",
		Summary: "Receive an event from Sonarr or Radarr",
		Description: "Where Sonarr and Radarr send file events once Loomarr has registered itself with them. " +
			"Authenticated by the generated secret as the basic-auth password, not by a session.",
		Tags: []string{"library"}, DefaultStatus: http.StatusAccepted,
	}, RolePublic), s.receiveLibraryHook)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "testLibraryHook", Method: http.MethodPost, Path: "/v1/library/hooks/{app}/test",
		Summary: "Test an arr's webhook",
		Description: "Registers Loomarr with the arr again and asks it to send its test event. " +
			"502 with the arr's answer when it cannot save or reach Loomarr's connection.",
		Tags: []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.testLibraryHook)
}

type libraryHookInput struct {
	App           string `path:"app" enum:"sonarr,radarr"`
	Authorization string `header:"Authorization"`
	RawBody       []byte
}

func (s *Server) receiveLibraryHook(ctx context.Context, in *libraryHookInput) (*struct{}, error) {
	if s.libraryHooks == nil {
		return nil, httpcore.FeatureNotConfigured("Webhooks unavailable", "The library scanner is not set up on this server.")
	}
	user, password, ok := basicAuth(in.Authorization)
	if !ok || !s.libraryHooks.Authorized(user, password) {
		return nil, httpcore.Problem(http.StatusUnauthorized, "Not authorized", "This webhook needs the credentials Loomarr registered in the arr.")
	}
	if err := s.libraryHooks.Receive(ctx, in.App, in.RawBody); err != nil {
		if errors.Is(err, scan.ErrHookUnknownApp) {
			return nil, httpcore.Problem(http.StatusNotFound, "Not found", "Loomarr takes webhooks only from Sonarr and Radarr.")
		}
		return nil, httpcore.ProblemWithCause(http.StatusBadRequest, "Couldn't read the event", "The body is not a Sonarr or Radarr webhook.", err)
	}
	return nil, nil
}

// basicAuth reads an Authorization: Basic header.
func basicAuth(header string) (username, password string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), ":")
}

type libraryHookTestInput struct {
	App string `path:"app" enum:"sonarr,radarr"`
}

func (s *Server) testLibraryHook(ctx context.Context, in *libraryHookTestInput) (*struct{}, error) {
	if s.libraryHooks == nil {
		return nil, httpcore.FeatureNotConfigured("Webhooks unavailable", "The library scanner is not set up on this server.")
	}
	if err := s.libraryHooks.Test(ctx, in.App); err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "The webhook test failed", hookTestDetail(err), err)
	}
	return nil, nil
}

// hookTestDetail says what to do about a failed test.
func hookTestDetail(err error) string {
	switch {
	case errors.Is(err, scan.ErrHookNoAddress):
		return "Set Loomarr's address (server.public_url) so the arr knows where to send events."
	case errors.Is(err, scan.ErrHookNotConfigured):
		return "Connect the arr in Settings first."
	}
	return "Check that the arr can reach Loomarr at its address (server.public_url)."
}

type libraryHookDTO struct {
	App          string     `json:"app" enum:"sonarr,radarr"`
	State        string     `json:"state" enum:"registered,not_configured,no_address,failed"`
	Error        string     `json:"error,omitempty" doc:"Why registering failed"`
	RegisteredAt *time.Time `json:"registeredAt,omitempty"`
	LastEvent    string     `json:"lastEvent,omitempty" doc:"The arr's event type, since Loomarr started"`
	LastEventAt  *time.Time `json:"lastEventAt,omitempty"`
}

// libraryHooksView is every arr's webhook; empty without the service.
func (s *Server) libraryHooksView() []libraryHookDTO {
	out := []libraryHookDTO{}
	if s.libraryHooks == nil {
		return out
	}
	for _, h := range s.libraryHooks.Statuses() {
		dto := libraryHookDTO{App: h.App, State: string(h.State), Error: h.Error, LastEvent: h.LastEvent}
		if !h.RegisteredAt.IsZero() {
			at := h.RegisteredAt
			dto.RegisteredAt = &at
		}
		if !h.LastEventAt.IsZero() {
			at := h.LastEventAt
			dto.LastEventAt = &at
		}
		out = append(out, dto)
	}
	return out
}
