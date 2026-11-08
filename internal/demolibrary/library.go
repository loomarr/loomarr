package demolibrary

import (
	"fmt"
	"path/filepath"
	"sync"
)

// Content picks which library the stand-in serves.
type Content string

const (
	// ContentInvented is the invented catalogue in catalog.go, with generated media. Tests,
	// screenshots and the docs use it, and it is the default everywhere.
	ContentInvented Content = "invented"
	// ContentOpen is the open content pack (openpack.json): real, openly licensed films, episodes
	// and vintage commercials, downloaded and hash-checked at setup. Only the public demo site uses
	// it, so visitors see real pictures instead of test cards.
	ContentOpen Content = "open"
)

// ParseContent reads DEMO_CONTENT / --content. Empty is the invented library.
func ParseContent(s string) (Content, error) {
	switch Content(s) {
	case "", ContentInvented:
		return ContentInvented, nil
	case ContentOpen:
		return ContentOpen, nil
	}
	return "", fmt.Errorf("unknown demo content %q (want %q or %q)", s, ContentInvented, ContentOpen)
}

// Library is one servable demo library: its titles, episodes, interstitials and channels, and the
// shape of every playable file. The stand-in server, the asset step and the seed all read a
// Library, so the invented catalogue and the open pack run through the same code.
type Library struct {
	Content  Content
	Titles   []Title
	Fillers  []Filler
	Channels []Channel

	episodes map[string][]Episode // series id → its episodes
	shapes   map[string]Format    // playable item id (film, episode, filler) → its file's shape
	pack     *Pack                // the open pack's pinned files; nil for the invented library
}

var (
	inventedOnce sync.Once
	invented     *Library
)

// Invented is the invented catalogue as a Library.
func Invented() *Library {
	inventedOnce.Do(func() {
		lib := &Library{Content: ContentInvented, Titles: Catalogue, Fillers: Fillers, Channels: Channels,
			episodes: map[string][]Episode{}, shapes: map[string]Format{}}
		for _, t := range Catalogue {
			f, ok := FormatByID(t.Format)
			if !ok {
				panic(fmt.Sprintf("demo title %s names unknown format %q", t.ID(), t.Format))
			}
			if t.Kind == Movie {
				lib.shapes[t.ID()] = f
				continue
			}
			eps := EpisodesOf(t)
			lib.episodes[t.ID()] = eps
			for _, e := range eps {
				lib.shapes[e.ID] = f
			}
		}
		for _, f := range Fillers {
			lib.shapes[f.ID] = Format{Width: 1920, Height: 1080, Codec: "h264", Duration: f.Duration}
		}
		invented = lib
	})
	return invented
}

// Load returns the library for a content mode. The open pack is parsed and validated from the
// embedded manifest, so a malformed pack fails here, before anything is downloaded.
func Load(c Content) (*Library, error) {
	switch c {
	case ContentInvented, "":
		return Invented(), nil
	case ContentOpen:
		p, err := EmbeddedPack()
		if err != nil {
			return nil, err
		}
		return p.Library(), nil
	}
	return nil, fmt.Errorf("unknown demo content %q", c)
}

// ByID finds a title by its item id.
func (lib *Library) ByID(id string) (Title, bool) {
	for _, t := range lib.Titles {
		if t.ID() == id {
			return t, true
		}
	}
	return Title{}, false
}

// EpisodesOf lists a series' episodes.
func (lib *Library) EpisodesOf(t Title) []Episode { return lib.episodes[t.ID()] }

// Shape is the file shape of a playable item (film, episode or filler clip).
func (lib *Library) Shape(itemID string) (Format, bool) {
	f, ok := lib.shapes[itemID]
	return f, ok
}

// FillerPath is where a filler clip's file lives.
func (lib *Library) FillerPath(l Layout, id string) string {
	if lib.pack != nil {
		return l.PackFiller(id)
	}
	return l.Filler(id)
}

// FillerDir is the folder the filler library reports as its location.
func (lib *Library) FillerDir(l Layout) string { return filepath.Dir(lib.FillerPath(l, "x")) }
