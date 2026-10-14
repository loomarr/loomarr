package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ListClipFingerprints reads one algorithm's cache in a single query. Duplicate detection walks
// the whole catalog, so one lookup per clip would replace an ffmpeg bottleneck with an N+1 database
// bottleneck. A corrupt row is omitted while valid siblings remain reusable; the joined error lets
// the caller log the degradation without throwing away the partial result.
func (s *sqlStore) ListClipFingerprints(ctx context.Context, algorithm string) (map[string][]uint64, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT clip_hash, frames_json FROM filler_clip_fingerprints WHERE algorithm = ?`), algorithm)
	if err != nil {
		return nil, fmt.Errorf("list clip fingerprints %s: %w", algorithm, err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]uint64)
	var invalid []error
	for rows.Next() {
		var clipHash, raw string
		if err := rows.Scan(&clipHash, &raw); err != nil {
			return out, fmt.Errorf("scan clip fingerprint %s: %w", algorithm, err)
		}
		var frames []uint64
		if err := json.Unmarshal([]byte(raw), &frames); err != nil {
			invalid = append(invalid, fmt.Errorf("clip fingerprint %s/%s corrupt: %w", clipHash, algorithm, err))
			continue
		}
		if len(frames) == 0 {
			invalid = append(invalid, fmt.Errorf("clip fingerprint %s/%s is empty", clipHash, algorithm))
			continue
		}
		out[clipHash] = frames
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list clip fingerprints %s: %w", algorithm, err)
	}
	return out, errors.Join(invalid...)
}

// UpsertClipFingerprint is the cache's only writer. Empty evidence is refused structurally: an
// undecodable clip is not evidence that it duplicates anything, and a later tooling repair must
// get another chance to decode it.
func (s *sqlStore) UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, frames []uint64) error {
	if clipHash == "" || algorithm == "" || len(frames) == 0 {
		return fmt.Errorf("upsert clip fingerprint: clip hash, algorithm and frames are required")
	}
	raw, err := json.Marshal(frames)
	if err != nil {
		return fmt.Errorf("marshal clip fingerprint %s/%s: %w", clipHash, algorithm, err)
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_clip_fingerprints (clip_hash, algorithm, frames_json)
		 VALUES (?, ?, ?)
		 ON CONFLICT(clip_hash, algorithm) DO UPDATE SET frames_json=excluded.frames_json`),
		clipHash, algorithm, string(raw))
	if err != nil {
		return fmt.Errorf("upsert clip fingerprint %s/%s: %w", clipHash, algorithm, err)
	}
	return nil
}

func (s *sqlStore) pruneOrphanClipFingerprints(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM filler_clip_fingerprints WHERE NOT EXISTS (
		SELECT 1 FROM clips c WHERE c.hash = filler_clip_fingerprints.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan clip fingerprints: %w", err)
	}
	return nil
}
