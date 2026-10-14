package api

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/viewing"
)

// GET /v1/household/viewing — who is watching what, on which device, for Home (#1662).
//
// Visibility is the maintainer's rule (#1659 H2), enforced here rather than by hiding it in a UI:
// an admin sees every viewing by name; a member sees the counts, plus their own viewings and their
// own continue-watching. Knowing that the household's channels are being watched is catalogue-like;
// knowing WHO watches WHAT is viewing history, which is not the catalogue (§342 covers titles and
// channels only).
//
// The source is the in-app players' own playlist polls (internal/viewing), attributed through the
// viewer tag each play URL carries (viewingtag.go). Viewing through the media server's Live TV
// (Tunarr, or the playout token) carries no person, so it isn't counted here.

type viewingScope string

const (
	// viewingScopeHousehold: viewers names everyone watching (admin).
	viewingScopeHousehold viewingScope = "household"
	// viewingScopeSelf: viewers holds only the caller's own devices (member).
	viewingScopeSelf viewingScope = "self"
)

func (s *Server) registerHouseholdViewing(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "household-viewing", Method: http.MethodGet, Path: "/v1/household/viewing",
		Summary: "Who is watching what right now",
		Description: "Any signed-in person. Counts of devices watching each channel for everyone; the named viewings " +
			"(person, device, channel, since) only for an admin (`scope: household`). A member gets their own viewings " +
			"only (`scope: self`). Both get their own continue-watching channel. Counts only in-app playback; " +
			"watching through the media server's Live TV isn't attributed to a person.",
		Tags: []string{"dashboard"},
	}, RoleMember), s.householdViewing)
}

type channelViewersDTO struct {
	ChannelID string `json:"channelId"`
	Viewers   int    `json:"viewers" doc:"Devices watching this channel now"`
}

type viewerDTO struct {
	UserID    string    `json:"userId"`
	Name      string    `json:"name" doc:"The person's display name"`
	Device    string    `json:"device" doc:"What the household calls the device: a paired device's name, or the browser and system"`
	ChannelID string    `json:"channelId"`
	Since     time.Time `json:"since" doc:"When this device started watching this channel"`
	You       bool      `json:"you" doc:"Whether this is the caller's own viewing"`
}

type continueWatchingDTO struct {
	ChannelID string    `json:"channelId"`
	TunedAt   time.Time `json:"tunedAt" doc:"The caller's latest settled tune, from any of their devices"`
}

type householdViewingOutput struct {
	Body struct {
		Scope    viewingScope        `json:"scope" enum:"household,self" doc:"household: viewers names everyone watching (admin). self: viewers holds only the caller's own devices (member)"`
		Watching int                 `json:"watching" doc:"Devices watching anything right now, across the household"`
		Channels []channelViewersDTO `json:"channels" doc:"Devices watching each channel, busiest first"`
		Viewers  []viewerDTO         `json:"viewers" doc:"Named viewings, oldest first, limited by scope"`
		// ContinueWatching is absent for a caller with no person (API token) or no tune yet.
		ContinueWatching *continueWatchingDTO `json:"continueWatching,omitempty" doc:"The caller's last-tuned channel, for Home's continue-watching and Watch"`
	}
}

func (s *Server) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Server) householdViewing(ctx context.Context, _ *struct{}) (*householdViewingOutput, error) {
	out := &householdViewingOutput{}
	var watches []viewing.Watch
	if s.viewing != nil {
		watches = s.viewing.Watching(s.clock())
	}
	me, hasMe := userFrom(ctx)
	admin := roleFrom(ctx) == RoleAdmin
	out.Body.Scope = viewingScopeSelf
	if admin {
		out.Body.Scope = viewingScopeHousehold
	}

	perChannel := map[string]int{}
	for _, w := range watches {
		perChannel[w.ChannelID]++
	}
	out.Body.Watching = len(watches)
	out.Body.Channels = make([]channelViewersDTO, 0, len(perChannel))
	for id, n := range perChannel {
		out.Body.Channels = append(out.Body.Channels, channelViewersDTO{ChannelID: id, Viewers: n})
	}
	slices.SortFunc(out.Body.Channels, func(a, b channelViewersDTO) int {
		if a.Viewers != b.Viewers {
			return b.Viewers - a.Viewers
		}
		if a.ChannelID < b.ChannelID {
			return -1
		}
		return 1
	})

	names := map[string]string{}
	if hasMe {
		names[me.ID] = me.Name
	}
	if admin && len(watches) > 0 && s.store != nil {
		// One read for every name, not one per viewer.
		users, err := s.store.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			names[u.ID] = u.Name
		}
	}
	out.Body.Viewers = make([]viewerDTO, 0, len(watches))
	for _, w := range watches {
		mine := hasMe && w.UserID == me.ID
		if !admin && !mine {
			continue
		}
		out.Body.Viewers = append(out.Body.Viewers, viewerDTO{UserID: w.UserID, Name: names[w.UserID],
			Device: w.DeviceLabel, ChannelID: w.ChannelID, Since: w.Since.UTC(), You: mine})
	}

	if hasMe && s.store != nil {
		lists, err := s.store.UserChannelLists(ctx, me.ID)
		if err != nil {
			return nil, err
		}
		if len(lists.Recent) > 0 {
			out.Body.ContinueWatching = &continueWatchingDTO{ChannelID: lists.Recent[0].ChannelID, TunedAt: lists.Recent[0].TunedAt}
		}
	}
	return out, nil
}
