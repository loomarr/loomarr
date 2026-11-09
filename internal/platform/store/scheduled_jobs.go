package store

import "github.com/loomarr/loomarr/internal/platform/store/jobstore"

// ScheduledJob is the persisted runtime state of one named, recurring background job (§18.1)
// (jobstore.ScheduledJob).
type ScheduledJob = jobstore.ScheduledJob

// jobsCluster names the embedded scheduler-state store; see identityCluster for why an alias. The
// ScheduledJobStore role it satisfies is declared in store.go.
type jobsCluster = jobstore.Store

// bindJobs installs the scheduler-state cluster on s; bindClusters calls it on every construction.
func bindJobs(s *sqlStore) *sqlStore {
	s.jobsCluster = jobstore.New(s.kernel())
	return s
}
