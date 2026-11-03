// Package identitystore persists who may act in Loomarr and how they are told things: users and
// their sessions, paired devices, invitations and their grants, contact addresses, password
// recovery, and the notification intents, delivery attempts, destinations and recipients that
// reach those people (§11).
//
// It is a cluster of internal/store, extracted so the identity tables are owned by one package
// that builds only on the persistence kernel (internal/store/storedb). It imports storedb and
// never internal/store: internal/store imports it, re-exports its types by alias, and embeds a
// Store in its aggregate, so every existing caller of store.Store keeps working unchanged.
// internal/architecture_test.go enforces the direction.
//
// # Seams
//
// A Store runs on a storedb.Handle. Its keyed locks (invitation identity, password recovery) are
// its own: each lock domain owns its KeyedLock and its Postgres advisory-lock namespace.
//
// The proposal approval transaction lives in internal/store, because it commits proposals, titles
// and channels together. It reaches into this cluster only through the transaction-taking
// functions GetUserIn and InsertNotificationReferenceRecipient, which run on the caller's
// storedb.Querier and never begin or commit anything themselves.
//
// Errors: ErrNotFound is storedb.ErrNotFound (the same value, so errors.Is matches either name);
// ErrContactAddressConflict and ErrInvitationIdentityConflict are defined here and re-exported by
// internal/store as the same values.
package identitystore
