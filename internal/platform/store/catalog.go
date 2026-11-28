package store

import (
	"context"
	"database/sql"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// The clip catalog (clips and their tags, the taxonomy, holidays, images, broadcast stations, and
// the defect, fingerprint and creative-identity tables) lives in internal/platform/store/clipstore.

// channelPolicies is clipstore.ChannelPolicies over the channels table, which programming owns.
// The catalog rekeys clip hashes inside a policy and the SQL on channels stays here.
type channelPolicies struct{ s *sqlStore }

// LockChannelPolicies reads every channel's policy, locking the rows on Postgres so a concurrent
// channel CAS cannot commit between the read and SetChannelPolicy.
func (c channelPolicies) LockChannelPolicies(ctx context.Context, q storedb.Querier) (*sql.Rows, error) {
	return q.QueryContext(ctx, storedb.LockRows(c.s.dialect, `SELECT id, policy_json FROM channels`))
}

// SetChannelPolicy replaces one channel's policy and bumps its revision.
func (c channelPolicies) SetChannelPolicy(ctx context.Context, q storedb.Querier, id, policyJSON string) error {
	_, err := q.ExecContext(ctx, c.s.ph(
		`UPDATE channels SET policy_json = ?, revision = revision + 1 WHERE id = ?`),
		policyJSON, id)
	return err
}
