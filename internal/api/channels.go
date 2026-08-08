package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// ChannelDTO is the API view of a scheduler channel (§9). The desired lineup is
// summarized (counts) rather than dumped slot-by-slot on the channel resource;
func (s *Server) registerChannels(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-channels", Method: http.MethodGet, Path: "/v1/channels",
		Summary: "List channels", Tags: []string{"channels"},
	}, RoleMember), s.listChannels)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "channels-now-next", Method: http.MethodGet, Path: "/v1/channels/now-next",
		Summary: "What is airing now and next", Description: "Per channel, the program currently airing and the one after it, read from Tunarr's generated guide (§6: Tunarr owns playout, so airtimes are its truth, never recomputed here). ONE upstream call serves every channel.",
		Tags: []string{"channels"},
	}, RoleMember), s.channelsNowNext)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-channel", Method: http.MethodGet, Path: "/v1/channels/{id}",
		Summary: "Get a channel definition + status", Tags: []string{"channels"},
	}, RoleMember), s.getChannel)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-upcoming", Method: http.MethodGet, Path: "/v1/channels/{id}/upcoming",
		Summary:     "What's on this channel now and next",
		Description: "The program airing now (first) then the next few, in airtime order, from Tunarr's generated guide (§6 airtimes; gaps skipped). Powers the Overview 'what's on later' strip. Read-only — any authenticated user (§8.1 viewer-facing).",
		Tags:        []string{"channels"},
	}, RoleMember), s.channelUpcoming)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-icon-suggestions", Method: http.MethodGet, Path: "/v1/channels/{id}/icon-suggestions",
		Summary:     "Suggest channel icons from the lineup",
		Description: "Candidate poster images drawn from the channel's OWN lineup titles (§icon) — a Star Trek channel offers its five series' posters. Read-only, so any authenticated user may call it. 501 when TMDB isn't configured.",
		Tags:        []string{"channels"},
	}, RoleMember), s.channelIconSuggestions)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-channel-pods", Method: http.MethodGet, Path: "/v1/channels/{id}/pods",
		Summary:     "Preview the commercial pool this channel would get",
		Description: "Assembles the channel's SAVED filler pool WITHOUT touching Tunarr (§10, §12). Same code path and same seed as reconcile, so what you see is what the channel gets. Read-only, so any authenticated user may call it.",
		Tags:        []string{"channels", "filler"},
	}, RoleMember), s.previewChannelPods)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-filler-coverage", Method: http.MethodGet, Path: "/v1/channels/{id}/filler/coverage",
		Summary:     "How well the catalog covers this channel's breaks",
		Description: "Which rung of the §10 fallback ladder this channel's breaks would draw from, and how much material each rung holds. Computed from the SAME ladder reconcile uses (internal/filler), through the same per-channel selection as the pod preview — so the meter and the pods cannot disagree. Read-only, so any authenticated user may call it. A rung the channel's policy skips is ABSENT rather than zero, and `total` is the widest rung rather than a sum (the rungs nest).",
		Tags:        []string{"channels", "filler"},
	}, RoleMember), s.channelFillerCoverage)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-channel-cycle", Method: http.MethodGet, Path: "/v1/channels/{id}/cycle",
		Summary:     "Preview what airs at a chosen time (curation rules)",
		Description: "The time-travel cycle preview (§8.1): what this channel would air at `at` (default now), and WHICH curation rule is active then. Runs the SAME pure lineup builder as reconcile — preview and reality cannot disagree — WITHOUT touching Tunarr or the store beyond a read. Makes first-match-by-priority rule resolution legible (\"at Saturday 9am, the Weekend TNG marathon rule is active\"). Read-only, so any authenticated user may call it; `at` may be past or future.",
		Tags:        []string{"channels"},
	}, RoleMember), s.previewChannelCycle)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-draft-channel-pods", Method: http.MethodPost, Path: "/v1/channels/{id}/pods/preview",
		Summary:     "Preview a draft filler selection without saving it",
		Description: "Assembles the pool a DRAFT filler selection would produce, without persisting it (§10, §12) — the live sandbox on the channel's Filler section. Same assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what will air once applied. Admin-only (an authoring tool); applying is a normal PATCH of policy.filler.",
		Tags:        []string{"channels", "filler"},
	}, RoleAdmin), s.previewDraftChannelPods)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "create-channel", Method: http.MethodPost, Path: "/v1/channels",
		Summary: "Create a channel", Description: "Admin only. From an approved proposal or hand-made.",
		Tags: []string{"channels"},
	}, RoleAdmin), s.createChannel)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "update-channel", Method: http.MethodPatch, Path: "/v1/channels/{id}",
		Summary:     "Edit a channel",
		Description: "Admin only. Partial update of operator-owned fields (name/number/group/strategy), the per-channel programming policy, and pause/resume via status. Renumber is unique-checked (409). Every edit auto-reconciles — there is no manual rebuild.",
		Tags:        []string{"channels"},
	}, RoleAdmin), s.updateChannel)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "refine-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/refine",
		Summary:     "Refine a channel with the LLM",
		Description: "Admin only. Describe a change; the LLM re-proposes using the channel's current lineup as context, grounded like any suggestion. Returns a jobId — poll /v1/proposals for the proposal, review the diff, and approve to apply (patches THIS channel).",
		Tags:        []string{"channels", "suggestions"},
	}, RoleAdmin), s.refineChannel)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "reconcile-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/reconcile",
		Summary: "Force desired→Tunarr reconciliation", Description: "Admin only. Idempotent (§9).",
		Tags: []string{"channels"},
	}, RoleAdmin), s.reconcileChannel)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "delete-channel", Method: http.MethodDelete, Path: "/v1/channels/{id}",
		Summary: "Remove a channel", Description: "Admin only. Detaches by default (§7).",
		Tags: []string{"channels"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.deleteChannel)

	// The Watch surface's play-url op (§9.1). Mounted here so both the live router and the
	// OpenAPI-export parity path (export.go) pick it up from one call, never a hand-kept list.
	s.registerChannelPlayURL(api)
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
		// nil resolver: the list shows counts, not per-entry state — no per-key fan-out.
		out.Body.Channels = append(out.Body.Channels, channelToDTO(ch, nil))
	}
	return out, nil
}

func (s *Server) getChannel(ctx context.Context, in *channelIDInput) (*channelOutput, error) {
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: channelToDTO(ch, s.entryStateResolver(ctx))}, nil
}

type iconSuggestionsOutput struct {
	Body struct {
		Suggestions []IconSuggestion `json:"suggestions"`
	}
}

// channelIconSuggestions offers candidate icons drawn from the channel's OWN lineup
// (§icon P2): a Star Trek channel's five series posters, rather than a generic
// placeholder. Read-only — any authenticated user may call it, matching get-channel.
func (s *Server) channelIconSuggestions(ctx context.Context, in *channelIDInput) (*iconSuggestionsOutput, error) {
	if s.icons == nil {
		return nil, errNotImplemented("Icon suggestions aren't set up", "Connect TMDB in Settings to get channel icon suggestions from your lineup.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	suggestions, err := s.icons.IconSuggestions(ctx, in.ID)
	if err != nil {
		return nil, err
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
		ID        string `json:"id,omitempty" doc:"Loomarr channel id (stable). Optional — omit and the server assigns one (ch_…); pass one to keep a caller-assigned id." example:"ch_abc123"`
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
	// The id is optional: a caller (e.g. the proposal-approval path) may supply a stable
	// id, or omit it and let the server mint one — same `ch_…` scheme binder.BindApprovedChannel
	// uses, so a hand-made channel is indistinguishable from an approved one. This is what
	// lets the "New channel" UI action create without a client-side id scheme.
	ch.ID = in.Body.ID
	if ch.ID == "" {
		ch.ID = binder.NewChannelID()
	}
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
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Couldn't build the lineup",
			"Loomarr couldn't resolve the lineup for the approved suggestion. Try refining or re-approving it.", err)
	}
	// Bind the proposal's grounded ChannelPolicy (programming-design §8) onto the
	// channel so enforcement (scope/audience/separation/seasonal) applies from the
	// first reconcile. A hand-made channel / policy-less proposal → built-in defaults.
	policy, err := s.policyFromIntent(ctx, in.Body.IntentRef)
	if err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Couldn't apply the policy",
			"Loomarr couldn't resolve the programming policy for the approved suggestion. Try refining or re-approving it.", err)
	}
	ch.Policy = policy
	// Hand-made single-series channel (§7 "or hand-made"): one series entry with an
	// optional season range (§9 expansion). Only when no proposal intent is given.
	if in.Body.IntentRef == "" && in.Body.Series != nil {
		key := provision.Key(in.Body.Series.Key)
		if !key.IsSeries() {
			return nil, errUnprocessable("Invalid series",
				"For a single-series channel, pick a series (not a movie or episode).")
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
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid channel",
			"Some channel details are invalid. Check the name, number, and strategy, then try again.", err)
	}
	// Reject a duplicate id or number up front (the store's unique index would
	// also reject, but a clean 409 is friendlier than a 500).
	if _, err := s.store.GetChannel(ctx, ch.ID); err == nil {
		return nil, errConflict("Channel already exists", "A channel with that id already exists.")
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if _, err := s.store.GetChannelByNumber(ctx, ch.Number); err == nil {
		return nil, errConflict("Channel number in use", "Another channel already uses that number. Pick a different one.")
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, err
	}
	// Kick an initial reconcile so the channel goes live immediately (§9 "live
	// immediately — never dead air"). Best-effort: a reconcile failure leaves the
	// channel in `building` for the sweep to pick up, it doesn't fail creation.
	if s.channels != nil && !s.unconfigured("tunarr.url") {
		if err := s.channels.Reconcile(ctx, ch.ID); err != nil {
			s.log.Warn("initial channel reconcile failed (sweep will retry)", "channel", ch.ID, "err", err)
		}
	}
	fresh, err := s.store.GetChannel(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: channelToDTO(fresh, s.entryStateResolver(ctx))}, nil
}

// updateChannelInput is a PARTIAL edit (§7): a nil field is "leave unchanged", so
// omitting a field and setting it to its zero value are distinguishable (a rename to
// "" is rejected by Validate, not silently ignored). Operator-owned fields + the
// per-channel policy + pause/resume via status; lineup edits arrive with the lineup
// read shape (Phase 3).
type updateChannelInput struct {
	ID   string `path:"id" example:"ch_abc123"`
	Body struct {
		Name     *string                 `json:"name,omitempty"`
		Number   *int                    `json:"number,omitempty" minimum:"1"`
		Group    *string                 `json:"group,omitempty"`
		Logo     *string                 `json:"logo,omitempty" doc:"Channel icon URL (from TMDB, an upload path, or set directly). Empty string clears it."`
		Strategy *string                 `json:"strategy,omitempty" enum:"sequential,shuffle,time_slot"`
		Policy   *schedule.ChannelPolicy `json:"policy,omitempty" doc:"Per-channel programming policy; merged onto the channel. policy.applied is reconcile-owned and ignored on write."`
		// Status is limited to pause/resume: "paused" takes the channel off the sweep,
		// "building" resumes it. Other lifecycle values (live/drifted/detached) are the
		// reconciler's/delete's to set, never a client's.
		Status *string `json:"status,omitempty" enum:"paused,building" doc:"Pause (off air, keep the channel) or resume."`
		// Lineup is a WHOLE-LIST replace (§7): the full ordered set of entries. Add = a new
		// entry, remove = an omitted one, reorder = the same entries reordered. Each key is
		// validated (provision.ParseKey); a key not `available` in the library is inert (a
		// pending slot, never played) until it lands, so this can't play unapproved content.
		// nil = leave the lineup unchanged; an empty (non-nil) array clears it.
		Lineup *[]LineupEntryDTO `json:"lineup,omitempty" doc:"Whole-list replace of the channel's lineup (ordered). Omit to leave unchanged. Each entry's key is validated; a not-yet-available key renders as a pending slot until the title lands."`
	}
}

// updateChannel edits a channel (§7 PATCH). It mirrors createChannel's validate →
// uniqueness → upsert → auto-reconcile shape, but as a partial update that preserves
// operator/proposal ownership: a client may set name/number/group/strategy/policy and
// pause/resume, but never policy.applied (reconcile owns it) nor the derived Desired.
// The edit AUTO-RECONCILES (best-effort, seamless) — there is no manual rebuild step.
func (s *Server) updateChannel(ctx context.Context, in *updateChannelInput) (*channelOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

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
	// Renumber: unique-check EXCLUDING self (a no-op renumber to the channel's own
	// number must not 409). The store's unique index would also reject, but a clean
	// 409 beats a 500 (matches createChannel's rationale).
	if in.Body.Number != nil && *in.Body.Number != ch.Number {
		if other, gerr := s.store.GetChannelByNumber(ctx, *in.Body.Number); gerr == nil {
			if other.ID != ch.ID {
				return nil, errConflict("Channel number in use", "Another channel already uses that number. Pick a different one.")
			}
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return nil, gerr
		}
		ch.Number = *in.Body.Number
	}
	// Policy merge goes through the single ownership site (schedule.MergeFromOperator, §2.1):
	// the operator's values win, the reconcile-owned `applied` is force-preserved (never
	// client-set), and every proposal-owned field the operator CHANGED is recorded in
	// OperatorSet so a later refine can't silently revert it (§8.2 stickiness). Validate
	// rejects an off-ladder audience ceiling / bad enum values (§4 safety).
	if in.Body.Policy != nil {
		next := ch.Policy.MergeFromOperator(*in.Body.Policy)
		if verr := next.Validate(); verr != nil {
			return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid policy",
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
			return nil, errUnprocessable("Unsupported status",
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
			return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid lineup",
				"The lineup couldn't be saved. Check that each entry is a valid title, then try again.", lerr)
		}
		ch.Lineup = next
	}

	if err := ch.Validate(); err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid channel",
			"Some channel details are invalid. Check the name, number, and strategy, then try again.", err)
	}
	ch.UpdatedAt = time.Now().Unix() // UpsertChannel does not stamp this
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, err
	}

	// Auto-reconcile so the edit reaches Tunarr with no user action (§9 "self-
	// maintaining"; there is no manual rebuild). Best-effort + skipped while paused or
	// Tunarr-unconfigured: the edit is durable regardless, and the sweep is the
	// guarantee. A reconcile emits a `channel` SSE frame so the UI updates live.
	if ch.Status != schedule.StatusPaused && s.channels != nil && !s.unconfigured("tunarr.url") {
		if rerr := s.channels.Reconcile(ctx, ch.ID); rerr != nil {
			s.log.Warn("reconcile after channel edit failed (sweep will retry)", "channel", ch.ID, "err", rerr)
		}
	}
	fresh, err := s.store.GetChannel(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	return &channelOutput{Body: channelToDTO(fresh, s.entryStateResolver(ctx))}, nil
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
// seeds a refine intent from the channel's CURRENT lineup + the original description + the
// change, and re-queues the channel's OWN suggestion job (its IntentRef) so the refined
// proposal binds back to this channel — approving it patches in place, no duplicate.
// Grounding is unchanged: the current lineup is context; every pick is still catalog-tool
// grounded.
func (s *Server) refineChannel(ctx context.Context, in *refineChannelInput) (*refineChannelOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.suggest == nil || s.featureOff(ctx, "suggestions") {
		return nil, errNotImplemented("AI isn't set up", "Connect an AI provider in Settings → AI to refine channels.")
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// Refine only makes sense for an LLM-created channel: it re-runs that channel's own
	// suggestion job. A hand-made channel (no IntentRef) has no job to re-run.
	if ch.IntentRef == "" {
		return nil, errUnprocessable("Can't refine this channel",
			"This channel was made by hand, not from an AI suggestion, so there's nothing to refine.")
	}

	// Seed the refine intent from the channel's original intent (for description/constraints)
	// plus the current lineup as context and the requested change.
	intent := suggest.Intent{Description: ch.Name}
	if job, jerr := s.store.GetJob(ctx, ch.IntentRef); jerr == nil {
		var orig suggest.Intent
		if json.Unmarshal([]byte(job.IntentJSON), &orig) == nil {
			intent = orig // keep the original description + era/tone/constraints
		}
	}
	intent.RefineText = strings.TrimSpace(in.Body.Change)
	intent.CurrentLineup = lineupContext(ch.Lineup)

	jobID, err := s.suggest.Refine(ctx, ch.IntentRef, intent)
	if err != nil {
		return nil, err
	}
	out := &refineChannelOutput{}
	out.Body.JobID = jobID
	return out, nil
}

// lineupContext turns a channel's stored lineup entries into the lightweight
// name/year/key context the refiner reasons about.
func lineupContext(entries []schedule.LineupEntry) []suggest.LineupContext {
	out := make([]suggest.LineupContext, 0, len(entries))
	for _, e := range entries {
		out = append(out, suggest.LineupContext{Name: e.Title, Year: e.Year, Key: string(e.Key)})
	}
	return out
}

type reconcileOutput struct{ Body ChannelDTO }

func (s *Server) reconcileChannel(ctx context.Context, in *channelIDInput) (*reconcileOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.channels == nil || s.unconfigured("tunarr.url") {
		return nil, errNotImplemented("Tunarr isn't set up", "Connect Tunarr in Settings before reconciling a channel.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	if err := s.channels.Reconcile(ctx, in.ID); err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't reach Tunarr",
			"Loomarr couldn't push this channel to Tunarr. Check its connection in Settings and try again.", err)
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &reconcileOutput{Body: channelToDTO(ch, s.entryStateResolver(ctx))}, nil
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
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	// Purge (?purge=true): delete the Tunarr channel AND hard-delete the store row,
	// through the engine (which holds the programmer). Idempotent on the Tunarr side.
	if in.Purge {
		if s.channels == nil {
			return nil, errNotImplemented("Tunarr isn't set up", "Connect Tunarr in Settings before purging its channel.")
		}
		if err := s.channels.Purge(ctx, in.ID); err != nil {
			return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't reach Tunarr",
				"Loomarr couldn't delete this channel in Tunarr. Check its connection in Settings and try again.", err)
		}
		return &deleteChannelOutput{}, nil
	}
	// Default: detach (stop managing; leave the Tunarr channel + the store row).
	// Detached channels are never reconciled again (§9 ownership).
	ch.Status = schedule.StatusDetached
	ch.UpdatedAt = time.Now().Unix()
	if err := s.store.UpsertChannel(ctx, ch); err != nil {
		return nil, err
	}
	return &deleteChannelOutput{}, nil
}
