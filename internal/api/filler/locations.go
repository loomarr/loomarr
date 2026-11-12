package filler

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/location"
)

const dbIPAttribution = "https://db-ip.com"

type locationDTO struct {
	ID          string `json:"id" doc:"Stable place identifier"`
	Label       string `json:"label" example:"New York City, United States" doc:"Human place label"`
	Country     string `json:"country" example:"US" minLength:"2" maxLength:"2" doc:"ISO 3166-1 alpha-2 setting value"`
	Place       string `json:"place,omitempty" example:"5128581" doc:"filler.home_place (or a Channel's place) setting value; empty for a country-only choice"`
	City        string `json:"city,omitempty" example:"New York City" doc:"Display-only city name; empty for a country-only choice"`
	Region      string `json:"region,omitempty" example:"New York" doc:"Display-only first-level region used to distinguish search results"`
	Source      string `json:"source,omitempty" enum:"device,cloudflare,cloudfront,vercel,proxy,ip" doc:"Evidence used for an automatic suggestion"`
	Approximate bool   `json:"approximate,omitempty" doc:"True when the result came from IP/CDN evidence rather than selected device coordinates"`
	Attribution string `json:"attribution,omitempty" doc:"Required data-provider attribution URL"`
}

type locationsSearchInput struct {
	Q     string `query:"q" minLength:"2" maxLength:"120" doc:"City, populated municipality, region, or country name"`
	Limit int    `query:"limit" minimum:"1" maximum:"20" default:"8" doc:"Maximum suggestions"`
}

type locationsSearchOutput struct {
	Body struct {
		Locations []locationDTO `json:"locations"`
	}
}

type locationResolveInput struct {
	Body struct {
		Latitude  float64 `json:"latitude" minimum:"-90" maximum:"90"`
		Longitude float64 `json:"longitude" minimum:"-180" maximum:"180"`
	}
}

type locationOutput struct {
	Body struct {
		Location locationDTO `json:"location"`
	}
}

type locationSuggestionOutput struct {
	Body struct {
		Suggestion *locationDTO `json:"suggestion,omitempty"`
	}
}

func (s *Handlers) registerLocations(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "locations-search", Method: http.MethodGet, Path: "/v1/locations",
		Summary: "Search installation locations", Description: "Admin only. Searches Loomarr's embedded city, populated-municipality, and country index; no runtime geocoder is called.",
		Tags: []string{"settings"},
	}, httpcore.RoleAdmin), s.locationsSearch)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "locations-resolve", Method: http.MethodPost, Path: "/v1/locations/resolve",
		Summary: "Resolve device coordinates", Description: "Admin only. Resolves transient browser coordinates against Loomarr's embedded place index. Coordinates are not stored.",
		Tags: []string{"settings"},
	}, httpcore.RoleAdmin), s.locationResolve)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "locations-get", Method: http.MethodGet, Path: "/v1/locations/{id}",
		Summary: "Get a saved place", Description: "Admin only. Returns the embedded index entry for a saved city place id, so a form can show the place it holds.",
		Tags: []string{"settings"},
	}, httpcore.RoleAdmin), s.locationGet)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "locations-suggestion", Method: http.MethodGet, Path: "/v1/locations/suggestion",
		Summary: "Suggest an approximate installation location", Description: "Admin only. Uses trusted CDN geography or the effective public client IP against Loomarr's embedded index. Never saves the suggestion.",
		Tags: []string{"settings"},
	}, httpcore.RoleAdmin), s.locationSuggestion)
}

func (s *Handlers) locationsSearch(_ context.Context, input *locationsSearchInput) (*locationsSearchOutput, error) {
	index, err := location.Default()
	if err != nil {
		return nil, httpcore.ServiceUnavailable("Location search unavailable", "The built-in place index couldn't be opened. Try again after restarting Loomarr.")
	}
	out := &locationsSearchOutput{}
	out.Body.Locations = make([]locationDTO, 0)
	for _, place := range index.Search(input.Q, input.Limit) {
		out.Body.Locations = append(out.Body.Locations, locationDTOFrom(place))
	}
	return out, nil
}

func (s *Handlers) locationResolve(_ context.Context, input *locationResolveInput) (*locationOutput, error) {
	index, err := location.Default()
	if err != nil {
		return nil, httpcore.ServiceUnavailable("Location lookup unavailable", "The built-in place index couldn't be opened. Try again after restarting Loomarr.")
	}
	place, ok := index.Resolve(input.Body.Latitude, input.Body.Longitude)
	if !ok {
		return nil, httpcore.Unprocessable("Location not found", "Loomarr couldn't match those coordinates to a nearby place. Search for your location instead.")
	}
	out := &locationOutput{}
	out.Body.Location = locationDTOFrom(place)
	out.Body.Location.Source = "device"
	return out, nil
}

func (s *Handlers) locationSuggestion(ctx context.Context, _ *struct{}) (*locationSuggestionOutput, error) {
	index, err := location.Default()
	if err != nil {
		return nil, httpcore.ServiceUnavailable("Location suggestion unavailable", "The built-in location index couldn't be opened. Search for your location instead.")
	}
	out := &locationSuggestionOutput{}
	request := httpcore.RequestFrom(ctx)
	if request == nil {
		return out, nil
	}
	if s.origin.TrustProxy {
		if place, source, ok := trustedLocationHeader(index, request); ok {
			dto := locationDTOFrom(place)
			dto.Source, dto.Approximate = source, true
			out.Body.Suggestion = &dto
			return out, nil
		}
	}
	address, ok := effectiveLocationClientAddress(request, s.origin)
	if !ok {
		return out, nil
	}
	if place, ok := index.LookupCountry(address); ok {
		dto := locationDTOFrom(place)
		dto.Source, dto.Approximate, dto.Attribution = "ip", true, dbIPAttribution
		out.Body.Suggestion = &dto
	}
	return out, nil
}

// effectiveLocationClientAddress returns the address the approximate location suggestion looks up:
// the client as httpcore.Origin.ClientIP finds it, so X-Forwarded-For counts only from a proxy and
// only its right-most hop that is not a proxy, never the left-most one a client wrote itself.
func effectiveLocationClientAddress(request *http.Request, origin httpcore.Origin) (netip.Addr, bool) {
	address, err := netip.ParseAddr(origin.ClientIP(request))
	if err != nil {
		return netip.Addr{}, false
	}
	return address, true
}

type locationHeaders struct {
	source, country, city string
}

var supportedLocationHeaders = []locationHeaders{
	{source: "proxy", country: "X-Loomarr-Geo-Country", city: "X-Loomarr-Geo-City"},
	{source: "cloudflare", country: "CF-IPCountry", city: "CF-IPCity"},
	{source: "cloudfront", country: "CloudFront-Viewer-Country", city: "CloudFront-Viewer-City"},
	{source: "vercel", country: "X-Vercel-IP-Country", city: "X-Vercel-IP-City"},
}

func trustedLocationHeader(index *location.Index, request *http.Request) (location.Place, string, bool) {
	for _, candidate := range supportedLocationHeaders {
		country := strings.ToUpper(strings.TrimSpace(request.Header.Get(candidate.country)))
		countryPlace, validCountry := index.Country(country)
		if !validCountry {
			continue
		}
		city, unescapeErr := url.PathUnescape(strings.TrimSpace(request.Header.Get(candidate.city)))
		if unescapeErr == nil && city != "" {
			for _, place := range index.Search(city, 20) {
				if place.Country == country && strings.EqualFold(place.Name, city) {
					return place, candidate.source, true
				}
			}
		}
		return countryPlace, candidate.source, true
	}
	return location.Place{}, "", false
}

func locationDTOFrom(place location.Place) locationDTO {
	dto := locationDTO{ID: place.ID, Label: place.Label, Country: place.Country, Region: place.Region}
	if place.City != "" {
		dto.Place, dto.City = place.ID, place.City
	}
	return dto
}

type locationGetInput struct {
	ID string `path:"id" minLength:"1" maxLength:"40" doc:"A saved place id (filler.home_place or a Channel's place)"`
}

func (s *Handlers) locationGet(_ context.Context, input *locationGetInput) (*locationOutput, error) {
	index, err := location.Default()
	if err != nil {
		return nil, httpcore.ServiceUnavailable("Location lookup unavailable", "The built-in place index couldn't be opened. Try again after restarting Loomarr.")
	}
	place, ok := index.Lookup(input.ID)
	if !ok {
		return nil, httpcore.NotFound("Place not found", "That saved place isn't in Loomarr's location index. Search for your city again.")
	}
	out := &locationOutput{}
	out.Body.Location = locationDTOFrom(place)
	return out, nil
}
