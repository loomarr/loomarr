package channels

import (
	"context"
	"errors"
	"net/http"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/location"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// channelEdits is the one write path for operator channel changes (create, PATCH, icon and
// watermark uploads, detach), built from the server's own collaborators so every route shares the
// edit rules channeledit owns.
func (s *Handlers) channelEdits() *edit.Service {
	svc := &edit.Service{Store: s.store, Log: s.log, Activity: s.activity, Lifecycle: s.applyInternalPlayoutLifecycle}
	// Interface fields stay nil when unwired, so the service's nil checks mean "absent".
	if s.binder != nil {
		svc.Binder = s.binder
	}
	if s.channels != nil {
		svc.Reconciler = s.channels
	}
	return svc
}

func editBackend(c httpcore.BackendCheckpoint) edit.Backend {
	return edit.Backend{Applied: c.Applied, PublishedInternal: c.PublishedInternal}
}

// channelEditProblem turns a channeledit refusal into the problem the operator sees. An error the
// edit itself returned (already a problem) or an unexpected one passes through.
func channelEditProblem(err error) error {
	switch {
	case errors.Is(err, edit.ErrNotFound):
		return httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	case errors.Is(err, edit.ErrStale):
		return staleChannelConflict()
	case errors.Is(err, edit.ErrNumberInUse):
		return httpcore.Conflict("Channel number in use", "Another channel already uses that number. Pick a different one.")
	case errors.Is(err, edit.ErrIDTaken):
		return httpcore.Conflict("Channel already exists", "A channel with that id already exists.")
	case errors.Is(err, edit.ErrClaimed):
		return httpcore.Conflict("Channel already exists", "That channel number was claimed by another channel. Refresh and try again.")
	case errors.Is(err, edit.ErrInvalidSeries):
		return httpcore.Unprocessable("Invalid series", "For a single-series channel, pick a series (not a movie or episode).")
	case errors.Is(err, edit.ErrInvalid):
		return httpcore.ProblemWithCause(http.StatusUnprocessableEntity, "Invalid channel",
			"Some channel details are invalid. Check the name, number, and strategy, then try again.", err)
	}
	return err
}

// linkUploadToChannel records an already-stored upload (`what`: "icon", "image") on the channel.
// The bytes are saved by then, so a failure says so rather than reading as a lost upload.
func (s *Handlers) linkUploadToChannel(ctx context.Context, id, what string, change func(*channelstore.Channel) error) error {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return err
	}
	_, err = s.channelEdits().Edit(ctx, editBackend(checkpoint), id, 0, change)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, edit.ErrNotFound):
		return channelEditProblem(err)
	}
	return httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't update the channel",
		"The "+what+" was saved but linking it to the channel failed. Try again.", err)
}

// validateChannelPlace rejects a channel place the location index does not hold as a city in the
// channel's own country. The schedule package cannot read the index, so the write path checks it.
func validateChannelPlace(policy schedule.ChannelPolicy) error {
	if policy.Filler == nil || policy.Filler.Geography == nil {
		return nil
	}
	geo := pod.Geography{Country: policy.Filler.Geography.Country, Place: policy.Filler.Geography.Place}.Normalize()
	if geo.Place == "" {
		return nil
	}
	if geo.Country == "" {
		return httpcore.Unprocessable("Invalid location", "Choose the channel's country along with its city.")
	}
	index, err := location.Default()
	if err != nil {
		return httpcore.ServiceUnavailable("Location check unavailable", "The built-in location index couldn't be opened. Try again.")
	}
	if place, ok := index.Lookup(geo.Place); !ok || place.Country != geo.Country {
		return httpcore.Unprocessable("Invalid location", "That city isn't one Loomarr knows in "+geo.Country+". Search for it again.")
	}
	return nil
}

// staleChannelConflict is deliberately distinct from number/intent collisions: this edit was
// valid when authored, but the channel changed before it could commit. The only honest recovery
// is to refresh and let the operator decide again, especially for whole-list lineup replacement.
func staleChannelConflict() error {
	return httpcore.Conflict("Channel changed", "This channel changed after you opened it. Refresh it, review the latest version, and try again.")
}
