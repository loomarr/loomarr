package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Dated episodes: a daily or weekly show's episode named by the day it aired, "2013-02-15", and a
// TV recording's or a camera's video named by when it was made, which is no show's.

// airDateIn is the day a file's name says its episode aired, YYYY-MM-DD; "" for none. A day that
// never was, such as "2024-02-30" or a number that only looks like one, is none.
func airDateIn(name string) string { return airDateOrdered(name, false) }

// airDateOrdered is airDateIn with a day written "05-06-2023" read month first when monthFirst: its
// folder's other days say America's order ("03-14-2023" beside it, folderNotes.MonthFirst).
func airDateOrdered(name string, monthFirst bool) string {
	pad := func(s string) string {
		if len(s) == 1 {
			return "0" + s
		}
		return s
	}
	for _, m := range airDateMark.FindAllStringSubmatch(name, -1) {
		day, same := m[1]+"-"+pad(m[3])+"-"+pad(m[5]), m[2] == m[4]
		if m[1] != "" && (len(m[3]) == 1 || len(m[5]) == 1) && (m[2] == "" || m[2] == " ") {
			continue
		}
		if m[1] == "" {
			day, same = m[10]+"-"+m[8]+"-"+m[6], m[7] == m[9]
			// Day first as Plex writes it; a month that can't be one is America's month first,
			// "03-14-2024".
			if atoi(m[8]) > 12 || monthFirst && atoi(m[6]) <= 12 {
				day = m[10] + "-" + m[6] + "-" + m[8]
			}
		}
		if _, err := time.Parse(time.DateOnly, day); err == nil && same {
			return day
		}
	}
	return ""
}

// monthFirstDays reads folderNotes.MonthFirst from a folder's videos' names.
func monthFirstDays(names []string) bool {
	american, european := false, false
	for _, name := range names {
		for _, m := range airDateMark.FindAllStringSubmatch(stem(name), -1) {
			if m[1] != "" {
				continue
			}
			american = american || atoi(m[8]) > 12
			european = european || atoi(m[6]) > 12
		}
	}
	return american && !european
}

// datedEpisode says whether a file its name dates is a show's episode: one in a season folder, or
// one whose name before its day is its show's ("Kettle Bay/Kettle Bay 2024-03-14.mkv"). A day alone
// does not make a show: "Home Videos/2019-06-01 Beach Day.mp4" is a day's video, and
// "Concerts/Ferrow Band - Live 2019.10.05.mkv" a film of one.
func datedEpisode(file File) bool {
	if seasonDirOf(path.Dir(file.Rel)) != "" {
		return true
	}
	name, dir := plainStem(path.Base(file.Rel)), path.Dir(file.Rel)
	before, _, _ := searchQuery(datedShow(name), 0)
	// A name that is its day alone, "Nightly Ledger/2024-03-14.mkv", is the episode of the show
	// whose folder it is in; a catch-all's, "Home Videos/2019-06-01.mp4", is a day's video.
	if before == "" && dir != "." && airDateMark.ReplaceAllString(name, "") == "" &&
		(underShows(shelved(file, dir)) || file.Notes.Episodes) && !shelfAt(dir) {
		return true
	}
	if dateFirst(name) && !underShows(shelved(file, file.Rel)) {
		return false
	}
	if before == "" || cameraPrefixes[strings.ToLower(before)] {
		return false
	}
	if recording.MatchString(name) || recordedAt.MatchString(name) {
		// A recorded film is named as a guide lists one, with its year in brackets: "Iron Spire
		// (2011)_Film4_2022_05_01_21_00_00".
		return !bracketYear.MatchString(datedShow(name))
	}
	// So is one recorded loose, "Recorded TV/Grey Harbour (1958) 2022-01-01.ts": in a show's own
	// folder, "Ledger (2019) - 2024-03-18" is Sonarr's daily episode of the 2019 show.
	if recordedFilm(name, dir) {
		return false
	}
	// Loose at the top or in a catch-all ("Downloads/"), a release's name is its own show's:
	// "Evening.Ledger.2024.03.18.720p.WEB"; in a shows' catch-all ("Recordings/", "TV/") any dated
	// name is: "Harbour Watch 2022-01-05 21-00".
	if dir == "." || shelfAt(dir) {
		return showsFolderNames[folderKey(path.Base(dir))] || datedRelease(name)
	}
	show, _, _ := searchQuery(showFolderName(path.Base(seriesFolderOf(file))), 0)
	return relatedName(before, show)
}

// recordedFilm says whether a dated name loose at the top or in a catch-all is a film's recording:
// its year in brackets ends what it names before its day, "Grey Harbour (1958) 2022-01-01".
func recordedFilm(name, dir string) bool {
	return (dir == "." || shelfAt(dir)) && recordedTitle(name) != ""
}

// recordedTitle is a recorded film's name and year from its recording's, "" for none: the name
// before its day ends with its year in brackets, or with its year and the channel it was on,
// "Hallow Point (2009) - Channel 9 - 2023-12-24".
func recordedTitle(name string) string {
	before := strings.TrimSpace(datedShow(name))
	if m := yearThenChannel.FindStringIndex(before); m != nil {
		return strings.TrimSpace(before[:m[0]+len("(2009)")])
	}
	return ""
}

// yearThenChannel matches a film's year in brackets last, or before a channel's name after a dash:
// "(2009)", "(2009) - Channel 9".
var yearThenChannel = regexp.MustCompile(`\((?:19|20)\d{2}\)(?:\s*-\s*[^()]+)?$`)

// recording matches a TV recorder's name for what it recorded, Windows Media Center's: its show,
// its channel, and the day and time it began, "Larchmont Road_ITV_2022_11_02_20_00_00". Each is
// that show's episode of that day.
var recording = regexp.MustCompile(`^(.+?)_[A-Za-z0-9 +]+_((?:19|20)\d{2})_\d{2}_\d{2}_\d{2}_\d{2}_\d{2}$`)

// recordedAt matches a DVR's name for what it recorded, with the day and the time it began after
// the show and the episode's title: "Wexcombe - Hedge (2023-01-02 21-00)". A film's name never
// carries the minute it began.
var recordedAt = regexp.MustCompile(`^(.+?)(?: - .+?)? \((?:19|20)\d{2}-\d{2}-\d{2}[ _T]\d{2}[-.:]\d{2}(?:[-.:]\d{2})?\)$`)

// cameraPrefixes are what a phone or camera names its videos by before their day: "VID_20190601_
// 123456", "PXL_20230704_…". A home video, never a show's episode.
var cameraPrefixes = map[string]bool{
	"vid": true, "img": true, "pxl": true, "mvi": true, "dsc": true, "mov": true, "dji": true,
	"gopr": true, "video": true, "screen recording": true, "signal": true, "whatsapp video": true,
}

// datedRelease says whether a dated name is a release's, written as a downloader leaves it: with
// dots for spaces, or with a release's words after its day. So is one that ends with its whole day,
// "News Hour 2024 03 14": no film's title ends with a month and a day.
func datedRelease(name string) bool {
	if !strings.Contains(name, " ") || endingDay.MatchString(name) {
		return true
	}
	return slices.ContainsFunc(strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune(" ._[]()-", r) }), releaseMark.MatchString)
}

// endingDay matches a name's whole day at its end, its month and day each two digits apart by
// one separator: "2024 03 14", "2024-03-14", "2024.03.14".
var endingDay = regexp.MustCompile(`\D(?:19|20)\d{2}(?: \d{2} |\.\d{2}\.|_\d{2}_|-\d{2}-)\d{2}$`)

// datedShow is the show a dated name names before its day: "Evening.Ledger.2024.03.18.720p" is
// "Evening Ledger"; "" for none.
func datedShow(name string) string {
	if m := recording.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := recordedAt.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	m := airDateMark.FindStringSubmatchIndex(name)
	if m == nil {
		return ""
	}
	start := m[2] // the year first, else the day first
	if start < 0 {
		start = m[12]
	}
	show := strings.NewReplacer(".", " ", "_", " ").Replace(leadingTags.ReplaceAllString(name[:start], ""))
	// A day in brackets, "Kite Hill (2023-01-30)", leaves its bracket behind.
	if show = strings.Trim(strings.Join(strings.Fields(show), " "), " -(["); show != "" {
		return show
	}
	// A recorder that names its recordings by their day first names the show after it, and the
	// time and channel it began: "2023-03-08 Ironfield", "20230308_2100_Channel5_Ironmoor Lane",
	// "20230309 Ironfield - The Well" (dateFirst).
	after := strings.NewReplacer(".", " ", "_", " ").Replace(name[m[1]:])
	after = afterDayNotes.ReplaceAllString(strings.TrimLeft(after, " -"), "")
	if title, _, found := strings.Cut(after, " - "); found {
		after = title
	}
	return strings.Trim(strings.Join(strings.Fields(after), " "), " -")
}

// afterDayNotes matches the time and channel a recorder names after a recording's day: "2100 ",
// "21-00 Channel5 ".
var afterDayNotes = regexp.MustCompile(`(?i)^(?:\d{2}[-:.]?\d{2}(?:[-:.]?\d{2})?[ -]+)?(?:channel[ ]?\d+[ -]+)?`)

// dateFirst says whether a name starts with its day: "2023-03-08 Ironfield". Only a shows' shelf's
// is a show's episode; elsewhere it is a day's video ("Home Videos/2019-06-01 Beach Day.mp4").
func dateFirst(name string) bool {
	at := airDateMark.FindStringIndex(name)
	return at != nil && strings.Trim(leadingTags.ReplaceAllString(name[:at[0]], ""), " ._-") == ""
}
