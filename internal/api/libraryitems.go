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
	huma.Register(api, withRole(huma.Operation{
		OperationID: "updateLibraryItemDetails", Method: http.MethodPost, Path: "/v1/library/items/{id}/refresh",
		Summary:     "Update a title's details",
		Description: "Fetches the title's details from TMDB now, and reads what its app and its files say again. 503 when TMDB can't be reached.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.updateLibraryItemDetails)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "checkLibraryItemFiles", Method: http.MethodPost, Path: "/v1/library/items/{id}/check",
		Summary:     "Check a title's files again",
		Description: "Opens the title's files and reads their details again, such as after a file was replaced.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.checkLibraryItemFiles)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "removeLibraryExtraCopy", Method: http.MethodPost, Path: "/v1/library/items/{id}/remove-copy",
		Summary: "Remove a title's extra copy",
		Description: "Moves one of the title's files that never plays into its media folder's trash " +
			"(.loomarr-trash), which empties after 30 days.",
		Tags: []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.removeLibraryExtraCopy)
}

type removeLibraryExtraCopyInput struct {
	ID   string `path:"id" maxLength:"64"`
	Body struct {
		Path string `json:"path" minLength:"1" maxLength:"4096" doc:"The extra copy, as the title's files list it"`
	}
}

func (s *Server) removeLibraryExtraCopy(ctx context.Context, in *removeLibraryExtraCopyInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.RemoveExtraCopy(ctx, in.ID, in.Body.Path)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, apiErr(http.StatusNotFound, "Not one of this title's files", "Check files again, then try again.")
	case errors.Is(err, scan.ErrPlays):
		return nil, apiErr(http.StatusConflict, "That copy is the one that plays", "Only an extra copy can be removed.")
	case errors.Is(err, scan.ErrTrash):
		return nil, apiErrWithCause(http.StatusConflict, "Couldn't move the extra copy",
			"Loomarr may not be allowed to change this folder.", err)
	default:
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't remove the extra copy", "Try again in a moment.", err)
	}
}

func (s *Server) updateLibraryItemDetails(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.UpdateDetails(ctx, in.ID)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, apiErr(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	case errors.Is(err, scan.ErrCatalogUnreachable):
		return nil, apiErr(http.StatusServiceUnavailable, "Couldn't reach TMDB", "Try again in a moment.")
	default:
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't update this title", "Try again in a moment.", err)
	}
}

func (s *Server) checkLibraryItemFiles(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.CheckFiles(ctx, in.ID)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, apiErr(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	default:
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't check this title's files", "Try again in a moment.", err)
	}
}

// libraryMatchDTO says what decides which title an item is, for "Wrong title?" (decision 26).
type libraryMatchDTO struct {
	By   string `json:"by" enum:"app,file,you,none" doc:"What says which title this is: the app that manages it, the file itself (an id in its name, or an NFO beside it), your answer, or nothing yet"`
	App  string `json:"app,omitempty" enum:"sonarr,radarr" doc:"The app, when by is app"`
	Page string `json:"page,omitempty" format:"uri" doc:"The title's page in that app, when by is app and it has one"`
}

func libraryMatchOf(m store.LibraryMatch) libraryMatchDTO {
	by := scan.DecidedBy(m)
	if by == scan.DecidedByApp {
		return libraryMatchDTO{By: string(by), App: m.App, Page: m.Page}
	}
	return libraryMatchDTO{By: string(by)}
}

type libraryItemDTO struct {
	ID    string `json:"id"`
	Kind  string `json:"kind" enum:"movie,series"`
	Name  string `json:"name"`
	Year  int    `json:"year,omitempty" doc:"Absent when unknown"`
	Files int    `json:"files" doc:"Present files; a series counts its episodes' files"`
	// Ready is false while any file's details are unread.
	Ready  bool      `json:"ready" doc:"False while some of its files' details aren't read yet"`
	Poster *ImageDTO `json:"poster,omitempty" doc:"Its poster; absent when it has none, or while one is still being fetched"`
	// A title with either count above zero needs a look (#2151 decision 24).
	Unplayable  int `json:"unplayable" doc:"Files that can't be played: their details couldn't be read"`
	ExtraCopies int `json:"extraCopies" doc:"A movie's files beyond the one that plays"`
}

func libraryItemOf(it store.LibraryItemSummary, posters map[string]*ImageDTO) libraryItemDTO {
	extra := 0
	if it.Kind == "movie" && it.Files > 1 {
		extra = it.Files - 1
	}
	return libraryItemDTO{
		ID: it.ID, Kind: it.Kind, Name: it.Name, Year: it.Year, Files: it.Files, Ready: it.Pending == 0,
		Unplayable: it.Unplayable, ExtraCopies: extra,
		Poster: posters[it.Poster],
	}
}

// libraryPosters renders the items' posters, by hash. An image whose bytes have not arrived is
// left out, so the grid shows the title's name rather than a broken image.
func (s *Server) libraryPosters(ctx context.Context, items []store.LibraryItemSummary) map[string]*ImageDTO {
	out := map[string]*ImageDTO{}
	if s.images == nil {
		return out
	}
	for _, it := range items {
		if it.Poster == "" {
			continue
		}
		if _, seen := out[it.Poster]; seen {
			continue
		}
		out[it.Poster] = nil
		rec, err := s.images.Get(ctx, it.Poster)
		if err != nil || !rec.HasBytes() {
			continue
		}
		dto := s.imageToDTO(ctx, rec)
		out[it.Poster] = &dto
	}
	return out
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
	posters := s.libraryPosters(ctx, page.Items)
	for _, it := range page.Items {
		out.Body.Items = append(out.Body.Items, libraryItemOf(it, posters))
	}
	return out, nil
}

type libraryItemFileDTO struct {
	Path  string `json:"path" doc:"The file on this machine"`
	Size  int64  `json:"size" doc:"Bytes"`
	Ready bool   `json:"ready" doc:"Its details are read"`
	// What its header says; absent until read.
	LengthMillis  int64                `json:"lengthMillis,omitempty" doc:"How long it plays, measured from the file"`
	Picture       *libraryPictureDTO   `json:"picture,omitempty" doc:"Its first video stream"`
	AudioChannels int                  `json:"audioChannels,omitempty" doc:"The channels of the sound that plays: its default audio stream, else its first"`
	Subtitles     []librarySubtitleDTO `json:"subtitles" doc:"Its subtitle streams in order; empty for none or while unread"`
	Extra         bool                 `json:"extra" doc:"A copy that never plays, because another of the title's files does"`
}

type libraryPictureDTO struct {
	Width  int  `json:"width"`
	Height int  `json:"height"`
	HDR    bool `json:"hdr" doc:"High dynamic range"`
}

type librarySubtitleDTO struct {
	Language string `json:"language,omitempty" doc:"ISO 639 code; absent when the file doesn't say"`
}

type libraryDetailsFromDTO struct {
	Description  string `json:"description,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its description came from"`
	Rating       string `json:"rating,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its rating came from"`
	EpisodeNames string `json:"episodeNames,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where most of a series' episode names came from"`
}

// libraryFileOf is f with what its header says.
func libraryFileOf(f scan.ItemFile) libraryItemFileDTO {
	out := libraryItemFileDTO{
		Path: f.Path, Size: f.Size, Ready: f.Header != store.LibraryHeaderPending,
		LengthMillis: f.LengthMillis, AudioChannels: f.AudioChannels, Extra: f.Extra,
		Subtitles: make([]librarySubtitleDTO, 0, len(f.Subtitles)),
	}
	if f.Picture != nil {
		out.Picture = &libraryPictureDTO{Width: f.Picture.Width, Height: f.Picture.Height, HDR: f.Picture.HDR}
	}
	for _, language := range f.Subtitles {
		out.Subtitles = append(out.Subtitles, librarySubtitleDTO{Language: language})
	}
	return out
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
	Item          libraryItemDTO        `json:"item"`
	Overview      string                `json:"overview,omitempty"`
	Genres        []string              `json:"genres"`
	Rating        string                `json:"rating,omitempty" doc:"The certification, such as TV-14"`
	RuntimeMillis int64                 `json:"runtimeMillis,omitempty"`
	Files         []libraryItemFileDTO  `json:"files" doc:"A movie's files; empty for a series"`
	Seasons       []librarySeasonDTO    `json:"seasons" doc:"A series' episodes by season; empty for a movie"`
	Match         libraryMatchDTO       `json:"match"`
	DetailsFrom   libraryDetailsFromDTO `json:"detailsFrom"`
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
	posters := s.libraryPosters(ctx, []store.LibraryItemSummary{d.LibraryItemSummary})
	return &getLibraryItemOutput{Body: libraryItemDetailOf(d, posters)}, nil
}

func libraryItemDetailOf(d scan.ItemDetail, posters map[string]*ImageDTO) libraryItemDetailDTO {
	out := libraryItemDetailDTO{
		Item:     libraryItemOf(d.LibraryItemSummary, posters),
		Overview: d.Facts.Overview, Genres: d.Facts.Genres, Rating: d.Facts.OfficialRating,
		RuntimeMillis: d.Facts.RuntimeMillis,
		Files:         make([]libraryItemFileDTO, 0, len(d.Files)),
		Seasons:       make([]librarySeasonDTO, 0, len(d.Seasons)),
		Match:         libraryMatchOf(d.Match),
		DetailsFrom: libraryDetailsFromDTO{
			Description: string(d.From.Description), Rating: string(d.From.Rating), EpisodeNames: string(d.EpisodeNamesFrom),
		},
	}
	if out.Genres == nil {
		out.Genres = []string{}
	}
	for _, f := range d.Files {
		out.Files = append(out.Files, libraryFileOf(f))
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
