package api

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/installationlocation"
)

const dbIPAttribution = "https://db-ip.com"

type locationDTO struct {
	ID          string `json:"id" doc:"Stable place identifier"`
	Label       string `json:"label" example:"New York City, United States" doc:"Human place label"`
	Country     string `json:"country" example:"US" minLength:"2" maxLength:"2" doc:"ISO 3166-1 alpha-2 setting value"`
	Market      string `json:"market,omitempty" example:"New York City" doc:"Local market setting value; empty for a country-only choice"`
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

func (s *Server) registerLocations(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "locations-search", Method: http.MethodGet, Path: "/v1/locations",
		Summary: "Search installation locations", Description: "Admin only. Searches Loomarr's embedded city, populated-municipality, and country index; no runtime geocoder is called.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.locationsSearch)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "locations-resolve", Method: http.MethodPost, Path: "/v1/locations/resolve",
		Summary: "Resolve device coordinates", Description: "Admin only. Resolves transient browser coordinates against Loomarr's embedded place index. Coordinates are not stored.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.locationResolve)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "locations-suggestion", Method: http.MethodGet, Path: "/v1/locations/suggestion",
		Summary: "Suggest an approximate installation location", Description: "Admin only. Uses trusted CDN geography or the effective public client IP against Loomarr's embedded index. Never saves the suggestion.",
		Tags: []string{"settings"},
	}, RoleAdmin), s.locationSuggestion)
}

func (s *Server) locationsSearch(_ context.Context, input *locationsSearchInput) (*locationsSearchOutput, error) {
	index, err := installationlocation.Default()
	if err != nil {
		return nil, errServiceUnavailable("Location search unavailable", "The built-in place index couldn't be opened. Try again after restarting Loomarr.")
	}
	out := &locationsSearchOutput{}
	out.Body.Locations = make([]locationDTO, 0)
	for _, place := range index.Search(input.Q, input.Limit) {
		out.Body.Locations = append(out.Body.Locations, locationDTOFrom(place))
	}
	return out, nil
}

func (s *Server) locationResolve(_ context.Context, input *locationResolveInput) (*locationOutput, error) {
	index, err := installationlocation.Default()
	if err != nil {
		return nil, errServiceUnavailable("Location lookup unavailable", "The built-in place index couldn't be opened. Try again after restarting Loomarr.")
	}
	place, ok := index.Resolve(input.Body.Latitude, input.Body.Longitude)
	if !ok {
		return nil, errUnprocessable("Location not found", "Loomarr couldn't match those coordinates to a nearby place. Search for your location instead.")
	}
	out := &locationOutput{}
	out.Body.Location = locationDTOFrom(place)
	out.Body.Location.Source = "device"
	return out, nil
}

func (s *Server) locationSuggestion(ctx context.Context, _ *struct{}) (*locationSuggestionOutput, error) {
	index, err := installationlocation.Default()
	if err != nil {
		return nil, errServiceUnavailable("Location suggestion unavailable", "The built-in location index couldn't be opened. Search for your location instead.")
	}
	out := &locationSuggestionOutput{}
	request := requestFrom(ctx)
	if request == nil {
		return out, nil
	}
	if s.trustProxy {
		if place, source, ok := trustedLocationHeader(index, request); ok {
			dto := locationDTOFrom(place)
			dto.Source, dto.Approximate = source, true
			out.Body.Suggestion = &dto
			return out, nil
		}
	}
	address, ok := effectiveLocationClientAddress(request, s.trustProxy)
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

// effectiveLocationClientAddress returns the public-address candidate used only for
// the approximate location suggestion. Keep it separate from clientIP: the auth rate
// limiter has a deliberately narrow, established string contract, while location
// lookup must correctly accept both IPv4 and IPv6 socket forms.
func effectiveLocationClientAddress(request *http.Request, trustProxy bool) (netip.Addr, bool) {
	raw := strings.TrimSpace(request.RemoteAddr)
	if trustProxy {
		if forwarded := request.Header.Get("X-Forwarded-For"); forwarded != "" {
			raw, _, _ = strings.Cut(forwarded, ",")
			raw = strings.TrimSpace(raw)
		}
	}
	if address, err := netip.ParseAddr(raw); err == nil {
		return address.WithZone("").Unmap(), true
	}
	if addressPort, err := netip.ParseAddrPort(raw); err == nil {
		return addressPort.Addr().WithZone("").Unmap(), true
	}
	return netip.Addr{}, false
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

func trustedLocationHeader(index *installationlocation.Index, request *http.Request) (installationlocation.Place, string, bool) {
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
	return installationlocation.Place{}, "", false
}

func locationDTOFrom(place installationlocation.Place) locationDTO {
	return locationDTO{ID: place.ID, Label: place.Label, Country: place.Country, Market: place.Market, Region: place.Region}
}
