// Package demolibrary is Loomarr's demo library: an invented catalogue, the artwork and video
// generated for it, and an Emby-compatible stand-in media server that serves both (#1587).
//
// It exists because the repo is public. Screenshots, docs clips, visual tests and demos need a
// library that looks real and plays for real, but must never show a real title, poster or logo.
// Every title below is invented; guard_test.go fails the build if a real one creeps in.
//
// Nothing here is committed media: the clips, posters, backdrops, icons and the watermark are all
// generated at setup time (assets.go) into a local directory, deterministically, from this file.
package demolibrary

import (
	"fmt"
	"strconv"
)

// ProviderIDBase offsets every demo provider id. Loomarr keys titles on TMDB/TVDB ids, and a
// real id would let a configured TMDB key fetch a REAL poster for an invented title. Ids from
// 90,000,000 are far above anything either service has issued, so a lookup finds nothing.
const ProviderIDBase = 90_000_000

// Kind is the media-server item type a title is served as.
type Kind string

const (
	Movie  Kind = "Movie"
	Series Kind = "Series"
)

// Format is one generated video file shape. Every item of a format plays the same file, so the
// runtime a title reports is exactly the file's duration and a scheduled programme never runs
// past the end of its media.
type Format struct {
	ID       string
	Label    string // burnt into the picture, e.g. "4K HDR10 · HEVC 10-bit"
	Width    int
	Height   int
	Codec    string // "h264" | "hevc"
	TenBit   bool
	HDR10    bool
	Duration int // seconds
}

// Formats are the generated clip shapes: several codecs and resolutions so playout, 4K/HDR and
// watermarks can be shown for real. Long runtimes are cheap: each file is a short encoded
// segment stream-copied end to end (assets.go).
var Formats = []Format{
	{ID: "cartoon-1080p-h264", Label: "1080p · H.264", Width: 1920, Height: 1080, Codec: "h264", Duration: 11 * 60},
	{ID: "episode-1080p-h264", Label: "1080p · H.264", Width: 1920, Height: 1080, Codec: "h264", Duration: 22 * 60},
	{ID: "episode-1080p-hevc10", Label: "1080p · HEVC 10-bit", Width: 1920, Height: 1080, Codec: "hevc", TenBit: true, Duration: 22 * 60},
	{ID: "episode-480p-4x3", Label: "480p · 4:3 SD", Width: 640, Height: 480, Codec: "h264", Duration: 22 * 60},
	{ID: "film-1080p-h264", Label: "1080p · H.264", Width: 1920, Height: 1080, Codec: "h264", Duration: 44 * 60},
	{ID: "film-2160p-hdr10", Label: "4K HDR10 · HEVC 10-bit", Width: 3840, Height: 2160, Codec: "hevc", TenBit: true, HDR10: true, Duration: 30 * 60},
}

// Filler is one generated interstitial (bumpers and idents between programmes). The stand-in
// serves them as its "Demo Filler" library; Loomarr's filler pipeline classifies and gates them
// like any library clip, so nothing here pre-decides a clip's kind or tags.
type Filler struct {
	ID       string
	Name     string
	Duration int // seconds
}

// Fillers are the generated interstitials. Every one clears the quality gate's 10-second floor.
var Fillers = []Filler{
	{ID: "bumper-stay-tuned", Name: "Stay tuned", Duration: 15},
	{ID: "bumper-up-next", Name: "Up next", Duration: 15},
	{ID: "ident-always-on", Name: "Always something on", Duration: 20},
}

// Title is one invented catalogue entry.
type Title struct {
	N        int // 1-based index within its kind; drives the id and provider id
	Kind     Kind
	Name     string
	Year     int
	Rating   string
	Genres   []string
	Overview string
	Format   string // Format.ID
	Episodes int    // series only
}

// ID is the stand-in server's item id.
func (t Title) ID() string {
	if t.Kind == Series {
		return fmt.Sprintf("demo-series-%02d", t.N)
	}
	return fmt.Sprintf("demo-film-%02d", t.N)
}

// ProviderID is the TVDB id (series) or TMDB id (film) the title is keyed on.
func (t Title) ProviderID() int { return ProviderIDBase + t.N }

// Key is the Loomarr provisioning key the title resolves to.
func (t Title) Key() string {
	if t.Kind == Series {
		return "series:tvdb:" + strconv.Itoa(t.ProviderID())
	}
	return "movie:tmdb:" + strconv.Itoa(t.ProviderID())
}

// Episode is one generated episode of a series.
type Episode struct {
	ID     string
	Name   string
	Season int
	Number int
}

// EpisodesOf lists a series' episodes: one season, invented episode names.
func EpisodesOf(t Title) []Episode {
	out := make([]Episode, 0, t.Episodes)
	for i := 1; i <= t.Episodes; i++ {
		out = append(out, Episode{
			ID:     fmt.Sprintf("%s-s01e%02d", t.ID(), i),
			Name:   episodeWords[(t.N*7+i*3)%len(episodeWords)] + " " + episodeNouns[(t.N*5+i)%len(episodeNouns)],
			Season: 1, Number: i,
		})
	}
	return out
}

var (
	episodeWords = []string{"The Borrowed", "A Very Loud", "The Upside-Down", "Nobody's", "The Seventh", "A Borrowed", "The Runaway", "The Quiet", "The Tangled", "An Unexpected"}
	episodeNouns = []string{"Teapot", "Lighthouse", "Weather Vane", "Accordion", "Map", "Doorbell", "Parade", "Postcard", "Staircase", "Kite", "Suitcase"}
)

func series(n int, name string, year int, rating, genre, format, overview string) Title {
	return Title{N: n, Kind: Series, Name: name, Year: year, Rating: rating, Genres: []string{genre}, Overview: overview, Format: format, Episodes: 6}
}

func film(n int, name string, year int, rating, genre, format, overview string) Title {
	return Title{N: n, Kind: Movie, Name: name, Year: year, Rating: rating, Genres: []string{genre}, Overview: overview, Format: format}
}

const (
	cartoon = "cartoon-1080p-h264"
	episode = "episode-1080p-h264"
	hevc10  = "episode-1080p-hevc10"
	sd4x3   = "episode-480p-4x3"
	film108 = "film-1080p-h264"
	uhdHDR  = "film-2160p-hdr10"
)

// Catalogue is the whole invented library: 40 series and 30 films. Append only; never renumber,
// because ids, provider ids and the docs' screenshots all derive from N.
var Catalogue = []Title{
	series(1, "Captain Pickle and the Brine Brigade", 1987, "TV-Y7", "Animation", cartoon, "A cucumber captain and his crew keep the harbour safe from soggy sandwiches."),
	series(2, "Robo-Rabbits of Zone Nine", 1991, "TV-Y7", "Animation", cartoon, "Four clockwork rabbits defend a vegetable patch on a far-off moon."),
	series(3, "The Marvelous Mudpuddles", 1984, "TV-Y", "Animation", cartoon, "A family of puddles who can jump into any picture book."),
	series(4, "Kettle Kids", 1995, "TV-Y7", "Animation", cartoon, "Three siblings shrink to the size of a teaspoon whenever the kettle whistles."),
	series(5, "Snorkelsaurus", 1989, "TV-Y", "Animation", cartoon, "A shy dinosaur explores the reef with a snorkel two sizes too small."),
	series(6, "Gadget Goats Go!", 1993, "TV-Y7", "Animation", cartoon, "Mountain goats with jetpacks run a delivery service for the whole valley."),
	series(7, "Professor Pumpernickel's Time Wagon", 1982, "TV-G", "Animation", cartoon, "A baker's cart that travels through time, one loaf at a time."),
	series(8, "Lunar Lemmings", 1998, "TV-Y7", "Animation", cartoon, "Small, brave and hopelessly lost on the dark side of the moon."),
	series(9, "Orbit of Quiet Machines", 2004, "TV-14", "Science Fiction", hevc10, "The last crew of a silent space station learns what the machines have been waiting for."),
	series(10, "The Halvorsen Drift", 2011, "TV-14", "Science Fiction", hevc10, "A cargo hauler drifts between colonies carrying a passenger nobody booked."),
	series(11, "Signal from Tessera", 1997, "TV-PG", "Science Fiction", hevc10, "Radio astronomers decode a message that answers questions they have not asked yet."),
	series(12, "Ferrocene Station", 2016, "TV-14", "Science Fiction", hevc10, "A mining outpost where the rock itself seems to remember."),
	series(13, "Glasshouse Nebula", 2008, "TV-PG", "Science Fiction", hevc10, "Botanists grow a forest inside a cloud of stellar gas."),
	series(14, "Cold Relay", 2019, "TV-MA", "Science Fiction", hevc10, "Relay operators at the edge of the network hear voices on a dead channel."),
	series(15, "Inspector Quillfeather", 1979, "TV-PG", "Mystery", episode, "A soft-spoken inspector solves village crimes with a notebook and a bicycle."),
	series(16, "The Lantern Street Files", 1994, "TV-14", "Mystery", episode, "Two night-shift archivists reopen cases nobody else remembers."),
	series(17, "Murmur Bay", 2013, "TV-14", "Mystery", episode, "Every winter, one house in a fishing town goes dark and someone vanishes."),
	series(18, "A Case for Mrs. Abernathy-Pike", 1988, "TV-PG", "Mystery", episode, "A retired crossword setter finds clues where the police find none."),
	series(19, "Fogbound Parish", 2002, "TV-14", "Mystery", episode, "A new vicar, an old feud and a fog that never quite lifts."),
	series(20, "The Wendelmeyer Family Hour", 1966, "TV-G", "Comedy", sd4x3, "Songs, sketches and one very patient dog."),
	series(21, "Bowling Alley Blues", 1974, "TV-PG", "Comedy", sd4x3, "The staff of a struggling bowling alley keep the lanes open against the odds."),
	series(22, "Hank & Delphine", 1971, "TV-G", "Comedy", sd4x3, "Newlyweds run a diner that is always one pie short."),
	series(23, "Cul-de-Sac Capers", 1978, "TV-G", "Comedy", sd4x3, "The kids of one short street turn every summer day into an adventure."),
	series(24, "Mister Fennimore's Neighborhood Garage", 1969, "TV-G", "Family", sd4x3, "A kindly mechanic fixes cars and answers children's questions."),
	series(25, "Copperline Foundry", 2010, "TV-14", "Drama", episode, "Three generations run a family foundry in a town that has stopped needing bells."),
	series(26, "Saltmarsh General", 2006, "TV-14", "Drama", episode, "The only hospital for fifty miles, and everyone in it knows everyone."),
	series(27, "The Understudies", 2018, "TV-14", "Comedy", episode, "Backstage at a theatre where the understudies are far better than the stars."),
	series(28, "Pocket Kingdom", 2021, "TV-G", "Documentary", episode, "The secret lives of the animals in a single hedgerow."),
	series(29, "Tidepool Diaries", 2015, "TV-G", "Documentary", episode, "One rock pool, filmed through a year of tides."),
	series(30, "Dust Road Marshal", 1962, "TV-PG", "Western", sd4x3, "A marshal with a stubborn mule keeps the peace on a road nobody maps."),
	series(31, "Grand Oak Hotel", 1999, "TV-PG", "Comedy", episode, "A family hotel where every guest arrives with a secret and a suitcase."),
	series(32, "Night Shift at Pellingham", 2009, "TV-14", "Comedy", episode, "The overnight crew of a regional airport that has one flight a week."),
	series(33, "The Beekeeper's Almanac", 2020, "TV-G", "Documentary", episode, "A year with the beekeepers of a mountain valley."),
	series(34, "Rivet & Rye", 2014, "TV-14", "Drama", episode, "Two rival distillers share a wall, a water source and a grudge."),
	series(35, "Paper Lanterns of Wexcombe", 2003, "TV-PG", "Drama", episode, "A lantern festival brings a scattered family home for one night a year."),
	series(36, "Stationary Bicycle Club", 2022, "TV-PG", "Comedy", episode, "Five neighbours training for a race none of them will ever enter."),
	series(37, "Harborlight Ferry", 1996, "TV-PG", "Drama", episode, "The crew and passengers of the last ferry across a northern sound."),
	series(38, "Moth & Moonbeam", 2017, "TV-Y7", "Animation", cartoon, "A moth and a moonbeam keep the night garden running until dawn."),
	series(39, "Quasar Quartet", 1986, "TV-Y7", "Animation", cartoon, "Four musicians tour the galaxy in a bus shaped like a trumpet."),
	series(40, "Crumb Street Bakery", 2012, "TV-G", "Comedy", episode, "An all-night bakery and the regulars who keep it going."),

	film(1, "The Clockmaker's Umbrella", 1958, "G", "Drama", film108, "A clockmaker builds an umbrella that stops the rain for exactly one minute."),
	film(2, "Seventeen Paper Cranes", 1983, "PG", "Drama", film108, "A girl folds paper cranes to count the days until her father comes home."),
	film(3, "Harvest of Brass", 1949, "G", "Drama", film108, "A village band saves its harvest fair with borrowed instruments."),
	film(4, "Rooftop Semaphore", 1976, "PG", "Comedy", film108, "Two apartment blocks conduct a feud entirely in flag signals."),
	film(5, "A Tuesday in Varnholm", 1964, "G", "Comedy", film108, "A travelling salesman arrives in a town where it is always Tuesday."),
	film(6, "Bellwether Junction", 1953, "G", "Western", film108, "A railway junction, a runaway flock and a stationmaster out of his depth."),
	film(7, "The Last Tram to Oddsbury", 1971, "PG", "Comedy", film108, "Strangers share the final tram before the line closes for good."),
	film(8, "Marigold Protocol", 1989, "PG-13", "Thriller", film108, "A florist discovers her orders are coded messages."),
	film(9, "Understory Hours", 2007, "PG-13", "Drama", film108, "A forest ranger spends one last season in the woods she grew up in."),
	film(10, "The Velvet Cartographer", 1995, "PG-13", "Mystery", film108, "A mapmaker's maps keep showing a street that does not exist."),
	film(11, "Beacon of the Hollow Moon", 1979, "PG", "Science Fiction", hevc10, "A lighthouse keeper on the moon tends a beacon nobody remembers building."),
	film(12, "Parallax Garden", 2002, "PG-13", "Science Fiction", hevc10, "A gardener finds a greenhouse that is a little bigger inside every day."),
	film(13, "Iron Lullaby", 2015, "PG-13", "Science Fiction", hevc10, "A retired robot nanny is called back for one last family."),
	film(14, "The Seventh Satellite Choir", 1992, "PG", "Science Fiction", hevc10, "Seven satellites begin to sing, and one astronomer learns the words."),
	film(15, "Ashgrove Anomaly", 2020, "R", "Science Fiction", hevc10, "A research town where the clocks run backwards after midnight."),
	film(16, "Aurora Over Kestrel Fjord", 2023, "G", "Documentary", uhdHDR, "One winter of northern lights over a remote fjord."),
	film(17, "Glacier Cathedral", 2021, "G", "Documentary", uhdHDR, "Inside the blue ice caves of a retreating glacier."),
	film(18, "Emberfall Canyon", 2024, "PG-13", "Adventure", uhdHDR, "Climbers race a wildfire season across a desert canyon."),
	film(19, "Coral Meridian", 2022, "G", "Documentary", uhdHDR, "A reef along the equator, filmed from dawn to dusk."),
	film(20, "Tin Soldier Waltz", 1961, "G", "Family", film108, "Toys in a closed shop hold a ball on the last night before the sale."),
	film(21, "Grandmother's Radio", 1987, "PG", "Family", film108, "An old radio picks up a station that plays tomorrow's news."),
	film(22, "Pepperpot Summer", 1998, "PG", "Family", film108, "Cousins spend a summer running a pepper farm stall."),
	film(23, "The Nightingale Ledger", 1946, "G", "Mystery", film108, "A bookkeeper finds a column of figures that adds up to a confession."),
	film(24, "Smoke over Calloway", 1957, "PG", "Western", film108, "A frontier town, a missing payroll and a sheriff on his first day."),
	film(25, "Quiet Engines", 2011, "PG-13", "Drama", film108, "A mechanic restores the car her brother never finished."),
	film(26, "Lemon Tree Heist", 2005, "PG-13", "Comedy", film108, "Retirees plan the theft of a prize-winning lemon tree."),
	film(27, "Pelican Point Motel", 1993, "PG", "Comedy", film108, "A roadside motel's staff try to impress a mysterious travel writer."),
	film(28, "Cider Press Sonata", 1977, "PG", "Drama", film108, "A pianist returns to the orchard where she learned to play."),
	film(29, "Midnight at the Mapleton", 1984, "PG-13", "Mystery", film108, "A hotel detective has until dawn to find a stolen violin."),
	film(30, "Thirteen Kites", 2009, "PG", "Family", film108, "A boy builds a kite for every week of his summer holiday."),
}

// Channel is one pre-approved demo channel. Numbers and names are cited by the docs ("channel
// 101"), so they are stable across releases: add channels, never renumber.
type Channel struct {
	Number    int
	Name      string
	Group     string
	Strategy  string // "shuffle" mixes a series channel's shows; "sequential" airs in lineup order
	Watermark bool
	Titles    []string // Title.ID()s, in lineup order
}

// Channels are the channels `make demo-seed` creates.
var Channels = []Channel{
	{Number: 101, Name: "Saturday Morning Cartoons", Group: "Kids", Strategy: "shuffle", Watermark: true,
		Titles: []string{"demo-series-01", "demo-series-02", "demo-series-03", "demo-series-05", "demo-series-38", "demo-series-39"}},
	{Number: 102, Name: "Late-Night Sci-Fi", Group: "Movies & Series", Strategy: "shuffle", Watermark: true,
		Titles: []string{"demo-series-09", "demo-series-11", "demo-series-14", "demo-film-11", "demo-film-13", "demo-film-14"}},
	{Number: 103, Name: "Sunday Matinee", Group: "Movies & Series", Strategy: "sequential",
		Titles: []string{"demo-film-01", "demo-film-03", "demo-film-05", "demo-film-06", "demo-film-20", "demo-film-23"}},
	{Number: 104, Name: "Premium 4K", Group: "Movies & Series", Strategy: "sequential",
		Titles: []string{"demo-film-16", "demo-film-17", "demo-film-18", "demo-film-19"}},
	{Number: 105, Name: "Retro Rewind", Group: "Classics", Strategy: "shuffle",
		Titles: []string{"demo-series-20", "demo-series-21", "demo-series-22", "demo-series-23", "demo-series-30"}},
	{Number: 106, Name: "Mystery Hour", Group: "Movies & Series", Strategy: "shuffle",
		Titles: []string{"demo-series-15", "demo-series-16", "demo-series-18", "demo-film-10", "demo-film-29"}},
}

// ByID finds a catalogue title by its item id.
func ByID(id string) (Title, bool) {
	for _, t := range Catalogue {
		if t.ID() == id {
			return t, true
		}
	}
	return Title{}, false
}

// FormatByID finds a generated format by id.
func FormatByID(id string) (Format, bool) {
	for _, f := range Formats {
		if f.ID == id {
			return f, true
		}
	}
	return Format{}, false
}
