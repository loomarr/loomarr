package scan

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// The naming round trip: every way Sonarr, Radarr, Plex, Jellyfin, Emby and Kodi name a library,
// as their own documentation sets it out, laid out with invented titles in every combination the
// apps allow, and read back. A library an app named is read as that app means it: the same title,
// year, show, season and episodes, the same ids and edition, and its extras are not titles.
//
// Each case is a namingRow, so it reads exactly as the naming table does; an outcome adds the
// grounded ids the title (or an episode's show) carries and a movie's edition:
//
//	"movie Ashgrove 2015 [imdb:tt9000001] {Final Cut}"
//	"episode Kettle Bay 2019 S1E2-4 [tvdb:90123456]"
//	"episode Evening Ledger 2024-03-14"   a dated episode, placed by its day
type roundTripCase struct {
	app  string
	from string // the template, as the app writes it
	row  namingRow
}

// Invented values a template fills in.
const (
	rtShow     = "Kettle Bay"
	rtShowYear = 2019
	rtTVDB     = "90123456"
	rtEpisode  = "Tidewater"
	rtMovie    = "Ashgrove"
	rtYear     = 2015
	rtIMDB     = "tt9000001"
	rtTMDB     = "900001"
)

// rtShowFolder is a way an app names a show's folder, and how the show reads back.
type rtShowFolder struct {
	dir, show, ids string
}

func rtShowFolders() []rtShowFolder {
	y := fmt.Sprint(rtShowYear)
	withYear := rtShow + " " + y
	tvdb := " [tvdb:" + rtTVDB + "]"
	return []rtShowFolder{
		{rtShow, rtShow, ""},
		{rtShow + " (" + y + ")", withYear, ""},
		{rtShow + " (" + y + ") {tvdb-" + rtTVDB + "}", withYear, tvdb},
		{rtShow + " (" + y + ") [tvdbid-" + rtTVDB + "]", withYear, tvdb},
		{rtShow + " [tvdbid=" + rtTVDB + "]", rtShow, tvdb},
		{rtShow + " (" + y + ") (tvdbid-" + rtTVDB + ")", withYear, tvdb},
		{strings.ReplaceAll(rtShow, " ", "."), rtShow, ""},
	}
}

// rtSeasonFolders are the ways an app names season s's folder; "" is none, every episode in the
// show's own folder.
func rtSeasonFolders(s int) []string {
	if s == 0 {
		return []string{"Specials", "Season 0", "Season 00", ""}
	}
	return []string{
		fmt.Sprintf("Season %d", s), fmt.Sprintf("Season %02d", s), fmt.Sprintf("S%02d", s),
		fmt.Sprintf("Series %d", s), fmt.Sprintf("Staffel %d", s), fmt.Sprintf("Saison %d", s),
		fmt.Sprintf("Temporada %d", s), fmt.Sprintf("Season %d [1080p]", s), "",
	}
}

// rtMarks are the ways the apps mark season s, episodes e through last (e alone when last is e),
// with the name the app documents each by.
func rtMarks(s, e, last int) map[string]string {
	se := func(e int) string { return fmt.Sprintf("S%02dE%02d", s, e) }
	x := func(e int) string { return fmt.Sprintf("%dx%02d", s, e) }
	if last == e {
		return map[string]string{
			"SxxEyy": se(e), "lower sxxeyy": strings.ToLower(se(e)), "NxNN": x(e),
			"Emby 0NxNN": fmt.Sprintf("%02dx%02d", s, e),
		}
	}
	mid := e + 1
	marks := map[string]string{
		// Sonarr's six multi-episode styles, for both its marks.
		"Extend":         se(e) + fmt.Sprintf("-%02d-%02d", mid, last),
		"Duplicate":      se(e) + " - " + se(mid) + " - " + se(last),
		"Repeat":         se(e) + fmt.Sprintf("E%02dE%02d", mid, last),
		"Scene":          se(e) + fmt.Sprintf("-E%02d-E%02d", mid, last),
		"Range":          se(e) + fmt.Sprintf("-%02d", last),
		"Prefixed Range": se(e) + fmt.Sprintf("-E%02d", last),
		"x Extend":       x(e) + fmt.Sprintf("-%02d-%02d", mid, last),
		"x Duplicate":    x(e) + " - " + x(mid) + " - " + x(last),
		"x Repeat":       x(e) + fmt.Sprintf("x%02dx%02d", mid, last),
		"x Scene":        x(e) + fmt.Sprintf("-x%02d-x%02d", mid, last),
		"x Range":        x(e) + fmt.Sprintf("-%02d", last),
		"x Prefixed":     x(e) + fmt.Sprintf("-x%02d", last),
		// Plex, Emby and the scene's own.
		"Plex lower":      strings.ToLower(se(e)) + fmt.Sprintf("-e%02d", last),
		"Emby SxxExExE":   fmt.Sprintf("S%02dxE%02dxE%02dxE%02d", s, e, mid, last),
		"Emby range":      fmt.Sprintf("%02dx%02d - %02dx%02d", s, e, s, last),
		"plus":            se(e) + fmt.Sprintf("+E%02d+E%02d", mid, last),
		"SxxEyy-SxxEzz":   se(e) + "-" + se(last),
		"spaced Prefixed": se(e) + fmt.Sprintf(" - E%02d", last),
	}
	return marks
}

// rtEpisodeNames are the ways an app names an episode's file around its mark.
func rtEpisodeNames(mark string) map[string]string {
	dotted := strings.ReplaceAll(rtShow, " ", ".")
	return map[string]string{
		"Sonarr standard":   rtShow + " - " + mark + " - " + rtEpisode + " WEBDL-1080p.mkv",
		"Sonarr bracketed":  rtShow + " - " + mark + " - " + rtEpisode + " [HDTV-720p] v2.mkv",
		"Sonarr proper":     rtShow + " - " + mark + " - " + rtEpisode + " Bluray-1080p Proper.mkv",
		"Sonarr dotted":     dotted + "." + mark + "." + rtEpisode + ".WEBDL-1080p.mkv",
		"Sonarr lower":      strings.ToLower(rtShow) + " - " + mark + " - " + strings.ToLower(rtEpisode) + ".mkv",
		"Sonarr underscore": strings.ReplaceAll(rtShow, " ", "_") + "_" + mark + "_" + rtEpisode + ".mkv",
		"Sonarr colon":      rtShow + " - " + mark + " - Low Tide - Part 1.mkv",
		"Sonarr illegal":    rtShow + " - " + mark + " - Why! Tide+Wake - Again.mkv",
		"Sonarr year":       rtShow + fmt.Sprintf(" (%d) - ", rtShowYear) + mark + " - " + rtEpisode + ".mkv",
		"Plex":              rtShow + fmt.Sprintf(" (%d) - ", rtShowYear) + strings.ToLower(mark) + " - " + rtEpisode + ".mkv",
		"mark only":         mark + ".mkv",
		"mark and title":    mark + " - " + rtEpisode + ".mkv",
		"scene":             dotted + "." + mark + ".1080p.WEB.h264-GRP.mkv",
	}
}

func roundTripCases() []roundTripCase {
	var out []roundTripCase
	add := func(app, from string, files map[string]string, nfo map[string]string) {
		out = append(out, roundTripCase{app: app, from: from, row: namingRow{
			id: fmt.Sprintf("%s-%d", app, len(out)), files: files, nfo: nfo,
		}})
	}
	one := func(rel, want string) map[string]string { return map[string]string{rel: want} }

	// Shows: every mark in every file name, each pair once, the show and season folders turning
	// through every way of writing them; then every show folder with every season folder.
	shows, i := rtShowFolders(), 0
	episode := func(sf rtShowFolder, season, name string, s, e, last int) {
		at := fmt.Sprintf("S%dE%d", s, e)
		if last != e {
			at += fmt.Sprintf("-%d", last)
		}
		rel := "TV/" + sf.dir + "/" + name
		if season != "" {
			rel = "TV/" + sf.dir + "/" + season + "/" + name
		}
		// A show whose folder gives no year takes the one its episode's name gives it.
		show := sf.show
		if y := fmt.Sprintf(" (%d) - ", rtShowYear); !strings.Contains(show, fmt.Sprint(rtShowYear)) && strings.Contains(name, y) {
			show += " " + fmt.Sprint(rtShowYear)
		}
		add("shows", sf.dir+" / "+season+" / "+name, one(rel, "episode "+show+" "+at+sf.ids), nil)
	}
	for _, ep := range []struct{ s, e, last int }{{1, 2, 2}, {2, 5, 5}, {0, 3, 3}, {1, 2, 4}, {12, 101, 103}} {
		marks := rtMarks(ep.s, ep.e, ep.last)
		for _, style := range slices.Sorted(maps.Keys(marks)) {
			if ep.e > 99 && strings.Contains(style, "x") && style != "Emby SxxExExE" {
				continue // an NxNN mark numbers episodes to 99 at most
			}
			names := rtEpisodeNames(marks[style])
			for _, nameStyle := range slices.Sorted(maps.Keys(names)) {
				seasons := rtSeasonFolders(ep.s)
				episode(shows[i%len(shows)], seasons[i%len(seasons)], names[nameStyle], ep.s, ep.e, ep.last)
				i++
			}
		}
	}
	for _, sf := range shows {
		for _, s := range []int{1, 0} {
			for _, season := range rtSeasonFolders(s) {
				episode(sf, season, rtShow+fmt.Sprintf(" - S%02dE07 - ", s)+rtEpisode+".mkv", s, 7, 7)
				episode(sf, season, fmt.Sprintf("%dx08.mkv", s), s, 8, 8)
			}
		}
	}

	// Sonarr's daily and anime series.
	for _, sf := range rtShowFolders() {
		dated := "episode " + sf.show + " 2024-03-14" + sf.ids
		for _, name := range []string{
			rtShow + " - 2024-03-14 - " + rtEpisode + " WEBDL-1080p.mkv",
			strings.ReplaceAll(rtShow, " ", ".") + ".2024.03.14." + rtEpisode + ".mkv",
			rtShow + " 2024 03 14.mkv",
			rtShow + " - 14-03-2024 - " + rtEpisode + ".mkv", // Plex's day-first
			rtShow + " - 14.03.2024.mkv",
			rtShow + " 20240314.mkv",
		} {
			for _, season := range []string{"", "Season 2024"} {
				rel := "TV/" + sf.dir + "/" + name
				if season != "" {
					rel = "TV/" + sf.dir + "/" + season + "/" + name
				}
				add("daily", name, one(rel, dated), nil)
			}
		}
		// A fansub's number across the show is the first season's episode in its folder: the two
		// count alike there.
		for _, f := range []struct{ name, want, inFirst string }{
			{rtShow + " - S01E13 - 013 - " + rtEpisode + " [HDTV-1080p].mkv", "S1E13", "S1E13"},
			{rtShow + " - S01E13-E15 - 013-015 - " + rtEpisode + ".mkv", "S1E13-15", "S1E13-15"},
			{rtShow + " - 013 - " + rtEpisode + " [HDTV-1080p].mkv", "S?E?", "S?E?"},
			{rtShow + " - 001-003 - " + rtEpisode + ".mkv", "S?E?", "S?E?"},
			{"[Kumo] " + rtShow + " - 13 [1080p][A1B2C3D4].mkv", "S?E?", "S1E13"},
			{"[Kumo][" + rtShow + "][13][1080p].mkv", "S?E?", "S1E13"},
		} {
			for _, season := range []string{"", "Season 1"} {
				rel, want := "TV/"+sf.dir+"/"+f.name, f.want
				if season != "" {
					rel, want = "TV/"+sf.dir+"/"+season+"/"+f.name, f.inFirst
				}
				add("anime", f.name, one(rel, "episode "+sf.show+" "+want+sf.ids), nil)
			}
		}
	}

	// Movies: every folder and file name Radarr, Plex, Jellyfin, Emby and Kodi write.
	type movieName struct{ folder, file, want string }
	y := fmt.Sprint(rtYear)
	base := rtMovie + " " + y
	imdb, tmdb := " [imdb:"+rtIMDB+"]", " [tmdb:"+rtTMDB+"]"
	movieNames := []movieName{
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ").mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") Bluray-1080p.mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") [Bluray-1080p][DTS 5.1][x264]-GRP.mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + "." + y + ".Bluray-1080p.mkv", base},
		{rtMovie + " (" + y + ")", strings.ToLower(rtMovie) + " (" + y + ").mkv", strings.ToLower(rtMovie) + " " + y},
		{rtMovie + " (" + y + ") {imdb-" + rtIMDB + "}", rtMovie + " (" + y + ") {imdb-" + rtIMDB + "}.mkv", base + imdb},
		{rtMovie + " (" + y + ") {imdb-" + rtIMDB + "}", rtMovie + " (" + y + ").mkv", base + imdb},
		{rtMovie + " (" + y + ") {tmdb-" + rtTMDB + "}", rtMovie + " (" + y + ") Bluray-1080p.mkv", base + tmdb},
		{rtMovie + " (" + y + ") [imdbid-" + rtIMDB + "]", rtMovie + " (" + y + ") [imdbid-" + rtIMDB + "].mkv", base + imdb},
		{rtMovie + " (" + y + ") [tmdbid-" + rtTMDB + "]", rtMovie + " (" + y + ").mkv", base + tmdb},
		{rtMovie + " (" + y + ") (imdbid-" + rtIMDB + ")", rtMovie + " (" + y + ").mkv", base + imdb},
		{rtMovie + " (" + y + ") [tmdbid=" + rtTMDB + "]", rtMovie + " (" + y + ").mkv", base + tmdb},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") {edition-Final Cut}.mkv", base + " {Final Cut}"},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") {edition-Final Cut} Bluray-1080p.mkv", base + " {Final Cut}"},
		{rtMovie + " (" + y + ") {edition-Final Cut}", rtMovie + " (" + y + ") {edition-Final Cut}.mkv", base + " {Final Cut}"},
		{"Quiet Signal, The (" + y + ")", "Quiet Signal, The (" + y + ") Bluray-1080p.mkv", "The Quiet Signal " + y},
		{"Low Orchard - Return (" + y + ")", "Low Orchard - Return (" + y + ").mkv", "Low Orchard - Return " + y},
		{"", rtMovie + " (" + y + ").mkv", base},
		{"", rtMovie + "." + y + ".1080p.BluRay.x264-GRP.mkv", base},
		{"", rtMovie + " (" + y + ") {imdb-" + rtIMDB + "}.mkv", base + imdb},
		// Versions, as Plex, Jellyfin and Emby label them.
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") - 1080p.mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") - 4K HDR.mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") - Final Cut.mkv", base},
		{rtMovie + " (" + y + ")", rtMovie + " (" + y + ") - [1080p].mkv", base},
		// A disc image is listed for the maintainer, not imported (Emby's ".dvd.iso").
		{rtMovie + " (" + y + ")", rtMovie + ".dvd.iso", "problem"},
	}
	for _, m := range movieNames {
		for _, top := range []string{"Movies/", "Movies/A/", "Movies/Ashgrove Saga Collection/"} {
			rel := top + m.file
			if m.folder != "" {
				rel = top + m.folder + "/" + m.file
			}
			want := "movie " + m.want
			if m.want == "problem" {
				want = m.want
			}
			add("movies", m.folder+" / "+m.file, one(rel, want), nil)
		}
	}

	// Parts of one movie (stacking): each part reads as the movie.
	folder := "Movies/" + rtMovie + " (" + y + ")/"
	for _, parts := range [][2]string{
		{" - pt1", " - pt2"}, {" - part1", " - part2"}, {" - cd1", " - cd2"}, {" - disc1", " - disc2"},
		{" - disk1", " - disk2"}, {" - dvd1", " - dvd2"}, {" cd1", " cd2"}, {".part1", ".part2"},
		{"-cd1", "-cd2"}, {" (cd 1)", " (cd 2)"}, {" [part 1]", " [part 2]"}, {"cd1", "cd2"}, {" - pt A", " - pt B"},
		{" - parta", " - partb"},
	} {
		files := map[string]string{}
		for _, part := range parts {
			files[folder+rtMovie+" ("+y+")"+part+".mkv"] = "movie " + base
		}
		add("stacking", parts[0], files, nil)
	}

	// Extras: never titles, beside a movie or in a show, by folder or by name.
	movieFile := folder + rtMovie + " (" + y + ").mkv"
	for _, sub := range []string{
		"extras", "Extras", "featurettes", "behind the scenes", "deleted scenes", "interviews", "scenes",
		"shorts", "trailers", "other", "samples", "sample", "clips", "backdrops", "theme-music",
		"Special Features", "Bonus Features",
	} {
		add("extras", sub+"/", map[string]string{movieFile: "movie " + base, folder + sub + "/Making Of.mkv": "skip"}, nil)
	}
	for _, suffix := range []string{
		"-trailer", "-featurette", "-behindthescenes", "-deleted", "-deletedscene", "-interview", "-scene",
		"-short", "-sample", "-other", "-clip", "-extra", ".trailer", "_trailer", "-trailer2", " - trailer",
		".sample", "_sample", " - sample", "-sample3",
	} {
		add("extras", suffix, map[string]string{
			movieFile: "movie " + base, folder + rtMovie + " (" + y + ")" + suffix + ".mkv": "skip",
		}, nil)
	}
	for _, name := range []string{"trailer.mkv", "sample.mkv", "trailer2.mkv", "Sample-asg.mkv", "ashgrove.sample.mkv"} {
		add("extras", name, map[string]string{movieFile: "movie " + base, folder + name: "skip"}, nil)
	}
	showDir := "TV/" + rtShow + "/"
	for _, extra := range []string{
		"Extras/Making Of.mkv", "Season 1/Extras/Making Of.mkv", "Season 1/Featurettes/Cast.mkv",
		"Season 1/Specials/Behind the Waves.mkv", "Season 1/Kettle Bay - S01E02-trailer.mkv",
		"Season 1/Kettle Bay - S01E02 - Tidewater-deleted.mkv", "Backdrops/loop.mkv", "theme-music/theme.mkv",
		"Season 1/sample.mkv", "Season 1/Kettle Bay - NCOP1.mkv", "Season 1/Kettle Bay - NCED2 [1080p].mkv",
		"[Kumo] Kettle Bay - OP [1080p].mkv",
	} {
		add("extras", extra, map[string]string{
			showDir + "Season 1/Kettle Bay - S01E02 - Tidewater.mkv": "episode " + rtShow + " S1E2",
			showDir + extra: "skip",
		}, nil)
	}

	// Kodi's own: episodes Kodi's scrapers read beyond the apps' marks.
	for _, f := range []struct{ name, want string }{
		{"Kettle Bay 104.mkv", "S1E4"}, {"Kettle Bay ep04.mkv", "S1E4"}, {"Kettle Bay S01E02a.mkv", "S1E2"},
		{"Kettle Bay S01E02.5.mkv", "S1E2"}, {"Kettle Bay - S01E02 - pt1.mkv", "S1E2"},
		{"Kettle Bay Season 1 Episode 2.mkv", "S1E2"}, {"Kettle.Bay.s1.e2.mkv", "S1E2"},
		{"Kettle Bay - 1of6.mkv", "S1E1"},
	} {
		add("kodi", f.name, one(showDir+f.name, "episode "+rtShow+" "+f.want), nil)
	}
	// Emby's per-episode folders.
	add("emby", "Season 2/S02E05/", one(showDir+"Season 2/S02E05/Kettle Bay - S02E05.mkv", "episode "+rtShow+" S2E5"), nil)
	add("emby", "Season 2/Kettle Bay S02E05/",
		one(showDir+"Season 2/Kettle Bay S02E05/video.mkv", "episode "+rtShow+" S2E5"), nil)
	add("emby", "S02E05/ in the show", one(showDir+"S02E05/Kettle Bay - S02E05.mkv", "episode "+rtShow+" S2E5"), nil)
	return out
}

func TestNamingRoundTrip(t *testing.T) {
	cases := roundTripCases()
	var right, files atomic.Int32
	var wrong []string
	var mu = make(chan struct{}, 1)
	t.Run("cases", func(t *testing.T) {
		for _, c := range cases {
			t.Run(c.row.id, func(t *testing.T) {
				t.Parallel()
				got := roundTripOutcomes(t, c.row)
				for rel, want := range c.row.files {
					files.Add(1)
					if got[rel] == want {
						right.Add(1)
						continue
					}
					mu <- struct{}{}
					wrong = append(wrong, fmt.Sprintf("%s [%s]\n\t%s\n\t= %q, want %q", c.app, c.from, rel, got[rel], want))
					<-mu
				}
			})
		}
	})
	slices.Sort(wrong)
	for _, w := range wrong {
		t.Error(w)
	}
	t.Logf("naming round trip: %d of %d files right", right.Load(), files.Load())
}

// roundTripOutcomes is what each of a row's files became, with the ids its title (or an episode's
// show) carries and a movie's edition.
func roundTripOutcomes(t *testing.T, row namingRow) map[string]string {
	t.Helper()
	return namingOutcomesWith(t, row, true)
}
