import {
  type ChannelLineup,
  type ChannelLineupArgs,
  createChannelLineup,
  type LineupChannel,
} from "@loomarr/core/channel-lineup";
import {
  createWatchScreenState,
  type WatchLayout,
  type WatchRequest,
  type WatchScreenEvent,
  type WatchScreenState,
  type WatchScreenView,
  watchScreenReduce,
  watchScreenView,
} from "@loomarr/core/watch-screen";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Platform } from "react-native";

/** An event as a caller sends it; the hook dates it. */
type WatchScreenInput = WatchScreenEvent extends infer E
  ? E extends { at: number }
    ? Omit<E, "at">
    : never
  : never;

interface UseWatchScreenArgs<C extends LineupChannel> {
  /** The lineup, but for the channels left, which the screen keeps. */
  lineup: Omit<ChannelLineupArgs<C>, "leftIds">;
  layout: WatchLayout;
  /**
   * The channel on screen when the screen opens. A later change from outside the screen (another
   * device, the Guide) is a `tuned` event. A tune the screen asked for is never echoed back: the
   * model moved already. An echo of the channel on screen changes nothing, but a late echo of an
   * older tune reads as a change from outside, back to that channel, with no tune asked for, so the
   * screen would name a channel the player has left.
   */
  watchedId: string | undefined;
  /** The first-run hint was seen in an earlier session. */
  hintSeen?: boolean;
  /** Does what the screen asks: tunes, pauses, opens the Guide, shows a message. */
  onRequest: (request: WatchRequest) => void;
  /** Milliseconds from any fixed point; `Date.now` unless a test passes its own. */
  now?: () => number;
}

interface WatchScreenModel<C extends LineupChannel> {
  /** Built with the channels this screen has left, so its `lastChannel` is the one just left. */
  lineup: ChannelLineup<C>;
  state: WatchScreenState;
  view: WatchScreenView<C>;
  send: (event: WatchScreenInput) => void;
  /**
   * Whether the last input anywhere on the page was a key or a remote's, not a pointer's. The one
   * note the screen reads before it moves focus by itself, and before it tells the model a list
   * event came from a key.
   */
  keyed: () => boolean;
}

/**
 * Runs `@loomarr/core/watch-screen` for a screen: it dates each event, hands the requests to
 * `onRequest`, and wakes itself at the view's `nextAt`.
 *
 * Keys reach it from the caller, in the bubbling phase and after every control: listen on the
 * document's `keydown` without `capture`, and send `{ type: "key" }` only for what `watchKeyOf`
 * passes, which leaves out a key a control or the open list already took. A caller must not handle
 * the same keys in the capture phase: a key would then reach the screen before the control that
 * holds focus, and Space would both press the control and pause.
 */
const useWatchScreen = <C extends LineupChannel>({
  hintSeen,
  layout,
  lineup: lineupArgs,
  now = Date.now,
  onRequest,
  watchedId,
}: UseWatchScreenArgs<C>): WatchScreenModel<C> => {
  const clock = useRef(now);
  clock.current = now;
  const latest = useRef({ lineupArgs, onRequest });
  latest.current = { lineupArgs, onRequest };

  const [snapshot, setSnapshot] = useState(() => {
    const at = now();
    return { at, state: createWatchScreenState({ at, hintSeen, layout, watchedId }) };
  });
  const current = useRef(snapshot.state);

  const apply = useCallback((event: WatchScreenEvent) => {
    const lineup = createChannelLineup({ ...latest.current.lineupArgs, leftIds: current.current.leftIds });
    const step = watchScreenReduce(current.current, event, lineup);
    current.current = step.state;
    setSnapshot({ at: event.at, state: step.state });
    for (const request of step.requests) latest.current.onRequest(request);
  }, []);
  // The last input's kind, for the whole document: a key reaches the screen from the document
  // whatever holds focus, so a note kept inside the screen would miss one pressed on the page.
  // The listeners only record; they handle, prevent and stop nothing, and so may listen first, in
  // the capture phase, where a control that stops a key cannot hide it. Off the web every input
  // is a key's or a remote's.
  const lastKey = useRef(Platform.OS !== "web");
  useEffect(() => {
    type Listen = (type: string, listener: () => void, options: object) => void;
    const page = (globalThis as { document?: { addEventListener: Listen; removeEventListener: Listen } })
      .document;
    if (Platform.OS !== "web" || !page) return undefined;
    const key = () => {
      lastKey.current = true;
    };
    const pointer = () => {
      lastKey.current = false;
    };
    const options = { capture: true, passive: true };
    page.addEventListener("keydown", key, options);
    page.addEventListener("pointerdown", pointer, options);
    return () => {
      page.removeEventListener("keydown", key, options);
      page.removeEventListener("pointerdown", pointer, options);
    };
  }, []);
  const keyed = useCallback(() => lastKey.current, []);

  const send = useCallback(
    (event: WatchScreenInput) => {
      if (event.type === "key") lastKey.current = true;
      apply({ ...event, at: clock.current() } as WatchScreenEvent);
    },
    [apply],
  );

  const { channels, favouriteIds, lastTunedId, recentIds } = lineupArgs;
  const { leftIds } = snapshot.state;
  const lineup = useMemo(
    () => createChannelLineup({ channels, favouriteIds, lastTunedId, leftIds, recentIds }),
    [channels, favouriteIds, lastTunedId, leftIds, recentIds],
  );
  const view = watchScreenView(snapshot.state, lineup, snapshot.at);

  useEffect(() => {
    if (current.current.layout !== layout) send({ layout, type: "layout" });
  }, [layout, send]);

  // A timer may fire a little early; the tick is dated no earlier than the time it was set for.
  const { nextAt } = view;
  useEffect(() => {
    if (nextAt === null) return undefined;
    const timer = setTimeout(
      () => apply({ at: Math.max(clock.current(), nextAt), type: "tick" }),
      Math.max(0, nextAt - clock.current()),
    );
    return () => clearTimeout(timer);
  }, [apply, nextAt]);

  return { keyed, lineup, send, state: snapshot.state, view };
};

export type { UseWatchScreenArgs, WatchScreenInput, WatchScreenModel };
export { useWatchScreen };
