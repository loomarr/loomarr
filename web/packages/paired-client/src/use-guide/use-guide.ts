import type { GuideController, GuideControllerSnapshot } from "@loomarr/core/guide";
import { createGuideController, createGuideSourcePort, guideWindow } from "@loomarr/core/guide";
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
  const guide = useMemo(
    () =>
      createGuideController({
        resolveWindow: guideWindowMinutes
          ? (at) =>
              guideWindow({
                at,
                dayOffset: 0,
                hourShift: 0,
                startHour: null,
                windowMinutes: guideWindowMinutes,
              })
          : undefined,
        source: createGuideSourcePort(request),
      }),
    [guideWindowMinutes, request],
  );
  useDisposedWith(guide, (built) => built.dispose());
  const guideSnapshot = useSyncExternalStore(guide.subscribe, guide.getSnapshot, guide.getSnapshot);
  useEffect(() => {
    if (channelId) void guide.refresh(channelId);
  }, [channelId, guide]);
  return { guide, guideSnapshot };
};

export type { UseGuideOptions };
export { useGuide };
