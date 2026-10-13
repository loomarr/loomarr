import { clampNumberEntryMs, DEFAULT_NUMBER_ENTRY_MS } from "../watching-navigation";
import type { TvAutoTuneDurationStorage, TvAutoTuneSettingStore } from "./auto-tune-setting.type";

const AUTO_TUNE_DURATION_KEY = "loomarr.tv.autoTuneDurationMs";

/** The Surf rail's "Channel-number wait" choices (maintainer-approved mock, #1659), default first. */
const TV_AUTO_TUNE_CHOICES_MS = [DEFAULT_NUMBER_ENTRY_MS, 2_000, 3_000, 5_000, 10_000] as const;

/**
 * Per-device auto-tune timer (#1659 decision N4, WCAG 2.2.1): the typed-digit countdown defaults
 * to `DEFAULT_NUMBER_ENTRY_MS`, but a device may persist a longer duration for motor-impaired
 * viewers. Storage is injected (the TV app's SecureStore) so this stays testable without a native
 * module.
 *
 * `apps/tv/src/app.tsx` loads it once, passes it as `reduceTvWatchingRemote`'s `durationMs`, and
 * saves the Surf rail's choice.
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

export { createTvAutoTuneSetting, TV_AUTO_TUNE_CHOICES_MS };
