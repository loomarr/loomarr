package scan

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Shelves: the catch-all folders a library sorts its titles onto ("TV/", "Movies/", "Anime/",
// "A-C/", "Downloads/"), and what a shelf above a folder says it holds.

// catchAllNames are folders that hold a library's files rather than name a show.
// Those that hold shows only are showsFolderNames, and films only filmsFolderNames; these hold
// either, or both.
var catchAllNames = map[string]bool{
	"media": true, "video": true, "videos": true, "downloads": true, "download": true,
	"complete": true, "completed": true, "finished": true, "unsorted": true, "new": true,
	"torrents": true, "torrent": true, "misc": true, "to sort": true, "tosort": true, "stuff": true,
	"inbox": true, "temp": true, "tmp": true, "various": true, "mixed": true, "library": true,
	// A household's folders by audience and genre, and by kind in its language.
	"kids": true, "children": true, "family": true, "documentaries": true, "docs": true,
	"documentary": true, "films et séries": true, "comedy": true,
	// A household's folders by what it has watched.
	"new episodes": true, "to watch": true, "towatch": true, "watchlist": true, "unwatched": true,
	"watched": true, "currently watching": true, "new shows": true, "ongoing": true, "ended": true,
}

// alphabetFolder matches a folder a library is shelved into by its titles' first letters: "C",
// "D-F", "#", "0-9". A show is never named by a letter alone, as a shelf is.
var alphabetFolder = regexp.MustCompile(`^(?:#|0-9|\pL|\pL ?- ?\pL)$`)

// showsFolderNames are catch-alls that hold shows: a folder in one is a show's.
var showsFolderNames = map[string]bool{
	"tv": true, "tv shows": true, "tvshows": true, "shows": true, "television": true, "anime": true,
	"series": true, "tv series": true, "serien": true, "séries": true, "cartoons": true, "kids tv": true,
	"miniseries": true, "mini series": true, "recorded tv": true, "kids shows": true, "all shows": true,
	"recordings": true, "tv recordings": true, "kids series": true, "dvr": true, "recorded shows": true,
	// A league's matches are kept as a daily show's episodes are, by their day.
	"sports": true, "sport": true,
}

// amongFilms says whether a folder, relative to the root, is a films' catch-all or in one: the
// nearest catch-all of a kind, itself or above it, is a films' ("Movies/", "Movies/Kids/",
// "Movies/Ferrow Lane (2019)/"), not a shows' ("Movies/TV/Kettle Bay/").
//
// A films' run's folder ("Thistledown Saga/") is among films unless a shows' catch-all holds it: a
// show may be called a saga ("TV/Lisk Saga/").
func amongFilms(dir string) bool {
	run := false
	for d := dir; d != "."; d = path.Dir(d) {
		switch key := folderKey(path.Base(d)); {
		case showsFolderNames[key], showLabelled(path.Base(d)):
			// A folder labelled a show's, "Movies/The Long Watch Miniseries/", is one on any shelf.
			return false
		case filmsFolderNames[key], franchiseFilms.MatchString(path.Base(d)):
			// So is a franchise's own, "TV/Ashwell Movies/", wherever it is kept.
			return true
		case filmSeries.MatchString(path.Base(d)):
			run = true
		}
	}
	return run
}

// rootShelf is a root's own name when it is a shelf of a kind or an audience's (walker.shelf): a
// household that adds "/media/TV" and "/media/Movies" as two roots keeps its shows and films apart
// by them, as one root's "TV/" and "Movies/" do.
func rootShelf(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if key := folderKey(base); showsFolderNames[key] || filmsFolderNames[key] || animeFolderNames[key] || audienceNames[key] {
		return base
	}
	return ""
}

// shelved is a folder of a file's, relative to the root, as the shelf predicates (underShows,
// amongFilms, inAnime) read it: under the root's own name when that is a shelf, so "Hop Glen" in a
// root named "TV" is read as "TV/Hop Glen".
func shelved(file File, dir string) string {
	if file.Notes.Root == "" {
		return dir
	}
	return path.Join(file.Notes.Root, dir)
}

// inAnime says whether a folder, relative to the root, is an anime catch-all or in one.
func inAnime(dir string) bool {
	for d := dir; d != "."; d = path.Dir(d) {
		if animeFolderNames[folderKey(path.Base(d))] {
			return true
		}
	}
	return false
}

// animeFolderNames are the catch-alls that hold anime.
var animeFolderNames = map[string]bool{"anime": true, "animes": true, "anime series": true, "アニメ": true}

// filmSeries matches a folder named as films' run is, its franchise's name then the word last:
// "Thistledown Saga", "Hazelgrot Trilogy (2001-2005)". A show's run is never called so; "Saga Two"
// is a title, and "Collection" may be a show's box set.
var filmSeries = regexp.MustCompile(`(?i)^.+[ ._-](?:saga|duology|trilogy|quadrilogy|tetralogy|pentalogy|hexalogy)(?:[ ._]*[(\[][^)\]]*[)\]])*$`)

// franchiseFilms matches a folder a franchise's films are kept in, its name then "Movies" or
// "Films": "Ashwell Movies", "Glen Hollow Films (2010-2018)".
var franchiseFilms = regexp.MustCompile(`(?i)^.+[ ._-](?:movies|films)(?:[ ._]*[(\[][^)\]]*[)\]])*$`)

// underShows says whether a folder, relative to the root, is in a catch-all that holds shows.
func underShows(dir string) bool {
	for d := path.Dir(dir); d != "."; d = path.Dir(d) {
		if showsFolderNames[folderKey(path.Base(d))] {
			return true
		}
	}
	return false
}

// catchAllFolder says whether a folder's name is a catch-all's.
func catchAllFolder(name string) bool {
	key := folderKey(name)
	return catchAllNames[key] || showsFolderNames[key] || filmsFolderNames[key] || alphabetFolder.MatchString(key)
}

// folderKey is a folder's name as the catch-all lists read it: its words, in lower case. A name
// the lists don't hold is read by its words when every one of them is a shelf's (shelfWords): one
// with a films' word is a films' shelf ("Christmas Movies", "Movies - Kids", "Movies2"), one with
// a shows' word a shows' ("TV Show", "TV 2"), and any other a catch-all ("DVD Rips", "Westerns").
func folderKey(name string) string {
	key := strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))), " ")
	if knownKey(key) {
		return key
	}
	words := strings.Fields(strings.ReplaceAll(key, "-", " "))
	if joined := strings.Join(words, " "); knownKey(joined) {
		return joined
	}
	// "Toddlers" and "Kids & Family" are an audience's shelf, as "Kids" is.
	if !slices.ContainsFunc(words, func(w string) bool { return !audienceWords[w] }) && len(words) > 0 {
		return "kids"
	}
	// A copy's word alone, "4K/" or "BluRay/" in a film's folder, is its version's folder.
	if !slices.ContainsFunc(words, func(w string) bool { return !copyWords[w] }) {
		return key
	}
	kind := "misc"
	for i, w := range words {
		// "Movies2" and "TV 2" are a second shelf of their kind.
		if m := shelfNumber.FindStringSubmatch(w); m != nil && m[1] != "" {
			w = m[1]
		} else if m != nil && i > 0 {
			continue
		}
		switch shelfWords[w] {
		case "":
			return key
		case "films":
			kind = "movies"
		case "shows":
			if kind != "movies" {
				kind = "tv"
			}
		}
	}
	if len(words) == 0 {
		return key
	}
	return kind
}

// knownKey says whether a folder's key is on a shelf list.
func knownKey(key string) bool {
	return catchAllNames[key] || showsFolderNames[key] || filmsFolderNames[key] || animeFolderNames[key] ||
		audienceNames[key] || contentShelves[key]
}

// audienceWords are the words an audience's shelf is named by.
var audienceWords = map[string]bool{
	"kids": true, "children": true, "family": true, "toddler": true, "toddlers": true, "&": true, "and": true,
	"childrens": true, "children's": true, "kid's": true, "kids'": true,
}

// copyWords name one copy's kind, as a film's version folder is named.
var copyWords = map[string]bool{"dvd": true, "bluray": true, "blu": true, "ray": true, "hd": true, "4k": true, "uhd": true}

// shelfNumber matches a shelf's word with its number after it, "movies2", or the number alone.
var shelfNumber = regexp.MustCompile(`^([a-z]*?)\d{1,2}$`)

// shelfWords are the words a household's shelf is named by, and the kind each says: a films',
// a shows', or neither (an audience, a genre, a disc's or a copy's word).
var shelfWords = map[string]string{
	"movies": "films", "movie": "films", "films": "films", "film": "films", "cinema": "films", "kino": "films",
	"filme": "films", "filmes": "films", "filmer": "films", "feature": "films", "features": "films",
	"tv": "shows", "shows": "shows", "show": "shows", "series": "shows", "television": "shows",
	"kids": "any", "children": "any", "family": "any", "toddler": "any", "toddlers": "any", "christmas": "any",
	"holiday": "any", "classic": "any", "classics": "any", "westerns": "any", "western": "any", "disney": "any",
	"pixar": "any", "horror": "any", "action": "any", "animated": "any", "animation": "any", "cartoon": "any",
	"dvd": "any", "dvds": "any", "rips": "any", "ripped": "any", "bluray": "any", "blu": "any", "ray": "any",
	"blurays": "any", "hd": "any", "4k": "any", "uhd": "any", "old": "any", "my": "any", "all": "any", "and": "any",
	"&": "any", "sci": "any", "fi": "any", "scifi": "any", "drama": "any", "thriller": "any", "thrillers": "any",
	"musicals": "any", "foreign": "any", "backup": "any", "backups": "any", "archive": "any", "more": "any",
}
