//go:build integration

package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFillerConditioningJourney_RestartDistinguishesPreAndPostRekeyPublicationPostgres(t *testing.T) {
	runFillerConditioningRestartJourney(t, func(t *testing.T) store.Store { return testkit.PostgresStore(t) })
}
