package playout

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

var guideNow = time.Date(2026, 7, 25, 20, 0, 0, 0, time.UTC)

func programme(title string, start time.Time, mins int) Broadcast {
	return Broadcast{
		Kind: schedule.SlotProgram, Title: title,
		Start: start, Stop: start.Add(time.Duration(mins) * time.Minute),
	}
}

func renderOne(t *testing.T, g Guide) string {
	t.Helper()
	out, err := RenderXMLTV(g, guideNow)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The document must be well-formed XML with the DOCTYPE several media servers key on to
// recognise it as XMLTV at all. encoding/xml will not emit a DOCTYPE, so this catches its loss.
func TestRenderXMLTV_IsWellFormedWithADoctype(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Classic Sci-Fi", Number: 50}},
		Programmes: map[string][]Broadcast{"ch1": {programme("Heat", guideNow, 60)}},
	})

	if !strings.Contains(got, `<!DOCTYPE tv SYSTEM "xmltv.dtd">`) {
		t.Errorf("missing the DOCTYPE:\n%s", got)
	}
	if !strings.HasPrefix(got, xml.Header) {
		t.Error("missing the XML declaration")
	}
	// Parseable is the real bar — an unparseable guide is a SILENTLY empty guide.
	var doc struct {
		XMLName  xml.Name `xml:"tv"`
		Channels []struct {
			ID string `xml:"id,attr"`
		} `xml:"channel"`
		Programmes []struct {
			Title string `xml:"title"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("the guide does not parse, so a media server would show nothing: %v", err)
	}
	if len(doc.Channels) != 1 || len(doc.Programmes) != 1 {
		t.Errorf("got %d channels / %d programmes, want 1/1", len(doc.Channels), len(doc.Programmes))
	}
}

// ⚠ THE MOST COMMON LIVE TV WIRING FAILURE: `channel@id` not matching the M3U's `tvg-id`. It is
// silent — every channel plays and every guide is empty — so it is asserted rather than trusted.
func TestRenderXMLTV_ChannelIDMatchesTheTunerID(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "classic-simpsons", Name: "Classic Simpsons", Number: 52}},
		Programmes: map[string][]Broadcast{"classic-simpsons": {programme("Bart the Genius", guideNow, 22)}},
	})

	// The tuner writes tvg-id="classic-simpsons"; the guide must use the identical string.
	if !strings.Contains(got, `<channel id="classic-simpsons">`) {
		t.Errorf("channel id does not match the tuner's tvg-id:\n%s", got)
	}
	if !strings.Contains(got, `channel="classic-simpsons"`) {
		t.Errorf("programme is not attributed to the channel id:\n%s", got)
	}
}

// XMLTV's own timestamp format, not RFC3339 — and always UTC. Sending local time invites a
// double-conversion that shifts the whole guide by hours, and "everything is on at the wrong
// time" does not look like a timezone bug.
func TestRenderXMLTV_TimestampsAreXMLTVFormatInUTC(t *testing.T) {
	// A non-UTC start, to prove it is converted rather than formatted as-is.
	zone := time.FixedZone("UTC-5", -5*3600)
	start := time.Date(2026, 7, 25, 15, 30, 0, 0, zone) // = 20:30 UTC

	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{"ch1": {programme("Heat", start, 60)}},
	})

	if !strings.Contains(got, `start="20260725203000 +0000"`) {
		t.Errorf("start is not XMLTV-format UTC (want 20260725203000 +0000):\n%s", got)
	}
	if !strings.Contains(got, `stop="20260725213000 +0000"`) {
		t.Errorf("stop is not XMLTV-format UTC:\n%s", got)
	}
	if strings.Contains(got, "T") && strings.Contains(got, "Z") {
		t.Error("looks like RFC3339 — a media server will not parse it, and will show nothing")
	}
}

// DECISION #12: breaks are NOT advertised. §10 — a break rendering as its own EPG entry is
// confusing in a family's guide, and empty breaks already caused exactly that (a bare channel
// name between episodes).
func TestRenderXMLTV_DoesNotAdvertiseBreaks(t *testing.T) {
	got := renderOne(t, Guide{
		Channels: []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{"ch1": {
			programme("Heat", guideNow, 60),
			{Kind: schedule.SlotFiller, Title: "commercials", Start: guideNow.Add(60 * time.Minute), Stop: guideNow.Add(61 * time.Minute)},
			{Kind: schedule.SlotFlex, Title: "flex", Start: guideNow.Add(61 * time.Minute), Stop: guideNow.Add(62 * time.Minute)},
			programme("Predator", guideNow.Add(62*time.Minute), 30),
		}},
	})

	if strings.Contains(got, "commercials") || strings.Contains(got, ">flex<") {
		t.Errorf("a break was advertised in the EPG (#12):\n%s", got)
	}
	// …but the real programmes on either side survive.
	for _, want := range []string{"Heat", "Predator"} {
		if !strings.Contains(got, want) {
			t.Errorf("dropped the real programme %q:\n%s", want, got)
		}
	}
}

// A pending acquisition must not be advertised: it promises a household a programme that may
// never arrive.
func TestRenderXMLTV_DoesNotAdvertisePendingAcquisitions(t *testing.T) {
	got := renderOne(t, Guide{
		Channels: []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{"ch1": {
			{Kind: schedule.SlotPending, Title: "Con Air", Start: guideNow, Stop: guideNow.Add(time.Hour)},
		}},
	})
	if strings.Contains(got, "Con Air") {
		t.Errorf("advertised a title that has not been acquired:\n%s", got)
	}
}

// Operator- and library-supplied text lands in XML. `&`, `<` and quotes must be escaped or the
// document breaks — and a broken document is a silently empty guide.
func TestRenderXMLTV_EscapesOperatorAndLibraryText(t *testing.T) {
	got := renderOne(t, Guide{
		Channels: []GuideChannel{{ID: "ch1", Name: `Rock & "Roll" <Hits>`, Number: 7}},
		Programmes: map[string][]Broadcast{"ch1": {
			programme(`Truffaut's "Jules & Jim" <1962>`, guideNow, 105),
		}},
	})

	// Raw metacharacters would end the element early.
	if strings.Contains(got, `"Jules & Jim"`) {
		t.Errorf("title was not escaped:\n%s", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("no escaping at all:\n%s", got)
	}
	// The document must still parse, which is the property that actually matters.
	var doc struct {
		Programmes []struct {
			Title string `xml:"title"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("escaped document does not parse: %v", err)
	}
	if len(doc.Programmes) != 1 || doc.Programmes[0].Title != `Truffaut's "Jules & Jim" <1962>` {
		t.Errorf("title did not round-trip: %+v", doc.Programmes)
	}
}

// Three display-names, in Tunarr's order. Media servers pick differently between them, and
// offering all three is what makes a channel show a sensible label everywhere rather than a
// bare number in one client and a name in another.
func TestRenderXMLTV_OffersNumberAndNameDisplayNames(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Classic Sci-Fi", Number: 50}},
		Programmes: map[string][]Broadcast{},
	})
	for _, want := range []string{
		"<display-name>50 Classic Sci-Fi</display-name>",
		"<display-name>50</display-name>",
		"<display-name>Classic Sci-Fi</display-name>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s:\n%s", want, got)
		}
	}
}

// An icon is omitted when unset rather than emitted empty — an empty src reads as a broken
// image rather than no image.
func TestRenderXMLTV_OmitsAnAbsentIcon(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{},
	})
	if strings.Contains(got, "<icon") {
		t.Errorf("emitted an icon element with no source:\n%s", got)
	}

	withIcon := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1, IconURL: "http://loomarr/i.png"}},
		Programmes: map[string][]Broadcast{},
	})
	if !strings.Contains(withIcon, `<icon src="http://loomarr/i.png">`) &&
		!strings.Contains(withIcon, `<icon src="http://loomarr/i.png"></icon>`) {
		t.Errorf("icon not emitted when set:\n%s", withIcon)
	}
}

// Season/episode numbers reach the guide in xmltv_ns format, which is ZERO-BASED: S1E5 is
// "0.4.". A media server that understands it renders "S1E5"; one that does not ignores it.
func TestRenderXMLTV_EpisodeNumbersAreZeroBasedXmltvNs(t *testing.T) {
	b := programme("Bart the Genius", guideNow, 22)
	b.Season, b.Episode = 1, 5

	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{"ch1": {b}},
	})
	if !strings.Contains(got, `system="xmltv_ns"`) {
		t.Errorf("no episode-num system:\n%s", got)
	}
	if !strings.Contains(got, ">0.4.<") {
		t.Errorf("episode-num is not zero-based xmltv_ns (S1E5 ⇒ 0.4.):\n%s", got)
	}
}

// A movie carries no season/episode, and must not get a bogus "-1.-1." entry.
func TestRenderXMLTV_MoviesGetNoEpisodeNumber(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "ch1", Name: "Ch", Number: 1}},
		Programmes: map[string][]Broadcast{"ch1": {programme("Heat", guideNow, 170)}},
	})
	if strings.Contains(got, "episode-num") {
		t.Errorf("a movie got an episode number:\n%s", got)
	}
}

// A channel with nothing to advertise still appears, so the media server keeps the channel and
// shows "no information" rather than dropping it from Live TV entirely.
func TestRenderXMLTV_ChannelWithNoProgrammesStillAppears(t *testing.T) {
	got := renderOne(t, Guide{
		Channels:   []GuideChannel{{ID: "empty", Name: "Nothing Yet", Number: 99}},
		Programmes: map[string][]Broadcast{},
	})
	if !strings.Contains(got, `<channel id="empty">`) {
		t.Errorf("channel disappeared from the guide because it had no programmes:\n%s", got)
	}
}
