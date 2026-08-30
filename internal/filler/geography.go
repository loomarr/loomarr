package filler

import (
	"fmt"
	"strings"
)

type GeographicScope string

const (
	GeographicUnknown  GeographicScope = "unknown"
	GeographicNational GeographicScope = "national"
	GeographicLocal    GeographicScope = "local"
)

// Geography identifies the audience location a Channel serves.
type Geography struct {
	Country string `json:"country,omitempty"`
	Market  string `json:"market,omitempty"`
}

func (g Geography) Normalize() Geography {
	g.Country = strings.ToUpper(strings.TrimSpace(g.Country))
	g.Market = strings.Join(strings.Fields(strings.TrimSpace(g.Market)), " ")
	return g
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
		return country == target.Country
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
