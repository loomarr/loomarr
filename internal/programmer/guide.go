package programmer

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Reads Tunarr's generated guide — the ONLY source of "what is airing right now".
// Loomarr owns the lineup (what should play); Tunarr owns playout (when it actually
// plays), so airtimes cannot be recomputed here without duplicating its scheduling
// math (§6 division of labor).
//
// The shape is pinned in internal/testkit/fixtures/tunarr/guide_channels_response.json
// and documented in GUIDE-FINDINGS.md. Two traps that capture records, both load-bearing:
//
//  1. Use the PLURAL /api/guide/channels. The singular /api/guide/channels/{id} returns a
//     different, leaner shape ({index, lineupItem, startTimeMs}) with NO title and no
//     stop time — building against it costs a second lookup just to name a program.
//  2. The plural endpoint is keyed by channel id, so a Channels LIST costs one Tunarr
//     call, not one per card.
//
// The vendored OpenAPI does not type this response (no guide component schemas), so the
// field names come from Tunarr's own zod schemas at the pinned tag, confirmed live.

// GuideEntry is one program Tunarr reports on a channel's timeline.
type GuideEntry struct {
	Title   string
	StartMs int64
	StopMs  int64
	// Kind is Tunarr's discriminator: content | flex | custom | redirect. `flex` is a
	// gap (commercial pod or dead-air padding), not a program — the UI renders it as
	// such rather than pretending something is on.
	Kind string
	// TMDBID is lifted from the program's identifiers when present, so a guide entry
	// joins to Loomarr's provisioning key (movie:tmdb:<id>) with no second lookup.
	TMDBID string
}

// IsGap reports whether this entry is filler/dead-air rather than a real program.
func (g GuideEntry) IsGap() bool { return g.Kind == "flex" }

type guideIdentifier struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type guideProgram struct {
	Type  string `json:"type"`
	Start int64  `json:"start"`
	Stop  int64  `json:"stop"`
	// Flex entries carry their own title at the top level; content entries carry it
	// on the nested program.
	Title   string `json:"title"`
	Program struct {
		Title       string            `json:"title"`
		Identifiers []guideIdentifier `json:"identifiers"`
	} `json:"program"`
}

type guideChannel struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Number   int            `json:"number"`
	Programs []guideProgram `json:"programs"`
}

// Guide returns each channel's programs overlapping [from, to), keyed by Tunarr channel
// id. An unprogrammed or unknown channel simply has no entry.
func (t *Tunarr) Guide(ctx context.Context, from, to time.Time) (map[string][]GuideEntry, error) {
	q := url.Values{}
	q.Set("dateFrom", from.UTC().Format(time.RFC3339))
	q.Set("dateTo", to.UTC().Format(time.RFC3339))

	var resp map[string]guideChannel
	status, snippet, err := t.doStatus(ctx, t.http, http.MethodGet, "/api/guide/channels?"+q.Encode(), nil, &resp)
	if err != nil {
		return nil, err
	}
	// A guide that has not been generated yet is not an error — the channel simply has
	// nothing to show, the same way an unprogrammed lineup reads as empty (finding 4).
	if status == http.StatusBadRequest || status == http.StatusNotFound {
		return map[string][]GuideEntry{}, nil
	}
	if status < 200 || status >= 300 {
		return nil, statusErr("get guide", status, snippet)
	}

	out := make(map[string][]GuideEntry, len(resp))
	for id, ch := range resp {
		entries := make([]GuideEntry, 0, len(ch.Programs))
		for _, p := range ch.Programs {
			entries = append(entries, GuideEntry{
				Title:   firstNonEmpty(p.Program.Title, p.Title),
				StartMs: p.Start,
				StopMs:  p.Stop,
				Kind:    p.Type,
				TMDBID:  identifier(p.Program.Identifiers, "tmdb"),
			})
		}
		out[id] = entries
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func identifier(ids []guideIdentifier, kind string) string {
	for _, i := range ids {
		if i.Type == kind {
			return i.ID
		}
	}
	return ""
}
