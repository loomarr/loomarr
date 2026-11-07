package filler

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The curated sources are Loomarr's vetted list of archive.org collections and YouTube channels
// that carry commercials (#2035, decision 0045). Each upgrade offers an install the entries it has
// not been offered yet, switched off: enabling one is the operator's consent to fetch from it. A
// scheduled CI job (scripts/check-curated-sources.sh) checks every entry against the live
// provider, so a removed or renamed one fails the build rather than an install.
//
//go:embed curated/sources.json
var curatedJSON []byte

// CuratedList is one version of the curated sources.
type CuratedList struct {
	Version string          `json:"version"`
	Sources []CuratedSource `json:"sources"`
}

// CuratedSource is one vetted source. Target is the provider's canonical form, the one adding the
// source by hand would store: an archive.org identifier, or a YouTube channel's
// https://www.youtube.com/channel/<id>/videos.
type CuratedSource struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Label  string `json:"label"`
	// EraFrom and EraTo bound the years its commercials come from, when known; zero is unknown.
	EraFrom int `json:"eraFrom,omitempty"`
	EraTo   int `json:"eraTo,omitempty"`
	// Country is where its commercials aired, when the source is of one country. Empty is mixed or
	// undeclared, and asserts nothing about its clips.
	Country string `json:"country,omitempty"`
	// Stations are the broadcast stations whose material the source mostly holds, by station id
	// (#2059 decision 10). They choose only where to look: a household near one of them gets the
	// source switched on, while each clip's locality is still decided on arrival. Empty for a source
	// that is not one market's.
	Stations []string `json:"stations,omitempty"`
	Notes    string   `json:"notes"`
}

// SourceID is the row id adding the source by hand would give it, so an operator who already
// added one is not offered it twice.
func (c CuratedSource) SourceID() string { return c.Kind + ":" + c.Target }

// CuratedSources returns the curated list this build ships.
func CuratedSources() (CuratedList, error) {
	var list CuratedList
	if err := json.Unmarshal(curatedJSON, &list); err != nil {
		return CuratedList{}, fmt.Errorf("read curated sources: %w", err)
	}
	if list.Version == "" {
		return CuratedList{}, fmt.Errorf("curated sources have no version")
	}
	seen := map[string]bool{}
	for _, s := range list.Sources {
		switch {
		case s.Kind != "archive" && s.Kind != "youtube":
			return CuratedList{}, fmt.Errorf("curated source %q: kind %q is not archive or youtube", s.Target, s.Kind)
		case s.Target == "" || s.Label == "" || s.Notes == "":
			return CuratedList{}, fmt.Errorf("curated source %q: target, label and notes are required", s.Target)
		case seen[s.SourceID()]:
			return CuratedList{}, fmt.Errorf("curated source %q is listed twice", s.SourceID())
		case s.EraFrom != 0 && s.EraTo != 0 && s.EraFrom > s.EraTo:
			return CuratedList{}, fmt.Errorf("curated source %q: era %d-%d is backwards", s.Target, s.EraFrom, s.EraTo)
		}
		if err := (Geography{Country: s.Country}).Validate(); err != nil {
			return CuratedList{}, fmt.Errorf("curated source %q: %w", s.Target, err)
		}
		for i, id := range s.Stations {
			if !strings.HasPrefix(id, "fcc:") && !strings.HasPrefix(id, "wd:") || slices.Contains(s.Stations[:i], id) {
				return CuratedList{}, fmt.Errorf("curated source %q: station %q is not a distinct fcc: or wd: id", s.Target, id)
			}
		}
		seen[s.SourceID()] = true
	}
	return list, nil
}
