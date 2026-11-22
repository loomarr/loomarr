package library

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Library → Items (#2151 step 10, decisions 22 and 25): the movies and series Loomarr found in its
// media folders, and one title's page. A member reads both, without the admin-only facts
// (#4476, library.md "Who reads the Library"); every fix is admin-only.

// libraryItemsPageMax caps one page of the poster grid.
const libraryItemsPageMax = 200

// readsAdminFacts reports whether the caller reads the Library's admin-only facts: a title's
// files and paths, what decides which title it is, where its details came from, and the counts
// and marks that send it to Needs you. Every Library response asks here, once per request, and
// leaves those parts out for anyone but an admin; the client hides nothing on its own.
func readsAdminFacts(ctx context.Context) bool {
	return httpcore.RoleFrom(ctx) == httpcore.RoleAdmin
}

func (s *Handlers) registerLibraryItems(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listLibraryItems", Method: http.MethodGet, Path: "/v1/library/items",
		Summary: "List the library's titles",
		Description: "Movies and series Loomarr found in its media folders, by name. A title still getting ready has files whose details aren't read yet. " +
			"A member's titles leave out the admin-only counts.",
		Tags: []string{"library"},
	}, httpcore.RoleMember), s.listLibraryItems)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "getLibraryItem", Method: http.MethodGet, Path: "/v1/library/items/{id}",
		Summary: "One title",
		Description: "A movie with its files, or a series with its episodes by season. 404 when no file of it is present. " +
			"A member's title leaves out its files, its match and where its details came from.",
		Tags: []string{"library"},
	}, httpcore.RoleMember), s.getLibraryItem)
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
	case errors.Is(err, scan.ErrPlayingCopyUnseen):
		return nil, httpcore.Problem(http.StatusConflict, "Kept this copy",
			"Loomarr can't find the copy that plays right now, so it kept this one. Check that its drive is connected, then try again.")
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
	ID   string `json:"id"`
	Kind string `json:"kind" enum:"movie,series"`
	Name string `json:"name"`
	Year int    `json:"year,omitempty" doc:"Absent when unknown"`
	// Files is present for an admin, zero included: beside the playing copy's line it would say
	// how many other copies a title has.
	Files *int `json:"files,omitempty" doc:"Present files; a series counts its episodes' files. Admins only"`
	// Ready is false while any file's details are unread.
	Ready  bool               `json:"ready" doc:"False while some of its files' details aren't read yet"`
	Poster *httpcore.ImageDTO `json:"poster,omitempty" doc:"Its poster; absent when it has none, or while one is still being fetched"`
	// A title with either count above zero needs a look (#2151 decision 24). Both are present
	// for an admin, zero included.
	Unplayable  *int `json:"unplayable,omitempty" doc:"Files that can't be played: their details couldn't be read. Admins only"`
	ExtraCopies *int `json:"extraCopies,omitempty" doc:"A movie's files that never play, beyond the copy of each edition (or each part) that does. Admins only"`
}

// libraryItemOf is it as the caller reads it: admin carries its file count and the counts that
// send it to Needs you.
func libraryItemOf(it librarystore.LibraryItemSummary, posters map[string]*httpcore.ImageDTO, admin bool) libraryItemDTO {
	out := libraryItemDTO{
		ID: it.ID, Kind: it.Kind, Name: it.Name, Year: it.Year, Ready: it.Pending == 0,
		Poster: posters[it.Poster],
	}
	if admin {
		files, unplayable, extra := it.Files, it.Unplayable, 0
		if it.Kind == "movie" {
			extra = it.ExtraCopies
		}
		out.Files, out.Unplayable, out.ExtraCopies = &files, &unplayable, &extra
	}
	return out
}

// libraryPosters renders the items' posters, by hash. An image whose bytes have not arrived is
// left out, so the grid shows the title's name rather than a broken image.
func (s *Handlers) libraryPosters(ctx context.Context, items []librarystore.LibraryItemSummary) map[string]*httpcore.ImageDTO {
	if s.libraryImagesFn == nil {
		return map[string]*httpcore.ImageDTO{}
	}
	hashes := make([]string, 0, len(items))
	for _, it := range items {
		if it.Poster != "" {
			hashes = append(hashes, it.Poster)
		}
	}
	// One read for the page's posters, not one per title.
	return s.libraryImagesFn(ctx, hashes)
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
	admin := readsAdminFacts(ctx)
	page, err := svc.Items(ctx, librarystore.LibraryItemQuery{
		Kind: in.Kind, Search: in.Search, Ready: in.Ready, Recent: in.Sort == "recent",
		Member: !admin, Limit: min(in.Limit, libraryItemsPageMax), Offset: in.Offset,
	})
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list the library", "Try again in a moment.", err)
	}
	out := &listLibraryItemsOutput{}
	out.Body.Total = page.Total
	out.Body.Items = make([]libraryItemDTO, 0, len(page.Items))
	posters := s.libraryPosters(ctx, page.Items)
	for _, it := range page.Items {
		out.Body.Items = append(out.Body.Items, libraryItemOf(it, posters, admin))
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
	Part          int                  `json:"part,omitempty" doc:"Which part of a movie split over several files it is, from 1; absent for a whole file. Every part plays, in order"`
	Edition       string               `json:"edition,omitempty" doc:"The labelled version of the title it is, such as Extended; absent for the default"`
	Container     string               `json:"container,omitempty" doc:"The format its header names, such as matroska,webm"`
	Bitrate       int64                `json:"bitrate,omitempty" doc:"Its overall bit rate in bit/s"`
	Streams       []libraryStreamDTO   `json:"streams" doc:"Every track in order, the file's own then the files beside it; empty while unread"`
	Chapters      []libraryChapterDTO  `json:"chapters" doc:"Its chapters in order; empty for none or while unread"`
}

// libraryStreamDTO is one track of a file. A value the file doesn't state is absent.
type libraryStreamDTO struct {
	Index           int    `json:"index" doc:"Its place among the file's tracks, from 0; a track in a file beside it follows the file's own"`
	Kind            string `json:"kind" enum:"video,audio,subtitle"`
	Codec           string `json:"codec,omitempty"`
	Profile         string `json:"profile,omitempty"`
	BitDepth        int    `json:"bitDepth,omitempty" doc:"Bits per sample; absent for lossy audio"`
	Bitrate         int    `json:"bitrate,omitempty" doc:"The track's own bit rate in bit/s"`
	Width           int    `json:"width,omitempty" doc:"A video track's width in pixels"`
	Height          int    `json:"height,omitempty" doc:"A video track's height in pixels"`
	FrameRate       string `json:"frameRate,omitempty" doc:"As the file states it, such as 24000/1001"`
	DynamicRange    string `json:"dynamicRange,omitempty" enum:"Dolby Vision,HDR10,HLG,HDR" doc:"A video track's high dynamic range; absent for standard range"`
	Language        string `json:"language,omitempty" doc:"ISO 639-1 where one exists, else as the file says it"`
	LanguageName    string `json:"languageName,omitempty" doc:"The language's English name"`
	Channels        int    `json:"channels,omitempty" doc:"An audio track's channel count"`
	ChannelLayout   string `json:"channelLayout,omitempty" doc:"An audio track's layout, such as 5.1(side)"`
	AudioFormat     string `json:"audioFormat,omitempty" doc:"An audio track's format by name, such as Dolby TrueHD"`
	Title           string `json:"title,omitempty"`
	Default         bool   `json:"default"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearingImpaired" doc:"Subtitles for the deaf and hard of hearing"`
	Commentary      bool   `json:"commentary"`
	Descriptive     bool   `json:"descriptive" doc:"Audio that describes the picture"`
	External        bool   `json:"external" doc:"A file beside the media file holds it"`
}

type libraryChapterDTO struct {
	StartMillis int64  `json:"startMillis"`
	Name        string `json:"name,omitempty" doc:"Absent when the file names none"`
}

// libraryCreditDTO is one person credited on a title.
type libraryCreditDTO struct {
	Name      string `json:"name"`
	Role      string `json:"role" enum:"actor,director,writer,creator"`
	Character string `json:"character,omitempty" doc:"An actor's part"`
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
	Year         string `json:"year,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its year came from"`
	Score        string `json:"score,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its score came from"`
	Runtime      string `json:"runtime,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its runtime came from"`
	Premiere     string `json:"premiere,omitempty" enum:"sonarr,radarr,nfo,tmdb" doc:"Where its premiere date came from"`
}

// libraryFileOf is f with what its header says.
func libraryFileOf(f scan.ItemFile) libraryItemFileDTO {
	out := libraryItemFileDTO{
		Path: f.Path, Size: f.Size, Ready: f.Header != librarystore.LibraryHeaderPending,
		LengthMillis: f.LengthMillis, AudioChannels: f.AudioChannels, Extra: f.Extra, Part: f.Part,
		Edition: f.Edition, Container: f.Container, Bitrate: f.Bitrate,
		Subtitles: make([]librarySubtitleDTO, 0, len(f.Subtitles)),
		Streams:   make([]libraryStreamDTO, 0, len(f.Streams)),
		Chapters:  make([]libraryChapterDTO, 0, len(f.Chapters)),
	}
	if f.Picture != nil {
		out.Picture = &libraryPictureDTO{Width: f.Picture.Width, Height: f.Picture.Height, HDR: f.Picture.HDR}
	}
	for _, language := range f.Subtitles {
		out.Subtitles = append(out.Subtitles, librarySubtitleDTO{Language: language})
	}
	for _, s := range f.Streams {
		out.Streams = append(out.Streams, libraryStreamDTO{
			Index: s.Index, Kind: s.Kind, Codec: s.Codec, Profile: s.Profile, BitDepth: s.BitDepth, Bitrate: s.BitRate,
			Width: s.Width, Height: s.Height, FrameRate: s.FrameRate, DynamicRange: s.HDR, Language: s.Language,
			LanguageName: s.LanguageName, Channels: s.Channels, ChannelLayout: s.ChannelLayout,
			AudioFormat: s.AudioFormat, Title: s.Title, Default: s.Default, Forced: s.Forced,
			HearingImpaired: s.HearingImpaired, Commentary: s.Commentary, Descriptive: s.Descriptive,
			External: s.ExternalRef != "",
		})
	}
	for _, c := range f.Chapters {
		out.Chapters = append(out.Chapters, libraryChapterDTO{StartMillis: c.StartMs, Name: c.Title})
	}
	return out
}

type libraryEpisodeDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
	End    int    `json:"end,omitempty" doc:"The last episode a multi-episode file holds"`
	Ready  bool   `json:"ready"`
	// FromFile waits on the household saying which order the show's files follow (#2151 decision 53);
	// present for an admin, false included.
	FromFile *bool  `json:"fromFile,omitempty" doc:"Its name and number are its file's, until the show's episode order is said. Admins only"`
	Overview string `json:"overview,omitempty" doc:"Its own story"`
	// LengthMillis reads its files' durations only, never their tracks.
	LengthMillis int64 `json:"lengthMillis,omitempty" doc:"How long it plays: measured from its files once each is read, else its catalog's runtime; absent when neither is known"`
	Still        *httpcore.ImageDTO `json:"still,omitempty" doc:"Its still; absent when it has none, or while one is still being fetched"`
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
	LogoImage     *httpcore.ImageDTO  `json:"logoImage,omitempty" doc:"Its clear logo; absent when it has none, or while one is still being fetched"`
	BackdropImage *httpcore.ImageDTO  `json:"backdropImage,omitempty" doc:"Its fanart; absent when it has none, or while one is still being fetched"`
	Still         *httpcore.ImageDTO  `json:"still,omitempty" doc:"An episode's still; absent for a movie or a series, when it has none, or while one is still being fetched"`
	Overview      string              `json:"overview,omitempty"`
	Genres        []string            `json:"genres"`
	Rating        string              `json:"rating,omitempty" doc:"The certification, such as TV-14"`
	RuntimeMillis int64               `json:"runtimeMillis,omitempty"`
	OriginalTitle string              `json:"originalTitle,omitempty" doc:"Its title in its own language, as the source gives it"`
	Tagline       string              `json:"tagline,omitempty" doc:"Its one-line pitch"`
	PremiereDate  string              `json:"premiereDate,omitempty" format:"date" doc:"The day it first aired or was released"`
	Score         float64             `json:"score,omitempty" doc:"The community score, 0 to 10"`
	Studios       []string            `json:"studios" doc:"A movie's or a series' studios; empty for an episode, whose studios are its show's"`
	Countries     []string            `json:"countries" doc:"A movie's or a series' countries; empty for an episode, whose countries are its show's"`
	Credits       []libraryCreditDTO  `json:"credits" doc:"A movie's or a series' cast in billing order, then its key crew; empty for an episode, whose credits are its show's"`
	Playing       *libraryPlayingDTO  `json:"playing,omitempty" doc:"The copy that plays, in one line; absent for a series, and while none of its files plays"`
	Seasons       []librarySeasonDTO  `json:"seasons" doc:"A series' episodes by season; empty for a movie or an episode"`
	Channels      []libraryChannelDTO `json:"channels" doc:"The channels whose lineup holds it, by number"`
	// Files, Match and DetailsFrom are present for an admin, an empty files list included.
	Files       *[]libraryItemFileDTO  `json:"files,omitempty" doc:"A movie's files, or an episode's; empty for a series. Admins only"`
	Match       *libraryMatchDTO       `json:"match,omitempty" doc:"What decides which title it is. Admins only"`
	DetailsFrom *libraryDetailsFromDTO `json:"detailsFrom,omitempty" doc:"Where its details came from. Admins only"`
}

// libraryPlayingDTO is the copy of a title that plays, as one plain line on its page. It names no
// file: a member reads it in place of the files.
type libraryPlayingDTO struct {
	Picture           *libraryPictureDTO `json:"picture,omitempty" doc:"Its first video stream; absent while its details are unread"`
	AudioChannels     int                `json:"audioChannels,omitempty" doc:"The channels of the sound that plays: its default audio stream, else its first"`
	SubtitleLanguages []string           `json:"subtitleLanguages" doc:"Its subtitles' languages, each once, in order: its own then the files beside it; empty for none, unknown or unread"`
}

// libraryPlayingOf is d's playing copy in one line, or nil when none of its files plays.
func libraryPlayingOf(d scan.ItemDetail) *libraryPlayingDTO {
	f, ok := d.PlayingCopy()
	if !ok {
		return nil
	}
	out := &libraryPlayingDTO{AudioChannels: f.AudioChannels, SubtitleLanguages: []string{}}
	if f.Picture != nil {
		out.Picture = &libraryPictureDTO{Width: f.Picture.Width, Height: f.Picture.Height, HDR: f.Picture.HDR}
	}
	for _, language := range f.Subtitles {
		if language != "" && !slices.Contains(out.SubtitleLanguages, language) {
			out.SubtitleLanguages = append(out.SubtitleLanguages, language)
		}
	}
	return out
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
	admin := readsAdminFacts(ctx)
	// A title a member doesn't read answers the same 404 as an id that names nothing.
	d, err := svc.Item(ctx, in.ID, !admin)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.Problem(http.StatusNotFound, "Not in the library", "No file of this title is in your media folders.")
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read this title", "Try again in a moment.", err)
	}
	images := s.pageImages(ctx, d, admin)
	body := libraryItemDetailOf(d, images, admin)
	body.LogoImage, body.BackdropImage = images[d.Logo], images[d.Backdrop]
	if d.Still != "" {
		body.Still = images[d.Still]
	}
	return &getLibraryItemOutput{Body: body}, nil
}

// episodeShown is whether the caller reads ep in its series' season: an episode still named by
// its file, whether it waits on the show's order in Needs you or on its show's catalogue record,
// is left out of a member's.
func episodeShown(ep scan.Episode, admin bool) bool {
	return admin || (!ep.FromFile && ep.Named)
}

// pageImages renders, by hash, every image d's page draws in one read (#4476): its poster, logo,
// fanart and still, and the stills of the episodes the caller reads, whose hashes came with the
// page's one read of its episodes. A hidden episode's still is not read, so it cannot be sent.
func (s *Handlers) pageImages(ctx context.Context, d scan.ItemDetail, admin bool) map[string]*httpcore.ImageDTO {
	if s.libraryImagesFn == nil {
		return map[string]*httpcore.ImageDTO{}
	}
	var hashes []string
	for _, h := range []string{d.Poster, d.Logo, d.Backdrop, d.Still} {
		if h != "" {
			hashes = append(hashes, h)
		}
	}
	for _, season := range d.Seasons {
		for _, ep := range season.Episodes {
			if ep.Still != "" && episodeShown(ep, admin) {
				hashes = append(hashes, ep.Still)
			}
		}
	}
	return s.libraryImagesFn(ctx, hashes)
}

// libraryItemDetailOf is d's page as the caller reads it: admin carries its files, its match,
// where its details came from and each episode's fromFile. images renders its poster and its
// episodes' stills by hash.
func libraryItemDetailOf(d scan.ItemDetail, images map[string]*httpcore.ImageDTO, admin bool) libraryItemDetailDTO {
	out := libraryItemDetailDTO{
		Item:     libraryItemOf(d.LibraryItemSummary, images, admin),
		Overview: d.Facts.Overview, Genres: d.Facts.Genres, Rating: d.Facts.OfficialRating,
		RuntimeMillis: d.Facts.RuntimeMillis,
		OriginalTitle: d.Facts.OriginalTitle, Tagline: d.Facts.Tagline, PremiereDate: d.Facts.PremiereDate,
		Score:     d.Facts.CommunityScore,
		Studios:   append([]string{}, d.Facts.Studios...),
		Countries: append([]string{}, d.Facts.Countries...),
		Credits:   make([]libraryCreditDTO, 0, len(d.Facts.People)),
		Playing:   libraryPlayingOf(d),
		Seasons:   make([]librarySeasonDTO, 0, len(d.Seasons)),
		Channels:  make([]libraryChannelDTO, 0, len(d.Channels)),
	}
	if out.Genres == nil {
		out.Genres = []string{}
	}
	for _, p := range d.Facts.People {
		out.Credits = append(out.Credits, libraryCreditDTO{Name: p.Name, Role: p.Role, Character: p.Character})
	}
	for _, c := range d.Channels {
		out.Channels = append(out.Channels, libraryChannelDTO{ID: c.ID, Name: c.Name, Number: c.Number})
	}
	for _, season := range d.Seasons {
		dto := librarySeasonDTO{Number: season.Number, Episodes: make([]libraryEpisodeDTO, 0, len(season.Episodes))}
		for _, ep := range season.Episodes {
			// A season of only episodes a member doesn't read is left out of theirs whole.
			if !episodeShown(ep, admin) {
				continue
			}
			episode := libraryEpisodeDTO{ID: ep.ID, Name: ep.Name, Number: ep.Number, End: ep.End, Ready: ep.Ready,
				Overview: ep.Overview, LengthMillis: ep.LengthMillis}
			if ep.Still != "" {
				episode.Still = images[ep.Still]
			}
			if admin {
				episode.FromFile = &ep.FromFile
			}
			dto.Episodes = append(dto.Episodes, episode)
		}
		if admin || len(dto.Episodes) > 0 {
			out.Seasons = append(out.Seasons, dto)
		}
	}
	if !admin {
		return out
	}
	files := make([]libraryItemFileDTO, 0, len(d.Files))
	for _, f := range d.Files {
		files = append(files, libraryFileOf(f))
	}
	match := libraryMatchOf(d.Match)
	out.Files, out.Match = &files, &match
	out.DetailsFrom = &libraryDetailsFromDTO{
		Description: string(d.From.Description), Rating: string(d.From.Rating), EpisodeNames: string(d.EpisodeNamesFrom),
		Year: string(d.From.Year), Score: string(d.From.Score), Runtime: string(d.From.Runtime),
		Premiere: string(d.From.Premiere),
	}
	return out
}
