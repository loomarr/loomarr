package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// Home's Channel ideas (#1665): channels the household's own library could make, built without
// the LLM because it can be off. Members-only on Home (maintainer H4); the route is RoleMember like
// every Home read, so an admin can call it too. Each idea's reason is typed with its numbers and
// clients word it. Hides are per person.

// IdeaLibrary lists the media library's movies and series for idea building. The composition root
// adapts the library client and caches the listing, because Home reads ideas on every visit.
type IdeaLibrary interface {
	IdeaItems(ctx context.Context) ([]ideas.Item, error)
}

// ideaHolidayHorizon is how far ahead a holiday becomes an idea: long enough to request and build
// a channel before the window opens.
const ideaHolidayHorizon = 6 * 7 * 24 * time.Hour

func (s *Server) registerChannelIdeas(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-channel-ideas", Method: http.MethodGet, Path: "/v1/discovery/ideas",
		Summary: "Channel ideas from your library",
		Description: "Channels the library could make, for Home's Channel ideas (#1665). Built from the library's genres and " +
			"decades that no channel plays yet, and from holidays on now or within six weeks (titles about the holiday, " +
			"matched the way seasonal scheduling matches them). No LLM is involved. Holidays come first, soonest first, " +
			"then the ideas that would put the most titles on air. The caller's hidden ideas are left out. Clients page " +
			"through the list for \"Different ideas\".",
		Tags: []string{"discovery"},
	}, RoleMember), s.listChannelIdeas)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "hide-channel-idea", Method: http.MethodPut, Path: "/v1/me/hidden-ideas/{ideaId}",
		Summary:     "Hide a channel idea",
		Description: "Hides the idea for the caller only. Idempotent. DELETE is the undo.",
		Tags:        []string{"discovery"},
	}, RoleMember), s.hideChannelIdea)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "unhide-channel-idea", Method: http.MethodDelete, Path: "/v1/me/hidden-ideas/{ideaId}",
		Summary:     "Undo hiding a channel idea",
		Description: "Brings a hidden idea back for the caller; unhiding one that isn't hidden is not an error.",
		Tags:        []string{"discovery"},
	}, RoleMember), s.unhideChannelIdea)
}

type channelIdeaReasonDTO struct {
	Kind         string `json:"kind" enum:"unaired,holiday" doc:"unaired: count library titles in this facet play on no channel. holiday: a holiday window is on or ahead and count titles are about it."`
	Count        int    `json:"count"`
	HolidayID    string `json:"holidayId,omitempty" doc:"holiday only"`
	HolidayLabel string `json:"holidayLabel,omitempty" doc:"holiday only; the display name the seasonal presets use"`
	StartsAtMs   int64  `json:"startsAtMs,omitempty" doc:"holiday only: the window's start (Unix ms)"`
	EndsAtMs     int64  `json:"endsAtMs,omitempty" doc:"holiday only: the window's end (Unix ms)"`
}

type channelIdeaDTO struct {
	ID         string               `json:"id" example:"genre:comedy" doc:"Stable across calls; the handle for hide and undo"`
	Facet      string               `json:"facet" enum:"genre,decade,holiday"`
	Value      string               `json:"value" doc:"The genre as the library spells it, the decade's first year ('1990'), or the holiday id"`
	Reason     channelIdeaReasonDTO `json:"reason"`
	Keys       []string             `json:"keys" doc:"Title keys, newest first, at most 100. The first four are the posters; artwork comes from /v1/images."`
	Movies     int                  `json:"movies"`
	Series     int                  `json:"series"`
	InLibrary  int                  `json:"inLibrary" doc:"Titles already in the library"`
	ToDownload int                  `json:"toDownload" doc:"Titles the idea would have to request. Library-grounded ideas need none."`
}

type listChannelIdeasOutput struct {
	Body struct {
		Ideas []channelIdeaDTO `json:"ideas"`
	}
}

type channelIdeaInput struct {
	IdeaID string `path:"ideaId" maxLength:"200" example:"genre:comedy"`
}

// ideasUser is the person whose hides a request reads or writes; ideas are personal because
// hides are, so a break-glass API token (no person) gets none.
func (s *Server) ideasUser(ctx context.Context) (string, error) {
	if s.store == nil {
		return "", errFeatureNotConfigured("Channel ideas unavailable", "The persistence service is not configured.")
	}
	userID := userIDFromHuma(ctx)
	if userID == "" {
		return "", errUnauthorized("Not signed in", "Channel ideas belong to a person. Sign in, or use a paired device.")
	}
	return userID, nil
}

func (s *Server) listChannelIdeas(ctx context.Context, _ *struct{}) (*listChannelIdeasOutput, error) {
	userID, err := s.ideasUser(ctx)
	if err != nil {
		return nil, err
	}
	if s.ideaLibrary == nil || s.libraryUnconfigured() {
		return nil, errNotImplemented("Channel ideas aren't available", "Connect your media library in Settings to get channel ideas.")
	}
	items, err := s.ideaLibrary.IdeaItems(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't load channel ideas",
			"The media library didn't answer. Check the connection in Settings and try again.", err)
	}
	hidden, err := s.store.HiddenIdeas(ctx, userID)
	if err != nil {
		return nil, err
	}
	onChannel, err := s.keysOnChannels(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	var holidays []ideas.Holiday
	for _, h := range schedule.UpcomingHolidays(now, ideaHolidayHorizon) {
		holidays = append(holidays, ideas.Holiday{ID: h.ID, Start: h.Start, End: h.End})
	}

	built := ideas.Build(ideas.Input{Library: items, OnChannel: onChannel, Holidays: holidays, Hidden: hidden, Now: now})
	labels := map[string]string{}
	for _, d := range holidayvocab.Definitions() {
		labels[d.ID] = d.Label
	}
	out := &listChannelIdeasOutput{}
	out.Body.Ideas = make([]channelIdeaDTO, 0, len(built))
	for _, idea := range built {
		out.Body.Ideas = append(out.Body.Ideas, channelIdeaToDTO(idea, labels))
	}
	return out, nil
}

// keysOnChannels is every key on a channel that is, or will be, airing. A detached channel's
// lineup no longer plays, so its titles count as unaired again.
func (s *Server) keysOnChannels(ctx context.Context) (map[provision.Key]bool, error) {
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := map[provision.Key]bool{}
	for _, ch := range channels {
		if ch.Status == schedule.StatusDetached {
			continue
		}
		for _, e := range ch.Lineup {
			out[e.Key] = true
		}
	}
	return out, nil
}

func channelIdeaToDTO(idea ideas.Idea, holidayLabels map[string]string) channelIdeaDTO {
	keys := make([]string, 0, len(idea.Keys))
	for _, k := range idea.Keys {
		keys = append(keys, string(k))
	}
	reason := channelIdeaReasonDTO{Kind: string(idea.Reason.Kind), Count: idea.Reason.Count}
	if idea.Reason.Kind == ideas.ReasonHoliday {
		reason.HolidayID, reason.HolidayLabel = idea.Reason.HolidayID, holidayLabels[idea.Reason.HolidayID]
		reason.StartsAtMs, reason.EndsAtMs = idea.Reason.StartsAt.UnixMilli(), idea.Reason.EndsAt.UnixMilli()
	}
	return channelIdeaDTO{
		ID: idea.ID, Facet: string(idea.Facet), Value: idea.Value, Reason: reason, Keys: keys,
		Movies: idea.Movies, Series: idea.Series, InLibrary: idea.Movies + idea.Series,
	}
}

func (s *Server) hideChannelIdea(ctx context.Context, in *channelIdeaInput) (*struct{}, error) {
	userID, err := s.ideasUser(ctx)
	if err != nil {
		return nil, err
	}
	return nil, s.store.HideIdea(ctx, userID, in.IdeaID, s.clock().UTC())
}

func (s *Server) unhideChannelIdea(ctx context.Context, in *channelIdeaInput) (*struct{}, error) {
	userID, err := s.ideasUser(ctx)
	if err != nil {
		return nil, err
	}
	return nil, s.store.UnhideIdea(ctx, userID, in.IdeaID)
}
