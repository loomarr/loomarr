package filler

import (
	"fmt"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/clipcatalog"
)

// GeographicScope is the clip catalog's broadcast reach (see clip.go for why filler names it).
type GeographicScope = clipcatalog.GeographicScope

const (
	GeographicUnknown  = clipcatalog.GeographicUnknown
	GeographicNational = clipcatalog.GeographicNational
	GeographicLocal    = clipcatalog.GeographicLocal
)

// Geography identifies the audience location a Channel serves: a country and, optionally, a Place
// (an installationlocation id) whose nearby broadcast stations are local to it (#2062).
type Geography struct {
	Country string `json:"country,omitempty"`
	Place   string `json:"place,omitempty"`
	// Fallback is the installation's ordered region preference after Country (#2035): countries
	// whose national commercials air, and whose sources fetch, behind the home country's. It is
	// the installation's alone, so a channel never stores one; a channel inherits it.
	//
	// ⚠ Comma-joined ("GB,CA,AU"), not a slice, so Geography stays comparable: channels, sources
	// and their tests compare geographies with ==.
	//
	// ⚠ Not serialised. Anything persisted with a Geography (a pull's intent) drops it, so a
	// stored geography must never be trusted to carry the preference: read the live home again
	// (pull approval re-checks every source against it).
	Fallback string `json:"-"`
}

func (g Geography) Normalize() Geography {
	g.Country = strings.ToUpper(strings.TrimSpace(g.Country))
	g.Place = strings.TrimSpace(g.Place)
	g.Fallback = strings.Join(fallbackCountries(g.Fallback, g.Country), ",")
	return g
}

// fallbackCountries reads a fallback list, "gb, CA,AU": upper-cased and trimmed, each country once,
// in order, and never the home country, which already comes first.
func fallbackCountries(raw, home string) []string {
	var out []string
	for _, c := range strings.Split(raw, ",") {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" && c != home && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func (g Geography) fallsBackTo(country string) bool {
	return country != "" && slices.Contains(fallbackCountries(g.Fallback, g.Country), country)
}

// RegionRank orders a country by the installation's preference: 0 for home (or no boundary), then
// each fallback country in turn. A country outside both ranks last.
func (g Geography) RegionRank(country string) int { return g.regionRanker()(country) }

// regionRanker is RegionRank with the preference parsed once, for ranking a whole catalog.
func (g Geography) regionRanker() func(country string) int {
	home := strings.ToUpper(strings.TrimSpace(g.Country))
	fallback := fallbackCountries(g.Fallback, home)
	return func(country string) int {
		country = strings.ToUpper(strings.TrimSpace(country))
		if home == "" || country == home {
			return 0
		}
		if i := slices.Index(fallback, country); i >= 0 {
			return i + 1
		}
		return len(fallback) + 1
	}
}

func (g Geography) Validate() error {
	g = g.Normalize()
	if g.Country == "" {
		if g.Place != "" {
			return fmt.Errorf("place requires a country")
		}
		return nil
	}
	if len(g.Country) != 2 || g.Country[0] < 'A' || g.Country[0] > 'Z' || g.Country[1] < 'A' || g.Country[1] > 'Z' {
		return fmt.Errorf("country %q must be an ISO 3166-1 alpha-2 code", g.Country)
	}
	return nil
}

// LocalStations returns the ids of the broadcast stations within the local radius of a place in
// country, or nil when the place is unknown or not in that country.
type LocalStations func(country, place string) map[string]bool

// Area is a resolved Geography: its country, its fallback countries, and the broadcast stations
// local to its place.
type Area struct {
	Country  string
	Fallback string
	Stations map[string]bool
}

func (a Area) fallsBackTo(country string) bool {
	return Geography{Country: a.Country, Fallback: a.Fallback}.fallsBackTo(country)
}

// Resolve turns a Geography into the Area eligibility checks. A nil resolver or an unknown place
// leaves no local stations, so local clips fail closed.
func (g Geography) Resolve(local LocalStations) Area {
	g = g.Normalize()
	area := Area{Country: g.Country, Fallback: g.Fallback}
	if local != nil && g.Country != "" && g.Place != "" {
		area.Stations = local(g.Country, g.Place)
	}
	return area
}

// GeographicallyEligible applies the hard boundary before every matching rung.
// An empty target preserves legacy behaviour until Installation geography is set.
func GeographicallyEligible(c Clip, target Area) bool {
	if target.Country == "" {
		return true
	}
	country := strings.ToUpper(strings.TrimSpace(c.Country))
	switch c.GeographicScope {
	case GeographicNational:
		// A fallback country's national ads air; its local ones are for someone else's town.
		return country == target.Country || target.fallsBackTo(country)
	case GeographicLocal:
		return country == target.Country && target.Stations[c.Station]
	default:
		return false
	}
}

// SourceGeographicallyEligible determines whether an acquisition source can contribute to an
// installation: it must serve the installation's country. Once an installation country is
// configured, an unclassified source is not safe for unattended pulls.
func SourceGeographicallyEligible(source, target Geography) bool {
	target = target.Normalize()
	if target.Country == "" {
		return true
	}
	source = source.Normalize()
	return source.Country != "" && (source.Country == target.Country || target.fallsBackTo(source.Country))
}

// effectiveGeography is the channel's own location when it names one, else the household's (#2030
// decision 16). The channel's country replaces the household's, so a channel of British shows in a
// US household can opt into British commercials; it is never inferred from where the shows are
// from. The region preference stays the household's (#2035): a channel inherits it, and Normalize
// drops the channel's own country from it.
//
// An empty place inherits the household's only within the household's country (#2059 decision 8):
// a household city abroad would put foreign stations in reach, so a channel of another country
// without a place of its own airs no local clips.
func effectiveGeography(channel, installation Geography) Geography {
	channel, installation = channel.Normalize(), installation.Normalize()
	if channel.Country == "" {
		return installation
	}
	channel.Fallback = installation.Fallback
	if channel.Place == "" && channel.Country == installation.Country {
		channel.Place = installation.Place
	}
	return channel.Normalize()
}

// area is the resolved boundary a channel's breaks are limited to.
func (p Policy) area(channel Geography) Area {
	return effectiveGeography(channel, p.Geography).Resolve(p.LocalStations)
}

func filterGeography(clips []Clip, target Area) []Clip {
	if target.Country == "" {
		return clips
	}
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if GeographicallyEligible(c, target) {
			out = append(out, c)
		}
	}
	return out
}
