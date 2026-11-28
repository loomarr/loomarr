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
import type { WatchTuneReport } from "../use-watch-player";

/** The tune attempts whose flips are remembered, so a late outcome still finds its flip. */
const REMEMBERED_FLIPS = 8;

interface WatchTune {
  /** What to draw now. */
  view: TuneView;
  /**
   * The player's report that a tune began: pass it to `useWatchPlayer`'s `onTune`, so no report
   * can come before its listener. A reconnect to the channel on screen is not a tune.
   */
  onTune: (report: WatchTuneReport) => void;
  /** What the player's current attempt came to: a picture, or none (a failure, off air). */
  settle: (attemptId: number | undefined, outcome: "picture" | "none") => void;
  /** The player is reconnecting to the channel on screen. */
  reconnecting: boolean;
}

/**
 * The tune (`@loomarr/core/tune`) for the web's watching screen, driven by what the player really
 * does. A tune that begins is a flip: a change when the player had the channel warmed, a tune-in
 * for the page's first tune and for a channel that must start. Its picture, or its failure, ends
 * it. The view is read again at the time it names, never earlier than that time.
 */
const useWatchTune = (now: () => number = Date.now): WatchTune => {
  const state = useRef<TuneState>(idleTune);
  // A new object each time, so an event at the same millisecond still draws its state.
  const [read, setRead] = useState(() => ({ at: now() }));
  const [reconnecting, setReconnecting] = useState(false);
  const flips = useRef(new Map<number, number>());
  const played = useRef(false);
  const lastChannel = useRef<string | undefined>(undefined);
  const update = useCallback(
    (next: TuneState, at: number) => {
      state.current = next;
      setRead({ at: Math.max(at, now()) });
    },
    [now],
  );

  const onTune = useCallback(
    (report: WatchTuneReport) => {
      const again = report.reason === "retry" && report.channelId === lastChannel.current;
      lastChannel.current = report.channelId;
      setReconnecting(again);
      if (again) return;
      const at = now();
      const next = tuneFlip(state.current, { at, firstTune: !played.current, streaming: report.warm });
      flips.current.set(report.attemptId, next.flips);
      for (const stale of [...flips.current.keys()].slice(0, -REMEMBERED_FLIPS)) flips.current.delete(stale);
      update(next, at);
    },
    [now, update],
  );

  const settle = useCallback(
    (attemptId: number | undefined, outcome: "picture" | "none") => {
      if (outcome === "picture") {
        played.current = true;
        setReconnecting(false);
      }
      const flip = attemptId === undefined ? undefined : flips.current.get(attemptId);
      if (flip === undefined) return;
      const at = now();
      update(
        outcome === "picture" ? tunePictureReady(state.current, flip, at) : tuneAbandon(state.current, flip),
        at,
      );
    },
    [now, update],
  );

  const view = tuneView(state.current, read.at);
  const { nextAt } = view;
  useEffect(() => {
    if (nextAt === null) return undefined;
    const timer = setTimeout(() => setRead({ at: Math.max(now(), nextAt) }), Math.max(0, nextAt - now()));
    return () => clearTimeout(timer);
  }, [nextAt, now]);

  return { onTune, reconnecting, settle, view };
};

export type { WatchTune };
export { useWatchTune };
