import type {
  TvNumberEntryController,
  TvNumberEntryOptions,
  TvNumberEntrySnapshot,
} from "./number-entry.type";

const DEFAULT_DELAY_MS = 1_200;
const DEFAULT_MAX_DIGITS = 3;

const nativeTimer = {
  cancel: (handle: unknown) => clearTimeout(handle as ReturnType<typeof setTimeout>),
  schedule: (callback: () => void, delayMs: number) => setTimeout(callback, delayMs),
};

const createTvNumberEntryController = ({
  delayMs = DEFAULT_DELAY_MS,
  maxDigits = DEFAULT_MAX_DIGITS,
  onCommit,
  timer = nativeTimer,
}: TvNumberEntryOptions): TvNumberEntryController => {
  let disposed = false;
  let timeout: unknown;
  let snapshot: TvNumberEntrySnapshot = { digits: "" };
  const listeners = new Set<() => void>();

  const publish = (digits: string) => {
    snapshot = { digits };
    for (const listener of listeners) listener();
  };
  const clearPending = () => {
    if (timeout !== undefined) timer.cancel(timeout);
    timeout = undefined;
  };
  const commit = () => {
    if (disposed || !snapshot.digits) return;
    const digits = snapshot.digits;
    clearPending();
    publish("");
    onCommit(digits);
  };

  return {
    cancel: () => {
      if (disposed) return;
      clearPending();
      if (snapshot.digits) publish("");
    },
    commit,
    dispose: () => {
      if (disposed) return;
      disposed = true;
      clearPending();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    pushEvent: (eventType) => {
      if (disposed || !/^\d$/.test(eventType)) return false;
      clearPending();
      publish(`${snapshot.digits}${eventType}`.slice(-maxDigits));
      timeout = timer.schedule(commit, delayMs);
      return true;
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createTvNumberEntryController };
