package library

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// Library → Collections (#2151 step 12, decision 28): the household's own collections, each an
// ordered list of titles. A collection a channel uses can't be deleted; the API says which
// channels use it, and the UI words it.
//
// `/v1/library/collections` lists the same collections, names and sizes only, for the channel
// scope picker.

func (s *Handlers) registerLibraryCollections(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listCollections", Method: http.MethodGet, Path: "/v1/collections",
		Summary:     "Your collections",
		Description: "The household's collections, by name, each with its titles in order.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.listCollections)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "getCollection", Method: http.MethodGet, Path: "/v1/collections/{id}",
		Summary: "One collection",
		Tags:    []string{"library"},
	}, httpcore.RoleAdmin), s.getCollection)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "createCollection", Method: http.MethodPost, Path: "/v1/collections",
		Summary:     "Make a collection",
		Description: "Titles are library title ids, in the order the collection lists them.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusCreated,
	}, httpcore.RoleAdmin), s.createCollection)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "updateCollection", Method: http.MethodPut, Path: "/v1/collections/{id}",
		Summary:     "Change a collection",
		Description: "Replaces its name, description and titles.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.updateCollection)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "deleteCollection", Method: http.MethodDelete, Path: "/v1/collections/{id}",
		Summary:     "Delete a collection",
		Description: "409 while a channel uses it.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.deleteCollection)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "copySet", Method: http.MethodPost, Path: "/v1/collections/sets/{id}/copy",
		Summary:     "Make my own copy of a ready-made set",
		Description: "Makes a collection of the household's own with the set's name and the library's movies of it, in its order.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusCreated,
	}, httpcore.RoleAdmin), s.copySet)
}

type collectionDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	ImportedFrom string `json:"importedFrom,omitempty" enum:"emby,jellyfin" doc:"The media server it was brought over from; absent when made in Loomarr"`
	// Titles leaves out a title that has left the library; it comes back in its place.
	Titles []libraryItemDTO    `json:"titles" doc:"In the collection's order"`
	UsedBy []libraryChannelDTO `json:"usedBy" doc:"The channels that use it, by number; it can't be deleted while any do"`
}

type collectionBody struct {
	Name        string   `json:"name" minLength:"1" maxLength:"200"`
	Description string   `json:"description,omitempty" maxLength:"2000"`
	Titles      []string `json:"titles" maxItems:"5000" doc:"Library title ids, in order"`
}

type collectionIDInput struct {
	ID string `path:"id"`
}

type collectionOutput struct {
	Body collectionDTO
}

type listCollectionsOutput struct {
	Body struct {
		Collections []collectionDTO `json:"collections"`
		Sets        []setDTO        `json:"sets" doc:"Ready-made sets (TMDB collections) the library has a movie of, by name"`
	}
}

// setDTO is a ready-made set: it can't be changed, only copied into the household's collections.
type setDTO struct {
	ID     string           `json:"id" doc:"The TMDB collection id"`
	Name   string           `json:"name"`
	Total  int              `json:"total" doc:"Every movie in the set, in the library or not"`
	Titles []libraryItemDTO `json:"titles" doc:"The library's movies of the set, in its order"`
}

func (s *Handlers) listCollections(ctx context.Context, _ *struct{}) (*listCollectionsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	list, err := svc.Collections(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read your collections", "Try again in a moment.", err)
	}
	sets, err := svc.Sets(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read your collections", "Try again in a moment.", err)
	}
	uses, err := s.collectionUses(ctx)
	if err != nil {
		return nil, err
	}
	var titles []store.LibraryItemSummary
	for _, c := range list {
		titles = append(titles, c.Titles...)
	}
	for _, set := range sets {
		titles = append(titles, set.Titles...)
	}
	posters := s.libraryPosters(ctx, titles)
	out := &listCollectionsOutput{}
	out.Body.Collections = make([]collectionDTO, 0, len(list))
	for _, c := range list {
		out.Body.Collections = append(out.Body.Collections, collectionOf(c, posters, uses))
	}
	out.Body.Sets = make([]setDTO, 0, len(sets))
	for _, set := range sets {
		dto := setDTO{ID: set.ID, Name: set.Name, Total: set.Total, Titles: make([]libraryItemDTO, 0, len(set.Titles))}
		for _, it := range set.Titles {
			dto.Titles = append(dto.Titles, libraryItemOf(it, posters))
		}
		out.Body.Sets = append(out.Body.Sets, dto)
	}
	return out, nil
}

func (s *Handlers) copySet(ctx context.Context, in *collectionIDInput) (*collectionOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	id, err := svc.CopySet(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.Problem(http.StatusNotFound, "No such set in your library", "Your library has none of its movies.")
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't copy the set", "Try again in a moment.", err)
	}
	return s.collectionResponse(ctx, id)
}

func (s *Handlers) getCollection(ctx context.Context, in *collectionIDInput) (*collectionOutput, error) {
	return s.collectionResponse(ctx, in.ID)
}

func (s *Handlers) createCollection(ctx context.Context, in *struct{ Body collectionBody }) (*collectionOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Body.Name)
	if name == "" {
		return nil, httpcore.Problem(http.StatusUnprocessableEntity, "A collection needs a name", "Give it a name, then save.")
	}
	id, err := svc.CreateCollection(ctx, name, strings.TrimSpace(in.Body.Description), in.Body.Titles)
	if errors.Is(err, store.ErrUnknownLibraryItem) {
		return nil, unknownCollectionTitle()
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't save the collection", "Try again in a moment.", err)
	}
	return s.collectionResponse(ctx, id)
}

func (s *Handlers) updateCollection(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body collectionBody
}) (*collectionOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Body.Name)
	if name == "" {
		return nil, httpcore.Problem(http.StatusUnprocessableEntity, "A collection needs a name", "Give it a name, then save.")
	}
	err = svc.UpdateCollection(ctx, in.ID, name, strings.TrimSpace(in.Body.Description), in.Body.Titles)
	if errors.Is(err, store.ErrNotFound) {
		return nil, collectionNotFound()
	}
	if errors.Is(err, store.ErrUnknownLibraryItem) {
		return nil, unknownCollectionTitle()
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't save the collection", "Try again in a moment.", err)
	}
	return s.collectionResponse(ctx, in.ID)
}

// unknownCollectionTitle refuses a collection naming a title the library has no record of.
func unknownCollectionTitle() error {
	return httpcore.Problem(http.StatusUnprocessableEntity, "A title isn't in your library",
		"One of the titles is no longer in your library. Reload the page, then save again.")
}

func (s *Handlers) deleteCollection(ctx context.Context, in *collectionIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	uses, err := s.collectionUses(ctx)
	if err != nil {
		return nil, err
	}
	if len(uses[in.ID]) > 0 {
		return nil, httpcore.Problem(http.StatusConflict, "A channel uses this collection", "Take it off the channel first.")
	}
	err = svc.DeleteCollection(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, collectionNotFound()
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't delete the collection", "Try again in a moment.", err)
	}
	return nil, nil
}

func (s *Handlers) collectionResponse(ctx context.Context, id string) (*collectionOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	c, err := svc.Collection(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, collectionNotFound()
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the collection", "Try again in a moment.", err)
	}
	uses, err := s.collectionUses(ctx)
	if err != nil {
		return nil, err
	}
	return &collectionOutput{Body: collectionOf(c, s.libraryPosters(ctx, c.Titles), uses)}, nil
}

func collectionNotFound() error {
	return httpcore.Problem(http.StatusNotFound, "No such collection", "It may have been deleted.")
}

func collectionOf(c scan.Collection, posters map[string]*httpcore.ImageDTO, uses map[string][]libraryChannelDTO) collectionDTO {
	out := collectionDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, ImportedFrom: c.ImportedFrom,
		Titles: make([]libraryItemDTO, 0, len(c.Titles)),
		UsedBy: uses[c.ID],
	}
	if out.UsedBy == nil {
		out.UsedBy = []libraryChannelDTO{}
	}
	for _, it := range c.Titles {
		out.Titles = append(out.Titles, libraryItemOf(it, posters))
	}
	return out
}

// collectionUses maps each collection id a channel scope names to those channels, by number: the
// channel's own scope and each rule's.
func (s *Handlers) collectionUses(ctx context.Context) (map[string][]libraryChannelDTO, error) {
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the channels", "Try again in a moment.", err)
	}
	slices.SortFunc(channels, func(a, b channelstore.Channel) int { return a.Number - b.Number })
	out := map[string][]libraryChannelDTO{}
	for _, ch := range channels {
		named := map[string]bool{}
		scopes := []*schedule.ScopePolicy{&ch.Policy.Scope}
		for _, r := range ch.Policy.Rules {
			scopes = append(scopes, r.What)
		}
		for _, scope := range scopes {
			if scope == nil {
				continue
			}
			for _, id := range scope.Collections {
				if named[id] {
					continue
				}
				named[id] = true
				out[id] = append(out[id], libraryChannelDTO{ID: ch.ID, Name: ch.Name, Number: ch.Number})
			}
		}
	}
	return out, nil
}
