package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// The channel WIRE SHAPE and the mapping to and from it (§7.1).
//
// Split out of channels.go: translation is a different job from handling, and keeping it here
// means a DTO change is reviewable without reading past fifteen handlers.
//
// The direction of the mapping matters. `channelToDTO` is a projection — it summarises the
// desired lineup as counts rather than dumping every slot, because a channel resource is not a
// schedule. `mergeLineupEdit` goes the other way and is deliberately a MERGE rather than a
// replace: an edit carries only what the operator changed, so a naive overwrite would silently
// discard fields the UI never sent.

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

// registerChannels mounts /v1/channels* (§7). Reads are visible to any
// authenticated user; create/update/delete/reconcile require admin.
