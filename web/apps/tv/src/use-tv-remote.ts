import {
  createTvRemote,
  type TvRemote,
  type TvRemoteOptions,
  type TvWatchingRemoteState,
} from "@loomarr/ui-tv";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";

const realClock: TvRemoteOptions["clock"] = {
  clearTimeout: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
  now: () => Date.now(),
  setTimeout: (run, delayMs) => setTimeout(run, delayMs),
};

/** The app's one TV remote, re-rendering on its number entry; `onEvent` always runs as last rendered. */
const useTvRemote = (
  numberEntryMs: number,
  onEvent: TvRemoteOptions["onEvent"],
): readonly [TvRemote, TvWatchingRemoteState] => {
  const latestOnEvent = useRef(onEvent);
  useEffect(() => {
    latestOnEvent.current = onEvent;
  });
  const [remote] = useState(() =>
    createTvRemote({
      clock: realClock,
      numberEntryMs,
      onEvent: (event, outcome) => latestOnEvent.current(event, outcome),
    }),
  );
  useEffect(() => remote.setNumberEntryMs(numberEntryMs), [remote, numberEntryMs]);
  useEffect(() => remote.dispose, [remote]);
  const state = useSyncExternalStore(remote.subscribe, remote.getSnapshot);
  return [remote, state];
};

export { useTvRemote };
