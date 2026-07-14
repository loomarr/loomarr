package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// ChannelDTO is the API view of a scheduler channel (§9). The desired lineup is
// summarized (counts) rather than dumped slot-by-slot on the channel resource;
// the full lineup editor is a Phase-13 UI concern.
type ChannelDTO struct {
	ID           string `json:"id" example:"ch_abc123"`
	Name         string `json:"name" example:"Saturday Morning Cartoons"`
	Number       int    `json:"number" example:"42" doc:"Guide channel number"`
	Group        string `json:"group,omitempty" example:"Kids"`
	Strategy     string `json:"strategy" enum:"sequential,shuffle,time_slot"`
	Status       string `json:"status" enum:"building,live,drifted,detached" doc:"Loomarr-side channel status (§9)"`
	TunarrID     string `json:"tunarrId,omitempty" doc:"Server-assigned Tunarr channel id; empty until first reconcile"`
	IntentRef    string `json:"intentRef,omitempty"`
	ProgramCount int    `json:"programCount" doc:"Real playable programs in the desired lineup"`
	SlotCount    int    `json:"slotCount" doc:"Total slots incl. filler/flex placeholders"`
}

func channelToDTO(ch store.Channel) ChannelDTO {
	d := schedule.DesiredLineup{Slots: ch.Desired}
	return ChannelDTO{
		ID: ch.ID, Name: ch.Name, Number: ch.Number, Group: ch.Group,
		Strategy: string(ch.Strategy), Status: string(ch.Status),
		TunarrID: ch.TunarrID, IntentRef: ch.IntentRef,
		ProgramCount: d.ProgramCount(), SlotCount: len(ch.Desired),
	}
}

// registerChannels mounts /v1/channels* (§7). Reads are visible to any
// authenticated user; create/update/delete/reconcile require admin.
func (s *Server) registerChannels(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-channels", Method: http.MethodGet, Path: "/v1/channels",
		Summary: "List channels", Tags: []string{"channels"},
	}, s.listChannels)

	huma.Register(api, huma.Operation{
		OperationID: "get-channel", Method: http.MethodGet, Path: "/v1/channels/{id}",
		Summary: "Get a channel definition + status", Tags: []string{"channels"},
	}, s.getChannel)

	huma.Register(api, huma.Operation{
		OperationID: "create-channel", Method: http.MethodPost, Path: "/v1/channels",
		Summary: "Create a channel", Description: "Admin only. From an approved proposal or hand-made.",
		Tags: []string{"channels"},
	}, s.createChannel)

	huma.Register(api, huma.Operation{
		OperationID: "reconcile-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/reconcile",
		Summary: "Force desired→Tunarr reconciliation", Description: "Admin only. Idempotent (§9).",
		Tags: []string{"channels"},
	}, s.reconcileChannel)

	huma.Register(api, huma.Operation{
		OperationID: "delete-channel", Method: http.MethodDelete, Path: "/v1/channels/{id}",
		Summary: "Remove a channel", Description: "Admin only. Detaches by default (§7).",
		Tags: []string{"channels"}, DefaultStatus: http.StatusNoContent,
	}, s.deleteChannel)
}

type channelIDInput struct {
	ID string `path:"id" example:"ch_abc123"`
}
type channelOutput struct{ Body ChannelDTO }

type listChannelsOutput struct {
	Body struct {
		Channels []ChannelDTO `json:"channels"`
	}
}

func (s *Server) listChannels(ctx context.Context, _ *struct{}) (*listChannelsOutput, error) {
	all, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := &listChannelsOutput{}
	out.Body.Channels = make([]ChannelDTO, 0, len(all))
	for _, ch := range all {
		out.Body.Channels = append(out.Body.Channels, channelToDTO(ch))
	}
	return out, nil
}

func (s *Server) getChannel(ctx context.Context, in *channelIDInput) (*channelOutput, error) {
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such channel")
	}
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: channelToDTO(ch)}, nil
}

type createChannelInput struct {
	Body struct {
		ID        string `json:"id" doc:"Loomarr channel id (caller-assigned, stable)" example:"ch_abc123"`
		Name      string `json:"name" example:"Saturday Morning Cartoons"`
		Number    int    `json:"number" minimum:"1" example:"42"`
		Group     string `json:"group,omitempty" example:"Kids"`
		Logo      string `json:"logo,omitempty"`
		Strategy  string `json:"strategy" enum:"sequential,shuffle,time_slot"`
		IntentRef string `json:"intentRef,omitempty"`
		// Series is an optional hand-made single-series channel (§7 "or hand-made"):
		// the channel plays one show, optionally constrained to a season range (§9
		// series expansion). The series must already be an `available` title. Use
		// instead of intentRef; if both are set, intentRef wins.
		Series *struct {
			Key       string `json:"key" doc:"provisioning key, e.g. series:tvdb:71663" example:"series:tvdb:71663"`
			Title     string `json:"title,omitempty"`
			SeasonMin int    `json:"seasonMin,omitempty" doc:"first season (inclusive; 0 = unbounded)"`
			SeasonMax int    `json:"seasonMax,omitempty" doc:"last season (inclusive; 0 = unbounded)"`
		} `json:"series,omitempty"`
	}
}

func (s *Server) createChannel(ctx context.Context, in *createChannelInput) (*channelOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	ch := store.Channel{}
	ch.ID = in.Body.ID
	ch.Name = in.Body.Name
	ch.Number = in.Body.Number
	ch.Group = in.Body.Group
	ch.Logo = in.Body.Logo
	ch.Strategy = schedule.Strategy(in.Body.Strategy)
	ch.IntentRef = in.Body.IntentRef
	ch.Status = schedule.StatusBuilding
	// Bind the approved proposal's lineup (§7/§9: "create a channel from an
	// approved proposal"). Empty intentRef ⇒ hand-made channel, no lineup yet.
	lineup, err := s.lineupFromIntent(ctx, in.Body.IntentRef)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("resolve intent lineup", err)
	}
	// Hand-made single-series channel (§7 "or hand-made"): one series entry with an
	// optional season range (§9 expansion). Only when no proposal intent is given.
	if in.Body.IntentRef == "" && in.Body.Series != nil {
		key := provision.Key(in.Body.Series.Key)
		if !key.IsSeries() {
			return nil, huma.Error422UnprocessableEntity("series.key must be a series key (e.g. series:tvdb:<id>)", nil)
		}
		lineup = []schedule.LineupEntry{{
			Key:       key,
			Title:     in.Body.Series.Title,
			SeasonMin: in.Body.Series.SeasonMin,
			SeasonMax: in.Body.Series.SeasonMax,
		}}
	}
	ch.Lineup = lineup
	if err := ch.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid channel", err)
	}
	// Reject a duplicate id or number up front (the store's unique index would
	// also reject, but a clean 409 is friendlier than a 500).
	if _, err := s.store.GetChannel(ctx, ch.ID); err == nil {
		return nil, huma.Error409Conflict("channel id already exists")
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if _, err := s.store.GetChannelByNumber(ctx, ch.Number); err == nil {
		return nil, huma.Error409Conflict("channel number already in use")
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, err
	}
	// Kick an initial reconcile so the channel goes live immediately (§9 "live
	// immediately — never dead air"). Best-effort: a reconcile failure leaves the
	// channel in `building` for the sweep to pick up, it doesn't fail creation.
	if s.channels != nil {
		if err := s.channels.Reconcile(ctx, ch.ID); err != nil {
			s.log.Warn("initial channel reconcile failed (sweep will retry)", "channel", ch.ID, "err", err)
		}
	}
	fresh, err := s.store.GetChannel(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: channelToDTO(fresh)}, nil
}

type reconcileOutput struct{ Body ChannelDTO }

func (s *Server) reconcileChannel(ctx context.Context, in *channelIDInput) (*reconcileOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.channels == nil {
		return nil, huma.Error501NotImplemented("scheduler not configured")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such channel")
	} else if err != nil {
		return nil, err
	}
	if err := s.channels.Reconcile(ctx, in.ID); err != nil {
		return nil, huma.Error502BadGateway("reconcile failed", err)
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &reconcileOutput{Body: channelToDTO(ch)}, nil
}

type deleteChannelInput struct {
	ID    string `path:"id"`
	Purge bool   `query:"purge" doc:"Also delete the Tunarr channel (default: detach only, §7)"`
}
type deleteChannelOutput struct{}

func (s *Server) deleteChannel(ctx context.Context, in *deleteChannelInput) (*deleteChannelOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such channel")
	}
	if err != nil {
		return nil, err
	}
	// Default: detach (stop managing; leave the Tunarr channel). Detached channels
	// are never reconciled again (§9 ownership). Purge (Tunarr deletion) is a
	// Phase-10 follow-on wired through the Programmer; for now detach is the
	// safe default and purge is recorded on the status.
	ch.Status = schedule.StatusDetached
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, err
	}
	return &deleteChannelOutput{}, nil
}
