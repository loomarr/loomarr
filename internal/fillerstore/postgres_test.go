//go:build integration

package fillerstore

import (
	"testing"

	"github.com/loomarr/loomarr/internal/store/storetest"
)

// TestPostgresConformance runs the SAME filler suite as SQLite (AGENTS.md: one suite, two
// backends — never forked). Requires Docker; run via `make test-pg`.
func TestPostgresConformance(t *testing.T) {
	dsn, _ := storetest.StartPostgres(t)
	RunConformance(t, storetest.PostgresClones(t, dsn, openExtended))
}
