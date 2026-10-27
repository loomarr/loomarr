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

// Geography identifies the audience location a Channel serves.
type Geography struct {
	Country string `json:"country,omitempty"`
	Market  string `json:"market,omitempty"`
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
	g.Market = strings.Join(strings.Fields(strings.TrimSpace(g.Market)), " ")
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
		if g.Market != "" {
			return fmt.Errorf("market requires a country")
		}
		return nil
	}
	if len(g.Country) != 2 || g.Country[0] < 'A' || g.Country[0] > 'Z' || g.Country[1] < 'A' || g.Country[1] > 'Z' {
		return fmt.Errorf("country %q must be an ISO 3166-1 alpha-2 code", g.Country)
	}
	return nil
}

// GeographicallyEligible applies the hard boundary before every matching rung.
// An empty target preserves legacy behaviour until Installation geography is set.
func GeographicallyEligible(c Clip, target Geography) bool {
	target = target.Normalize()
	if target.Country == "" {
		return true
	}
	country := strings.ToUpper(strings.TrimSpace(c.Country))
	switch c.GeographicScope {
	case GeographicNational:
		// A fallback country's national ads air; its local ones are for someone else's town.
		return country == target.Country || target.fallsBackTo(country)
	case GeographicLocal:
		return country == target.Country && target.Market != "" &&
			strings.EqualFold(strings.Join(strings.Fields(c.Market), " "), target.Market)
	default:
		return false
	}
}

// SourceGeographicallyEligible determines whether an acquisition source can contribute to an
// installation. Country-only sources are country-wide; a market makes the source local. Once an
// installation country is configured, an unclassified source is not safe for unattended pulls.
func SourceGeographicallyEligible(source, target Geography) bool {
	target = target.Normalize()
	if target.Country == "" {
		return true
	}
	source = source.Normalize()
	if source.Market == "" && target.fallsBackTo(source.Country) {
		return true
	}
	if source.Country == "" || source.Country != target.Country {
		return false
	}
	if source.Market == "" {
		return true
	}
	return target.Market != "" && strings.EqualFold(source.Market, target.Market)
}

func effectiveGeography(channel, installation Geography) Geography {
	channel, installation = channel.Normalize(), installation.Normalize()
	if installation.Country != "" {
		if channel.Country == "" {
			return installation
		}
		if channel.Country != installation.Country {
			// An impossible target fails every clip closed. A Channel may narrow the
			// installation country to a market, never escape that country boundary.
			return Geography{Country: "--"}
		}
		channel.Fallback = installation.Fallback
		return channel
	}
	if channel.Country != "" {
		return channel
	}
	return installation
}

func filterGeography(clips []Clip, target Geography) []Clip {
	if target.Normalize().Country == "" {
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
