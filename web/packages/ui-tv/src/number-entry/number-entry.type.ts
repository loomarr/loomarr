interface TvNumberEntrySnapshot {
  digits: string;
}

interface TvNumberEntryTimer {
  cancel: (handle: unknown) => void;
  schedule: (callback: () => void, delayMs: number) => unknown;
}

interface TvNumberEntryOptions {
  delayMs?: number;
  maxDigits?: number;
  onCommit: (digits: string) => void;
  timer?: TvNumberEntryTimer;
}

interface TvNumberEntryController {
  cancel: () => void;
  commit: () => void;
  dispose: () => void;
  getSnapshot: () => TvNumberEntrySnapshot;
  pushEvent: (eventType: string) => boolean;
  subscribe: (listener: () => void) => () => void;
}

export type { TvNumberEntryController, TvNumberEntryOptions, TvNumberEntrySnapshot, TvNumberEntryTimer };
