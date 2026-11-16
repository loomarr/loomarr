package acquisition

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// A download that fails retries on its own: three attempts over a day, then it rests 30 days
// (decision 12 of #3838, #3855). The per-item record below is what remembers, across restarts and
// across the source checks that would otherwise offer the same item again every pass.

// DownloadAttempts is how many times an item is tried before it rests.
const DownloadAttempts = 3

// downloadRetryDelays is the wait after the first and second failure; with the first attempt they
// spread the three attempts over a day.
var downloadRetryDelays = [DownloadAttempts - 1]time.Duration{6 * time.Hour, 18 * time.Hour}

// DownloadRest is how long an item rests once its attempts are used.
const DownloadRest = pipelinestate.FailureRest

// DownloadWait is how long an item waits for a temporary cause to clear (no room, a source that
// cannot be reached) before it is tried again. It spends no attempt.
const DownloadWait = time.Hour

// DownloadRetryAt is when an item that has now failed `failures` times is tried again: after the
// first and second failure the delays above, after the third and every later one the rest.
func DownloadRetryAt(failures int, at time.Time) time.Time {
	if failures >= DownloadAttempts {
		return at.Add(DownloadRest)
	}
	return at.Add(downloadRetryDelays[max(failures, 1)-1])
}

// DownloadItem is one item to download, as much of it as a retry needs.
type DownloadItem struct {
	// Kind is the provider (source.ID); empty for a one-off URL, whose provider the URL decides.
	Kind string
	// SourceID is the registered source the item came from; empty for a one-off URL.
	SourceID string
	// RemoteID is the provider's item id; empty for a one-off URL.
	RemoteID   string
	URL        string
	DurationMS int64
	Height     int
	Gap        string
}

// Key is the item's identity in the record: the provider's own id when it has one, else its URL.
func (d DownloadItem) Key() string {
	id := d.RemoteID
	if id == "" {
		id = "url:" + d.URL
	}
	return RemoteIdentity{Provider: d.Kind, SourceID: d.SourceID, RemoteID: id}.Key()
}

// DownloadFailure is the record of one item that has not downloaded yet.
type DownloadFailure struct {
	DownloadItem
	// Failures counts the attempts that failed. A wait for a temporary cause does not count.
	Failures      int
	FirstFailedAt time.Time
	LastFailedAt  time.Time
	// RetryAt is the earliest the item is tried again.
	RetryAt   time.Time
	LastError string
}

// DownloadLedger keeps one DownloadFailure per item until it downloads.
type DownloadLedger interface {
	// RecordDownloadFailure counts a failed attempt and schedules the next by DownloadRetryAt.
	RecordDownloadFailure(ctx context.Context, item DownloadItem, cause string, at time.Time) (DownloadFailure, error)
	// DeferDownload schedules the next attempt at retryAt for a temporary cause. It counts nothing.
	DeferDownload(ctx context.Context, item DownloadItem, reason string, retryAt, at time.Time) error
	// ClearDownload forgets an item that downloaded.
	ClearDownload(ctx context.Context, key string) error
	// RestingDownloads is every item whose next attempt is after at, by key.
	RestingDownloads(ctx context.Context, at time.Time) (map[string]time.Time, error)
	// ClaimDueDownloads returns up to limit items whose next attempt is due at at, oldest first, and
	// moves each one's next attempt to until, so a pass that dies before it finishes is not
	// followed at once by another.
	ClaimDueDownloads(ctx context.Context, at, until time.Time, limit int) ([]DownloadFailure, error)
}
