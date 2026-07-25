package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/filler"
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
	Logo         string `json:"logo,omitempty" doc:"Channel icon URL — pushed to Tunarr's channel icon (from TMDB, an upload, or set directly)"`
	Strategy     string `json:"strategy" enum:"sequential,shuffle,time_slot"`
	Status       string `json:"status" enum:"building,live,drifted,detached,paused" doc:"Loomarr-side channel status (§9)"`
	TunarrID     string `json:"tunarrId,omitempty" doc:"Server-assigned Tunarr channel id; empty until first reconcile"`
	IntentRef    string `json:"intentRef,omitempty"`
	ProgramCount int    `json:"programCount" doc:"Real playable programs (available titles) in the desired lineup"`
	PendingCount int    `json:"pendingCount" doc:"Lineup titles not yet available — awaiting acquisition (coming-soon gaps + pod-fill placeholders). Health keys on this: pendingCount==0 means every title is ready, even on a channel full of commercial breaks."`
	BreakCount   int    `json:"breakCount" doc:"Commercial-break gaps (§10) — NOT titles; a healthy break-heavy channel has a large breakCount and zero pendingCount"`
	SlotCount    int    `json:"slotCount" doc:"Total desired slots incl. breaks + placeholders. NOT a readiness signal — use programCount/pendingCount (a break gap inflates this without any title pending). Kept for diagnostics."`
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
	Key  string `json:"key" doc:"Provisioning key, e.g. movie:tmdb:603"`
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
	// State is the entry's acquisition/availability, resolved from the provision Record per
	// key so the "not here yet" badge survives a reload (§7). Only the single-channel GET
	// populates it (the list omits it to avoid an N-query fan-out); "" ⇒ not resolved.
	State  string   `json:"state,omitempty" enum:"available,acquiring,pending,unavailable" doc:"available = in the library, plays now; acquiring = wanted/requested/downloading; pending = added but nothing has requested it yet; unavailable = acquisition gave up."`
	Genres []string `json:"genres,omitempty"`
	// SeasonMin/SeasonMax surface a series entry's airing season window (§8/§9) so the
	// UI can show and edit "seasons 1–10". 0 = unbounded on that end (all seasons).
	SeasonMin int `json:"seasonMin,omitempty" doc:"series airing window: first season (0 = unbounded)"`
	SeasonMax int `json:"seasonMax,omitempty" doc:"series airing window: last season (0 = unbounded)"`
	// Acquisition progress (§18.1), surfaced on an `acquiring` entry so the UI can show how
	// far along a download is. Progress is a 0..1 fraction from the direct-arr queue poll; on
	// the Seerr path it stays 0 and the meaning rides DownloadStatus ("Downloading" / "Partly
	// available"), which the UI renders as an indeterminate indicator. Empty/zero once
	// available or on a pending entry with no record yet.
	Progress       float64 `json:"progress,omitempty" doc:"Download completion 0..1 (arr queue poll); 0 = indeterminate on the Seerr path"`
	ETAText        string  `json:"etaText,omitempty" doc:"Human time-left from the download client (direct arr only)"`
	DownloadStatus string  `json:"downloadStatus,omitempty" doc:"Coarse acquisition status label (e.g. Downloading, Partly available) or the arr download-client status"`
}

// lineupEntryState collapses the provision.Record lifecycle (plus the no-record case) into
// the small UI-facing vocabulary the lineup badge needs. A key with NO record is `pending`,
// not `unavailable`: a manually-added title has a lineup entry but no Record until the
// acquisition pipeline creates one, and "gave up" (unavailable) is a different thing from
// "nobody has asked for it yet".
func lineupEntryState(rec provision.Record, found bool) string {
	if !found {
		return "pending" // added to the lineup, but no acquisition Record exists yet
	}
	switch rec.State {
	case provision.Available:
		return "available"
	case provision.Wanted, provision.Requested, provision.Downloading:
		return "acquiring"
	case provision.Unavailable:
		return "unavailable"
	default:
		return "pending"
	}
}

// entryAcq is what the per-entry resolver returns: the collapsed lineup state plus the
// acquisition progress fields (§18.1) for an in-flight entry, so a single GetTitle populates
// both the badge and the progress indicator.
type entryAcq struct {
	State          string
	Progress       float64
	ETAText        string
	DownloadStatus string
}

// channelToDTO renders a channel for the API. entryState, when non-nil, resolves each
// lineup entry's acquisition state + progress by key (the single-channel handlers pass a
// store-backed resolver; the list passes nil to skip the per-entry lookups it does not need).
func channelToDTO(ch store.Channel, entryState func(provision.Key) entryAcq) ChannelDTO {
	d := schedule.DesiredLineup{Slots: ch.Desired}
	lineup := make([]LineupEntryDTO, 0, len(ch.Lineup))
	for _, e := range ch.Lineup {
		dto := LineupEntryDTO{
			Key: string(e.Key), Name: e.Title, Year: e.Year, Genres: e.Genres,
			SeasonMin: e.SeasonMin, SeasonMax: e.SeasonMax,
		}
		if entryState != nil {
			acq := entryState(e.Key)
			dto.State = acq.State
			dto.Progress, dto.ETAText, dto.DownloadStatus = acq.Progress, acq.ETAText, acq.DownloadStatus
		}
		lineup = append(lineup, dto)
	}
	return ChannelDTO{
		ID: ch.ID, Name: ch.Name, Number: ch.Number, Group: ch.Group, Logo: ch.Logo,
		Strategy: string(ch.Strategy), Status: string(ch.Status),
		TunarrID: ch.TunarrID, IntentRef: ch.IntentRef,
		ProgramCount: d.ProgramCount(), PendingCount: d.PendingCount(),
		BreakCount: d.BreakCount(), SlotCount: len(ch.Desired),
		Policy: ch.Policy, Lineup: lineup,
	}
}

// entryStateResolver returns a per-key acquisition-state lookup for a SINGLE channel's DTO
// (getChannel/createChannel/updateChannel/reconcileChannel). A channel has at most a few
// dozen lineup entries, so a GetTitle per key is cheap here — and this path is per-request,
// not a list fan-out. A key with no Record resolves to `pending` (lineupEntryState), so a
// manually-added, not-yet-requested title reads as pending durably rather than only at
// add-time. The list endpoint deliberately does NOT use this (it shows counts, not entries).
func (s *Server) entryStateResolver(ctx context.Context) func(provision.Key) entryAcq {
	return func(key provision.Key) entryAcq {
		rec, err := s.store.GetTitle(ctx, key)
		found := err == nil
		acq := entryAcq{State: lineupEntryState(rec, found)}
		// Surface progress only for an in-flight entry (acquiring). An available/pending/
		// unavailable entry has no meaningful live progress, and the record's stale
		// download fields (e.g. a lingering label after success) must not leak onto it.
		if found && acq.State == "acquiring" {
			acq.Progress, acq.ETAText, acq.DownloadStatus = rec.Progress, rec.ETAText, rec.DownloadStatus
		}
		return acq
	}
}

// mergeLineupEdit turns a whole-list lineup edit (the DTO shape the read side emits) into
// scheduler entries, for the PATCH path (§7). The read DTO is deliberately lossy — it
// drops a series' season range, the content rating, and the runtime cap — so a naive
// DTO→entry rebuild would silently reset those on an unrelated reorder. The fix: for each
// incoming entry, if its key already exists in the current lineup, carry that entry's rich
// metadata forward and only take the (possibly edited) name/year/genres from the DTO; a
// genuinely new key becomes a fresh entry (its rating/duration heal from the library at
// reconcile). Order follows the incoming list (add/remove/reorder in one payload). Every
// key is validated up front — a malformed key fails the whole edit rather than landing a
// junk entry. A key not `available` in the library is fine: it renders as a pending slot
// until the title lands (§9), so this never plays or acquires unapproved content.
func mergeLineupEdit(current []schedule.LineupEntry, edit []LineupEntryDTO) ([]schedule.LineupEntry, error) {
	incoming, err := lineupEntriesFromDTOs(edit)
	if err != nil {
		return nil, err
	}
	return schedule.ApplyLineup(current, incoming, schedule.LineupReplace,
		schedule.ApplyOpts{PreserveByKey: true}), nil
}

// lineupEntriesFromDTOs validates + converts the lossy edit DTOs into partial LineupEntries
// (display fields + an optional season window; rich scheduling metadata is carried forward
// from the current entry by ApplyLineup's PreserveByKey, §7). Shared by the PATCH merge and
// the programming/preview draft so a preview lowers the edit exactly as the save would.
func lineupEntriesFromDTOs(edit []LineupEntryDTO) ([]schedule.LineupEntry, error) {
	incoming := make([]schedule.LineupEntry, 0, len(edit))
	seen := make(map[provision.Key]struct{}, len(edit))
	for i, dto := range edit {
		key := provision.Key(strings.TrimSpace(dto.Key))
		if _, _, _, ok := provision.ParseKey(key); !ok {
			return nil, fmt.Errorf("entry %d: malformed key %q", i, dto.Key)
		}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("entry %d: duplicate key %q in lineup", i, dto.Key)
		}
		seen[key] = struct{}{}
		incoming = append(incoming, schedule.LineupEntry{
			Key: key, Title: dto.Name, Year: dto.Year, Genres: dto.Genres,
			SeasonMin: dto.SeasonMin, SeasonMax: dto.SeasonMax,
		})
	}
	return incoming, nil
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

type upcomingInput struct {
	ID    string `path:"id"`
	Limit int    `query:"limit" doc:"How many programs to return (now + upcoming), default 6, max 24"`
}

type upcomingOutput struct {
	Body struct {
		Upcoming []NowNextEntry `json:"upcoming" doc:"The program airing now (first, if any) then the next ones in airtime order; gaps skipped"`
	}
}

// channelUpcoming answers the Overview "what's on later" strip for ONE channel: the program
// airing now followed by the next few, from Tunarr's generated guide (§6 airtimes). Any
// authenticated user may read it (the guide is viewer-facing, §8.1). A channel with no guide
// yet (never reconciled, or a Tunarr hiccup) returns an empty list, not an error — the strip
// simply shows "nothing scheduled" rather than blanking the page.
func (s *Server) channelUpcoming(ctx context.Context, in *upcomingInput) (*upcomingOutput, error) {
	out := &upcomingOutput{}
	out.Body.Upcoming = []NowNextEntry{}

	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	if s.guide == nil || ch.TunarrID == "" {
		return out, nil // no guide reader, or never reconciled — nothing airing yet
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 6
	}
	if limit > 24 {
		limit = 24
	}
	entries, err := s.guide.Upcoming(ctx, ch.TunarrID, time.Now(), limit)
	if err != nil {
		return out, nil // a Tunarr hiccup must not blank the Overview
	}
	out.Body.Upcoming = entries
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
		OperationID: "channel-upcoming", Method: http.MethodGet, Path: "/v1/channels/{id}/upcoming",
		Summary:     "What's on this channel now and next",
		Description: "The program airing now (first) then the next few, in airtime order, from Tunarr's generated guide (§6 airtimes; gaps skipped). Powers the Overview 'what's on later' strip. Read-only — any authenticated user (§8.1 viewer-facing).",
		Tags:        []string{"channels"},
	}, s.channelUpcoming)

	huma.Register(api, huma.Operation{
		OperationID: "channel-icon-suggestions", Method: http.MethodGet, Path: "/v1/channels/{id}/icon-suggestions",
		Summary:     "Suggest channel icons from the lineup",
		Description: "Candidate poster images drawn from the channel's OWN lineup titles (§icon) — a Star Trek channel offers its five series' posters. Read-only, so any authenticated user may call it. 501 when TMDB isn't configured.",
		Tags:        []string{"channels"},
	}, s.channelIconSuggestions)

	huma.Register(api, huma.Operation{
		OperationID: "preview-channel-pods", Method: http.MethodGet, Path: "/v1/channels/{id}/pods",
		Summary:     "Preview the commercial pool this channel would get",
		Description: "Assembles the channel's SAVED filler pool WITHOUT touching Tunarr (§10, §12). Same code path and same seed as reconcile, so what you see is what the channel gets. Read-only, so any authenticated user may call it.",
		Tags:        []string{"channels", "filler"},
	}, s.previewChannelPods)

	huma.Register(api, huma.Operation{
		OperationID: "preview-channel-cycle", Method: http.MethodGet, Path: "/v1/channels/{id}/cycle",
		Summary:     "Preview what airs at a chosen time (curation rules)",
		Description: "The time-travel cycle preview (§8.1): what this channel would air at `at` (default now), and WHICH curation rule is active then. Runs the SAME pure lineup builder as reconcile — preview and reality cannot disagree — WITHOUT touching Tunarr or the store beyond a read. Makes first-match-by-priority rule resolution legible (\"at Saturday 9am, the Weekend TNG marathon rule is active\"). Read-only, so any authenticated user may call it; `at` may be past or future.",
		Tags:        []string{"channels"},
	}, s.previewChannelCycle)

	huma.Register(api, huma.Operation{
		OperationID: "preview-draft-channel-pods", Method: http.MethodPost, Path: "/v1/channels/{id}/pods/preview",
		Summary:     "Preview a draft filler selection without saving it",
		Description: "Assembles the pool a DRAFT filler selection would produce, without persisting it (§10, §12) — the live sandbox on the channel's Filler section. Same assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what will air once applied. Admin-only (an authoring tool); applying is a normal PATCH of policy.filler.",
		Tags:        []string{"channels", "filler"},
	}, s.previewDraftChannelPods)

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

// PodEntryDTO is one clip placed in the previewed pool, in play order.
type PodEntryDTO struct {
	Path            string `json:"path,omitempty" doc:"The clip's identity, relative to FILLER_DIR. Empty for the embedded fallback bumper card, which is not a file."`
	TunarrProgramID string `json:"tunarrProgramId,omitempty" doc:"Tunarr's program id for this clip, when Tunarr knows it. Empty for the fallback bumper card AND on installs without Tunarr — key on path, not this."`
	Name            string `json:"name"`
	Kind            string `json:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	DurationMs      int64  `json:"durationMs"`
	IsFallbackCard  bool   `json:"isFallbackCard" doc:"The embedded default bumper card — the bottom of the fallback ladder"`
	// Era and Quality are DISPLAY context for the guide's pod hover card (§12): they explain
	// why a break looks the way it does ("1994 · 480p" reads as an authentic capture rather
	// than a playback fault). Neither affects selection — see filler.Clip.Quality.
	Era     int    `json:"era,omitempty" doc:"The clip's tagged era (1994); 0 when untagged"`
	Quality string `json:"quality,omitempty" doc:"Resolution label from the probed video height (1080p, 480p); empty when unknown or audio-only"`
}

type previewPodsInput struct {
	ID string `path:"id"`
}

// PodPoolDTO is a channel's assembled filler pool (§10) — the break preview. Named (not an
// inline body) so the pods endpoints AND the programming/preview endpoint share one shape.
type PodPoolDTO struct {
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

type previewPodsOutput struct {
	Body PodPoolDTO
}

// previewChannelPods shows the filler pool a channel would receive, without touching
// Tunarr (§12). It runs the SAME assembler and the SAME seed as reconcile, so preview
// and reality cannot disagree — a preview that could drift from what actually ships
// would be worse than none.
func (s *Server) previewChannelPods(ctx context.Context, in *previewPodsInput) (*previewPodsOutput, error) {
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before previewing a channel's pods.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	pod, err := s.pods.Preview(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return podToPreviewOutput(pod), nil
}

// CycleSlotDTO is one slot of the previewed cycle (§8.1): what airs, in play order. A
// program carries its title + provisioning key + series identity + multi-part index; a break
// gap is `kind:"break"` (Tunarr owns the clip that fills it — the preview shows the gap).
type CycleSlotDTO struct {
	Kind  string `json:"kind" enum:"program,pending,break" doc:"program = a playable title; pending = an acquisition not yet available; break = a commercial gap"`
	Title string `json:"title,omitempty" doc:"Display label; empty for a break gap"`
	Key   string `json:"key,omitempty" doc:"Provisioning key of the title (empty for a break)"`
	// Part is the 1-based play order within a multi-part/franchise group (§5) — >0 when this
	// slot is part of a two-parter or a movie franchise kept together, 0 for a standalone.
	Part int `json:"part,omitempty" doc:"Play order within a multi-part or franchise group (0 = standalone)"`
	// Season / Episode are the episode's numbers for a series program, so the UI can show
	// "S1E5" alongside the episode title. 0 for a movie or when the media server doesn't
	// number the item. The show *name* is not here — the client maps Key → its lineup entry's
	// name (it already has the lineup), so the DTO stays lookup-free.
	Season  int `json:"season,omitempty" doc:"Series season number (0 = movie/unknown)"`
	Episode int `json:"episode,omitempty" doc:"Series episode number (0 = movie/unknown)"`
}

// ActiveRuleDTO attributes the previewed cycle to the curation rule active at the previewed
// moment (§8.1) — the answer to "which rule is playing right now". Matched=false means no
// rule matched and the channel is on its base whole-policy behavior (label "Base policy").
type ActiveRuleDTO struct {
	ID       string `json:"id" doc:"Stable rule id ('' when no rule matched)"`
	Label    string `json:"label" doc:"Human-readable rule name, or 'Base policy' when none matched"`
	Priority int    `json:"priority" doc:"The rule's priority (higher wins overlaps); 0 for the base policy"`
	Matched  bool   `json:"matched" doc:"Whether a curation rule matched (false = base whole-policy behavior)"`
}

type previewCycleInput struct {
	ID string `path:"id"`
	At string `query:"at" doc:"RFC3339 wall-clock to preview (default: now). May be past or future — 'what airs next Christmas morning?'"`
}

type previewCycleOutput struct {
	Body struct {
		At         string         `json:"at" doc:"The resolved wall-clock this preview was computed for (RFC3339)"`
		ActiveRule ActiveRuleDTO  `json:"activeRule"`
		WindowMs   int64          `json:"windowMs" doc:"Resolved rolling-window horizon in ms (0 = the whole run, no truncation)"`
		Slots      []CycleSlotDTO `json:"slots" doc:"The leading slots of the resolved cycle, in play order (capped)"`
	}
}

// cyclePreviewSlotCap bounds how many slots the preview returns — enough to see intermixing,
// marathons, and franchise/two-parter adjacency at a glance without shipping an 800-item deck.
const cyclePreviewSlotCap = 50

// previewChannelCycle answers "what airs at this moment, and which rule is active" (§8.1). It
// runs the SAME pure lineup builder as reconcile at the chosen wall-clock, so the preview and
// the real guide cannot disagree — WITHOUT touching Tunarr or persisting anything. Read-only,
// so any authenticated user may call it; the moment may be past or future.
func (s *Server) previewChannelCycle(ctx context.Context, in *previewCycleInput) (*previewCycleOutput, error) {
	if s.channels == nil {
		return nil, errNotImplemented("Scheduling isn't set up", "Connect Tunarr in Settings → Connections to preview a channel's cycle.")
	}
	at := time.Time{} // zero ⇒ the engine uses "now"
	if strings.TrimSpace(in.At) != "" {
		t, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			return nil, errBadRequest("Invalid time", "`at` must be an RFC3339 timestamp like 2026-12-25T09:00:00Z.")
		}
		at = t
	}

	resolvedAt, slots, active, window, err := s.channels.CyclePreview(ctx, in.ID, at)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	out := &previewCycleOutput{}
	out.Body.At = resolvedAt.UTC().Format(time.RFC3339)
	out.Body.ActiveRule = ActiveRuleDTO{ID: active.ID, Label: active.Label, Priority: active.Priority, Matched: active.Matched}
	out.Body.WindowMs = window.Milliseconds()
	out.Body.Slots = cycleSlotsToDTO(slots, cyclePreviewSlotCap)
	return out, nil
}

// cycleSlotsToDTO maps the resolved cycle's slots to the preview DTO, capped at `limit`.
// Only program/pending/break kinds are meaningful to the preview; a filler slot with no clip
// yet renders as a "break" gap (that's what the guide shows). Franchise/multi-part order is
// preserved via Part.
func cycleSlotsToDTO(slots []schedule.Slot, limit int) []CycleSlotDTO {
	out := make([]CycleSlotDTO, 0, min(len(slots), limit))
	for _, sl := range slots {
		if len(out) >= limit {
			break
		}
		dto := CycleSlotDTO{Title: sl.Title, Part: sl.PartIndex}
		switch sl.Kind {
		case schedule.SlotProgram:
			dto.Kind = "program"
			dto.Key = string(sl.Key)
			dto.Season, dto.Episode = sl.Season, sl.Episode
		case schedule.SlotPending:
			dto.Kind = "pending"
			dto.Key = string(sl.Key)
		default: // SlotFiller / flex → a commercial break gap
			dto.Kind = "break"
			dto.Title = ""
		}
		out = append(out, dto)
	}
	return out
}

// podToPreviewOutput renders an assembled pod into the preview response — shared by the
// saved (GET) and draft (POST) preview handlers so their shapes cannot drift.
func podToPreviewOutput(pod filler.Pod) *previewPodsOutput {
	return &previewPodsOutput{Body: podToPoolDTO(pod)}
}

// podToPoolDTO maps an assembled pod to its DTO. Shared by the pods endpoints and the
// programming/preview endpoint (which embeds the pool alongside the cycle slots).
func podToPoolDTO(pod filler.Pod) PodPoolDTO {
	// Always a slice, never null: an empty catalog is a normal state the UI renders as
	// "no clips yet", and a null here would make the FE guard a case that isn't an error.
	entries := make([]PodEntryDTO, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		entries = append(entries, PodEntryDTO{
			Path:            e.Path,
			TunarrProgramID: e.TunarrProgramID,
			Name:            e.Name,
			Kind:            string(e.Kind),
			DurationMs:      e.DurationMs,
			IsFallbackCard:  e.IsFallbackCard,
			Era:             e.Era,
			Quality:         e.Quality,
		})
	}
	return PodPoolDTO{Entries: entries, TotalMs: pod.TotalMs, MatchLevel: string(pod.MatchLevel)}
}

type previewDraftPodsInput struct {
	ID   string `path:"id"`
	Body struct {
		// Filler is the DRAFT selection to preview (§10) — the unsaved selection the
		// sandbox is experimenting with. Same shape as policy.filler; validated like a
		// policy write, then assembled without persisting anything.
		Filler schedule.FillerSelection `json:"filler"`
	}
}

// previewDraftChannelPods assembles the pool a DRAFT filler selection would produce,
// without saving it (§10/§12 — the channel-page sandbox). Admin-only: it's an authoring
// tool (applying the selection is a normal PATCH of policy.filler). Runs the SAME
// assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what
// will air once applied — only the (draft) selection differs.
func (s *Server) previewDraftChannelPods(ctx context.Context, in *previewDraftPodsInput) (*previewPodsOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before previewing a channel's pods.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// Validate the draft with the same rules a policy write uses (bad audience/kind/
	// category/era → 422) — the sandbox must reject a nonsense selection, not assemble it.
	if err := (schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Filler: &in.Body.Filler}}).Validate(); err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid filler selection",
			"Some filler options are invalid. Check the audience, kinds, and categories, then try again.", err)
	}

	pod, err := s.pods.PreviewDraft(ctx, in.ID, fillerSelectionToDomain(in.Body.Filler))
	if err != nil {
		return nil, err
	}
	return podToPreviewOutput(pod), nil
}

// fillerSelectionToDomain translates the policy/DTO FillerSelection into the filler-
// package Selection the assembler consumes (the API's boundary translation, mirroring
// channels.SelectionForChannel but for a draft that isn't tied to a stored channel).
func fillerSelectionToDomain(f schedule.FillerSelection) filler.Selection {
	sel := filler.Selection{
		Audience:   filler.Audience(f.Audience),
		Categories: f.Categories,
		Kinds:      f.Kinds,
		Pinned:     f.Pinned,
		Excluded:   f.Excluded,
	}
	if f.Era != nil {
		sel.Era = f.Era.From
	}
	return sel
}
