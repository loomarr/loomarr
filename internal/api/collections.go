package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerCollections mounts GET /v1/library/collections — the media server's collections
// (BoxSets) that back `scope.collections` (programming-design §2.2).
//
// Member-readable, like /v1/search: it is read-only, exposes no more than the media server
// already shows the same user, and picking a collection still routes a channel through
// submit→approve. So it adds no privilege surface (§7.2).
func (s *Server) registerCollections(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "listLibraryCollections", Method: http.MethodGet, Path: "/v1/library/collections",
		Summary: "List media-server collections", Tags: []string{"library"},
	}, RoleMember), s.doListCollections)
}

type collectionsOutput struct {
	Body struct {
		Collections []LibraryCollection `json:"collections"`
	}
}

func (s *Server) doListCollections(ctx context.Context, _ *struct{}) (*collectionsOutput, error) {
	if s.collections == nil || s.libraryUnconfigured() {
		return nil, errNotImplemented("Collections aren't available",
			"Connect your media library in Settings to use collections.")
	}
	colls, err := s.collections.Collections(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't load collections",
			"The media library didn't answer. Check the connection in Settings and try again.", err)
	}
	out := &collectionsOutput{}
	// Always a slice, never nil: an operator with no collections must serialize as `[]` so the
	// picker renders its empty state rather than the client having to treat null as empty.
	out.Body.Collections = colls
	if out.Body.Collections == nil {
		out.Body.Collections = []LibraryCollection{}
	}
	return out, nil
}
