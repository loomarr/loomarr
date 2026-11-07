package store

import "github.com/loomarr/loomarr/internal/store/identitystore"

// The identity cluster (users, sessions, devices, invitations, contact addresses, password
// recovery, notifications) lives in internal/store/identitystore. Its types are re-exported here by
// alias, so importers of this package keep one import and see the same types, not copies.
type (
	// Role is a user's authorization level (§11) (identitystore.Role).
	Role = identitystore.Role
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

const (
	RoleAdmin  = identitystore.RoleAdmin
	RoleMember = identitystore.RoleMember
)

// bindIdentity gives s its identity cluster, built on s's own kernel handle. Every constructor of
// a sqlStore calls it; the *identitystore.Store it installs is embedded, so the cluster's methods
// are sqlStore's and the aggregate Store interface is satisfied without forwarders.
func bindIdentity(s *sqlStore) *sqlStore {
	s.identityCluster = identitystore.New(s.kernel())
	return s
}

// identityCluster and catalogCluster (catalog.go) name the two embedded clusters. Both packages call
// their type Store, and an embedded field takes its type's name, so embedding them directly would
// collide on the field and make every method they share ambiguous. An alias gives each embed its
// own field name and keeps method promotion, so the aggregate Store interface is still satisfied
// without a forwarder per method.
type identityCluster = identitystore.Store
