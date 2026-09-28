package fillerstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/store"
)

const catalogProjectionBackfillVersion = 2

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

func (s *sqlStore) ListFillerEnrichmentCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]store.Clip, error) {
	return s.listFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit, true)
}

// ListFillerEnrichmentCapabilityCandidates selects an exact paid-capability identity only once.
// Unlike free/text projections, transcript and frame work does not become payable again merely
// because another descriptive input advanced the clip revision. A changed provider/model/prompt or
// taxonomy has a different identity and is therefore selected once in its own right.
func (s *sqlStore) ListFillerEnrichmentCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]store.Clip, error) {
	return s.listFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit, false)
}

// listFillerEnrichmentCandidates picks the candidate identities here, where the pass table is, and
// loads the clips through the core's batch read, which owns how a clip and its tags are read.
func (s *sqlStore) listFillerEnrichmentCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int, currentRevision bool) ([]store.Clip, error) {
	if limit <= 0 {
		return []store.Clip{}, nil
	}
	revisionPredicate := ""
	if currentRevision {
		revisionPredicate = " AND p.input_revision = clips.enrichment_revision"
	}
	query := `SELECT hash FROM clips WHERE removed_at = 0 AND is_composite = false
		AND NOT EXISTS (
			SELECT 1 FROM filler_enrichment_passes p
			WHERE p.clip_hash = clips.hash AND p.producer = ? AND p.producer_version = ? AND p.taxonomy_version = ?
			  ` + revisionPredicate + `
		)
		ORDER BY created_at, hash LIMIT ?`
	rows, err := s.db.QueryContext(ctx, s.ph(query), producer, producerVersion, taxonomyVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("list filler enrichment candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("list filler enrichment candidates: %w", err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filler enrichment candidates: %w", err)
	}
	if len(hashes) == 0 {
		return []store.Clip{}, nil
	}
	// Held clips are enrichment work too, so the batch read lifts its held exclusion; removed and
	// composite clips stay excluded, as the selection above already required.
	loaded, err := s.core.ListClips(ctx, store.ClipFilter{Hashes: hashes, IncludeHeld: true})
	if err != nil {
		return nil, fmt.Errorf("load filler enrichment candidates: %w", err)
	}
	byHash := make(map[string]store.Clip, len(loaded))
	for _, clip := range loaded {
		byHash[clip.Hash] = clip
	}
	clips := make([]store.Clip, 0, len(hashes))
	for _, hash := range hashes {
		if clip, ok := byHash[hash]; ok {
			clips = append(clips, clip)
		}
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
	tx, err := s.db.Begin(ctx)
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

func (s *sqlStore) requireEnrichmentClipTx(ctx context.Context, tx store.Tx, clipHash string) error {
	if err := s.db.Clips(tx).Require(ctx, clipHash); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("apply filler enrichment: %w", err)
	} else if err != nil {
		return err
	}
	return nil
}

func (s *sqlStore) applyFillerEnrichmentTx(ctx context.Context, tx store.Tx, candidate fillerenrichment.State, updatedAt time.Time) (fillerenrichment.State, bool, error) {
	grounded, err := s.groundEnrichmentStateTx(ctx, tx, candidate)
	if err != nil {
		return fillerenrichment.State{}, false, err
	}
	candidate = grounded
	query := `SELECT ` + enrichmentColumns + ` FROM filler_enrichment_axes WHERE clip_hash = ? AND axis = ?`
	if s.dialect == store.DialectPostgres {
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

func (s *sqlStore) groundEnrichmentStateTx(ctx context.Context, tx store.Tx, state fillerenrichment.State) (fillerenrichment.State, error) {
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

func (s *sqlStore) projectFillerEnrichmentTx(ctx context.Context, tx store.Tx, state fillerenrichment.State, updatedAt time.Time) error {
	var err error
	clips := s.db.Clips(tx)
	switch state.Axis {
	case fillerenrichment.AxisKind:
		if kind := filler.Kind(state.Value.Text); kind == filler.Commercial || kind == filler.Bumper ||
			kind == filler.StationID || kind == filler.PSA || kind == filler.Trailer || kind == filler.Interstitial {
			err = clips.ClassifyKind(ctx, state.ClipHash, kind, updatedAt)
		}
	case fillerenrichment.AxisEra:
		if state.Value.Year > 0 {
			err = clips.SetEra(ctx, state.ClipHash, state.Value.Year, updatedAt)
		}
	case fillerenrichment.AxisAudience:
		if audience := filler.AudienceFromString(state.Value.Text); audience != "" {
			err = clips.FillAudience(ctx, state.ClipHash, string(audience), updatedAt)
		}
	case fillerenrichment.AxisBrand:
		if state.Value.Text != "" {
			err = clips.SetBrand(ctx, state.ClipHash, state.Value.Text, updatedAt)
		}
	case fillerenrichment.AxisLanguage:
		if state.Value.Text != "" {
			err = clips.FillLanguage(ctx, state.ClipHash, state.Value.Text, updatedAt)
		}
	case fillerenrichment.AxisGeography:
		g := state.Value.Geography
		if g != (fillerenrichment.Geography{}) {
			err = clips.FillGeography(ctx, state.ClipHash, store.ClipGeography{
				Scope: g.Scope, Country: g.Country, Market: g.Market,
				Network: g.Network, Station: g.Station, AirDate: g.AirDate,
			}, state.Evidence.Reference, updatedAt)
		}
	default:
		if taxonomyEnrichmentAxis(state.Axis) && len(state.Value.Tags) > 0 {
			if err := clips.AddLeafTags(ctx, state.ClipHash, state.Value.Tags); err != nil {
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.requireEnrichmentClipTx(ctx, tx, pass.ClipHash); err != nil {
		return 0, err
	}
	clips := s.db.Clips(tx)
	inputRevision, err := clips.EnrichmentRevision(ctx, pass.ClipHash)
	if err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: read input revision: %w", err)
	}
	changed := 0
	for _, candidate := range pass.States {
		if _, applied, err := s.applyFillerEnrichmentTx(ctx, tx, candidate, pass.CompletedAt); err != nil {
			return 0, err
		} else if applied {
			changed++
		}
	}
	if pass.Observation != nil && pass.Observation.Transcript != nil {
		if err := clips.RecordTranscript(ctx, pass.ClipHash, *pass.Observation.Transcript, pass.CompletedAt); err != nil {
			return 0, fmt.Errorf("apply filler enrichment pass: record transcript observation: %w", err)
		}
	}
	if pass.Observation != nil && pass.Observation.Vision != nil {
		vision := pass.Observation.Vision
		if err := clips.RecordVision(ctx, pass.ClipHash, vision.VisibleText, vision.SuggestedEra, pass.CompletedAt); err != nil {
			return 0, fmt.Errorf("apply filler enrichment pass: record vision observation: %w", err)
		}
	}
	// The pass owns the exact input revision it just committed. Reading again after raw media
	// observations prevents that same expensive result from immediately waking its own runner.
	if inputRevision, err = clips.EnrichmentRevision(ctx, pass.ClipHash); err != nil {
		return 0, fmt.Errorf("apply filler enrichment pass: refresh input revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_enrichment_passes
		(clip_hash, producer, producer_version, taxonomy_version, input_revision, completed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(clip_hash, producer, producer_version, taxonomy_version) DO UPDATE SET
		 input_revision=excluded.input_revision, completed_at=excluded.completed_at`),
		pass.ClipHash, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion,
		inputRevision, epoch(pass.CompletedAt)); err != nil {
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
// Open runs it after the core's own boot seeds, so the taxonomy it reads is already converged.
func (s *sqlStore) backfillFillerEnrichment(ctx context.Context, completedAt time.Time) error {
	var applied int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_enrichment_backfills WHERE version = ?`), catalogProjectionBackfillVersion).Scan(&applied); err != nil {
		return fmt.Errorf("backfill filler enrichment: read marker: %w", err)
	}
	if applied > 0 {
		return nil
	}
	clips, err := s.core.ListClips(ctx, store.ClipFilter{IncludeHeld: true, IncludeRemoved: true, IncludeComposites: true})
	if err != nil {
		return fmt.Errorf("backfill filler enrichment: list clips: %w", err)
	}
	taxa, err := s.core.ListTaxa(ctx)
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
		if clip.Kind != filler.Unclassified {
			states = append(states, base(fillerenrichment.AxisKind, fillerenrichment.Value{Text: string(clip.Kind)}, textKind, "catalog.kind"))
		}
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
