// PROTOTYPE — throwaway (#2151 step 2). The contract every /prototype/library variant renders
// against: one screen, one scan state and one device frame at a time, all forced from the URL.
//
// Decisions 22–35 of #2151 fixed WHAT exists (Library nav: Items · Unmatched · Collections;
// Settings → Library; item view with provenance; progress as a stage stepper + per-item state).
// The variants disagree only about the PAGE SHAPE: how list, detail and review fit on screen.

const SCREENS = [
  "items", // Library → Items: movies + series table
  "item-series", // the item view for a series (The West Wing), episodes inside
  "item-movie", // the item view for a movie with a stray copy (Fargo)
  "provenance", // the item view with a field's provenance open ("why TV-14?")
  "unmatched", // Library → Unmatched: candidates waiting for the maintainer
  "collections", // Library → Collections: TMDB (read-only) + Loomarr (editable)
  "collection-edit", // editing one Loomarr collection
  "settings", // Settings → Library (same in every variant)
  "wizard", // the setup wizard's Library step (same in every variant)
] as const;
type Screen = (typeof SCREENS)[number];

// What the scanner is doing right now. Drives the health strip, the nav progress bar and the
// per-item facts states — every screen reflects it.
const SCAN_STATES = [
  "idle", // up to date: last walk 12m ago, live changes healthy
  "importing", // first import: reading headers 12,400 / 33,135 (~8 min)
  "paused", // importing, but yielded while a channel plays
  "offline-root", // /media/movies unreadable (NAS off): nothing marked missing
  "held-removal", // a completed walk would remove 2,406 items: held for Confirm / Rescan
  "fresh", // a new install: roots found but not confirmed yet, nothing imported
] as const;
type ScanState = (typeof SCAN_STATES)[number];

const DEVICES = ["desktop", "phone"] as const;
type Device = (typeof DEVICES)[number];

interface VariantProps {
  screen: Screen;
  scan: ScanState;
  device: Device;
  // A variant may move between its own screens (row click → item view etc.).
  go: (screen: Screen) => void;
}

export type { Device, ScanState, Screen, VariantProps };
export { DEVICES, SCAN_STATES, SCREENS };
