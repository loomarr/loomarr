import { surfChannelData } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import { Button } from "@/components/ui/button";
import { SectionHeader } from "@/components/ui/section-header";
import type { RecentlyTunedProps } from "./recently-tuned.type";

// RecentlyTuned — Home's "Recently tuned" return card (#1822 evidence, X2): the caller's
// last-tuned channel and what is on it now. It does not promise saved episode progress — the
// copy is "Join what's on now.", never "Resume" or "Continue" — because a tune is not a bookmark.
//
// Hidden rather than shown stale: its channel may since have been removed, may have nothing
// airing right now, or may already be the channel an active own-viewing card (Watching now)
// represents, and the record says not to duplicate that choice.
const RecentlyTuned = ({ continueWatching, layout, nowMs, activeChannelIds }: RecentlyTunedProps) => {
  if (!continueWatching || activeChannelIds.has(continueWatching.channelId)) return null;
  const channel = layout.channels.find((c) => c.source.channelId === continueWatching.channelId);
  if (!channel) return null;
  const data = surfChannelData(channel, nowMs, layout.timezone);
  if (!data.now) return null;

  return (
    <section aria-labelledby="home-recently-tuned">
      <SectionHeader id="home-recently-tuned" title="Recently tuned" />
      <div className="flex flex-col items-start justify-between gap-4 rounded-lg border border-static-700 bg-static-900 p-4 sm:flex-row sm:items-center">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ChannelIdent name={data.channelName} number={channel.source.number} logo={data.channelLogoUri} />
            <span className="font-mono text-signal text-sm">
              {String(channel.source.number).padStart(2, "0")}
            </span>
            <span className="text-sm text-static-400">{data.channelName}</span>
          </div>
          <h3 className="mt-2.5 break-words font-semibold text-base">
            {data.now.seriesTitle ?? data.now.title}
          </h3>
          <p className="m-0 text-sm text-static-400">Join what’s on now.</p>
        </div>
        <Button render={<Link to="/channels/$id/watch" params={{ id: channel.source.channelId }} />}>
          Watch live
        </Button>
      </div>
    </section>
  );
};

export { RecentlyTuned };
