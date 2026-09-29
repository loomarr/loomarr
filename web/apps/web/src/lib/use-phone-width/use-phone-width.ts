import { useSyncExternalStore } from "react";

// usePhoneWidth — true below the tablet breakpoint, the same `md` (48rem) the app shell's rail
// widens at. For layouts that change what they render on a phone, not just how it's arranged: the
// portrait Watch puts its controls under the picture (#1785, native mock 5e), so the desktop
// overlay controls aren't mounted there at all. Read during render, so the first paint is right.
// Without matchMedia (jsdom), nothing is a phone: the desktop layout is the default.
const QUERY = "(min-width: 48rem)";

const media = () => (typeof window.matchMedia === "function" ? window.matchMedia(QUERY) : undefined);

const subscribe = (onChange: () => void) => {
  const list = media();
  list?.addEventListener("change", onChange);
  return () => list?.removeEventListener("change", onChange);
};

const usePhoneWidth = (): boolean =>
  useSyncExternalStore(
    subscribe,
    () => media()?.matches === false,
    () => false,
  );

export { usePhoneWidth };
