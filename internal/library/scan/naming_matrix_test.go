package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Naming rows: how messy libraries are laid out, and what each file must become. Most
// households' libraries are far less tidy than an arr-managed one, so the rows come from how
// downloads, rips and hand-sorted folders look in general (scene names, everything loose, mixed
// movies and TV, no sidecars), each written with invented titles.
//
// Each file's outcome reads:
//
//	"movie <title> <year>"         the title and year the catalog is searched for ("-" for no year)
//	"episode <show> S<n>E<n>[-<n>]" the show it files under, as searched for, and where it sits
//	"episode <show> S?E?"          an episode of the show the scanner could not place
//	"skip"                         not a title: an extra, a sample, junk
//	"problem"                      listed for the maintainer: a disc structure, an image
//
// A file the scanner gets wrong today is in namingToday with what it does instead. It still runs,
// and fails when the scanner changes either way, so a fix flips it to right and a regression is
// caught. The scoreboard says how many files are right.
type namingRow struct {
	id    string            // the pattern, as the catalogue numbers it (A1, E5, ...)
	files map[string]string // a file, relative to the root, and what it must become
	nfo   map[string]string // sidecars: a path relative to the root and its content
}

// namingToday is what the scanner makes today of each file it gets wrong, keyed "<row id> <file>".
var namingToday = map[string]string{
	"A7 Movies/[TrackerNet] Ferrow Lane (2019).mkv":                   "movie TrackerNet Ferrow Lane 2019",
	"A12 Movies/Marrow Point (1920x1080).mkv":                         "movie Marrow Point 1920x1080 -",
	"C9 Movies/(2015) Ashgrove.mkv":                                   "movie 2015 Ashgrove -",
	"E6 TV/Kettle Bay/Kettle.Bay.102.mkv":                             "movie Kettle Bay 102 -",
	"E8 TV/Kettle Bay/Kettle Bay Season 1 Episode 2.mkv":              "movie Kettle Bay Season 1 Episode 2 -",
	"E9 TV/Kettle Bay/Season 1/Kettle Bay Ep02.mkv":                   "episode Kettle Bay S?E?",
	"E10 TV/Kettle Bay/Season 2/05 - Tidewater.mkv":                   "episode Kettle Bay S?E?",
	"E13 TV/Ferrow Lane/Ferrow.Lane.2of6.mkv":                         "movie Ferrow Lane 2of6 -",
	"G1 TV/[Kumo] Sora no Kettle - 07 [1080p][A1B2C3D4].mkv":          "movie Kumo Sora no Kettle - 07 -",
	"G11 TV/Sora no Kettle/[Kumo] Sora no Kettle - NCOP1 [1080p].mkv": "movie Kumo Sora no Kettle - NCOP1 -",
	"I2 TV/Evening Ledger/Evening Ledger 20240314.mkv":                "movie Evening Ledger 20240314 -",
	"I4 TV/Evening Ledger/Evening Ledger 14.03.2024.mkv":              "movie Evening Ledger 14.03.2024 -",
	"K3 TV/Kettle Bay/1/Kettle Bay S01E01.mkv":                        "episode 1 S1E1",
	"P3 Movies/Ashgrove.2015-GRP/grp-asg.mkv":                         "movie grp-asg -",
	"P9 Downloads/Complete/Kettle.Bay.S01E02.mkv":                     "episode Complete S1E2",
	"P9 Downloads/Incomplete/Kettle.Bay.S01E03.mkv":                   "episode Incomplete S1E3",
	"R1 Media/Kettle.Bay.S01E02.mkv":                                  "episode Media S1E2",
	"R4 TV/Kettle Bay/kb 101.avi":                                     "movie kb 101 -",
	"R4 TV/Kettle Bay/kb 201.avi":                                     "movie kb 201 -",
	"R5 TV/All Shows/Kettle.Bay.S01E01.mkv":                           "episode All Shows S1E1",
	"R5 TV/All Shows/Ferrow.Lane.S02E03.mkv":                          "episode All Shows S2E3",
	"S4 Movies/ＦＥＲＲＯＷ　ＬＡＮＥ（２０１９）.mkv":                                 "movie ＦＥＲＲＯＷ ＬＡＮＥ（２０１９） -",
	"U3 TV/Kettle Bay/Kettle Bay - Tidewater.mkv":                     "movie Kettle Bay - Tidewater -",
}

var namingMatrix = []namingRow{
	// A. Release and scene words around the title.
	{id: "A1", files: map[string]string{"Movies/Ferrow.Lane.2019.1080p.mkv": "movie Ferrow Lane 2019"}},
	{id: "A2", files: map[string]string{"Movies/Kettle.Bay.2021.AMZN.WEB-DL.2160p.mkv": "movie Kettle Bay 2021"}},
	{id: "A3", files: map[string]string{"Movies/Ashgrove.2015.720p.BluRay.x265.10bit.mkv": "movie Ashgrove 2015"}},
	{id: "A5", files: map[string]string{"Movies/Low.Orchard.2017.BluRay.DTS-HD.MA.7.1.mkv": "movie Low Orchard 2017"}},
	{id: "A6", files: map[string]string{"Movies/Tinder.Hollow.2012.1080p.BluRay.x264-WRDS.mkv": "movie Tinder Hollow 2012"}},
	{id: "A7", files: map[string]string{"Movies/[TrackerNet] Ferrow Lane (2019).mkv": "movie Ferrow Lane 2019"}},
	{id: "A9", files: map[string]string{"Movies/Der.Stille.Hafen.2011.German.DL.1080p.mkv": "movie Der Stille Hafen 2011"}},
	{id: "A10", files: map[string]string{"Movies/Ferrow Lane (2019) [1080p] [BluRay] [5.1].mp4": "movie Ferrow Lane 2019"}},
	{id: "A12", files: map[string]string{"Movies/Marrow Point (1920x1080).mkv": "movie Marrow Point -"}},
	{id: "A14", files: map[string]string{"Movies/Tinder.Hollow.2012.1080p.BluRay-WRDS/a8f3c91e0b.mkv": "movie Tinder Hollow 2012"}},
	{id: "A15", files: map[string]string{"Downloads/Kettle.Bay.S02.1080p.WEB/kb.s02e04.mkv": "episode Kettle Bay S2E4"}},

	// B. Separators and casing.
	{id: "B1", files: map[string]string{"Movies/The.Quiet.Signal.2008.mkv": "movie The Quiet Signal 2008"}},
	{id: "B2", files: map[string]string{
		"Movies/the_quiet_signal_2008.avi":                "movie the quiet signal 2008",
		"TV/Kettle Bay/Kettle_Bay_-_1x05_-_Tidewater.avi": "episode Kettle Bay S1E5",
	}},
	{id: "B3", files: map[string]string{"TV/Kettle Bay/Kettle Bay.S01E05_Tidewater-720p.mkv": "episode Kettle Bay S1E5"}},
	{id: "B4", files: map[string]string{"Movies/FERROW LANE 2019.MKV": "movie FERROW LANE 2019"}},
	{id: "B8", files: map[string]string{"Movies/Ferrow.Lane.2019.mkv.mkv": "movie Ferrow Lane 2019"}},

	// C. Years.
	{id: "C1", files: map[string]string{"Movies/Ashgrove (2015).mkv": "movie Ashgrove 2015"}},
	{id: "C2", files: map[string]string{"Movies/Ashgrove [2015].mkv": "movie Ashgrove 2015"}},
	{id: "C3", files: map[string]string{"Movies/Ashgrove 2015.mkv": "movie Ashgrove 2015"}},
	{id: "C4", files: map[string]string{"Movies/Ashgrove (2015)/Ashgrove.mkv": "movie Ashgrove 2015"}},
	{id: "C6", files: map[string]string{"Movies/Ashgrove.mkv": "movie Ashgrove -"}},
	{id: "C9", files: map[string]string{"Movies/(2015) Ashgrove.mkv": "movie Ashgrove 2015"}},
	{id: "C10", files: map[string]string{"Movies/Kettle.Bay.1985.2003.mkv": "movie Kettle Bay 1985 2003"}},
	{id: "C12", files: map[string]string{
		"TV/Kettle Bay (2019)/Season 1/Kettle Bay (2019) S01E01.mkv": "episode Kettle Bay 2019 S1E1",
		"TV/Kettle Bay/Season 1/Kettle Bay S01E01.mkv":               "episode Kettle Bay S1E1",
	}},

	// D. Hard titles.
	{id: "D1", files: map[string]string{"Movies/1987 (2014).mkv": "movie 1987 2014"}},
	{id: "D3", files: map[string]string{
		"Movies/81.Days.2010.mkv": "movie 81 Days 2010",
		"Movies/9 (2019).mkv":     "movie 9 2019",
	}},
	{id: "D5", files: map[string]string{
		"Movies/Marrow.Point.2.2013.mkv": "movie Marrow Point 2 2013",
		"Movies/Marrow Point (2010).mkv": "movie Marrow Point 2010",
	}},
	{id: "D8", files: map[string]string{
		"Movies/The Sample Room (2004).mkv":        "movie The Sample Room 2004",
		"Movies/Trailer Park Kings (2011).mkv":     "movie Trailer Park Kings 2011",
		"Movies/Sample (2016).mkv":                 "movie Sample 2016",
		"Movies/Trailer (2019)/Trailer (2019).mkv": "movie Trailer 2019",
		"Movies/Sampler.2018.mkv":                  "movie Sampler 2018",
	}},
	{id: "D9", files: map[string]string{"Movies/Ward 101 (2012).mkv": "movie Ward 101 2012"}},

	// E. One episode.
	{id: "E1", files: map[string]string{"TV/Kettle Bay/Season 1/Kettle.Bay.S01E02.Tidewater.720p.mkv": "episode Kettle Bay S1E2"}},
	{id: "E2", files: map[string]string{"TV/Kettle Bay/kettle bay s1e2.mkv": "episode Kettle Bay S1E2"}},
	{id: "E3", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01.E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "E4", files: map[string]string{"TV/Kettle Bay/Kettle Bay S03E1105.mkv": "episode Kettle Bay S3E1105"}},
	{id: "E5", files: map[string]string{"TV/Kettle Bay/Kettle Bay - 01x02 - Tidewater.avi": "episode Kettle Bay S1E2"}},
	{id: "E6", files: map[string]string{"TV/Kettle Bay/Kettle.Bay.102.mkv": "episode Kettle Bay S1E2"}},
	{id: "E8", files: map[string]string{"TV/Kettle Bay/Kettle Bay Season 1 Episode 2.mkv": "episode Kettle Bay S1E2"}},
	{id: "E9", files: map[string]string{"TV/Kettle Bay/Season 1/Kettle Bay Ep02.mkv": "episode Kettle Bay S1E2"}},
	{id: "E10", files: map[string]string{"TV/Kettle Bay/Season 2/05 - Tidewater.mkv": "episode Kettle Bay S2E5"}},
	{id: "E13", files: map[string]string{"TV/Ferrow Lane/Ferrow.Lane.2of6.mkv": "episode Ferrow Lane S1E2"}},
	{id: "E14", files: map[string]string{"TV/Kettle Bay/Season 2014/Kettle Bay S2014E07.mkv": "episode Kettle Bay S2014E7"}},
	{id: "E15", files: map[string]string{"TV/Kettle Bay/Season 1/S01E02 - Tidewater.mkv": "episode Kettle Bay S1E2"}},
	{id: "E16", files: map[string]string{"TV/Kettle Bay/Season 1/KB.S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "E17", files: map[string]string{"TV/Kettle Bay/Kettle.Bay.S01E07.5.1.mkv": "episode Kettle Bay S1E7"}},

	// F. Several episodes in one file.
	{id: "F1", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01E01-E03.mkv": "episode Kettle Bay S1E1-3"}},
	{id: "F1b", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01E01-03.mkv": "episode Kettle Bay S1E1-3"}},
	{id: "F2", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01E01E02.mkv": "episode Kettle Bay S1E1-2"}},
	{id: "F3", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01E01-S01E02.mkv": "episode Kettle Bay S1E1-2"}},
	{id: "F4", files: map[string]string{"TV/Kettle Bay/Kettle Bay 1x01-1x02.mkv": "episode Kettle Bay S1E1-2"}},
	{id: "F8", files: map[string]string{"TV/Kettle Bay/Kettle Bay S01E01+E02.mkv": "episode Kettle Bay S1E1-2"}},
	// What follows one episode's mark is not a last episode: a resolution, a year in its title.
	{id: "F9", files: map[string]string{
		"TV/Kettle Bay/Kettle.Bay.S01E02-1080p.mkv":   "episode Kettle Bay S1E2",
		"TV/Kettle Bay/Kettle Bay S01E03 - 1984.mkv":  "episode Kettle Bay S1E3",
		"TV/Kettle Bay/Kettle Bay 1x04-720p.mkv":      "episode Kettle Bay S1E4",
		"TV/Kettle Bay/Kettle Bay S01E05-E04.mkv":     "episode Kettle Bay S1E5",
		"TV/Kettle Bay/Kettle.Bay.S01E06.Endgame.mkv": "episode Kettle Bay S1E6",
	}},

	// G. Absolute (anime) numbering: the show is placed, its episode by the catalog later.
	{id: "G1", files: map[string]string{"TV/[Kumo] Sora no Kettle - 07 [1080p][A1B2C3D4].mkv": "episode Sora no Kettle S?E?"}},
	{id: "G11", files: map[string]string{"TV/Sora no Kettle/[Kumo] Sora no Kettle - NCOP1 [1080p].mkv": "skip"}},

	// H. Specials.
	{id: "H1", files: map[string]string{"TV/Kettle Bay/Season 0/Kettle Bay S00E01.mkv": "episode Kettle Bay S0E1"}},
	{id: "H2", files: map[string]string{"TV/Kettle Bay/Specials/Kettle Bay S00E03.mkv": "episode Kettle Bay S0E3"}},
	{id: "H3", files: map[string]string{"TV/Kettle Bay/Specials/Kettle Bay - Holiday Special.mkv": "episode Kettle Bay S?E?"}},
	{id: "H3-nfo",
		files: map[string]string{"TV/Kettle Bay/Specials/Kettle Bay - The Movie.mkv": "episode Kettle Bay S?E?"},
		nfo:   map[string]string{"TV/Kettle Bay/Specials/Kettle Bay - The Movie.nfo": "<episodedetails><title>Kettle Bay - The Movie</title><season>0</season></episodedetails>"}},

	// I. Shows named by the day they aired.
	{id: "I1", files: map[string]string{"TV/Evening Ledger/Evening.Ledger.2024-03-14.Guest.Name.mkv": "episode Evening Ledger S?E?"}},
	{id: "I2", files: map[string]string{"TV/Evening Ledger/Evening Ledger 20240314.mkv": "episode Evening Ledger S?E?"}},
	{id: "I4", files: map[string]string{"TV/Evening Ledger/Evening Ledger 14.03.2024.mkv": "episode Evening Ledger S?E?"}},
	{id: "I10", files: map[string]string{"TV/Evening Ledger/Season 2024/Evening Ledger - 2024-03-14 - Guest.mkv": "episode Evening Ledger S?E?"}},

	// J. Show folders.
	{id: "J2", files: map[string]string{"TV/Kettle Bay (2019)/Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay 2019 S1E1"}},
	{id: "J3", files: map[string]string{"TV/Kettle Bay (2019) {tvdb-90123456}/Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay 2019 S1E1"}},
	{id: "J5", files: map[string]string{"TV/Kids/Cartoons/Kettle Bay/Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "J6", files: map[string]string{"TV/Kettle.Bay.S02.1080p.WEB-DL-GRP/Kettle.Bay.S02E01.mkv": "episode Kettle Bay S2E1"}},
	{id: "J7", files: map[string]string{"TV/Kettle Bay Complete Series (2010-2015) [1080p]/Season 3/Kettle Bay S03E01.mkv": "episode Kettle Bay S3E1"}},
	{id: "J9", files: map[string]string{
		"TV/Kettle Bay/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Kettle Bay S02E01.mkv": "episode Kettle Bay S2E1",
	}},

	// K. Season folders.
	{id: "K1", files: map[string]string{"TV/Kettle Bay/season1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K2", files: map[string]string{"TV/Kettle Bay/S01/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K3", files: map[string]string{"TV/Kettle Bay/1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K6", files: map[string]string{"TV/Kettle Bay/Series 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K7", files: map[string]string{"TV/Kettle Bay/Staffel 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K8", files: map[string]string{"TV/Kettle Bay/Kettle Bay Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "K9", files: map[string]string{"TV/Kettle Bay/Season 2 [1080p]/Kettle Bay S02E01.mkv": "episode Kettle Bay S2E1"}},
	// A folder named for a show's season with no show folder above it is the show's own folder, named
	// by its words before the season.
	{id: "K10", files: map[string]string{"Ferrow Lane Season 2/Ferrow Lane S02E01.mkv": "episode Ferrow Lane S2E1"}},
	// A show whose own name has a season-like word keeps it.
	{id: "K11", files: map[string]string{
		"TV/Season of Ash/Season 1/Season of Ash S01E01.mkv": "episode Season of Ash S1E1",
		"TV/The Series Club/The Series Club S01E01.mkv":      "episode The Series Club S1E1",
	}},

	// L. Movie layouts.
	{id: "L1", files: map[string]string{"Movies/Ashgrove (2015)/Ashgrove (2015).mkv": "movie Ashgrove 2015"}},
	{id: "L3", files: map[string]string{"Movies/Ashgrove (2015)/asg-1080p.mkv": "movie Ashgrove 2015"}},
	{id: "L5", files: map[string]string{"Movies/New Folder/Ferrow Lane (2019).mkv": "movie Ferrow Lane 2019"}},
	{id: "L6", files: map[string]string{
		"Movies/Marrow Point Collection/Marrow Point (2010).mkv":   "movie Marrow Point 2010",
		"Movies/Marrow Point Collection/Marrow Point 2 (2013).mkv": "movie Marrow Point 2 2013",
	}},
	{id: "L7", files: map[string]string{"Movies/Marrow Point Trilogy/Marrow Point 1 (2010)/Marrow Point 1 (2010).mkv": "movie Marrow Point 1 2010"}},
	{id: "L8", files: map[string]string{"Movies/Ashgrove (2015) {tmdb-90012345}/Ashgrove (2015).mkv": "movie Ashgrove 2015"}},

	// N. Editions.
	{id: "N1", files: map[string]string{"Movies/Ashgrove.2015.Directors.Cut.1080p.mkv": "movie Ashgrove 2015"}},
	{id: "N3", files: map[string]string{"Movies/Ashgrove (2015) {edition-Director's Cut}.mkv": "movie Ashgrove 2015"}},
	{id: "N6", files: map[string]string{"Movies/Ashgrove.2015.3D.HSBS.1080p.mkv": "movie Ashgrove 2015"}},

	// O. Extras.
	{id: "O1", files: map[string]string{
		"Movies/Ashgrove (2015)/Ashgrove (2015).mkv":         "movie Ashgrove 2015",
		"Movies/Ashgrove (2015)/Ashgrove (2015)-trailer.mkv": "skip",
	}},
	{id: "O2", files: map[string]string{
		"Movies/Ashgrove (2015)/Ashgrove (2015).mkv": "movie Ashgrove 2015",
		"Movies/Ashgrove (2015)/trailer.mkv":         "skip",
	}},
	{id: "O4", files: map[string]string{
		"Movies/Ashgrove (2015)/Ashgrove (2015).mkv":             "movie Ashgrove 2015",
		"Movies/Ashgrove (2015)/Featurettes/Making Ashgrove.mkv": "skip",
		"Movies/Ashgrove (2015)/Special Features/Gag Reel.mkv":   "skip",
	}},
	{id: "O5", files: map[string]string{
		"TV/Kettle Bay/Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Season 1/Extras/Gag Reel.mkv":   "skip",
	}},
	{id: "O6", files: map[string]string{
		"Movies/Ashgrove (2015).mkv":         "movie Ashgrove 2015",
		"Movies/Ashgrove (2015)-trailer.mkv": "skip",
	}},
	{id: "O10", files: map[string]string{"Movies/Ashgrove (2015)/trailerthings.mkv": "movie Ashgrove 2015"}},

	// P. Samples and junk.
	{id: "P1", files: map[string]string{
		"Movies/Ashgrove.2015.1080p-GRP/Ashgrove.2015.1080p-GRP.mkv": "movie Ashgrove 2015",
		"Movies/Ashgrove.2015.1080p-GRP/sample.mkv":                  "skip",
	}},
	{id: "P2", files: map[string]string{"Movies/ashgrove.2015-sample.mkv": "skip"}},
	{id: "P3", files: map[string]string{
		"Movies/Ashgrove.2015-GRP/grp-asg.mkv":        "movie Ashgrove 2015",
		"Movies/Ashgrove.2015-GRP/Sample/grp-asg.mkv": "skip",
	}},
	{id: "P5", files: map[string]string{"Movies/Ashgrove.2015.mkv.part": "skip"}},
	{id: "P9", files: map[string]string{
		"Downloads/Complete/Kettle.Bay.S01E02.mkv":   "episode Kettle Bay S1E2",
		"Downloads/Incomplete/Kettle.Bay.S01E03.mkv": "skip",
	}},
	{id: "P10", files: map[string]string{
		"Movies/Ashgrove (2015).mkv":   "movie Ashgrove 2015",
		"Movies/._Ashgrove (2015).mkv": "skip",
		"Movies/@eaDir/Ferrow.mkv":     "skip",
		"Movies/#recycle/Ferrow.mkv":   "skip",
	}},
	{id: "P11", files: map[string]string{"Movies/.hidden/Ashgrove (2015).mkv": "skip"}},

	// Q. Discs.
	{id: "Q1", files: map[string]string{"Movies/Ashgrove (2015)/VIDEO_TS/VTS_01_1.VOB": "problem"}},
	{id: "Q2", files: map[string]string{"Movies/Ashgrove (2015)/BDMV/STREAM/00001.m2ts": "problem"}},
	{id: "Q4", files: map[string]string{"Movies/Ashgrove (2015).iso": "problem"}},

	// R. Mixed roots and missing structure.
	{id: "R1", files: map[string]string{
		"Media/Ashgrove (2015).mkv":    "movie Ashgrove 2015",
		"Media/Kettle.Bay.S01E02.mkv":  "episode Kettle Bay S1E2",
		"Media/Ferrow Lane/S02E03.mkv": "episode Ferrow Lane S2E3",
	}},
	{id: "R2", files: map[string]string{"Kettle.Bay.S01E02.720p.mkv": "episode Kettle Bay S1E2"}},
	{id: "R4", files: map[string]string{
		"TV/Kettle Bay/kb 101.avi": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/kb 201.avi": "episode Kettle Bay S2E1",
	}},
	{id: "R5", files: map[string]string{
		"TV/All Shows/Kettle.Bay.S01E01.mkv":  "episode Kettle Bay S1E1",
		"TV/All Shows/Ferrow.Lane.S02E03.mkv": "episode Ferrow Lane S2E3",
	}},

	// S. Other scripts and wide characters.
	{id: "S1", files: map[string]string{"Movies/La Maison Grisée (1998).mkv": "movie La Maison Grisée 1998"}},
	{id: "S2", files: map[string]string{"Movies/Тихий сигнал (2008).mkv": "movie Тихий сигнал 2008"}},
	{id: "S4", files: map[string]string{"Movies/ＦＥＲＲＯＷ　ＬＡＮＥ（２０１９）.mkv": "movie FERROW LANE 2019"}},

	// T. Copies.
	{id: "T4", files: map[string]string{"Movies/Ashgrove (2015) (1).mkv": "movie Ashgrove 2015"}},

	// U. Sidecars that change the answer.
	{id: "U3",
		files: map[string]string{"TV/Kettle Bay/Kettle Bay - Tidewater.mkv": "episode Kettle Bay S1E2"},
		nfo: map[string]string{"TV/Kettle Bay/Kettle Bay - Tidewater.nfo": "<episodedetails><title>Tidewater</title>" +
			"<season>1</season><episode>2</episode></episodedetails>"}},
}

func TestNamingMatrix(t *testing.T) {
	known := map[string]bool{}
	for _, row := range namingMatrix {
		for rel := range row.files {
			known[row.id+" "+rel] = true
		}
	}
	for key := range namingToday {
		if !known[key] {
			t.Errorf("namingToday names %q, which no row has", key)
		}
	}
	var right, files atomic.Int32
	t.Run("rows", func(t *testing.T) {
		for _, row := range namingMatrix {
			t.Run(row.id, func(t *testing.T) {
				t.Parallel()
				got := namingOutcomes(t, row)
				for rel, want := range row.files {
					files.Add(1)
					today, wrong := namingToday[row.id+" "+rel]
					switch {
					case wrong && got[rel] == want:
						t.Errorf("%s is right now; drop it from namingToday", rel)
					case wrong && got[rel] != today:
						t.Errorf("%s = %q, was %q (want %q)", rel, got[rel], today, want)
					case !wrong && got[rel] == want:
						right.Add(1)
					case !wrong:
						t.Errorf("%s = %q, want %q", rel, got[rel], want)
					}
				}
			})
		}
	})
	t.Logf("naming matrix: %d of %d files right", right.Load(), files.Load())
}

// namingOutcomes lays a row's files out under a fresh root, runs a full pass, and says what each
// file became.
func namingOutcomes(t *testing.T, row namingRow) map[string]string {
	t.Helper()
	h := newHarness(t)
	for rel := range row.files {
		h.file(rel, 10)
	}
	for rel, doc := range row.nfo {
		p := filepath.Join(h.root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.run()
	ctx := context.Background()
	rows, err := h.st.LibraryFiles(ctx, h.root)
	if err != nil {
		t.Fatal(err)
	}
	// A title is read as the Library lists it: projected from its inventory item.
	listed := func(origin inventory.OriginKey) (librarystore.LibraryItem, bool) {
		item, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &origin})
		if err != nil || !ok {
			return librarystore.LibraryItem{}, false
		}
		return ProjectLibraryItem(item)
	}
	showOf := func(o librarystore.LibraryOrigin) librarystore.LibraryItem {
		it, _ := listed(inventory.OriginKey{Authority: inventory.AuthorityID(o.Authority), ExternalItemID: o.Key})
		return it
	}
	byPath := map[string]librarystore.LibraryFile{}
	for _, r := range rows {
		byPath[r.Path] = r
	}
	out := map[string]string{}
	for rel := range row.files {
		p := filepath.Join(h.root, filepath.FromSlash(rel))
		r, ok := byPath[p]
		// A disc structure is listed as one problem, at its folder.
		for d := filepath.Dir(p); !ok && d != h.root; d = filepath.Dir(d) {
			if up, listed := byPath[d]; listed && up.Problem != "" {
				r, ok = up, true
			}
		}
		switch {
		case !ok:
			out[rel] = "skip"
		case r.Problem != "" && r.ItemID == "":
			out[rel] = "problem"
		default:
			it, ok := listed(fileOrigin(h.root, rel))
			if !ok {
				out[rel] = "no title"
				continue
			}
			out[rel] = describeItem(showOf, it)
		}
	}
	return out
}

// describeItem is a title's outcome, as a naming row reads it.
func describeItem(showOf func(librarystore.LibraryOrigin) librarystore.LibraryItem, it librarystore.LibraryItem) string {
	searched := func(it librarystore.LibraryItem) string {
		title, year, _ := searchQuery(it.Name, it.Year)
		if year == 0 {
			return title + " -"
		}
		return fmt.Sprintf("%s %d", title, year)
	}
	switch it.Kind {
	case "movie":
		return "movie " + searched(it)
	case "episode":
		show := strings.TrimSuffix(searched(showOf(it.Series)), " -")
		if it.Season == 0 && it.Episode == 0 {
			return "episode " + show + " S?E?"
		}
		at := fmt.Sprintf("S%dE%d", it.Season, it.Episode)
		if it.EpisodeEnd > it.Episode {
			at += fmt.Sprintf("-%d", it.EpisodeEnd)
		}
		return "episode " + show + " " + at
	case "":
		return "no title"
	default:
		return it.Kind + " " + it.Name
	}
}
