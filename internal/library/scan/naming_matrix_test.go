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
	// An extra is one beside its title, however it is named; with no title beside it, a name with an
	// extra's word in it is a title of its own.
	{id: "W51", files: map[string]string{
		"Movies/Copper Hollow (2018)/Copper Hollow (2018).mkv":                   "movie Copper Hollow 2018",
		"Movies/Copper Hollow (2018)/Copper.Hollow.2018.Trailer.1080p.mkv":       "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018) [Trailer].mkv":         "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018) (Trailer).mkv":         "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018)-trailer.1080p.mkv":     "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018) - Trailer #2.mkv":      "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow Official Trailer.mkv":         "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow Teaser.mkv":                   "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018) - Deleted Scene 1.mkv": "skip",
		"Movies/Copper Hollow (2018)/Copper Hollow (2018) - Music Video.mkv":     "skip",
		"Movies/Copper Hollow (2018)/Bonus/Making Of Part 1.mkv":                 "skip",
	}},
	// A title's apostrophe is in its extra's name too; Kodi's trailer carries the release's notes.
	{id: "W51b", files: map[string]string{
		"Movies/Barley's Ferry (2010)/Barley's Ferry (2010) 1080p DTS 6ch.mkv":              "movie Barley's Ferry 2010",
		"Movies/Barley's Ferry (2010)/Barley's Ferry (2010) 1080p DTSHD-MA 6ch-trailer.flv": "skip",
		"Movies/Barley's Ferry (2010)/Barley's Ferry (2010) - Director's Commentary.mkv":    "skip",
	}},
	{id: "W52", files: map[string]string{
		"Movies/Night.Trailer.mkv":                         "movie Night Trailer -",
		"Movies/The.Last.Sample.mkv":                       "movie The Last Sample -",
		"Movies/Cut-Short.mkv":                             "movie Cut-Short -",
		"Movies/the-other.avi":                             "movie the-other -",
		"Movies/Outtakes.mkv":                              "movie Outtakes -",
		"Movies/Ashgrove Short (2017).mkv":                 "movie Ashgrove Short 2017",
		"Movies/.45 Kettles (2014).mkv":                    "movie 45 Kettles 2014",
		"Movies/@Home Again (2019)/@Home Again (2019).mkv": "movie @Home Again 2019",
	}},
	{id: "W53", files: map[string]string{
		"TV/Pelican Drift/Season 2/Pelican Drift - S02E08 - Trailer.mkv": "episode Pelican Drift S2E8",
		"TV/Pelican Drift/Season 1/Pelican Drift - S01E01.mkv":           "episode Pelican Drift S1E1",
		"TV/Pelican Drift/Pelican Drift - Christmas Special.mkv":         "episode Pelican Drift S?E?",
	}},
	{id: "W54", files: map[string]string{
		"Docs/Ocean Halls/01 - The Deep.mkv":  "episode Ocean Halls S1E1",
		"Docs/Ocean Halls/02 - The Shelf.mkv": "episode Ocean Halls S1E2",
	}},
	{id: "W55", files: map[string]string{"TV/Pelican Drift (Complete Series)/Season 1/Pelican Drift S01E01.mkv": "episode Pelican Drift S1E1"}},
	{id: "W56", files: map[string]string{"TV/Pelican Drift Complete/Season 1/Pelican Drift S01E01.mkv": "episode Pelican Drift S1E1"}},
	{id: "W57", files: map[string]string{"TV/Pelican Drift - The Complete Series/Season 1/Pelican Drift S01E01.mkv": "episode Pelican Drift S1E1"}},
	{id: "W58", files: map[string]string{
		"Movies/Copper.Hollow.2018.[1080p].mkv":  "movie Copper Hollow 2018",
		"Movies/【Lantern Cove】(2016).mkv":        "movie Lantern Cove 2016",
		"Movies/Copy of Marrow Point (2010).mkv": "movie Marrow Point 2010",
		"Movies/Brindle Cove -2014-.mkv":         "movie Brindle Cove 2014",
	}},
	{id: "W59", files: map[string]string{"TV/Dōkoku/第1期/第3話.mkv": "episode Dōkoku S1E3"}},
	{id: "W60", files: map[string]string{"TV/Guri/시즌 1/3화.mkv": "episode Guri S1E3"}},
	{id: "W61", files: map[string]string{"TV/Med/Сезон 1/Med - 3 серия.mkv": "episode Med S1E3"}},
	{id: "W62", files: map[string]string{"TV/Series 7 Club/Season 1/Series 7 Club S01E01.mkv": "episode Series 7 Club S1E1"}},
	// A show's folder holding its episodes in folders it does not name a season by: one show.
	{id: "W63", files: map[string]string{
		"TV/Brass Horn/Season 1/Brass Horn S01E01.mkv":                       "episode Brass Horn S1E1",
		"TV/Brass Horn/Seaosn 2/Brass Horn S02E01.mkv":                       "episode Brass Horn S2E1",
		"TV/Brass Horn/new/Brass Horn S02E02.mkv":                            "episode Brass Horn S2E2",
		"TV/Brass Horn/Vol 1/Brass Horn S02E03.mkv":                          "episode Brass Horn S2E3",
		"TV/Brass Horn/BH S02/Brass Horn S02E04.mkv":                         "episode Brass Horn S2E4",
		"TV/Brass Horn/Brass.Horn.S03.WEB-DL/Season 3/Brass.Horn.S03E01.mkv": "episode Brass Horn S3E1",
		"TV/Brass Horn/Brass Horn Complete/Season 4/Brass Horn S04E01.mkv":   "episode Brass Horn S4E1",
		"TV/Brass Horn/Brass Horn (Season 5)/Brass Horn S05E01.mkv":          "episode Brass Horn S5E1",
		"TV/Brass Horn/Brass Horn DVD 1/Brass Horn S06E01.mkv":               "episode Brass Horn S6E1",
	}},
	{id: "W64", files: map[string]string{
		"Downloads/Brass.Reed.S01E01.720p.mkv": "episode Brass Reed S1E1",
		"Downloads/brass.reed.s01e02.720p.mkv": "episode Brass Reed S1E2",
	}},
	// Fansub releases, however their number is written; a film with a number in its title stays one.
	{id: "W66", files: map[string]string{
		"[Kumo] Hoshi Tanken 19 [1080p].mkv":             "episode Hoshi Tanken S?E?",
		"[Kumo] Hoshi Tanken - Episode 17 [1080p].mkv":   "episode Hoshi Tanken S?E?",
		"[Kumo] Hoshi Tanken - E20 [1080p].mkv":          "episode Hoshi Tanken S?E?",
		"[Kumo] Hoshi Tanken - 05.5 [1080p].mkv":         "episode Hoshi Tanken S?E?",
		"[Kumo] Hoshi Tanken - 01-03 [1080p].mkv":        "episode Hoshi Tanken S?E?",
		"Hoshi Tanken - 10 [1080p].mkv":                  "episode Hoshi Tanken S?E?",
		"[Kumo]_hoshi_tanken_-_04_[720p].mkv":            "episode Hoshi Tanken S?E?",
		"Movies/[TrackerNet] Eleven Hands 11 (2001).mkv": "movie Eleven Hands 11 2001",
		"Movies/Eleven Hands 11 [1080p].mkv":             "movie Eleven Hands 11 -",
	}},
	// A show's folder in a shows' catch-all numbers its episodes alone; a film's folder holds sequels.
	{id: "W67", files: map[string]string{
		"TV/Ivory Fen/Ivory Fen 1.mkv":           "episode Ivory Fen S?E?",
		"TV/Ivory Fen/Ivory Fen 2.mkv":           "episode Ivory Fen S?E?",
		"Anime/Hoshi Kettle/Hoshi Kettle 12.mkv": "episode Hoshi Kettle S?E?",
		"TV/Ivory Bay/01.mkv":                    "episode Ivory Bay S?E?",
		"TV/Ivory Bay/02.mkv":                    "episode Ivory Bay S?E?",
		"Movies/Marrow Point/Marrow Point 2.mkv": "movie Marrow Point 2 -",
		"Kids/Spotty Dog - Episode 3.mkv":        "episode Spotty Dog S1E3",
		"TV/Spotty Cat/Spotty Cat - Ep 04.mkv":   "episode Spotty Cat S1E4",
	}},
	// A film in its own folder in a films' catch-all is a film, whatever its title says.
	{id: "W68", files: map[string]string{
		"Movies/Season One (2012)/Season One (2012).mkv":   "movie Season One 2012",
		"Movies/Episode 50 (2011)/Episode 50 (2011).mkv":   "movie Episode 50 2011",
		"Movies/Hour 2 of 3 (2018)/Hour 2 of 3 (2018).mkv": "movie Hour 2 of 3 2018",
		"Movies/Grid 4x4/Grid 4x4.mkv":                     "movie Grid 4x4 -",
		"Movies/Gravel Shore/Gravel Shore - 2.mkv":         "movie Gravel Shore - 2 -",
	}},
	// A title ending in a word an extra or a sample is named by is a title.
	{id: "W69", files: map[string]string{
		"TV/Cold Coast/Season 1/Cold Coast S01E02 - The Sample.mkv":       "episode Cold Coast S1E2",
		"TV/Cold Coast/Season 1/Cold Coast - S01E03 - Free Sample.mkv":    "episode Cold Coast S1E3",
		"TV/Cold Coast/Season 1/Cold Coast - S01E05 - Paper-Clip.mkv":     "episode Cold Coast S1E5",
		"TV/Cold Coast/Season 1/Cold Coast - S01E06 - Re-Interview.mkv":   "episode Cold Coast S1E6",
		"TV/Cold Coast/Season 1/Cold Coast - S01E07 - Tidewater-clip.mkv": "skip",
		"TV/Cold Coast/Season 1/Cold Coast S01E08 sample.mkv":             "skip",
		"Movies/The Last Sample/The Last Sample.mkv":                      "movie The Last Sample -",
		"Movies/Free Sample (2014).mkv":                                   "movie Free Sample 2014",
		"Movies/Ferrow Lane (2019)/Ferrow Lane (2019) sample.mkv":         "skip",
	}},
	// A show named as a season folder is, in a shows' catch-all.
	{id: "W70", files: map[string]string{
		"TV/Series Seven/Series Seven S01E01.mkv":      "episode Series Seven S1E1",
		"TV/Series Seven/Series Seven S01E02.mkv":      "episode Series Seven S1E2",
		"TV/Season One/Season 1/Season One S01E01.mkv": "episode Season One S1E1",
	}},
	// An extra of a film whose title holds an extra's word, or is written in letters apart.
	{id: "W72", files: map[string]string{
		"Movies/Q.T. the Extra-Lunar (1982)/Q.T. the Extra-Lunar (1982) [imdbid-tt0000002] - [Remux-2160p][HEVC].mkv": "movie Q.T. the Extra-Lunar 1982",
		"Movies/Q.T. the Extra-Lunar (1982)/Q.T. the Extra-Lunar (1982) 1080p DTSHD-MA 8ch-trailer.mp4":               "skip",
		"Movies/Ferry Extra (2004)/Ferry Extra (2004).mkv":                                                            "movie Ferry Extra 2004",
		"Movies/Ferry Extra (2004)/Ferry Extra (2004) Official Trailer.mkv":                                           "skip",
	}},
	// A miniseries' parts in a shows' catch-all are its episodes; a film's parts in a films' are one film.
	{id: "W73", files: map[string]string{
		"TV/Ridge Point/Ridge Point Part 1.mkv":       "episode Ridge Point S1E1",
		"TV/Ridge Point/Ridge Point Part 2.mkv":       "episode Ridge Point S1E2",
		"TV/Ridge Fall/Ridge Fall - Part One.mkv":     "episode Ridge Fall S1E1",
		"TV/Ridge Fall/Ridge Fall - Part Two.mkv":     "episode Ridge Fall S1E2",
		"Movies/Ridge Gate (2001)/Ridge Gate CD1.mkv": "movie Ridge Gate 2001",
		"Movies/Ridge Gate (2001)/Ridge Gate CD2.mkv": "movie Ridge Gate 2001",
	}},
	// An extra named by its title's year and its kind, alone; or by its show beside an episode.
	{id: "W74", files: map[string]string{
		"Movies/Gravel Peak (2003) Trailer.mp4":                          "skip",
		"Movies/Gravel Pass Trailer (2003).mkv":                          "movie Gravel Pass Trailer 2003",
		"Movies/Gravel Moor (2003) Short.mkv":                            "movie Gravel Moor 2003",
		"TV/Cold Bay/Cold Bay S01E01.mkv":                                "episode Cold Bay S1E1",
		"TV/Cold Bay/Cold Bay - Making Of.mkv":                           "skip",
		"TV/Cold Bay/Cold Bay - The Interview.mkv":                       "episode Cold Bay S?E?",
		"Movies/Pale Mount (2009)/Pale Mount (2009).mkv":                 "movie Pale Mount 2009",
		"Movies/Pale Mount (2009)/Pale Mount (2009) - Photo Gallery.mkv": "skip",
		"Movies/Pale Mount (2009)/Pale Mount (2009) - TV Spot 2.mkv":     "skip",
		"Movies/Pale Mount (2009)/Pale Mount (2009) - Blooper Reel.mkv":  "skip",
		"Movies/Pale Mount (2009)/Pale Mount (2009) - Promo.mkv":         "skip",
		"Movies/Pale Mount (2009)/Pale Mount (2009) - Storyboards.mkv":   "skip",
	}},
	// A show's folder in a shows' catch-all holds its episodes, however they are named or kept.
	{id: "W75", files: map[string]string{
		"TV/Puddle Pals/Puddle Pals - Big Splash.mp4":                          "episode Puddle Pals S?E?",
		"Kids TV/Puddle Pups/Big Splash.mp4":                                   "episode Puddle Pups S?E?",
		"TV/Tiddlywinks Farm/Tiddlywinks Farm - Series 2 - 05 - Muddy Day.mp4": "episode Tiddlywinks Farm S2E5",
		"TV/Tiddlywinks Farm/Volume 1/01 Muddy Day.mp4":                        "episode Tiddlywinks Farm S1E1",
		"TV/Tiddlywinks Farm/Volume 1/02 Windy Day.mp4":                        "episode Tiddlywinks Farm S1E2",
		"TV/Hollowmere/Season 2/Disc 1/title_t00.mkv":                          "episode Hollowmere S?E?",
		"TV/Grimsby Lanes/Grimsby Lanes S3/Grimsby Lanes - Episode 3.mkv":      "episode Grimsby Lanes S3E3",
		"Recorded TV/Larchmont Road_ITV_2022_11_02_20_00_00.wtv":               "episode Larchmont Road 2022-11-02",
		"Recorded TV/Larchmont Road_ITV_2022_11_09_20_00_00.wtv":               "episode Larchmont Road 2022-11-09",
		"Miniseries/Dunmore Road/Dunmore Road Part One.mkv":                    "episode Dunmore Road S1E1",
		"Miniseries/Dunmore Road/Dunmore Road Part Two.mkv":                    "episode Dunmore Road S1E2",
	}},
	// Among films, a file is a film: a collection's numbered films, a title with "4x4", MakeMKV's.
	{id: "W76", files: map[string]string{
		"Movies/Hazelgrot Trilogy/1. Hazelgrot.mkv":       "movie Hazelgrot -",
		"Movies/Hazelgrot Trilogy/2. Hazelgrot Rises.mkv": "movie Hazelgrot Rises -",
		"Movies/5x5 Fighters.mkv":                         "movie 5x5 Fighters -",
		"Movies/Kids/Mudcar 4x4.mp4":                      "movie Mudcar 4x4 -",
		"Movies/NETTLECOMBE_RISE/title_t00.mkv":           "movie NETTLECOMBE RISE -",
		"Movies/Corvallo Nights/Corvallo Nights_t00.mkv":  "movie Corvallo Nights -",
		"Movies/OSTREVIN_D1/OSTREVIN_D1_t00.mkv":          "movie OSTREVIN -",
		"Movies/Unsorted/VID_20190601_123456.mp4":         "movie VID 20190601 123456 -",
	}},
	// Anime as Sonarr writes it, a remake's and a sequel's folders, old scene names, a tracker's site.
	{id: "W77", files: map[string]string{
		"Anime/Kumori Sora - 01.mkv":                                        "episode Kumori Sora S?E?",
		"Anime/Kumori Sora - 02.mkv":                                        "episode Kumori Sora S?E?",
		"Anime/Tsukishiro Kan Episode 09.mp4":                               "episode Tsukishiro Kan S1E9",
		"TV/Starhaven/Starhaven (2005)/Season 1/Starhaven S01E01.mkv":       "episode Starhaven 2005 S1E1",
		"TV/Starhaven/Starhaven (1978)/Season 1/Starhaven S01E01.mkv":       "episode Starhaven 1978 S1E1",
		"TV/Kettlebridge/Kettlebridge 2/Season 1/Kettlebridge 2 S01E01.mkv": "episode Kettlebridge 2 S1E1",
		"Downloads/larchmont.road.312.hdtv-lol.avi":                         "episode Larchmont Road S3E12",
		"Downloads/larchmont.road.313.hdtv-lol.avi":                         "episode Larchmont Road S3E13",
		"Movies/Fahrenheit.Lane.451.1080p.mkv":                              "movie Fahrenheit Lane 451 -",
		"Downloads/www.Tracker.Example - Quillmere Two (2016) 720p.mkv":     "movie Quillmere Two 2016",
		"Movies/Tracker.io - Brindlewood Four (2017).mkv":                   "movie Brindlewood Four 2017",
	}},
	// Re-score #6's misreads: a number in an anime's name, a dated anime, a batch, a recorder's
	// names, a site's promo, a typo'd episode in a shows' catch-all, a TV capture's parts.
	{id: "W78", files: map[string]string{
		"Anime/[Kumo] Hoshizora 86 - 04 [1080p].mkv":                       "episode Hoshizora 86 S?E?",
		"Anime/[Kumo] Ridgeback (2019) - 07 [1080p].mkv":                   "episode Ridgeback 2019 S?E?",
		"Anime/[Kumo] Saltmarsh - 01-12 [Batch]/[Kumo] Saltmarsh - 03.mkv": "episode Saltmarsh S?E?",
		"Movies/S01E01 (2019).mkv":                                         "movie S01E01 2019",
		"Recordings/Wexcombe_20231105_2000.ts":                             "episode Wexcombe 2023-11-05",
		"Movies/Trust.Me.mkv":                                              "movie Trust Me -",
		"Downloads/Brackenridge (2014)/www.Torrent.to.mp4":                 "skip",
		"Downloads/Brackenridge (2014)/RARBG.com.mp4":                      "skip",
		"Downloads/Brackenridge (2014)/Brackenridge (2014).mkv":            "movie Brackenridge 2014",
		"TV Shows/Pebble Pals Season 1/Pebble Pals - Rainy Day.mp4":        "episode Pebble Pals S?E?",
		"TV Shows/Pebble Pals Season 1/Pebble Pals - Sunny Day.mp4":        "episode Pebble Pals S?E?",
		"TV Shows/Pebble Pals Season 1/Pebbel Pals - Windy Day.mp4":        "episode Pebble Pals S?E?",
	}},
	// Re-score #7: a season folder's dashed number is that season's; "1.05" is season 1's fifth; a
	// "Show Season 2" folder's numbers are season 2's; "Season I"; an episode titled "Sample".
	{id: "W79", files: map[string]string{
		"TV/Bramble House/Season 2/Bramble House - 03.mkv":         "episode Bramble House S2E3",
		"TV/Bramble House/Season 2/Bramble House - 04 [1080p].mkv": "episode Bramble House S2E4",
		"TV/Hedgerow/Season 1/1.05 Hedge.mkv":                      "episode Hedgerow S1E5",
		"TV/Hedgerow/Season 2/2.01 - Gate.mkv":                     "episode Hedgerow S2E1",
		"Puddle Pals Season 2/01 Big Splash.mp4":                   "episode Puddle Pals S2E1",
		"Puddle Pals Season 2/02 Small Splash.mp4":                 "episode Puddle Pals S2E2",
		"Kids/Puddle Pups Series 1/01 Big Splash.mp4":              "episode Puddle Pups S1E1",
		"Kids/Puddle Pups Series 1/02 Small Splash.mp4":            "episode Puddle Pups S1E2",
		"Bramble Hill Season 2/Episode 3.mkv":                      "episode Bramble Hill S2E3",
		"TV/Thornbury/Season I/Thornbury - Hedge.mkv":              "episode Thornbury S?E?",
		"TV/Thornbury/Season I/Thornbury S01E07 - Sample.mkv":      "episode Thornbury S1E7",
	}},
	// Re-score #7: films with no "Movies" folder above them — sequels, a saga, parts, a film named
	// as its folder is — and a miniseries' "Part 1 of 3" loose at the top.
	{id: "W80", files: map[string]string{
		"Ironvale/Ironvale.mkv":                          "movie Ironvale -",
		"Ironvale/Ironvale 2.mkv":                        "movie Ironvale 2 -",
		"Ironvale/Ironvale 3.mkv":                        "movie Ironvale 3 -",
		"Kids/Pippin Paws/Pippin Paws.mp4":               "movie Pippin Paws -",
		"Kids/Pippin Paws/Pippin Paws 2.mp4":             "movie Pippin Paws 2 -",
		"Kids/Pippin Paws/Pippin Paws 3 - Snowed In.mp4": "movie Pippin Paws 3 - Snowed In -",
		"Gravelton Collection/Gravelton 1.mkv":           "movie Gravelton 1 -",
		"Gravelton Collection/Gravelton 2.mkv":           "movie Gravelton 2 -",
		"Thistledown Saga/1 - Thistledown.mkv":           "movie Thistledown -",
		"Thistledown Saga/2 - Thistledown Rises.mkv":     "movie Thistledown Rises -",
		"Old Stuff/Grimwold/Grimwold CD1.avi":            "movie Grimwold -",
		"Old Stuff/Grimwold/Grimwold CD2.avi":            "movie Grimwold -",
		"Larkspur/Larkspur - Disc 1.mkv":                 "movie Larkspur -",
		"Larkspur/Larkspur - Disc 2.mkv":                 "movie Larkspur -",
		"Kids/Series 7 (2001)/Series 7 (2001).mkv":       "movie Series 7 2001",
		"Episode 50 (2011)/Episode 50 (2011).mkv":        "movie Episode 50 2011",
		"Ashen Coast - Part 1 of 3.mkv":                  "episode Ashen Coast S1E1",
		"Ashen Coast - Part 2 of 3.mkv":                  "episode Ashen Coast S1E2",
	}},
	// Re-score #7: plural extras, extras named by their film or another, a release group's promo;
	// a dated daily show's file, a recording's day, a CJK episode.
	{id: "W81", files: map[string]string{
		"Movies/Glimmerfold (2011)/Glimmerfold (2011).mkv":               "movie Glimmerfold 2011",
		"Movies/Glimmerfold (2011)/Glimmerfold (2011) - Interviews.mkv":  "skip",
		"Movies/Glimmerfold (2011)/Glimmerfold (2011) - Trailers.mkv":    "skip",
		"Movies/Glimmerfold (2011)/Glimmerfold (2011) - Featurettes.mkv": "skip",
		"Movies/Glimmerfold (2011)/Glimmerfold Sequel Teaser.mkv":        "skip",
		"Movies/Glimmerfold (2011)/Ironvale Trailer.mkv":                 "skip",
		"TV/Nightly Ledger/2024-03-14.mkv":                               "episode Nightly Ledger 2024-03-14",
		"TV/Nightly Ledger/2024-03-15.mkv":                               "episode Nightly Ledger 2024-03-15",
		"Recorded TV/Wexcombe - Hedge (2023-01-02 21-00).ts":             "episode Wexcombe 2023-01-02",
		"Anime/[Kumo] Hoshi Kettle 第03話 [1080p].mkv":                     "episode Hoshi Kettle S?E?",
	}},
	// An episode whose title holds an extra's word is the episode: the kind must end the name, and
	// a dated name is an episode as a marked one is.
	{id: "W84", files: map[string]string{
		"TV/Hilltop/Season 1/Hilltop S01E01.mkv":                             "episode Hilltop S1E1",
		"TV/Hilltop/Season 1/Hilltop - Scene of the Crime.mkv":               "episode Hilltop S?E?",
		"TV/Hilltop/Season 1/Hilltop - Short Circuit.mkv":                    "episode Hilltop S?E?",
		"TV/Hilltop/Season 1/Hilltop - Other Worlds.mkv":                     "episode Hilltop S?E?",
		"TV/Hilltop/Season 1/Hilltop - Extra Time.mkv":                       "episode Hilltop S?E?",
		"TV/Hilltop/Season 1/Hilltop - Theme Park.mkv":                       "episode Hilltop S?E?",
		"TV/Hilltop/Season 1/Hilltop - Clip Show.mkv":                        "episode Hilltop S?E?",
		"TV/Night Talk/Night Talk S01E01.mkv":                                "episode Night Talk S1E1",
		"TV/Night Talk/Night Talk - 2023-05-13 - Interview with Jo Fern.mkv": "episode Night Talk 2023-05-13",
		"TV/Night Talk/Night.Talk.2023.05.15.Preview.Special.720p.mkv":       "episode Night Talk 2023-05-15",
		"TV/Quillon Hall/Quillon Hall S01E01.mkv":                            "episode Quillon Hall S1E1",
		"TV/Quillon Hall/Quillon Hall - 2018-04-02 - Recap.mkv":              "episode Quillon Hall 2018-04-02",
		"Movies/Glimmerdale (2012)/Glimmerdale (2012).mkv":                   "movie Glimmerdale 2012",
		"Movies/Glimmerdale (2012)/Glimmerdale (2012) - Trailer 2.mkv":       "skip",
		"Movies/Glimmerdale (2012)/Glimmerdale (2012) - Deleted Scenes.mkv":  "skip",
	}},
	// Anime under "Anime/" with its group's tag last or none; DVR recordings named by show and day.
	{id: "W85", files: map[string]string{
		"Anime/Kumo Tetsu - 07 (BD 1080p) [Grp].mkv":   "episode Kumo Tetsu S?E?",
		"Anime/Usagi Den - 06 [Grp][1080p].mkv":        "episode Usagi Den S?E?",
		"Anime/Usagi Den 07 [1080p].mkv":               "episode Usagi Den S?E?",
		"Anime/Usagi.Den.08.1080p.mkv":                 "episode Usagi Den S?E?",
		"Recordings/Harbour Watch 2022-01-05 21-00.ts": "episode Harbour Watch 2022-01-05",
		"Recordings/Harbour Watch 2022-01-12 21-00.ts": "episode Harbour Watch 2022-01-12",
		"Recorded TV/Ferry Talk 2022-01-05.ts":         "episode Ferry Talk 2022-01-05",
		"TV Recordings/Harbour Light - 20220105.ts":    "episode Harbour Light 2022-01-05",
		"Hana.Ame.Ep01.1080p.mkv":                      "episode Hana Ame S1E1",
	}},
	// A generic file name takes its folder's title; a show's seasons in folders of their own are
	// one show; a franchise's films named after it are films.
	{id: "W86", files: map[string]string{
		"Movies/Pikestaff/movie.mkv":             "movie Pikestaff -",
		"Movies/Pikemoor (2004)/1.mkv":           "movie Pikemoor 2004",
		"Pikefen/film.mkv":                       "movie Pikefen -",
		"Owlpen Season 1/Owlpen 1x01.avi":        "episode Owlpen S1E1",
		"Owlpen Season 2/Owlpen 2x01.avi":        "episode Owlpen S2E1",
		"TV/Owlbeck Season 1/Owlbeck 01.mkv":     "episode Owlbeck S1E1",
		"TV/Owlbeck Season 2/Owlbeck 01.mkv":     "episode Owlbeck S2E1",
		"Wren Lake/Wren Lake and the Stone.avi":  "movie Wren Lake and the Stone -",
		"Wren Lake/Wren Lake and the Cup.avi":    "movie Wren Lake and the Cup -",
		"Wren Lake/Wren Lake and the Prince.avi": "movie Wren Lake and the Prince -",
		"Movies/Thorn Hill 1x01.avi":             "episode Thorn Hill S1E1",
	}},
	// A season folder's "3 - 01" and "2-03"; a season run in brackets; Finnish seasons; a season's
	// extras folder named by the show.
	{id: "W87", files: map[string]string{
		"TV/Bramley Court/Season 3/Bramley Court - 3 - 01.mkv": "episode Bramley Court S3E1",
		"TV/Cobble Way/Season 2/2-03.mkv":                      "episode Cobble Way S2E3",
		"TV/Brine Sea (Season 1-3)/Brine Sea S02E01.mkv":       "episode Brine Sea S2E1",
		"TV/Satama/Kausi 1/Jakso 7.mkv":                        "episode Satama S1E7",
		"TV/Brine Rock/Brine Rock S01 Extras/Interview.mkv":    "skip",
		"TV/Brine Rock/Season 1/Brine Rock S01E01.mkv":         "episode Brine Rock S1E1",
	}},
	// Re-score #8's over-corrections: films kept with anime, a film recorded with no day, an episode
	// titled by a kind's word and more, a film's folder with an extras folder of its own name.
	{id: "W88", files: map[string]string{
		"Anime/Kumo Tetsu (2016)/Kumo Tetsu (2016).mkv":            "movie Kumo Tetsu 2016",
		"Anime/Hoshizora The Movie (2015).mkv":                     "movie Hoshizora The Movie 2015",
		"Recordings/Gravel Moor (2019).ts":                         "movie Gravel Moor 2019",
		"TV/Hilltop/Season 2/Hilltop S02E01.mkv":                   "episode Hilltop S2E1",
		"TV/Hilltop/Season 2/Hilltop - Trailer Park.mkv":           "episode Hilltop S?E?",
		"TV/Hilltop/Season 2/Hilltop - Interview with a Ghost.mkv": "episode Hilltop S?E?",
		"Movies/Sallowmere (2008)/Sallowmere (2008).mkv":           "movie Sallowmere 2008",
		"Movies/Sallowmere (2008)/Sallowmere Extras/Cast.mkv":      "skip",
		"Movies/Kettlewick (2013)/The Movie.mkv":                   "movie Kettlewick 2013",
	}},
	// Re-score #7's over-corrections: a film named by a kind in its own folder, a day's home video,
	// a show's folder holding one video named as the show, a DVR-less dated film, a pack with ids.
	{id: "W83", files: map[string]string{
		"Movies/Night Trailer (2010)/Night Trailer.mkv":    "movie Night Trailer 2010",
		"Home Videos/2019-06-01.mp4":                       "movie 2019-06-01 -",
		"Kids/Pebble Pals/Pebble Pals.mp4":                 "episode Pebble Pals S?E?",
		"Kids/Pebble Pals/Pebble Pals - Rainy Day.mp4":     "episode Pebble Pals S?E?",
		"Kids/Pebble Pals/Pebble Pals - Sunny Day.mp4":     "episode Pebble Pals S?E?",
		"Concerts/Ferrow Band - Live (2019).mkv":           "movie Ferrow Band - Live 2019",
		"Ashen Point Part 2 (2009)/Ashen Point Part 2.mkv": "movie Ashen Point Part 2 2009",
	}},
	// Re-score #7: one show in two release folders, or in a show's folder and a release's.
	{id: "W82", files: map[string]string{
		"Downloads/Bramble.House.S01.720p.WEB-DL/Bramble.House.S01E01.720p.WEB-DL.mkv": "episode Bramble House S1E1",
		"Downloads/Bramble.House.S02.720p.WEB-DL/Bramble.House.S02E01.720p.WEB-DL.mkv": "episode Bramble House S2E1",
		"TV/Kettle Lane/Season 1/Kettle Lane S01E01.mkv":                               "episode Kettle Lane S1E1",
		"TV/Kettle.Lane.S02.720p.WEB-DL/Kettle.Lane.S02E01.720p.WEB-DL.mkv":            "episode Kettle Lane S2E1",
		"Downloads/Hob.Lane.S01E03.720p.mkv":                                           "episode Hob Lane S1E3",
		"Downloads/HOB.LANE.S01E04.mkv":                                                "episode Hob Lane S1E4",
	}},
	// A franchise's folder holds its shows, each its own.
	{id: "W65", files: map[string]string{
		"TV/Star Ridge/Star Ridge Origins/Season 1/Star Ridge Origins S01E01.mkv": "episode Star Ridge Origins S1E1",
		"TV/Star Ridge/Star Ridge Origins/Season 1/Star Ridge Origins S01E02.mkv": "episode Star Ridge Origins S1E2",
		"TV/Star Ridge/Star Ridge Beyond/Season 1/Star Ridge Beyond S01E01.mkv":   "episode Star Ridge Beyond S1E1",
	}},
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
	// A show's episodes are one show's: the name it is listed by names one show, and an episode in
	// a second show of that name reads " (split)".
	showKeys, showOfRel := map[string]map[string]bool{}, map[string]string{}
	defer func() {
		for rel, show := range showOfRel {
			if len(showKeys[show]) > 1 {
				out[rel] += " (split)"
			}
		}
	}()
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
			if show := showNameOf(out[rel]); show != "" {
				showOfRel[rel] = show
				if showKeys[show] == nil {
					showKeys[show] = map[string]bool{}
				}
				showKeys[show][it.Series.Authority+" "+it.Series.Key] = true
			}
			if grounded {
				out[rel] += groundedSuffix(showOf, it)
			}
		}
	}
	return out
}

// showNameOf is the show an episode's outcome names, "" for any other outcome: "episode Kettle Bay
// S1E2" names "Kettle Bay".
func showNameOf(outcome string) string {
	rest, ok := strings.CutPrefix(outcome, "episode ")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(rest, " "); i > 0 {
		return rest[:i]
	}
	return rest
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
