import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { formatEpgTime } from "@loomarr/core/format";
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

  return (
    <section className={cn("flex flex-col gap-2", className)}>
      <h3 className="font-medium text-muted-foreground text-xs uppercase tracking-wide">Up next</h3>
      {entries.length === 0 ? (
        // The API deliberately lowers no guide, an empty schedule, and a backend read failure to
        // the same empty list. Do not infer broadcast state from a response that cannot prove it.
        <p className="text-muted-foreground text-sm">Programme information isn't available.</p>
      ) : (
        <ol className="flex flex-col gap-0.5">
          {entries.map((entry, i) => {
            // The first entry is what's airing now when the channel is live and it has already
            // started (start ≤ now); everything after is genuinely upcoming.
            const onNow = live && i === 0 && entry.startMs <= Date.now();
            return (
              <li
                key={`${entry.startMs}-${entry.title}`}
                className={cn(
                  "flex items-baseline gap-3 rounded-md px-2 py-1.5",
                  onNow ? "bg-signal-tint-10" : null,
                )}
              >
                <span
                  className={cn(
                    "w-16 shrink-0 font-mono text-xs tabular-nums",
                    onNow ? "font-medium text-signal" : "text-muted-foreground",
                  )}
                >
                  {onNow ? "Now" : formatEpgTime(entry.startMs)}
                </span>
                <span className={cn("min-w-0 flex-1 truncate text-sm", onNow && "font-medium")}>
                  {entry.title}
                </span>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
};

export { ChannelUpcoming };
