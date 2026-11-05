package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/store"
)

// Library → Items (#2151 step 10, decisions 22 and 25): the movies and series Loomarr found in its
// media folders, and one title's page. Admin-only like the rest of the Library.

// libraryItemsPageMax caps one page of the poster grid.
const libraryItemsPageMax = 200

func (s *Server) registerLibraryItems(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "listLibraryItems", Method: http.MethodGet, Path: "/v1/library/items",
		Summary:     "List the library's titles",
		Description: "Movies and series Loomarr found in its media folders, by name. A title still getting ready has files whose details aren't read yet.",
		Tags:        []string{"library"},
	}, RoleAdmin), s.listLibraryItems)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "getLibraryItem", Method: http.MethodGet, Path: "/v1/library/items/{id}",
		Summary:     "One title",
		Description: "A movie with its files, or a series with its episodes by season. 404 when no file of it is present.",
		Tags:        []string{"library"},
	}, RoleAdmin), s.getLibraryItem)
}

type libraryItemDTO struct {
	ID    string `json:"id"`
	Kind  string `json:"kind" enum:"movie,series"`
	Name  string `json:"name"`
	Year  int    `json:"year,omitempty" doc:"Absent when unknown"`
	Files int    `json:"files" doc:"Present files; a series counts its episodes' files"`
	// Ready is false while any file's details are unread.
	Ready bool `json:"ready" doc:"False while some of its files' details aren't read yet"`
}

func libraryItemOf(it store.LibraryItemSummary) libraryItemDTO {
	return libraryItemDTO{ID: it.ID, Kind: it.Kind, Name: it.Name, Year: it.Year, Files: it.Files, Ready: it.Pending == 0}
}

type listLibraryItemsInput struct {
	Kind   string `query:"kind" enum:"movie,series" doc:"Only movies or only series; both when absent"`
	Search string `query:"search" maxLength:"200" doc:"Part of the name, any case"`
	Limit  int    `query:"limit" minimum:"1" maximum:"200" default:"60"`
	Offset int    `query:"offset" minimum:"0"`
}

type listLibraryItemsOutput struct {
	Body struct {
		Items []libraryItemDTO `json:"items"`
		Total int              `json:"total" doc:"Titles matching, across every page"`
	}
}

func (s *Server) listLibraryItems(ctx context.Context, in *listLibraryItemsInput) (*listLibraryItemsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	page, err := svc.Items(ctx, store.LibraryItemQuery{
		Kind: in.Kind, Search: in.Search, Limit: min(in.Limit, libraryItemsPageMax), Offset: in.Offset,
	})
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't list the library", "Try again in a moment.", err)
	}
	out := &listLibraryItemsOutput{}
	out.Body.Total = page.Total
	out.Body.Items = make([]libraryItemDTO, 0, len(page.Items))
	for _, it := range page.Items {
		out.Body.Items = append(out.Body.Items, libraryItemOf(it))
	}
	return out, nil
}

type libraryItemFileDTO struct {
	Path  string `json:"path" doc:"The file on this machine"`
	Size  int64  `json:"size" doc:"Bytes"`
	Ready bool   `json:"ready" doc:"Its details are read"`
}

type libraryEpisodeDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
	End    int    `json:"end,omitempty" doc:"The last episode a multi-episode file holds"`
	Ready  bool   `json:"ready"`
}

type librarySeasonDTO struct {
	Number   int                 `json:"number" doc:"0 is the specials"`
	Episodes []libraryEpisodeDTO `json:"episodes"`
}

type libraryItemDetailDTO struct {
	// Item is a named field: huma leaves an unexported embedded struct out of the schema.
	Item          libraryItemDTO       `json:"item"`
	Overview      string               `json:"overview,omitempty"`
	Genres        []string             `json:"genres"`
	Rating        string               `json:"rating,omitempty" doc:"The certification, such as TV-14"`
	RuntimeMillis int64                `json:"runtimeMillis,omitempty"`
	Files         []libraryItemFileDTO `json:"files" doc:"A movie's files; empty for a series"`
	Seasons       []librarySeasonDTO   `json:"seasons" doc:"A series' episodes by season; empty for a movie"`
}

type getLibraryItemInput struct {
	ID string `path:"id" maxLength:"64"`
}

type getLibraryItemOutput struct {
	Body libraryItemDetailDTO
}

func (s *Server) getLibraryItem(ctx context.Context, in *getLibraryItemInput) (*getLibraryItemOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	d, err := svc.Item(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, apiErr(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read this title", "Try again in a moment.", err)
	}
	return &getLibraryItemOutput{Body: libraryItemDetailOf(d)}, nil
}

func libraryItemDetailOf(d scan.ItemDetail) libraryItemDetailDTO {
	out := libraryItemDetailDTO{
		Item:     libraryItemOf(d.LibraryItemSummary),
		Overview: d.Facts.Overview, Genres: d.Facts.Genres, Rating: d.Facts.OfficialRating,
		RuntimeMillis: d.Facts.RuntimeMillis,
		Files:         make([]libraryItemFileDTO, 0, len(d.Files)),
		Seasons:       make([]librarySeasonDTO, 0, len(d.Seasons)),
	}
	if out.Genres == nil {
		out.Genres = []string{}
	}
	for _, f := range d.Files {
		out.Files = append(out.Files, libraryItemFileDTO{Path: f.Path, Size: f.Size, Ready: f.Header != store.LibraryHeaderPending})
	}
	for _, season := range d.Seasons {
		dto := librarySeasonDTO{Number: season.Number, Episodes: make([]libraryEpisodeDTO, 0, len(season.Episodes))}
		for _, ep := range season.Episodes {
			dto.Episodes = append(dto.Episodes, libraryEpisodeDTO{ID: ep.ID, Name: ep.Name, Number: ep.Number, End: ep.End, Ready: ep.Ready})
		}
		out.Seasons = append(out.Seasons, dto)
	}
	return out
}
