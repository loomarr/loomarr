package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/loomarr/loomarr/internal/channeledit"
	"github.com/loomarr/loomarr/internal/store"
)

// channelEdits is the one write path for operator channel changes (create, PATCH, icon and
// watermark uploads, detach), built from the server's own collaborators so every route shares the
// edit rules channeledit owns.
func (s *Server) channelEdits() *channeledit.Service {
	svc := &channeledit.Service{Store: s.store, Log: s.log, Activity: s.activity, Lifecycle: s.applyInternalPlayoutLifecycle}
	// Interface fields stay nil when unwired, so the service's nil checks mean "absent".
	if s.binder != nil {
		svc.Binder = s.binder
	}
	if s.channels != nil {
		svc.Reconciler = s.channels
	}
	return svc
}

func (c BackendCheckpoint) editBackend() channeledit.Backend {
	return channeledit.Backend{Applied: c.Applied, PublishedInternal: c.PublishedInternal}
}

// channelEditProblem turns a channeledit refusal into the problem the operator sees. An error the
// edit itself returned (already a problem) or an unexpected one passes through.
func channelEditProblem(err error) error {
	switch {
	case errors.Is(err, channeledit.ErrNotFound):
		return errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	case errors.Is(err, channeledit.ErrStale):
		return staleChannelConflict()
	case errors.Is(err, channeledit.ErrNumberInUse):
		return errConflict("Channel number in use", "Another channel already uses that number. Pick a different one.")
	case errors.Is(err, channeledit.ErrIDTaken):
		return errConflict("Channel already exists", "A channel with that id already exists.")
	case errors.Is(err, channeledit.ErrIntentBound):
		return errConflict("Channel already exists", "That approved suggestion is already bound to a channel.")
	case errors.Is(err, channeledit.ErrClaimed):
		return errConflict("Channel already exists", "That channel number or approved suggestion was claimed by another channel. Refresh and try again.")
	case errors.Is(err, channeledit.ErrInvalidSeries):
		return errUnprocessable("Invalid series", "For a single-series channel, pick a series (not a movie or episode).")
	case errors.Is(err, channeledit.ErrLineupUnresolved):
		return apiErrWithCause(http.StatusUnprocessableEntity, "Couldn't build the lineup",
			"Loomarr couldn't resolve the lineup for the approved suggestion. Try refining or re-approving it.", err)
	case errors.Is(err, channeledit.ErrPolicyUnresolved):
		return apiErrWithCause(http.StatusUnprocessableEntity, "Couldn't apply the policy",
			"Loomarr couldn't resolve the programming policy for the approved suggestion. Try refining or re-approving it.", err)
	case errors.Is(err, channeledit.ErrInvalid):
		return apiErrWithCause(http.StatusUnprocessableEntity, "Invalid channel",
			"Some channel details are invalid. Check the name, number, and strategy, then try again.", err)
	}
	return err
}

// linkUploadToChannel records an already-stored upload (`what`: "icon", "image") on the channel.
// The bytes are saved by then, so a failure says so rather than reading as a lost upload.
func (s *Server) linkUploadToChannel(ctx context.Context, id, what string, change func(*store.Channel) error) error {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return err
	}
	_, err = s.channelEdits().Edit(ctx, checkpoint.editBackend(), id, 0, change)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, channeledit.ErrNotFound):
		return channelEditProblem(err)
	}
	return apiErrWithCause(http.StatusInternalServerError, "Couldn't update the channel",
		"The "+what+" was saved but linking it to the channel failed. Try again.", err)
}

// staleChannelConflict is deliberately distinct from number/intent collisions: this edit was
// valid when authored, but the channel changed before it could commit. The only honest recovery
// is to refresh and let the operator decide again, especially for whole-list lineup replacement.
func staleChannelConflict() error {
	return errConflict("Channel changed", "This channel changed after you opened it. Refresh it, review the latest version, and try again.")
}
