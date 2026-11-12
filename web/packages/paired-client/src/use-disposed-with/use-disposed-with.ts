import { useEffect, useRef } from "react";

/**
 * Disposes `value` when it is replaced or the component goes away, and only then. The effect is
 * keyed on the value alone, so an unrelated input changing never disposes an object that the
 * `useMemo` which made it keeps handing out. Call it right after the `useMemo` that creates `value`,
 * one call per object.
 */
const useDisposedWith = <T>(value: T | undefined, dispose: (value: T) => void) => {
  const disposeRef = useRef(dispose);
  disposeRef.current = dispose;
  useEffect(() => {
    if (value === undefined) return;
    return () => disposeRef.current(value);
  }, [value]);
};

export { useDisposedWith };
