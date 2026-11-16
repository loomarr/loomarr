package clipstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/clips"
)

func scanClip(sc scannable) (Clip, error) {
	var (
		c        Clip
		kind     string
		audience string
		// tunarr_program_id is NULLABLE since §9.1 — an install with no Tunarr has none,
		// which is a supported configuration. sql.NullString rather than string so a NULL
		// scans cleanly instead of erroring on a nil-to-string conversion.
		tunarrID     sql.NullString
		lastPlayedAt int64
		removedAt    int64
		// Flags are BOOLEAN on Postgres (storage.md), scanned into a bool, never an int.
		held bool
		// is_composite is BOOLEAN (00039), the same side again.
		isComposite bool
		updatedAt   int64
		// created_at is BIGINT on Postgres and INTEGER on sqlite (00046) — the same per-column
		// dialect split every epoch value here follows, and both are 64-bit.
		createdAt int64
	)
	err := sc.Scan(&c.Hash, &c.Path, &tunarrID, &c.Name, &c.NameSource, &kind, &c.Placement, &c.Era, &audience, &c.Category,
		&c.DurationMs, &c.GeographicScope, &c.Country, &c.Network, &c.Station, &c.AirDate, &c.GeoEvidence,
		&c.Source, &c.Quality, &c.License, &c.Thumbnail, &c.Preview,
		&c.ThumbImageHash, &c.HoverImageHash, &c.Language,
		&c.Transcript, &c.Brand, &c.VisibleText,
		&isComposite, &c.ParentHash,
		&c.PlayCount, &lastPlayedAt, &removedAt, &held, &updatedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Clip{}, ErrNotFound
	}
	if err != nil {
		return Clip{}, err
	}
	c.Role = clips.Role(kind)
	c.Audience = clips.Audience(audience)
	c.TunarrProgramID = tunarrID.String // "" when NULL — the no-Tunarr case
	c.IsComposite = isComposite
	c.LastPlayedAt = fromEpoch(lastPlayedAt)
	if removedAt != 0 {
		c.RemovedAt = fromEpoch(removedAt)
	}
	c.Held = held
	c.UpdatedAt = fromEpoch(updatedAt)
	c.CreatedAt = fromEpoch(createdAt)
	return c, nil
}

func scanClips(rows *sql.Rows) ([]Clip, error) {
	var out []Clip
	for rows.Next() {
		c, err := scanClip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// attachTags loads the taxonomy tag set (§10 V45a) for a batch of clips and fills both the full
// Clip.Tags match set and Clip.AssertedTags, the subset an editor may safely round-trip.
//
// ⚠ ONE query for the whole batch (`WHERE clip_hash IN (…)`), never one per clip — pod assembly loads
// the entire catalog through ListClips, so a per-clip tag read would be a textbook N+1 on the hot path
// (waiting time is invisible to a CPU profile). The full leaf+rollup set is
// loaded (leaf and rollup rows alike), because that is what curation matches against; `Category` is
// derived from it separately at write time, not here.
//
// A clip with no tag rows keeps Tags == nil, which is the honest "tagger has not reached it" state.
func (s *Store) attachTags(ctx context.Context, clips []Clip) error {
	if len(clips) == 0 {
		return nil
	}
	idx := make(map[string]int, len(clips)) // hash → position in clips, so we can fill in place
	placeholders := make([]string, len(clips))
	args := make([]any, len(clips))
	for i, c := range clips {
		idx[c.Hash] = i
		placeholders[i] = "?"
		args[i] = c.Hash
	}
	q := `SELECT clip_hash, taxon, leaf FROM clip_tags WHERE clip_hash IN (` + strings.Join(placeholders, ",") + `) ORDER BY clip_hash, taxon`
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return fmt.Errorf("attach clip tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var hash, taxon string
		var asserted bool
		if err := rows.Scan(&hash, &taxon, &asserted); err != nil {
			return err
		}
		if i, ok := idx[hash]; ok {
			clips[i].Tags = append(clips[i].Tags, taxon)
			if asserted {
				clips[i].AssertedTags = append(clips[i].AssertedTags, taxon)
			}
		}
	}
	return rows.Err()
}
