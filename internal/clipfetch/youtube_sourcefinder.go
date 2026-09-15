package clipfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/proctree"
)

const youtubeSourceFinderTimeout = 8 * time.Second
const maxYouTubeSourceSuggestions = 6
const maxYouTubeSourceSearchHits = 12

// YouTubeSourceFinder turns bounded yt-dlp video-search results into unique channel
// suggestions and resolves exact channel or playlist URLs. It has no file sink and always
// passes --skip-download, so this boundary can identify a source but cannot acquire media.
type YouTubeSourceFinder struct {
	ytDlpPath  string
	mu         sync.Mutex
	cache      map[string]sourceFinderCacheEntry
	inflight   map[string]*sourceFinderCall
	now        func() time.Time
	ttl        time.Duration
	maxCache   int
	searchGate chan struct{}
}

func NewYouTubeSourceFinder(ytDlpPath string) *YouTubeSourceFinder {
	return &YouTubeSourceFinder{
		ytDlpPath: ytDlpPath,
		cache:     make(map[string]sourceFinderCacheEntry), inflight: make(map[string]*sourceFinderCall),
		now: time.Now, ttl: 10 * time.Minute, maxCache: 100, searchGate: make(chan struct{}, 1),
	}
}

type ytDlpSourceMetadata struct {
	ID            string                `json:"id"`
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Duration      float64               `json:"duration"`
	WebpageURL    string                `json:"webpage_url"`
	OriginalURL   string                `json:"original_url"`
	Channel       string                `json:"channel"`
	ChannelID     string                `json:"channel_id"`
	ChannelURL    string                `json:"channel_url"`
	Uploader      string                `json:"uploader"`
	UploaderID    string                `json:"uploader_id"`
	UploaderURL   string                `json:"uploader_url"`
	PlaylistCount int                   `json:"playlist_count"`
	Entries       []ytDlpSourceMetadata `json:"entries"`
}

type ytDlpSourceListing struct {
	Entries []ytDlpSourceMetadata `json:"entries"`
}

func (f *YouTubeSourceFinder) Suggest(ctx context.Context, query string, limit int) ([]filler.SourceSuggestion, error) {
	query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
	if len(query) < 3 || len(query) > 120 {
		return nil, fmt.Errorf("%w: YouTube channel search must be between 3 and 120 characters", filler.ErrInvalidSourceReference)
	}
	if _, _, ok := youtubeSourceInput(query); ok {
		resolved, err := f.Resolve(ctx, query)
		if err != nil {
			return nil, err
		}
		return []filler.SourceSuggestion{resolved}, nil
	}
	if limit <= 0 || limit > maxYouTubeSourceSuggestions {
		limit = maxYouTubeSourceSuggestions
	}
	return f.cached(ctx, "suggest:"+strings.ToLower(query)+":"+strconv.Itoa(limit), func() ([]filler.SourceSuggestion, error) {
		return f.suggestUncached(ctx, query, limit)
	})
}

func (f *YouTubeSourceFinder) suggestUncached(ctx context.Context, query string, limit int) ([]filler.SourceSuggestion, error) {
	select {
	case f.searchGate <- struct{}{}:
		defer func() { <-f.searchGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// ytsearch finds videos, so inspect a few bounded matches per requested result and collapse
	// them by channel. This remains one listing-only process and never follows recommendations.
	searchLimit := min(limit*2, maxYouTubeSourceSearchHits)
	data, err := f.run(ctx,
		"--no-config", "--flat-playlist", "--skip-download", "--dump-single-json", "--no-warnings",
		"--playlist-end", strconv.Itoa(searchLimit), fmt.Sprintf("ytsearch%d:%s", searchLimit, query),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: YouTube channel search %q: %w", filler.ErrSourceProvider, query, err)
	}
	var listing ytDlpSourceListing
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, fmt.Errorf("%w: decode YouTube search results: %v", filler.ErrSourceProvider, err)
	}
	results := make([]filler.SourceSuggestion, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, item := range listing.Entries {
		id, canonicalURL := youtubeChannelIdentity(item)
		if id == "" || canonicalURL == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		title := strings.TrimSpace(item.Channel)
		if title == "" {
			title = strings.TrimSpace(item.Uploader)
		}
		if title == "" {
			title = id
		}
		description := strings.TrimSpace(item.Title)
		if description != "" {
			description = "Found from “" + description + "”"
		}
		results = append(results, filler.SourceSuggestion{
			Provider: "youtube", TargetType: "channel", CanonicalID: id,
			CanonicalURL: canonicalURL, Title: title, Description: description,
		})
		if len(results) == limit {
			break
		}
	}
	if len(listing.Entries) > 0 && len(results) == 0 {
		return nil, fmt.Errorf("%w: YouTube search results carried no channel identity", filler.ErrSourceProvider)
	}
	return results, nil
}

func (f *YouTubeSourceFinder) Resolve(ctx context.Context, input string) (filler.SourceSuggestion, error) {
	normalized, inputType, ok := youtubeSourceInput(input)
	if !ok {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: enter a YouTube channel, playlist, or video URL", filler.ErrInvalidSourceReference)
	}
	results, err := f.cached(ctx, "resolve:"+strings.ToLower(normalized), func() ([]filler.SourceSuggestion, error) {
		resolved, resolveErr := f.resolveUncached(ctx, normalized, inputType)
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

func (f *YouTubeSourceFinder) resolveUncached(ctx context.Context, input, inputType string) (filler.SourceSuggestion, error) {
	data, err := f.run(ctx,
		"--no-config", "--flat-playlist", "--skip-download", "--dump-single-json", "--no-warnings",
		"--playlist-end", strconv.Itoa(maxSourcePreviewItems), input,
	)
	if err != nil {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: check YouTube source: %w", filler.ErrSourceProvider, err)
	}
	var metadata ytDlpSourceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: decode YouTube source: %v", filler.ErrSourceProvider, err)
	}
	if inputType == "channel" && metadata.ChannelID == "" && strings.HasPrefix(strings.TrimSpace(metadata.ID), "UC") {
		metadata.ChannelID = strings.TrimSpace(metadata.ID)
	}
	if inputType == "playlist" {
		parsed, _ := url.Parse(input)
		id := strings.TrimSpace(parsed.Query().Get("list"))
		if id == "" {
			id = strings.TrimSpace(metadata.ID)
		}
		if id == "" {
			return filler.SourceSuggestion{}, fmt.Errorf("%w: that YouTube URL did not resolve to a playlist", filler.ErrInvalidSourceReference)
		}
		title := strings.TrimSpace(metadata.Title)
		if title == "" {
			title = id
		}
		return filler.SourceSuggestion{
			Provider: "youtube", TargetType: "playlist", CanonicalID: id,
			CanonicalURL: "https://www.youtube.com/playlist?list=" + url.QueryEscape(id),
			Title:        title, Description: strings.TrimSpace(metadata.Description), ItemCount: metadata.PlaylistCount,
			PreviewItems: youtubePreviewItems(metadata.Entries),
		}, nil
	}

	id, canonicalURL := youtubeChannelIdentity(metadata)
	if id == "" || canonicalURL == "" {
		return filler.SourceSuggestion{}, fmt.Errorf("%w: that YouTube URL did not resolve to a channel", filler.ErrInvalidSourceReference)
	}
	title := strings.TrimSpace(metadata.Channel)
	if title == "" {
		title = strings.TrimSpace(metadata.Uploader)
	}
	if title == "" && inputType == "channel" {
		title = strings.TrimSpace(metadata.Title)
	}
	if title == "" {
		title = id
	}
	return filler.SourceSuggestion{
		Provider: "youtube", TargetType: "channel", CanonicalID: id,
		CanonicalURL: canonicalURL, Title: title, Description: strings.TrimSpace(metadata.Description),
		ItemCount: metadata.PlaylistCount, PreviewItems: youtubePreviewItems(metadata.Entries),
	}, nil
}

func youtubePreviewItems(entries []ytDlpSourceMetadata) []filler.SourcePreviewItem {
	items := make([]filler.SourcePreviewItem, 0, min(len(entries), maxSourcePreviewItems))
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if !validYouTubeSlug(id) {
			continue
		}
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			title = id
		}
		durationMS := int64(0)
		if entry.Duration > 0 {
			durationMS = int64(entry.Duration * 1000)
		}
		items = append(items, filler.SourcePreviewItem{
			Title: title, URL: "https://www.youtube.com/watch?v=" + url.QueryEscape(id), DurationMS: durationMS,
		})
		if len(items) == maxSourcePreviewItems {
			break
		}
	}
	return items
}

func youtubeChannelIdentity(item ytDlpSourceMetadata) (string, string) {
	id := strings.TrimSpace(item.ChannelID)
	if id == "" {
		id = strings.TrimSpace(item.UploaderID)
	}
	if id != "" && strings.HasPrefix(id, "UC") {
		return id, "https://www.youtube.com/channel/" + url.PathEscape(id) + "/videos"
	}
	for _, candidate := range []string{item.ChannelURL, item.UploaderURL, item.WebpageURL, item.OriginalURL} {
		normalized, kind, ok := youtubeSourceInput(candidate)
		if !ok || kind != "channel" {
			continue
		}
		parsed, _ := url.Parse(normalized)
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && parts[0] == "channel" {
			return parts[1], "https://www.youtube.com/channel/" + url.PathEscape(parts[1]) + "/videos"
		}
		if id == "" {
			id = normalized
		}
		return id, normalized
	}
	return "", ""
}

// youtubeSourceInput accepts only a YouTube target from which Loomarr can resolve a channel or
// playlist. A video URL is accepted as a convenient way to identify its channel; it never becomes
// a single-video registered source.
func youtubeSourceInput(raw string) (normalized, targetType string, ok bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", false
	}
	if strings.HasPrefix(value, "@") && validYouTubeSlug(value[1:]) {
		return "https://www.youtube.com/" + value + "/videos", "channel", true
	}
	if strings.HasPrefix(value, "UC") && len(value) >= 12 && validYouTubeSlug(value) {
		return "https://www.youtube.com/channel/" + value + "/videos", "channel", true
	}
	if youtubePlaylistID(value) {
		return "https://www.youtube.com/playlist?list=" + url.QueryEscape(value), "playlist", true
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host != "youtube.com" && host != "m.youtube.com" && host != "music.youtube.com" && host != "youtu.be" {
		return "", "", false
	}
	parsed.Scheme = "https"
	parsed.Host = "www.youtube.com"
	parsed.Fragment = ""
	if listID := strings.TrimSpace(parsed.Query().Get("list")); listID != "" {
		return "https://www.youtube.com/playlist?list=" + url.QueryEscape(listID), "playlist", true
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 1 && strings.HasPrefix(parts[0], "@") && len(parts[0]) > 1 {
		return "https://www.youtube.com/" + parts[0] + "/videos", "channel", true
	}
	if len(parts) >= 2 && (parts[0] == "channel" || parts[0] == "c" || parts[0] == "user") && parts[1] != "" {
		return "https://www.youtube.com/" + parts[0] + "/" + url.PathEscape(parts[1]) + "/videos", "channel", true
	}
	return "", "", false
}

func validYouTubeSlug(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func youtubePlaylistID(value string) bool {
	if len(value) < 10 || !validYouTubeSlug(value) {
		return false
	}
	for _, prefix := range []string{"PL", "UU", "LL", "FL", "RD", "OLAK5uy_"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

type boundedSourceFinderOutput struct {
	data     bytes.Buffer
	limit    int
	overflow bool
}

func (w *boundedSourceFinderOutput) Write(p []byte) (int, error) {
	written := len(p)
	remaining := w.limit - w.data.Len()
	if remaining > 0 {
		_, _ = w.data.Write(p[:min(remaining, len(p))])
	}
	if len(p) > remaining {
		w.overflow = true
	}
	return written, nil
}

func (f *YouTubeSourceFinder) run(ctx context.Context, args ...string) ([]byte, error) {
	if f.ytDlpPath == "" {
		return nil, errors.New("yt-dlp is unavailable")
	}
	runCtx, cancel := context.WithTimeout(ctx, youtubeSourceFinderTimeout)
	defer cancel()
	cmd := exec.Command(f.ytDlpPath, args...) //nolint:gosec // resolved configured tool; arguments are separate and bounded
	stdout := boundedSourceFinderOutput{limit: maxSourceFinderJSONBytes}
	stderr := diagnosticTail{limit: ytDlpDiagnosticLimit}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	supervisor, err := proctree.Start(runCtx, cmd)
	if err != nil {
		return nil, fmt.Errorf("start yt-dlp: %w", err)
	}
	err = supervisor.Wait()
	if supervisor.Stopped() && runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("yt-dlp exited: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.overflow {
		return nil, fmt.Errorf("yt-dlp JSON exceeded %d bytes", maxSourceFinderJSONBytes)
	}
	return append([]byte(nil), stdout.data.Bytes()...), nil
}

func (f *YouTubeSourceFinder) cached(ctx context.Context, key string, load func() ([]filler.SourceSuggestion, error)) ([]filler.SourceSuggestion, error) {
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
		for len(f.cache) > f.maxCache {
			var oldestKey string
			var oldest time.Time
			for candidate, entry := range f.cache {
				if oldestKey == "" || entry.usedAt.Before(oldest) {
					oldestKey, oldest = candidate, entry.usedAt
				}
			}
			delete(f.cache, oldestKey)
		}
	}
	delete(f.inflight, key)
	close(call.done)
	f.mu.Unlock()
	return results, err
}
