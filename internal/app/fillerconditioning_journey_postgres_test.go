//go:build integration

package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFillerConditioningJourney_RestartDistinguishesPreAndPostRekeyPublicationPostgres(t *testing.T) {
	runFillerConditioningRestartJourney(t, func(t *testing.T) records.Store { return testkit.PostgresStore(t) })
}
