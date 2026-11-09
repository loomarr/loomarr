package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Library → Needs you (#2151 step 11, decision 27): the titles nothing grounds, with what a search
// found for each, and the maintainer's answer. The API says which facts agree; the words are the
// UI's.

// libraryCandidateImageOwner owns the posters of a title's candidates, so the image service keeps
// them while the title is listed.
const libraryCandidateImageOwner = "library_candidate"

func (s *Server) registerLibraryNeeds(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listLibraryNeeds", Method: http.MethodGet, Path: "/v1/library/needs",
		Summary:     "Titles that need you",
		Description: "Titles Loomarr can't tell what they are, each with the best guesses a search found, best first.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.listLibraryNeeds)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "linkLibraryItem", Method: http.MethodPost, Path: "/v1/library/items/{id}/link",
		Summary:     "Say which title this is",
		Description: "Links a title to a TMDB title. Its files are read again with it.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.linkLibraryItem)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "ignoreLibraryItem", Method: http.MethodPost, Path: "/v1/library/items/{id}/ignore",
		Summary:     "Ignore a title",
		Description: "Says a title isn't one: it leaves the library and Needs you.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.ignoreLibraryItem)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "unanswerLibraryItem", Method: http.MethodDelete, Path: "/v1/library/items/{id}/answer",
		Summary:     "Take back an answer",
		Description: "Undoes a link or an ignore. A title nothing else identifies needs you again.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.unanswerLibraryItem)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "chooseLibraryEpisodeOrder", Method: http.MethodPost, Path: "/v1/library/items/{id}/episode-order",
		Summary:     "Say which order a show's episodes are in",
		Description: "TMDB's own seasons, one of its other orders, or the names and numbers in the files. Its episodes are read again with it.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.chooseLibraryEpisodeOrder)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "searchLibraryTitles", Method: http.MethodGet, Path: "/v1/library/search",
		Summary:     "Search TMDB for a title",
		Description: "For picking a different title. 503 when TMDB isn't set up.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.searchLibraryTitles)
}

// externalIDDTO is a title by one catalog's id.
type externalIDDTO struct {
	Namespace string `json:"namespace" minLength:"1" maxLength:"40" doc:"The catalog the id is from, such as tmdb"`
	Value     string `json:"value" minLength:"1" maxLength:"40" doc:"The id in that catalog"`
}

type libraryCandidateDTO struct {
	Kind       string        `json:"kind" enum:"movie,series"`
	ExternalID externalIDDTO `json:"externalId"`
	Name       string        `json:"name"`
	Year       int           `json:"year,omitempty" doc:"Absent when unknown"`
	Overview   string        `json:"overview,omitempty"`
	Poster     *ImageDTO     `json:"poster,omitempty" doc:"Absent when it has none, or while it is still being fetched"`
	// The agreement flags say which of the file's facts the candidate shares.
	NameMatches     bool `json:"nameMatches"`
	YearMatches     bool `json:"yearMatches"`
	LengthMatches   bool `json:"lengthMatches" doc:"Movies only: its runtime and the file's length agree"`
	EpisodesMatches bool `json:"episodesMatches" doc:"Series only: every season and episode on disk fits its seasons"`
}

type libraryNeedDTO struct {
	Item       libraryItemDTO        `json:"item"`
	Searched   bool                  `json:"searched" doc:"False until TMDB has been searched for it"`
	Candidates []libraryCandidateDTO `json:"candidates" doc:"Best first; empty when a search found nothing"`
	Files      []libraryNeedFileDTO  `json:"files"`
}

// libraryNeedFileDTO is a file as a Needs you card lists it: no stream facts, which a title's page
// carries instead.
type libraryNeedFileDTO struct {
	Path  string `json:"path" doc:"The file on this machine"`
	Size  int64  `json:"size" doc:"Bytes"`
	Ready bool   `json:"ready" doc:"Its details are read"`
}

// libraryProblemDTO is a title that needs a look: its item says how many files can't be played and
// how many copies are extra.
type libraryProblemDTO struct {
	Item       libraryItemDTO `json:"item"`
	ExtraBytes int64          `json:"extraBytes" doc:"What a movie's extra copies take, in bytes; 0 for none"`
}

// librarySeasonSizeDTO is how many episodes one season has.
type librarySeasonSizeDTO struct {
	Season   int `json:"season" doc:"0 is specials"`
	Episodes int `json:"episodes"`
}

// libraryOrderNeedDTO is a show whose episodes keep their files' names and numbers until the
// household says which order its files follow.
type libraryOrderNeedDTO struct {
	Item    libraryItemDTO          `json:"item"`
	Disk    []librarySeasonSizeDTO  `json:"disk" doc:"How far each season runs on disk: its highest episode, by season"`
	Files   []libraryOrderFileDTO   `json:"files" doc:"The files of the episodes that wait, in path order"`
	Choices []libraryOrderChoiceDTO `json:"choices" doc:"TMDB's own seasons, then its other orders; empty while the show's details are fetched"`
}

type libraryOrderFileDTO struct {
	Path    string `json:"path" doc:"The file on this machine"`
	Season  int    `json:"season" doc:"As the file numbers it"`
	Episode int    `json:"episode" doc:"As the file numbers it"`
}

// libraryOrderChoiceDTO is one order a show's files may follow.
type libraryOrderChoiceDTO struct {
	Choice   string                 `json:"choice" enum:"catalog,order" doc:"catalog is TMDB's own seasons; order is one of its other orders"`
	OrderID  string                 `json:"orderId,omitempty" doc:"The order to send back; only with choice order"`
	Kind     string                 `json:"kind,omitempty" enum:"aired,absolute,dvd,digital,story,production,tv" doc:"What kind of order it is; only with choice order"`
	Name     string                 `json:"name,omitempty" doc:"TMDB's name for it; only with choice order"`
	Seasons  []librarySeasonSizeDTO `json:"seasons" doc:"Its layout; empty while an order's layout is fetched"`
	Episodes int                    `json:"episodes" doc:"How many episodes it has in all"`
	Fits     bool                   `json:"fits" doc:"Every season on disk ends where its season does"`
}

type listLibraryNeedsOutput struct {
	Body struct {
		Needs    []libraryNeedDTO      `json:"needs"`
		Orders   []libraryOrderNeedDTO `json:"orders" doc:"Shows whose episode order to say, by name"`
		Problems []libraryProblemDTO   `json:"problems" doc:"Titles that need a look, by name"`
	}
}

func (s *Server) listLibraryNeeds(ctx context.Context, _ *struct{}) (*listLibraryNeedsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	needs, err := svc.Needs(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list what needs you", "Try again in a moment.", err)
	}
	problems, err := svc.Problems(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list what needs you", "Try again in a moment.", err)
	}
	orders, err := svc.OrderNeeds(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list what needs you", "Try again in a moment.", err)
	}
	summaries := make([]store.LibraryItemSummary, 0, len(needs)+len(orders)+len(problems))
	for _, n := range needs {
		summaries = append(summaries, n.LibraryItemSummary)
	}
	for _, o := range orders {
		summaries = append(summaries, o.LibraryItemSummary)
	}
	summaries = append(summaries, problems...)
	posters := s.libraryPosters(ctx, summaries)
	out := &listLibraryNeedsOutput{}
	out.Body.Orders = make([]libraryOrderNeedDTO, 0, len(orders))
	for _, o := range orders {
		out.Body.Orders = append(out.Body.Orders, libraryOrderNeedOf(o, posters))
	}
	out.Body.Problems = make([]libraryProblemDTO, 0, len(problems))
	for _, p := range problems {
		out.Body.Problems = append(out.Body.Problems, libraryProblemDTO{
			Item: libraryItemOf(p, posters), ExtraBytes: p.ExtraBytes,
		})
	}
	out.Body.Needs = make([]libraryNeedDTO, 0, len(needs))
	for _, n := range needs {
		dto := libraryNeedDTO{
			Item: libraryItemOf(n.LibraryItemSummary, posters), Searched: n.Searched != "",
			Candidates: make([]libraryCandidateDTO, 0, len(n.Candidates)),
			Files:      make([]libraryNeedFileDTO, 0, len(n.Present)),
		}
		for _, c := range n.Candidates {
			dto.Candidates = append(dto.Candidates, libraryCandidateDTO{
				Kind: c.Kind, ExternalID: externalIDDTO{Namespace: c.ID.Namespace, Value: c.ID.Value}, Name: c.Name, Year: c.Year, Overview: c.Overview,
				Poster:      s.candidatePoster(ctx, n.ID, c.Poster),
				NameMatches: c.NameMatch, YearMatches: c.YearMatch,
				// One stored flag: what the files say beyond the name agrees, by kind.
				LengthMatches:   c.LengthMatch && c.Kind == "movie",
				EpisodesMatches: c.LengthMatch && c.Kind == "series",
			})
		}
		for _, f := range n.Present {
			dto.Files = append(dto.Files, libraryNeedFileDTO{Path: f.Path, Size: f.Size, Ready: f.Header != store.LibraryHeaderPending})
		}
		out.Body.Needs = append(out.Body.Needs, dto)
	}
	return out, nil
}

// candidatePoster adopts a candidate's poster, so the image service fetches it, and renders it once
// its bytes are here. Until then, or when it cannot be adopted, the card shows the name.
func (s *Server) candidatePoster(ctx context.Context, itemID, url string) *ImageDTO {
	if s.images == nil || url == "" {
		return nil
	}
	rec, err := s.images.Adopt(ctx, url, images.IngestRequest{
		Role: images.RolePoster, Visibility: images.VisibilityMember,
		OwnerKind: libraryCandidateImageOwner, OwnerID: itemID,
	})
	if err != nil || !rec.HasBytes() {
		return nil
	}
	dto := s.imageToDTO(ctx, rec)
	return &dto
}

type libraryItemIDInput struct {
	ID string `path:"id" maxLength:"64"`
}

type linkLibraryItemInput struct {
	ID   string `path:"id" maxLength:"64"`
	Body struct {
		ExternalID externalIDDTO `json:"externalId" doc:"The title it is, as a search result or candidate names it"`
	}
}

func (s *Server) linkLibraryItem(ctx context.Context, in *linkLibraryItemInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	id := store.LibraryID{Namespace: in.Body.ExternalID.Namespace, Value: in.Body.ExternalID.Value}
	return nil, libraryAnswerErr(svc.Link(ctx, in.ID, id))
}

func (s *Server) ignoreLibraryItem(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	return nil, libraryAnswerErr(svc.Ignore(ctx, in.ID))
}

func (s *Server) unanswerLibraryItem(ctx context.Context, in *libraryItemIDInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	return nil, libraryAnswerErr(svc.Unanswer(ctx, in.ID))
}

// libraryOrderNeedOf is an order need as Needs you shows it: the disk's seasons by number.
func libraryOrderNeedOf(o scan.OrderNeed, posters map[string]*ImageDTO) libraryOrderNeedDTO {
	dto := libraryOrderNeedDTO{
		Item:    libraryItemOf(o.LibraryItemSummary, posters),
		Disk:    make([]librarySeasonSizeDTO, 0, len(o.Last)),
		Files:   make([]libraryOrderFileDTO, 0, len(o.Waiting)),
		Choices: make([]libraryOrderChoiceDTO, 0, len(o.Choices)),
	}
	for season, last := range o.Last {
		dto.Disk = append(dto.Disk, librarySeasonSizeDTO{Season: season, Episodes: last})
	}
	slices.SortFunc(dto.Disk, func(a, b librarySeasonSizeDTO) int { return a.Season - b.Season })
	for _, f := range o.Waiting {
		dto.Files = append(dto.Files, libraryOrderFileDTO{Path: f.Path, Season: f.Season, Episode: f.Episode})
	}
	for _, c := range o.Choices {
		choice := libraryOrderChoiceDTO{
			Choice: string(c.Choice), Seasons: make([]librarySeasonSizeDTO, 0, len(c.Seasons)), Fits: c.Fits,
		}
		for _, s := range c.Seasons {
			choice.Seasons = append(choice.Seasons, librarySeasonSizeDTO{Season: s.Number, Episodes: s.Episodes})
			choice.Episodes += s.Episodes
		}
		if c.Choice == store.OrderByOrder {
			choice.OrderID, choice.Kind, choice.Name = c.Order.ID, string(c.Order.Kind), c.Order.Name
			choice.Episodes = max(choice.Episodes, c.Order.Episodes)
		}
		dto.Choices = append(dto.Choices, choice)
	}
	return dto
}

type chooseLibraryEpisodeOrderInput struct {
	ID   string `path:"id" maxLength:"64"`
	Body struct {
		Choice  string `json:"choice" enum:"catalog,order,files" doc:"files keeps the name and number in each file"`
		OrderID string `json:"orderId,omitempty" maxLength:"64" doc:"The order a choice names; only with choice order"`
	}
}

func (s *Server) chooseLibraryEpisodeOrder(ctx context.Context, in *chooseLibraryEpisodeOrderInput) (*struct{}, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	return nil, libraryAnswerErr(svc.ChooseOrder(ctx, in.ID, store.LibraryOrderChoice(in.Body.Choice), in.Body.OrderID))
}

func libraryAnswerErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return httpcore.Problem(http.StatusNotFound, "Not in the library", "This title isn't in your media folders.")
	case errors.Is(err, scan.ErrNotAnswerable):
		return httpcore.Problem(http.StatusConflict, "Loomarr didn't find this title itself",
			"Only titles Loomarr found in your media folders can be answered here.")
	case errors.Is(err, store.ErrBadEpisodeOrder):
		return httpcore.Problem(http.StatusBadRequest, "Pick one order", "Name an order only when you choose one of TMDB's other orders.")
	case errors.Is(err, scan.ErrUnknownOrder):
		return httpcore.Problem(http.StatusConflict, "TMDB no longer lists that order", "Reload Needs you and choose again.")
	default:
		return httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't save your answer", "Try again in a moment.", err)
	}
}

type searchLibraryTitlesInput struct {
	Query string `query:"q" minLength:"1" maxLength:"200" required:"true" doc:"The title's name"`
	Kind  string `query:"kind" enum:"movie,series" doc:"Only movies or only series; both when absent"`
}

type librarySearchHitDTO struct {
	Kind       string        `json:"kind" enum:"movie,series"`
	ExternalID externalIDDTO `json:"externalId"`
	Name       string        `json:"name"`
	Year       int           `json:"year,omitempty"`
	Overview   string        `json:"overview,omitempty"`
}

type searchLibraryTitlesOutput struct {
	Body struct {
		Results []librarySearchHitDTO `json:"results"`
	}
}

func (s *Server) searchLibraryTitles(ctx context.Context, in *searchLibraryTitlesInput) (*searchLibraryTitlesOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	hits, err := svc.SearchTitles(ctx, in.Query, in.Kind)
	if errors.Is(err, scan.ErrNoCatalog) {
		return nil, httpcore.Problem(http.StatusServiceUnavailable, "TMDB isn't set up", "Add a TMDB key in Settings to search for titles.")
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't search TMDB", "Try again in a moment.", err)
	}
	out := &searchLibraryTitlesOutput{}
	out.Body.Results = make([]librarySearchHitDTO, 0, len(hits))
	for _, h := range hits {
		out.Body.Results = append(out.Body.Results, librarySearchHitDTO{
			Kind: h.Kind, ExternalID: externalIDDTO{Namespace: string(h.ID.Namespace), Value: h.ID.Value}, Name: h.Name, Year: h.Year, Overview: h.Overview,
		})
	}
	return out, nil
}
