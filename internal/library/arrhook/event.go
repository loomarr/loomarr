// Package arrhook speaks Sonarr's and Radarr's webhooks (#2151 decisions 9 and 10): it reads the
// events they send, naming the folders each one touched, and registers Loomarr as one of their
// connections so they send them.
package arrhook

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
)

// Event is one webhook delivery, reduced to what the scanner needs.
type Event struct {
	// Type is the arr's eventType: Download, Rename, EpisodeFileDelete, MovieDelete, Test, …
	Type string
	// Folders are the folders the event touched, in the arr's own paths, sorted. Empty for an
	// event that changes no file (Grab, Health, Test).
	Folders []string
}

// IsTest reports whether the arr sent its connection test.
func (e Event) IsTest() bool { return e.Type == "Test" }

type file struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
}

type renamed struct {
	Path         string `json:"path"`
	PreviousPath string `json:"previousPath"`
}

// payload is the union of Sonarr's and Radarr's webhook bodies, as far as files go.
type payload struct {
	EventType string `json:"eventType"`
	Series    *struct {
		Path string `json:"path"`
	} `json:"series"`
	Movie *struct {
		FolderPath string `json:"folderPath"`
	} `json:"movie"`
	EpisodeFile         *file     `json:"episodeFile"`
	EpisodeFiles        []file    `json:"episodeFiles"`
	MovieFile           *file     `json:"movieFile"`
	DeletedFiles        []file    `json:"deletedFiles"`
	RenamedEpisodeFiles []renamed `json:"renamedEpisodeFiles"`
	RenamedMovieFiles   []renamed `json:"renamedMovieFiles"`
}

// wholeFolder are the events that change the series' or movie's folder itself rather than one file
// in it: a delete may take the folder away, and a rename may move files between season folders.
var wholeFolder = map[string]bool{
	"SeriesDelete": true, "MovieDelete": true, "Rename": true,
}

// Parse reads a webhook body.
func Parse(body []byte) (Event, error) {
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("arrhook: %w", err)
	}
	if p.EventType == "" {
		return Event{}, fmt.Errorf("arrhook: no eventType")
	}
	base := ""
	switch {
	case p.Series != nil:
		base = p.Series.Path
	case p.Movie != nil:
		base = p.Movie.FolderPath
	}
	folders := map[string]bool{}
	addFile := func(f file) {
		p := f.Path
		if p == "" && f.RelativePath != "" && base != "" {
			p = path.Join(slashed(base), slashed(f.RelativePath))
		}
		if p != "" {
			folders[path.Dir(slashed(p))] = true
		}
	}
	for _, f := range p.EpisodeFiles {
		addFile(f)
	}
	for _, f := range p.DeletedFiles {
		addFile(f)
	}
	for _, f := range []*file{p.EpisodeFile, p.MovieFile} {
		if f != nil {
			addFile(*f)
		}
	}
	for _, r := range slices.Concat(p.RenamedEpisodeFiles, p.RenamedMovieFiles) {
		addFile(file{Path: r.Path})
		addFile(file{Path: r.PreviousPath})
	}
	if wholeFolder[p.EventType] && base != "" {
		folders[path.Clean(slashed(base))] = true
	}
	out := make([]string, 0, len(folders))
	for f := range folders {
		out = append(out, f)
	}
	slices.Sort(out)
	return Event{Type: p.EventType, Folders: out}, nil
}

// slashed is p with forward slashes: an arr on Windows names folders with backslashes, and the
// path map compares slash paths.
func slashed(p string) string { return strings.ReplaceAll(p, `\`, "/") }
