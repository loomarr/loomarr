package catalogstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/creative"
)

// ListClipFingerprints reads one algorithm's cache in a single query. Identity compares the whole
// catalog, so one lookup per clip would replace an ffmpeg bottleneck with an N+1 database
// bottleneck. A corrupt row is omitted while valid siblings remain reusable; the joined error lets
// the caller log the degradation without throwing away the partial result.
func (s *Store) ListClipFingerprints(ctx context.Context, algorithm string) (map[string]creative.Fingerprint, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT clip_hash, fingerprint_json FROM filler_clip_fingerprints WHERE algorithm = ?`), algorithm)
	if err != nil {
		return nil, fmt.Errorf("list clip fingerprints %s: %w", algorithm, err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]creative.Fingerprint)
	var invalid []error
	for rows.Next() {
		var clipHash, raw string
		if err := rows.Scan(&clipHash, &raw); err != nil {
			return out, fmt.Errorf("scan clip fingerprint %s: %w", algorithm, err)
		}
		var fp creative.Fingerprint
		if err := json.Unmarshal([]byte(raw), &fp); err != nil {
			invalid = append(invalid, fmt.Errorf("clip fingerprint %s/%s corrupt: %w", clipHash, algorithm, err))
			continue
		}
		if len(fp.Video) == 0 {
			invalid = append(invalid, fmt.Errorf("clip fingerprint %s/%s is empty", clipHash, algorithm))
			continue
		}
		out[clipHash] = fp
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list clip fingerprints %s: %w", algorithm, err)
	}
	return out, errors.Join(invalid...)
}

// UpsertClipFingerprint is the cache's only writer. Empty evidence is refused structurally: an
// undecodable clip is not evidence that it copies anything, and a later tooling repair must get
// another chance to decode it.
func (s *Store) UpsertClipFingerprint(ctx context.Context, clipHash, algorithm string, fp creative.Fingerprint) error {
	if clipHash == "" || algorithm == "" || len(fp.Video) == 0 {
		return fmt.Errorf("upsert clip fingerprint: clip hash, algorithm and video frames are required")
	}
	raw, err := json.Marshal(fp)
	if err != nil {
		return fmt.Errorf("marshal clip fingerprint %s/%s: %w", clipHash, algorithm, err)
	}
	_, err = s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO filler_clip_fingerprints (clip_hash, algorithm, fingerprint_json)
		 VALUES (?, ?, ?)
		 ON CONFLICT(clip_hash, algorithm) DO UPDATE SET fingerprint_json=excluded.fingerprint_json`),
		clipHash, algorithm, string(raw))
	if err != nil {
		return fmt.Errorf("upsert clip fingerprint %s/%s: %w", clipHash, algorithm, err)
	}
	return nil
}

func (s *Store) pruneOrphanClipFingerprints(ctx context.Context) error {
	_, err := s.h.ExecContext(ctx, `DELETE FROM filler_clip_fingerprints WHERE NOT EXISTS (
		SELECT 1 FROM clips c WHERE c.hash = filler_clip_fingerprints.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan clip fingerprints: %w", err)
	}
	return nil
}
