//go:build integration

package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFoundationStorageBudgetAndProjectionSurvivePostgresRestart(t *testing.T) {
	testFoundationStorageBudgetAndProjectionSurviveRestart(t, testkit.PostgresStore(t))
}
