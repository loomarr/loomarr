package channels

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/provision"
)

// registerChannels mounts /v1/channels* (§7). Reads are visible to any
// authenticated user; create/update/delete/reconcile require admin.
func (s *Handlers) registerChannels(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-channels", Method: http.MethodGet, Path: "/v1/channels",
		Summary: "List channels", Tags: []string{"channels"},
	}, httpcore.RoleMember), s.listChannels)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-channel", Method: http.MethodGet, Path: "/v1/channels/{id}",
		Summary: "Get a channel definition + status", Tags: []string{"channels"},
	}, httpcore.RoleMember), s.getChannel)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-icon-suggestions", Method: http.MethodGet, Path: "/v1/channels/{id}/icon-suggestions",
		Summary:     "Suggest channel icons from the lineup",
		Description: "Candidate poster images drawn from the channel's OWN lineup titles (§icon) — a Star Trek channel offers its five series' posters. Read-only, so any authenticated user may call it. 501 when TMDB isn't configured.",
		Tags:        []string{"channels"},
	}, httpcore.RoleMember), s.channelIconSuggestions)

	// ⚠ **There is no `channel-icon` serve route (GET /v1/channels/{id}/icon)**, and no
	// `channel_icons` table behind one (retired-ok). A channel's logo is a /v1/images URL, which Tunarr and
	// the media server fetch unauthenticated — the public-with-no-credential property lives on
	// the image serve route, where §22 makes visibility a property of the IMAGE rather than of
	// the route.
	//
	// Only the upload half is here.

	// The upload — admin, multipart. MaxBodyBytes fences a hostile upload at the framework
	// edge, before the handler's own read limit; the handler still checks, because the multipart
	// header's declared size is a claim, not a measurement.
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "upload-channel-icon", Method: http.MethodPost, Path: "/v1/channels/{id}/icon",
		Summary: "Upload a channel icon",
		Description: "Admin only. A raster image under 2 MB in the `file` field (PNG, JPEG, WebP, GIF — " +
			"SVG is refused because the serve half is public and an SVG can carry script). Ingests the " +
			"bytes into the image service (§22), points the channel's logo at the content-addressed URL, " +
			"and reconciles so Tunarr picks it up. Re-uploading the same image yields the same URL; a " +
			"different image yields a different one, so no cache-bust is needed.",
		Tags:         []string{"channels"},
		MaxBodyBytes: maxIconBytes + 1024,
	}, httpcore.RoleAdmin), s.uploadChannelIcon)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "upload-channel-watermark", Method: http.MethodPost, Path: "/v1/channels/{id}/watermark",
		Summary: "Upload a custom channel watermark",
		Description: "Admin only. A PNG or WebP with transparency, under 2 MB, in the `file` field. The image's " +
			"alpha is the bug's shape. Stores it in the image service (role watermark, original kept) and sets " +
			"the channel's policy.watermark.image, which wins over the automatic image. Clear it by saving the " +
			"policy without watermark.image.",
		Tags:         []string{"channels"},
		MaxBodyBytes: maxIconBytes + 1024,
	}, httpcore.RoleAdmin), s.uploadChannelWatermark)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "create-channel", Method: http.MethodPost, Path: "/v1/channels",
		Summary: "Create a channel", Description: "Admin only. From an approved proposal or hand-made.",
		Tags: []string{"channels"},
	}, httpcore.RoleAdmin), s.createChannel)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "update-channel", Method: http.MethodPatch, Path: "/v1/channels/{id}",
		Summary:     "Edit a channel",
		Description: "Admin only. Partial update of operator-owned fields (name/number/group/strategy), the per-channel programming policy, and pause/resume via status. Renumber is unique-checked (409). Every edit auto-reconciles — there is no manual rebuild.",
		Tags:        []string{"channels"},
	}, httpcore.RoleAdmin), s.updateChannel)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "refine-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/refine",
		Summary:     "Refine a channel with the LLM",
		Description: "Admin only. Describe a change; the LLM re-proposes using the channel's current lineup as context, grounded like any suggestion. Returns a jobId — poll /v1/proposals for the proposal, review the diff, and approve to apply (patches THIS channel).",
		Tags:        []string{"channels", "suggestions"},
	}, httpcore.RoleAdmin), s.refineChannel)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "reconcile-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/reconcile",
		Summary: "Force channel convergence", Description: "Admin only. Rebuilds desired programming on the channel's selected playout backend. Idempotent (§9). " + httpcore.OperatorAPI,
		Tags: []string{"channels"},
	}, httpcore.RoleAdmin), s.reconcileChannel)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "delete-channel", Method: http.MethodDelete, Path: "/v1/channels/{id}",
		Summary: "Remove a channel", Description: "Admin only. Detaches by default (§7).",
		Tags: []string{"channels"}, DefaultStatus: http.StatusNoContent,
	}, httpcore.RoleAdmin), s.deleteChannel)
	// The Watch surface's play-url op (§9.1) is mounted by the playout routes.
}

type channelOutput struct{ Body ChannelDTO }

type listChannelsOutput struct {
	Body struct {
		Channels []ChannelDTO `json:"channels"`
	}
}

func (s *Handlers) listChannels(ctx context.Context, _ *struct{}) (*listChannelsOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := &listChannelsOutput{}
	out.Body.Channels = make([]ChannelDTO, 0, len(all))
	// ⚠ Resolved ONCE for the whole list, before the loop. A lookup inside channelToDTO would be
	// an N+1 — one image row read per channel — which is the shape a profile here has already
	// caught once. Distinct hashes are fetched once each even when channels share an icon.
	logos := make([]string, 0, len(all))
	for _, ch := range all {
		logos = append(logos, ch.Logo)
	}
	logoImage := s.logoImageResolver(ctx, logos)
	requesters := s.channelRequesters(ctx) // once for the list, like the logos
	for _, ch := range all {
		// nil entry-state resolver: the list shows counts, not per-entry state — no per-key fan-out.
		out.Body.Channels = append(out.Body.Channels, s.channelDTOAt(ch, nil, logoImage, requesters, checkpoint))
	}
	return out, nil
}

func (s *Handlers) getChannel(ctx context.Context, in *httpcore.ChannelIDInput) (*channelOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: s.channelDTOAt(ch, s.entryStateResolver(ctx), s.logoImageResolver(ctx, []string{ch.Logo}), s.channelRequesters(ctx), checkpoint)}, nil
}

type iconSuggestionsOutput struct {
	Body struct {
		Suggestions []IconSuggestion `json:"suggestions"`
	}
}

// channelIconSuggestions offers candidate icons drawn from the channel's OWN lineup
// (§icon P2): a Star Trek channel's five series posters, rather than a generic
// placeholder. Read-only — any authenticated user may call it, matching get-channel.
func (s *Handlers) channelIconSuggestions(ctx context.Context, in *httpcore.ChannelIDInput) (*iconSuggestionsOutput, error) {
	if s.icons == nil || s.unconfigured(settings.TMDBAPIKey) {
		return nil, httpcore.NotImplemented("Icon suggestions aren't set up", "Connect TMDB in Settings to get channel icon suggestions from your lineup.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	suggestions, err := s.icons.IconSuggestions(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	// Enrich each suggestion with its image record in ONE deduplicated pass (§22). A per-row
	// lookup here would be the same N+1 `logoImageResolver` exists to avoid, and the picker is
	// twelve rows — the exact size where the naive shape starts to show.
	hashes := make([]string, 0, len(suggestions))
	for _, sg := range suggestions {
		if sg.ImageHash != "" {
			hashes = append(hashes, sg.ImageHash)
		}
	}
	byHash := s.imageDTOsByHash(ctx, hashes)
	for i := range suggestions {
		// A miss leaves Image nil and the plain `url` still renders — §22's Durability section
		// makes "the row is here, the file is not" an accepted state, so this must never fail.
		suggestions[i].Image = byHash[suggestions[i].ImageHash]
	}

	out := &iconSuggestionsOutput{}
	// Always a slice, never null: a lineup with no resolvable posters (e.g. all-TVDB
	// entries with no TMDB bridge match) is a normal state, not a failure.
	out.Body.Suggestions = suggestions
	if out.Body.Suggestions == nil {
		out.Body.Suggestions = []IconSuggestion{}
	}
	return out, nil
}

type createChannelInput struct {
	Body struct {
		ID       string `json:"id,omitempty" doc:"Loomarr channel id (stable). Optional — omit and the server assigns one (ch_…); pass one to keep a caller-assigned id." example:"ch_abc123"`
		Name     string `json:"name" example:"Saturday Morning Cartoons"`
		Number   int    `json:"number" minimum:"1" example:"42"`
		Group    string `json:"group,omitempty" example:"Kids"`
		Logo     string `json:"logo,omitempty"`
		Strategy string `json:"strategy" enum:"sequential,shuffle,time_slot"`
		// Series is an optional hand-made single-series channel (§7 "or hand-made"):
		// the channel plays one show, optionally constrained to a season range (§9
		// series expansion). The series must already be an `available` title.
		Series *struct {
			Key       string `json:"key" doc:"provisioning key, e.g. series:tvdb:71663" example:"series:tvdb:71663"`
			Title     string `json:"title,omitempty"`
			SeasonMin int    `json:"seasonMin,omitempty" doc:"first season (inclusive; 0 = unbounded)"`
			SeasonMax int    `json:"seasonMax,omitempty" doc:"last season (inclusive; 0 = unbounded)"`
		} `json:"series,omitempty"`
	}
}

func (s *Handlers) createChannel(ctx context.Context, in *createChannelInput) (*channelOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	req := edit.NewChannel{
		ID: in.Body.ID, Name: in.Body.Name, Number: in.Body.Number, Group: in.Body.Group, Logo: in.Body.Logo,
		Strategy: schedule.Strategy(in.Body.Strategy),
	}
	if in.Body.Series != nil {
		req.Series = &edit.SeriesScope{
			Key: provision.Key(in.Body.Series.Key), Title: in.Body.Series.Title,
			SeasonMin: in.Body.Series.SeasonMin, SeasonMax: in.Body.Series.SeasonMax,
		}
	}
	fresh, err := s.channelEdits().Create(ctx, req)
	if err != nil {
		return nil, channelEditProblem(err)
	}
	return &channelOutput{Body: s.channelDTOAt(fresh, s.entryStateResolver(ctx), s.logoImageResolver(ctx, []string{fresh.Logo}), s.channelRequesters(ctx), checkpoint)}, nil
}

// updateChannelInput is a PARTIAL edit (§7): a nil field is "leave unchanged", so
// omitting a field and setting it to its zero value are distinguishable (a rename to
// "" is rejected by Validate, not silently ignored). Operator-owned fields + the
// per-channel policy + pause/resume via status + the whole-list lineup replace.
type updateChannelInput struct {
	ID   string `path:"id" example:"ch_abc123"`
	Body struct {
		Revision int64                   `json:"revision" minimum:"1" doc:"Revision returned by the channel read this edit was based on. A stale revision is rejected with 409."`
		Name     *string                 `json:"name,omitempty"`
		Number   *int                    `json:"number,omitempty" minimum:"1"`
		Group    *string                 `json:"group,omitempty"`
		Logo     *string                 `json:"logo,omitempty" doc:"Channel icon URL (from TMDB, an upload path, or set directly). Empty string clears it."`
		Strategy *string                 `json:"strategy,omitempty" enum:"sequential,shuffle,time_slot"`
		Policy   *schedule.ChannelPolicy `json:"policy,omitempty" doc:"Per-channel programming policy; merged onto the channel. policy.applied is reconcile-owned and ignored on write."`
		// Status is limited to pause/resume: "paused" takes the channel off the sweep,
		// "building" resumes it. Other lifecycle values (live/drifted/detached) are the
		// reconciler's/delete's to set, never a client's.
		Status *string `json:"status,omitempty" enum:"paused,building" doc:"Pause Loomarr-managed playout and guide surfaces, or resume. In v1, a retained Tunarr projection keeps its last lineup."`
		// Lineup is a WHOLE-LIST replace (§7): the full ordered set of entries. Add = a new
		// entry, remove = an omitted one, reorder = the same entries reordered. Each key is
		// validated (provision.ParseKey); a key not `available` in the library is inert (a
		// pending slot, never played) until it lands, so this can't play unapproved content.
		// nil = leave the lineup unchanged; an empty (non-nil) array clears it.
		Lineup *[]LineupEntryDTO `json:"lineup,omitempty" doc:"Whole-list replace of the channel's lineup (ordered). Omit to leave unchanged. Each entry's key is validated; a not-yet-available key renders as a pending slot until the title lands."`
	}
}

// updateChannel edits a channel (§7 PATCH) through channeledit, as a partial update that
// preserves operator/proposal ownership: a client may set name/number/group/strategy/policy and
// pause/resume, but never policy.applied (reconcile owns it) nor the derived Desired.
// The edit AUTO-RECONCILES (best-effort, seamless) — there is no manual rebuild step.
func (s *Handlers) updateChannel(ctx context.Context, in *updateChannelInput) (*channelOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	fresh, err := s.channelEdits().Edit(ctx, editBackend(checkpoint), in.ID, in.Body.Revision, func(ch *channelstore.Channel) error {
		return applyChannelPatch(ch, in)
	})
	if err != nil {
		return nil, channelEditProblem(err)
	}
	return &channelOutput{Body: s.channelDTOAt(fresh, s.entryStateResolver(ctx), s.logoImageResolver(ctx, []string{fresh.Logo}), s.channelRequesters(ctx), checkpoint)}, nil
}

// applyChannelPatch applies a PATCH body's fields. The edit rules around it (number check,
// `building` signal, deadline, revision, reconcile) are channeledit's.
func applyChannelPatch(ch *channelstore.Channel, in *updateChannelInput) error {
	if in.Body.Name != nil {
		ch.Name = strings.TrimSpace(*in.Body.Name)
	}
	if in.Body.Group != nil {
		ch.Group = *in.Body.Group
	}
	if in.Body.Logo != nil {
		// The icon URL is pushed to Tunarr's channel icon on the auto-reconcile below. An
		// empty string clears it (Tunarr falls back to no icon). Trimmed so a stray space
		// doesn't become a broken URL.
		ch.Logo = strings.TrimSpace(*in.Body.Logo)
	}
	if in.Body.Strategy != nil {
		ch.Strategy = schedule.Strategy(*in.Body.Strategy)
	}
	// A renumber is checked by the service, and only when the number actually changes.
	if in.Body.Number != nil {
		ch.Number = *in.Body.Number
	}
	// Policy merge goes through the single ownership site (schedule.MergeFromOperator, §2.1):
	// the operator's values win, the reconcile-owned `applied` is force-preserved (never
	// client-set), and every proposal-owned field the operator CHANGED is recorded in
	// OperatorSet so a later refine can't silently revert it (§8.2 stickiness). Validate
	// rejects an off-ladder audience ceiling / bad enum values (§4 safety).
	if in.Body.Policy != nil {
		if err := in.Body.Policy.DateAliasConflict(); err != nil {
			return httpcore.AmbiguousDatesError(err)
		}
		next := ch.Policy.MergeFromOperator(*in.Body.Policy)
		if verr := next.Validate(); verr != nil {
			return httpcore.ProblemWithCause(http.StatusUnprocessableEntity, "Invalid policy",
				"Some programming policy settings are invalid. Check the audience and filler options, then try again.", verr)
		}
		ch.Policy = next
	}
	// Pause/resume. Only paused↔building; the transition is deliberately narrow so a
	// client can't force a channel `live`/`detached` out from under the reconciler.
	if in.Body.Status != nil {
		switch schedule.ChannelStatus(*in.Body.Status) {
		case schedule.StatusPaused:
			ch.Status = schedule.StatusPaused
		case schedule.StatusBuilding:
			// Resume only makes sense from paused; from any other state it's a no-op-ish
			// nudge back onto the sweep, which the reconcile below will settle.
			ch.Status = schedule.StatusBuilding
		default:
			return httpcore.Unprocessable("Unsupported status",
				"A channel can only be paused or resumed here.")
		}
	}
	// Lineup edit (add/remove/reorder as a whole-list replace). Validate every key and
	// preserve the rich scheduling metadata the read DTO drops (season range, rating,
	// runtime) by matching incoming entries to the current lineup by key — a reorder must
	// not silently reset a series' season scope. Desired is NOT computed here; the
	// reconcile below derives it from ch.Lineup (§9).
	if in.Body.Lineup != nil {
		next, lerr := mergeLineupEdit(ch.Lineup, *in.Body.Lineup)
		if lerr != nil {
			return httpcore.ProblemWithCause(http.StatusUnprocessableEntity, "Invalid lineup",
				"The lineup couldn't be saved. Check that each entry is a valid title, then try again.", lerr)
		}
		ch.Lineup = next
	}
	return validateChannelPlace(ch.Policy)
}

type refineChannelInput struct {
	ID   string `path:"id" example:"ch_abc123"`
	Body struct {
		Change string `json:"change" minLength:"1" doc:"What to change, in plain words: \"add more Schwarzenegger, drop the slow ones\"." example:"add more Schwarzenegger"`
	}
}
type refineChannelOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Poll /v1/proposals for the refined proposal (matched on this jobId), then approve to apply."`
	}
}

// refineChannel re-runs the channel's suggestion with a plain-language change (§7). It
// seeds a refine intent from the channel's CURRENT lineup + the intent the channel owns + the
// change, and submits a new job that names the channel so the refined proposal binds back to
// it — approving it patches in place, no duplicate.
// Grounding is unchanged: the current lineup is context; every pick is still catalog-tool
// grounded.
func (s *Handlers) refineChannel(ctx context.Context, in *refineChannelInput) (*refineChannelOutput, error) {
	if s.suggest == nil {
		return nil, httpcore.NotImplemented("AI isn't set up", "Connect an AI provider in Settings → AI to refine channels.")
	}
	if err := httpcore.SuggestionConfigurationError(ctx, s.settings); err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// Refine only makes sense for an LLM-created channel: it is seeded from the request the
	// channel owns. A hand-made channel (no IntentRef) has no request to refine.
	if ch.IntentRef == "" {
		return nil, httpcore.Unprocessable("Can't refine this channel",
			"This channel was made by hand, not from an AI suggestion, so there's nothing to refine.")
	}

	// Seed the refine intent from the channel's own approved intent (description, era, tone,
	// constraints) plus the current lineup as context and the requested change. A channel whose
	// request is gone is never refined from a guess: the admin describes it again.
	intent, err := suggest.IntentOfChannel(ch)
	if errors.Is(err, suggest.ErrChannelIntentUnavailable) {
		return nil, httpcore.Unprocessable("Can't refine this channel yet",
			"Loomarr no longer has the original request for this channel. Describe the channel again to get a new proposal.")
	} else if err != nil {
		return nil, err
	}
	intent.RefineText = strings.TrimSpace(in.Body.Change)
	intent.CurrentLineup = lineupContext(ch.Lineup)

	// One channel job at a time: a second refine, or one during a re-curation, would race the
	// first to replace the same lineup.
	if busy, err := s.store.ChannelHasUnfinishedJob(ctx, ch.ID); err != nil {
		return nil, err
	} else if busy {
		return nil, httpcore.Conflict("A refine of this channel is already in progress",
			"This channel already has a change being generated or waiting for review. Finish or dismiss it before asking for another.")
	}

	jobID, err := s.suggest.Refine(ctx, ch.ID, intent, httpcore.UserIDFrom(ctx))
	if err != nil {
		return nil, err
	}
	out := &refineChannelOutput{}
	out.Body.JobID = jobID
	return out, nil
}

// lineupContext turns a channel's stored lineup entries into the lightweight
// name/year/key context the refiner reasons about.
func lineupContext(entries []schedule.LineupEntry) []proposals.LineupContext {
	out := make([]proposals.LineupContext, 0, len(entries))
	for _, e := range entries {
		out = append(out, proposals.LineupContext{Name: e.Title, Year: e.Year, Key: string(e.Key)})
	}
	return out
}

type reconcileOutput struct{ Body ChannelDTO }

func (s *Handlers) reconcileChannel(ctx context.Context, in *httpcore.ChannelIDInput) (*reconcileOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	if s.channels == nil {
		return nil, httpcore.NotImplemented("Channel scheduling isn't set up", "Channel scheduling isn't available on this instance.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	if err := s.channels.Reconcile(ctx, in.ID); err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't reconcile the channel",
			"Loomarr couldn't rebuild this channel on its selected playout backend. Check that backend's settings and try again.", err)
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &reconcileOutput{Body: s.channelDTOAt(ch, s.entryStateResolver(ctx), s.logoImageResolver(ctx, []string{ch.Logo}), s.channelRequesters(ctx), checkpoint)}, nil
}

type deleteChannelInput struct {
	ID    string `path:"id"`
	Purge bool   `query:"purge" doc:"Hard-delete Loomarr's channel state and any retained managed Tunarr projection (default: detach only, §7)"`
}
type deleteChannelOutput struct{}

func (s *Handlers) deleteChannel(ctx context.Context, in *deleteChannelInput) (*deleteChannelOutput, error) {
	checkpoint, err := s.checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	wasTransportPlayable := httpcore.TransportPlayableAt(ch, checkpoint)
	// Purge (?purge=true): hard-delete Loomarr's local state and any retained managed
	// Tunarr projection through the channel service. This includes a historical projection
	// preserved after switching the channel to internal playout.
	if in.Purge {
		if s.channels == nil {
			return nil, httpcore.NotImplemented("Channel scheduling isn't set up", "Channel scheduling isn't available on this instance.")
		}
		if err := s.channels.Purge(ctx, in.ID); err != nil {
			if errors.Is(err, channelstore.ErrChannelStale) || errors.Is(err, channelstore.ErrChannelConflict) {
				return nil, staleChannelConflict()
			}
			return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't purge the channel",
				"Loomarr couldn't hard-delete its local state and any retained managed Tunarr projection. Check the configured integrations and try again.", err)
		}
		if wasTransportPlayable && s.playout != nil {
			s.playout.StopChannel(ch.ID)
		}
		return &deleteChannelOutput{}, nil
	}
	// Default: detach (stop managing; leave the Tunarr channel + the store row).
	// Detached channels are never reconciled again (§9 ownership).
	ch.Status = schedule.StatusDetached
	ch.UpdatedAt = time.Now().Unix()
	saved, err := s.store.SaveChannel(ctx, ch)
	if err != nil {
		if errors.Is(err, channelstore.ErrChannelStale) {
			return nil, staleChannelConflict()
		}
		return nil, err
	}
	s.applyInternalPlayoutLifecycle(ctx, saved.ID, wasTransportPlayable, false)
	return &deleteChannelOutput{}, nil
}

// applyInternalPlayoutLifecycle handles membership changes in Loomarr's internal tuner catalog.
// The persisted row is already committed when this runs, so a removal first fails new tune
// requests through the canonical eligibility check, then disconnects existing sessions. Both
// additions and removals re-scan the tuner: a guide refresh cannot change its channel list.
//
// Tunarr projections are intentionally untouched. §9 ownership preserves them across backend
// transitions, and the v1 pause contract retains their last lineup; durable remote pause needs a
// separate projection state rather than a one-shot best-effort clear.
func (s *Handlers) applyInternalPlayoutLifecycle(
	ctx context.Context, channelID string, wasTransportPlayable, isTransportPlayable bool,
) {
	if wasTransportPlayable == isTransportPlayable {
		return
	}
	if wasTransportPlayable && s.playout != nil {
		s.playout.StopChannel(channelID)
	}
	if s.tunerRescanner != nil {
		if err := s.tunerRescanner.RescanTuner(ctx); err != nil {
			if s.log != nil {
				s.log.Warn("channel lifecycle: tuner rescan failed",
					"channel", channelID, "err", err)
			}
		}
	}
}
