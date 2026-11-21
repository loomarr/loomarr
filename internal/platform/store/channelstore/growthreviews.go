package channelstore

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Growth review verdicts (#4333): the model's fixed-choice answer for a title on a channel.
const (
	GrowthFits       = "fits"
	GrowthDoesNotFit = "does_not_fit"
)

// GrowthReview is the model's verdict on one title for one channel, given against Line.
type GrowthReview struct {
	ChannelID  string
	Key        provision.Key
	Line       string
	Verdict    string // GrowthFits or GrowthDoesNotFit
	Reason     string
	Model      string
	ReviewedAt time.Time
}

// GrowthReviews is every verdict the channel holds, by title.
func (s *Store) GrowthReviews(ctx context.Context, channelID string) (map[provision.Key]GrowthReview, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT title_key, line, verdict, reason, model, reviewed_at FROM growth_reviews WHERE channel_id = ?`), channelID)
	if err != nil {
		return nil, fmt.Errorf("growth reviews %s: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[provision.Key]GrowthReview{}
	for rows.Next() {
		r := GrowthReview{ChannelID: channelID}
		var key string
		var at int64
		if err := rows.Scan(&key, &r.Line, &r.Verdict, &r.Reason, &r.Model, &at); err != nil {
			return nil, fmt.Errorf("growth reviews %s: %w", channelID, err)
		}
		r.Key, r.ReviewedAt = provision.Key(key), storedb.FromEpoch(at)
		out[r.Key] = r
	}
	return out, rows.Err()
}

// SaveGrowthReview records r, replacing the channel's earlier verdict on the same title.
func (s *Store) SaveGrowthReview(ctx context.Context, r GrowthReview) error {
	if r.Verdict != GrowthFits && r.Verdict != GrowthDoesNotFit {
		return fmt.Errorf("growth review %s %s: verdict %q", r.ChannelID, r.Key, r.Verdict)
	}
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO growth_reviews (channel_id, title_key, line, verdict, reason, model, reviewed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (channel_id, title_key) DO UPDATE SET line = excluded.line, verdict = excluded.verdict,
		   reason = excluded.reason, model = excluded.model, reviewed_at = excluded.reviewed_at`),
		r.ChannelID, string(r.Key), r.Line, r.Verdict, r.Reason, r.Model, storedb.Epoch(r.ReviewedAt))
	if err != nil {
		return fmt.Errorf("save growth review %s %s: %w", r.ChannelID, r.Key, err)
	}
	return nil
}
