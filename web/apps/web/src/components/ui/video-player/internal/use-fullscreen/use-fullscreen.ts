import { type RefObject, useCallback, useEffect, useState } from "react";

// useFullscreen owns the player's fullscreen concern: whether the WRAPPER is currently fullscreen,
// and a toggle that enters/exits it. Extracted from VideoPlayer so the component reads as pure
// composition and this browser-quirk logic is testable on its own.
//
// It targets the wrapper (not the bare <video>) on purpose — going fullscreen on the wrapper takes
// the overlay controls along with the video. And it tracks the browser's OWN `fullscreenchange`
// event rather than just our button clicks, so exiting with Esc still flips the state (and the icon).
const useFullscreen = (wrapperRef: RefObject<HTMLElement | null>) => {
  const [fullscreen, setFullscreen] = useState(false);

  useEffect(() => {
    const onChange = () => setFullscreen(document.fullscreenElement === wrapperRef.current);
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, [wrapperRef]);

  // Best-effort: requestFullscreen rejects if not from a user gesture, which a button click satisfies.
  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) {
      void document.exitFullscreen().catch(() => {});
    } else {
      void wrapperRef.current?.requestFullscreen().catch(() => {});
    }
  }, [wrapperRef]);

  return { fullscreen, toggleFullscreen };
};

export { useFullscreen };
