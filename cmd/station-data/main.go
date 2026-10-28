// Command station-data builds the deterministic broadcast-station seed (#2060): every US TV station's
// community of license and its dated call signs, from the FCC's frozen CDBS dump plus a reviewed
// Wikidata snapshot for history the FCC never recorded.
//
//	go run ./cmd/station-data -output internal/broadcaststation
//	go run ./cmd/station-data -refresh-wikidata   # re-query Wikidata into wikidata-call-signs.tsv
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/broadcaststation"
	"github.com/loomarr/loomarr/internal/installationlocation"
)

// CDBS stopped updating on 2023-10-01 when the FCC moved to LMS, so these files are fixed and pin
// cleanly. Call-sign changes after that date are absent until a later revision adds an LMS input.
const (
	cdbsBase       = "https://transition.fcc.gov/Bureaus/MB/Databases/cdbs/"
	facilitySHA    = "42a08c870feea389416533375ba8923ba4e449d6912a28aa5ad1c9d51f943d62"
	historySHA     = "b012b007ac287c09cd987d2eb938880431093094603f5407a1794b2ae4c40c0c"
	engineeringSHA = "e61d1026fdac3131ed561dd3edf7ad8f75d4232a01aa0cd63fccd0d44e1ffdc3"
	fccCredit      = "FCC Consolidated Database System (CDBS) public data, a US Government work: https://www.fcc.gov/media/radio/cdbs-database-public-files"
	wikidataCredit = "Wikidata, CC0: https://www.wikidata.org/"
	wikidataPath   = "cmd/station-data/wikidata-call-signs.tsv"
	// FCC endpoints refuse Go's default client identity.
	userAgent = "loomarr-station-data/1 (+https://github.com/loomarr/loomarr)"
	// A community of license farther than this from the licensed transmitter is a different town
	// with the same name (Cleveland, Ohio is not Cleveland, Tennessee).
	maxPlaceDistanceKM = 100
)

//go:embed wikidata-call-signs.rq
var wikidataQuery string

// Only full-power and Class A television; translators and LPTV relay programming rather than make
// the local station material archive titles name. FVOID and PRCAN permits never went on air.
var (
	tvServices       = map[string]bool{"TV": true, "DT": true, "CA": true, "DC": true}
	neverOnAir       = map[string]bool{"FVOID": true, "PRCAN": true}
	licensed         = map[string]bool{"LICEN": true, "LICAN": true, "LICSL": true}
	territoryCountry = map[string]string{"PR": "PR", "VI": "VI", "GU": "GU", "AS": "AS", "MP": "MP"}
	usStates         = strings.Fields("AL AK AZ AR CA CO CT DE DC FL GA HI ID IL IN IA KS KY LA ME MD MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY")
)

type source struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Credit string `json:"credit"`
}

type output struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Rows   int    `json:"rows"`
}

type metadata struct {
	Version  string   `json:"version"`
	Sources  []source `json:"sources"`
	Outputs  []output `json:"outputs"`
	Stations int      `json:"stations"`
	// Ambiguous counts pairs of stations shown holding one call sign at once; a clip naming one
	// resolves to no station. Review a rise before shipping a revision.
	Ambiguous int `json:"ambiguousCallSignPairs"`
}

type facility struct {
	id, city, state, callSign string
	classA                    bool // a Class A licence: full-power television keeps a shared base call sign
	licensed                  bool
}

type point struct{ lat, lon float64 }

type historyRow struct {
	seq   int
	sign  string
	begin string
}

type wikidataRow struct {
	qid, facilityID, sign, from, to string
	at                              *point
}

func main() {
	outputDir := flag.String("output", ".", "output directory")
	snapshot := flag.String("wikidata", wikidataPath, "the committed Wikidata snapshot")
	refresh := flag.Bool("refresh-wikidata", false, "re-run the Wikidata query into the snapshot and exit")
	flag.Parse()
	if *refresh {
		refreshWikidata(*snapshot)
		return
	}

	tempDir, err := os.MkdirTemp("", "loomarr-station-data-")
	must(err)
	defer func() { must(os.RemoveAll(tempDir)) }()

	facilities := readFacilities(readZip(download(tempDir, cdbsBase+"facility.zip", facilitySHA)))
	history := readHistory(readZip(download(tempDir, cdbsBase+"call_sign_history.zip", historySHA)), facilities)
	transmitters := readTransmitters(readZip(download(tempDir, cdbsBase+"tv_eng_data.zip", engineeringSHA)), facilities)
	places := placesByName()
	wikidata := readWikidata(*snapshot)

	stations := buildStations(facilities, history, transmitters, places, wikidata)
	preferFullPower(stations, facilities)
	clipReusedCallSigns(stations)
	stations = slices.DeleteFunc(stations, func(s broadcaststation.Station) bool { return len(s.CallSigns) == 0 })
	ambiguous := concurrentHolders(stations)
	fmt.Fprintf(os.Stderr, "station-data: %d stations; %d call-sign spans held by two stations at once (they resolve to no station)\n", len(stations), ambiguous)

	seedPath := filepath.Join(*outputDir, "stations.tsv.gz")
	var encoded bytes.Buffer
	must(broadcaststation.EncodeSeed(&encoded, stations))
	must(os.WriteFile(seedPath, encoded.Bytes(), 0o644))
	rows := 0
	for _, s := range stations {
		rows += len(s.CallSigns)
	}
	meta := metadata{
		Version: "cdbs-2023-10+wikidata-" + wikidataRetrieved(*snapshot),
		Sources: []source{
			{URL: cdbsBase + "facility.zip", SHA256: facilitySHA, Credit: fccCredit},
			{URL: cdbsBase + "call_sign_history.zip", SHA256: historySHA, Credit: fccCredit},
			{URL: cdbsBase + "tv_eng_data.zip", SHA256: engineeringSHA, Credit: fccCredit},
			{URL: wikidataPath, SHA256: fileSHA(*snapshot), Credit: wikidataCredit},
		},
		Outputs:   []output{{File: filepath.Base(seedPath), SHA256: fileSHA(seedPath), Rows: rows}},
		Stations:  len(stations),
		Ambiguous: ambiguous,
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*outputDir, "station-data.generated.json"), append(metaJSON, '\n'), 0o644))
}

func buildStations(facilities map[string]facility, history map[string][]historyRow, transmitters map[string]point,
	places map[string][]installationlocation.Place, wikidata map[string][]wikidataRow) []broadcaststation.Station {
	var stations []broadcaststation.Station
	for id, f := range facilities {
		if !f.licensed && len(history[id]) == 0 {
			continue // an unbuilt record: placeholder call signs such as WTB
		}
		at, ok := transmitters[id]
		station := broadcaststation.Station{ID: "fcc:" + id, Country: countryOf(f.state), Region: f.state, City: titleCase(f.city)}
		if place, found := nearestPlace(places[normalizeCity(f.city)], at, ok); found {
			station.City, at, ok = place.Name, point{place.Latitude, place.Longitude}, true
		}
		if !ok {
			continue // nowhere to measure a radius from
		}
		station.Latitude, station.Longitude = at.lat, at.lon
		station.CallSigns = mergeWikidata(spansFromHistory(history[id], f.callSign), wikidata[id])
		delete(wikidata, id)
		if len(station.CallSigns) > 0 {
			stations = append(stations, station)
		}
	}
	sort.Slice(stations, func(i, j int) bool { return stations[i].ID < stations[j].ID })
	// Wikidata items no kept FCC facility claims by id. Most are FCC stations Wikidata never linked
	// (KSAZ Phoenix): exactly one FCC station holding one of the item's call signs over the same
	// years takes its history. An item matching none is a station CDBS never knew; one matching
	// several cannot be placed.
	byQID := map[string][]wikidataRow{}
	for _, rows := range wikidata {
		for _, row := range rows {
			byQID[row.qid] = append(byQID[row.qid], row)
		}
	}
	qids := make([]string, 0, len(byQID))
	for qid := range byQID {
		qids = append(qids, qid)
	}
	sort.Strings(qids)
	fccCount := len(stations)
	for _, qid := range qids {
		rows := byQID[qid]
		spans := mergeWikidata(nil, rows)
		var matches []int
		for i := range stations[:fccCount] {
			if sharesTenure(stations[i].CallSigns, spans) {
				matches = append(matches, i)
			}
		}
		switch {
		case len(matches) == 1:
			stations[matches[0]].CallSigns = mergeWikidata(stations[matches[0]].CallSigns, rows)
		case len(matches) == 0 && len(spans) > 0:
			var at *point
			for _, row := range rows {
				if row.at != nil {
					at = row.at
				}
			}
			if at != nil {
				stations = append(stations, broadcaststation.Station{ID: "wd:" + qid, Country: "US", Latitude: at.lat, Longitude: at.lon, CallSigns: spans})
			}
		}
	}
	sort.Slice(stations, func(i, j int) bool { return stations[i].ID < stations[j].ID })
	return stations
}

// sharesTenure reports whether two stations' histories hold one call sign over overlapping dates.
func sharesTenure(a, b []broadcaststation.CallSign) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Sign == y.Sign && overlaps(x, y) {
				return true
			}
		}
	}
	return false
}

func overlaps(x, y broadcaststation.CallSign) bool {
	return (x.To == "" || y.From < x.To) && (y.To == "" || x.From < y.To)
}

// preferFullPower settles the commonest shared tenure: a Class A station licensed with the same
// letters as a full-power one (WWNY-CD beside WWNY-TV). Titles name the full-power station, so the
// Class A station loses the overlapping dates.
func preferFullPower(stations []broadcaststation.Station, facilities map[string]facility) {
	fullPower := map[string][]broadcaststation.CallSign{}
	for _, s := range stations {
		if f, ok := facilities[strings.TrimPrefix(s.ID, "fcc:")]; ok && !f.classA {
			for _, c := range s.CallSigns {
				fullPower[c.Sign] = append(fullPower[c.Sign], c)
			}
		}
	}
	for i := range stations {
		f, ok := facilities[strings.TrimPrefix(stations[i].ID, "fcc:")]
		if !ok || !f.classA {
			continue
		}
		var kept []broadcaststation.CallSign
		for _, c := range stations[i].CallSigns {
			kept = append(kept, subtract(c, fullPower[c.Sign])...)
		}
		stations[i].CallSigns = compact(kept)
	}
}

// subtract removes every taken span's dates from c, returning what remains in order.
func subtract(c broadcaststation.CallSign, taken []broadcaststation.CallSign) []broadcaststation.CallSign {
	pieces := []broadcaststation.CallSign{c}
	for _, t := range taken {
		var next []broadcaststation.CallSign
		for _, p := range pieces {
			if !overlaps(p, t) {
				next = append(next, p)
				continue
			}
			if t.From != "" && (p.From == "" || p.From < t.From) {
				next = append(next, broadcaststation.CallSign{Sign: p.Sign, From: p.From, To: t.From})
			}
			if t.To != "" && (p.To == "" || t.To < p.To) {
				next = append(next, broadcaststation.CallSign{Sign: p.Sign, From: t.To, To: p.To})
			}
		}
		pieces = next
	}
	return pieces
}

// concurrentHolders counts pairs of stations holding one call sign over overlapping dates.
func concurrentHolders(stations []broadcaststation.Station) int {
	type held struct {
		station int
		span    broadcaststation.CallSign
	}
	bySign := map[string][]held{}
	for i, s := range stations {
		for _, c := range s.CallSigns {
			bySign[c.Sign] = append(bySign[c.Sign], held{i, c})
		}
	}
	n := 0
	for _, holders := range bySign {
		for i := range holders {
			for j := i + 1; j < len(holders); j++ {
				if holders[i].station != holders[j].station && overlaps(holders[i].span, holders[j].span) {
					n++
				}
			}
		}
	}
	return n
}

// spansFromHistory turns CDBS rows into call-sign spans. The first row's date is when CDBS began
// tracking the facility, not when the call sign started (WOR-TV's first row is 1978; it was WOR-TV
// from 1949), so the first span is always open at the start. Each later row ends the one before it.
//
// A later row dated at or before the one it follows is a correction: it supersedes every kept row
// from its date on (WDNI-LP 2001-05-29, W65DW 2001-11-29, then WDNI-LP 2001-05-29 again means the
// W65DW change was undone).
func spansFromHistory(rows []historyRow, current string) []broadcaststation.CallSign {
	sort.Slice(rows, func(i, j int) bool { return rows[i].seq < rows[j].seq })
	var kept []historyRow
	for i, row := range rows {
		if i > 0 && row.begin == "" {
			continue // an undated later row cannot be placed in time
		}
		for n := len(kept); i > 0 && n > 0 && kept[n-1].begin >= row.begin; n = len(kept) {
			kept = kept[:n-1]
		}
		kept = append(kept, row)
	}
	var spans []broadcaststation.CallSign
	for i, row := range kept {
		from := row.begin
		if i == 0 {
			from = ""
		}
		if n := len(spans); n > 0 {
			spans[n-1].To = from
		}
		spans = append(spans, broadcaststation.CallSign{Sign: broadcaststation.NormalizeCallSign(row.sign), From: from})
	}
	if len(spans) == 0 {
		spans = []broadcaststation.CallSign{{Sign: broadcaststation.NormalizeCallSign(current)}}
	}
	return compact(spans)
}

// mergeWikidata adds history from before CDBS. Dated Wikidata call signs other than the first FCC
// one, that ended before it did, precede it, and the first FCC span then starts where they end; a
// dated Wikidata entry for the same call sign gives its start. WKYC's KYW-TV 1956-1965 comes from here.
func mergeWikidata(spans []broadcaststation.CallSign, rows []wikidataRow) []broadcaststation.CallSign {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].from != rows[j].from {
			return rows[i].from < rows[j].from
		}
		return rows[i].to < rows[j].to
	})
	if len(spans) == 0 {
		for _, row := range rows {
			spans = append(spans, broadcaststation.CallSign{Sign: broadcaststation.NormalizeCallSign(row.sign), From: row.from, To: row.to})
		}
		sort.SliceStable(spans, func(i, j int) bool { return spans[i].From < spans[j].From })
		return compact(spans)
	}
	first := spans[0]
	if first.From != "" {
		return spans
	}
	var before []broadcaststation.CallSign
	start := ""
	for _, row := range rows {
		sign := broadcaststation.NormalizeCallSign(row.sign)
		switch {
		case sign == "":
		case sign != first.Sign && row.to != "" && (first.To == "" || row.to <= first.To):
			before = append(before, broadcaststation.CallSign{Sign: sign, From: row.from, To: row.to})
			start = max(start, row.to)
		case sign == first.Sign && row.from != "" && len(before) == 0 && (first.To == "" || row.from < first.To):
			if start == "" || row.from < start {
				start = row.from
			}
		}
	}
	spans[0].From = start
	return compact(append(before, spans...))
}

// clipReusedCallSigns applies the FCC's one-holder-at-a-time rule to open starts. KYW is Philadelphia's
// with no recorded start, but Cleveland held it until 1965, so Philadelphia's tenure starts in 1965.
func clipReusedCallSigns(stations []broadcaststation.Station) {
	type held struct{ station, to string }
	ends := map[string][]held{}
	for _, s := range stations {
		for _, c := range s.CallSigns {
			if c.To != "" {
				ends[c.Sign] = append(ends[c.Sign], held{s.ID, c.To})
			}
		}
	}
	for i := range stations {
		s := &stations[i]
		for j := range s.CallSigns {
			c := &s.CallSigns[j]
			if c.From != "" {
				continue
			}
			for _, other := range ends[c.Sign] {
				if other.station != s.ID && (c.To == "" || other.to < c.To) && other.to > c.From {
					c.From = other.to
				}
			}
		}
	}
}

// compact drops unparseable signs and empty spans, and joins neighbours that are the same base call
// sign and touch or overlap (WEWS then WEWS-TV is one tenure; Wikidata may list WJZ and WJZ-TV for
// the same years).
func compact(spans []broadcaststation.CallSign) []broadcaststation.CallSign {
	out := spans[:0]
	for _, span := range spans {
		if span.Sign == "" || (span.From != "" && span.To != "" && span.From >= span.To) {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Sign == span.Sign && (out[n-1].To == "" || out[n-1].To >= span.From) {
			if out[n-1].To != "" && (span.To == "" || span.To > out[n-1].To) {
				out[n-1].To = span.To
			}
			continue
		}
		// Contradictory community data (Wikidata dating KOY 1953-1954 and KOOL 1953-1982): a later
		// start ends the span before it; a span starting with it keeps only the years after it, so
		// KOOL holds 1954-1982. An undated span after another cannot be placed.
		if n := len(out); n > 0 && (out[n-1].To == "" || out[n-1].To > span.From) {
			switch {
			case span.From != "" && span.From > out[n-1].From:
				out[n-1].To = span.From
			case out[n-1].To != "" && (span.To == "" || span.To > out[n-1].To):
				span.From = out[n-1].To
			default:
				continue
			}
		}
		out = append(out, span)
	}
	return out
}

func readFacilities(records [][]string) map[string]facility {
	out := map[string]facility{}
	us := map[string]bool{}
	for _, state := range usStates {
		us[state] = true
	}
	for _, r := range records {
		if len(r) < 17 {
			continue
		}
		state := strings.TrimSpace(r[1])
		if !tvServices[r[10]] || neverOnAir[r[16]] || (!us[state] && territoryCountry[state] == "") ||
			broadcaststation.NormalizeCallSign(r[5]) == "" {
			continue
		}
		id := strings.TrimSpace(r[14])
		out[id] = facility{id: id, city: strings.TrimSpace(r[0]), state: state, callSign: r[5],
			classA: r[10] == "CA" || r[10] == "DC", licensed: licensed[r[16]]}
	}
	return out
}

func readHistory(records [][]string, facilities map[string]facility) map[string][]historyRow {
	out := map[string][]historyRow{}
	for _, r := range records {
		if len(r) < 5 {
			continue
		}
		id := strings.TrimSpace(r[4])
		if _, ok := facilities[id]; !ok {
			continue // radio and translator history shares this file
		}
		seq, err := strconv.Atoi(strings.TrimSpace(r[3]))
		if err != nil {
			continue
		}
		out[id] = append(out[id], historyRow{seq: seq, sign: r[1], begin: fccDate(r[0])})
	}
	return out
}

// readTransmitters keeps one licensed transmitter site per facility, the first in file order.
func readTransmitters(records [][]string, facilities map[string]facility) map[string]point {
	licensed, any := map[string]point{}, map[string]point{}
	for _, r := range records {
		if len(r) < 52 {
			continue
		}
		id := strings.TrimSpace(r[21])
		if _, ok := facilities[id]; !ok {
			continue
		}
		lat, latOK := dms(r[28], r[29], r[30], r[31])
		lon, lonOK := dms(r[32], r[33], r[34], r[35])
		if !latOK || !lonOK {
			continue
		}
		if _, seen := any[id]; !seen {
			any[id] = point{lat, lon}
		}
		if _, seen := licensed[id]; !seen && strings.TrimSpace(r[51]) == "LIC" {
			licensed[id] = point{lat, lon}
		}
	}
	for id, p := range any {
		if _, ok := licensed[id]; !ok {
			licensed[id] = p
		}
	}
	return licensed
}

func placesByName() map[string][]installationlocation.Place {
	all, err := installationlocation.Places()
	must(err)
	out := map[string][]installationlocation.Place{}
	for _, place := range all {
		if place.City == "" {
			continue // a country row
		}
		key := normalizeCity(place.Name)
		out[key] = append(out[key], place)
	}
	return out
}

// nearestPlace is the same-named place closest to the transmitter. With no transmitter on record a
// name alone is ambiguous, so only a unique match counts.
func nearestPlace(candidates []installationlocation.Place, at point, known bool) (installationlocation.Place, bool) {
	if !known {
		if len(candidates) == 1 {
			return candidates[0], true
		}
		return installationlocation.Place{}, false
	}
	best, bestKM := installationlocation.Place{}, math.Inf(1)
	for _, place := range candidates {
		if km := distanceKM(at, point{place.Latitude, place.Longitude}); km < bestKM {
			best, bestKM = place, km
		}
	}
	return best, bestKM <= maxPlaceDistanceKM
}

func normalizeCity(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r == ' ' {
			return r
		}
		return ' '
	}, name)
	words := strings.Fields(name)
	if len(words) > 0 && (words[0] == "st" || words[0] == "ste") {
		words[0] = "saint"
	}
	if n := len(words); n > 1 && words[n-1] == "city" {
		words = words[:n-1]
	}
	return strings.Join(words, " ")
}

func titleCase(name string) string {
	words := strings.Fields(strings.ToLower(name))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func countryOf(state string) string {
	if c := territoryCountry[state]; c != "" {
		return c
	}
	return "US"
}

func fccDate(s string) string {
	t, err := time.Parse("01/02/2006", strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

func dms(deg, dir, minutes, seconds string) (float64, bool) {
	d, errD := strconv.ParseFloat(strings.TrimSpace(deg), 64)
	m, errM := strconv.ParseFloat(strings.TrimSpace(minutes), 64)
	s, errS := strconv.ParseFloat(strings.TrimSpace(seconds), 64)
	if errD != nil || errM != nil || errS != nil {
		return 0, false
	}
	v := d + m/60 + s/3600
	switch strings.TrimSpace(dir) {
	case "S", "W":
		v = -v
	case "N", "E":
	default:
		return 0, false
	}
	return v, true
}

func distanceKM(a, b point) float64 {
	const earthKM = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (b.lat-a.lat)*rad, (b.lon-a.lon)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.lat*rad)*math.Cos(b.lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthKM * math.Asin(math.Sqrt(h))
}

// readZip returns the pipe-delimited rows of a one-file CDBS archive. CDBS is Latin-1.
func readZip(path string) [][]string {
	archive, err := zip.OpenReader(path)
	must(err)
	defer func() { must(archive.Close()) }()
	if len(archive.File) != 1 {
		panic(fmt.Sprintf("%s contains %d files, want 1", path, len(archive.File)))
	}
	reader, err := archive.File[0].Open()
	must(err)
	defer func() { must(reader.Close()) }()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var rows [][]string
	for scanner.Scan() {
		line := []rune{}
		for _, b := range scanner.Bytes() {
			line = append(line, rune(b))
		}
		rows = append(rows, strings.Split(string(line), "|"))
	}
	must(scanner.Err())
	return rows
}

// The committed snapshot: one row per dated call-sign statement, reviewed in the PR that changes it.
var wikidataColumns = []string{"qid", "facility_id", "call_sign", "from", "to", "latitude", "longitude"}

func readWikidata(path string) map[string][]wikidataRow {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.Comment = '#'
	reader.FieldsPerRecord = len(wikidataColumns)
	records, err := reader.ReadAll()
	must(err)
	out := map[string][]wikidataRow{}
	for _, r := range records[1:] {
		row := wikidataRow{qid: r[0], facilityID: r[1], sign: r[2], from: r[3], to: r[4]}
		if lat, errLat := strconv.ParseFloat(r[5], 64); errLat == nil {
			if lon, errLon := strconv.ParseFloat(r[6], 64); errLon == nil {
				row.at = &point{lat, lon}
			}
		}
		key := row.facilityID
		if key == "" {
			key = "wd:" + row.qid
		}
		out[key] = append(out[key], row)
	}
	return out
}

func wikidataRetrieved(path string) string {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if date, ok := strings.CutPrefix(scanner.Text(), "# retrieved "); ok {
			return date
		}
	}
	panic(path + ": missing '# retrieved' line")
}

func refreshWikidata(path string) {
	request, err := http.NewRequest(http.MethodGet, "https://query.wikidata.org/sparql?query="+url.QueryEscape(wikidataQuery), nil)
	must(err)
	request.Header.Set("Accept", "text/tab-separated-values")
	request.Header.Set("User-Agent", userAgent)
	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(request)
	must(err)
	defer func() { must(response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		panic("wikidata query: " + response.Status)
	}
	// SPARQL TSV is not CSV: a term such as "WOR-TV" or "1949-..."^^<type> is never quoted as a field,
	// so encoding/csv would run one term's quote into the next line. Split on tabs.
	body, err := io.ReadAll(response.Body)
	must(err)
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	var rows [][]string
	for _, line := range lines[1:] {
		r := strings.Split(line, "\t")
		r = append(r, make([]string, max(0, 6-len(r)))...)
		qid := strings.TrimSuffix(strings.TrimPrefix(r[0], "<http://www.wikidata.org/entity/"), ">")
		lat, lon := "", ""
		if wkt := literal(r[5]); strings.HasPrefix(wkt, "Point(") {
			if parts := strings.Fields(strings.Trim(strings.TrimPrefix(wkt, "Point"), "()")); len(parts) == 2 {
				lon, lat = parts[0], parts[1]
			}
		}
		rows = append(rows, []string{qid, literal(r[1]), literal(r[2]), wikidataDate(r[3]), wikidataDate(r[4]), lat, lon})
	}
	sort.Slice(rows, func(i, j int) bool { return strings.Join(rows[i], "\t") < strings.Join(rows[j], "\t") })
	file, err := os.Create(path)
	must(err)
	defer func() { must(file.Close()) }()
	_, err = fmt.Fprintf(file, "# Dated call signs of US television stations on Wikidata (CC0); query: wikidata-call-signs.rq\n# retrieved %s\n", time.Now().UTC().Format(time.DateOnly))
	must(err)
	writer := csv.NewWriter(file)
	writer.Comma = '\t'
	must(writer.Write(wikidataColumns))
	rows = dedupe(rows)
	must(writer.WriteAll(rows))
}

func dedupe(rows [][]string) [][]string {
	out := rows[:0]
	for i, r := range rows {
		if i > 0 && strings.Join(r, "\t") == strings.Join(rows[i-1], "\t") {
			continue
		}
		out = append(out, r)
	}
	return out
}

// literal strips a SPARQL TSV term to its lexical value: "WOR-TV" and "1949-01-01T00:00:00Z"^^<...>.
func literal(term string) string {
	if i := strings.Index(term, "\"^^<"); i >= 0 {
		term = term[:i+1]
	}
	return strings.Trim(term, "\"")
}

func wikidataDate(term string) string {
	v := literal(term)
	if len(v) < len(time.DateOnly) {
		return ""
	}
	return v[:len(time.DateOnly)]
}

func download(dir, rawURL, wantSHA string) string {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	must(err)
	request.Header.Set("User-Agent", userAgent)
	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(request)
	must(err)
	defer func() { must(response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("download %s: %s", rawURL, response.Status))
	}
	path := filepath.Join(dir, filepath.Base(rawURL))
	file, err := os.Create(path)
	must(err)
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(file, hash), response.Body)
	must(err)
	must(file.Close())
	if got := hex.EncodeToString(hash.Sum(nil)); got != wantSHA {
		panic(fmt.Sprintf("download %s: sha256 %s, want %s", rawURL, got, wantSHA))
	}
	return path
}

func fileSHA(path string) string {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	must(err)
	return hex.EncodeToString(hash.Sum(nil))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
