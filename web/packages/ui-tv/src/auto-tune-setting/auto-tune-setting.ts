import { clampNumberEntryMs, DEFAULT_NUMBER_ENTRY_MS } from "../watching-navigation";
import type { TvAutoTuneDurationStorage, TvAutoTuneSettingStore } from "./auto-tune-setting.type";

const AUTO_TUNE_DURATION_KEY = "loomarr.tv.autoTuneDurationMs";

/**
 * Per-device auto-tune timer (#1659 decision N4, WCAG 2.2.1): the typed-digit countdown defaults
 * to `DEFAULT_NUMBER_ENTRY_MS`, but a device may persist a longer duration for motor-impaired
 * viewers. Storage is injected (the TV app's SecureStore) so this stays testable without a native
 * module.
 *
 * Not yet wired into `apps/tv/src/app.tsx`: the app's remote-event loop needs to load this once
 * at startup and pass the result as `reduceTvWatchingRemote`'s `durationMs`, plus a settings
 * surface to call `save`. Tracked as a follow-up (#1659 PR 15).
 */
const createTvAutoTuneSetting = (storage: TvAutoTuneDurationStorage): TvAutoTuneSettingStore => ({
  async load() {
    const stored = await storage.getItem(AUTO_TUNE_DURATION_KEY);
    const parsed = stored === null ? Number.NaN : Number.parseInt(stored, 10);
    return Number.isFinite(parsed) ? clampNumberEntryMs(parsed) : DEFAULT_NUMBER_ENTRY_MS;
  },
  async save(durationMs) {
    const clamped = clampNumberEntryMs(durationMs);
    await storage.setItem(AUTO_TUNE_DURATION_KEY, String(clamped));
    return clamped;
  },
});

export { createTvAutoTuneSetting };
