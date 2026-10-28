package broadcaststation

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/installationlocation"
)

// Directory resolves the call signs a clip's text names to stations. It is immutable once built and
// safe for concurrent use.
type Directory struct {
	stations []Station
	byID     map[string]int
	bySign   map[string][]holding
}

type holding struct {
	station *Station
	span    CallSign
}

// NewDirectory indexes stations by every call sign they held.
func NewDirectory(stations []Station) *Directory {
	d := &Directory{stations: stations, byID: map[string]int{}, bySign: map[string][]holding{}}
	for i := range stations {
		s := &stations[i]
		d.byID[s.ID] = i
		for _, c := range s.CallSigns {
			d.bySign[c.Sign] = append(d.bySign[c.Sign], holding{station: s, span: c})
		}
	}
	return d
}

// Near returns the ids of the stations in country whose community of license lies within km of the
// coordinates: the stations a household there could receive (#2059 decision 2).
func (d *Directory) Near(country string, latitude, longitude, km float64) map[string]bool {
	out := map[string]bool{}
	for _, s := range d.stations {
		if s.Country == country && installationlocation.DistanceKM(latitude, longitude, s.Latitude, s.Longitude) <= km {
			out[s.ID] = true
		}
	}
	return out
}

// Station returns the station with the given id.
func (d *Directory) Station(id string) (Station, bool) {
	at, ok := d.byID[id]
	if !ok {
		return Station{}, false
	}
	return d.stations[at], true
}

// Resolve returns the station an operator means by a call sign (with or without its suffix) or a
// station id, on airDate when one is given. Like Identify, two candidates mean no station.
func (d *Directory) Resolve(callSignOrID, airDate string) (Station, bool) {
	value := strings.TrimSpace(callSignOrID)
	if s, ok := d.Station(value); ok {
		return s, true
	}
	sign := NormalizeCallSign(value)
	if sign == "" {
		return Station{}, false
	}
	window := CallSign{}
	if day, err := time.Parse(time.DateOnly, airDate); err == nil {
		window = CallSign{From: airDate, To: day.AddDate(0, 0, 1).Format(time.DateOnly)}
	}
	var found *Station
	for _, h := range d.bySign[sign] {
		if !overlaps(h.span, window) {
			continue
		}
		if found != nil && found.ID != h.station.ID {
			return Station{}, false
		}
		found = h.station
	}
	if found == nil {
		return Station{}, false
	}
	return *found, true
}

// Match is the one station a clip's text names, and the call sign and date that named it.
type Match struct {
	Station  Station
	CallSign string
	AirDate  string // YYYY-MM-DD when the text gives a full date
}

// Archive titles write a call sign in capitals with a cue that it is one: a licence suffix
// ("WABC-TV"), a network prefix ("CBS/WJW") or a channel number ("WJW 8", "WEWS NewsChannel 5").
// Many call signs are also words (KING, WHO, WEEK), so a bare capitalised word never counts. A
// channel number counts only in mixed-case text that is about broadcasting ("The WEEK 1 Recap" is
// not); in all-capitals text ("BURGER KING 2 FOR 1") and on-screen text only a suffix or prefix does.
var (
	callSignToken = regexp.MustCompile(`(?:\b(?:ABC|CBS|NBC|FOX|PBS|UPN|WB|CW|IND)(/))?\b([KW][A-Z]{2,3})(-(?:TV|DT|CD|CA|LD|LP|HD))?\b`)
	channelCue    = regexp.MustCompile(`^\s+(?:(?:News\s?Channel|Channel|TV)\s?)?[0-9]{1,2}\b`)
	broadcasting  = regexp.MustCompile(`(?i)\b(?:news\w*|weather|forecast|promo|ident|id|station|channel|tv|television|commercial|ad|aired|broadcast|recorded|sign[- ]?(?:on|off)|eyewitness|meteorologist)\b`)
	lowercase     = regexp.MustCompile(`[a-z]`)
)

var (
	isoDate = regexp.MustCompile(`\b(19[3-9][0-9]|20[0-3][0-9])-([01][0-9])-([0-3][0-9])\b`)
	usDate  = regexp.MustCompile(`\b([01]?[0-9])/([0-3]?[0-9])/(19[3-9][0-9]|20[0-3][0-9])\b`)
	year    = regexp.MustCompile(`\b(19[3-9][0-9]|20[0-3][0-9])\b`)
)

// Identify returns the single station a clip names. Call signs come from the title, original name,
// description and on-screen text (#2059 decision 5); the date only from the title and original name,
// which describe the broadcast, while a description often says when the tape was digitised. Each
// call sign is resolved against the stations holding it on that date: one full date, else one year,
// else any time. Two stations, from one call sign or two, mean no match (#2059 decision 15).
func (d *Directory) Identify(title, originalName, description, visibleText string) (Match, bool) {
	// Filenames separate words with underscores, which regexp counts as word characters.
	title, originalName = spaced(title), spaced(originalName)
	var signs []string
	seen := map[string]bool{}
	add := func(text string, strict bool) {
		for _, sign := range callSigns(text, strict) {
			if !seen[sign] {
				seen[sign] = true
				signs = append(signs, sign)
			}
		}
	}
	add(title, false)
	add(originalName, false)
	add(description, false)
	// OCR is all capitals and full of ad copy, so on-screen text needs a suffix or prefix.
	add(visibleText, true)
	if len(signs) == 0 {
		return Match{}, false
	}
	window, airDate := dateWindow([]string{title, originalName})
	var found *holding
	for _, sign := range signs {
		for i := range d.bySign[sign] {
			h := &d.bySign[sign][i]
			if !overlaps(h.span, window) {
				continue
			}
			if found != nil && found.station.ID != h.station.ID {
				return Match{}, false
			}
			found = h
		}
	}
	if found == nil {
		return Match{}, false
	}
	return Match{Station: *found.station, CallSign: found.span.Sign, AirDate: airDate}, true
}

func spaced(text string) string { return strings.ReplaceAll(text, "_", " ") }

// callSigns returns the cued call signs in text, in order. Strict text accepts only a suffix or a
// network prefix.
func callSigns(text string, strict bool) []string {
	numberCounts := !strict && lowercase.MatchString(text) && broadcasting.MatchString(text)
	var out []string
	for _, m := range callSignToken.FindAllStringSubmatchIndex(text, -1) {
		prefixed, suffixed := m[2] >= 0, m[6] >= 0
		if prefixed || suffixed || (numberCounts && channelCue.MatchString(text[m[1]:])) {
			out = append(out, text[m[4]:m[5]])
		}
	}
	return out
}

// dateWindow is the span a clip aired in, as far as its text says.
func dateWindow(texts []string) (CallSign, string) {
	var dates []string
	years := map[int]bool{}
	for _, text := range texts {
		for _, m := range isoDate.FindAllStringSubmatch(text, -1) {
			dates = appendDate(dates, m[1], m[2], m[3])
		}
		for _, m := range usDate.FindAllStringSubmatch(text, -1) {
			dates = appendDate(dates, m[3], m[1], m[2])
		}
		for _, m := range year.FindAllStringIndex(text, -1) {
			// A digit run joined to other digits by a hyphen or slash is a phone number or a date,
			// not a year ("1-800-433-2020"); dates are read above. "WEWS-news-1991" is a year.
			if joinedToDigits(text, m[0], -1) || joinedToDigits(text, m[1]-1, 1) {
				continue
			}
			y, _ := strconv.Atoi(text[m[0]:m[1]])
			years[y] = true
		}
	}
	for _, date := range dates {
		y, _ := strconv.Atoi(date[:4])
		years[y] = true
	}
	if len(dates) == 1 {
		day, _ := time.Parse(time.DateOnly, dates[0])
		return CallSign{From: dates[0], To: day.AddDate(0, 0, 1).Format(time.DateOnly)}, dates[0]
	}
	if len(years) == 1 {
		for y := range years {
			return CallSign{From: strconv.Itoa(y) + "-01-01", To: strconv.Itoa(y+1) + "-01-01"}, ""
		}
	}
	return CallSign{}, ""
}

// joinedToDigits reports whether the digit at text[i] continues, in direction step, through a hyphen
// or slash into another digit.
func joinedToDigits(text string, i, step int) bool {
	sep, next := i+step, i+2*step
	return next >= 0 && next < len(text) && (text[sep] == '-' || text[sep] == '/') &&
		text[next] >= '0' && text[next] <= '9'
}

func appendDate(dates []string, y, m, d string) []string {
	if len(m) == 1 {
		m = "0" + m
	}
	if len(d) == 1 {
		d = "0" + d
	}
	date := y + "-" + m + "-" + d
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return dates
	}
	for _, existing := range dates {
		if existing == date {
			return dates
		}
	}
	return append(dates, date)
}

// overlaps reports whether two spans share any day; an empty bound is open.
func overlaps(a, b CallSign) bool {
	return (a.To == "" || b.From < a.To) && (b.To == "" || a.From < b.To)
}
