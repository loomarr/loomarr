package store

import "github.com/loomarr/loomarr/internal/platform/store/proposalstore"

// Generation jobs, their attempts and the proposals they produce live in
// internal/platform/store/proposalstore. Their row types and sentinel errors are re-exported here by
// alias, for the store's own contract.
type (
	// Job is a persisted suggester generation task (proposalstore.Job).
	Job = proposalstore.Job
	// ProposalJobAttempt is one exact execution lease (proposalstore.ProposalJobAttempt).
	ProposalJobAttempt = proposalstore.ProposalJobAttempt
	// ProposalJob is the consistent read projection of one generation (proposalstore.ProposalJob).
	ProposalJob = proposalstore.ProposalJob
	// Proposal is a persisted suggester output (proposalstore.Proposal).
	Proposal = proposalstore.Proposal
)

// The proposal sentinel errors are the proposalstore values under second names, so
// errors.Is matches an error created in either package.
var (
	// ErrProposalNotSubmitted reports a terminal decision that lost the submitted -> decided swap.
	ErrProposalNotSubmitted = proposalstore.ErrProposalNotSubmitted
	// ErrProposalSuperseded reports an approval candidate a newer approval already superseded.
	ErrProposalSuperseded = proposalstore.ErrProposalSuperseded
	// ErrProposalRevisionActive keeps the approval gate closed while a replacement generates.
	ErrProposalRevisionActive = proposalstore.ErrProposalRevisionActive
)

// proposalCluster names the embedded proposal store; see identityCluster for why an alias.
type proposalCluster = proposalstore.Store

// bindProposals installs the proposal cluster on s; bindClusters calls it on every construction.
func bindProposals(s *sqlStore) *sqlStore {
	s.proposalCluster = proposalstore.New(s.kernel(), s.channelCluster)
	return s
}
