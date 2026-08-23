package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func TestTMDBFranchises_UnconfiguredIsUnavailableNotReconcileFailure(t *testing.T) {
	resolver := tmdbFranchises{tmdb: tmdb.NewDynamic(func() string { return "" })}

	collectionID, resolved, err := resolver.Collection(context.Background(), provision.Key("movie:tmdb:603"))
	if err != nil {
		t.Fatalf("Collection() error = %v, want nil for an unconfigured optional enrichment", err)
	}
	if resolved || collectionID != 0 {
		t.Fatalf("Collection() = (%d, %v), want unresolved so a configured reconcile can heal it", collectionID, resolved)
	}
}
