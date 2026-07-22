package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
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
	Status       string `json:"status" enum:"building,live,drifted,detached,paused" doc:"Loomarr-side channel status (§9)"`
	TunarrID     string `json:"tunarrId,omitempty" doc:"Server-assigned Tunarr channel id; empty until first reconcile"`
	IntentRef    string `json:"intentRef,omitempty"`
	ProgramCount int    `json:"programCount" doc:"Real playable programs in the desired lineup"`
	SlotCount    int    `json:"slotCount" doc:"Total slots incl. filler/flex placeholders"`
	// Policy is the channel's ChannelPolicy (programming-design §2): scope/audience/
	// separation/ordering/seasonal, plus the relaxation-ladder steps the last
	// reconcile applied (policy.applied) — the UI renders these as policy chips and
	// relaxation banners. Empty ⇒ the channel runs on built-in defaults.
	Policy schedule.ChannelPolicy `json:"policy" doc:"Programming policy (scope/audience/separation/ordering/seasonal) + applied relaxations"`
	// Lineup is the intent-level "what should play" — the titles the channel is built
	// from, in order (distinct from the summarized Desired slots above). Read-only here;
	// the diff a refine shows (kept/added/removed) is computed against this. Editing the
	// lineup entries is Phase 3.
	Lineup []LineupEntryDTO `json:"lineup" doc:"The channel's titles, in order (the intent-level lineup, not the expanded slots)"`
}

// LineupEntryDTO is one title on the channel — enough to display and to diff a refine
// against (a real key + human name/year). Not the full scheduler entry.
type LineupEntryDTO struct {
	Key    string   `json:"key" doc:"Provisioning key, e.g. movie:tmdb:603"`
	Name   string   `json:"name"`
	Year   int      `json:"year,omitempty"`
	Genres []string `json:"genres,omitempty"`
}

func channelToDTO(ch store.Channel) ChannelDTO {
	d := schedule.DesiredLineup{Slots: ch.Desired}
	lineup := make([]LineupEntryDTO, 0, len(ch.Lineup))
	for _, e := range ch.Lineup {
		lineup = append(lineup, LineupEntryDTO{
			Key: string(e.Key), Name: e.Title, Year: e.Year, Genres: e.Genres,
		})
	}
	return ChannelDTO{
		ID: ch.ID, Name: ch.Name, Number: ch.Number, Group: ch.Group,
		Strategy: string(ch.Strategy), Status: string(ch.Status),
		TunarrID: ch.TunarrID, IntentRef: ch.IntentRef,
		ProgramCount: d.ProgramCount(), SlotCount: len(ch.Desired),
		Policy: ch.Policy, Lineup: lineup,
	}
}

// NowNextEntry is one program on a channel's timeline (§9 guide freshness). Airtimes
// come from Tunarr, which owns playout — Loomarr owns the lineup (what should play), not
// when it plays, so recomputing these locally would duplicate Tunarr's scheduling math.
type NowNextEntry struct {
	Title   string `json:"title"`
	StartMs int64  `json:"startMs" doc:"Epoch ms; airtime as Tunarr scheduled it"`
	StopMs  int64  `json:"stopMs"`
	Gap     bool   `json:"gap" doc:"A flex/commercial-pod gap rather than a program"`
	TMDBID  string `json:"tmdbId,omitempty" doc:"Joins to a provisioning key (movie:tmdb:<id>) when known"`
}

// ChannelNowNext is what a channel card shows: what is on, and what follows.
type ChannelNowNext struct {
	ChannelID string        `json:"channelId" doc:"Loomarr channel id"`
	Now       *NowNextEntry `json:"now,omitempty"`
	Next      *NowNextEntry `json:"next,omitempty"`
}

type nowNextOutput struct {
	Body struct {
		Channels []ChannelNowNext `json:"channels"`
	}
}

// channelsNowNext answers the Channels LIST in one upstream call: Tunarr's guide is keyed
// by channel id, so N cards cost one request, not N. A channel with no generated guide
// simply has no entry — an empty guide is "nothing to show", not a failure (finding 4).
func (s *Server) channelsNowNext(ctx context.Context, _ *struct{}) (*nowNextOutput, error) {
	out := &nowNextOutput{}
	out.Body.Channels = []ChannelNowNext{}
	if s.guide == nil {
		return out, nil // guide reader not configured (unit tests, no Tunarr) — empty, not 501
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	byTunarr, err := s.guide.NowNext(ctx, time.Now())
	if err != nil {
		// The guide is a nicety on a list view; a Tunarr hiccup must not blank the page.
		return out, nil
	}
	for _, ch := range channels {
		if ch.TunarrID == "" {
			continue // never reconciled: nothing is airing yet
		}
		if nn, ok := byTunarr[ch.TunarrID]; ok {
			nn.ChannelID = ch.ID
			out.Body.Channels = append(out.Body.Channels, nn)
		}
	}
	return out, nil
}

// registerChannels mounts /v1/channels* (§7). Reads are visible to any
// authenticated user; create/update/delete/reconcile require admin.
func (s *Server) registerChannels(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-channels", Method: http.MethodGet, Path: "/v1/channels",
		Summary: "List channels", Tags: []string{"channels"},
	}, s.listChannels)

	huma.Register(api, huma.Operation{
		OperationID: "channels-now-next", Method: http.MethodGet, Path: "/v1/channels/now-next",
		Summary: "What is airing now and next", Description: "Per channel, the program currently airing and the one after it, read from Tunarr's generated guide (§6: Tunarr owns playout, so airtimes are its truth, never recomputed here). ONE upstream call serves every channel.",
		Tags: []string{"channels"},
	}, s.channelsNowNext)

	huma.Register(api, huma.Operation{
		OperationID: "get-channel", Method: http.MethodGet, Path: "/v1/channels/{id}",
		Summary: "Get a channel definition + status", Tags: []string{"channels"},
	}, s.getChannel)

	huma.Register(api, huma.Operation{
		OperationID: "preview-channel-pods", Method: http.MethodGet, Path: "/v1/channels/{id}/pods",
		Summary:     "Preview the commercial pool this channel would get",
		Description: "Assembles the channel's filler pool WITHOUT touching Tunarr (§10, §12). Same code path and same seed as reconcile, so what you see is what the channel gets. Read-only, so any authenticated user may call it.",
		Tags:        []string{"channels", "filler"},
	}, s.previewChannelPods)

	huma.Register(api, huma.Operation{
		OperationID: "create-channel", Method: http.MethodPost, Path: "/v1/channels",
		Summary: "Create a channel", Description: "Admin only. From an approved proposal or hand-made.",
		Tags: []string{"channels"},
	}, s.createChannel)

	huma.Register(api, huma.Operation{
		OperationID: "update-channel", Method: http.MethodPatch, Path: "/v1/channels/{id}",
		Summary:     "Edit a channel",
		Description: "Admin only. Partial update of operator-owned fields (name/number/group/strategy), the per-channel programming policy, and pause/resume via status. Renumber is unique-checked (409). Every edit auto-reconciles — there is no manual rebuild.",
		Tags:        []string{"channels"},
	}, s.updateChannel)

	huma.Register(api, huma.Operation{
		OperationID: "refine-channel", Method: http.MethodPost, Path: "/v1/channels/{id}/refine",
		Summary:     "Refine a channel with the LLM",
		Description: "Admin only. Describe a change; the LLM re-proposes using the channel's current lineup as context, grounded like any suggestion. Returns a jobId — poll /v1/suggestions for the proposal, review the diff, and approve to apply (patches THIS channel).",
		Tags:        []string{"channels", "suggestions"},
	}, s.refineChannel)

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
	// Bind the proposal's grounded ChannelPolicy (programming-design §8) onto the
	// channel so enforcement (scope/audience/separation/seasonal) applies from the
	// first reconcile. A hand-made channel / policy-less proposal → built-in defaults.
	policy, err := s.policyFromIntent(ctx, in.Body.IntentRef)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("resolve intent policy", err)
	}
	ch.Policy = policy
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
	if s.channels != nil && !s.unconfigured("tunarr.url") {
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
		Strategy *string                 `json:"strategy,omitempty" enum:"sequential,shuffle,time_slot"`
		Policy   *schedule.ChannelPolicy `json:"policy,omitempty" doc:"Per-channel programming policy; merged onto the channel. policy.applied is reconcile-owned and ignored on write."`
		// Status is limited to pause/resume: "paused" takes the channel off the sweep,
		// "building" resumes it. Other lifecycle values (live/drifted/detached) are the
		// reconciler's/delete's to set, never a client's.
		Status *string `json:"status,omitempty" enum:"paused,building" doc:"Pause (off air, keep the channel) or resume."`
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
		return nil, huma.Error404NotFound("no such channel")
	} else if err != nil {
		return nil, err
	}

	if in.Body.Name != nil {
		ch.Name = strings.TrimSpace(*in.Body.Name)
	}
	if in.Body.Group != nil {
		ch.Group = *in.Body.Group
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
				return nil, huma.Error409Conflict("channel number already in use")
			}
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return nil, gerr
		}
		ch.Number = *in.Body.Number
	}
	// Policy merge: take the client's policy but PRESERVE the reconcile-owned `applied`
	// (a client can't set relaxations; they're recomputed each reconcile). Validate
	// rejects an off-ladder audience ceiling / bad enum values (§4 safety).
	if in.Body.Policy != nil {
		next := *in.Body.Policy
		next.Applied = ch.Policy.Applied // reconcile owns this; never client-set
		if verr := next.Validate(); verr != nil {
			return nil, huma.Error422UnprocessableEntity("invalid policy", verr)
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
			return nil, huma.Error422UnprocessableEntity("status may only be set to paused or building", nil)
		}
	}

	if err := ch.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid channel", err)
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
	return &channelOutput{Body: channelToDTO(fresh)}, nil
}

type refineChannelInput struct {
	ID   string `path:"id" example:"ch_abc123"`
	Body struct {
		Change string `json:"change" minLength:"1" doc:"What to change, in plain words: \"add more Schwarzenegger, drop the slow ones\"." example:"add more Schwarzenegger"`
	}
}
type refineChannelOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Poll /v1/suggestions for the refined proposal (matched on this jobId), then approve to apply."`
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
		return nil, huma.Error501NotImplemented("suggester not configured")
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such channel")
	} else if err != nil {
		return nil, err
	}
	// Refine only makes sense for an LLM-created channel: it re-runs that channel's own
	// suggestion job. A hand-made channel (no IntentRef) has no job to re-run.
	if ch.IntentRef == "" {
		return nil, huma.Error422UnprocessableEntity("this channel wasn't created from a suggestion, so it can't be refined")
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
	// Purge (?purge=true): delete the Tunarr channel AND hard-delete the store row,
	// through the engine (which holds the programmer). Idempotent on the Tunarr side.
	if in.Purge {
		if s.channels == nil {
			return nil, huma.Error501NotImplemented("scheduler not configured (cannot purge the Tunarr channel)")
		}
		if err := s.channels.Purge(ctx, in.ID); err != nil {
			return nil, huma.Error502BadGateway("purge failed", err)
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

// PodEntryDTO is one clip placed in the previewed pool, in play order.
type PodEntryDTO struct {
	TunarrProgramID string `json:"tunarrProgramId,omitempty" doc:"Empty for the embedded fallback bumper card, which is not a Tunarr program"`
	Name            string `json:"name"`
	Kind            string `json:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	DurationMs      int64  `json:"durationMs"`
	IsFallbackCard  bool   `json:"isFallbackCard" doc:"The embedded default bumper card — the bottom of the fallback ladder"`
}

type previewPodsInput struct {
	ID string `path:"id"`
}
type previewPodsOutput struct {
	Body struct {
		Entries []PodEntryDTO `json:"entries"`
		TotalMs int64         `json:"totalMs"`
		// MatchLevel is how far down the §10 fallback ladder assembly had to go. This
		// is the answer to "why are my commercials wrong": exact means era+audience
		// matched, and bumper_card means nothing matched and the channel is running on
		// the embedded card alone.
		// Enumerated so orval generates a union the FE can switch on exhaustively. Left
		// as a bare string, the frontend would hand-mirror these values — and the one
		// that already did drifted out of sync with the ladder's real levels.
		MatchLevel string `json:"matchLevel" enum:"exact,widened,audience,bumper_card" doc:"How far down the fallback ladder assembly went (§10)"`
	}
}

// previewChannelPods shows the filler pool a channel would receive, without touching
// Tunarr (§12). It runs the SAME assembler and the SAME seed as reconcile, so preview
// and reality cannot disagree — a preview that could drift from what actually ships
// would be worse than none.
func (s *Server) previewChannelPods(ctx context.Context, in *previewPodsInput) (*previewPodsOutput, error) {
	if s.pods == nil {
		return nil, huma.Error501NotImplemented("filler pod assembly not configured")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such channel")
	} else if err != nil {
		return nil, err
	}

	pod, err := s.pods.Preview(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	out := &previewPodsOutput{}
	// Always a slice, never null: an empty catalog is a normal state the UI renders as
	// "no clips yet", and a null here would make the FE guard a case that isn't an error.
	out.Body.Entries = make([]PodEntryDTO, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		out.Body.Entries = append(out.Body.Entries, PodEntryDTO{
			TunarrProgramID: e.TunarrProgramID,
			Name:            e.Name,
			Kind:            string(e.Kind),
			DurationMs:      e.DurationMs,
			IsFallbackCard:  e.IsFallbackCard,
		})
	}
	out.Body.TotalMs = pod.TotalMs
	out.Body.MatchLevel = string(pod.MatchLevel)
	return out, nil
}
