package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/store"
)

// Per-person Favourites and Recent channel lists (#1666): the guide's filters, the surf rail's
// groups and the watch console's star. They're keyed by the signed-in person, and a paired TV
// resolves to the person who paired it (sessionauth.go), so a channel starred on the TV shows on
// that person's phone (#1659 N3). Server-side rather than client-local so the lists follow the
// person across devices; the counts on the filter labels are the list lengths.
//
// Recents are recorded by an explicit call, not as a side effect of minting a play URL, because
// the channel warmer mints URLs for the neighbouring channels too: every surf would otherwise
// "visit" the channels either side.

func (s *Server) registerMyChannels(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "my-channels", Method: http.MethodGet, Path: "/v1/me/channels",
		Summary:     "Your favourite and recent channels",
		Description: "Any signed-in person, or a TV they paired. Both of the caller's channel lists in one read: favourites in the order they were starred, and up to 20 recents, newest first. The filter counts are the list lengths.",
		Tags:        []string{"channels"},
	}, RoleMember), s.myChannels)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "add-favourite-channel", Method: http.MethodPut, Path: "/v1/me/favourites/{channelId}",
		Summary:     "Star a channel",
		Description: "Adds the channel to the caller's favourites. Idempotent. Returns both lists.",
		Tags:        []string{"channels"},
	}, RoleMember), s.addFavouriteChannel)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "remove-favourite-channel", Method: http.MethodDelete, Path: "/v1/me/favourites/{channelId}",
		Summary:     "Unstar a channel",
		Description: "Removes the channel from the caller's favourites; removing one that isn't starred is not an error. Returns both lists.",
		Tags:        []string{"channels"},
	}, RoleMember), s.removeFavouriteChannel)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "record-channel-tune", Method: http.MethodPut, Path: "/v1/me/recent-channels/{channelId}",
		Summary: "Record that you tuned a channel",
		Description: "Moves the channel to the front of the caller's recents. Call it once a tune settles (the first decoded frame), " +
			"never when warming or prefetching a neighbour. Returns both lists.",
		Tags: []string{"channels"},
	}, RoleMember), s.recordChannelTune)
}

type myChannelInput struct {
	ChannelID string `path:"channelId" example:"ch_abc123"`
}

type favouriteChannelDTO struct {
	ChannelID string    `json:"channelId"`
	AddedAt   time.Time `json:"addedAt" doc:"When the person starred it"`
}

type recentChannelDTO struct {
	ChannelID string    `json:"channelId"`
	TunedAt   time.Time `json:"tunedAt" doc:"The person's latest tune of this channel, from any of their devices"`
}

type myChannelsOutput struct {
	Body struct {
		Favourites []favouriteChannelDTO `json:"favourites" doc:"Starred channels, oldest first"`
		Recent     []recentChannelDTO    `json:"recent" doc:"Recently tuned channels, newest first, at most 20"`
	}
}

// channelListsUser is the person whose lists a request reads or writes. A break-glass API_TOKEN
// caller has no user, so it has no lists.
func (s *Server) channelListsUser(ctx context.Context) (string, error) {
	if s.store == nil {
		return "", errFeatureNotConfigured("Channel lists unavailable", "The persistence service is not configured.")
	}
	userID := userIDFromHuma(ctx)
	if userID == "" {
		return "", errUnauthorized("Not signed in", "Favourite and recent channels belong to a person. Sign in, or use a paired device.")
	}
	return userID, nil
}

func (s *Server) myChannels(ctx context.Context, _ *struct{}) (*myChannelsOutput, error) {
	userID, err := s.channelListsUser(ctx)
	if err != nil {
		return nil, err
	}
	return s.readMyChannels(ctx, userID)
}

func (s *Server) addFavouriteChannel(ctx context.Context, in *myChannelInput) (*myChannelsOutput, error) {
	return s.writeMyChannels(ctx, func(userID string) error {
		return s.store.AddFavouriteChannel(ctx, userID, in.ChannelID, time.Now().UTC())
	})
}

func (s *Server) removeFavouriteChannel(ctx context.Context, in *myChannelInput) (*myChannelsOutput, error) {
	return s.writeMyChannels(ctx, func(userID string) error {
		return s.store.RemoveFavouriteChannel(ctx, userID, in.ChannelID)
	})
}

func (s *Server) recordChannelTune(ctx context.Context, in *myChannelInput) (*myChannelsOutput, error) {
	return s.writeMyChannels(ctx, func(userID string) error {
		return s.store.RecordChannelTune(ctx, userID, in.ChannelID, time.Now().UTC())
	})
}

func (s *Server) writeMyChannels(ctx context.Context, write func(userID string) error) (*myChannelsOutput, error) {
	userID, err := s.channelListsUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := write(userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
		}
		return nil, err
	}
	return s.readMyChannels(ctx, userID)
}

func (s *Server) readMyChannels(ctx context.Context, userID string) (*myChannelsOutput, error) {
	lists, err := s.store.UserChannelLists(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &myChannelsOutput{}
	out.Body.Favourites = make([]favouriteChannelDTO, 0, len(lists.Favourites))
	for _, f := range lists.Favourites {
		out.Body.Favourites = append(out.Body.Favourites, favouriteChannelDTO{ChannelID: f.ChannelID, AddedAt: f.AddedAt})
	}
	out.Body.Recent = make([]recentChannelDTO, 0, len(lists.Recent))
	for _, r := range lists.Recent {
		out.Body.Recent = append(out.Body.Recent, recentChannelDTO{ChannelID: r.ChannelID, TunedAt: r.TunedAt})
	}
	return out, nil
}
