import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { formatEpgTime } from "@loomarr/core/format";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ChannelUpcomingProps } from "./channel-upcoming.type";

// ChannelUpcoming — the viewer-facing "what's on later" strip (P7 / plan finding 5): the
// program airing now, then the next few, with airtimes from the backend streaming this channel
// (GET /v1/channels/{id}/upcoming). Read-only, shown to every user on the channel Overview.
// Gaps are already filtered out server-side, so this just renders the list of shows. The first
// entry is "on now" when the channel is live and it has started.
const ChannelUpcoming = ({ channelId, live = false, className }: ChannelUpcomingProps) => {
  const upcoming = channelsApi.useChannelUpcoming(channelId, undefined, {
    query: { retry: false },
  });
  const entries = unwrap(upcoming.data, (b) => b.upcoming) ?? [];

  // The web mock's "What's on": a mono eyebrow, then one row per show: its start time, the title,
  // and a red NOW badge on the one airing. Every row keeps its time, so the NOW row still says
  // when it started.
  return (
    <section className={cn("flex flex-col gap-1.5", className)}>
      <h3 className="mb-1 font-mono font-normal text-2xs text-muted-foreground uppercase tracking-wide">
        What's on
      </h3>
      {entries.length === 0 ? (
        // The API deliberately lowers no guide, an empty schedule, and a backend read failure to
        // the same empty list. Do not infer broadcast state from a response that cannot prove it.
        <p className="text-muted-foreground text-sm">Programme information isn't available.</p>
      ) : (
        <ol className="flex flex-col gap-1.5">
          {entries.map((entry, i) => {
            // The first entry is what's airing now when the channel is live and it has already
            // started (start ≤ now); everything after is genuinely upcoming.
            const onNow = live && i === 0 && entry.startMs <= Date.now();
            return (
              <li key={`${entry.startMs}-${entry.title}`} className="flex items-baseline gap-3 text-sm">
                <span
                  className={cn(
                    "w-16 shrink-0 font-mono text-xs tabular-nums",
                    onNow ? "text-foreground" : "text-muted-foreground",
                  )}
                >
                  {formatEpgTime(entry.startMs)}
                </span>
                <span className="min-w-0 flex-1 truncate">{entry.title}</span>
                {onNow && <Badge variant="onair">Now</Badge>}
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
};

export { ChannelUpcoming };
