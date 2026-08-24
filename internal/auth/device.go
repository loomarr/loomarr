package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// PairingTTL is how long a code shown on a TV stays valid.
//
// Short on purpose: the code is low-entropy because a human reads it off a screen and types it, so
// its safety comes from the narrow window plus single use, not from its length. Long enough to walk
// to a laptop; short enough that an abandoned code on a living-room screen stops mattering quickly.
const PairingTTL = 10 * time.Minute

// userCodeAlphabet omits I/1, O/0, S/5, and vowels. Vowels go so a random code cannot spell a word;
// look-alikes go because this is read off a TV across a room and typed on a phone, and a
// transcription error is indistinguishable from a wrong code.
const userCodeAlphabet = "BCDFGHJKLMNPQRTVWXYZ"

// userCodeLength is 8 characters, formatted as two groups of four.
//
// ~34 bits from the reduced alphabet. That is not cryptographic strength, and it does not need to
// be: a code is single-use, expires in PairingTTL, and only ever grants what the approving human
// already has. Guessing must be defeated by the window and by rate limiting the poll, not by length
// a person has to type with a D-pad.
const userCodeLength = 8

// ErrPairingNotApproved means the pairing exists and is still pending — the device should keep
// polling. Distinct from ErrNotFound, which means the code is wrong, consumed, or expired: the
// device must stop and show a fresh code.
var ErrPairingNotApproved = errors.New("auth: pairing not approved")

// DeviceManager runs the pairing handshake and issues device credentials.
type DeviceManager struct {
	store store.Store
	now   func() time.Time
}

// DevicePrincipal is the authenticated identity carried by one paired-device credential. ID is the
// token hash: it is safe to use as a revocation handle and cannot authenticate a request.
type DevicePrincipal struct {
	ID   string
	User store.User
}

// NewDeviceManager builds a device manager. now defaults to time.Now.
func NewDeviceManager(st store.Store, now func() time.Time) *DeviceManager {
	if now == nil {
		now = time.Now
	}
	return &DeviceManager{store: st, now: now}
}

// newUserCode returns a human-typeable code like "BCDF-GHJK".
//
// Uses crypto/rand and rejection sampling rather than modulo: with a 20-character alphabet, modulo
// over a byte would make the first 16 letters measurably likelier than the last 4. The bias would be
// invisible in testing and would shrink the effective keyspace of a security-relevant code.
func newUserCode() (string, error) {
	out := make([]byte, 0, userCodeLength+1)
	buf := make([]byte, 1)
	const limit = 256 - (256 % len(userCodeAlphabet))
	for len(out) < userCodeLength+1 {
		if len(out) == 4 {
			out = append(out, '-')
			continue
		}
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("auth: generate user code: %w", err)
		}
		if int(buf[0]) >= limit {
			continue // would bias the distribution; draw again
		}
		out = append(out, userCodeAlphabet[int(buf[0])%len(userCodeAlphabet)])
	}
	return string(out), nil
}

// NormalizeUserCode makes a typed code comparable: upper-cased, with spaces and dashes dropped, then
// re-grouped. A human typing "bcdf ghjk" or "bcdfghjk" means the same code they read as "BCDF-GHJK",
// and rejecting those is a support ticket, not security.
func NormalizeUserCode(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		if r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	code := b.String()
	if len(code) == userCodeLength {
		return code[:4] + "-" + code[4:]
	}
	return code
}

// StartPairing mints a pending pairing. It returns the plaintext device code (kept only by the
// device, stored as a hash) and the user code to display.
func (m *DeviceManager) StartPairing(ctx context.Context, deviceName string) (deviceCode, userCode string, expires time.Time, err error) {
	deviceCode, err = newToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	userCode, err = newUserCode()
	if err != nil {
		return "", "", time.Time{}, err
	}
	now := m.now()
	expires = now.Add(PairingTTL)
	err = m.store.CreateDevicePairing(ctx, store.DevicePairing{
		DeviceCodeHash: hashToken(deviceCode),
		UserCode:       userCode,
		DeviceName:     deviceName,
		CreatedAt:      now,
		ExpiresAt:      expires,
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	return deviceCode, userCode, expires, nil
}

// Approve completes a pairing on behalf of a signed-in user. Single-use is enforced by the store's
// conditional update, so a double click or two racing humans still produce one credential.
func (m *DeviceManager) Approve(ctx context.Context, rawUserCode, userID string) error {
	ok, err := m.store.ApproveDevicePairing(ctx, NormalizeUserCode(rawUserCode), userID, m.now())
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrNotFound
	}
	return nil
}

// Redeem exchanges an approved pairing for a durable device token. The pairing is consumed here, so
// a device code can never yield two credentials.
//
// Returns ErrPairingNotApproved while a human has not acted yet — the device keeps polling. Any
// other error means stop.
func (m *DeviceManager) Redeem(ctx context.Context, deviceCode string) (token string, name string, err error) {
	hash := hashToken(deviceCode)
	pairing, err := m.store.GetDevicePairing(ctx, hash, m.now())
	if err != nil {
		return "", "", err
	}
	if !pairing.Approved() {
		return "", "", ErrPairingNotApproved
	}
	token, err = newToken()
	if err != nil {
		return "", "", err
	}
	now := m.now()
	err = m.store.CreateDeviceToken(ctx, store.DeviceToken{
		TokenHash:  hashToken(token),
		UserID:     pairing.UserID,
		DeviceName: pairing.DeviceName,
		CreatedAt:  now,
		LastSeenAt: now,
	})
	if err != nil {
		return "", "", err
	}
	// Consume the pairing. A failure here would leave a spent code that can no longer mint anything
	// (the token already exists), so it is not worth failing the device's redemption over.
	_ = m.store.DeleteDevicePairing(ctx, hash)
	return token, pairing.DeviceName, nil
}

// Pending resolves a still-unapproved pairing by the code a human typed, so an approval screen can
// name the device before granting it anything ("Approve Living Room Shield?"). Read-only.
func (m *DeviceManager) Pending(ctx context.Context, rawUserCode string) (store.DevicePairing, error) {
	return m.store.GetDevicePairingByUserCode(ctx, NormalizeUserCode(rawUserCode), m.now())
}

// ListFor returns a user's paired devices for the revocation UI.
func (m *DeviceManager) ListFor(ctx context.Context, userID string) ([]store.DeviceToken, error) {
	return m.store.ListDeviceTokensForUser(ctx, userID)
}

// Revoke kills one device. Scoped by user in the store, so this cannot revoke someone else's TV.
func (m *DeviceManager) Revoke(ctx context.Context, tokenHash, userID string) (bool, error) {
	return m.store.DeleteDeviceToken(ctx, tokenHash, userID)
}

// ResolveDevice authenticates a device token and returns its non-secret revocation identity plus
// its owning user, so a device acts AS the member who approved it and inherits that member's role —
// never more.
func (m *DeviceManager) ResolveDevice(ctx context.Context, token string) (DevicePrincipal, error) {
	id := hashToken(token)
	dt, err := m.store.GetDeviceToken(ctx, id)
	if err != nil {
		return DevicePrincipal{}, err
	}
	user, err := m.store.GetUser(ctx, dt.UserID)
	if err != nil {
		return DevicePrincipal{}, err
	}
	if user.Disabled {
		// Same rule sessions follow: disabling a user must take effect immediately everywhere,
		// including on a TV that has not been touched in weeks.
		return DevicePrincipal{}, store.ErrNotFound
	}
	return DevicePrincipal{ID: id, User: user}, nil
}
