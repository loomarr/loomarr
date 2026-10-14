import {
  initialTvWatchingRemoteState,
  reduceTvWatchingRemote,
  type TvRemoteDigit,
  type TvWatchingRemoteResult,
  type TvWatchingRemoteState,
} from "@loomarr/ui-tv";
import { useCallback, useEffect, useRef, useState } from "react";
import type { UseNumberEntry, UseNumberEntryOptions } from "./use-number-entry.type";

// Once every digit a channel number can have is in, the entry tunes after this short beat instead
// of waiting out the whole entry window (the console mock: 350 ms after the last digit).
const FULL_ENTRY_MS = 350;
// How long "No such channel" stays up before the readout clears (the console mock).
const MISS_MS = 900;

const isDigit = (key: string): key is TvRemoteDigit => /^[0-9]$/.test(key);

// useNumberEntry is the Watch page's 0-9 direct tune (#1659 W1). The digit buffer, its window and
// Enter are the TV remote's reducer, so a number typed on the web and one pressed on a TV remote
// behave alike (three digits, 1.2 s to finish); this hook adds the timers and the channel lookup.
const useNumberEntry = ({ channels, onTune }: UseNumberEntryOptions): UseNumberEntry => {
  const [state, setState] = useState<TvWatchingRemoteState>(initialTvWatchingRemoteState);
  // The number that matched no channel, kept on screen beside "No such channel" for a moment.
  const [missedDigits, setMissedDigits] = useState<string>();
  const stateRef = useRef(state);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const latest = useRef({ channels, onTune });
  latest.current = { channels, onTune };

  const width = Math.max(1, ...channels.map((channel) => String(channel.number).length));

  const apply = useCallback((result: TvWatchingRemoteResult) => {
    stateRef.current = result.state;
    setState(result.state);
    if (result.intent?.kind !== "tune-number") return;
    const { digits } = result.intent;
    const wanted = Number.parseInt(digits, 10);
    const match = latest.current.channels.find((channel) => channel.number === wanted);
    if (match) {
      latest.current.onTune(match.id);
      return;
    }
    setMissedDigits(digits);
    timer.current = setTimeout(() => setMissedDigits(undefined), MISS_MS);
  }, []);

  const press = useCallback(
    (key: string) => {
      if (!isDigit(key)) return false;
      clearTimeout(timer.current);
      setMissedDigits(undefined);
      const atMs = Date.now();
      const result = reduceTvWatchingRemote(stateRef.current, { atMs, digit: key, key: "digit" });
      apply(result);
      const entry = result.state.numberEntry;
      if (!entry) return true;
      const waitMs = entry.digits.length >= width ? FULL_ENTRY_MS : entry.expiresAtMs - atMs;
      // The timeout event carries the entry's own expiry, so the reducer always finishes it here.
      timer.current = setTimeout(
        () => apply(reduceTvWatchingRemote(stateRef.current, { atMs: entry.expiresAtMs, key: "timeout" })),
        waitMs,
      );
      return true;
    },
    [apply, width],
  );

  const commit = useCallback(() => {
    if (!stateRef.current.numberEntry) return false;
    clearTimeout(timer.current);
    apply(reduceTvWatchingRemote(stateRef.current, { key: "select" }));
    return true;
  }, [apply]);

  useEffect(() => () => clearTimeout(timer.current), []);

  return {
    digits: state.numberEntry?.digits ?? missedDigits,
    missed: missedDigits !== undefined,
    width,
    press,
    commit,
  };
};

export { useNumberEntry };
