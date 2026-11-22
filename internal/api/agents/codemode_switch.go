package agents

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// Code Mode's switch (#4486): an admin turns it on or off for one of their own paired devices, in
// Settings → Access and devices.
//
// ⚠ ONLY FROM A SIGNED-IN BROWSER. A device credential is refused, even an admin's: otherwise an
// agent holding its admin token could turn Code Mode on for itself, and the point of the switch is
// that a person decides. Scripts are refused these routes too (scriptRefuses).
//
// Hidden from the OpenAPI document, like the rest of the agents surface, until the device list's
// own DTO can carry the flag.

const codeModePath = "/v1/agents/code-mode"

type codeModeListOutput struct {
	Body struct {
		// Devices are the ids (as the device list gives them) of the caller's devices with Code
		// Mode on.
		Devices []string `json:"devices"`
	}
}

type codeModeSetInput struct {
	ID   string `path:"id" doc:"The device's id, as the device list gives it"`
	Body struct {
		On bool `json:"on" doc:"Whether the device may use Code Mode"`
	}
}

func (h *Handlers) registerCodeModeSwitch(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "agents-code-mode-list", Method: http.MethodGet, Path: codeModePath,
		Summary: "List your devices with Code Mode on", Tags: []string{"Agents"}, Hidden: true,
	}, httpcore.RoleAdmin), h.listCodeMode)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "agents-code-mode-set", Method: http.MethodPut, Path: codeModePath + "/{id}",
		Summary: "Turn Code Mode on or off for one of your devices", Tags: []string{"Agents"}, Hidden: true,
	}, httpcore.RoleAdmin), h.setCodeMode)
}

func (h *Handlers) personOnly(ctx context.Context) (string, error) {
	if _, isDevice := httpcore.DeviceIDFrom(ctx); isDevice {
		return "", huma.Error403Forbidden("Code Mode is turned on or off by a person signed in to Loomarr, not by a device.")
	}
	user, ok := httpcore.UserFrom(ctx)
	if !ok || user.ID == "" {
		return "", huma.Error403Forbidden("Sign in to change Code Mode.")
	}
	if h.deps.Devices == nil {
		return "", huma.Error503ServiceUnavailable("Paired devices are not available.")
	}
	return user.ID, nil
}

func (h *Handlers) listCodeMode(ctx context.Context, _ *struct{}) (*codeModeListOutput, error) {
	userID, err := h.personOnly(ctx)
	if err != nil {
		return nil, err
	}
	devices, err := h.deps.Devices.ListFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &codeModeListOutput{}
	out.Body.Devices = []string{}
	for _, d := range devices {
		if d.CodeMode {
			out.Body.Devices = append(out.Body.Devices, d.TokenHash)
		}
	}
	return out, nil
}

func (h *Handlers) setCodeMode(ctx context.Context, in *codeModeSetInput) (*struct{}, error) {
	userID, err := h.personOnly(ctx)
	if err != nil {
		return nil, err
	}
	ok, err := h.deps.Devices.SetCodeMode(ctx, in.ID, userID, in.Body.On)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, huma.Error404NotFound("You have no device with that id.")
	}
	return nil, nil
}
