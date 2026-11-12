package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// Every row reads right; a new row the scanner cannot read yet goes here until it can.
var namingToday = map[string]string{}

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
	{id: "I1", files: map[string]string{"TV/Evening Ledger/Evening.Ledger.2024-03-14.Guest.Name.mkv": "episode Evening Ledger 2024-03-14"}},
	{id: "I2", files: map[string]string{"TV/Evening Ledger/Evening Ledger 20240314.mkv": "episode Evening Ledger 2024-03-14"}},
	{id: "I4", files: map[string]string{"TV/Evening Ledger/Evening Ledger 14.03.2024.mkv": "episode Evening Ledger 2024-03-14"}},
	{id: "I10", files: map[string]string{"TV/Evening Ledger/Season 2024/Evening Ledger - 2024-03-14 - Guest.mkv": "episode Evening Ledger 2024-03-14"}},

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
	// An archive unpacked into a folder of its own name leaves a season folder in a season folder.
	{id: "K12", files: map[string]string{
		"TV/Kettle Bay/Season 1/Season 1/Kettle Bay S01E01.mkv":  "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Specials/Specials/Kettle Bay - Recap.mkv": "episode Kettle Bay S?E?",
	}},
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

	// V. Found by the 2026-11-11 re-score's probes.
	{id: "V1", files: map[string]string{"Movies/10x10 (2018)/10x10 (2018).mkv": "movie 10x10 2018"}},
	{id: "V2", files: map[string]string{"TV/Kettle Bay/Season 1/101 - Tidewater.mkv": "episode Kettle Bay S1E1"}},
	{id: "V3", files: map[string]string{
		"TV/Kettle Bay/Season 1/101 - Tidewater.mkv": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Season 1/102 - Low Water.mkv": "episode Kettle Bay S1E2",
	}},
	{id: "V4", files: map[string]string{"TV/Kettle Bay/Season 2/Kettle Bay - 03 - Tidewater.mkv": "episode Kettle Bay S2E3"}},
	{id: "V5", files: map[string]string{"TV/Kettle Bay/Season 3/Kettle Bay - 3.05 - Tidewater.mkv": "episode Kettle Bay S3E5"}},
	{id: "V6", files: map[string]string{"TV/Kettle Bay/Season 2/Kettle Bay E03.mkv": "episode Kettle Bay S2E3"}},
	{id: "V7", files: map[string]string{
		"Movies/Ferrow Lane (2019).divx":   "movie Ferrow Lane 2019",
		"Movies/Kettle Run (2003).flv":     "movie Kettle Run 2003",
		"Movies/Low Orchard (2017).ogm":    "movie Low Orchard 2017",
		"Movies/Tinder Hollow (2012).rmvb": "movie Tinder Hollow 2012",
		"Movies/Marrow Point (2001).VOB":   "movie Marrow Point 2001",
	}},
	{id: "V8", files: map[string]string{
		"TV/Kettle Bay/Season 1/Kettle Bay - S01E04 - Each-Other.mkv":       "episode Kettle Bay S1E4",
		"TV/Kettle Bay/Season 1/Kettle Bay - S01E05 - Behind-the-Scene.mkv": "episode Kettle Bay S1E5",
		"TV/Kettle Bay/Season 1/Kettle Bay - S01E01-sample.mkv":             "skip",
	}},
	{id: "V9", files: map[string]string{
		"Movies/Shorts/Lantern Keeper (2010)/Lantern Keeper (2010).mkv": "movie Lantern Keeper 2010",
		"Movies/Ashgrove (2015)/Shorts/Ashgrove Reel.mkv":               "skip",
	}},
	// W: what a messy library does to a name. Season folders spelled out, in other languages, with
	// notes or the show's initials, or a disc or codec folder inside them.
	{id: "W1", files: map[string]string{"TV/Kettle Bay/Season One/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W2", files: map[string]string{"TV/Kettle Bay/Sezon 2/Kettle Bay S02E02.mkv": "episode Kettle Bay S2E2"}},
	{id: "W3", files: map[string]string{"TV/Kettle Bay/Säsong 1/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W4", files: map[string]string{"TV/Kettle Bay/Season 1 - 2019/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W5", files: map[string]string{"TV/Kettle Bay/Season 1 1080p WEB-DL/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W6", files: map[string]string{"TV/Kettle Bay/Season 01 Complete/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W7", files: map[string]string{"TV/Kettle Bay/KB Season 1/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W8", files: map[string]string{"TV/Kettle Bay/(Season 1)/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W9", files: map[string]string{"TV/Kettle Bay/Season 1/Disc 2/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	{id: "W10", files: map[string]string{"TV/Kettle Bay/Season 1/x265/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2"}},
	// A day alone makes no show: a home video and a concert are films of a day.
	{id: "W11", files: map[string]string{
		"Home Videos/2019-06-01 Beach Day.mp4":                        "movie 2019-06-01 Beach Day -",
		"Concerts/Ferrow Band - Live at Ashgrove Hall 2019.10.05.mkv": "movie Ferrow Band - Live at Ashgrove Hall 2019.10.05 -",
	}},
	{id: "W12", files: map[string]string{
		"TV/Daily Kettle/Daily Kettle 2024_03_14.mkv": "episode Daily Kettle 2024-03-14",
		"TV/Daily Kettle/Daily Kettle 2024-3-15.mkv":  "episode Daily Kettle 2024-03-15",
	}},
	// A year after a dot or a dash, and one after the release words.
	{id: "W13", files: map[string]string{
		"Movies/Ferrow Lane.2019.mkv":                     "movie Ferrow Lane 2019",
		"Movies/The Quiet Signal.2008.720p.mkv":           "movie The Quiet Signal 2008",
		"Movies/Lantern.Keeper (2010).mkv":                "movie Lantern Keeper 2010",
		"Movies/marrow-point-2011.mkv":                    "movie marrow point 2011",
		"Movies/Ashgrove 1080p 2015.mkv":                  "movie Ashgrove 2015",
		"Movies/S.W.A.L.E. (2012).mkv":                    "movie S.W.A.L.E 2012",
		"Movies/Harbor Lights (2016) (2020 Remaster).mkv": "movie Harbor Lights 2016",
	}},
	// Release notes, a collection's place and a tracker's name are not the title.
	{id: "W14", files: map[string]string{
		"Movies/Ashgrove.CAM.avi":                                        "movie Ashgrove -",
		"Movies/Ferrow.HDRip.avi":                                        "movie Ferrow -",
		"Movies/Lantern.MULTi.VOSTFR.mkv":                                "movie Lantern -",
		"Movies/Marrow [HD].mkv":                                         "movie Marrow -",
		"Movies/Tidewater (Dual Audio).mkv":                              "movie Tidewater -",
		"Movies/Low Water (Directors Cut).mkv":                           "movie Low Water -",
		"Movies/Kettle Bay (US) (2004).mkv":                              "movie Kettle Bay US 2004",
		"Movies/www.TrackerNet.org - Harbor Lights (2016).mkv":           "movie Harbor Lights 2016",
		"Movies/Marrow Point Collection/01 - Marrow Point (2010).mkv":    "movie Marrow Point 2010",
		"Movies/Marrow Point Collection/2. Marrow Point II (2012).mkv":   "movie Marrow Point II 2012",
		"Movies/Marrow Point Collection/(3) Marrow Point III (2014).mkv": "movie Marrow Point III 2014",
		"Movies/10 Kettles (2009).mkv":                                   "movie 10 Kettles 2009",
	}},
	// A NAS's snapshots and recycle bins, a downloader's unfinished work, an editor's lock file and
	// a film's extras are none of the library's titles; an Xvid file is a video.
	{id: "W16", files: map[string]string{
		"Movies/#snapshot/Ashgrove (2015).mkv":                         "skip",
		"Movies/@Recently-Snapshot/Ashgrove (2015).mkv":                "skip",
		"Movies/@Recycle/Ashgrove (2015).mkv":                          "skip",
		"Movies/System Volume Information/Ashgrove (2015).mkv":         "skip",
		"Downloads/_UNPACK_Kettle.Bay.S01E02/Kettle.Bay.S01E02.mkv":    "skip",
		"Downloads/_FAILED_Kettle.Bay.S01E03/Kettle.Bay.S01E03.mkv":    "skip",
		"Movies/~Ferrow (2019).mkv":                                    "skip",
		"Movies/Lantern (2010)/Lantern (2010).mkv":                     "movie Lantern 2010",
		"Movies/Lantern (2010)/Lantern (2010) - Behind the Scenes.mkv": "skip",
		"Movies/Lantern (2010)/Lantern (2010) - Interview.mkv":         "skip",
		"Movies/Lantern (2010)/Lantern (2010) Trailer.mkv":             "skip",
		"Movies/Lantern (2010)/theme.mp4":                              "skip",
		"Movies/Marrow (2011).xvid":                                    "movie Marrow 2011",
	}},
	// An extras folder is a title's only in the title's own folder: under a genre or age folder it
	// is a folder of titles.
	{id: "W17", files: map[string]string{
		"Movies/Kids/Shorts/Lantern Keeper (2010)/Lantern Keeper (2010).mkv": "movie Lantern Keeper 2010",
		"Movies/Kids/Other/Ferrow Lane (2019).mkv":                           "movie Ferrow Lane 2019",
		"Movies/Comedy/Clips/Kettle Comic (2018).mkv":                        "movie Kettle Comic 2018",
		"Movies/Ashgrove (2015)/Ashgrove (2015).mkv":                         "movie Ashgrove 2015",
		"Movies/Ashgrove (2015)/Extras/Ashgrove Reel.mkv":                    "skip",
	}},
	{id: "W18", files: map[string]string{
		"TV/Cartoons/Shorts/Kettle Bay/Season 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1",
		"TV/Marrow Point/Season 1/Marrow Point S01E01.mkv":             "episode Marrow Point S1E1",
		"TV/Marrow Point/Extras/Marrow Point Reel.mkv":                 "skip",
	}},
	// A folder whose episodes name several shows, none its own, is a dump of them.
	{id: "W19", files: map[string]string{
		"Torrents/Kettle Bay S01E01.mkv":  "episode Kettle Bay S1E1",
		"Torrents/Ferrow Lane S02E03.mkv": "episode Ferrow Lane S2E3",
		"Pile/Kettle Bay S01E02.mkv":      "episode Kettle Bay S1E2",
		"Pile/Ferrow Lane S02E04.mkv":     "episode Ferrow Lane S2E4",
	}},
	{id: "W20", files: map[string]string{
		"TV/Kettle Bay (US)/Kettle.Bay.US.S01E01.mkv": "episode Kettle Bay US S1E1",
		"TV/Kettle Bay (US)/KB S01E02.mkv":            "episode Kettle Bay US S1E2",
	}},
	// Extras a household named in words, beside the film.
	{id: "W21", files: map[string]string{
		"Movies/Ferrow Lane (2019)/Ferrow Lane (2019).mkv":             "movie Ferrow Lane 2019",
		"Movies/Ferrow Lane (2019)/Gag Reel.mkv":                       "skip",
		"Movies/Ferrow Lane (2019)/Making Of.mkv":                      "skip",
		"Movies/Ferrow Lane (2019)/Trailer 2.mkv":                      "skip",
		"Movies/Ferrow Lane (2019)/Featurette - Costumes.mkv":          "skip",
		"Movies/Ferrow Lane (2019)/Ferrow Lane (2019) - Teaser.mkv":    "skip",
		"Movies/Ferrow Lane (2019)/Ferrow Lane (2019)-Trailer 2.mkv":   "skip",
		"Movies/Ferrow Lane (2019)/Ferrow Lane (2019) - Making Of.mkv": "skip",
		"Movies/Ferrow.Lane.2019.Uncut.mkv":                            "movie Ferrow Lane 2019",
		"Movies/Ashgrove-(2015).mkv":                                   "movie Ashgrove 2015",
		"Movies/Plex Versions/Optimized for TV/Ashgrove (2015).mp4":    "skip",
	}},
	{id: "W22", files: map[string]string{
		"TV/Kettle Bay/Season 2/Kettle Bay - S02E05 - Behind the Scenes.mkv": "episode Kettle Bay S2E5",
	}},
	// Episodes with no season folder or full mark, and seasons as other folders name them.
	{id: "W23", files: map[string]string{
		"TV/Kettle Bay/Kettle Bay E01.mkv": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Kettle Bay E02.mkv": "episode Kettle Bay S1E2",
	}},
	{id: "W24", files: map[string]string{"TV/Evening Ledger/Evening Ledger E1523.mkv": "episode Evening Ledger S1E1523"}},
	{id: "W25", files: map[string]string{"TV/Kettle Bay/Season Specials/Kettle Bay S00E02.mkv": "episode Kettle Bay S0E2"}},
	{id: "W26", files: map[string]string{"TV/Kettle Bay/Special/Kettle Bay S00E01.mkv": "episode Kettle Bay S0E1"}},
	{id: "W27", files: map[string]string{"TV/Kettle Bay/Seasons 1/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "W28", files: map[string]string{"TV/Kettle Bay/1st Season/Kettle Bay 06.mkv": "episode Kettle Bay S1E6"}},
	{id: "W29", files: map[string]string{"TV/Kettle Bay/2019/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1"}},
	{id: "W30", files: map[string]string{"TV/Das Kettle Haus/Staffel 1/Folge 1.mkv": "episode Das Kettle Haus S1E1"}},
	{id: "W31", files: map[string]string{"TV/Kettle Huset/Säsong 1/Avsnitt 2.mkv": "episode Kettle Huset S1E2"}},
	// A daily show's year folders, and its days written America's way.
	{id: "W32", files: map[string]string{
		"TV/Evening Ledger/2024/Evening Ledger 2024-03-14.mkv": "episode Evening Ledger 2024-03-14",
		"TV/Evening Ledger/2023/Evening Ledger 2023-11-02.mkv": "episode Evening Ledger 2023-11-02",
		"TV/Evening Ledger/Evening Ledger 03-15-2024.mkv":      "episode Evening Ledger 2024-03-15",
	}},
	// A fansub show's name that ends in its season.
	{id: "W33", files: map[string]string{
		"[Kumo] Sora no Kettle S2 - 03 [1080p].mkv":        "episode Sora no Kettle S2E3",
		"[Kumo] Ferrow Gakuen 2nd Season - 05 [1080p].mkv": "episode Ferrow Gakuen S2E5",
		"[Kumo] Ferrow Gakuen - PV1 [1080p].mkv":           "skip",
	}},
	// A catch-all's "Part 2 of 3" is a film's, not an episode of a show named "Movies".
	{id: "W34", files: map[string]string{"Movies/Ferrow Lane Part 2 of 3 (2010).mkv": "movie Ferrow Lane Part 2 of 3 2010"}},
	// A title may carry a word a release name does: it ends the title only after the year, before
	// another release word, or last in a dotted name.
	{id: "W35", files: map[string]string{
		"Movies/Tangled Web (2016).mkv":                "movie Tangled Web 2016",
		"Movies/The Cam (2017).mkv":                    "movie The Cam 2017",
		"Movies/Mister Proper (2010).mkv":              "movie Mister Proper 2010",
		"Movies/Kettle Extended Family (2013).mkv":     "movie Kettle Extended Family 2013",
		"Movies/Ferrow DVD Night (2012).mkv":           "movie Ferrow DVD Night 2012",
		"Movies/Marrow.2014.PROPER.1080p.mkv":          "movie Marrow 2014",
		"Movies/Lantern.Cove.German.DL.1080p.mkv":      "movie Lantern Cove -",
		"Movies/Ashgrove+Cove+2014+720p.mkv":           "movie Ashgrove Cove 2014",
		"Movies/www.TrackerNet.org_Low.Water.2014.mkv": "movie Low Water 2014",
		"Movies/Trailer (2014).mkv":                    "movie Trailer 2014",
		"Movies/The 4x4 Club (2016).mkv":               "movie The 4x4 Club 2016",
	}},
	{id: "W36", files: map[string]string{"TV/The Web Crew/Season 1/The Web Crew S01E01.mkv": "episode The Web Crew S1E1"}},
	{id: "W37", files: map[string]string{
		"TV/Kids/Kettle Bay S01E04.mkv":      "episode Kettle Bay S1E4",
		"Dad's Stuff/Ferrow.Lane.S01E01.mkv": "episode Ferrow Lane S1E1",
		"Dad's Stuff/Ashgrove (2015).mkv":    "movie Ashgrove 2015",
	}},
	{id: "W38", files: map[string]string{
		"Movies/Brindle Cove (2014)/Brindle Cove (2014).mkv":                       "movie Brindle Cove 2014",
		"Movies/Brindle Cove (2014)/Brindle Cove (2014) - Official Trailer.mkv":    "skip",
		"Movies/Brindle Cove (2014)/Brindle Cove (2014) Teaser Trailer.mkv":        "skip",
		"Movies/Brindle Cove (2014)/Brindle Cove (2014) - Director Commentary.mkv": "skip",
		"Movies/Brindle Cove (2014)/Brindle Cove (2014) - Interview with Cast.mkv": "skip",
		"Movies/Brindle Cove (2014)/Brindle Cove (2014) - Alternate Ending.mkv":    "skip",
	}},
	{id: "W39", files: map[string]string{
		"Downloads/Evening.Ledger.2024.03.18.720p.WEB.mkv": "episode Evening Ledger 2024-03-18",
		"Evening.Ledger.2024.03.19.Guest.Name.720p.mkv":    "episode Evening Ledger 2024-03-19",
	}},
	{id: "W40", files: map[string]string{"TV/Evening Ledger/The Evening Ledger 2024-03-25.mkv": "episode Evening Ledger 2024-03-25"}},
	{id: "W41", files: map[string]string{
		"TV/La Bahia/Temporada 1/Capitulo 5.mkv":   "episode La Bahia S1E5",
		"TV/La Bahia/Temporada 1/Capítulo 06.mkv":  "episode La Bahia S1E6",
		"TV/La Bahia/La Bahia [HDTV][Cap.117].avi": "episode La Bahia S1E17",
	}},
	{id: "W42", files: map[string]string{"TV/Gavan/Сезон 1/Серия 9.mkv": "episode Gavan S1E9"}},
	{id: "W43", files: map[string]string{"TV/Liman/Sezon 1/Bölüm 15.mkv": "episode Liman S1E15"}},
	{id: "W44", files: map[string]string{"TV/Kettle Kou/Season 1/第11話.mkv": "episode Kettle Kou S1E11"}},
	{id: "W45", files: map[string]string{
		"Movies/Ashgrove (2015).wtv":                                               "movie Ashgrove 2015",
		"Movies/Ferrow (2016).dvr-ms":                                              "movie Ferrow 2016",
		"Movies/Lantern (2017).mk3d":                                               "movie Lantern 2017",
		"Movies/Network Trash Folder/Marrow (2018).mkv":                            "skip",
		"Movies/Temporary Items/Marrow (2018).mkv":                                 "skip",
		"Movies/RECYCLER/Marrow (2018).mkv":                                        "skip",
		"Movies/~snapshot/Marrow (2018).mkv":                                       "skip",
		"Downloads/intermediate/Kettle.Bay.S01E05.720p/Kettle.Bay.S01E05.720p.mkv": "skip",
	}},
	{id: "W46", files: map[string]string{
		"Anime/[Kumo] Sora no Kettle (01-12) [BD 1080p]/[Kumo] Sora no Kettle - 01 [BD 1080p].mkv": "episode Sora no Kettle S?E?",
		"[Kumo] Ferrow Gakuen - 10 END [1080p].mkv":                                                "episode Ferrow Gakuen S?E?",
	}},
	{id: "W47", files: map[string]string{
		"TV/Kettle Bay/Season 1/Kettle Bay S01E01.mkv":                    "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Kettle.Bay.S02.1080p.WEB-DL/Kettle.Bay.S02E01.mkv": "episode Kettle Bay S2E1",
	}},
	{id: "W48", files: map[string]string{
		"TV/Kettle Bay (Complete)/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1",
	}},
	{id: "W49", files: map[string]string{
		"TV/Kettle Bay/Kettle Bay S01E01.mkv": "episode Kettle Bay S1E1",
		"TV/Kettle Bay/Kettle Bay S01E02.mkv": "episode Kettle Bay S1E2",
		"TV/Kettle Bay/Pilot Extended.mkv":    "episode Kettle Bay S?E?",
	}},
	{id: "W50", files: map[string]string{"Movies/Brindle Cove/bc.mkv": "movie Brindle Cove -"}},
	// Episode marks as other apps and languages write them, and one file of several.
	{id: "W15", files: map[string]string{
		"TV/Kettle Bay/Kettle Bay 1x2.avi":               "episode Kettle Bay S1E2",
		"TV/Kettle Bay/Kettle Bay S01Ep03.avi":           "episode Kettle Bay S1E3",
		"TV/Kettle Bay/Kettle Bay S01 - E04.avi":         "episode Kettle Bay S1E4",
		"TV/Kettle Bay/Kettle Bay T01E05.avi":            "episode Kettle Bay S1E5",
		"TV/Kettle Bay/Kettle Bay Staffel 1 Folge 6.avi": "episode Kettle Bay S1E6",
		"TV/Kettle Bay/Kettle Bay S02E01-720.mkv":        "episode Kettle Bay S2E1",
		"TV/Kettle Bay/Kettle Bay S02E02 and E03.mkv":    "episode Kettle Bay S2E2-3",
	}},
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
	return namingOutcomesWith(t, row, false)
}

// namingOutcomesWith is namingOutcomes, and with grounded, each title's ids (an episode's show's)
// and a movie's edition too.
func namingOutcomesWith(t *testing.T, row namingRow, grounded bool) map[string]string {
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
	return outcomesIn(t, h, row.files, grounded)
}

// outcomesIn is what each of files became in h's last pass.
func outcomesIn(t *testing.T, h *harness, files map[string]string, grounded bool) map[string]string {
	t.Helper()
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
	for rel := range files {
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
			if grounded {
				out[rel] += groundedSuffix(showOf, it)
			}
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
			if it.PremiereDate != "" {
				return "episode " + show + " " + it.PremiereDate
			}
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

// groundedSuffix is the ids a title carries (an episode, its show's), sorted, and a movie's
// edition: " [imdb:tt9000001] {Final Cut}".
func groundedSuffix(showOf func(librarystore.LibraryOrigin) librarystore.LibraryItem, it librarystore.LibraryItem) string {
	holder := it
	if it.Kind == "episode" {
		holder = showOf(it.Series)
	}
	var ids []string
	for _, id := range holder.IDs {
		ids = append(ids, id.Namespace+":"+id.Value)
	}
	slices.Sort(ids)
	out := ""
	for _, id := range ids {
		out += " [" + id + "]"
	}
	for _, s := range it.Sources {
		if s.Edition != "" {
			out += " {" + s.Edition + "}"
		}
	}
	return out
}
