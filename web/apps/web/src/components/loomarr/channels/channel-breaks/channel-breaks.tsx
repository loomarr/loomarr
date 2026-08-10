import { channelsApi, unwrap } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { cn } from "@/lib";
import { PodTimeline } from "../../filler/pod-timeline";
import type { ChannelBreaksProps } from "./channel-breaks.type";

// ChannelBreaks — the read-only "what plays in this channel's breaks" (§10, §12).
//
// ⚠ **This exists because `GET /v1/channels/{id}/pods` had no caller at all.** The route shipped,
// three cache invalidations pointed at its query key, and nothing ever filled that cache — an
// endpoint reachable only by curl. Deleting it was the wrong fix: it is MEMBER-readable, where the
// draft sandbox (`POST …/pods/preview`) is admin-only, so it is the only way a non-admin can see
// why a channel sounds the way it does. Leaving it orphaned is exactly what `/surface-audit`
// exists to find.
//
// ⚠ Deliberately NOT folded into the Filler section. That section is the admin authoring sandbox
// and is gated accordingly; this is the member-visible answer, on the one screen a member sees.
//
// ⚠ `retry: false` because an install without filler configured answers 501, and this renders
// nothing in that case rather than an error — a channel with no commercials is an ordinary state,
// not a fault. Same treatment the coverage meter gives the same condition.
const ChannelBreaks = ({ channelId, className }: ChannelBreaksProps) => {
  const pods = channelsApi.usePreviewChannelPods(channelId, { query: { retry: false } });
  const pod = unwrap(pods.data);
  const entries = pod?.entries ?? [];

  // Nothing to say yet: still loading, filler is not set up (501), or this channel genuinely has
  // no break pool. All three render as absence rather than as a problem.
  if (entries.length === 0) return null;

  return (
    <section className={cn("flex flex-col gap-2", className)}>
      <h3 className="font-medium text-muted-foreground text-xs uppercase tracking-wide">In the breaks</h3>
      <PodTimeline entries={entries} matchLevel={pod?.matchLevel} />
      <p className="text-muted-foreground text-sm">
        {`${pluralize(entries.length, "clip")} between shows, assembled exactly as the channel builds them.`}
      </p>
    </section>
  );
};

export { ChannelBreaks };
