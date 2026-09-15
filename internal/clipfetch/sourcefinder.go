package clipfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

const maxSourceSuggestions = 8
const maxSourcePreviewItems = 3
const maxSourceFinderJSONBytes = 2 << 20

// ArchiveSourceFinder hides Archive's search grammar and canonicalisation behind one read-only
// boundary. It never receives a file sink, so suggesting or resolving cannot download media.
type ArchiveSourceFinder struct {
	client   *archiveClient
	mu       sync.Mutex
	cache    map[string]sourceFinderCacheEntry
	inflight map[string]*sourceFinderCall
	now      func() time.Time
	ttl      time.Duration
	maxCache int
}

type sourceFinderCacheEntry struct {
	results []filler.SourceSuggestion
	usedAt  time.Time
}

type sourceFinderCall struct {
	done    chan struct{}
	results []filler.SourceSuggestion
	err     error
}

func NewArchiveSourceFinder() *ArchiveSourceFinder {
	return newArchiveSourceFinder("https://archive.org", &http.Client{Timeout: 8 * time.Second})
}

func newArchiveSourceFinder(base string, httpc *http.Client) *ArchiveSourceFinder {
	client := newArchiveClient(base, httpc, nil)
	client.maxJSONBytes = maxSourceFinderJSONBytes
	client.userAgent = "Loomarr filler-source-finder"
	return &ArchiveSourceFinder{
		client: client, cache: make(map[string]sourceFinderCacheEntry),
		inflight: make(map[string]*sourceFinderCall), now: time.Now, ttl: 2 * time.Minute, maxCache: 32,
	}
}

type collectionSearchResp struct {
	Response struct {
		Docs []struct {
			Identifier  string      `json:"identifier"`
			Title       string      `json:"title"`
			Description archiveText `json:"description"`
			ItemCount   int         `json:"item_count"`
		} `json:"docs"`
	} `json:"response"`
}

type archiveText string

func (text *archiveText) UnmarshalJSON(data []byte) error {
	var scalar string
	if err := json.Unmarshal(data, &scalar); err == nil {
		*text = archiveText(scalar)
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*text = archiveText(strings.Join(values, " "))
	return nil
}

func (f *ArchiveSourceFinder) Suggest(ctx context.Context, query string, limit int) ([]filler.SourceSuggestion, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > 120 {
		return nil, fmt.Errorf("%w: archive collection search must be between 2 and 120 characters", filler.ErrInvalidSourceReference)
	}
	normalized := strings.Join(strings.Fields(query), " ")
	return f.cached(ctx, "suggest:"+strings.ToLower(normalized)+":"+strconv.Itoa(limit), func() ([]filler.SourceSuggestion, error) {
		return f.suggestUncached(ctx, normalized, limit)
	})
}

func (f *ArchiveSourceFinder) suggestUncached(ctx context.Context, query string, limit int) ([]filler.SourceSuggestion, error) {
	if strings.Contains(query, "/") {
		resolved, err := f.resolveUncached(ctx, query)
		if err != nil {
			return nil, err
		}
		return []filler.SourceSuggestion{resolved}, nil
	}
	terms := archiveSearchTerms(query)
	if terms == "" {
		return nil, fmt.Errorf("%w: archive collection search contains no searchable text", filler.ErrInvalidSourceReference)
	}
	if limit <= 0 || limit > maxSourceSuggestions {
		limit = maxSourceSuggestions
	}

	values := url.Values{}
	values.Set("q", "mediatype:collection AND (title:("+terms+") OR identifier:("+terms+"))")
	values["fl[]"] = []string{"identifier", "title", "description", "item_count"}
	values.Set("rows", strconv.Itoa(limit))
	values.Set("output", "json")

	var response collectionSearchResp
	if err := f.client.getJSON(ctx, f.client.base+"/advancedsearch.php?"+values.Encode(), &response); err != nil {
		return nil, fmt.Errorf("%w: archive collection search %q: %v", filler.ErrSourceProvider, query, err)
	}
	results := make([]filler.SourceSuggestion, 0, len(response.Response.Docs))
	for _, doc := range response.Response.Docs {
		id := strings.TrimSpace(doc.Identifier)
		if id == "" {
			continue
		}
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = id
		}
		results = append(results, filler.SourceSuggestion{
			Provider: "archive", TargetType: "collection", CanonicalID: id,
			CanonicalURL: "https://archive.org/details/" + url.PathEscape(id),
			Title:        title, Description: strings.TrimSpace(string(doc.Description)), ItemCount: doc.ItemCount,
		})
	}
	return results, nil
}

func (f *ArchiveSourceFinder) Resolve(ctx context.Context, input string) (filler.SourceSuggestion, error) {
	input = strings.TrimSpace(input)
	results, err := f.cached(ctx, "resolve:"+strings.ToLower(input), func() ([]filler.SourceSuggestion, error) {
		resolved, resolveErr := f.resolveUncached(ctx, input)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return []filler.SourceSuggestion{resolved}, nil
	})
	if err != nil {
		return filler.SourceSuggestion{}, err
	}
	return results[0], nil
}

func (f *ArchiveSourceFinder) resolveUncached(ctx context.Context, input string) (filler.SourceSuggestion, error) {
	id := archiveCollectionID(input)
	if id == "" {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: enter an Archive.org identifier or details URL", filler.ErrInvalidSourceReference)
	}
	metadata, err := f.client.metadata(ctx, id)
	if err != nil {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: %v", filler.ErrSourceProvider, err)
	}
	if metadata.Metadata.MediaType != "collection" {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: archive item %q is %q, not a collection", filler.ErrInvalidSourceReference, id, metadata.Metadata.MediaType)
	}
	title := strings.TrimSpace(metadata.Metadata.Title)
	if title == "" {
		title = id
	}
	previewItems, itemCount, _ := f.collectionPreview(ctx, id)
	return filler.SourceSuggestion{
		Provider: "archive", TargetType: "collection", CanonicalID: id,
		CanonicalURL: "https://archive.org/details/" + url.PathEscape(id),
		Title:        title, Description: strings.TrimSpace(metadata.Metadata.Description),
		ItemCount: itemCount, PreviewItems: previewItems,
	}, nil
}

// collectionPreview asks Archive for a small, recognizable sample rather than trusting its
// unspecified default row order. The filter mirrors the video formats the downloader can consume,
// while popularity is deliberately preview-only and does not alter acquisition ordering.
// It is advisory: callers deliberately keep a valid collection resolvable when examples are
// temporarily unavailable.
func (f *ArchiveSourceFinder) collectionPreview(ctx context.Context, id string) ([]filler.SourcePreviewItem, int, error) {
	values := url.Values{}
	escapedID := strings.ReplaceAll(id, `"`, `\"`)
	values.Set("q", `collection:"`+escapedID+`" AND mediatype:movies AND (`+
		`format:"MPEG4" OR format:"h.264 IA" OR format:"512Kb MPEG4")`)
	values["fl[]"] = []string{"identifier", "title"}
	values["sort[]"] = []string{"downloads desc"}
	values.Set("rows", strconv.Itoa(maxSourcePreviewItems))
	values.Set("output", "json")

	var response searchResp
	if err := f.client.getJSON(ctx, f.client.base+"/advancedsearch.php?"+values.Encode(), &response); err != nil {
		return nil, 0, err
	}
	items := make([]filler.SourcePreviewItem, 0, min(len(response.Response.Docs), maxSourcePreviewItems))
	for _, doc := range response.Response.Docs {
		id := strings.TrimSpace(doc.Identifier)
		if id == "" {
			continue
		}
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = id
		}
		items = append(items, filler.SourcePreviewItem{
			Title: title,
			URL:   "https://archive.org/details/" + url.PathEscape(id),
		})
		if len(items) == maxSourcePreviewItems {
			break
		}
	}
	return items, response.Response.NumFound, nil
}

func archiveSearchTerms(query string) string {
	query = sanitizeQuery(query)
	if query == "" {
		return ""
	}
	terms := strings.Fields(query)
	for i, term := range terms {
		terms[i] = `"` + strings.ReplaceAll(term, `"`, `\"`) + `"`
	}
	return strings.Join(terms, " ")
}

func (f *ArchiveSourceFinder) cached(ctx context.Context, key string, load func() ([]filler.SourceSuggestion, error)) ([]filler.SourceSuggestion, error) {
	now := f.now()
	f.mu.Lock()
	if entry, ok := f.cache[key]; ok && now.Sub(entry.usedAt) < f.ttl {
		entry.usedAt = now
		f.cache[key] = entry
		results := append([]filler.SourceSuggestion(nil), entry.results...)
		f.mu.Unlock()
		return results, nil
	}
	if call, ok := f.inflight[key]; ok {
		done := call.done
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return append([]filler.SourceSuggestion(nil), call.results...), call.err
		}
	}
	call := &sourceFinderCall{done: make(chan struct{})}
	f.inflight[key] = call
	f.mu.Unlock()

	results, err := load()
	f.mu.Lock()
	call.results, call.err = append([]filler.SourceSuggestion(nil), results...), err
	if err == nil {
		f.cache[key] = sourceFinderCacheEntry{results: append([]filler.SourceSuggestion(nil), results...), usedAt: now}
		f.pruneCache()
	}
	delete(f.inflight, key)
	close(call.done)
	f.mu.Unlock()
	return results, err
}

func (f *ArchiveSourceFinder) pruneCache() {
	for len(f.cache) > f.maxCache {
		var oldestKey string
		var oldest time.Time
		for key, entry := range f.cache {
			if oldestKey == "" || entry.usedAt.Before(oldest) {
				oldestKey, oldest = key, entry.usedAt
			}
		}
		delete(f.cache, oldestKey)
	}
}

// archiveCollectionID accepts the spellings a person reasonably pastes while refusing foreign
// hosts and ambiguous Archive pages before they become an external request.
func archiveCollectionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "/") {
		if strings.ContainsAny(raw, ":. ?#\t") {
			return ""
		}
		return raw
	}
	if strings.HasPrefix(raw, "/details/") || strings.HasPrefix(raw, "/metadata/") {
		raw = "https://archive.org" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "archive.org") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || (parts[0] != "details" && parts[0] != "metadata") {
		return ""
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil || id == "" || strings.ContainsAny(id, "/:. ?#\t") {
		return ""
	}
	return id
}
