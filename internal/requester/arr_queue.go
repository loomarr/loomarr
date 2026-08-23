package requester

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/loomarr/loomarr/internal/provision"
)

// QueueItem is one title's download status from the arr queue — what the queue poller (§18.1,
// Phase 4) reports to drive the Grabbed transition + surface progress. Progress is a 0..1
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
}

// arrQueueRecord is the slice of an /api/v3/queue record we read. movieId/seriesId correlate
// to the arr's internal id; size/sizeleft give progress; timeleft/status are display.
type arrQueueRecord struct {
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

// QueueStatus reports download status for a set of in-flight titles (Phase 4). It fetches each
// configured arr's queue ONCE, indexes records by the arr-internal id, then resolves each
// title's arr id via a lookup and probes the index — so the poll costs one queue fetch per arr
// plus one lookup per title (the in-flight set is small). A title with no queue record is
// returned with Grabbed=false (still merely requested, or already imported). Titles whose arr
// isn't configured are skipped.
func (a *Arr) QueueStatus(ctx context.Context, titles []provision.Title) ([]QueueItem, error) {
	// Fetch + index each arr's queue lazily (only if a title needs it), keyed by kind.
	byKind := map[string]map[int]arrQueueRecord{} // "movie"/"series" → arrID → record

	loadQueue := func(ep arrEndpoint) error {
		if _, done := byKind[ep.kind]; done {
			return nil
		}
		resp, err := a.do(ctx, ep, http.MethodGet, "/api/v3/queue?pageSize=1000", nil)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		var page arrQueuePage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return err
		}
		idx := make(map[int]arrQueueRecord, len(page.Records))
		for _, rec := range page.Records {
			id := rec.MovieID
			if ep.kind == "series" {
				id = rec.SeriesID
			}
			if id != 0 {
				idx[id] = rec
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
		rec, queued := byKind[ep.kind][arrID]
		if arrID == 0 || !queued {
			out = append(out, QueueItem{Key: key}) // not in the queue → not grabbed
			continue
		}
		out = append(out, QueueItem{
			Key:      key,
			Grabbed:  true,
			Progress: progressFraction(rec.Size, rec.SizeLeft),
			ETAText:  rec.TimeLeft,
			Status:   rec.Status,
		})
	}
	return out, nil
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
