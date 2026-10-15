import { useSyncExternalStore } from "react";

// usePhoneWidth — true below the tablet breakpoint, the same `md` (48rem) the app shell's rail
// widens at. For layouts that change what they render on a phone, not just how it's arranged: the
// portrait Watch puts its controls under the picture (#1785, native mock 5e), so the desktop
// overlay controls aren't mounted there at all. Read during render, so the first paint is right.
// Without matchMedia (jsdom), nothing is a phone: the desktop layout is the default.
const media = (query: string) =>
  typeof window.matchMedia === "function" ? window.matchMedia(query) : undefined;

// True while the viewport is narrower than `minWidth` (a `(min-width: …)` query that does not match).
const useNarrowerThan = (minWidth: string): boolean =>
  useSyncExternalStore(
    (onChange) => {
      const list = media(`(min-width: ${minWidth})`);
      list?.addEventListener("change", onChange);
      return () => list?.removeEventListener("change", onChange);
    },
    () => media(`(min-width: ${minWidth})`)?.matches === false,
    () => false,
  );

const usePhoneWidth = (): boolean => useNarrowerThan("48rem");

// useBelowSm — true below Tailwind's `sm` (40rem), for a control that changes what it renders where
// a layout's `max-sm:` classes rearrange it: the channel header's name field (#1785).
const useBelowSm = (): boolean => useNarrowerThan("40rem");

export { useBelowSm, usePhoneWidth };
