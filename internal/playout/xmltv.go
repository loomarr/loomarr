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
	// Title is the SERIES name for an episode, the film's name for a movie. A media server
	// groups and searches by this, which is why an episode must not put its own name here.
	Title string `xml:"title"`
	// SubTitle is the EPISODE name, omitted for a movie.
	SubTitle string `xml:"sub-title,omitempty"`
	// Desc is the plot description — what makes a guide entry worth selecting. Its absence is
	// why the series/episode split had to be collapsed into the title: with no detail pane
	// content, the grid was the only surface carrying information.
	Desc string `xml:"desc,omitempty"`
	// Categories are genres. XMLTV allows several, and media servers use them for filtering
	// and for the coloured category chips some guides draw.
	Categories []string `xml:"category,omitempty"`
	// Date is the original release/air date. Year-only is valid XMLTV and is all we have;
	// a media server renders it as "(1993)" next to the title.
	Date string `xml:"date,omitempty"`
	// Rating is the content rating, in the system a media server expects to see named.
	Rating *xmlTVRating `xml:"rating,omitempty"`
	// EpisodeNums — TWO of them when season/episode are known, because clients disagree about
	// which they read:
	//
	//	xmltv_ns  "6.1."  machine-readable, ZERO-BASED (S7E2), the format's own convention
	//	onscreen  "S7E2"  what a client displays verbatim when it does not parse xmltv_ns
	//
	// Tunarr emits both, and it is the reason its guide shows "S7E2" where a xmltv_ns-only
	// document shows nothing.
	EpisodeNums []xmlTVEpisodeNum `xml:"episode-num,omitempty"`
}

type xmlTVEpisodeNum struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

// xmlTVRating is a content rating. The nested `<value>` is XMLTV's shape, not a nicety —
// a bare `<rating>TV-PG</rating>` is not valid and parsers skip it.
type xmlTVRating struct {
	System string `xml:"system,attr"`
	Value  string `xml:"value"`
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
	// !Nominal is redundant with the Kind check today (only pending slots are nominal, and
	// pending is not SlotProgram) and is stated anyway: it is the ACTUAL reason such a block
	// must not be advertised. A nominal block's times are an invented display width, so
	// publishing one tells a media server a programme starts at a time it never will. Anyone
	// later widening the Kind test — to advertise breaks, say — would otherwise silently
	// re-open that hole.
	return b.Kind == schedule.SlotProgram && b.Title != "" && !b.Nominal
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
			// An episode's title carries BOTH names: "The Simpsons: Bart the Mother".
			//
			// This is a deliberate departure from the strict XMLTV reading, and it took two
			// wrong answers to arrive at. The spec says `<title>` is the series and
			// `<sub-title>` the episode, and both were tried live against Emby:
			//
			//	episode in <title>   grid showed "Bart the Mother" — no idea which show
			//	series in <title>    grid showed "The Simpsons" — no idea which episode
			//
			// Emby parses BOTH correctly (its API reports Name="The Simpsons",
			// EpisodeTitle="Bart the Mother"), but its guide GRID renders `Name` alone, and
			// the grid is where a viewer actually reads the schedule. With no `<desc>` to
			// fill the detail pane there is no second surface, so one field has to carry it.
			//
			// `<sub-title>` is still emitted, so a client that shows it — or that groups a
			// series — keeps working; this only changes what the single-field grid displays.
			if b.SeriesTitle != "" {
				p.Title = b.SeriesTitle + ": " + b.Title
				p.SubTitle = b.Title
			}
			p.Desc = b.Description
			p.Categories = b.Genres
			if b.Year > 0 {
				// Year-only is valid XMLTV; a media server renders it as "(1993)". Padding it
				// to a fake January 1st would assert a precision we do not have.
				p.Date = strconv.Itoa(b.Year)
			}
			if b.Rating != "" {
				// The system name matters: a media server maps ratings per system, and an
				// unnamed one is ignored. Emby reports US-style ratings ("TV-PG", "PG-13"),
				// which is what both MPAA and TV Parental Guidelines values look like here —
				// Tunarr labels the same values "MPAA", so that is the compatible choice.
				p.Rating = &xmlTVRating{System: "MPAA", Value: b.Rating}
			}
			if b.Season > 0 && b.Episode > 0 {
				p.EpisodeNums = []xmlTVEpisodeNum{
					// Machine-readable, zero-based: S1E5 is "0.4.".
					{System: "xmltv_ns", Value: fmt.Sprintf("%d.%d.", b.Season-1, b.Episode-1)},
					// Displayed verbatim by clients that do not parse xmltv_ns.
					{System: "onscreen", Value: fmt.Sprintf("S%dE%d", b.Season, b.Episode)},
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
