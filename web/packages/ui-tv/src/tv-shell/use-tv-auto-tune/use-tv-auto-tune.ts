import { useCallback, useEffect, useRef, useState } from "react";

import type { TvAutoTuneSettingStore } from "../../auto-tune-setting";
import { DEFAULT_NUMBER_ENTRY_MS } from "../../watching-navigation";

/**
 * The per-device channel-number wait (#1659 decision N4, WCAG 2.2.1): the stored duration once it
 * has loaded, and the function the Surf rail's choice goes through. Pass a stable store.
 */
const useTvAutoTune = (setting: TvAutoTuneSettingStore): readonly [number, (durationMs: number) => void] => {
  const [autoTuneMs, setAutoTuneMs] = useState<number>(DEFAULT_NUMBER_ENTRY_MS);
  // A choice made before the stored value finishes loading wins over that stale read.
  const chosen = useRef(false);
  useEffect(() => {
    let current = true;
    void setting
      .load()
      .then((durationMs) => {
        if (current && !chosen.current) setAutoTuneMs(durationMs);
      })
      // An unreadable store keeps the default rather than leaving digit entry without a timer.
      .catch(() => undefined);
    return () => {
      current = false;
    };
  }, [setting]);
  const change = useCallback(
    (durationMs: number) => {
      chosen.current = true;
      setAutoTuneMs(durationMs);
      // The choice applies now; an unwritable store just means the next launch reads the default.
      void setting.save(durationMs).catch(() => undefined);
    },
    [setting],
  );
  return [autoTuneMs, change];
};

export { useTvAutoTune };
