import { createContext, useContext } from "react";

// HoldControlsContext lets a control INSIDE the player's bar keep the overlay controls from
// auto-hiding while it is interacting (§9.1 V47). The problem it solves: a menu (audio/subtitles)
// opens from the bar, but the player's hide timer keeps ticking and hides the whole bar — pulling
// the open menu's trigger out from under it. A control calls `hold(true)` when it opens and
// `hold(false)` when it closes; while any control holds, the player forces the controls shown.
//
// A context rather than a prop so the player stays generic: it never learns what the control IS,
// only that something is holding. The default is a no-op (a control used outside a player just does
// nothing extra).
interface HoldControls {
  hold: (held: boolean) => void;
}

const HoldControlsContext = createContext<HoldControls>({ hold: () => {} });

const useHoldControls = () => useContext(HoldControlsContext);

export type { HoldControls };
export { HoldControlsContext, useHoldControls };
