/** Shaped like expo-secure-store's async key/value API, so the TV app can inject it directly. */
type TvAutoTuneDurationStorage = {
  getItem: (key: string) => Promise<string | null>;
  setItem: (key: string, value: string) => Promise<void>;
};

type TvAutoTuneSettingStore = {
  /** The persisted duration, clamped; `DEFAULT_NUMBER_ENTRY_MS` when nothing is stored yet. */
  load: () => Promise<number>;
  /** Persists a clamped duration and returns the value actually stored. */
  save: (durationMs: number) => Promise<number>;
};

export type { TvAutoTuneDurationStorage, TvAutoTuneSettingStore };
