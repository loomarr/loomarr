package library

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The choices of the wall's filters, and the people titles credit (#4476, library.md "Read
// path"). All three are member-readable and read only the titles the caller's list does.

func (s *Handlers) registerLibraryFilters(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listLibraryFacets", Method: http.MethodGet, Path: "/v1/library/facets",
		Summary: "The library's filter choices",
		Description: "The studios, audio languages, subtitle languages, 4K and HDR of the titles the list shows, each with how many " +
			"titles it keeps. A member's counts leave out the titles a member doesn't read.",
		Tags: []string{"library"},
	}, httpcore.RoleMember), s.listLibraryFacets)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "searchLibraryPeople", Method: http.MethodGet, Path: "/v1/library/people",
		Summary: "Find people in the library",
		Description: "People the listed titles credit, by part of their name, the most credited first. " +
			"A member finds only people their titles credit.",
		Tags: []string{"library"},
	}, httpcore.RoleMember), s.searchLibraryPeople)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "getLibraryPerson", Method: http.MethodGet, Path: "/v1/library/people/{id}",
		Summary: "One person",
		Description: "Their name and how many of the listed titles credit them; list those titles with person=<id>. " +
			"404 when no title the caller reads credits them.",
		Tags: []string{"library"},
	}, httpcore.RoleMember), s.getLibraryPerson)
}

type listLibraryFacetsInput struct {
	Kind string `query:"kind" enum:"movie,series" doc:"Only movies' or only series' choices; both when absent"`
}

type libraryFacetDTO struct {
	Value  string `json:"value" doc:"What the list's filter takes"`
	Name   string `json:"name" doc:"What to call it"`
	Titles int    `json:"titles" doc:"Titles it keeps"`
}

type listLibraryFacetsOutput struct {
	Body struct {
		Studios   []libraryFacetDTO `json:"studios" doc:"For studio=, by name"`
		Audio     []libraryFacetDTO `json:"audio" doc:"For audio=, by language name"`
		Subtitles []libraryFacetDTO `json:"subtitles" doc:"For subtitles=, by language name"`
		UHD       int               `json:"uhd" doc:"Titles uhd=true keeps"`
		HDRAny    int               `json:"hdrAny" doc:"Titles hdr=any keeps"`
		HDR       []libraryFacetDTO `json:"hdr" doc:"For hdr=, one per kind of high dynamic range present"`
	}
}

func libraryFacetsOf(in []librarystore.LibraryFacet) []libraryFacetDTO {
	out := make([]libraryFacetDTO, 0, len(in))
	for _, f := range in {
		out = append(out, libraryFacetDTO{Value: f.Value, Name: f.Name, Titles: f.Titles})
	}
	return out
}

func (s *Handlers) listLibraryFacets(ctx context.Context, in *listLibraryFacetsInput) (*listLibraryFacetsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	f, err := svc.Facets(ctx, in.Kind, !readsAdminFacts(ctx))
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list the library's filters", "Try again in a moment.", err)
	}
	out := &listLibraryFacetsOutput{}
	out.Body.Studios, out.Body.Audio, out.Body.Subtitles = libraryFacetsOf(f.Studios), libraryFacetsOf(f.Audio), libraryFacetsOf(f.Subtitles)
	out.Body.UHD, out.Body.HDRAny, out.Body.HDR = f.UHD, f.HDRAny, libraryFacetsOf(f.HDR)
	return out, nil
}

type libraryPersonDTO struct {
	ID     string `json:"id" doc:"What the list's person= takes"`
	Name   string `json:"name"`
	Titles int    `json:"titles" doc:"Listed titles that credit them"`
}

func libraryPersonOf(p librarystore.LibraryPerson) libraryPersonDTO {
	return libraryPersonDTO{ID: p.ID, Name: p.Name, Titles: p.Titles}
}

type searchLibraryPeopleInput struct {
	Q     string `query:"q" maxLength:"200" doc:"Part of the name, any case; the most credited people when absent"`
	Kind  string `query:"kind" enum:"movie,series" doc:"Only people of movies or of series; both when absent"`
	Limit int    `query:"limit" minimum:"1" maximum:"50" default:"20"`
}

type searchLibraryPeopleOutput struct {
	Body struct {
		People []libraryPersonDTO `json:"people" doc:"The most credited first, then by name"`
	}
}

func (s *Handlers) searchLibraryPeople(ctx context.Context, in *searchLibraryPeopleInput) (*searchLibraryPeopleOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	people, err := svc.People(ctx, in.Q, in.Kind, !readsAdminFacts(ctx), in.Limit)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't search the library's people", "Try again in a moment.", err)
	}
	out := &searchLibraryPeopleOutput{}
	out.Body.People = make([]libraryPersonDTO, 0, len(people))
	for _, p := range people {
		out.Body.People = append(out.Body.People, libraryPersonOf(p))
	}
	return out, nil
}

type getLibraryPersonInput struct {
	ID string `path:"id" maxLength:"600"`
}

type getLibraryPersonOutput struct {
	Body libraryPersonDTO
}

func (s *Handlers) getLibraryPerson(ctx context.Context, in *getLibraryPersonInput) (*getLibraryPersonOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	p, err := svc.Person(ctx, in.ID, !readsAdminFacts(ctx))
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.Problem(http.StatusNotFound, "Not in the library", "No title in the library credits this person.")
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read this person", "Try again in a moment.", err)
	}
	return &getLibraryPersonOutput{Body: libraryPersonOf(p)}, nil
}
