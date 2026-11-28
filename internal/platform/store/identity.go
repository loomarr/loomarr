package store

import "github.com/loomarr/loomarr/internal/platform/store/identitystore"

// The identity cluster (users, sessions, devices, invitations, contact addresses, password
// recovery, notifications) lives in internal/platform/store/identitystore. Its types are re-exported here by
// alias, so importers of this package keep one import and see the same types, not copies.
type (
	// User is a Loomarr account with local role and quota (§11) (identitystore.User).
	User = identitystore.User
	// Session is a revocable session row (§11) (identitystore.Session).
	Session = identitystore.Session
	// SessionSeen is one authenticated use of a session (identitystore.SessionSeen).
	SessionSeen = identitystore.SessionSeen
	// DevicePairing is an in-flight TV pairing (identitystore.DevicePairing).
	DevicePairing = identitystore.DevicePairing
	// DeviceToken is a paired device's durable credential (identitystore.DeviceToken).
	DeviceToken = identitystore.DeviceToken
)
