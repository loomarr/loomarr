import type { Ref } from "react";

import type { StripAiring } from "./series-runs";

/** What a viewer is pointing at or has focused, or pressed. */
interface FilmStripLook {
  /** The programme the facts describe: the one airing now in a run, else its first. */
  airing: StripAiring;
  /** Undefined where the run may go on past the airings supplied. */
  episodes: number | undefined;
  startMs: number;
  stopMs: number;
}

interface FilmStripProps {
  /**
   * The channel's airings, wider than `window` where the caller can. A series run that touches the
   * first or last programme here is open and shows no episode count, since the run may go on.
   */
  airings: readonly StripAiring[];
  /** Where an airing's picture comes from; a client with a server base URL resolves it here. */
  artworkUri?: (airing: StripAiring) => string | undefined;
  /** A clock time. For a programme after midnight the caller's formatter adds the day. */
  formatTime: (ms: number) => string;
  /**
   * `strip` is the strip under the picture: words under each still, and a block's width follows its
   * length between the two limits. `row` is a Guide row: words inside, and true to the shared clock.
   */
  layout?: "row" | "strip";
  /** The widest a block is drawn in `strip` layout. */
  maxBlockWidth?: number;
  /** The narrowest a block is drawn in `strip` layout. */
  minBlockWidth?: number;
  /** Live: what has aired is everything before it. */
  nowMs: number;
  /** A pointer or focus moved onto a programme, or off every one (undefined). */
  onLook?: (look: FilmStripLook | undefined) => void;
  /** A programme was pressed: a click, Enter, or a television's OK. */
  onSelect?: (look: FilmStripLook) => void;
  /** Where the viewer is. Behind `nowMs`, the live mark shows too. */
  playheadMs?: number;
  ref?: Ref<FilmStripHandle> | undefined;
  /**
   * The span the strip draws. In `row` layout a block is as wide as its share of it; in `strip`
   * layout it picks the programmes and trims what has aired before it.
   */
  window: { startMs: number; stopMs: number };
}

interface FilmStripHandle {
  /**
   * Moves focus onto the programme `by` steps from the one on now, for a key that reached the strip
   * from outside it; at either end it stops on the last there is.
   */
  focusFromNow: (by: -1 | 1) => void;
}

export type { FilmStripHandle, FilmStripLook, FilmStripProps };
