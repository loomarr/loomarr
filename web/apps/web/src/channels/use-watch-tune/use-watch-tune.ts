import {
  idleTune,
  type TuneState,
  type TuneView,
  tuneAbandon,
  tuneFlip,
  tunePictureReady,
  tuneView,
} from "@loomarr/core/tune";
import { useCallback, useEffect, useRef, useState } from "react";

interface WatchTune {
  /** What to draw now. */
  view: TuneView;
  /**
   * A tune began. `streaming` says the player had the channel warmed; `firstTune` that no picture
   * has played on this page yet. Returns the flip's identity.
   */
  flip: (flip: { streaming: boolean; firstTune: boolean }) => number;
  /** The picture for `flip` arrived: a tune-in waiting on it is uncovered now. */
  ready: (flip: number) => void;
  /** `flip` will have no picture: the tune-in ends without one. */
  abandon: (flip: number) => void;
}

/**
 * The tune (`@loomarr/core/tune`) for the web's watching screen, driven by what the player really
 * does: the caller says when a tune begins and when its picture arrives or fails, and the view is
 * read again at the time it names, never earlier than that time.
 */
const useWatchTune = (now: () => number = Date.now): WatchTune => {
  const state = useRef<TuneState>(idleTune);
  // A new object each time, so an event at the same millisecond still draws its state.
  const [read, setRead] = useState(() => ({ at: now() }));
  const update = useCallback(
    (next: TuneState, at: number) => {
      state.current = next;
      setRead({ at: Math.max(at, now()) });
    },
    [now],
  );

  const flip = useCallback(
    ({ firstTune, streaming }: { streaming: boolean; firstTune: boolean }) => {
      const at = now();
      const next = tuneFlip(state.current, { at, firstTune, streaming });
      update(next, at);
      return next.flips;
    },
    [now, update],
  );
  const ready = useCallback(
    (which: number) => {
      const at = now();
      update(tunePictureReady(state.current, which, at), at);
    },
    [now, update],
  );
  const abandon = useCallback((which: number) => update(tuneAbandon(state.current, which), now()), [now, update]);

  const view = tuneView(state.current, read.at);
  const { nextAt } = view;
  useEffect(() => {
    if (nextAt === null) return undefined;
    const timer = setTimeout(() => setRead({ at: Math.max(now(), nextAt) }), Math.max(0, nextAt - now()));
    return () => clearTimeout(timer);
  }, [nextAt, now]);

  return { abandon, flip, ready, view };
};

export type { WatchTune };
export { useWatchTune };
