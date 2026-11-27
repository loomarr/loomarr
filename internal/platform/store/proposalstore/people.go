package proposalstore

import (
	"database/sql"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// AutoApprovedBy is the ApprovedBy of a proposal the grant, not a person, approved (§11:
// approvals are audited). Deliberately not a user id: the trail must be able to answer "did a
// person decide this?" without inferring it. Stored as decided_via 'auto' with no decided_by.
const AutoApprovedBy = "auto"

// AutoCuratedBy is the ApprovedBy of a proposal the §8.2 scheduled re-curation approved, under a
// channel's AutoCurate opt-in. Distinct from AutoApprovedBy so the trail separates "a user's
// auto-approve grant" from "the channel's auto-curate loop". Stored as decided_via 'auto-curate'.
const AutoCuratedBy = "auto-curate"

// MemberValue writes a person column (created_by, decided_by): the member, or NULL when no member
// has that id. Each is a foreign key to users that a deleted person clears (SET NULL, #2793); the
// API token is no member and writes "", and a job read before its author went may be saved after,
// so both record "no person" rather than failing.
const MemberValue = `(SELECT id FROM users WHERE id = ?)`

// DecisionArgs are the decided_by and decided_via a proposal stores for its ApprovedBy: a grant's
// decision has no person, and an approved or denied one with no grant was a person's, named or
// (the API token) not.
func DecisionArgs(p Proposal) (decidedBy storedb.OptionalID, decidedVia any) {
	switch {
	case p.ApprovedBy == AutoApprovedBy || p.ApprovedBy == AutoCuratedBy:
		return "", p.ApprovedBy
	case p.Status == "approved" || p.Status == "denied":
		return storedb.OptionalID(p.ApprovedBy), "person"
	default:
		return storedb.OptionalID(p.ApprovedBy), nil
	}
}

// approvedBy reads ApprovedBy back from decided_by and decided_via.
func approvedBy(decidedBy storedb.OptionalID, decidedVia sql.NullString) string {
	if decidedVia.String == AutoApprovedBy || decidedVia.String == AutoCuratedBy {
		return decidedVia.String
	}
	return string(decidedBy)
}

// CreatorMatch is the WHERE condition and argument matching created_by to a member, or to no
// one for "" (the API token): IS NULL, not = NULL, which matches nothing. Both stay index lookups.
func CreatorMatch(column, createdBy string) (string, []any) {
	if createdBy == "" {
		return column + ` IS NULL`, nil
	}
	return column + ` = ?`, []any{createdBy}
}
