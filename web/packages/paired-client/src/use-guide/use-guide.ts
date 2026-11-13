import type { GuideController, GuideControllerSnapshot } from "@loomarr/core/guide";
import {
  createGuideController,
  createGuideSourcePort,
  defaultGuideWindow,
  GUIDE_CLOCK_TICK_MS,
  GUIDE_SLIDE_MARGIN_MS,
  guideWindow,
} from "@loomarr/core/guide";
import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useDisposedWith } from "../use-disposed-with";

interface UseGuideOptions {
  /** The channel the player is on; the guide reads its schedule whenever it changes. */
  channelId?: string;
  /** How much of the schedule the guide loads from now; unset reads the server's default window. */
  guideWindowMinutes?: number;
  request: typeof globalThis.fetch;
}

/** The paired server's guide: made for the request and window, disposed when replaced or unmounted. */
const useGuide = ({
  channelId,
  guideWindowMinutes,
  request,
}: UseGuideOptions): { guide: GuideController; guideSnapshot: GuideControllerSnapshot } => {
  const resolveWindow = useMemo(
    () =>
      guideWindowMinutes
        ? (at: number) =>
            guideWindow({
              at,
              dayOffset: 0,
              hourShift: 0,
              startHour: null,
              windowMinutes: guideWindowMinutes,
            })
        : defaultGuideWindow,
    [guideWindowMinutes],
  );
  const guide = useMemo(
    () => createGuideController({ resolveWindow, source: createGuideSourcePort(request) }),
    [request, resolveWindow],
  );
  useDisposedWith(guide, (built) => built.dispose());
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  useEffect(() => {
    if (channelId) void guide.refresh(channelId);
  }, [channelId, guide]);
  // The guide's clock, as the web page's `useGuideWindow`: a screen left open reads the live window
  // again once now is near the end of the one it holds, so it never runs out of schedule. A read that
  // fails does not move the mark, so the next tick tries again.
  useEffect(() => {
    let readAt = Date.now();
    const timer = setInterval(() => {
      const now = Date.now();
      if (now < resolveWindow(readAt).to - GUIDE_SLIDE_MARGIN_MS) return;
      void guide.refresh().then(() => {
        if (guide.getSnapshot().error === undefined) readAt = now;
      });
    }, GUIDE_CLOCK_TICK_MS);
    return () => clearInterval(timer);
  }, [guide, resolveWindow]);
  return { guide, guideSnapshot };
};

export type { UseGuideOptions };
export { useGuide };
