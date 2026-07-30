import { useCallback, useEffect, useRef, useState } from "react";

// How long the "Copied" acknowledgement stays up. Long enough to register, short enough
// that a second copy doesn't feel stuck.
const ACK_MS = 2000;

// useCopied — copy a value and flash an acknowledgement, keyed so one list can track
// which row was copied.
//
// Web-only (it touches navigator + window), so it lives in lib rather than core.
// Extracted because both call sites hand-rolled it identically and both carried the same
// two bugs: the timeout was never cleared, so copying and then navigating away warned
// about setting state on an unmounted component; and a second copy started a second timer
// that raced the first, clearing the acknowledgement early.
const useCopied = <K = true>() => {
  const [copied, setCopied] = useState<K>();
  // ⚠ React 19's types require an explicit initial value — `useRef<number>()` no longer
  // infers one. `undefined` is what it always held before the first copy, and the cleanup
  // below already treats undefined as "no timer pending", so this is the same behaviour
  // stated out loud rather than a change.
  const timer = useRef<number | undefined>(undefined);

  // Cleared on unmount so a pending acknowledgement can't fire into a dead component.
  useEffect(
    () => () => {
      if (timer.current !== undefined) window.clearTimeout(timer.current);
    },
    [],
  );

  const copy = useCallback((value: string, key: K = true as K) => {
    // Optional-chained because clipboard is undefined on insecure origins — a
    // plain-HTTP LAN install is a supported deployment (§11 cookie.secure=auto), so
    // this must degrade rather than throw.
    void navigator.clipboard?.writeText(value);
    // Restart the window on every copy: without this, a second copy inherits the first
    // one's remaining time and the acknowledgement vanishes early.
    if (timer.current !== undefined) window.clearTimeout(timer.current);
    setCopied(key);
    timer.current = window.setTimeout(() => setCopied(undefined), ACK_MS);
  }, []);

  return { copied, copy };
};

export { useCopied };
