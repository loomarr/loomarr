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
		items = append(items, slotToItem(s))
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
	status, err := t.doStatus(ctx, http.MethodGet,
		"/api/channels/"+tunarrID+"/programming", nil, &resp)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusBadRequest || status == http.StatusNotFound:
		return []schedule.Slot{}, nil // unprogrammed channel (Phase-0 finding 4)
	case status < 200 || status >= 300:
		return nil, fmt.Errorf("get lineup for %s: status %d", tunarrID, status)
	}
	slots := make([]schedule.Slot, 0, len(resp.Lineup))
	for _, it := range resp.Lineup {
		slots = append(slots, itemToSlot(it))
	}
	return slots, nil
}

// slotToItem maps one domain slot to a Tunarr lineup item.
func slotToItem(s schedule.Slot) tunarrLineupItem {
	dur := float64(s.DurationMs)
	switch s.Kind {
	case schedule.SlotProgram:
		return tunarrLineupItem{Type: "content", ID: s.LibraryItemID, Duration: dur}
	case schedule.SlotFiller:
		if s.LibraryItemID != "" {
			return tunarrLineupItem{Type: "content", ID: s.LibraryItemID, Duration: dur}
		}
		// Unresolved pod placeholder → flex padding until §10 fills it.
		return tunarrLineupItem{Type: "flex", Duration: dur}
	default: // SlotPending, SlotFlex, and any unknown kind → flex (never dead air)
		return tunarrLineupItem{Type: "flex", Duration: dur}
	}
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
