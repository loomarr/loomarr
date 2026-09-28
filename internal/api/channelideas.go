package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
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
		OperationID: "request-channel-idea", Method: http.MethodPost, Path: "/v1/discovery/ideas/{ideaId}/request",
		Summary: "Request a channel from an idea",
		Description: "Puts the idea in the approval queue as the caller's own channel request (#1720): the idea's name, " +
			"its pitch and its library titles become the proposal an admin approves. No LLM is involved, so it works with " +
			"the AI off. Members only (maintainer H4); an admin makes channels directly. While the caller's request for " +
			"this idea waits for approval, asking again returns that request rather than a second one.",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"discovery"},
	}, RoleMember), s.requestChannelIdea)
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
	ID    string `json:"id" example:"genre:comedy" doc:"Stable across calls; the handle for hide, undo and request"`
	Name  string `json:"name" example:"Comedy Movies" doc:"The card's name, built from the facet and the seasonal calendar without the LLM"`
	Pitch string `json:"pitch" example:"Every comedy title in your library that no channel plays yet, on one channel." doc:"One sentence saying what the channel would be, built without the LLM; the counts are separate fields"`
	// Requested and RequestJobID say the caller's request for this idea is waiting for an admin.
	Requested    bool                 `json:"requested" doc:"The caller has requested this idea and it is waiting for approval"`
	RequestJobID string               `json:"requestJobId,omitempty" doc:"The waiting request's job, for its journey (/v1/proposal-jobs/{jobId})"`
	Facet        string               `json:"facet" enum:"genre,decade,holiday"`
	Value        string               `json:"value" doc:"The genre as the library spells it, the decade's first year ('1990'), or the holiday id"`
	Reason       channelIdeaReasonDTO `json:"reason"`
	Keys         []string             `json:"keys" doc:"Title keys, newest first, at most 100. The first four are the posters; artwork comes from /v1/images."`
	Movies       int                  `json:"movies"`
	Series       int                  `json:"series"`
	InLibrary    int                  `json:"inLibrary" doc:"Titles already in the library"`
	ToDownload   int                  `json:"toDownload" doc:"Titles the idea would have to request. Library-grounded ideas need none."`
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
	built, labels, err := s.buildChannelIdeas(ctx, userID)
	if err != nil {
		return nil, err
	}
	requested, err := s.requestedIdeas(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &listChannelIdeasOutput{}
	out.Body.Ideas = make([]channelIdeaDTO, 0, len(built))
	for _, idea := range built {
		dto := channelIdeaToDTO(idea, labels)
		dto.RequestJobID = requested[idea.ID]
		dto.Requested = dto.RequestJobID != ""
		out.Body.Ideas = append(out.Body.Ideas, dto)
	}
	return out, nil
}

// buildChannelIdeas builds the caller's ideas (their hides left out) and returns the seasonal
// calendar's display labels by holiday id, which names and pitches read.
func (s *Server) buildChannelIdeas(ctx context.Context, userID string) ([]ideas.Idea, map[string]string, error) {
	if s.ideaLibrary == nil || s.libraryUnconfigured() {
		return nil, nil, errNotImplemented("Channel ideas aren't available", "Connect your media library in Settings to get channel ideas.")
	}
	items, err := s.ideaLibrary.IdeaItems(ctx)
	if err != nil {
		return nil, nil, apiErrWithCause(http.StatusBadGateway, "Couldn't load channel ideas",
			"The media library didn't answer. Check the connection in Settings and try again.", err)
	}
	hidden, err := s.store.HiddenIdeas(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	onChannel, err := s.keysOnChannels(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	var holidays []ideas.Holiday
	for _, h := range schedule.UpcomingHolidays(now, ideaHolidayHorizon) {
		holidays = append(holidays, ideas.Holiday{ID: h.ID, Start: h.Start, End: h.End})
	}
	labels := map[string]string{}
	for _, d := range holidayvocab.Definitions() {
		labels[d.ID] = d.Label
	}
	built := ideas.Build(ideas.Input{Library: items, OnChannel: onChannel, Holidays: holidays, Hidden: hidden, Now: now})
	return built, labels, nil
}

// requestedIdeas maps each idea the caller has requested, and that still waits for approval, to
// that request's job. Once an admin decides it, the idea is requestable again: approved, its
// titles are on a channel; denied, the person may ask again.
func (s *Server) requestedIdeas(ctx context.Context, userID string) (map[string]string, error) {
	waiting, err := s.store.ListProposalsByStatus(ctx, "submitted")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range waiting {
		if p.CreatedBy != userID {
			continue
		}
		var provenance struct {
			FromIdea string `json:"fromIdea"`
		}
		if json.Unmarshal([]byte(p.ProposalJSON), &provenance) == nil && provenance.FromIdea != "" {
			out[provenance.FromIdea] = p.JobID
		}
	}
	return out, nil
}

type requestChannelIdeaOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"The request's job; its journey is /v1/proposal-jobs/{jobId}"`
	}
}

func (s *Server) requestChannelIdea(ctx context.Context, in *channelIdeaInput) (*requestChannelIdeaOutput, error) {
	userID, err := s.ideasUser(ctx)
	if err != nil {
		return nil, err
	}
	if u, ok := userFrom(ctx); ok && u.Role == store.RoleAdmin {
		return nil, apiErr(http.StatusForbidden, "Channel ideas are for members",
			"Requests go to an admin for approval. As an admin, make the channel yourself from the guide.")
	}
	if s.suggest == nil {
		return nil, errFeatureNotConfigured("Channel requests unavailable", "The request queue isn't running on this server.")
	}
	out := &requestChannelIdeaOutput{}
	requested, err := s.requestedIdeas(ctx, userID)
	if err != nil {
		return nil, err
	}
	if jobID := requested[in.IdeaID]; jobID != "" {
		out.Body.JobID = jobID
		return out, nil
	}
	built, labels, err := s.buildChannelIdeas(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, idea := range built {
		if idea.ID != in.IdeaID {
			continue
		}
		name, pitch := ideas.Describe(idea, labels[idea.Reason.HolidayID])
		proposal := suggest.Proposal{
			ChannelName: name, FromIdea: idea.ID, Lineup: ideaLineup(idea.Titles), Rationale: pitch,
			// Every title is already in the library: nothing to download.
			Scores: suggest.Scores{AvailabilityRatio: 1},
		}
		// The name is the request's brief, as a typed brief is for a member's own request: Your
		// requests and the approval queue title the request with it (#1659 web mock).
		jobID, err := s.suggest.SubmitBuilt(ctx, suggest.Intent{Description: name}, proposal, userID)
		if err != nil {
			return nil, err
		}
		out.Body.JobID = jobID
		return out, nil
	}
	return nil, errNotFound("Idea not available", "That idea isn't available any more. Refresh Home for today's ideas.")
}

// ideaLineup turns an idea's library titles into proposal items, in the idea's order (newest
// first). Each carries the id its key names, which is what grounds it for the approval gate.
func ideaLineup(titles []ideas.Item) []suggest.ProposalItem {
	out := make([]suggest.ProposalItem, 0, len(titles))
	for _, it := range titles {
		if len(it.Keys) == 0 {
			continue
		}
		mt, provider, id, ok := provision.ParseKey(it.Keys[0])
		if !ok {
			continue
		}
		item := suggest.ProposalItem{MediaType: mt, Name: it.Name, Year: it.Year, InLibrary: true, Genres: it.Genres}
		if provider == "tvdb" {
			item.TVDBID = id
		} else {
			item.TMDBID = id
		}
		out = append(out, item)
	}
	return out
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
	name, pitch := ideas.Describe(idea, holidayLabels[idea.Reason.HolidayID])
	return channelIdeaDTO{
		ID: idea.ID, Name: name, Pitch: pitch, Facet: string(idea.Facet), Value: idea.Value, Reason: reason, Keys: keys,
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
