import type { StripAiring } from "./series-runs";

/** What a viewer is pointing at or has focused, or pressed. */
interface FilmStripLook {
  /** The programme the facts describe: the one airing now in a run, else its first. */
  airing: StripAiring;
  episodes: number;
  startMs: number;
  stopMs: number;
}

interface FilmStripProps {
  airings: readonly StripAiring[];
  /** Where an airing's picture comes from; a client with a server base URL resolves it here. */
  artworkUri?: (airing: StripAiring) => string | undefined;
  formatTime: (ms: number) => string;
  /** `strip` sets the words under each still; `row` sets them inside it, as a Guide row does. */
  layout?: "row" | "strip";
  /** Live: what has aired is everything before it. */
  nowMs: number;
  /** A pointer or focus moved onto a programme, or off every one (undefined). */
  onLook?: (look: FilmStripLook | undefined) => void;
  /** A programme was pressed: a click, Enter, or a television's OK. */
  onSelect?: (look: FilmStripLook) => void;
  /** Where the viewer is. Behind `nowMs`, the live mark shows too. */
  playheadMs?: number;
  /** The span the strip draws; a block is as wide as its share of it. */
  window: { startMs: number; stopMs: number };
}

export type { FilmStripLook, FilmStripProps };
