// The timing of The Cross's tune (frontend-design §9.4), the same on every client. Pure: a player
// feeds it flips and "the picture is ready" with the time each happened, asks what to draw now, and
// sets a timer for `nextAt`. It starts no timer and makes no sound.

/** How long a lone flip's static is at full strength. */
const TUNE_BURST_MS = 150;
/** When the cover is gone, counted from the last flip. */
const TUNE_UNCOVER_MS = 220;
/** The whole of a flip: the seven bars run once inside it, and locked bars last as long. */
const TUNE_FLIP_MS = 340;
/** No more covers than this start in any second, so nothing flashes faster. */
const TUNE_MAX_COVERS_PER_SECOND = 3;

/** `changing`: the next channel is already streaming. `tuning-in`: there is a real wait. */
type TuneKind = "changing" | "tuning-in";

type TuneCover = "none" | "burst" | "fading" | "wash" | "cold";
type TuneBarsPhase = "none" | "run" | "waiting" | "locked";

type TuneFlip = {
  at: number;
  firstTune?: boolean;
  streaming: boolean;
};

type TuneState = {
  /** Start times of the latest covers, oldest first. */
  covers: readonly number[];
  /** Counts flips, so a part can run the bars again for each one. */
  flips: number;
  kind: TuneKind;
  lastFlipAt: number | null;
  readyAt: number | null;
  /** A flip landed on a cover that was still up. */
  washed: boolean;
};

type TuneView = {
  bars: TuneBarsPhase;
  cover: TuneCover;
  flip: number;
  /** When the view next changes on its own, or null while it waits for an event. */
  nextAt: number | null;
};

const idleTune: TuneState = {
  covers: [],
  flips: 0,
  kind: "changing",
  lastFlipAt: null,
  readyAt: null,
  washed: false,
};

const tuneKind = ({ firstTune, streaming }: Omit<TuneFlip, "at">): TuneKind =>
  streaming && !firstTune ? "changing" : "tuning-in";

// The latest of a full set of covers stays up until a second after the oldest, so the next one cannot
// be one too many.
const ceilingUntil = ({ covers }: TuneState) =>
  covers.length < TUNE_MAX_COVERS_PER_SECOND ? 0 : (covers[0] ?? 0) + 1000;

const clearAt = (state: TuneState) => {
  if (state.lastFlipAt === null) return 0;
  if (state.kind === "changing") return Math.max(state.lastFlipAt + TUNE_UNCOVER_MS, ceilingUntil(state));
  if (state.readyAt === null) return Number.POSITIVE_INFINITY;
  return Math.max(state.readyAt, ceilingUntil(state));
};

const tuneFlip = (state: TuneState, flip: TuneFlip): TuneState => {
  const covered = state.lastFlipAt !== null && flip.at < clearAt(state);
  return {
    covers: covered ? state.covers : [...state.covers, flip.at].slice(-TUNE_MAX_COVERS_PER_SECOND),
    flips: state.flips + 1,
    kind: tuneKind(flip),
    lastFlipAt: flip.at,
    readyAt: null,
    washed: covered,
  };
};

const tunePictureReady = (state: TuneState, at: number): TuneState =>
  state.kind === "tuning-in" && state.lastFlipAt !== null && state.readyAt === null
    ? { ...state, readyAt: at }
    : state;

const nextAfter = (now: number, times: readonly number[]) => {
  const ahead = times.filter((time) => time > now && Number.isFinite(time));
  return ahead.length ? Math.min(...ahead) : null;
};

const tuneView = (state: TuneState, now: number): TuneView => {
  const { flips: flip, lastFlipAt, readyAt } = state;
  if (lastFlipAt === null) return { bars: "none", cover: "none", flip, nextAt: null };
  const clear = clearAt(state);

  if (state.kind === "tuning-in") {
    if (readyAt === null) return { bars: "waiting", cover: "cold", flip, nextAt: null };
    const unlocked = readyAt + TUNE_FLIP_MS;
    return {
      bars: now < unlocked ? "locked" : "none",
      cover: now < clear ? "cold" : "none",
      flip,
      nextAt: nextAfter(now, [clear, unlocked]),
    };
  }

  const fadeAt = lastFlipAt + TUNE_BURST_MS;
  const ran = lastFlipAt + TUNE_FLIP_MS;
  const held = state.washed || clear > lastFlipAt + TUNE_UNCOVER_MS;
  const covering = held ? "wash" : now < fadeAt ? "burst" : "fading";
  return {
    bars: now < ran ? "run" : "none",
    cover: now < clear ? covering : "none",
    flip,
    nextAt: nextAfter(now, held ? [clear, ran] : [fadeAt, clear, ran]),
  };
};

export type { TuneBarsPhase, TuneCover, TuneFlip, TuneKind, TuneState, TuneView };
export {
  idleTune,
  TUNE_BURST_MS,
  TUNE_FLIP_MS,
  TUNE_MAX_COVERS_PER_SECOND,
  TUNE_UNCOVER_MS,
  tuneFlip,
  tuneKind,
  tunePictureReady,
  tuneView,
};
