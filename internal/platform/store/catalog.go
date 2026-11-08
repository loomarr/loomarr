package store

import (
	"context"
	"database/sql"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// The clip catalog (clips and their tags, the taxonomy, holidays, images, broadcast stations, and
// the defect, fingerprint and creative-identity tables) lives in internal/platform/store/clipstore. Its
// types are re-exported here by alias, so importers of this package keep one import and see the
// same types, not copies.
type (
	// Clip is the persisted form of a filler clip (§10) (clipstore.Clip).
	Clip = clipstore.Clip
	// ClipFilter narrows a ListClips query (clipstore.ClipFilter).
	ClipFilter = clipstore.ClipFilter
	// ClipSort is the catalog's closed sort vocabulary (clipstore.ClipSort).
	ClipSort = clipstore.ClipSort
	// ClipTx writes a clip's columns and tags inside a caller's transaction (clipstore.ClipTx).
	ClipTx = clipstore.ClipTx
	// ClipGeography is the broadcast provenance a clip's geography columns hold
	// (clipstore.ClipGeography).
	ClipGeography = clipstore.ClipGeography
	// ClipPublication is what terminal readiness checks about a clip before publishing it
	// (clipstore.ClipPublication).
	ClipPublication = clipstore.ClipPublication
	// ClipArtworkPending is a clip whose artwork the service has not produced yet
	// (clipstore.ClipArtworkPending).
	ClipArtworkPending = clipstore.ClipArtworkPending
	// WantedCopy is an ad whose only copy failed the defect floor (clipstore.WantedCopy).
	WantedCopy = clipstore.WantedCopy
	// InferiorCopy is one copy of an ad that lost to a better copy (clipstore.InferiorCopy).
	InferiorCopy = clipstore.InferiorCopy
	// ClipDefectRecord is one clip's latest defect measurement (clipstore.ClipDefectRecord).
	ClipDefectRecord = clipstore.ClipDefectRecord
	// ClipDefectLabel is the maintainer's label on a defect record (clipstore.ClipDefectLabel).
	ClipDefectLabel = clipstore.ClipDefectLabel

	// StoreTaxon is the persisted form of a taxonomy.Taxon (clipstore.StoreTaxon).
	StoreTaxon = clipstore.StoreTaxon
	// TaxonomyEdit is one semantic graph mutation (clipstore.TaxonomyEdit).
	TaxonomyEdit = clipstore.TaxonomyEdit
	// TaxonomyEditImpact is the durable-library half of a prospective graph edit
	// (clipstore.TaxonomyEditImpact).
	TaxonomyEditImpact = clipstore.TaxonomyEditImpact
	// TaxonUsage is one taxon's clip accounting (clipstore.TaxonUsage).
	TaxonUsage = clipstore.TaxonUsage
	// TaxonomyUsage is the clip accounting of a graph edit (clipstore.TaxonomyUsage).
	TaxonomyUsage = clipstore.TaxonomyUsage

	// Image is one logical image's record (clipstore.Image).
	Image = clipstore.Image
	// ImageRef records what an image decorates (clipstore.ImageRef).
	ImageRef = clipstore.ImageRef
	// ImageDerivative is one regenerable rendition on disk (clipstore.ImageDerivative).
	ImageDerivative = clipstore.ImageDerivative
	// ImageStore is the image half of the Store contract (clipstore.ImageStore).
	ImageStore = clipstore.ImageStore
)

const (
	ClipSortName       = clipstore.ClipSortName
	ClipSortDuration   = clipstore.ClipSortDuration
	ClipSortAdded      = clipstore.ClipSortAdded
	ClipSortPlays      = clipstore.ClipSortPlays
	ClipSortConfidence = clipstore.ClipSortConfidence
)

// Sentinels the catalog returns, re-declared as the same values so errors.Is matches either name.
var (
	// ErrCorruptClipDefects is clipstore.ErrCorruptClipDefects.
	ErrCorruptClipDefects = clipstore.ErrCorruptClipDefects
	// ErrInferiorCopyMismatch is clipstore.ErrInferiorCopyMismatch.
	ErrInferiorCopyMismatch = clipstore.ErrInferiorCopyMismatch
	// ErrUnknownClipSort is clipstore.ErrUnknownClipSort.
	ErrUnknownClipSort = clipstore.ErrUnknownClipSort
)

type catalogCluster = clipstore.Store

// bindCatalog gives s its catalog cluster, built on s's own kernel handle. Every constructor of a
// sqlStore calls it; the cluster is embedded, so its methods are sqlStore's and the aggregate Store
// interface is satisfied without forwarders.
func bindCatalog(s *sqlStore) *sqlStore {
	// See identityCluster for why the embed goes through an alias.
	s.catalogCluster = clipstore.New(s.kernel(), channelPolicies{s})
	return s
}

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
