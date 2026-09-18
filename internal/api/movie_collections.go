package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/provision"
)

const maxMovieCollectionKeys = 24

func (s *Server) registerMovieCollections(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "resolveMovieCollections", Method: http.MethodGet, Path: "/v1/movie-collections",
		Summary: "Resolve grounded movie collections", Tags: []string{"search"},
	}, RoleMember), s.doResolveMovieCollections)
}

type movieCollectionsInput struct {
	Keys []string `query:"key,explode" maxItems:"24" doc:"Visible movie provisioning Keys; repeat for multiple titles"`
}

type movieCollectionsOutput struct {
	Body MovieCollectionResolution
}

func (s *Server) doResolveMovieCollections(ctx context.Context, in *movieCollectionsInput) (*movieCollectionsOutput, error) {
	if s.movieCollections == nil || (s.liveConfig != nil && strings.TrimSpace(s.liveConfig("tmdb.api_key")) == "") {
		return nil, errNotImplemented("Movie collections aren't ready",
			"Connect TMDB in Settings to see the other films in a collection.")
	}
	if len(in.Keys) == 0 {
		return nil, errBadRequest("No movies provided", "Provide at least one movie Key.")
	}
	for _, key := range in.Keys {
		mediaType, provider, id, ok := provision.ParseKey(provision.Key(key))
		if !ok || mediaType != provision.Movie || provider != "tmdb" || id <= 0 {
			return nil, errBadRequest("That movie reference isn't valid",
				"Movie collections require a TMDB movie Key.")
		}
	}
	request := MovieCollectionRequest{Keys: append([]string(nil), in.Keys...)}
	resolution, err := s.movieCollections.ResolveMovieCollections(ctx, request)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Collection details didn't load",
			"Your current selections are unchanged. Try again in a moment.", err)
	}
	if resolution.Collections == nil {
		resolution.Collections = []MovieCollection{}
	}
	for i := range resolution.Collections {
		if resolution.Collections[i].Members == nil {
			resolution.Collections[i].Members = []SearchCandidate{}
		}
	}
	return &movieCollectionsOutput{Body: resolution}, nil
}
