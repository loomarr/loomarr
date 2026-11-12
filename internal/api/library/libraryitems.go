package library

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Library → Items (#2151 step 10, decisions 22 and 25): the movies and series Loomarr found in its
// media folders, and one title's page. Admin-only like the rest of the Library.

// libraryItemsPageMax caps one page of the poster grid.
const libraryItemsPageMax = 200

func (s *Handlers) registerLibraryItems(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listLibraryItems", Method: http.MethodGet, Path: "/v1/library/items",
		Summary:     "List the library's titles",
		Description: "Movies and series Loomarr found in its media folders, by name. A title still getting ready has files whose details aren't read yet.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.listLibraryItems)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "getLibraryItem", Method: http.MethodGet, Path: "/v1/library/items/{id}",
		Summary:     "One title",
		Description: "A movie with its files, or a series with its episodes by season. 404 when no file of it is present.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.getLibraryItem)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "updateLibraryItemDetails", Method: http.MethodPost, Path: "/v1/library/items/{id}/refresh",
		Summary:     "Update a title's details",
		Description: "Fetches the title's details from TMDB now, and reads what its app and its files say again. 503 when TMDB can't be reached.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.updateLibraryItemDetails)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "checkLibraryItemFiles", Method: http.MethodPost, Path: "/v1/library/items/{id}/check",
		Summary:     "Check a title's files again",
		Description: "Opens the title's files and reads their details again, such as after a file was replaced.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.checkLibraryItemFiles)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "removeLibraryExtraCopy", Method: http.MethodPost, Path: "/v1/library/items/{id}/remove-copy",
		Summary: "Remove a title's extra copy",
		Description: "Moves one of the title's files that never plays into its media folder's trash " +
			"(.loomarr-trash), which empties after 30 days.",
		Tags: []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.removeLibraryExtraCopy)
}

type removeLibraryExtraCopyInput struct {
	ID   string `path:"id" maxLength:"64"`
	Body struct {
		Path string `json:"path" minLength:"1" maxLength:"4096" doc:"The extra copy, as the title's files list it"`
	}
}

func (s *Handlers) removeLibraryExtraCopy(ctx context.Context, in *removeLibraryExtraCopyInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.RemoveExtraCopy(ctx, in.ID, in.Body.Path)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, httpcore.Problem(http.StatusNotFound, "Not one of this title's files", "Check files again, then try again.")
	case errors.Is(err, scan.ErrPlays):
		return nil, httpcore.Problem(http.StatusConflict, "That copy is the one that plays", "Only an extra copy can be removed.")
	case errors.Is(err, scan.ErrTrash):
		return nil, httpcore.ProblemWithCause(http.StatusConflict, "Couldn't move the extra copy",
			"Loomarr may not be allowed to change this folder.", err)
	default:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't remove the extra copy", "Try again in a moment.", err)
	}
}

func (s *Handlers) updateLibraryItemDetails(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.UpdateDetails(ctx, in.ID)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, httpcore.Problem(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	case errors.Is(err, scan.ErrCatalogUnreachable):
		return nil, httpcore.Problem(http.StatusServiceUnavailable, "Couldn't reach TMDB", "Try again in a moment.")
	default:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't update this title", "Try again in a moment.", err)
	}
}

func (s *Handlers) checkLibraryItemFiles(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	err = svc.CheckFiles(ctx, in.ID)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, store.ErrNotFound):
		return nil, httpcore.Problem(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	default:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't check this title's files", "Try again in a moment.", err)
	}
}

// libraryMatchDTO says what decides which title an item is, for "Wrong title?" (decision 26).
type libraryMatchDTO struct {
	By   string `json:"by" enum:"app,file,you,search,none" doc:"What says which title this is: the app that manages it, the file itself (an id in its name, or an NFO beside it), your answer, a TMDB search that agreed on name, year and length, or nothing yet"`
	App  string `json:"app,omitempty" enum:"sonarr,radarr" doc:"The app, when by is app"`
	Page string `json:"page,omitempty" format:"uri" doc:"The title's page in that app, when by is app and it has one"`
}

func libraryMatchOf(m librarystore.LibraryMatch) libraryMatchDTO {
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
	Ready  bool               `json:"ready" doc:"False while some of its files' details aren't read yet"`
	Poster *httpcore.ImageDTO `json:"poster,omitempty" doc:"Its poster; absent when it has none, or while one is still being fetched"`
	// A title with either count above zero needs a look (#2151 decision 24).
	Unplayable  int `json:"unplayable" doc:"Files that can't be played: their details couldn't be read"`
	ExtraCopies int `json:"extraCopies" doc:"A movie's files that never play, beyond the copy of each edition (or each part) that does"`
}

func libraryItemOf(it librarystore.LibraryItemSummary, posters map[string]*httpcore.ImageDTO) libraryItemDTO {
	extra := 0
	if it.Kind == "movie" {
		extra = it.ExtraCopies
	}
	return libraryItemDTO{
		ID: it.ID, Kind: it.Kind, Name: it.Name, Year: it.Year, Files: it.Files, Ready: it.Pending == 0,
		Unplayable: it.Unplayable, ExtraCopies: extra,
		Poster: posters[it.Poster],
	}
}

// libraryPosters renders the items' posters, by hash. An image whose bytes have not arrived is
// left out, so the grid shows the title's name rather than a broken image.
func (s *Handlers) libraryPosters(ctx context.Context, items []librarystore.LibraryItemSummary) map[string]*httpcore.ImageDTO {
	out := map[string]*httpcore.ImageDTO{}
	if s.libraryImageFn == nil {
		return out
	}
	for _, it := range items {
		if it.Poster == "" {
			continue
		}
		if _, seen := out[it.Poster]; seen {
			continue
		}
		out[it.Poster] = s.libraryImage(ctx, it.Poster)
	}
	return out
}

type listLibraryItemsInput struct {
	Kind   string `query:"kind" enum:"movie,series" doc:"Only movies or only series; both when absent"`
	Search string `query:"search" maxLength:"200" doc:"Part of the name, any case"`
	Ready  bool   `query:"ready" doc:"Only titles with every file's details read"`
	Sort   string `query:"sort" enum:"name,recent" default:"name" doc:"By name, or the titles written last first"`
	Limit  int    `query:"limit" minimum:"1" maximum:"200" default:"60"`
	Offset int    `query:"offset" minimum:"0"`
}

type listLibraryItemsOutput struct {
	Body struct {
		Items []libraryItemDTO `json:"items"`
		Total int              `json:"total" doc:"Titles matching, across every page"`
	}
}

func (s *Handlers) listLibraryItems(ctx context.Context, in *listLibraryItemsInput) (*listLibraryItemsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	page, err := svc.Items(ctx, librarystore.LibraryItemQuery{
		Kind: in.Kind, Search: in.Search, Ready: in.Ready, Recent: in.Sort == "recent",
		Limit: min(in.Limit, libraryItemsPageMax), Offset: in.Offset,
	})
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list the library", "Try again in a moment.", err)
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
		Path: f.Path, Size: f.Size, Ready: f.Header != librarystore.LibraryHeaderPending,
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
	// FromFile waits on the household saying which order the show's files follow (#2151 decision 53).
	FromFile bool `json:"fromFile" doc:"Its name and number are its file's, until the show's episode order is said"`
}

type librarySeasonDTO struct {
	Number   int                 `json:"number" doc:"0 is the specials"`
	Episodes []libraryEpisodeDTO `json:"episodes"`
}

type libraryItemDetailDTO struct {
	// Item is a named field: huma leaves an unexported embedded struct out of the schema.
	Item libraryItemDTO `json:"item"`
	// LogoImage and BackdropImage are drawn in place of a typed title and behind the details
	// (#2567). A logo keeps its transparency: its fallback src is WebP, never JPEG.
	LogoImage     *httpcore.ImageDTO    `json:"logoImage,omitempty" doc:"Its clear logo; absent when it has none, or while one is still being fetched"`
	BackdropImage *httpcore.ImageDTO    `json:"backdropImage,omitempty" doc:"Its fanart; absent when it has none, or while one is still being fetched"`
	Overview      string                `json:"overview,omitempty"`
	Genres        []string              `json:"genres"`
	Rating        string                `json:"rating,omitempty" doc:"The certification, such as TV-14"`
	RuntimeMillis int64                 `json:"runtimeMillis,omitempty"`
	Files         []libraryItemFileDTO  `json:"files" doc:"A movie's files; empty for a series"`
	Seasons       []librarySeasonDTO    `json:"seasons" doc:"A series' episodes by season; empty for a movie"`
	Match         libraryMatchDTO       `json:"match"`
	DetailsFrom   libraryDetailsFromDTO `json:"detailsFrom"`
	Channels      []libraryChannelDTO   `json:"channels" doc:"The channels whose lineup holds it, by number"`
}

// libraryChannelDTO is a channel as the Library names it: on a title's page, or using a collection.
type libraryChannelDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type getLibraryItemInput struct {
	ID string `path:"id" maxLength:"64"`
}

type getLibraryItemOutput struct {
	Body libraryItemDetailDTO
}

func (s *Handlers) getLibraryItem(ctx context.Context, in *getLibraryItemInput) (*getLibraryItemOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	d, err := svc.Item(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.Problem(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read this title", "Try again in a moment.", err)
	}
	posters := s.libraryPosters(ctx, []librarystore.LibraryItemSummary{d.LibraryItemSummary})
	body := libraryItemDetailOf(d, posters)
	body.LogoImage, body.BackdropImage = s.libraryImage(ctx, d.Logo), s.libraryImage(ctx, d.Backdrop)
	return &getLibraryItemOutput{Body: body}, nil
}

func libraryItemDetailOf(d scan.ItemDetail, posters map[string]*httpcore.ImageDTO) libraryItemDetailDTO {
	out := libraryItemDetailDTO{
		Item:     libraryItemOf(d.LibraryItemSummary, posters),
		Overview: d.Facts.Overview, Genres: d.Facts.Genres, Rating: d.Facts.OfficialRating,
		RuntimeMillis: d.Facts.RuntimeMillis,
		Files:         make([]libraryItemFileDTO, 0, len(d.Files)),
		Seasons:       make([]librarySeasonDTO, 0, len(d.Seasons)),
		Channels:      make([]libraryChannelDTO, 0, len(d.Channels)),
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
	for _, c := range d.Channels {
		out.Channels = append(out.Channels, libraryChannelDTO{ID: c.ID, Name: c.Name, Number: c.Number})
	}
	for _, season := range d.Seasons {
		dto := librarySeasonDTO{Number: season.Number, Episodes: make([]libraryEpisodeDTO, 0, len(season.Episodes))}
		for _, ep := range season.Episodes {
			dto.Episodes = append(dto.Episodes, libraryEpisodeDTO{ID: ep.ID, Name: ep.Name, Number: ep.Number, End: ep.End, Ready: ep.Ready,
				FromFile: ep.FromFile})
		}
		out.Seasons = append(out.Seasons, dto)
	}
	return out
}
