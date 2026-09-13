// Package installationlocation owns Loomarr's offline place search and location resolution.
package installationlocation

import (
	"encoding/binary"
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strings"
)

const maxResolveDistanceKM = 250

// Place is a user-selectable Installation geography. Latitude and longitude are index-only
// evidence; API projections deliberately expose only the identity and setting values.
type Place struct {
	ID          string
	Name        string
	SearchText  string
	Label       string
	Country     string
	CountryName string
	Region      string
	Market      string
	Latitude    float64
	Longitude   float64
	Population  int64
}

// IPRange is the generator/test input shape. Index compacts it before serving lookups.
type IPRange struct {
	From    string
	To      string
	Country string
}

type ipv4Range struct {
	from, to uint32
	country  [2]byte
}

type ipv6Range struct {
	from, to [16]byte
	country  [2]byte
}

// Index is immutable after construction and safe for concurrent requests.
type Index struct {
	places    []Place
	countries map[string]Place
	v4        []ipv4Range
	v6        []ipv6Range
}

// NewIndex validates and compacts generated/test data.
func NewIndex(places []Place, ranges []IPRange) (*Index, error) {
	idx := &Index{places: make([]Place, 0, len(places)), countries: make(map[string]Place)}
	for _, place := range places {
		place.Country = strings.ToUpper(strings.TrimSpace(place.Country))
		if len(place.Country) != 2 || strings.TrimSpace(place.Name) == "" {
			return nil, fmt.Errorf("installationlocation: invalid place %q/%q", place.Name, place.Country)
		}
		if place.CountryName == "" {
			place.CountryName = place.Name
		}
		if place.Label == "" {
			if place.Market == "" {
				place.Label = place.CountryName
			} else {
				place.Label = place.Market + ", " + place.CountryName
			}
		}
		idx.places = append(idx.places, place)
		if place.Market == "" {
			idx.countries[place.Country] = place
		}
	}

	for _, raw := range ranges {
		if err := idx.addRange(raw); err != nil {
			return nil, err
		}
	}
	idx.sortRanges()
	return idx, nil
}

func (i *Index) addRange(raw IPRange) error {
	from, err := netip.ParseAddr(raw.From)
	if err != nil {
		return fmt.Errorf("installationlocation: parse range start %q: %w", raw.From, err)
	}
	to, err := netip.ParseAddr(raw.To)
	if err != nil {
		return fmt.Errorf("installationlocation: parse range end %q: %w", raw.To, err)
	}
	country := strings.ToUpper(strings.TrimSpace(raw.Country))
	if len(country) != 2 || from.Is4() != to.Is4() || from.Compare(to) > 0 {
		return fmt.Errorf("installationlocation: invalid range %q-%q/%q", raw.From, raw.To, raw.Country)
	}
	code := [2]byte{country[0], country[1]}
	if from.Is4() {
		i.v4 = append(i.v4, ipv4Range{from: ipv4Number(from), to: ipv4Number(to), country: code})
	} else {
		i.v6 = append(i.v6, ipv6Range{from: from.As16(), to: to.As16(), country: code})
	}
	return nil
}

func (i *Index) sortRanges() {
	sort.Slice(i.v4, func(a, b int) bool { return i.v4[a].from < i.v4[b].from })
	sort.Slice(i.v6, func(a, b int) bool { return compare16(i.v6[a].from, i.v6[b].from) < 0 })
}

// Search returns a small ranked set for the UI combobox.
func (i *Index) Search(query string, limit int) []Place {
	query = normalize(query)
	if query == "" {
		return nil
	}
	if limit < 1 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	type match struct {
		place Place
		rank  int
	}
	matches := make([]match, 0, limit)
	for _, place := range i.places {
		name := normalize(place.Name)
		searchText := normalize(place.SearchText)
		label := normalize(place.Label)
		country := normalize(place.CountryName)
		var rank int
		switch {
		case query == name || query == label:
			rank = 0
		case query == country && place.Market == "":
			rank = 0
		case strings.HasPrefix(name, query) || strings.HasPrefix(label, query):
			rank = 1
		case strings.Contains(name, query) || strings.Contains(label, query) || strings.Contains(searchText, query):
			rank = 2
		case strings.Contains(country, query):
			rank = 3
		default:
			continue
		}
		matches = append(matches, match{place: place, rank: rank})
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].rank != matches[b].rank {
			return matches[a].rank < matches[b].rank
		}
		if matches[a].place.Population != matches[b].place.Population {
			return matches[a].place.Population > matches[b].place.Population
		}
		return matches[a].place.Label < matches[b].place.Label
	})
	out := make([]Place, 0, min(limit, len(matches)))
	seen := make(map[string]struct{}, len(matches))
	for _, candidate := range matches {
		place := candidate.place
		// City and administrative datasets can describe the same choice with different
		// GeoNames ids. The persisted identity is country + market; region keeps distinct
		// same-named places visible while collapsing a city/municipality pair.
		key := place.Country + "\x00" + normalize(place.Region) + "\x00" + normalize(place.Market)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, place)
		if len(out) == limit {
			break
		}
	}
	return out
}

// Resolve returns the nearest indexed city within a deliberately bounded distance.
func (i *Index) Resolve(latitude, longitude float64) (Place, bool) {
	if math.IsNaN(latitude) || math.IsNaN(longitude) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return Place{}, false
	}
	bestDistance := math.MaxFloat64
	var best Place
	for _, place := range i.places {
		if place.Market == "" {
			continue
		}
		distance := haversineKM(latitude, longitude, place.Latitude, place.Longitude)
		if distance < bestDistance {
			bestDistance, best = distance, place
		}
	}
	return best, bestDistance <= maxResolveDistanceKM
}

// Country returns the canonical country-only selection for an ISO alpha-2 code.
func (i *Index) Country(code string) (Place, bool) {
	place, ok := i.countries[strings.ToUpper(strings.TrimSpace(code))]
	return place, ok
}

// LookupCountry resolves only public addresses. Private and link-local addresses are never
// useful Installation evidence on a homelab and must not accidentally match a data range.
func (i *Index) LookupCountry(addr netip.Addr) (Place, bool) {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return Place{}, false
	}
	var code string
	if addr.Is4() {
		value := ipv4Number(addr)
		at := sort.Search(len(i.v4), func(n int) bool { return i.v4[n].from > value }) - 1
		if at < 0 || value > i.v4[at].to {
			return Place{}, false
		}
		code = string(i.v4[at].country[:])
	} else {
		value := addr.As16()
		at := sort.Search(len(i.v6), func(n int) bool { return compare16(i.v6[n].from, value) > 0 }) - 1
		if at < 0 || compare16(value, i.v6[at].to) > 0 {
			return Place{}, false
		}
		code = string(i.v6[at].country[:])
	}
	return i.Country(code)
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func ipv4Number(addr netip.Addr) uint32 {
	bytes := addr.As4()
	return binary.BigEndian.Uint32(bytes[:])
}

func compare16(a, b [16]byte) int {
	for n := range a {
		if a[n] < b[n] {
			return -1
		}
		if a[n] > b[n] {
			return 1
		}
	}
	return 0
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371
	toRadians := math.Pi / 180
	dLat := (lat2 - lat1) * toRadians
	dLon := (lon2 - lon1) * toRadians
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*toRadians)*math.Cos(lat2*toRadians)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKM * math.Asin(math.Sqrt(a))
}
