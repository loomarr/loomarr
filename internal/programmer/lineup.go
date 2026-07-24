package programmer

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mantonx/loomarr/internal/schedule"
)

// This file translates the domain []schedule.Slot to/from Tunarr's manual-lineup
// envelope (§9). Tunarr's programming POST is {type:"manual", lineup:[…]} where
// each item is a oneOf: content (a real program: id + duration), flex (dead-time
// padding), or custom (a custom-show ref). Our slot kinds map:
//
//	SlotProgram → content (LibraryItemID + DurationMs)
//	SlotFlex    → flex
//	SlotFiller  → content when it carries a LibraryItemID (a resolved clip),
//	              else flex (an un-filled pod placeholder — §10 fills it later).
//	SlotPending → flex (nothing to play yet; keeps the timeline continuous).
//
// The "never dead air" rule (§9) is satisfied because every non-program slot
// becomes flex at worst, so Tunarr always has something to broadcast.

// tunarrLineupItem is one entry in the manual-lineup array. The fields present
// depend on Type; JSON omitempty keeps content-only fields off flex items.
type tunarrLineupItem struct {
	Type          string  `json:"type"` // "content" | "flex" | "custom"
	Duration      float64 `json:"duration"`
	ID            string  `json:"id,omitempty"`
	StartOffsetMs float64 `json:"startOffsetMs,omitempty"`
}

type setLineupBody struct {
	Type   string             `json:"type"` // "manual"
	Lineup []tunarrLineupItem `json:"lineup"`
}

// lineupResponse is the GET /programming shape — a bare array of items in Tunarr;
// we read back type/duration/id to reconstruct slots.
type lineupResponse struct {
	Lineup []tunarrLineupItem `json:"lineup"`
}

// SetLineup implements Programmer: translates slots to a manual lineup and POSTs
// it. An empty slice clears the channel (sends an empty manual lineup) rather
// than erroring — desired-vs-actual may legitimately want an empty channel while
// content is still being acquired (the reconcile pads with flex upstream, so in
// practice slots is non-empty, but SetLineup itself must be total).
func (t *Tunarr) SetLineup(ctx context.Context, tunarrID string, slots []schedule.Slot) error {
	items := make([]tunarrLineupItem, 0, len(slots))
	for _, s := range slots {
		item, err := t.slotToItem(ctx, s)
		if err != nil {
			return fmt.Errorf("resolve slot for %s: %w", tunarrID, err)
		}
		items = append(items, item)
	}
	body := setLineupBody{Type: "manual", Lineup: items}
	if err := t.doJSON(ctx, http.MethodPost, "/api/channels/"+tunarrID+"/programming", body, nil); err != nil {
		return fmt.Errorf("set lineup for %s: %w", tunarrID, err)
	}
	return nil
}

// GetLineup implements Programmer: reads current programming back as slots. It
// absorbs Tunarr's 400-on-empty-lineup quirk (Phase-0 finding 4) by treating a
// 400/404 as "no programming yet" → empty slice, so the diff sees "actual is
// empty" rather than an error on a freshly-created channel.
func (t *Tunarr) GetLineup(ctx context.Context, tunarrID string) ([]schedule.Slot, error) {
	var resp lineupResponse
	status, snippet, err := t.doStatus(ctx, t.http, http.MethodGet,
		"/api/channels/"+tunarrID+"/programming", nil, &resp)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusBadRequest || status == http.StatusNotFound:
		return []schedule.Slot{}, nil // unprogrammed channel (Phase-0 finding 4)
	case status < 200 || status >= 300:
		return nil, statusErr("get lineup for "+tunarrID, status, snippet)
	}
	slots := make([]schedule.Slot, 0, len(resp.Lineup))
	for _, it := range resp.Lineup {
		slots = append(slots, itemToSlot(it))
	}
	return slots, nil
}

// slotToItem maps one domain slot to a Tunarr lineup item, resolving the slot's
// media-server item id → Tunarr program uuid (§6). A content slot whose item
// isn't in Tunarr's index yet (unscanned / just-landed) degrades to flex rather
// than failing the whole push — never dead air (§9); it resolves on a later
// reconcile once Tunarr has scanned it.
// minFlexMs is the floor for a flex slot's duration. Tunarr's manual-programming
// API rejects any lineup item with duration ≤ 0 ("Too small: expected number to
// be >0"), and a pending/unresolved slot often has DurationMs 0 (its runtime isn't
// known yet). Flooring flex at 30s keeps the push valid — the filler-list fills the
// gap regardless, and a later reconcile replaces it with the real program once it
// lands. (Caught by the live smoke: a flex slot with duration 0 → 400.)
const minFlexMs = 30_000

func (t *Tunarr) slotToItem(ctx context.Context, s schedule.Slot) (tunarrLineupItem, error) {
	dur := float64(s.DurationMs)
	// Only a program is content. Filler is no longer inlined here (§10 redesign):
	// commercials live in a Tunarr filler-list attached to the channel, which
	// Tunarr plays into these flex gaps — so SlotFiller, SlotPending, SlotFlex all
	// render as flex (never dead air, §9).
	if s.Kind == schedule.SlotProgram {
		return t.contentItem(ctx, s.LibraryItemID, dur)
	}
	return flexItem(dur), nil
}

// flexItem builds a flex lineup entry with a duration Tunarr will accept (≥ minFlexMs).
func flexItem(dur float64) tunarrLineupItem {
	if dur < minFlexMs {
		dur = minFlexMs
	}
	return tunarrLineupItem{Type: "flex", Duration: dur}
}

// contentItem resolves a media-server item id to a Tunarr content entry, or
// degrades to flex if Tunarr hasn't indexed the item yet.
func (t *Tunarr) contentItem(ctx context.Context, itemID string, dur float64) (tunarrLineupItem, error) {
	uuid, ok, err := t.resolver.resolve(ctx, itemID)
	if err != nil {
		return tunarrLineupItem{}, err
	}
	if !ok {
		// Not in Tunarr's index yet → flex now, resolves on a later reconcile.
		return flexItem(dur), nil
	}
	return tunarrLineupItem{Type: "content", ID: uuid, Duration: dur}, nil
}

// itemToSlot maps a Tunarr lineup item back to a domain slot for the diff. Tunarr
// doesn't round-trip our provisioning Key (it has no field for it), so read-back
// slots carry Kind + LibraryItemID + duration only; the diff compares on those.
func itemToSlot(it tunarrLineupItem) schedule.Slot {
	dur := int64(it.Duration)
	switch it.Type {
	case "content":
		return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: it.ID, DurationMs: dur}
	default: // "flex", "custom", unknown
		return schedule.Slot{Kind: schedule.SlotFlex, DurationMs: dur}
	}
}
