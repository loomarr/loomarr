package store

// Capabilities name what a backend permits, so code outside the store asks for the behaviour it
// needs instead of comparing against a dialect (docs/design/decisions/0001-postgres-and-sqlite.md).
// A dialect that is neither backend (a non-SQL test store reports "") has none of them.

// SupportsReplicas reports whether several Loomarr instances may share the store. Only Postgres
// permits it; SQLite is single-instance. A replica-aware behaviour (re-reading settings another
// instance wrote, re-reading a rotated generated secret) is on exactly when this is true.
func SupportsReplicas(d Dialect) bool { return d == DialectPostgres }

// PublishesInvalidations reports whether the store delivers durable cross-instance invalidations
// (Postgres LISTEN/NOTIFY, see ListenInvalidations). Behaviour that must stop serving when that
// feed's continuity cannot be proved applies only where it is true.
func PublishesInvalidations(d Dialect) bool { return d == DialectPostgres }

// CanMigrateToPostgres reports whether the in-app data move to Postgres starts from this store.
// Only a SQLite install is offered it (MigrateData, §18).
func CanMigrateToPostgres(d Dialect) bool { return d == DialectSQLite }

// IsMigrationTarget reports whether the store is the backend the in-app migration moves data into,
// so a process that restarted after a migration can tell whether it landed there.
func IsMigrationTarget(d Dialect) bool { return d == DialectPostgres }
