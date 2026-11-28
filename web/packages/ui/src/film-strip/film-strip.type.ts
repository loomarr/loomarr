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
  /**
   * Hands the cursor to the owner. The named block (a `groupSeriesRuns` id) is the Tab stop and the
   * only one that is ever amber (while keys hold it; a plain ring otherwise), none when `id` is
   * undefined, and Left, Right, Home and End are the owner's to handle. Focus follows a new `id` only while the strip holds focus, or when the owner
   * sets `takeFocus` as it moves its own focus in. Left out, the strip runs its own cursor.
   */
  cursor?: { id: string | undefined; takeFocus?: boolean };
  /** Words that end every block's name, such as the channel the strip is a row of. */
  blockContext?: string;
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
  /**
   * The span the strip draws. In `row` layout a block is as wide as its share of it; in `strip`
   * layout it picks the programmes and trims what has aired before it.
   */
  window: { startMs: number; stopMs: number };
}

export type { FilmStripLook, FilmStripProps };
