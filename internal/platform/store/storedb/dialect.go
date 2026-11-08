package storedb

import (
	"strconv"
	"strings"
	"time"
)

// Dialect names a backend. It exists so capability checks read as what they mean:
// backend identity used to be inferred by comparing `claimSQL` against the SQLite
// constant, which works but states "this store's claim statement is the SQLite one"
// when it means "this is SQLite" — and would break silently if the two backends ever
// shared a statement.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// Placeholder rewrites a query written with `?` markers into the dialect's
// style. SQLite uses ?; Postgres uses $1, $2, ….
type Placeholder func(query string) string

// Passthrough is SQLite's Placeholder: `?` is already its style.
func Passthrough(q string) string { return q }

// PgPlaceholders is Postgres's Placeholder: it rewrites `?` markers into `$1, $2, …` in order.
func PgPlaceholders(query string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

// Epoch encodes a time as Unix seconds; the zero time encodes to 0 (§3 note).
func Epoch(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// FromEpoch decodes Unix seconds; 0 decodes to the zero time.
func FromEpoch(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}
