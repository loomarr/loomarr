// Package youtube is the YouTube source provider: a channel or playlist the operator registers,
// listed newest-first through yt-dlp's flat-playlist mode.
//
// The provider owns what is particular to YouTube. A listing is one bounded newest-first sweep that
// a cursor spreads over several checks; its flat metadata can honestly say whether an item is live,
// private, unavailable or outside the clip envelope before anything is downloaded; and a campaign
// published in several duration cuts is offered as one, the fullest. Filler owns the budget, the
// lease, the dedupe against what is already known, and the selection.
package youtube

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/sourceprovider"
)

// ID is the provider's persisted identity.
const ID sourceprovider.ID = "youtube"

// InitialLookback bounds one newest-first sweep. A cursor can spread that sweep over several
// checks, but registration never means walking an unbounded channel history.
const InitialLookback = 100

// Enumerator lists what a playlist or channel offers, without downloading media, returning the
// items and the target's reported size.
type Enumerator func(ctx context.Context, target string, limit int) ([]sourceprovider.Item, int, error)

// Provider lists YouTube channels and playlists.
type Provider struct {
	enumerate Enumerator
	finder    sourceprovider.Finder
}

// New builds the provider over an Enumerator. A nil Enumerator makes every listing fail, as an
// installation without yt-dlp should.
func New(enumerate Enumerator) *Provider {
	return &Provider{enumerate: enumerate}
}

// YtDlp enumerates through yt-dlp's flat-playlist mode.
func YtDlp(e *clipfetch.YouTubeEnumerator) Enumerator {
	return func(ctx context.Context, target string, limit int) ([]sourceprovider.Item, int, error) {
		items, total, err := e.Enumerate(ctx, target, limit)
		if err != nil {
			return nil, 0, err
		}
		out := make([]sourceprovider.Item, len(items))
		for i, item := range items {
			out[i] = sourceprovider.Item{
				ID: item.ID, URL: item.URL, Title: item.Title, License: item.License,
				ObservedYear: item.ReleaseYear, PublishedAt: item.PublishedAt,
				DurationMS: item.DurationMS, DurationKnown: item.DurationKnown, Height: item.Height,
				Availability: item.Availability, LiveStatus: item.LiveStatus,
			}
		}
		return out, total, nil
	}
}

// ID implements sourceprovider.Provider.
func (*Provider) ID() sourceprovider.ID { return ID }

// Describe implements sourceprovider.Provider.
func (*Provider) Describe() sourceprovider.Description {
	return sourceprovider.Description{
		Label: "YouTube", AddressHint: "https://www.youtube.com/@channel/videos", Fetched: true,
		GroupDetail:  "playlists you've added — titles and descriptions are kept for tagging",
		SourceDetail: "New clips are checked before they can play.",
		SearchDown:   "YouTube search isn’t available right now. Paste a channel or playlist URL instead.",
		ResolveDown:  "YouTube couldn’t check that source right now. Try again in a moment.",
		BadAddress: sourceprovider.Problem{
			Title:  "That doesn't look like a YouTube playlist or channel",
			Detail: "Paste a playlist URL (youtube.com/playlist?list=…) or a channel URL.",
		},
		Blurb:          "Channels and playlists you choose from YouTube.",
		SourceNoun:     "YouTube channel or playlist",
		PreviewHeading: "A few videos from this source",
		Find: sourceprovider.FindCopy{
			Label:       "Find a YouTube channel or playlist",
			Placeholder: "Search channels or paste a channel or playlist URL",
			Add:         "Add source",
			NoMatches:   "No channels found. Paste a channel or playlist URL to check it.",
			MinQuery:    3,
		},
	}
}

var (
	// youtubeURL is any YouTube or youtu.be address.
	youtubeURL = regexp.MustCompile(`(?i)^(https?://)?((www|m|music)\.)?(youtube\.com|youtu\.be)/`)
	// youtubeHandle is an @handle.
	youtubeHandle = regexp.MustCompile(`(?i)^@[a-z0-9._-]+$`)
	// youtubeChannelID is a channel's UC… identifier.
	youtubeChannelID = regexp.MustCompile(`(?i)^UC[a-z0-9_-]{10,}$`)
	// youtubePlaylistID is a playlist's identifier, whichever prefix its kind carries.
	youtubePlaylistID = regexp.MustCompile(`(?i)^(PL|UU|LL|FL|RD|OLAK5uy_)[a-z0-9_-]{8,}$`)
	// youtubeVideoID is the v= parameter of a watch URL.
	youtubeVideoID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// Recognizes implements sourceprovider.Recognizer.
func (*Provider) Recognizes(input string) bool {
	input = strings.TrimSpace(input)
	return youtubeURL.MatchString(input) || youtubeHandle.MatchString(input) ||
		youtubeChannelID.MatchString(input) || youtubePlaylistID.MatchString(input)
}

// SourceURL implements sourceprovider.Linker: a registered source is stored as its URL.
func (*Provider) SourceURL(address string) string { return strings.TrimSpace(address) }

// EmbedURL implements sourceprovider.Linker: a watch URL plays in the privacy-enhanced embed.
func (*Provider) EmbedURL(itemURL string) string {
	u, err := url.Parse(itemURL)
	if err != nil || strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != "youtube.com" {
		return ""
	}
	id := u.Query().Get("v")
	if !youtubeVideoID.MatchString(id) {
		return ""
	}
	return "https://www.youtube-nocookie.com/embed/" + url.PathEscape(id) + "?autoplay=1&playsinline=1"
}

// WithFinder gives the provider the means to suggest channels and verify an address. Without one,
// as on an installation without yt-dlp, both fail with filler.ErrSourceProvider.
func (p *Provider) WithFinder(finder sourceprovider.Finder) *Provider {
	p.finder = finder
	return p
}

// Suggest implements sourceprovider.Finder.
func (p *Provider) Suggest(ctx context.Context, query string, limit int) ([]sourceprovider.Suggestion, error) {
	if p.finder == nil {
		return nil, fmt.Errorf("%w: yt-dlp is unavailable", filler.ErrSourceProvider)
	}
	return p.finder.Suggest(ctx, query, limit)
}

// Resolve implements sourceprovider.Finder.
func (p *Provider) Resolve(ctx context.Context, input string) (sourceprovider.Suggestion, error) {
	if p.finder == nil {
		return sourceprovider.Suggestion{}, fmt.Errorf("%w: yt-dlp is unavailable", filler.ErrSourceProvider)
	}
	return p.finder.Resolve(ctx, input)
}

// Address implements sourceprovider.Addresser: it validates a playlist or channel URL for
// registration and returns the canonical URL to store, or "" if it is not one.
//
// It is deliberately stricter than clipfetch.KindForURL, which defaults an unknown host to YouTube
// because yt-dlp handles the widest set of sites. That default is right for an ingest, where an
// admin pastes one URL deliberately and watches it work. It is wrong here: a registered source
// persists and is fetched unattended, so a typo'd host would become a permanent row that can never
// fetch, with the failure surfacing much later and far from the mistake.
//
// It accepts the shapes an operator actually pastes — a playlist, a channel, a @handle, a /c/ or
// /user/ URL — on youtube.com (with or without a subdomain) or youtu.be.
func (*Provider) Address(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	// Scheme is optional in what people paste; normalise so the host check is uniform.
	probe := v
	if !strings.Contains(probe, "://") {
		probe = "https://" + probe
	}
	u, err := url.Parse(probe)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	// Exact host match, never strings.Contains: "youtube.com.evil.test" contains the string and is
	// a different site. A substring check would register an attacker-chosen host as a trusted
	// source.
	if host != "youtube.com" && host != "m.youtube.com" && host != "music.youtube.com" && host != "youtu.be" {
		return ""
	}
	// A bare host with no path identifies nothing to fetch.
	if strings.Trim(u.Path, "/") == "" && u.RawQuery == "" {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}

// List reads the source's newest-first sweep from just past the checkpoint's cursor and offers up
// to req.Take of what the sweep has not yet covered.
func (p *Provider) List(ctx context.Context, src sourceprovider.Source, req sourceprovider.ListRequest) (sourceprovider.Listing, error) {
	if p.enumerate == nil {
		return sourceprovider.Listing{}, fmt.Errorf("youtube enumerator is unavailable")
	}
	items, _, err := p.enumerate(ctx, src.URI, InitialLookback)
	if err != nil {
		return sourceprovider.Listing{}, err
	}
	return read(src.Checkpoint, items, req), nil
}

// read walks one listing for one check.
func read(checkpoint sourceprovider.Checkpoint, items []sourceprovider.Item, req sourceprovider.ListRequest) sourceprovider.Listing {
	out := sourceprovider.Listing{Outcomes: sourceprovider.Outcomes{}}
	preferred := preferredCuts(items, req)
	consider, checkpoint, replaceOutcomes := window(checkpoint, items)
	out.ReplaceOutcomes = replaceOutcomes

	lastExamined := ""
	reachedCap := false
	for i, item := range consider {
		if checkpoint.Watermark != "" && item.ID == checkpoint.Watermark {
			break
		}
		if item.ID != "" {
			lastExamined = item.ID
		}
		if outcome, rejected := rejection(item, req); rejected {
			out.Outcomes[outcome]++
			continue
		}
		if campaign := campaignKey(item.Title); campaign != "" {
			if cut := preferred[campaign]; cut.ID != item.ID {
				out.Outcomes[sourceprovider.OutcomeAlreadyKnown]++
				continue
			}
		}
		if req.Known != nil && req.Known(item) {
			out.Outcomes[sourceprovider.OutcomeAlreadyKnown]++
			continue
		}
		out.Items = append(out.Items, item)
		if len(out.Items) == req.Take {
			reachedCap = i < len(consider)-1
			break
		}
	}
	if reachedCap {
		checkpoint.Cursor = lastExamined
	} else {
		if checkpoint.PendingWatermark != "" {
			checkpoint.Watermark = checkpoint.PendingWatermark
		}
		checkpoint.Cursor = ""
		checkpoint.PendingWatermark = ""
	}
	out.Checkpoint = checkpoint
	return out
}

// window is the part of a listing a check reads: from just past the checkpoint's cursor, or all of
// it when a sweep starts or its cursor item is gone. It returns the checkpoint with the sweep's
// pending watermark recorded, and whether the check begins a new sweep.
func window(checkpoint sourceprovider.Checkpoint, items []sourceprovider.Item) ([]sourceprovider.Item, sourceprovider.Checkpoint, bool) {
	start := 0
	replaceOutcomes := checkpoint.Cursor == ""
	if checkpoint.Cursor != "" {
		if i := slices.IndexFunc(items, func(item sourceprovider.Item) bool { return item.ID == checkpoint.Cursor }); i >= 0 {
			start = i + 1
		} else {
			// Provider lists can shift or remove an item. Restart the bounded sweep and rely
			// on exact durable identity dedupe; guessing an offset could skip unseen work.
			checkpoint.Cursor = ""
			checkpoint.PendingWatermark = ""
			replaceOutcomes = true
		}
	}
	if start < len(items) && checkpoint.PendingWatermark == "" {
		checkpoint.PendingWatermark = items[0].ID
	}
	return items[start:], checkpoint, replaceOutcomes
}

// rejection is why an item is not offered: the caller's shared rejection, then what the flat
// listing's own metadata rules out.
func rejection(item sourceprovider.Item, req sourceprovider.ListRequest) (sourceprovider.Outcome, bool) {
	if req.Reject != nil {
		if outcome, rejected := req.Reject(item); rejected {
			return outcome, true
		}
	}
	if item.ID == "" || item.URL == "" {
		return sourceprovider.OutcomeMetadataIncomplete, true
	}
	switch item.Availability {
	case "private":
		return sourceprovider.OutcomePrivate, true
	case "needs_auth", "premium_only", "subscriber_only", "unavailable":
		return sourceprovider.OutcomeUnavailable, true
	}
	switch item.LiveStatus {
	case "is_live", "post_live":
		return sourceprovider.OutcomeLive, true
	case "is_upcoming":
		return sourceprovider.OutcomeUpcoming, true
	}
	if !item.DurationKnown {
		return sourceprovider.OutcomeMetadataIncomplete, true
	}
	duration := time.Duration(item.DurationMS) * time.Millisecond
	if duration < req.MinDuration {
		return sourceprovider.OutcomeTooShort, true
	}
	if req.MaxDuration > 0 && duration > req.MaxDuration {
		return sourceprovider.OutcomeTooLong, true
	}
	return "", false
}

// preferredCuts is, for each campaign in a listing, the duration cut to take. The cursor can split
// one listing across several checks. Choosing the best explicit duration variant from the whole
// bounded listing before applying that cursor keeps a :15 cut in one check and a :30 cut in the
// next from both reaching the catalog.
func preferredCuts(items []sourceprovider.Item, req sourceprovider.ListRequest) map[string]sourceprovider.Item {
	preferred := map[string]sourceprovider.Item{}
	for _, item := range items {
		if _, rejected := rejection(item, req); rejected {
			continue
		}
		campaign := campaignKey(item.Title)
		if campaign == "" {
			continue
		}
		current, found := preferred[campaign]
		if !found || variantBetter(item, current) {
			preferred[campaign] = item
		}
	}
	return preferred
}

func variantBetter(candidate, current sourceprovider.Item) bool {
	if candidate.DurationMS != current.DurationMS {
		return candidate.DurationMS > current.DurationMS
	}
	if candidate.Height != current.Height {
		return candidate.Height > current.Height
	}
	return strings.TrimSpace(candidate.ID) < strings.TrimSpace(current.ID)
}

// campaignKey is a title with its duration cut removed ("Acme :15 sec. spot" and "Acme :30 sec.
// spot" share one), or "" when the title names no cut.
func campaignKey(title string) string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(title)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(fields) == 0 {
		return ""
	}
	durationAt := -1
	for index, field := range fields {
		if !durationToken(field) {
			continue
		}
		beforeTime := index > 0 && timeWord(fields[index-1])
		afterTime := index+1 < len(fields) && timeWord(fields[index+1])
		beforeLanguage := index > 0 && languageWord(fields[index-1]) && index == len(fields)-1
		afterLanguage := index+1 < len(fields) && languageWord(fields[index+1]) && index+2 == len(fields)
		if beforeTime || afterTime || beforeLanguage || afterLanguage {
			durationAt = index
			break
		}
	}
	if durationAt < 0 {
		return ""
	}
	key := make([]string, 0, len(fields)-1)
	for index, field := range fields {
		if index == durationAt || timeWord(field) || field == "spot" {
			continue
		}
		key = append(key, field)
	}
	return strings.Join(key, " ")
}

func durationToken(value string) bool {
	switch value {
	case "5", "10", "15", "20", "30", "45", "60", "90", "120":
		return true
	default:
		return false
	}
}

func timeWord(value string) bool {
	switch value {
	case "s", "sec", "secs", "second", "seconds":
		return true
	default:
		return false
	}
}

func languageWord(value string) bool {
	switch value {
	case "english", "spanish", "french", "german", "italian", "portuguese":
		return true
	default:
		return false
	}
}
