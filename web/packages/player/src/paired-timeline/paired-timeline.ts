import type { ChannelTimeline, ChannelTimelineSource } from "@loomarr/core/channel-timeline";
import { useEffect, useState } from "react";

/** As web Watch refetches it: often enough that the live edge and what's coming stay current. */
const TIMELINE_REFRESH_MS = 30_000;

/**
 * The tuned channel's timeline for the player chrome (§9), refreshed while it stays tuned. A new
 * channel drops the old one's answer at once, so the chrome never draws one channel's schedule
 * under another; a failed refresh keeps the last answer, which the projected clock carries on.
 */
const usePairedTimeline = (
  source: ChannelTimelineSource,
  channelId: string | undefined,
): ChannelTimeline | undefined => {
  const [loaded, setLoaded] = useState<{ channelId: string; timeline: ChannelTimeline }>();
  useEffect(() => {
    if (!channelId) return;
    let pending: AbortController | undefined;
    const load = () => {
      pending?.abort();
      const request = new AbortController();
      pending = request;
      source.load(channelId, request.signal).then(
        (timeline) => {
          if (!request.signal.aborted) setLoaded({ channelId, timeline });
        },
        () => undefined,
      );
    };
    load();
    const timer = setInterval(load, TIMELINE_REFRESH_MS);
    return () => {
      clearInterval(timer);
      pending?.abort();
    };
  }, [channelId, source]);
  return loaded?.channelId === channelId ? loaded?.timeline : undefined;
};

export { TIMELINE_REFRESH_MS, usePairedTimeline };
