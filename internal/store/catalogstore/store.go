package catalogstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/store/storedb"
)

// ErrNotFound is storedb.ErrNotFound, the kernel's value under this package's name, so errors.Is
// matches an error whichever of internal/store, catalogstore or storedb produced it.
var ErrNotFound = storedb.ErrNotFound

// ErrConditionedClipMismatch reports a conditioned clip adoption whose catalog rows are not one
// of the exact source-only, source-plus-held-reconstruction, or target-only states the owner-bound
// recovery protocol permits. Callers must hold it for review.
var ErrConditionedClipMismatch = errors.New("store: conditioned clip does not match the catalog")

// ErrTaxonConflict marks a taxonomy mutation that cannot safely apply: create over an existing
// slug, or delete of a taxon still directly asserted on clips. The caller should reload/retag,
// never retry the same write blindly.
var ErrTaxonConflict = errors.New("store: taxonomy conflict")

// Store is the clip catalog's persistence: clips and their tags, the taxonomy, holidays, images,
// broadcast stations, and the defect, fingerprint and creative-identity tables, over one
// storedb.Handle.
type Store struct {
	h        storedb.Handle
	channels ChannelPolicies
}

// New returns the catalog store for h. channels is how the catalog reaches the channel policies it
// must keep consistent with clip identity; the channels table belongs to programming, so the
// aggregate supplies the implementation.
func New(h storedb.Handle, channels ChannelPolicies) *Store {
	return &Store{h: h, channels: channels}
}

// ChannelPolicies is the catalog's only reach into the channels table, which programming owns. A
// clip rekey must carry channel filler pins and exclusions to the new hash, and a split
// regeneration must keep every clip a channel pins. Both read every channel's policy under a lock
// and the rekey writes back the policies it changed; the catalog decodes and rewrites the JSON
// and the implementation owns the SQL, the Postgres row lock and the revision bump.
//
// Both methods run on q, the caller's transaction, and never begin or commit.
type ChannelPolicies interface {
	// LockChannelPolicies returns a cursor over every channel's (id, policy_json). On Postgres the
	// rows are locked until q's transaction ends, so a concurrent channel compare-and-swap cannot
	// commit between this read and SetChannelPolicy. The caller closes the rows.
	LockChannelPolicies(ctx context.Context, q storedb.Querier) (*sql.Rows, error)
	// SetChannelPolicy replaces one channel's policy_json and bumps its revision, so a stale
	// editor's compare-and-swap loses to this rewrite.
	SetChannelPolicy(ctx context.Context, q storedb.Querier, id, policyJSON string) error
}

// The kernel's codecs and scan shape under the names this cluster's statements already use.
type scannable = storedb.Scannable

func epoch(t time.Time) int64     { return storedb.Epoch(t) }
func fromEpoch(n int64) time.Time { return storedb.FromEpoch(n) }
