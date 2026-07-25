package playout

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// XMLTV listings (§9.1) — the second half of what internal playout serves.
//
// The M3U tuner tells a media server WHICH channels exist; this tells it WHAT IS ON. Without
// it a channel appears in Live TV, tunes, plays — and shows "no information" forever, which
// reads as a broken integration rather than a missing feature.
//
// Shape verified against Tunarr's own `/api/xmltv.xml` on the dev stack, because that is the
// output Emby already accepts. Two details from that capture are easy to get wrong and fail
// SILENTLY (an unparseable guide is an empty guide, not an error):
//
//   - Timestamps are `YYYYMMDDHHMMSS +0000` — XMLTV's own format, not RFC3339.
//   - `channel@id` must match the M3U's `tvg-id` EXACTLY. A mismatch is the single most common
//     Live TV wiring failure: every channel plays and every guide is empty.

// xmltvTime is XMLTV's timestamp format.
//
// Always emitted in UTC with an explicit `+0000` offset rather than the operator's local zone.
// A media server applies its own timezone for display, so sending local time invites a
// double-conversion that shifts the whole guide by hours — and the symptom (everything is on at
// the wrong time) does not look like a timezone bug.
const xmltvTime = "20060102150405 -0700"

// GuideChannel is one channel's identity in the guide.
type GuideChannel struct {
	// ID must equal the M3U entry's `tvg-id`. See the package comment.
	ID     string
	Name   string
	Number int
	// IconURL is optional; omitted when empty rather than emitted as an empty element, which
	// some parsers treat as a broken image rather than no image.
	IconURL string
}

// Guide is everything the XMLTV document needs: the channel list, and each channel's
// programmes keyed by channel id.
type Guide struct {
	Channels   []GuideChannel
	Programmes map[string][]Broadcast
}

// --- The wire types. Separate from the domain types above so the XML tags live in one place
// and `encoding/xml` does the escaping. Hand-building this string would be an injection waiting
// to happen: channel names and programme titles are operator- and library-supplied text. ---

type xmlTV struct {
	XMLName           xml.Name         `xml:"tv"`
	GeneratorInfoName string           `xml:"generator-info-name,attr"`
	Date              string           `xml:"date,attr"`
	Channels          []xmlTVChannel   `xml:"channel"`
	Programmes        []xmlTVProgramme `xml:"programme"`
}

type xmlTVChannel struct {
	ID string `xml:"id,attr"`
	// Multiple display-names, in Tunarr's order: "50 Classic Sci-Fi", "50", "Classic Sci-Fi".
	// Media servers pick differently between them, and offering all three is what makes the
	// channel show up with a sensible label everywhere rather than as a bare number in one
	// client and a name in another.
	DisplayNames []string   `xml:"display-name"`
	Icon         *xmlTVIcon `xml:"icon,omitempty"`
}

type xmlTVIcon struct {
	Src string `xml:"src,attr"`
}

type xmlTVProgramme struct {
	Start   string `xml:"start,attr"`
	Stop    string `xml:"stop,attr"`
	Channel string `xml:"channel,attr"`
	Title   string `xml:"title"`
	// EpisodeNum in xmltv_ns format when the slot carries season/episode numbers. Zero-based
	// and dot-separated ("0.4." is S1E5), which is the format's own convention — a media server
	// that understands it renders "S1E5" from this.
	EpisodeNum *xmlTVEpisodeNum `xml:"episode-num,omitempty"`
}

type xmlTVEpisodeNum struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

// advertisable reports whether a broadcast belongs in a media server's EPG.
//
// THE ONE PLACE decision #12 is enforced. Only real programmes are advertised:
//
//   - FILLER and FLEX are breaks. §10: "a break rendering as its own EPG entry is confusing in
//     the family's TV guide, and empty breaks have already caused exactly that" — a bare
//     channel name appearing between episodes. The channel still PLAYS them; the guide simply
//     does not list them, so the surrounding programmes read as contiguous.
//   - PENDING is an acquisition still in flight. Advertising it promises a household a
//     programme that may never arrive.
//   - An untitled programme renders as a blank row, which is worse than the gap a media server
//     fills with "no information".
func advertisable(b Broadcast) bool {
	return b.Kind == schedule.SlotProgram && b.Title != ""
}

// RenderXMLTV produces the guide document.
//
// BREAKS ARE NOT ADVERTISED (decision #12, §10). A commercial break rendering as its own EPG
// entry is confusing in a family's TV guide, and empty breaks have already caused exactly that
// — a bare channel name appearing between episodes. Filler and flex are skipped here rather
// than in BroadcastsBetween, because Loomarr's own time-grid legitimately shows them; the
// filtering belongs to the renderer that has an opinion.
//
// A programme with no title is also skipped: an untitled entry renders as a blank row, which is
// worse than a gap the media server fills with "no information".
func RenderXMLTV(g Guide, now time.Time) ([]byte, error) {
	doc := xmlTV{
		GeneratorInfoName: "loomarr",
		Date:              now.UTC().Format(xmltvTime),
	}

	for _, ch := range g.Channels {
		num := strconv.Itoa(ch.Number)
		c := xmlTVChannel{
			ID: ch.ID,
			DisplayNames: []string{
				num + " " + ch.Name,
				num,
				ch.Name,
			},
		}
		if ch.IconURL != "" {
			c.Icon = &xmlTVIcon{Src: ch.IconURL}
		}
		doc.Channels = append(doc.Channels, c)

		for _, b := range g.Programmes[ch.ID] {
			if !advertisable(b) {
				continue
			}
			p := xmlTVProgramme{
				Start:   b.Start.UTC().Format(xmltvTime),
				Stop:    b.Stop.UTC().Format(xmltvTime),
				Channel: ch.ID,
				Title:   b.Title,
			}
			if b.Season > 0 && b.Episode > 0 {
				// xmltv_ns is zero-based: S1E5 is "0.4.".
				p.EpisodeNum = &xmlTVEpisodeNum{
					System: "xmltv_ns",
					Value:  fmt.Sprintf("%d.%d.", b.Season-1, b.Episode-1),
				}
			}
			doc.Programmes = append(doc.Programmes, p)
		}
	}

	body, err := xml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render xmltv: %w", err)
	}

	// The DOCTYPE is what several media servers key on to recognise the document as XMLTV at
	// all; encoding/xml will not emit it, so it is prepended here alongside the declaration.
	out := []byte(xml.Header + `<!DOCTYPE tv SYSTEM "xmltv.dtd">` + "\n")
	return append(out, body...), nil
}
