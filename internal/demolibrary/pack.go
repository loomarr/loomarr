package demolibrary

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// The open content pack: real, openly licensed media for the public demo site only (#2268).
//
// openpack.json pins every file by URL, SHA-256 and byte size, and records where each came from
// and under which licence. No media is committed: the asset step downloads and verifies the files
// (download.go), and the posters and backdrops are frames cut from the videos themselves.
//
// It is the only place in this package allowed to name real titles; guard_test.go exempts exactly
// this file, and a pack test checks every item's licence, hash, size and all-ages flag.

//go:embed openpack.json
var openPackJSON []byte

// PackVersion is the manifest schema the code reads. Bump it with a schema change.
const PackVersion = 1

// MaxPackBytes caps the pack's total download, so a careless addition cannot turn a demo boot into
// a multi-gigabyte pull.
const MaxPackBytes = 4 << 30

// Licences are the only licences a pack file may carry: Creative Commons Attribution (no NC, no
// ND, no SA) and public domain. The value is how the overview credits it.
var Licences = map[string]string{
	"https://creativecommons.org/licenses/by/3.0/":       "CC BY 3.0",
	"https://creativecommons.org/licenses/by/4.0/":       "CC BY 4.0",
	"https://creativecommons.org/publicdomain/mark/1.0/": "Public Domain Mark 1.0",
	"https://creativecommons.org/licenses/publicdomain/": "Public Domain",
}

// Pack is the parsed manifest.
type Pack struct {
	Version int `json:"version"`
	// Mirror is a base URL holding every pinned file under its SHA-256 (a loomarr/loomarr release,
	// scripts/mirror-demo-pack.sh). The downloader tries it before each file's own URL.
	Mirror   string      `json:"mirror,omitempty"`
	Titles   []PackTitle `json:"titles"`
	Fillers  []PackClip  `json:"fillers"`
	Channels []Channel   `json:"channels"`
}

// PackTitle is one film (Media) or series (Episodes).
type PackTitle struct {
	N        int           `json:"n"`
	Kind     Kind          `json:"kind"`
	Name     string        `json:"name"`
	Year     int           `json:"year"`
	Rating   string        `json:"rating"` // the pack's own all-ages classification, not an official rating
	Genres   []string      `json:"genres"`
	Overview string        `json:"overview"`
	Media    *PackMedia    `json:"media,omitempty"`
	Episodes []PackEpisode `json:"episodes,omitempty"`
	// PosterAt and BackdropAt are the seconds the poster (2:3) and backdrop (16:9) frames are cut
	// at, from the film or a series' first episode, picked by eye so each crop holds its subject.
	PosterAt   float64 `json:"posterAt"`
	BackdropAt float64 `json:"backdropAt"`
}

// artMedia is the file the title's poster and backdrop are cut from.
func (t PackTitle) artMedia() PackMedia {
	if t.Media != nil {
		return *t.Media
	}
	if len(t.Episodes) > 0 {
		return t.Episodes[0].Media
	}
	return PackMedia{}
}

// PackEpisode is one episode of a pack series, in airing order.
type PackEpisode struct {
	Name     string    `json:"name"`
	Year     int       `json:"year"`
	Overview string    `json:"overview"`
	Media    PackMedia `json:"media"`
}

// PackClip is one interstitial. Its id follows the filler filename convention (a kind token and
// a year), so the pipeline enrols it as a commercial or trailer with an era and no model.
type PackClip struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Media PackMedia `json:"media"`
}

// PackMedia is one pinned file and its provenance.
type PackMedia struct {
	URL     string  `json:"url"`
	SHA256  string  `json:"sha256"`
	Size    int64   `json:"size"`
	Seconds float64 `json:"seconds"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Codec   string  `json:"codec"` // video: h264 | hevc
	Audio   string  `json:"audio"` // aac | mp3
	Licence string  `json:"licence"`
	Source  string  `json:"source"` // the page the file and its licence were verified on
	Credit  string  `json:"credit"`
	AllAges bool    `json:"allAges"`
}

// EmbeddedPack parses and validates the committed manifest.
func EmbeddedPack() (*Pack, error) { return ParsePack(openPackJSON) }

// ParsePack parses and validates a manifest. Unknown fields fail, so a typo cannot silently drop
// a licence or a hash.
func ParsePack(raw []byte) (*Pack, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var p Pack
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("open pack: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("open pack: %w", err)
	}
	return &p, nil
}

var (
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
	clipID    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// Validate reports every problem in the manifest at once.
func (p *Pack) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if p.Version != PackVersion {
		bad("version %d, want %d", p.Version, PackVersion)
	}
	// A file's mirror URL is the base plus its SHA-256.
	if p.Mirror != "" && (!strings.HasPrefix(p.Mirror, "https://") || !strings.HasSuffix(p.Mirror, "/")) {
		bad("mirror %q must be an https:// base URL ending in /", p.Mirror)
	}
	if len(p.Titles) == 0 || len(p.Fillers) == 0 || len(p.Channels) == 0 {
		bad("needs titles, fillers and channels")
	}
	var total int64
	media := func(where string, m PackMedia) {
		total += m.Size
		for _, e := range m.problems() {
			bad("%s: %s", where, e)
		}
	}
	ids := map[string]bool{}
	for _, t := range p.Titles {
		id := t.title().ID()
		if ids[id] {
			bad("duplicate title %s", id)
		}
		ids[id] = true
		if t.N < 1 || t.Name == "" || t.Year < 1890 || t.Overview == "" || len(t.Genres) == 0 || t.Rating == "" {
			bad("%s: needs n ≥ 1, a name, year, rating, genres and overview", id)
		}
		if art := t.artMedia(); t.PosterAt < 0 || t.BackdropAt < 0 || t.PosterAt >= art.Seconds || t.BackdropAt >= art.Seconds {
			bad("%s: posterAt and backdropAt must fall inside the video they are cut from", id)
		}
		switch t.Kind {
		case Movie:
			if t.Media == nil || len(t.Episodes) != 0 {
				bad("%s: a film has media and no episodes", id)
				continue
			}
			media(id, *t.Media)
		case Series:
			if t.Media != nil || len(t.Episodes) == 0 {
				bad("%s: a series has episodes and no media of its own", id)
			}
			for i, e := range t.Episodes {
				where := fmt.Sprintf("%s episode %d", id, i+1)
				if e.Name == "" || e.Year < 1890 || e.Overview == "" {
					bad("%s: needs a name, year and overview", where)
				}
				media(where, e.Media)
			}
		default:
			bad("%s: kind %q, want Movie or Series", id, t.Kind)
		}
	}
	for _, f := range p.Fillers {
		if !clipID.MatchString(f.ID) || ids[f.ID] || f.Name == "" {
			bad("filler %q: needs a unique lowercase id and a name", f.ID)
		}
		ids[f.ID] = true
		// The filler quality gate holds anything under 10 seconds; such a clip would never air.
		if f.Media.Seconds < 10 {
			bad("filler %s: %.1fs is under the quality gate's 10-second floor", f.ID, f.Media.Seconds)
		}
		media("filler "+f.ID, f.Media)
	}
	numbers := map[int]bool{}
	for _, c := range p.Channels {
		if numbers[c.Number] || c.Number < 1 || c.Name == "" || len(c.Titles) == 0 {
			bad("channel %d: needs a unique number, a name and titles", c.Number)
		}
		numbers[c.Number] = true
		for _, id := range c.Titles {
			if !ids[id] || !strings.HasPrefix(id, "open-") {
				bad("channel %d lists unknown title %s", c.Number, id)
			}
		}
	}
	if total > MaxPackBytes {
		bad("total download %d bytes exceeds %d", total, int64(MaxPackBytes))
	}
	return errors.Join(errs...)
}

func (m PackMedia) problems() []string {
	var out []string
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || !strings.HasSuffix(strings.ToLower(u.Path), ".mp4") {
		out = append(out, "url must be an https .mp4 file")
	}
	if !sha256Hex.MatchString(m.SHA256) {
		out = append(out, "sha256 must be 64 lowercase hex digits")
	}
	if m.Size <= 0 {
		out = append(out, "size must be the file's byte count")
	}
	if m.Seconds <= 0 || m.Width <= 0 || m.Height <= 0 {
		out = append(out, "seconds, width and height must be the probed shape")
	}
	if m.Codec != "h264" && m.Codec != "hevc" {
		out = append(out, fmt.Sprintf("video codec %q, want h264 or hevc", m.Codec))
	}
	if m.Audio != "aac" && m.Audio != "mp3" {
		out = append(out, fmt.Sprintf("audio codec %q, want aac or mp3", m.Audio))
	}
	if _, ok := Licences[m.Licence]; !ok {
		out = append(out, fmt.Sprintf("licence %q is not CC BY or public domain", m.Licence))
	}
	if s, err := url.Parse(m.Source); err != nil || s.Scheme != "https" || s.Host == "" {
		out = append(out, "source must be the https page the licence was verified on")
	}
	if m.Credit == "" {
		out = append(out, "credit is required")
	}
	if !m.AllAges {
		out = append(out, "the public demo is all-ages only")
	}
	return out
}

// attribution is the credit line every overview ends with.
func (m PackMedia) attribution() string {
	return fmt.Sprintf("%s Licence: %s (%s). Source: %s", m.Credit, Licences[m.Licence], m.Licence, m.Source)
}

// format is the file's shape as the stand-in reports it. Whole seconds, rounded down, so a
// scheduled programme never runs past the end of its media.
func (m PackMedia) format() Format {
	return Format{Label: fmt.Sprintf("%dp", m.Height), Width: m.Width, Height: m.Height, Codec: m.Codec, Audio: m.Audio, Duration: int(m.Seconds)}
}

func (t PackTitle) title() Title {
	return Title{N: t.N, Kind: t.Kind, Name: t.Name, Year: t.Year, Rating: t.Rating, Genres: t.Genres, Overview: t.Overview, Open: true}
}

// Library is the pack as a servable library. Overviews carry each title's credit and licence,
// which is how the demo attributes its content without any new UI.
func (p *Pack) Library() *Library {
	lib := &Library{Content: ContentOpen, Channels: p.Channels, episodes: map[string][]Episode{}, shapes: map[string]Format{}, pack: p}
	for _, pt := range p.Titles {
		t := pt.title()
		if pt.Kind == Movie {
			t.Overview += "\n\n" + pt.Media.attribution()
			lib.shapes[t.ID()] = pt.Media.format()
		} else {
			t.Episodes = len(pt.Episodes)
			t.Overview += "\n\n" + pt.Episodes[0].Media.attribution()
			for i, pe := range pt.Episodes {
				e := Episode{ID: episodeID(t, i+1), Name: pe.Name, Season: 1, Number: i + 1,
					Year: pe.Year, Overview: pe.Overview + "\n\n" + pe.Media.attribution()}
				lib.episodes[t.ID()] = append(lib.episodes[t.ID()], e)
				lib.shapes[e.ID] = pe.Media.format()
			}
		}
		lib.Titles = append(lib.Titles, t)
	}
	for _, f := range p.Fillers {
		lib.Fillers = append(lib.Fillers, Filler{ID: f.ID, Name: f.Name, Duration: int(f.Media.Seconds)})
		lib.shapes[f.ID] = f.Media.format()
	}
	return lib
}

// packFile is one file the asset step downloads, with the item id it is stored under.
type packFile struct {
	itemID string
	filler bool
	media  PackMedia
}

// files lists every pinned file in manifest order.
func (p *Pack) files() []packFile {
	var out []packFile
	for _, pt := range p.Titles {
		t := pt.title()
		if pt.Media != nil {
			out = append(out, packFile{itemID: t.ID(), media: *pt.Media})
		}
		for i, e := range pt.Episodes {
			out = append(out, packFile{itemID: episodeID(t, i+1), media: e.Media})
		}
	}
	for _, f := range p.Fillers {
		out = append(out, packFile{itemID: f.ID, filler: true, media: f.Media})
	}
	return out
}
