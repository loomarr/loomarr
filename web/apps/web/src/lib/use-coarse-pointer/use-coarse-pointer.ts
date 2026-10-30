import { useSyncExternalStore } from "react";

// useCoarsePointer — true when the primary pointer is a finger (`(pointer: coarse)`): the player
// chrome's touch form (§9.5), with 44 px targets and no hover card or tooltips, since there is
// nothing to hover with. Without matchMedia (jsdom) the pointer is fine: the desktop form.
const QUERY = "(pointer: coarse)";

const media = () => (typeof window.matchMedia === "function" ? window.matchMedia(QUERY) : undefined);

const useCoarsePointer = (): boolean =>
  useSyncExternalStore(
    (onChange) => {
      const list = media();
      list?.addEventListener("change", onChange);
      return () => list?.removeEventListener("change", onChange);
    },
    () => media()?.matches === true,
    () => false,
  );

export { useCoarsePointer };
