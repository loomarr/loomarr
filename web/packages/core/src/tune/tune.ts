// The timing of The Cross's tune (frontend-design §9.4), the same on every client. Pure: a player
// feeds it flips and what became of their pictures, asks `tuneView` what to draw now, and sets a
// timer for `nextAt`. It starts no timer and makes no sound.
//
// Driving it: when the timer fires, read the view at `Math.max(now, nextAt)`. A timer may fire a
// millisecond early, and a view read before `nextAt` names the same `nextAt` again, so a caller
// waiting for it to change is never woken.

/** How long a lone flip's static is at full strength. */
const TUNE_BURST_MS = 150;
/** When the cover is gone, counted from the last flip. */
const TUNE_UNCOVER_MS = 220;
/** The whole of a flip: the seven bars run once inside it, and locked bars last as long. */
const TUNE_FLIP_MS = 340;
/** Warm flips start no more covers than this in any second, so nothing flashes faster. */
const TUNE_MAX_COVERS_PER_SECOND = 3;
const COVER_GAP_MS = Math.ceil(1000 / TUNE_MAX_COVERS_PER_SECOND);

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
  /** The tune-in ended with no picture. */
  abandoned: boolean;
  /** When the latest cover went up. */
  coveredAt: number | null;
  /** The current flip's identity. */
  flips: number;
  kind: TuneKind;
  lastFlipAt: number | null;
  readyAt: number | null;
  /** When the latest run of the bars began, or null when the latest flip waits instead. */
  runAt: number | null;
  /** Counts the runs, so a part knows when to run the bars again. */
  runs: number;
  /** `bare`: no static. `wash`: the flip landed on a cover that was still up. */
  treatment: "bare" | "burst" | "wash";
};

type TuneView = {
  bars: TuneBarsPhase;
  cover: TuneCover;
  /** The current flip's identity, to hand back to `tunePictureReady` or `tuneAbandon`. */
  flip: number;
  /** Which run of the bars this is: a flip that lands during a run leaves it to finish. */
  run: number;
  /** When the view next changes on its own, or null while it waits for an event. */
  nextAt: number | null;
};

const idleTune: TuneState = {
  abandoned: false,
  coveredAt: null,
  flips: 0,
  kind: "changing",
  lastFlipAt: null,
  readyAt: null,
  runAt: null,
  runs: 0,
  treatment: "bare",
};

const tuneKind = ({ firstTune, streaming }: Omit<TuneFlip, "at">): TuneKind =>
  streaming && !firstTune ? "changing" : "tuning-in";

const coveredAtTime = (state: TuneState, at: number) => {
  if (state.lastFlipAt === null || state.treatment === "bare") return false;
  if (state.kind === "changing") return at < state.lastFlipAt + TUNE_UNCOVER_MS;
  return state.readyAt === null && !state.abandoned;
};

const tuneFlip = (state: TuneState, flip: TuneFlip): TuneState => {
  // A flip dated before the last one means the caller's clock was reset. Nothing earlier can be
  // measured against it, so it starts afresh and cannot leave the picture covered.
  const history = state.lastFlipAt !== null && flip.at >= state.lastFlipAt;
  const kind = tuneKind(flip);
  const joins = history && coveredAtTime(state, flip.at);
  // A streaming picture this soon after the last cover is shown bare: covering it would flash too
  // fast. A tune-in has no picture to show, so it always covers.
  const bare =
    !joins &&
    history &&
    kind === "changing" &&
    state.coveredAt !== null &&
    flip.at - state.coveredAt < COVER_GAP_MS;
  // A run in progress finishes, so a held flip key cannot restart the bars at its repeat rate.
  const running = history && state.runAt !== null && flip.at < state.runAt + TUNE_FLIP_MS;
  const runs = kind === "changing" && !running;

  return {
    abandoned: false,
    coveredAt: joins || bare ? state.coveredAt : flip.at,
    flips: state.flips + 1,
    kind,
    lastFlipAt: flip.at,
    readyAt: null,
    runAt: kind === "tuning-in" ? null : runs ? flip.at : state.runAt,
    runs: state.runs + (runs ? 1 : 0),
    treatment: joins ? "wash" : bare ? "bare" : "burst",
  };
};

const awaits = (state: TuneState, flip: number) =>
  state.kind === "tuning-in" && state.flips === flip && state.readyAt === null && !state.abandoned;

/** The picture for `flip` has arrived. Does nothing unless that flip is the tune-in being waited on. */
const tunePictureReady = (state: TuneState, flip: number, at: number): TuneState =>
  awaits(state, flip) ? { ...state, readyAt: Math.max(at, state.lastFlipAt ?? at) } : state;

/** `flip` will have no picture (playback failed, the channel went off air): uncover, lock nothing. */
const tuneAbandon = (state: TuneState, flip: number): TuneState =>
  awaits(state, flip) ? { ...state, abandoned: true } : state;

const nextAfter = (now: number, times: readonly number[]) => {
  const ahead = times.filter((time) => time > now);
  return ahead.length ? Math.min(...ahead) : null;
};

/** What to draw at `now`. After a timer, pass `Math.max(now, nextAt)`. */
const tuneView = (state: TuneState, now: number): TuneView => {
  const { flips: flip, lastFlipAt, readyAt, runs: run } = state;
  const clear: TuneView = { bars: "none", cover: "none", flip, nextAt: null, run };
  if (lastFlipAt === null || state.abandoned) return clear;

  if (state.kind === "tuning-in") {
    if (readyAt === null) return { ...clear, bars: "waiting", cover: "cold" };
    const unlocked = readyAt + TUNE_FLIP_MS;
    return now < unlocked ? { ...clear, bars: "locked", nextAt: unlocked } : clear;
  }

  const fadeAt = lastFlipAt + TUNE_BURST_MS;
  const clearAt = lastFlipAt + TUNE_UNCOVER_MS;
  const ran = (state.runAt ?? lastFlipAt) + TUNE_FLIP_MS;
  const bars = now < ran ? "run" : "none";
  if (state.treatment === "bare") return { ...clear, bars, nextAt: nextAfter(now, [ran]) };

  const washed = state.treatment === "wash";
  const covering = washed ? "wash" : now < fadeAt ? "burst" : "fading";
  return {
    ...clear,
    bars,
    cover: now < clearAt ? covering : "none",
    nextAt: nextAfter(now, washed ? [clearAt, ran] : [fadeAt, clearAt, ran]),
  };
};

export type { TuneBarsPhase, TuneCover, TuneFlip, TuneKind, TuneState, TuneView };
export {
  idleTune,
  TUNE_BURST_MS,
  TUNE_FLIP_MS,
  TUNE_MAX_COVERS_PER_SECOND,
  TUNE_UNCOVER_MS,
  tuneAbandon,
  tuneFlip,
  tuneKind,
  tunePictureReady,
  tuneView,
};
