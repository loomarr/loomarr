package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

const catalogProjectionBackfillVersion = 1

const enrichmentColumns = `clip_hash, axis, state, value_json, evidence_kind,
       evidence_rank, evidence_reference, confidence, producer, producer_version,
       taxonomy_version, observed_at`

func (s *sqlStore) ListFillerEnrichment(ctx context.Context, clipHash string) ([]fillerenrichment.State, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+enrichmentColumns+`
		FROM filler_enrichment_axes WHERE clip_hash = ? ORDER BY axis`), clipHash)
	if err != nil {
		return nil, fmt.Errorf("list filler enrichment: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []fillerenrichment.State
	for rows.Next() {
		state, err := scanEnrichment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func (s *sqlStore) ListFillerEnrichmentCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]Clip, error) {
	if limit <= 0 {
		return []Clip{}, nil
	}
	query := clipSelect + ` WHERE removed_at = 0 AND is_composite = false
		AND NOT EXISTS (
			SELECT 1 FROM filler_enrichment_passes p
			WHERE p.clip_hash = clips.hash AND p.producer = ? AND p.producer_version = ? AND p.taxonomy_version = ?
		)
		ORDER BY created_at, hash LIMIT ?`
	rows, err := s.db.QueryContext(ctx, s.ph(query), producer, producerVersion, taxonomyVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("list filler enrichment candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	clips, err := scanClips(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, clips); err != nil {
		return nil, err
	}
	return clips, nil
}

type enrichmentScanner interface{ Scan(...any) error }

func scanEnrichment(row enrichmentScanner) (fillerenrichment.State, error) {
	var state fillerenrichment.State
	var valueJSON string
	var storedRank int
	var observedAt int64
	if err := row.Scan(&state.ClipHash, &state.Axis, &state.Status, &valueJSON,
		&state.Evidence.Kind, &storedRank, &state.Evidence.Reference, &state.Evidence.Confidence,
		&state.Evidence.Producer, &state.Evidence.ProducerVersion, &state.Evidence.TaxonomyVersion,
		&observedAt); err != nil {
		return fillerenrichment.State{}, fmt.Errorf("scan filler enrichment: %w", err)
	}
	value, err := fillerenrichment.DecodeValue(valueJSON)
	if err != nil {
		return fillerenrichment.State{}, fmt.Errorf("scan filler enrichment value: %w", err)
	}
	state.Value = value
	state.Evidence.ObservedAt = fromEpoch(observedAt)
	rank, ok := state.Evidence.Kind.Rank()
	if !ok || int(rank) != storedRank {
		return fillerenrichment.State{}, fmt.Errorf("scan filler enrichment: evidence rank %d does not match kind %q", storedRank, state.Evidence.Kind)
	}
	if err := state.Validate(); err != nil {
		return fillerenrichment.State{}, fmt.Errorf("scan filler enrichment: %w", err)
	}
	return state, nil
}

func (s *sqlStore) ApplyFillerEnrichment(ctx context.Context, candidate fillerenrichment.State, updatedAt time.Time) (fillerenrichment.State, bool, error) {
	if err := candidate.Validate(); err != nil {
		return fillerenrichment.State{}, false, err
	}
	// Missing is represented by no row. Persisting it would invent provenance for work that has not
	// happened and cannot satisfy the table's evidence invariants.
	if candidate.Status == fillerenrichment.StatusMissing {
		return fillerenrichment.State{}, false, fmt.Errorf("%w: missing enrichment state is not persisted", fillerenrichment.ErrInvalidState)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fillerenrichment.State{}, false, fmt.Errorf("apply filler enrichment: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnrichmentClipTx(ctx, tx, candidate.ClipHash); err != nil {
		return fillerenrichment.State{}, false, err
	}
	accepted, changed, err := s.applyFillerEnrichmentTx(ctx, tx, candidate, updatedAt)
	if err != nil || !changed {
		return accepted, false, err
	}
	if err := tx.Commit(); err != nil {
		return fillerenrichment.State{}, false, fmt.Errorf("apply filler enrichment: commit: %w", err)
	}
	return accepted, true, nil
}

func (s *sqlStore) requireEnrichmentClipTx(ctx context.Context, tx *sql.Tx, clipHash string) error {
	var clipExists int
	if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clips WHERE hash = ?`), clipHash).Scan(&clipExists); err != nil {
		return fmt.Errorf("apply filler enrichment: find clip: %w", err)
	}
	if clipExists == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) applyFillerEnrichmentTx(ctx context.Context, tx *sql.Tx, candidate fillerenrichment.State, updatedAt time.Time) (fillerenrichment.State, bool, error) {
	grounded, err := s.groundEnrichmentStateTx(ctx, tx, candidate)
	if err != nil {
		return fillerenrichment.State{}, false, err
	}
	candidate = grounded
	query := `SELECT ` + enrichmentColumns + ` FROM filler_enrichment_axes WHERE clip_hash = ? AND axis = ?`
	if s.dialect == DialectPostgres {
		query += ` FOR UPDATE`
	}
	current, err := scanEnrichment(tx.QueryRowContext(ctx, s.ph(query), candidate.ClipHash, string(candidate.Axis)))
	if errors.Is(err, sql.ErrNoRows) || errors.Is(errors.Unwrap(err), sql.ErrNoRows) {
		current = fillerenrichment.State{}
	} else if err != nil {
		return fillerenrichment.State{}, false, fmt.Errorf("apply filler enrichment: read current: %w", err)
	}
	accepted, changed, err := fillerenrichment.Apply(current, candidate)
	if err != nil || !changed {
		return accepted, false, err
	}
	valueJSON, err := fillerenrichment.EncodeValue(accepted.Value)
	if err != nil {
		return fillerenrichment.State{}, false, fmt.Errorf("apply filler enrichment: encode value: %w", err)
	}
	rank, _ := accepted.Evidence.Kind.Rank()
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_enrichment_axes
		(clip_hash, axis, state, value_json, evidence_kind, evidence_rank, evidence_reference,
		 confidence, producer, producer_version, taxonomy_version, observed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(clip_hash, axis) DO UPDATE SET
		 state=excluded.state, value_json=excluded.value_json, evidence_kind=excluded.evidence_kind,
		 evidence_rank=excluded.evidence_rank, evidence_reference=excluded.evidence_reference,
		 confidence=excluded.confidence, producer=excluded.producer,
		 producer_version=excluded.producer_version, taxonomy_version=excluded.taxonomy_version,
		 observed_at=excluded.observed_at, updated_at=excluded.updated_at`),
		accepted.ClipHash, string(accepted.Axis), string(accepted.Status), valueJSON,
		string(accepted.Evidence.Kind), int(rank), accepted.Evidence.Reference,
		accepted.Evidence.Confidence, accepted.Evidence.Producer, accepted.Evidence.ProducerVersion,
		accepted.Evidence.TaxonomyVersion, epoch(accepted.Evidence.ObservedAt), epoch(updatedAt)); err != nil {
		return fillerenrichment.State{}, false, fmt.Errorf("apply filler enrichment: write: %w", err)
	}
	if err := s.projectFillerEnrichmentTx(ctx, tx, accepted, updatedAt); err != nil {
		return fillerenrichment.State{}, false, err
	}
	return accepted, true, nil
}

func taxonomyEnrichmentAxis(axis fillerenrichment.Axis) bool {
	switch axis {
	case fillerenrichment.AxisProduct, fillerenrichment.AxisFormat, fillerenrichment.AxisSeasonal,
		fillerenrichment.AxisAudienceCue, fillerenrichment.AxisPresentation:
		return true
	default:
		return false
	}
}

func (s *sqlStore) groundEnrichmentStateTx(ctx context.Context, tx *sql.Tx, state fillerenrichment.State) (fillerenrichment.State, error) {
	if !taxonomyEnrichmentAxis(state.Axis) || len(state.Value.Tags) == 0 {
		return state, nil
	}
	grounded := make([]string, 0, len(state.Value.Tags))
	for _, tag := range state.Value.Tags {
		var exists int
		if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM taxa WHERE slug = ?`), tag).Scan(&exists); err != nil {
			return fillerenrichment.State{}, fmt.Errorf("ground filler enrichment taxon %s: %w", tag, err)
		}
		if exists > 0 {
			grounded = append(grounded, tag)
		}
	}
	state.Value.Tags = grounded
	return state, nil
}

func (s *sqlStore) projectFillerEnrichmentTx(ctx context.Context, tx *sql.Tx, state fillerenrichment.State, updatedAt time.Time) error {
	var err error
	switch state.Axis {
	case fillerenrichment.AxisEra:
		if state.Value.Year > 0 {
			_, err = tx.ExecContext(ctx, s.ph(`UPDATE clips SET era = ?, updated_at = ? WHERE hash = ? AND era = 0`), state.Value.Year, epoch(updatedAt), state.ClipHash)
		}
	case fillerenrichment.AxisAudience:
		if audience := filler.AudienceFromString(state.Value.Text); audience != "" {
			_, err = tx.ExecContext(ctx, s.ph(`UPDATE clips SET audience = ?, updated_at = ? WHERE hash = ? AND audience = ''`), string(audience), epoch(updatedAt), state.ClipHash)
		}
	case fillerenrichment.AxisBrand:
		if state.Value.Text != "" {
			_, err = tx.ExecContext(ctx, s.ph(`UPDATE clips SET brand = ?, updated_at = ? WHERE hash = ? AND brand = ''`), state.Value.Text, epoch(updatedAt), state.ClipHash)
		}
	case fillerenrichment.AxisLanguage:
		if state.Value.Text != "" {
			_, err = tx.ExecContext(ctx, s.ph(`UPDATE clips SET language = ?, updated_at = ? WHERE hash = ? AND language = ''`), state.Value.Text, epoch(updatedAt), state.ClipHash)
		}
	case fillerenrichment.AxisGeography:
		g := state.Value.Geography
		if g != (fillerenrichment.Geography{}) {
			_, err = tx.ExecContext(ctx, s.ph(`UPDATE clips SET geographic_scope = ?, country = ?, market = ?,
				network = ?, station = ?, air_date = ?, geo_evidence = ?, updated_at = ?
				WHERE hash = ? AND (geographic_scope = '' OR geographic_scope = 'unknown') AND country = ''`),
				g.Scope, g.Country, g.Market, g.Network, g.Station, g.AirDate, state.Evidence.Reference,
				epoch(updatedAt), state.ClipHash)
		}
	default:
		if taxonomyEnrichmentAxis(state.Axis) && len(state.Value.Tags) > 0 {
			if s.dialect == DialectPostgres {
				if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(hashtext('loomarr-taxonomy'))`); err != nil {
					return fmt.Errorf("project filler enrichment taxonomy lock: %w", err)
				}
			}
			leaves, err := getClipTagsFrom(ctx, tx, s.ph, state.ClipHash, true)
			if err != nil {
				return err
			}
			seen := make(map[string]bool, len(leaves)+len(state.Value.Tags))
			for _, leaf := range leaves {
				seen[leaf] = true
			}
			for _, tag := range state.Value.Tags {
				if !seen[tag] {
					leaves = append(leaves, tag)
					seen[tag] = true
				}
			}
			sort.Strings(leaves)
			if err := s.setClipTagsTx(ctx, tx, state.ClipHash, leaves); err != nil {
				return err
			}
		}
	}
	if err != nil {
		return fmt.Errorf("project filler enrichment %s: %w", state.Axis, err)
	}
	return nil
}

func (s *sqlStore) ApplyFillerEnrichmentPass(ctx context.Context, pass fillerenrichment.Pass) (int, error) {
	if err := pass.Validate(); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnrichmentClipTx(ctx, tx, pass.ClipHash); err != nil {
		return 0, err
	}
	changed := 0
	for _, candidate := range pass.States {
		if _, applied, err := s.applyFillerEnrichmentTx(ctx, tx, candidate, pass.CompletedAt); err != nil {
			return 0, err
		} else if applied {
			changed++
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_enrichment_passes
		(clip_hash, producer, producer_version, taxonomy_version, completed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(clip_hash, producer, producer_version, taxonomy_version) DO UPDATE SET
		 completed_at=excluded.completed_at`), pass.ClipHash, pass.Producer, pass.ProducerVersion,
		pass.TaxonomyVersion, epoch(pass.CompletedAt)); err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: record completion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: commit: %w", err)
	}
	return changed, nil
}

// backfillFillerEnrichment captures pre-axis catalog facts once so the new rank-aware module cannot
// mistake a real operator/item/content fact for an empty axis. It is restart-idempotent and records
// only the current projection; the old clip-wide origin flags are not used after this conversion.
func (s *sqlStore) backfillFillerEnrichment(ctx context.Context, completedAt time.Time) error {
	var applied int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_enrichment_backfills WHERE version = ?`), catalogProjectionBackfillVersion).Scan(&applied); err != nil {
		return fmt.Errorf("backfill filler enrichment: read marker: %w", err)
	}
	if applied > 0 {
		return nil
	}
	clips, err := s.ListClips(ctx, ClipFilter{IncludeHeld: true, IncludeRemoved: true, IncludeComposites: true})
	if err != nil {
		return fmt.Errorf("backfill filler enrichment: list clips: %w", err)
	}
	taxa, err := s.ListTaxa(ctx)
	if err != nil {
		return fmt.Errorf("backfill filler enrichment: list taxonomy: %w", err)
	}
	taxonAxes := make(map[string]fillerenrichment.Axis, len(taxa))
	for _, taxon := range taxa {
		taxonAxes[taxon.Slug] = fillerenrichment.Axis(taxon.Axis)
	}
	for _, clip := range clips {
		observedAt := clip.UpdatedAt
		if observedAt.IsZero() {
			observedAt = completedAt
		}
		base := func(axis fillerenrichment.Axis, value fillerenrichment.Value, kind fillerenrichment.EvidenceKind, reference string) fillerenrichment.State {
			confidence := clip.Confidence
			if confidence == 0 && kind != fillerenrichment.EvidenceInference {
				confidence = 100
			}
			return fillerenrichment.State{ClipHash: clip.Hash, Axis: axis, Status: fillerenrichment.StatusComplete, Value: value,
				Evidence: fillerenrichment.Evidence{Kind: kind, Reference: reference, Confidence: confidence,
					Producer: "catalog-projection", ProducerVersion: "1", ObservedAt: observedAt}}
		}
		textKind := fillerenrichment.EvidenceItem
		if clip.AITagged {
			textKind = fillerenrichment.EvidenceInference
		}
		var states []fillerenrichment.State
		if clip.Era > 0 {
			states = append(states, base(fillerenrichment.AxisEra, fillerenrichment.Value{Year: clip.Era}, textKind, "catalog.era"))
		}
		if clip.Audience != "" {
			states = append(states, base(fillerenrichment.AxisAudience, fillerenrichment.Value{Text: string(clip.Audience)}, textKind, "catalog.audience"))
		}
		if clip.Brand != "" {
			kind := textKind
			if clip.VisionTagged {
				kind = fillerenrichment.EvidenceContent
			}
			states = append(states, base(fillerenrichment.AxisBrand, fillerenrichment.Value{Text: clip.Brand}, kind, "catalog.brand"))
		}
		if clip.Language != "" {
			states = append(states, base(fillerenrichment.AxisLanguage, fillerenrichment.Value{Text: clip.Language}, fillerenrichment.EvidenceContent, "catalog.language"))
		}
		if clip.Country != "" || clip.Market != "" || clip.Network != "" || clip.Station != "" {
			kind := fillerenrichment.EvidenceItem
			if strings.EqualFold(clip.GeoEvidence, "operator") {
				kind = fillerenrichment.EvidenceOperator
			}
			states = append(states, base(fillerenrichment.AxisGeography, fillerenrichment.Value{Geography: fillerenrichment.Geography{
				Scope: string(clip.GeographicScope), Country: clip.Country, Market: clip.Market,
				Network: clip.Network, Station: clip.Station, AirDate: clip.AirDate,
			}}, kind, "catalog.geography"))
		}
		byAxis := make(map[fillerenrichment.Axis][]string)
		for _, tag := range clip.AssertedTags {
			if axis := taxonAxes[tag]; axis.Valid() && taxonomyEnrichmentAxis(axis) {
				byAxis[axis] = append(byAxis[axis], tag)
			}
		}
		for axis, tags := range byAxis {
			kind := textKind
			if clip.VisionTagged {
				kind = fillerenrichment.EvidenceContent
			}
			state := base(axis, fillerenrichment.Value{Tags: tags}, kind, "catalog.taxonomy")
			state.Evidence.TaxonomyVersion = fillerenrichment.ControlledTaxonomyVersion
			states = append(states, state)
		}
		for _, state := range states {
			if _, _, err := s.ApplyFillerEnrichment(ctx, state, completedAt); err != nil {
				return fmt.Errorf("backfill filler enrichment %s/%s: %w", clip.Hash, state.Axis, err)
			}
		}
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_enrichment_backfills (version, completed_at)
		VALUES (?, ?) ON CONFLICT(version) DO NOTHING`), catalogProjectionBackfillVersion, epoch(completedAt)); err != nil {
		return fmt.Errorf("backfill filler enrichment: record marker: %w", err)
	}
	return nil
}
