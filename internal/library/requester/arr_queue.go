package requester

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/httpx"
)

// QueueItem is one title's download status from the arr queue — what the queue poller (§18.1)
// reports to drive the Grabbed transition + surface progress. Progress is a 0..1
// fraction (1 - sizeleft/size); zero size ⇒ 0. Grabbed reports whether the title is genuinely
// in flight (a real download record exists), which promotes requested→downloading.
type QueueItem struct {
	Key      provision.Key
	Grabbed  bool    // a queue record exists for this title (downloading, not merely requested)
	Progress float64 // 0..1 completion; 0 when unknown
	// ETAText is the arr's human time-left string (e.g. "00:14:32"), passed through for display.
	ETAText string
	// Status is the arr's queue status ("downloading", "queued", "warning", "completed", …) —
	// exposed so a stalled/errored download is visible rather than looking like healthy progress.
	Status string
	// EpisodesHave of EpisodesWanted is a series' "8 of 36 episodes" (#1667): Sonarr's files on
	// disk of the episodes it wants (monitored and aired). Both 0 for a movie or when unknown.
	EpisodesHave   int
	EpisodesWanted int
}

// arrSeriesStats is the slice of Sonarr's GET /api/v3/series/{id} we read. The lookup the poll
// already makes carries no statistics, so a grabbed series costs this one extra read.
type arrSeriesStats struct {
	Statistics struct {
		EpisodeFileCount int `json:"episodeFileCount"`
		EpisodeCount     int `json:"episodeCount"`
	} `json:"statistics"`
}

// arrQueueRecord is the slice of an /api/v3/queue record we read. movieId/seriesId correlate
// to the arr's internal id; size/sizeleft give progress; timeleft/status are display.
type arrQueueRecord struct {
	ID       int     `json:"id"`
	MovieID  int     `json:"movieId"`
	SeriesID int     `json:"seriesId"`
	Size     float64 `json:"size"`
	SizeLeft float64 `json:"sizeleft"`
	TimeLeft string  `json:"timeleft"`
	Status   string  `json:"status"`
}

type arrQueuePage struct {
	Records []arrQueueRecord `json:"records"`
}

// arrID is the arr-internal id this record belongs to for the endpoint kind.
func (r arrQueueRecord) arrID(kind string) int {
	if kind == "series" {
		return r.SeriesID
	}
	return r.MovieID
}

// fetchQueue reads the arr's queue. A non-2xx answer (a wrong API key is a 401 with a JSON
// body) is an error, never an empty queue.
func (a *Arr) fetchQueue(ctx context.Context, ep arrEndpoint) ([]arrQueueRecord, error) {
	resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/queue?pageSize=1000", nil)
	if err != nil {
		return nil, fmt.Errorf("arr queue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return nil, fmt.Errorf("arr queue: %w", err)
	}
	var page arrQueuePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("arr queue decode: %w", err)
	}
	return page.Records, nil
}

// aggregateQueue folds a title's queue records into one. A series has a record per episode,
// so progress is bytes done over bytes total across all of them. The status and time left are
// the lead record's: among the records actually downloading (or, when none is, all of them),
// the one with the most left to fetch, since that is what the title still waits on.
func aggregateQueue(recs []arrQueueRecord) (progress float64, eta, status string) {
	var size, left float64
	for _, r := range recs {
		size += r.Size
		left += r.SizeLeft
	}
	candidates := recs
	if slices.ContainsFunc(recs, isDownloading) {
		candidates = slices.DeleteFunc(slices.Clone(recs), func(r arrQueueRecord) bool { return !isDownloading(r) })
	}
	lead := candidates[0]
	for _, r := range candidates[1:] {
		if r.SizeLeft > lead.SizeLeft {
			lead = r
		}
	}
	return progressFraction(size, left), lead.TimeLeft, lead.Status
}

func isDownloading(r arrQueueRecord) bool { return r.Status == "downloading" }

// QueueStatus reports download status for a set of in-flight titles. It fetches each
// configured arr's queue ONCE, indexes records by the arr-internal id, then resolves each
// title's arr id via a lookup and probes the index — so the poll costs one queue fetch per arr
// plus one lookup per title (the in-flight set is small). A title with no queue record is
// returned with Grabbed=false (still merely requested, or already imported). Titles whose arr
// isn't configured are skipped.
func (a *Arr) QueueStatus(ctx context.Context, titles []provision.Title) ([]QueueItem, error) {
	// Fetch + index each arr's queue lazily (only if a title needs it), keyed by kind.
	byKind := map[string]map[int][]arrQueueRecord{} // "movie"/"series" → arrID → records

	loadQueue := func(ep arrEndpoint) error {
		if _, done := byKind[ep.kind]; done {
			return nil
		}
		records, err := a.fetchQueue(ctx, ep)
		if err != nil {
			return err
		}
		idx := make(map[int][]arrQueueRecord, len(records))
		for _, rec := range records {
			if id := rec.arrID(ep.kind); id != 0 {
				idx[id] = append(idx[id], rec)
			}
		}
		byKind[ep.kind] = idx
		return nil
	}

	out := make([]QueueItem, 0, len(titles))
	for _, t := range titles {
		key, err := t.Key()
		if err != nil {
			continue // no usable key → can't correlate
		}
		ep, err := a.endpointFor(t)
		if err != nil {
			continue // that arr isn't configured — skip (Grabbed stays false for this title)
		}
		if err := loadQueue(ep); err != nil {
			return nil, err
		}
		// Resolve this title's arr-internal id via lookup, then probe the queue index.
		obj, err := a.lookup(ctx, ep)
		if err != nil {
			// A lookup failure for one title shouldn't abort the whole poll.
			out = append(out, QueueItem{Key: key})
			continue
		}
		arrID, _ := toInt(obj["id"])
		recs := byKind[ep.kind][arrID]
		if arrID == 0 || len(recs) == 0 {
			out = append(out, QueueItem{Key: key}) // not in the queue → not grabbed
			continue
		}
		progress, eta, status := aggregateQueue(recs)
		item := QueueItem{Key: key, Grabbed: true, Progress: progress, ETAText: eta, Status: status}
		if ep.kind == "series" {
			item.EpisodesHave, item.EpisodesWanted = a.seriesEpisodeCounts(ctx, ep, arrID)
		}
		out = append(out, item)
	}
	return out, nil
}

// seriesEpisodeCounts reads a series' files-on-disk and wanted episode counts. The counts only
// decorate the progress, so a failed read reports none rather than failing the poll.
func (a *Arr) seriesEpisodeCounts(ctx context.Context, ep arrEndpoint, arrID int) (have, wanted int) {
	resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/series/"+strconv.Itoa(arrID), nil)
	if err != nil {
		return 0, 0
	}
	defer func() { _ = resp.Body.Close() }()
	var series arrSeriesStats
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&series) != nil {
		return 0, 0
	}
	return series.Statistics.EpisodeFileCount, series.Statistics.EpisodeCount
}

// progressFraction converts size/sizeleft to a 0..1 completion fraction; unknown size ⇒ 0.
func progressFraction(size, sizeLeft float64) float64 {
	if size <= 0 {
		return 0
	}
	done := (size - sizeLeft) / size
	if done < 0 {
		return 0
	}
	if done > 1 {
		return 1
	}
	return done
}
