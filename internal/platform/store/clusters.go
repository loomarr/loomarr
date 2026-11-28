package store

import (
	"github.com/loomarr/loomarr/internal/platform/store/activitystore"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/crashstore"
	"github.com/loomarr/loomarr/internal/platform/store/diagnosticsstore"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/inventorystore"
	"github.com/loomarr/loomarr/internal/platform/store/jobstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/platform/store/operationstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/platform/store/qualitystore"
	"github.com/loomarr/loomarr/internal/platform/store/viewingstore"
)

// Activity is one row in the Recent activity feed (activitystore.Activity).
type Activity = activitystore.Activity

// The cluster names. Every cluster package calls its type Store, and an embedded field takes its
// type's name, so embedding two of them directly would collide on the field and make every method
// they share ambiguous. An alias gives each embed its own field name and keeps method promotion,
// so the aggregate Store interface is satisfied without a forwarder per method.
type (
	identityCluster    = identitystore.Store
	catalogCluster     = clipstore.Store
	crashCluster       = crashstore.Store
	diagnosticsCluster = diagnosticsstore.Store
	jobsCluster        = jobstore.Store
	activityCluster    = activitystore.Store
	channelDataCluster = channeldatastore.Store
	channelCluster     = channelstore.Store
	proposalCluster    = proposalstore.Store
	libraryCluster     = librarystore.Store
	qualityCluster     = qualitystore.Store
	inventoryCluster   = inventorystore.Store
	viewingCluster     = viewingstore.Store
	operationCluster   = operationstore.Store
)

// bindClusters installs every embedded cluster on s, each built on s's own kernel handle. Every
// constructor of a sqlStore calls it, so a store never exists with a nil cluster. The dialect must
// be set, and s.channelClaimSQL with it, before the call.
func bindClusters(s *sqlStore) *sqlStore {
	k := s.kernel()
	s.identityCluster = identitystore.New(k)
	// The catalog rekeys clip hashes inside a channel policy, and the SQL on channels stays here.
	s.catalogCluster = clipstore.New(k, channelPolicies{s})
	s.crashCluster = crashstore.New(k)
	s.diagnosticsCluster = diagnosticsstore.New(k)
	s.jobsCluster = jobstore.New(k)
	s.activityCluster = activitystore.New(k)
	s.channelDataCluster = channeldatastore.New(k)
	var inv channelstore.Invalidations // nil on SQLite: no wake-ups are published
	if s.dialect == DialectPostgres {
		inv = channelInvalidations{}
	}
	s.channelCluster = channelstore.New(k, s.channelClaimSQL, inv)
	// The proposal cluster builds on the channel cluster, so it binds after it.
	s.proposalCluster = proposalstore.New(k, s.channelCluster)
	s.libraryCluster = librarystore.New(k)
	s.qualityCluster = qualitystore.New(k)
	// The inventory cluster reaches the Library only through libraryProjection, so it never imports
	// this package.
	s.inventoryCluster = inventorystore.New(k, libraryProjection{s})
	s.viewingCluster = viewingstore.New(k)
	s.operationCluster = operationstore.New(k)
	return s
}
