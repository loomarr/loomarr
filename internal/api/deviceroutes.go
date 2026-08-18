package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// registerDeviceAuth mounts /v1/auth/device/* — the pairing handshake a keyboard-less client uses
// (§11, Shield P1), plus the paired-device list its owner revokes from.
//
// The shape is RFC 8628's device-authorization grant: the client starts a pairing and polls; a
// human approves it from a surface that already has a session. A TV cannot type a password, and the
// alternative was shipping the household admin token to every device in the house.
func (s *Server) registerDeviceAuth(api huma.API) {
	if s.devices == nil && !s.schemaOnly {
		return
	}

	// RolePublic because this IS the credential handshake: requiring a session to reach the route
	// that grants one would make it useless. Nothing is granted here — the response is a pending
	// code that only a signed-in human can turn into access.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "device-pair-start", Method: http.MethodPost, Path: "/v1/auth/device/start",
		Summary: "Begin pairing a device", Tags: []string{"auth"},
	}, RolePublic), s.handleDeviceStart)

	// Also public, and also grants nothing on its own: the caller must already hold the 256-bit
	// device code minted above, and the pairing must have been approved by a human.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "device-pair-poll", Method: http.MethodPost, Path: "/v1/auth/device/poll",
		Summary: "Exchange an approved pairing for a device token", Tags: []string{"auth"},
	}, RolePublic), s.handleDevicePoll)

	// The human half. RoleMember, and the approving user's identity is what the device inherits —
	// which is why this cannot be public: approval is the moment authority is delegated.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "device-pair-approve", Method: http.MethodPost, Path: "/v1/auth/device/approve",
		Summary: "Approve a device's pairing code", Tags: []string{"auth"},
	}, RoleMember), s.handleDeviceApprove)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "device-list", Method: http.MethodGet, Path: "/v1/auth/devices",
		Summary: "List this user's paired devices", Tags: []string{"auth"},
	}, RoleMember), s.handleDeviceList)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "device-revoke", Method: http.MethodDelete, Path: "/v1/auth/devices/{id}",
		Summary: "Revoke one paired device", Tags: []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, RoleMember), s.handleDeviceRevoke)
}

type deviceStartInput struct {
	Body struct {
		DeviceName string `json:"deviceName,omitempty" maxLength:"64" doc:"Label shown in the owner's device list" example:"Living Room Shield"`
	}
}

type deviceStartOutput struct {
	Body struct {
		DeviceCode string `json:"deviceCode" doc:"Secret the device keeps and polls with; never displayed"`
		UserCode   string `json:"userCode" doc:"Short code to display on screen" example:"BCDF-GHJK"`
		ExpiresAt  string `json:"expiresAt" doc:"RFC3339; after this the codes are dead"`
		Interval   int    `json:"interval" doc:"Seconds the device should wait between polls"`
	}
}

// pollInterval is what a device is told to wait between polls.
//
// Advisory, not enforced here: the poll is authenticated by a 256-bit device code, so a fast poller
// is a load concern rather than a security one. It exists so every client does not invent its own
// cadence and hammer a homelab box.
const pollInterval = 5

func (s *Server) handleDeviceStart(ctx context.Context, in *deviceStartInput) (*deviceStartOutput, error) {
	r := requestFrom(ctx)
	// Keyed on IP alone: there is no user yet, and the thing being limited is the minting of
	// pairings. Without this an attacker could farm codes to raise the odds that a guess at the
	// approve endpoint lands on a live one.
	if s.deviceLimiter != nil && !s.deviceLimiter.Allow("start|"+s.clientIP(r)) {
		return nil, errTooManyRequests("Too many pairing attempts",
			"Too many devices have started pairing from this address. Wait a moment and try again.")
	}
	name := in.Body.DeviceName
	if name == "" {
		name = "Device"
	}
	deviceCode, userCode, expires, err := s.devices.StartPairing(ctx, name)
	if err != nil {
		return nil, err
	}
	out := &deviceStartOutput{}
	out.Body.DeviceCode = deviceCode
	out.Body.UserCode = userCode
	out.Body.ExpiresAt = expires.UTC().Format(time.RFC3339)
	out.Body.Interval = pollInterval
	return out, nil
}

type devicePollInput struct {
	Body struct {
		DeviceCode string `json:"deviceCode" required:"true" doc:"The secret returned by /start"`
	}
}

type devicePollOutput struct {
	Body struct {
		Token      string `json:"token" doc:"Bearer credential; send as Authorization: Bearer <token>"`
		DeviceName string `json:"deviceName"`
	}
}

func (s *Server) handleDevicePoll(ctx context.Context, in *devicePollInput) (*devicePollOutput, error) {
	token, name, err := s.devices.Redeem(ctx, in.Body.DeviceCode)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrPairingNotApproved):
			// 428 Precondition Required, not 404: the device must keep polling. Distinguishing
			// "still waiting" from "this code is dead" is the whole reason a device knows whether
			// to wait or to show a fresh code — collapsing them would make a pending pairing look
			// like a failure and send the TV back to the start screen every few seconds.
			return nil, huma.Error428PreconditionRequired("Waiting for approval")
		case errors.Is(err, store.ErrNotFound):
			return nil, errNotFound("Pairing not found",
				"That pairing has expired or was already completed. Start again on the device.")
		default:
			return nil, err
		}
	}
	out := &devicePollOutput{}
	out.Body.Token = token
	out.Body.DeviceName = name
	return out, nil
}

type deviceApproveInput struct {
	Body struct {
		UserCode string `json:"userCode" required:"true" doc:"The code shown on the device" example:"BCDF-GHJK"`
	}
}

type deviceApproveOutput struct {
	Body struct {
		DeviceName string `json:"deviceName"`
	}
}

func (s *Server) handleDeviceApprove(ctx context.Context, in *deviceApproveInput) (*deviceApproveOutput, error) {
	r := requestFrom(ctx)
	user, ok := userFrom(ctx)
	if !ok {
		return nil, errUnauthorized("Sign-in required", "Sign in before approving a device.")
	}
	// ⚠ THE guess-guard for this feature. The user code is deliberately short so a human can read it
	// off a screen and type it, which means its safety comes from a narrow window plus a bounded
	// number of attempts — not from length. Keyed per user so one account cannot grind codes, and
	// per IP so a pool of accounts cannot either.
	if s.deviceLimiter != nil && !s.deviceLimiter.Allow("approve|"+user.ID+"|"+s.clientIP(r)) {
		return nil, errTooManyRequests("Too many attempts",
			"Too many pairing codes tried. Wait a moment and try again.")
	}
	pairing, err := s.devices.Pending(ctx, in.Body.UserCode)
	if err != nil {
		return nil, errNotFound("Code not found",
			"That code is wrong or has expired. Check the code on your device and try again.")
	}
	if err := s.devices.Approve(ctx, in.Body.UserCode, user.ID); err != nil {
		return nil, errNotFound("Code not found",
			"That code is wrong or has expired. Check the code on your device and try again.")
	}
	out := &deviceApproveOutput{}
	out.Body.DeviceName = pairing.DeviceName
	return out, nil
}

type deviceListOutput struct {
	Body struct {
		Devices []deviceDTO `json:"devices"`
	}
}

// deviceDTO is one paired device as its owner sees it.
//
// `id` is the token HASH, never the token. It is already what the store keys on, it is safe to hand
// out (it cannot authenticate anything), and it gives the revoke call a stable handle without ever
// putting a live credential back on the wire.
type deviceDTO struct {
	ID         string `json:"id"`
	DeviceName string `json:"deviceName"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
}

func (s *Server) handleDeviceList(ctx context.Context, _ *struct{}) (*deviceListOutput, error) {
	user, ok := userFrom(ctx)
	if !ok {
		return nil, errUnauthorized("Sign-in required", "Sign in to see your devices.")
	}
	devices, err := s.devices.ListFor(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := &deviceListOutput{}
	out.Body.Devices = make([]deviceDTO, 0, len(devices))
	for _, d := range devices {
		out.Body.Devices = append(out.Body.Devices, deviceDTO{
			ID:         d.TokenHash,
			DeviceName: d.DeviceName,
			CreatedAt:  d.CreatedAt.UTC().Format(time.RFC3339),
			LastSeenAt: d.LastSeenAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

type deviceRevokeInput struct {
	ID string `path:"id" doc:"Device id from the device list"`
}

func (s *Server) handleDeviceRevoke(ctx context.Context, in *deviceRevokeInput) (*struct{}, error) {
	user, ok := userFrom(ctx)
	if !ok {
		return nil, errUnauthorized("Sign-in required", "Sign in to revoke a device.")
	}
	// Ownership is enforced in the store's WHERE clause, so a caller passing someone else's device
	// id gets the same 404 as a caller passing a made-up one — no probing for which ids exist.
	revoked, err := s.devices.Revoke(ctx, in.ID, user.ID)
	if err != nil {
		return nil, err
	}
	if !revoked {
		return nil, errNotFound("Device not found", "That device is already gone.")
	}
	return nil, nil
}
