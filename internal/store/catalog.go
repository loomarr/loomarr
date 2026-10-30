package store

import (
	"context"
	"database/sql"

	"github.com/loomarr/loomarr/internal/store/catalogstore"
	"github.com/loomarr/loomarr/internal/store/storedb"
)

// The clip catalog (clips and their tags, the taxonomy, holidays, images, broadcast stations, and
// the defect, fingerprint and creative-identity tables) lives in internal/store/catalogstore. Its
// types are re-exported here by alias, so importers of this package keep one import and see the
// same types, not copies.
type (
	// Clip is the persisted form of a filler clip (§10) (catalogstore.Clip).
	Clip = catalogstore.Clip
	// ClipFilter narrows a ListClips query (catalogstore.ClipFilter).
	ClipFilter = catalogstore.ClipFilter
	// ClipSort is the catalog's closed sort vocabulary (catalogstore.ClipSort).
	ClipSort = catalogstore.ClipSort
	// ClipTx writes a clip's columns and tags inside a caller's transaction (catalogstore.ClipTx).
	ClipTx = catalogstore.ClipTx
	// ClipGeography is the broadcast provenance a clip's geography columns hold
	// (catalogstore.ClipGeography).
	ClipGeography = catalogstore.ClipGeography
	// ClipPublication is what terminal readiness checks about a clip before publishing it
	// (catalogstore.ClipPublication).
	ClipPublication = catalogstore.ClipPublication
	// ClipArtworkPending is a clip whose artwork the service has not produced yet
	// (catalogstore.ClipArtworkPending).
	ClipArtworkPending = catalogstore.ClipArtworkPending
	// WantedCopy is an ad whose only copy failed the defect floor (catalogstore.WantedCopy).
	WantedCopy = catalogstore.WantedCopy
	// InferiorCopy is one copy of an ad that lost to a better copy (catalogstore.InferiorCopy).
	InferiorCopy = catalogstore.InferiorCopy
	// ClipDefectRecord is one clip's latest defect measurement (catalogstore.ClipDefectRecord).
	ClipDefectRecord = catalogstore.ClipDefectRecord
	// ClipDefectLabel is the maintainer's label on a defect record (catalogstore.ClipDefectLabel).
	ClipDefectLabel = catalogstore.ClipDefectLabel

	// StoreTaxon is the persisted form of a taxonomy.Taxon (catalogstore.StoreTaxon).
	StoreTaxon = catalogstore.StoreTaxon
	// TaxonomyEdit is one semantic graph mutation (catalogstore.TaxonomyEdit).
	TaxonomyEdit = catalogstore.TaxonomyEdit
	// TaxonomyEditImpact is the durable-library half of a prospective graph edit
	// (catalogstore.TaxonomyEditImpact).
	TaxonomyEditImpact = catalogstore.TaxonomyEditImpact
	// TaxonUsage is one taxon's clip accounting (catalogstore.TaxonUsage).
	TaxonUsage = catalogstore.TaxonUsage
	// TaxonomyUsage is the clip accounting of a graph edit (catalogstore.TaxonomyUsage).
	TaxonomyUsage = catalogstore.TaxonomyUsage

	// Image is one logical image's record (catalogstore.Image).
	Image = catalogstore.Image
	// ImageRef records what an image decorates (catalogstore.ImageRef).
	ImageRef = catalogstore.ImageRef
	// ImageDerivative is one regenerable rendition on disk (catalogstore.ImageDerivative).
	ImageDerivative = catalogstore.ImageDerivative
	// ImageStore is the image half of the Store contract (catalogstore.ImageStore).
	ImageStore = catalogstore.ImageStore
)

const (
	ClipSortName       = catalogstore.ClipSortName
	ClipSortDuration   = catalogstore.ClipSortDuration
	ClipSortAdded      = catalogstore.ClipSortAdded
	ClipSortPlays      = catalogstore.ClipSortPlays
	ClipSortConfidence = catalogstore.ClipSortConfidence
)

// Sentinels the catalog returns, re-declared as the same values so errors.Is matches either name.
var (
	// ErrCorruptClipDefects is catalogstore.ErrCorruptClipDefects.
	ErrCorruptClipDefects = catalogstore.ErrCorruptClipDefects
	// ErrInferiorCopyMismatch is catalogstore.ErrInferiorCopyMismatch.
	ErrInferiorCopyMismatch = catalogstore.ErrInferiorCopyMismatch
	// ErrUnknownClipSort is catalogstore.ErrUnknownClipSort.
	ErrUnknownClipSort = catalogstore.ErrUnknownClipSort
)

type catalogCluster = catalogstore.Store

// bindCatalog gives s its catalog cluster, built on s's own kernel handle. Every constructor of a
// sqlStore calls it; the cluster is embedded, so its methods are sqlStore's and the aggregate Store
// interface is satisfied without forwarders.
func bindCatalog(s *sqlStore) *sqlStore {
	// See identityCluster for why the embed goes through an alias.
	s.catalogCluster = catalogstore.New(s.kernel(), channelPolicies{s})
	return s
}

// channelPolicies is catalogstore.ChannelPolicies over the channels table, which programming owns.
// The catalog rekeys clip hashes inside a policy and the SQL on channels stays here.
type channelPolicies struct{ s *sqlStore }

// LockChannelPolicies reads every channel's policy, locking the rows on Postgres so a concurrent
// channel CAS cannot commit between the read and SetChannelPolicy.
func (c channelPolicies) LockChannelPolicies(ctx context.Context, q storedb.Querier) (*sql.Rows, error) {
	query := `SELECT id, policy_json FROM channels`
	if c.s.dialect == DialectPostgres {
		query += ` FOR UPDATE`
	}
	return q.QueryContext(ctx, query)
}

// SetChannelPolicy replaces one channel's policy and bumps its revision.
func (c channelPolicies) SetChannelPolicy(ctx context.Context, q storedb.Querier, id, policyJSON string) error {
	_, err := q.ExecContext(ctx, c.s.ph(
		`UPDATE channels SET policy_json = ?, revision = revision + 1 WHERE id = ?`),
		policyJSON, id)
	return err
}
