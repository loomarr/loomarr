// Package catalogstore persists the clip catalog (design §10, §22): clips and their tags, the
// taxonomy and its closure and rollups, holidays, images and their derivatives and references,
// broadcast stations, and the clip defect, fingerprint, variant, inferior-copy and airing tables.
//
// It is a cluster of internal/store, extracted so the catalog tables are owned by one package that
// builds only on the persistence kernel (internal/store/storedb). It imports storedb and never
// internal/store: internal/store imports it, re-exports its types by alias, and embeds a Store in
// its aggregate, so every existing caller of store.Store keeps working unchanged.
// internal/architecture_test.go enforces the direction.
//
// # Seams
//
// A Store runs on a storedb.Handle. Its multi-statement writes begin their own transaction on it;
// the ones another part of the store composes (ClipTx, which internal/fillerstore uses through
// store.Handle.Clips) take the caller's storedb.Querier and never begin or commit.
//
// The channels table belongs to programming. Rekeying a clip must carry channel filler pins to the
// new hash, and regenerating a split must keep every clip a channel pins, so the catalog needs to
// read every channel policy under a lock and write back the ones it changed. It does that only
// through ChannelPolicies, an interface internal/store implements with the SQL, the Postgres row
// lock and the revision bump. The catalog decodes and rewrites the policy JSON.
//
// The boot seeds (taxonomy, holidays, broadcast stations) are exported (ConvergeTaxonomySeed,
// ConvergeHolidaySeed, HealUndatedHolidays, ConvergeBroadcastStationSeed, RebuildTaxonomyDerived)
// and sequenced by internal/store's Open, which owns the migration that must precede them.
//
// Errors: ErrNotFound is storedb.ErrNotFound (the same value, so errors.Is matches either name);
// ErrConditionedClipMismatch, ErrTaxonConflict, ErrCorruptClipDefects, ErrInferiorCopyMismatch and
// ErrUnknownClipSort are defined here and re-exported by internal/store as the same values.
package catalogstore
